package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

func TestPrintChangesRespectsSince(t *testing.T) {
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	changes := []storage.Change{
		{OccurredAt: old, Platform: "h1", ProgramURL: "https://h1.example/old", TargetRaw: "old.example", ChangeType: "added", InScope: true},
		{OccurredAt: recent, Platform: "h1", ProgramURL: "https://h1.example/new", TargetRaw: "new.example", ChangeType: "added", InScope: true},
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	printChanges(changes, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	_ = w.Close()
	os.Stdout = stdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "old.example") {
		t.Fatalf("old change should be filtered by --since, got: %s", out)
	}
	if !strings.Contains(out, "new.example") {
		t.Fatalf("recent change should be printed, got: %s", out)
	}
}

func TestFilterChangesForOutputDropsBaseWhenVariantExists(t *testing.T) {
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	changes := []storage.Change{
		{OccurredAt: now, Platform: "h1", ProgramURL: "https://h1/p", TargetRaw: "ex.com", ChangeType: "added"},
		{OccurredAt: now, Platform: "h1", ProgramURL: "https://h1/p", TargetRaw: "ex.com", TargetAINormalized: "ex.com", ChangeType: "added"},
	}
	got := filterChangesForOutput(changes, time.Time{})
	if len(got) != 1 {
		t.Fatalf("expected only the variant row, got %d", len(got))
	}
	if got[0].TargetAINormalized == "" {
		t.Fatal("the remaining row should be the AI variant")
	}
}
