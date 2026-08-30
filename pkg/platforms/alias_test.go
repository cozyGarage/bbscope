package platforms

import (
	"slices"
	"testing"
)

func TestCanonicalName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"HackerOne", "h1"},
		{"h1", "h1"},
		{" bugcrowd ", "bc"},
		{"yeswehack", "ywh"},
		{"immunefi", "immunefi"},
		{"custom", "custom"},
		{"dev", "dev"},
		{"Test_Platform", "test_platform"},
		{"", ""},
		{"all", "all"},
	}
	for _, tc := range tests {
		if got := CanonicalName(tc.in); got != tc.want {
			t.Errorf("CanonicalName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestKnownPlatform(t *testing.T) {
	if !KnownPlatform("hackerone") || !KnownPlatform("BC") || !KnownPlatform("custom") {
		t.Fatal("expected built-in aliases to be known")
	}
	if KnownPlatform("github") || KnownPlatform("") {
		t.Fatal("unknown names must not be KnownPlatform")
	}
}

func TestMatchingNames(t *testing.T) {
	got := MatchingNames("bugcrowd")
	if !slices.Contains(got, "bc") || !slices.Contains(got, "bugcrowd") {
		t.Fatalf("MatchingNames(bugcrowd) = %v, want bc and bugcrowd", got)
	}
	got = MatchingNames("h1")
	if !slices.Contains(got, "h1") || !slices.Contains(got, "hackerone") {
		t.Fatalf("MatchingNames(h1) = %v, want h1 and hackerone", got)
	}
	got = MatchingNames("Test_XYZ")
	if len(got) != 1 || got[0] != "test_xyz" {
		t.Fatalf("unknown platform should pass through lowercased, got %v", got)
	}
	if MatchingNames("all") != nil || MatchingNames("") != nil {
		t.Fatal("all/empty should yield no filter tokens")
	}
}
