package scope

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

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

func sampleProgram() ProgramData {
	return ProgramData{
		Url: "https://h1/example",
		InScope: []ScopeElement{
			{Target: "*.example.com", Description: "main", Category: "wildcard"},
			{Target: "https://api.example.com", Description: "api", Category: "website"},
		},
		OutOfScope: []ScopeElement{
			{Target: "blog.example.com", Description: "excluded", Category: "website"},
		},
	}
}

func TestPrintProgramScope_InScopeOnly(t *testing.T) {
	out := captureStdout(t, func() {
		PrintProgramScope(sampleProgram(), "tu", " ", false)
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 in-scope lines, got %d: %q", len(lines), out)
	}
	if lines[0] != "*.example.com https://h1/example" {
		t.Errorf("unexpected first line: %q", lines[0])
	}
	if strings.Contains(out, "[OOS]") {
		t.Errorf("OOS lines should be omitted when includeOOS=false: %q", out)
	}
}

func TestPrintProgramScope_WithOOSAndCategory(t *testing.T) {
	out := captureStdout(t, func() {
		// -o tc: target + (unified) category
		PrintProgramScope(sampleProgram(), "tc", " ", true)
	})
	// "website" must be normalized to the unified "url" category.
	if !strings.Contains(out, "https://api.example.com url") {
		t.Errorf("expected website category normalized to url: %q", out)
	}
	if !strings.Contains(out, "[OOS] blog.example.com url") {
		t.Errorf("expected OOS line with unified category: %q", out)
	}
}
