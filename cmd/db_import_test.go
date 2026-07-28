package cmd

import (
	"strings"
	"testing"
)

func TestParseImportJSONObject(t *testing.T) {
	raw := `{"exported_at":"2024-01-01T00:00:00Z","entries":[{"platform":"custom","program_url":"custom","target_raw":"a.example.com","category":"wildcard","in_scope":true,"is_bbp":false}]}`
	entries, err := parseimportJSON(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseimportJSON object: %v", err)
	}
	if len(entries) != 1 || entries[0].TargetRaw != "a.example.com" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestParseImportJSONArrayFallback(t *testing.T) {
	raw := `[{"platform":"custom","program_url":"custom","target_raw":"b.example.com","category":"domain","in_scope":true,"is_bbp":false}]`
	entries, err := parseimportJSON(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseimportJSON array fallback: %v", err)
	}
	if len(entries) != 1 || entries[0].TargetRaw != "b.example.com" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}
