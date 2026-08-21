package cmd

import (
	"context"
	"testing"
)

func TestParsePlatformFilter(t *testing.T) {
	if got := parsePlatformFilter("all"); got != nil {
		t.Fatalf("all should mean no filter, got %v", got)
	}
	if got := parsePlatformFilter(""); got != nil {
		t.Fatalf("empty should mean no filter, got %v", got)
	}
	got := parsePlatformFilter("h1, bugcrowd, IT")
	if !got["h1"] || !got["bc"] || !got["it"] {
		t.Fatalf("unexpected filter: %v", got)
	}
	if got["ywh"] {
		t.Fatalf("ywh should not be present: %v", got)
	}
}

func TestBuildPollersFromConfigFailsFastOnBadProxy(t *testing.T) {
	_, err := buildPollersFromConfig(context.Background(), "http://[::1", nil)
	if err == nil {
		t.Fatal("expected an error for an invalid --proxy value, got nil")
	}
}
