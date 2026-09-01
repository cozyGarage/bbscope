package cmd

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// captureStdout runs f and returns everything it wrote to os.Stdout.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	f()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

func sampleChanges() []storage.Change {
	ts := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	return []storage.Change{
		{OccurredAt: ts, Platform: "h1", ProgramURL: "https://h1/p", TargetNormalized: "a.example.com", Category: "url", ChangeType: "added"},
		{OccurredAt: ts, Platform: "h1", ProgramURL: "https://h1/p", TargetNormalized: "b,comma.example.com", Category: "url", ChangeType: "removed"},
	}
}

func TestNormalizeDiffFormat(t *testing.T) {
	for _, input := range []string{"TEXT", " json ", "csv"} {
		if _, err := normalizeDataFormat(input, "text", "json", "csv"); err != nil {
			t.Fatalf("normalizeDataFormat(%q): %v", input, err)
		}
	}
	if _, err := normalizeDataFormat("yaml", "text", "json", "csv"); err == nil {
		t.Fatal("unknown diff format must be rejected instead of falling back to text")
	}
}

// TestOutputDiffCSV parses the output rather than string-matching it, so the
// test confirms the CSV is well formed rather than that it looks a certain way.
func TestOutputDiffCSV(t *testing.T) {
	out := captureStdout(t, func() {
		if err := outputDiffCSV(sampleChanges()); err != nil {
			t.Errorf("outputDiffCSV: %v", err)
		}
	})

	records, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v\n%s", err, out)
	}
	if len(records) != 3 {
		t.Fatalf("expected header + 2 rows, got %d records: %q", len(records), out)
	}
	if got := strings.Join(records[0], ","); got != "type,platform,target,category,program,time" {
		t.Fatalf("unexpected CSV header: %q", got)
	}
	if records[1][0] != "added" || records[1][2] != "a.example.com" {
		t.Errorf("unexpected first row: %q", records[1])
	}
	// The comma in the target must survive a round trip through the parser.
	if records[2][2] != "b,comma.example.com" {
		t.Errorf("comma target did not round-trip: %q", records[2][2])
	}
}

// TestOutputDiffCSVEscapesHostileFields covers characters that broke the
// previous hand-rolled escaper.
func TestOutputDiffCSVEscapesHostileFields(t *testing.T) {
	changes := []storage.Change{{
		OccurredAt:       time.Unix(0, 0).UTC(),
		ChangeType:       "added",
		Platform:         "h1",
		TargetNormalized: "line\nbreak.example.com",
		Category:         "url",
		ProgramURL:       `quote"and,comma`,
	}}

	out := captureStdout(t, func() {
		if err := outputDiffCSV(changes); err != nil {
			t.Errorf("outputDiffCSV: %v", err)
		}
	})
	records, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v\n%s", err, out)
	}
	if len(records) != 2 {
		t.Fatalf("expected header + 1 row, got %d", len(records))
	}
	if records[1][2] != "line\nbreak.example.com" {
		t.Errorf("newline in target did not round-trip: %q", records[1][2])
	}
	if records[1][4] != `quote"and,comma` {
		t.Errorf("quote/comma in program did not round-trip: %q", records[1][4])
	}
}

// TestOutputDiffJSON checks the output parses, which the previous Printf-built
// JSON did not guarantee.
func TestOutputDiffJSON(t *testing.T) {
	out := captureStdout(t, func() {
		if err := outputDiffJSON(sampleChanges()); err != nil {
			t.Errorf("outputDiffJSON: %v", err)
		}
	})

	var got []diffEntry
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Type != "added" || got[1].Type != "removed" {
		t.Errorf("unexpected change types: %+v", got)
	}
	if got[0].Target != "a.example.com" {
		t.Errorf("unexpected target: %q", got[0].Target)
	}
}

// TestOutputDiffJSONEscapesHostileFields pins the bug the hand-rolled encoder
// had: a quote in any field produced invalid JSON.
func TestOutputDiffJSONEscapesHostileFields(t *testing.T) {
	changes := []storage.Change{{
		OccurredAt:       time.Unix(0, 0).UTC(),
		ChangeType:       "added",
		Platform:         "h1",
		TargetNormalized: `has"quote`,
		Category:         "url",
		ProgramURL:       `back\slash`,
	}}

	out := captureStdout(t, func() {
		if err := outputDiffJSON(changes); err != nil {
			t.Errorf("outputDiffJSON: %v", err)
		}
	})

	var got []diffEntry
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("a quote in a field produced invalid JSON: %v\n%s", err, out)
	}
	if got[0].Target != `has"quote` || got[0].Program != `back\slash` {
		t.Errorf("fields did not round-trip: %+v", got[0])
	}
}

// TestParseDiffTo pins the inclusive end-of-day boundary. `--to 2024-02-01`
// used to parse to midnight, which excluded every change made on Feb 1.
func TestParseDiffTo(t *testing.T) {
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	got, err := parseDiffTo("2024-02-01", now)
	if err != nil {
		t.Fatalf("parseDiffTo: %v", err)
	}
	want := time.Date(2024, 2, 1, 23, 59, 59, 999999999, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseDiffTo(\"2024-02-01\") = %v, want %v", got, want)
	}

	// A change late on the named day must fall inside the range.
	lateOnFeb1 := time.Date(2024, 2, 1, 23, 30, 0, 0, time.UTC)
	if lateOnFeb1.After(got) {
		t.Errorf("a change at %v should be within an inclusive --to 2024-02-01", lateOnFeb1)
	}

	if got, err := parseDiffTo("", now); err != nil || !got.Equal(now) {
		t.Errorf("parseDiffTo(\"\") = %v, %v; want %v, nil", got, err, now)
	}

	if _, err := parseDiffTo("not-a-date", now); err == nil {
		t.Error("parseDiffTo(\"not-a-date\") should return an error")
	}
}

func TestOutputDiffText(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	out := captureStdout(t, func() { outputDiffText(sampleChanges(), from, to) })
	if !strings.Contains(out, "Summary: 1 added, 1 removed, 2 total") {
		t.Errorf("missing/incorrect summary line in text output: %q", out)
	}
	if !strings.Contains(out, "+ ") || !strings.Contains(out, "- ") {
		t.Errorf("expected +/- markers in text output: %q", out)
	}
}
