package pollrun

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/ai"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

func names(pollers []platforms.PlatformPoller) []string {
	out := make([]string, 0, len(pollers))
	for _, p := range pollers {
		out = append(out, p.Name())
	}
	return out
}

// newRunner builds a runner with no database, the mode used for streaming
// results straight to the caller.
func newRunner(concurrency int, opts ...func(*Options)) *runner {
	o := Options{Concurrency: concurrency}
	for _, fn := range opts {
		fn(&o)
	}
	return &runner{opts: o}
}

func TestProcessPrograms_AllProcessed(t *testing.T) {
	p := platforms.NewMockPoller("fake")
	p.Handles = []string{"a", "b", "c", "d", "e"}

	r := newRunner(3)
	urls, err := r.processPrograms(context.Background(), p, p.Handles, nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != len(p.Handles) {
		t.Fatalf("expected %d program URLs, got %d", len(p.Handles), len(urls))
	}
	if len(p.FetchedHandles()) != len(p.Handles) {
		t.Fatalf("expected all %d handles fetched, got %d", len(p.Handles), len(p.FetchedHandles()))
	}
}

func TestProcessPrograms_ErrorIsolation(t *testing.T) {
	p := platforms.NewMockPoller("fake")
	p.FailOn = map[string]bool{"b": true}
	handles := []string{"a", "b", "c"}

	r := newRunner(2)
	urls, err := r.processPrograms(context.Background(), p, handles, nil, true)
	if err == nil {
		t.Fatal("expected an error to be surfaced when one handle fails")
	}
	if len(urls) != 2 {
		t.Fatalf("expected the 2 healthy handles to still succeed, got %d (%v)", len(urls), urls)
	}
}

func TestProcessPrograms_ContextCanceled(t *testing.T) {
	p := platforms.NewMockPoller("fake")
	handles := []string{"a", "b", "c", "d"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before any work starts

	r := newRunner(2)
	urls, err := r.processPrograms(ctx, p, handles, nil, true)
	if len(urls) != 0 {
		t.Fatalf("workers should bail out on a canceled context, but processed %d (%v)", len(urls), urls)
	}
	// The error matters as much as the empty list: the caller only skips
	// SyncPlatformPrograms when an error comes back.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a context.Canceled error, got %v", err)
	}
}

// cancelAfterFirstFetch cancels the poll once one handle has been fetched, which
// is the dangerous shape of cancellation: the result list is partial rather than
// empty.
type cancelAfterFirstFetch struct {
	*platforms.MockPoller
	cancel func()
	once   sync.Once
}

func (c *cancelAfterFirstFetch) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	pd, err := c.MockPoller.FetchProgramScope(ctx, handle, opts)
	c.once.Do(c.cancel)
	return pd, err
}

// TestProcessPrograms_CancelMidFlightReturnsError covers a Ctrl-C partway
// through a poll. Workers used to return without recording an error, so a
// truncated URL list was handed back as success and the caller went on to sync
// it — disabling every program that had not been fetched yet.
func TestProcessPrograms_CancelMidFlightReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	base := platforms.NewMockPoller("fake")
	p := &cancelAfterFirstFetch{MockPoller: base, cancel: cancel}
	handles := []string{"a", "b", "c", "d", "e", "f", "g", "h"}

	// Concurrency 1 so the cancellation lands before the remaining handles run.
	r := newRunner(1)
	urls, err := r.processPrograms(ctx, p, handles, nil, true)
	if err == nil {
		t.Fatal("a canceled poll must surface an error so the caller skips platform sync")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a context.Canceled error, got %v", err)
	}
	if len(urls) == len(handles) {
		t.Fatalf("expected a partial result list, got all %d handles", len(urls))
	}
}

// TestRunStreamsScopeWithoutDatabase covers the read-only mode the CLI uses
// when --db is absent: every fetched program is handed to OnScope.
func TestRunStreamsScopeWithoutDatabase(t *testing.T) {
	p := platforms.NewMockPoller("fake")
	p.Handles = []string{"a", "b", "c"}

	var mu sync.Mutex
	var seen []string

	err := Run(context.Background(), []platforms.PlatformPoller{p}, Options{
		Concurrency: 2,
		OnScope: func(pd scope.ProgramData) {
			mu.Lock()
			seen = append(seen, pd.Url)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 programs streamed to OnScope, got %d (%v)", len(seen), seen)
	}
}

// TestRunReportsProgress pins the callback the TUI relies on to render a
// progress indicator.
func TestRunReportsProgress(t *testing.T) {
	p := platforms.NewMockPoller("fake")
	p.Handles = []string{"a", "b", "c", "d"}

	var (
		mu         sync.Mutex
		updates    []Progress
		platforms_ []string
	)

	err := Run(context.Background(), []platforms.PlatformPoller{p}, Options{
		Concurrency: 1,
		OnPlatformStart: func(name string) {
			mu.Lock()
			platforms_ = append(platforms_, name)
			mu.Unlock()
		},
		OnProgress: func(pr Progress) {
			mu.Lock()
			updates = append(updates, pr)
			mu.Unlock()
		},
		OnScope: func(scope.ProgramData) {},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(platforms_) != 1 || platforms_[0] != "fake" {
		t.Fatalf("expected one platform-start callback for 'fake', got %v", platforms_)
	}
	if len(updates) != 4 {
		t.Fatalf("expected 4 progress updates, got %d", len(updates))
	}
	// Concurrency 1 makes the sequence deterministic.
	for i, u := range updates {
		if u.Total != 4 {
			t.Errorf("update %d: Total = %d, want 4", i, u.Total)
		}
		if u.Completed != i+1 {
			t.Errorf("update %d: Completed = %d, want %d", i, u.Completed, i+1)
		}
		if u.Platform != "fake" {
			t.Errorf("update %d: Platform = %q, want %q", i, u.Platform, "fake")
		}
	}
}

// TestRunProgressCountsFailures confirms progress advances even for programs
// that fail, so a TUI progress bar cannot stall short of its total.
func TestRunProgressCountsFailures(t *testing.T) {
	p := platforms.NewMockPoller("fake")
	p.Handles = []string{"a", "b", "c"}
	p.FailOn = map[string]bool{"b": true}

	var (
		mu   sync.Mutex
		last Progress
		n    int
	)

	_ = Run(context.Background(), []platforms.PlatformPoller{p}, Options{
		Concurrency: 1,
		OnProgress: func(pr Progress) {
			mu.Lock()
			last = pr
			n++
			mu.Unlock()
		},
		OnScope: func(scope.ProgramData) {},
	})

	if n != 3 {
		t.Fatalf("expected 3 progress updates including the failure, got %d", n)
	}
	if last.Completed != last.Total {
		t.Errorf("progress finished at %d/%d; a failed fetch must still advance it", last.Completed, last.Total)
	}
}

// TestRunRejectsAIWithoutDatabase guards the one option combination that cannot
// work, since enhancements are read from and written to the database.
func TestRunRejectsAIWithoutDatabase(t *testing.T) {
	err := Run(context.Background(), nil, Options{AI: stubNormalizer{}})
	if err == nil {
		t.Fatal("expected AI without a database to be rejected")
	}
}

type stubNormalizer struct{}

func (stubNormalizer) NormalizeTargets(context.Context, ai.ProgramInfo, []storage.TargetItem) ([]storage.TargetItem, error) {
	return nil, nil
}

func TestOptionsDefaults(t *testing.T) {
	o := Options{}
	if got := o.concurrency(); got != defaultConcurrency {
		t.Errorf("concurrency() = %d, want the default %d", got, defaultConcurrency)
	}
	o.Concurrency = -1
	if got := o.concurrency(); got != defaultConcurrency {
		t.Errorf("negative concurrency should fall back to the default, got %d", got)
	}
	o.Concurrency = 9
	if got := o.concurrency(); got != 9 {
		t.Errorf("concurrency() = %d, want 9", got)
	}
}

func TestParseSince(t *testing.T) {
	if got, err := ParseSince(""); err != nil || !got.IsZero() {
		t.Errorf("empty --since should yield the zero time, got %v, %v", got, err)
	}
	got, err := ParseSince("2024-01-02T03:04:05Z")
	if err != nil {
		t.Fatalf("ParseSince: %v", err)
	}
	if got.Year() != 2024 || got.Month() != 1 || got.Day() != 2 {
		t.Errorf("unexpected parsed time: %v", got)
	}
	if _, err := ParseSince("yesterday"); err == nil {
		t.Error("expected an error for a non-RFC3339 value")
	}
}
