package cmd

import (
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
