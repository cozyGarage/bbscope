package cmd

import (
	"strings"
	"testing"
)

func TestParseImportJSONObject(t *testing.T) {
	// Entry has no json tags, so export/import use Go field names.
	raw := `{"exported_at":"2024-01-01T00:00:00Z","entries":[{"Platform":"custom","ProgramURL":"custom","TargetRaw":"a.example.com","Category":"wildcard","InScope":true,"IsBBP":false}]}`
	entries, err := parseimportJSON(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseimportJSON object: %v", err)
	}
	if len(entries) != 1 || entries[0].TargetRaw != "a.example.com" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestParseImportJSONArrayFallback(t *testing.T) {
	raw := `[{"Platform":"custom","ProgramURL":"custom","TargetRaw":"b.example.com","Category":"domain","InScope":true,"IsBBP":false}]`
	entries, err := parseimportJSON(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseimportJSON array fallback: %v", err)
	}
	if len(entries) != 1 || entries[0].TargetRaw != "b.example.com" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}
