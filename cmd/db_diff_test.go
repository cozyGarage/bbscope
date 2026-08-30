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

func TestCSVEscape(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"has,comma", `"has,comma"`},
		{`has"quote`, `"has""quote"`},
		{"", ""},
	}
	for _, tc := range tests {
		if got := csvEscape(tc.in); got != tc.want {
			t.Errorf("csvEscape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOutputDiffCSV(t *testing.T) {
	out := captureStdout(t, func() { outputDiffCSV(sampleChanges()) })
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if lines[0] != "type,platform,target,category,program,time" {
		t.Fatalf("unexpected CSV header: %q", lines[0])
	}
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows, got %d lines: %q", len(lines), out)
	}
	if !strings.Contains(lines[1], "added,h1,a.example.com,url,https://h1/p,") {
		t.Errorf("unexpected first row: %q", lines[1])
	}
	// The comma in the target must be CSV-quoted.
	if !strings.Contains(lines[2], `"b,comma.example.com"`) {
		t.Errorf("comma target not quoted: %q", lines[2])
	}
}

func TestOutputDiffJSON(t *testing.T) {
	out := captureStdout(t, func() { outputDiffJSON(sampleChanges()) })
	if !strings.HasPrefix(strings.TrimSpace(out), "[") || !strings.HasSuffix(strings.TrimSpace(out), "]") {
		t.Fatalf("JSON output not bracketed: %q", out)
	}
	if !strings.Contains(out, `"type": "added"`) || !strings.Contains(out, `"type": "removed"`) {
		t.Errorf("expected both change types in JSON, got %q", out)
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
