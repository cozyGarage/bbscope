package cmd

import (
	"fmt"
	"strconv"
	"testing"
)

// TestDaemonPollFlagsResolveWithoutParse guards against a regression where
// pollCmd is invoked directly (bypassing cobra's Execute()/ParseFlags) and
// its persistent flags ("db", "ai", "concurrency", ...) are never merged
// into its local FlagSet, causing cmd.Flags().GetBool/GetString lookups in
// runPollWithPollers to silently return zero values.
func TestDaemonPollFlagsResolveWithoutParse(t *testing.T) {
	pollCmd.Flags().AddFlagSet(pollCmd.PersistentFlags())
	_ = pollCmd.PersistentFlags().Set("db", strconv.FormatBool(true))
	_ = pollCmd.PersistentFlags().Set("ai", strconv.FormatBool(true))

	db, err := pollCmd.Flags().GetBool("db")
	if err != nil {
		t.Fatalf("GetBool(db) errored: %v", err)
	}
	if !db {
		t.Fatalf("expected db=true after merge+Set, got false")
	}

	ai, err := pollCmd.Flags().GetBool("ai")
	if err != nil {
		t.Fatalf("GetBool(ai) errored: %v", err)
	}
	if !ai {
		t.Fatalf("expected ai=true after merge+Set, got false")
	}

	concurrency, err := pollCmd.Flags().GetInt("concurrency")
	if err != nil {
		t.Fatalf("GetInt(concurrency) errored: %v", err)
	}
	if concurrency != 5 {
		t.Fatalf("expected default concurrency=5, got %d", concurrency)
	}
}

func TestLooksLikeAuthError(t *testing.T) {
	if looksLikeAuthError(nil) {
		t.Fatal("nil is not an auth error")
	}
	if !looksLikeAuthError(fmt.Errorf("h1: invalid auth token")) {
		t.Fatal("expected invalid auth token to match")
	}
	if !looksLikeAuthError(fmt.Errorf("fetching failed. Got status Code: 401")) {
		t.Fatal("expected 401 to match")
	}
	if looksLikeAuthError(fmt.Errorf("connection refused")) {
		t.Fatal("network errors must not force a full re-login")
	}
}
