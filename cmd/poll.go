package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/ai"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/pollrun"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// pollCmd implements: bbscope poll
//
// Uses global flags from root (proxy, loglevel) plus the persistent flags
// declared below, which every poll subcommand inherits.
var pollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Poll platforms and fetch scopes",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown command: '%s'. See 'bbscope poll --help'", args[0])
		}

		proxyURL, _ := cmd.Flags().GetString("proxy")
		// Parent poll includes every configured real platform.
		pollers, err := pollrun.BuildPollers(cmd.Context(), proxyURL, nil)
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

// runPollWithPollers translates the command's flags into pollrun.Options and
// runs the poll, wiring the CLI's printing and notification behavior in as
// callbacks. The orchestration itself lives in pkg/pollrun so the TUI can drive
// the same code path.
func runPollWithPollers(cmd *cobra.Command, pollers []platforms.PlatformPoller) error {
	useDB, _ := cmd.Flags().GetBool("db")
	useAI, _ := cmd.Flags().GetBool("ai")
	sinceStr, _ := cmd.Flags().GetString("since")

	if sinceStr != "" && !useDB {
		return fmt.Errorf("--since requires --db")
	}
	since, err := pollrun.ParseSince(sinceStr)
	if err != nil {
		return err
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
	if useAI && useDB {
		aiNormalizer, err = buildAINormalizer()
		if err != nil {
			return err
		}
	}

	// Change detection only happens on the --db path, so there is nothing to
	// notify about without it. Warn rather than fail, since a config file shared
	// between DB and non-DB invocations is a reasonable setup.
	notifier := loadChangeNotifier()
	if notifier != nil && !useDB {
		utils.Log.Warn("Notifications are configured but require --db, which detects the changes to notify about")
		notifier = nil
	}

	// Use the command's context so SIGINT / cobra cancelation propagates to
	// in-flight polling work. Fall back to a background context if unset.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	categories, _ := cmd.Flags().GetString("category")
	bbpOnly, _ := cmd.Flags().GetBool("bbp-only")
	pvtOnly, _ := cmd.Flags().GetBool("private-only")
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	output, _ := cmd.Flags().GetString("output")
	delimiter, _ := cmd.Flags().GetString("delimiter")
	oos, _ := cmd.Flags().GetBool("oos")

	return pollrun.Run(ctx, pollers, pollrun.Options{
		Categories:  categories,
		BountyOnly:  bbpOnly,
		PrivateOnly: pvtOnly,
		Concurrency: concurrency,
		DB:          db,
		AI:          aiNormalizer,

		OnScope: func(pd scope.ProgramData) {
			scope.PrintProgramScope(pd, output, delimiter, oos)
		},
		OnChanges: func(ctx context.Context, changes []storage.Change) {
			printChanges(changes, since)
			notifier.Dispatch(ctx, changes)
		},
	})
}

// buildAINormalizer assembles the AI configuration from viper and flags.
func buildAINormalizer() (ai.Normalizer, error) {
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
	return ai.NewNormalizer(cfg)
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
