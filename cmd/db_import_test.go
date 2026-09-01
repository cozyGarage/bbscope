package cmd

import (
	"strings"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
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
	if !entries[0].InScope {
		t.Errorf("in_scope did not decode")
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

// TestParseImportJSONLegacyPascalCase covers backups written before
// storage.Entry gained json tags, when the encoder emitted Go field names.
// Those files must still restore.
func TestParseImportJSONLegacyPascalCase(t *testing.T) {
	raw := `{"entries":[{"Platform":"h1","ProgramURL":"https://h1/p","Handle":"p","TargetRaw":"legacy.example.com","TargetNormalized":"legacy.example.com","Category":"url","InScope":true,"IsBBP":true,"Description":"old backup"}]}`
	entries, err := parseimportJSON(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseimportJSON legacy: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0]
	if got.TargetRaw != "legacy.example.com" {
		t.Errorf("TargetRaw = %q, want the legacy value", got.TargetRaw)
	}
	if got.Platform != "h1" || got.Handle != "p" || got.ProgramURL != "https://h1/p" {
		t.Errorf("program fields did not decode: %+v", got)
	}
	if !got.InScope || !got.IsBBP || got.Description != "old backup" {
		t.Errorf("scope fields did not decode: %+v", got)
	}
}

func TestParseImportJSONInvalid(t *testing.T) {
	if _, err := parseimportJSON(strings.NewReader("not json")); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestNormalizeImportFormat(t *testing.T) {
	for _, input := range []string{"JSON", " csv "} {
		if _, err := normalizeDataFormat(input, "json", "csv"); err != nil {
			t.Fatalf("normalizeDataFormat(%q): %v", input, err)
		}
	}
	if _, err := normalizeDataFormat("yaml", "json", "csv"); err == nil {
		t.Fatal("unknown import format must be rejected")
	}
}

func TestGroupEntriesCanonicalizesKnownPlatformAlias(t *testing.T) {
	order, _, _ := groupEntriesForImport([]storage.Entry{{
		ProgramURL: "https://bugcrowd.com/acme",
		Platform:   "Bugcrowd",
		TargetRaw:  "example.com",
		Category:   "url",
	}})
	if len(order) != 1 || order[0].platform != "bc" {
		t.Fatalf("grouped platform = %q, want bc", order[0].platform)
	}
}

// TestParseImportCSVReadsAllColumns pins the fix for the truncated reader,
// which took only the first six columns and dropped in_scope, is_bbp,
// description and source — importing every target as in-scope regardless.
func TestParseImportCSVReadsAllColumns(t *testing.T) {
	raw := "program_url,platform,handle,target_raw,target_normalized,category,in_scope,is_bbp,description,source\n" +
		"https://h1/p,h1,p,api.example.com,api.example.com,url,true,true,a description,raw\n" +
		"https://h1/p,h1,p,oos.example.com,oos.example.com,url,false,false,,raw\n"

	entries, err := parseimportCSV(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseimportCSV: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if !entries[0].InScope || !entries[0].IsBBP {
		t.Errorf("in_scope/is_bbp not read for row 1: %+v", entries[0])
	}
	if entries[0].Description != "a description" || entries[0].Source != "raw" {
		t.Errorf("description/source not read for row 1: %+v", entries[0])
	}
	if entries[1].InScope {
		t.Errorf("out-of-scope row imported as in scope: %+v", entries[1])
	}
}

// TestParseImportCSVByHeaderName confirms columns are located by name, so a
// reordered or trimmed export still imports.
func TestParseImportCSVByHeaderName(t *testing.T) {
	raw := "category,target_raw,platform,program_url\n" +
		"wildcard,*.example.com,h1,https://h1/p\n"

	entries, err := parseimportCSV(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseimportCSV: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0]
	if got.TargetRaw != "*.example.com" || got.Category != "wildcard" || got.Platform != "h1" {
		t.Errorf("reordered columns not mapped: %+v", got)
	}
	// An absent in_scope column must not silently mean out of scope.
	if !got.InScope {
		t.Errorf("missing in_scope column should default to in scope, got %+v", got)
	}
}

func TestParseImportCSVRejectsMissingColumns(t *testing.T) {
	raw := "platform,handle\nh1,p\n"
	if _, err := parseimportCSV(strings.NewReader(raw)); err == nil {
		t.Fatal("expected an error when required columns are absent")
	}
}

func TestParseImportCSVRejectsEmptyInput(t *testing.T) {
	if _, err := parseimportCSV(strings.NewReader("")); err == nil {
		t.Fatal("expected an error for empty CSV input")
	}
}

// TestParseImportCSVQuotedFields confirms a description containing a comma
// survives the export/import round trip.
func TestParseImportCSVQuotedFields(t *testing.T) {
	raw := "program_url,platform,handle,target_raw,category,description,in_scope\n" +
		`https://h1/p,h1,p,api.example.com,url,"has, a comma",true` + "\n"

	entries, err := parseimportCSV(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseimportCSV: %v", err)
	}
	if len(entries) != 1 || entries[0].Description != "has, a comma" {
		t.Fatalf("quoted description did not round-trip: %+v", entries)
	}
}

func TestGroupEntriesForImportCollapsesAIVariants(t *testing.T) {
	_, grouped, flags := groupEntriesForImport([]storage.Entry{
		{
			ProgramURL: "https://h1/p", Platform: "h1", Handle: "p",
			TargetRaw: "https://*.ex.com/**", BaseTargetRaw: "https://*.ex.com/**",
			Category: "url", BaseCategory: "url", Source: "raw", InScope: true,
		},
		{
			ProgramURL: "https://h1/p", Platform: "h1", Handle: "p",
			TargetRaw: "https://*.ex.com/**", BaseTargetRaw: "https://*.ex.com/**",
			TargetNormalized: "ex.com", Category: "wildcard", BaseCategory: "url",
			Source: "ai", InScope: true,
		},
		{
			ProgramURL: "https://h1/p", Platform: "h1", Handle: "p",
			TargetRaw: "https://*.ex.com/**", TargetNormalized: "www.ex.com",
			Category: "url", Source: "ai", InScope: true, Disabled: true,
		},
	})
	key := programKey{url: "https://h1/p", platform: "h1", handle: "p"}
	items := grouped[key]
	if len(items) != 1 {
		t.Fatalf("expected 1 raw target, got %d", len(items))
	}
	if items[0].Category != "url" {
		t.Errorf("base category = %q, want url", items[0].Category)
	}
	if len(items[0].Variants) != 2 {
		t.Fatalf("expected 2 variants, got %#v", items[0].Variants)
	}
	if !flags[key].disabled {
		t.Error("disabled flag was dropped")
	}
}

func TestGroupEntriesForImportKeepsDistinctCategories(t *testing.T) {
	_, grouped, _ := groupEntriesForImport([]storage.Entry{
		{
			ProgramURL: "https://h1/p", Platform: "h1", Handle: "p",
			TargetRaw: "api.example.com", Category: "url", InScope: true,
		},
		{
			ProgramURL: "https://h1/p", Platform: "h1", Handle: "p",
			TargetRaw: "api.example.com", Category: "wildcard", InScope: true,
		},
	})
	key := programKey{url: "https://h1/p", platform: "h1", handle: "p"}
	items := grouped[key]
	if len(items) != 2 {
		t.Fatalf("expected 2 items (one per category), got %d: %#v", len(items), items)
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.URI != "api.example.com" {
			t.Errorf("unexpected URI %q", item.URI)
		}
		seen[item.Category] = true
	}
	if !seen["url"] || !seen["wildcard"] {
		t.Fatalf("categories = %v, want url and wildcard", seen)
	}
}

func TestGroupEntriesForImportCustomKeepsScope(t *testing.T) {
	_, grouped, _ := groupEntriesForImport([]storage.Entry{
		{
			Platform: "custom", ProgramURL: "custom", TargetRaw: "oos.example.com",
			Category: "url", InScope: false, IsBBP: true, Description: "notes",
		},
	})
	key := programKey{url: "custom", platform: "custom", handle: ""}
	items := grouped[key]
	if len(items) != 1 {
		t.Fatalf("expected 1 custom target, got %d", len(items))
	}
	got := items[0]
	if got.InScope || !got.IsBBP || got.Description != "notes" {
		t.Fatalf("custom fields dropped: %#v", got)
	}
}
