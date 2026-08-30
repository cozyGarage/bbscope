// Package pollrun holds the platform polling orchestration: building
// authenticated pollers from stored credentials, fetching each platform's
// programs concurrently, and reconciling the results against the database.
//
// It lives outside package cmd so that both the CLI and the TUI can drive a
// poll. Package cmd imports pkg/tui, so orchestration living in cmd could never
// be reached from the TUI without an import cycle.
package pollrun

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/ai"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// defaultConcurrency is used when Options.Concurrency is not a positive number.
const defaultConcurrency = 5

// emptyPollAbortThreshold is the number of stored programs above which a poll
// returning nothing is treated as a platform failure rather than a genuine
// emptying, and the platform's sync is skipped to avoid disabling everything.
const emptyPollAbortThreshold = 10

// Progress reports how far a platform has advanced through its programs.
type Progress struct {
	Platform string
	// Completed counts programs whose fetch has finished, successfully or not.
	Completed int
	Total     int
}

// Options configures a poll run.
//
// The callbacks are invoked from the orchestrator's worker goroutines but are
// serialized against one another, so implementations do not need their own
// locking. They must not block for long, since doing so stalls the workers.
type Options struct {
	// Categories, BountyOnly and PrivateOnly are passed through to each poller.
	Categories  string
	BountyOnly  bool
	PrivateOnly bool

	// Concurrency is the number of programs fetched in parallel per platform.
	Concurrency int

	// DB enables persistence and change detection. When nil the run is
	// read-only and every fetched program is handed to OnScope instead.
	DB *storage.DB

	// AI, when set, normalizes targets before they are stored. Requires DB.
	AI ai.Normalizer

	// OnPlatformStart fires once per platform, before its programs are listed.
	OnPlatformStart func(platform string)

	// OnScope receives each fetched program when DB is nil.
	OnScope func(scope.ProgramData)

	// OnChanges receives detected changes. Only called when DB is set, and
	// never for a platform's first poll, where every target would look new.
	OnChanges func(context.Context, []storage.Change)

	// OnProgress fires after each program is fetched.
	OnProgress func(Progress)
}

func (o *Options) concurrency() int {
	if o.Concurrency <= 0 {
		return defaultConcurrency
	}
	return o.Concurrency
}

func (o *Options) pollOptions() platforms.PollOptions {
	return platforms.PollOptions{
		Categories:  o.Categories,
		BountyOnly:  o.BountyOnly,
		PrivateOnly: o.PrivateOnly,
	}
}

// runner carries the per-run state that the workers share.
type runner struct {
	opts Options

	// callbackMu serializes the Options callbacks so consumers such as the TUI
	// can mutate their own state without additional locking.
	callbackMu sync.Mutex
}

// Run polls every supplied poller in turn.
//
// A platform that fails to list its programs aborts the run. A platform whose
// individual program fetches partly fail continues, but its removal sync is
// skipped: syncing against a partial list would disable every program that
// merely failed to fetch.
func Run(ctx context.Context, pollers []platforms.PlatformPoller, opts Options) error {
	if opts.DB == nil && opts.AI != nil {
		return errors.New("AI normalization requires a database")
	}

	r := &runner{opts: opts}

	for _, p := range pollers {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.runPlatform(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) runPlatform(ctx context.Context, p platforms.PlatformPoller) error {
	db := r.opts.DB
	platform := p.Name()

	r.emitPlatformStart(platform)

	// A platform with nothing stored is being seeded, not diffed: recording
	// every target as an addition would bury later real changes.
	isFirstRun := false
	storedPrograms := 0
	if db != nil {
		count, err := db.GetActiveProgramCount(ctx, platform)
		if err != nil {
			// Not fatal, but the first-run determination is now a guess.
			utils.Log.Warnf("Could not get program count for %s: %v", platform, err)
		} else {
			storedPrograms = count
			isFirstRun = count == 0
		}
	}

	ignored := r.loadIgnoredPrograms(ctx, platform)

	handles, err := p.ListProgramHandles(ctx, r.opts.pollOptions())
	if err != nil {
		return err
	}

	if isFirstRun && len(handles) > 0 {
		utils.Log.Infof("First poll for %s, populating database...", platform)
	}

	// A platform that suddenly reports nothing while the database holds many of
	// its programs is far more likely to be broken than genuinely empty.
	if db != nil && len(handles) == 0 && storedPrograms > emptyPollAbortThreshold {
		utils.Log.Errorf("Poller for %s returned 0 programs, but database has %d. Aborting sync for this platform to prevent data loss.",
			platform, storedPrograms)
		return nil
	}

	polledURLs, fetchErr := r.processPrograms(ctx, p, handles, ignored, isFirstRun)
	if fetchErr != nil {
		utils.Log.Warnf("Some program fetches failed for %s: %v; skipping platform sync for this run", platform, fetchErr)
		return nil
	}

	if db == nil {
		return nil
	}

	removed, err := db.SyncPlatformPrograms(ctx, platform, polledURLs)
	if err != nil {
		if errors.Is(err, storage.ErrAbortingPartialSync) {
			utils.Log.Errorf("Skipping platform sync for %s: %v", platform, err)
		} else {
			utils.Log.Warnf("Failed to sync removed programs for platform %s: %v", platform, err)
		}
	}
	// SyncPlatformPrograms logs its removals transactionally. On a first run
	// there is nothing stored to remove, so this list is empty anyway.
	if !isFirstRun {
		r.emitChanges(ctx, removed)
	}

	return nil
}

// loadIgnoredPrograms returns the set of program URLs the user has marked as
// ignored, keyed by both the stored and normalized form so either matches.
//
// A lookup failure is logged and treated as "nothing ignored" rather than
// aborting the poll: fetching a program the user wanted skipped is a far
// smaller problem than refusing to poll the platform at all.
func (r *runner) loadIgnoredPrograms(ctx context.Context, platform string) map[string]bool {
	if r.opts.DB == nil {
		return nil
	}

	raw, err := r.opts.DB.GetIgnoredPrograms(ctx, platform)
	if err != nil {
		utils.Log.Warnf("Could not get ignored programs for %s: %v", platform, err)
		return map[string]bool{}
	}

	ignored := make(map[string]bool, len(raw))
	for u, v := range raw {
		if !v {
			continue
		}
		ignored[u] = true
		if n := storage.NormalizeProgramURL(u); n != "" {
			ignored[n] = true
		}
	}
	return ignored
}

// processPrograms fetches a platform's programs with a bounded worker pool.
//
// It returns the URLs successfully polled along with the first error seen.
// Results are returned even on error so the caller can decide what partial
// success means; the caller must not sync removals against a partial list.
func (r *runner) processPrograms(
	ctx context.Context,
	p platforms.PlatformPoller,
	handles []string,
	ignored map[string]bool,
	isFirstRun bool,
) ([]string, error) {
	if len(handles) == 0 {
		return []string{}, nil
	}

	handleChan := make(chan string, len(handles))
	for _, h := range handles {
		handleChan <- h
	}
	close(handleChan)

	var (
		mu         sync.Mutex
		polledURLs = make([]string, 0, len(handles))
		completed  int

		errorMu    sync.Mutex
		firstError error
	)

	recordError := func(err error) {
		errorMu.Lock()
		if firstError == nil {
			firstError = err
		}
		errorMu.Unlock()
	}

	var wg sync.WaitGroup
	for i := 0; i < r.opts.concurrency(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for h := range handleChan {
				// Stop pulling new work once the context is done, and record the
				// cancellation: the caller uses a non-nil error to skip the
				// removal sync, and syncing against the truncated URL list a
				// canceled run produces would disable everything not yet fetched.
				if err := ctx.Err(); err != nil {
					recordError(err)
					return
				}

				pd, err := p.FetchProgramScope(ctx, h, r.opts.pollOptions())

				mu.Lock()
				completed++
				done := completed
				mu.Unlock()
				r.emitProgress(Progress{Platform: p.Name(), Completed: done, Total: len(handles)})

				if err != nil {
					utils.Log.Warnf("Failed to fetch scope for %s: %v", h, err)
					recordError(err)
					continue
				}

				if r.opts.DB != nil && (ignored[pd.Url] || ignored[storage.NormalizeProgramURL(pd.Url)]) {
					utils.Log.Debugf("Skipping ignored program: %s", pd.Url)
					continue
				}

				// Normalize so the sync identity matches what the upsert stores.
				mu.Lock()
				polledURLs = append(polledURLs, storage.NormalizeProgramURL(pd.Url))
				mu.Unlock()

				if r.opts.DB == nil {
					r.emitScope(pd)
					continue
				}

				if err := r.persistProgram(ctx, p, h, pd, isFirstRun); err != nil {
					recordError(err)
				}
			}
		}()
	}

	wg.Wait()

	return polledURLs, firstError
}

// persistProgram normalizes and stores one program's scope, emitting any
// detected changes.
func (r *runner) persistProgram(
	ctx context.Context,
	p platforms.PlatformPoller,
	handle string,
	pd scope.ProgramData,
	isFirstRun bool,
) error {
	items := targetItemsFor(pd)
	processed := r.applyAINormalization(ctx, p, handle, pd, items)

	entries, err := storage.BuildEntries(pd.Url, p.Name(), handle, processed)
	if err != nil {
		return err
	}

	changes, err := r.opts.DB.UpsertProgramEntriesWithOptions(
		ctx,
		storage.NormalizeProgramURL(pd.Url),
		p.Name(),
		handle,
		entries,
		storage.UpsertOptions{SkipChangeLog: isFirstRun},
	)
	if err != nil {
		if errors.Is(err, storage.ErrAbortingScopeWipe) {
			utils.Log.Warnf("Potential scope wipe detected for program %s. Skipping update. This might be due to a broken poller or a platform API change.", pd.Url)
			// Deliberately not an error: one suspicious program must not fail
			// the whole poll.
			return nil
		}
		utils.Log.Warnf("Database error for program %s: %v", pd.Url, err)
		return err
	}

	if !isFirstRun {
		r.emitChanges(ctx, changes)
	}
	return nil
}

// targetItemsFor flattens a program's in-scope and out-of-scope elements.
func targetItemsFor(pd scope.ProgramData) []storage.TargetItem {
	items := make([]storage.TargetItem, 0, len(pd.InScope)+len(pd.OutOfScope))
	for _, s := range pd.InScope {
		items = append(items, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: true})
	}
	for _, s := range pd.OutOfScope {
		items = append(items, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: false})
	}
	return items
}

// applyAINormalization returns the items to store, reusing enhancements already
// recorded for this program and only sending the remainder to the model.
func (r *runner) applyAINormalization(
	ctx context.Context,
	p platforms.PlatformPoller,
	handle string,
	pd scope.ProgramData,
	items []storage.TargetItem,
) []storage.TargetItem {
	if r.opts.AI == nil || len(items) == 0 {
		return items
	}

	existing, err := r.opts.DB.ListAIEnhancements(ctx, pd.Url)
	if err != nil {
		utils.Log.Warnf("Failed to load AI enhancements for %s: %v", pd.Url, err)
		existing = nil
	}

	processed := make([]storage.TargetItem, 0, len(items))
	candidates := make([]storage.TargetItem, 0, len(items))
	for _, item := range items {
		key := storage.BuildTargetCategoryKey(item.URI, item.Category)
		if variants, ok := existing[key]; ok && len(variants) > 0 {
			clone := item
			clone.Variants = append([]storage.TargetVariant(nil), variants...)
			processed = append(processed, clone)
			continue
		}
		candidates = append(candidates, item)
	}

	if len(candidates) > 0 {
		normalized, err := r.opts.AI.NormalizeTargets(ctx, ai.ProgramInfo{
			ProgramURL: pd.Url,
			Platform:   p.Name(),
			Handle:     handle,
		}, candidates)
		switch {
		case err != nil:
			utils.Log.Warnf("AI normalization failed for %s: %v", pd.Url, err)
			processed = append(processed, candidates...)
		case len(normalized) > 0:
			processed = append(processed, normalized...)
		default:
			processed = append(processed, candidates...)
		}
	}

	if len(processed) == 0 {
		return items
	}
	return processed
}

func (r *runner) emitPlatformStart(platform string) {
	utils.Log.Infof("Fetching scope from %s...", platform)
	if r.opts.OnPlatformStart == nil {
		return
	}
	r.callbackMu.Lock()
	defer r.callbackMu.Unlock()
	r.opts.OnPlatformStart(platform)
}

func (r *runner) emitScope(pd scope.ProgramData) {
	if r.opts.OnScope == nil {
		return
	}
	r.callbackMu.Lock()
	defer r.callbackMu.Unlock()
	r.opts.OnScope(pd)
}

func (r *runner) emitChanges(ctx context.Context, changes []storage.Change) {
	if r.opts.OnChanges == nil || len(changes) == 0 {
		return
	}
	r.callbackMu.Lock()
	defer r.callbackMu.Unlock()
	r.opts.OnChanges(ctx, changes)
}

func (r *runner) emitProgress(pr Progress) {
	if r.opts.OnProgress == nil {
		return
	}
	r.callbackMu.Lock()
	defer r.callbackMu.Unlock()
	r.opts.OnProgress(pr)
}

// ParseSince interprets an RFC3339 --since value.
func ParseSince(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since, need RFC3339: %w", err)
	}
	return parsed, nil
}
