package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/ai"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// pollCmd implements: bbscope poll
// Flags (from REFACTOR.md):
//
//	--platform string   Comma-separated platforms or "all" (default)
//	--program string    Filter by program (handle or full URL)
//	--db                Save results to the database
//	--concurrency int   Number of concurrent fetches
//	--since string      Print changes since RFC3339 timestamp (when using --db)
//
// Uses global flags from root (proxy, output, delimiter, bbp-only, private-only, oos, loglevel)
var pollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Poll platforms and fetch scopes",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown command: '%s'. See 'bbscope poll --help'", args[0])
		}

		proxyURL, _ := cmd.Flags().GetString("proxy")
		// Parent poll includes all configured platforms (including Immunefi).
		pollers, err := buildPollersFromConfig(cmd.Context(), proxyURL, nil)
		if err != nil {
			return err
		}
		if len(pollers) == 0 {
			utils.Log.Info("No platforms to poll. Set credentials with: bbscope config set <key>")
			return nil
		}

		return runPollWithPollers(cmd, pollers)
	},
}

func init() {
	rootCmd.AddCommand(pollCmd)

	// Make common flags persistent so subcommands inherit them
	pollCmd.PersistentFlags().String("category", "all", "Scope categories to include (wildcard, url, cidr, apple, android, ai, etc.)")
	pollCmd.PersistentFlags().Bool("db", false, "Save results to the database and print changes")
	pollCmd.PersistentFlags().Int("concurrency", 5, "Number of concurrent program fetches per platform")
	pollCmd.PersistentFlags().String("since", "", "Only print changes since this RFC3339 timestamp (requires --db)")
	pollCmd.PersistentFlags().Bool("oos", false, "Include out-of-scope elements")
	pollCmd.PersistentFlags().StringP("output", "o", "tu", "Output flags. Supported: t (target), d (target description), c (category), u (program URL). Can be combined. Example: -o tdu")
	pollCmd.PersistentFlags().StringP("delimiter", "d", " ", "Delimiter character to use for txt output format")
	pollCmd.PersistentFlags().BoolP("bbp-only", "b", false, "Only fetch programs offering monetary rewards")
	pollCmd.PersistentFlags().BoolP("private-only", "p", false, "Only fetch data from private programs")
	pollCmd.PersistentFlags().Bool("ai", false, "Enable LLM-assisted normalization (requires ai.api_key or OPENAI_API_KEY)")
}

// runPollWithPollers executes the polling flow using the provided pollers.
func runPollWithPollers(cmd *cobra.Command, pollers []platforms.PlatformPoller) error {
	categories, _ := cmd.Flags().GetString("category")
	useDB, _ := cmd.Flags().GetBool("db")
	useAI, _ := cmd.Flags().GetBool("ai")
	sinceStr, _ := cmd.Flags().GetString("since")

	var since time.Time
	if sinceStr != "" {
		if !useDB {
			return fmt.Errorf("--since requires --db")
		}
		parsed, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return fmt.Errorf("invalid --since, need RFC3339: %w", err)
		}
		since = parsed
	}

	var db *storage.DB
	if useDB {
		dbURL, err := GetDBConnectionString()
		if err != nil {
			return err
		}
		db, err = storage.Open(dbURL)
		if err != nil {
			return err
		}
		defer db.Close()
	}

	if useAI && !useDB {
		utils.Log.Warn("--ai flag currently only affects --db workflows; enable --db to persist normalized results")
	}

	var aiNormalizer ai.Normalizer
	if useAI {
		proxyURL, _ := rootCmd.Flags().GetString("proxy")
		cfg := ai.Config{
			Provider:           strings.TrimSpace(viper.GetString("ai.provider")),
			APIKey:             strings.TrimSpace(viper.GetString("ai.api_key")),
			Model:              strings.TrimSpace(viper.GetString("ai.model")),
			MaxBatch:           viper.GetInt("ai.max_batch"),
			MaxConcurrency:     viper.GetInt("ai.max_concurrency"),
			Endpoint:           strings.TrimSpace(viper.GetString("ai.endpoint")),
			Proxy:              strings.TrimSpace(viper.GetString("ai.proxy")),
			InsecureSkipVerify: viper.GetBool("ai.insecure_skip_verify"),
		}
		// Command-line proxy flag takes precedence over config file
		if proxyURL != "" {
			cfg.Proxy = proxyURL
		}
		if cfg.APIKey == "" {
			cfg.APIKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		}

		normalizer, err := ai.NewNormalizer(cfg)
		if err != nil {
			return err
		}
		aiNormalizer = normalizer
	}

	// Use the command's context so SIGINT / cobra cancelation propagates to
	// in-flight polling work. Fall back to a background context if unset.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	if concurrency <= 0 {
		concurrency = 5 // Default to 5 if invalid
	}

	for _, p := range pollers {
		utils.Log.Infof("Fetching scope from %s...", p.Name())

		bbpOnly, _ := cmd.Flags().GetBool("bbp-only")
		pvtOnly, _ := cmd.Flags().GetBool("private-only")
		opts := platforms.PollOptions{
			Categories:  categories,
			BountyOnly:  bbpOnly,
			PrivateOnly: pvtOnly,
		}

		isFirstRunForPlatform := false
		if useDB {
			programCount, err := db.GetActiveProgramCount(ctx, p.Name())
			if err != nil {
				// Don't fail the whole run, but we can't do the "first run" check.
				utils.Log.Warnf("Could not get program count for %s: %v", p.Name(), err)
			} else {
				isFirstRunForPlatform = programCount == 0
			}
		}

		var ignoredPrograms map[string]bool
		if useDB {
			rawIgnored, err := db.GetIgnoredPrograms(ctx, p.Name())
			if err != nil {
				utils.Log.Warnf("Could not get ignored programs for %s: %v", p.Name(), err)
				ignoredPrograms = make(map[string]bool) // Continue with an empty map
			} else {
				ignoredPrograms = make(map[string]bool, len(rawIgnored))
				for u, v := range rawIgnored {
					if !v {
						continue
					}
					ignoredPrograms[u] = true
					if n := storage.NormalizeProgramURL(u); n != "" {
						ignoredPrograms[n] = true
					}
				}
			}
		}

		handles, err := p.ListProgramHandles(ctx, opts)
		if err != nil {
			return err
		}

		if isFirstRunForPlatform && len(handles) > 0 {
			utils.Log.Infof("First poll for %s, populating database...", p.Name())
		}

		if useDB {
			dbProgramCount, err := db.GetActiveProgramCount(ctx, p.Name())
			if err != nil {
				utils.Log.Warnf("Could not get program count for %s: %v", p.Name(), err)
			}

			// PLATFORM-LEVEL SAFETY CHECK: If the poller returns 0 programs, but we have many in the DB,
			// it's likely the poller failed or there's a temporary API issue. We abort the sync
			// for this platform to prevent wiping all its programs.
			if len(handles) == 0 && dbProgramCount > 10 { // Using a threshold > 10
				utils.Log.Errorf("Poller for %s returned 0 programs, but database has %d. Aborting sync for this platform to prevent data loss.", p.Name(), dbProgramCount)
				continue // Skip to the next platform
			}
		}

		// Use concurrent processing with worker pool pattern
		polledProgramURLs, err := processProgramsConcurrently(ctx, cmd, p, handles, opts, useDB, db, ignoredPrograms, isFirstRunForPlatform, concurrency, aiNormalizer, since)
		if err != nil {
			// Do not abort remaining platforms, and skip SyncPlatformPrograms: a partial
			// success list would incorrectly disable programs that only failed to fetch.
			utils.Log.Warnf("Some program fetches failed for %s: %v; skipping platform sync for this run", p.Name(), err)
			continue
		}

		if useDB {
			// After processing all programs for a platform, sync the state.
			// This will mark any programs that were not in the latest poll as disabled.
			removedProgramChanges, err := db.SyncPlatformPrograms(ctx, p.Name(), polledProgramURLs)
			if err != nil {
				if errors.Is(err, storage.ErrAbortingPartialSync) {
					utils.Log.Errorf("Skipping platform sync for %s: %v", p.Name(), err)
				} else {
					utils.Log.Warnf("Failed to sync removed programs for platform %s: %v", p.Name(), err)
				}
			}
			if !isFirstRunForPlatform {
				printChanges(removedProgramChanges, since)
			}
			if !isFirstRunForPlatform {
				if err := db.LogChanges(ctx, removedProgramChanges); err != nil {
					utils.Log.Warnf("Could not log removed program changes for platform %s: %v", p.Name(), err)
				}
			}
		}
	}
	return nil
}

// processProgramsConcurrently processes programs using a worker pool pattern for concurrent fetching.
func processProgramsConcurrently(ctx context.Context, cmd *cobra.Command, p platforms.PlatformPoller, handles []string, opts platforms.PollOptions, useDB bool, db *storage.DB, ignoredPrograms map[string]bool, isFirstRunForPlatform bool, concurrency int, aiNormalizer ai.Normalizer, since time.Time) ([]string, error) {
	if len(handles) == 0 {
		return []string{}, nil
	}

	// Channel to distribute work
	handleChan := make(chan string, len(handles))

	// Results collection with mutex protection
	var mu sync.Mutex
	polledProgramURLs := make([]string, 0, len(handles))
	var firstError error
	var errorMu sync.Mutex

	// Worker pool
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for h := range handleChan {
				// Stop pulling new work once the context is done. Record the
				// cancellation as an error: the caller uses a non-nil error to skip
				// SyncPlatformPrograms, and syncing against the truncated URL list a
				// canceled run produces would disable every program not yet fetched.
				if err := ctx.Err(); err != nil {
					errorMu.Lock()
					if firstError == nil {
						firstError = err
					}
					errorMu.Unlock()
					return
				}
				pd, err := p.FetchProgramScope(ctx, h, opts)
				if err != nil {
					// Log error but continue processing other programs
					utils.Log.Warnf("Failed to fetch scope for %s: %v", h, err)
					errorMu.Lock()
					if firstError == nil {
						firstError = err // Store first error but don't stop processing
					}
					errorMu.Unlock()
					continue
				}

				if useDB && (ignoredPrograms[pd.Url] || ignoredPrograms[storage.NormalizeProgramURL(pd.Url)]) {
					utils.Log.Debugf("Skipping ignored program: %s", pd.Url)
					continue
				}

				// Add to polled URLs (thread-safe); normalize so sync identity matches upsert.
				mu.Lock()
				polledProgramURLs = append(polledProgramURLs, storage.NormalizeProgramURL(pd.Url))
				mu.Unlock()

				if !useDB {
					output, _ := cmd.Flags().GetString("output")
					delimiter, _ := cmd.Flags().GetString("delimiter")
					oos, _ := cmd.Flags().GetBool("oos")
					scope.PrintProgramScope(pd, output, delimiter, oos)
					continue
				}

				// Process database operations
				var allItems []storage.TargetItem
				for _, s := range pd.InScope {
					allItems = append(allItems, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: true})
				}
				for _, s := range pd.OutOfScope {
					allItems = append(allItems, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: false})
				}

				processedItems := make([]storage.TargetItem, 0, len(allItems))
				var aiCandidates []storage.TargetItem
				var aiEnhancements map[string][]storage.TargetVariant

				if aiNormalizer != nil && len(allItems) > 0 {
					var err error
					aiEnhancements, err = db.ListAIEnhancements(ctx, pd.Url)
					if err != nil {
						utils.Log.Warnf("Failed to load AI enhancements for %s: %v", pd.Url, err)
						aiEnhancements = nil
					}
				}

				if aiNormalizer != nil && len(allItems) > 0 {
					aiCandidates = make([]storage.TargetItem, 0, len(allItems))
					for _, item := range allItems {
						key := storage.BuildTargetCategoryKey(item.URI, item.Category)
						if variants, ok := aiEnhancements[key]; ok && len(variants) > 0 {
							clone := item
							clone.Variants = append([]storage.TargetVariant(nil), variants...)
							processedItems = append(processedItems, clone)
							continue
						}
						aiCandidates = append(aiCandidates, item)
					}

					if len(aiCandidates) > 0 {
						normalized, err := aiNormalizer.NormalizeTargets(ctx, ai.ProgramInfo{
							ProgramURL: pd.Url,
							Platform:   p.Name(),
							Handle:     h,
						}, aiCandidates)
						if err != nil {
							utils.Log.Warnf("AI normalization failed for %s: %v", pd.Url, err)
							processedItems = append(processedItems, aiCandidates...)
						} else if len(normalized) > 0 {
							processedItems = append(processedItems, normalized...)
						} else {
							processedItems = append(processedItems, aiCandidates...)
						}
					}

					// if there were no candidates but also no pre-existing enhancements,
					// ensure raw items still get processed
					if len(processedItems) == 0 {
						processedItems = append(processedItems, allItems...)
					}
				} else if len(processedItems) == 0 {
					processedItems = append(processedItems, allItems...)
				}

				entries, err := storage.BuildEntries(pd.Url, p.Name(), h, processedItems)
				if err != nil {
					errorMu.Lock()
					if firstError == nil {
						firstError = err
					}
					errorMu.Unlock()
					continue
				}

				changes, err := db.UpsertProgramEntries(ctx, storage.NormalizeProgramURL(pd.Url), p.Name(), h, entries)

				if err != nil {
					if errors.Is(err, storage.ErrAbortingScopeWipe) {
						utils.Log.Warnf("Potential scope wipe detected for program %s. Skipping update. This might be due to a broken poller or a platform API change.", pd.Url)
						continue // Don't treat this as a fatal error for the whole poll
					}
					// For other errors, log but continue processing
					utils.Log.Warnf("Database error for program %s: %v", pd.Url, err)
					errorMu.Lock()
					if firstError == nil {
						firstError = err
					}
					errorMu.Unlock()
					continue
				}

				// Print changes (thread-safe - fmt.Printf is safe for concurrent use)
				if !isFirstRunForPlatform {
					printChanges(changes, since)
				}
				if !isFirstRunForPlatform {
					if err := db.LogChanges(ctx, changes); err != nil {
						utils.Log.Warnf("Could not log changes for program %s: %v", pd.Url, err)
					}
				}
			}
		}()
	}

	// Send all handles to the channel
	for _, h := range handles {
		handleChan <- h
	}
	close(handleChan)

	// Wait for all workers to finish
	wg.Wait()

	// Return first error if any occurred, but still return the results
	// This allows partial success - some programs may have been processed successfully
	return polledProgramURLs, firstError
}

func printChanges(changes []storage.Change, since time.Time) {
	// Track which targets have variant changes (AI-normalized)
	hasVariants := make(map[string]bool)
	for _, c := range changes {
		if !since.IsZero() && c.OccurredAt.Before(since) {
			continue
		}
		if c.TargetAINormalized != "" {
			key := fmt.Sprintf("%s|%s|%s", c.Platform, c.ProgramURL, c.TargetRaw)
			hasVariants[key] = true
		}
	}

	for _, c := range changes {
		if !since.IsZero() && c.OccurredAt.Before(since) {
			continue
		}
		// Skip base target changes if there are variant changes for the same target
		if c.TargetAINormalized == "" {
			key := fmt.Sprintf("%s|%s|%s", c.Platform, c.ProgramURL, c.TargetRaw)
			if hasVariants[key] {
				continue
			}
		}

		var emoji string
		switch c.ChangeType {
		case "added":
			emoji = "🆕"
		case "removed":
			// Special case for entire program removals
			if c.Category == "program" {
				fmt.Printf("❌ Program removed: %s\n", c.ProgramURL)
				continue
			}
			emoji = "❌"
		case "updated":
			emoji = "🔄"
		}

		scopeStatus := ""
		if !c.InScope {
			scopeStatus = " [OOS]"
		}
		targetDisplay := c.TargetRaw
		if targetDisplay == "" {
			targetDisplay = c.TargetNormalized
		}
		if c.TargetAINormalized != "" {
			targetDisplay = fmt.Sprintf("%s -> %s", targetDisplay, c.TargetAINormalized)
		}
		fmt.Printf("%s  %s  %s  %s%s\n", emoji, c.Platform, c.ProgramURL, targetDisplay, scopeStatus)
	}
}
