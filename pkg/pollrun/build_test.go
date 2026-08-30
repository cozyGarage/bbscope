package pollrun

import (
	"context"
	"testing"
)

func TestParsePlatformFilter(t *testing.T) {
	if got := ParsePlatformFilter("all"); got != nil {
		t.Fatalf("all should mean no filter, got %v", got)
	}
	if got := ParsePlatformFilter(""); got != nil {
		t.Fatalf("empty should mean no filter, got %v", got)
	}
	got := ParsePlatformFilter("h1, bugcrowd, IT")
	if !got["h1"] || !got["bc"] || !got["it"] {
		t.Fatalf("unexpected filter: %v", got)
	}
	if got["ywh"] {
		t.Fatalf("ywh should not be present: %v", got)
	}
}

func TestBuildPollersFailsFastOnBadProxy(t *testing.T) {
	_, err := BuildPollers(context.Background(), "http://[::1", nil)
	if err == nil {
		t.Fatal("expected an error for an invalid --proxy value, got nil")
	}
}

// TestBuildPollersExcludesDevUnlessRequested pins the fix for a plain
// `bbscope poll` running the sample-data poller. The dev poller emits synthetic
// example.com programs, which with --db were written into the user's database
// alongside their real scope.
func TestBuildPollersExcludesDevUnlessRequested(t *testing.T) {
	ctx := context.Background()

	unfiltered, err := BuildPollers(ctx, "", nil)
	if err != nil {
		t.Fatalf("BuildPollers: %v", err)
	}
	for _, p := range unfiltered {
		if p.Name() == "dev" {
			t.Fatal("the dev poller must not be included in an unfiltered poll")
		}
	}

	// Immunefi needs no credentials, so an unfiltered build is never empty;
	// this confirms the test above is not passing on an empty list.
	if len(unfiltered) == 0 {
		t.Fatal("expected at least the credential-free Immunefi poller")
	}

	explicit, err := BuildPollers(ctx, "", map[string]bool{"dev": true})
	if err != nil {
		t.Fatalf("BuildPollers(dev): %v", err)
	}
	if len(explicit) != 1 || explicit[0].Name() != "dev" {
		t.Fatalf("asking for dev by name should yield only the dev poller, got %v", names(explicit))
	}
}

// TestBuildPollersRespectsFilter confirms an explicit filter excludes platforms
// that need no credentials, not just ones that are skipped for lacking them.
func TestBuildPollersRespectsFilter(t *testing.T) {
	pollers, err := BuildPollers(context.Background(), "", map[string]bool{"immunefi": true})
	if err != nil {
		t.Fatalf("BuildPollers: %v", err)
	}
	if len(pollers) != 1 || pollers[0].Name() != "immunefi" {
		t.Fatalf("expected only immunefi, got %v", names(pollers))
	}
}
