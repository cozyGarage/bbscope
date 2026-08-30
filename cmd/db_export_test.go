package cmd

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

func sampleExportEntries() []storage.Entry {
	return []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/acme",
			Platform:         "h1",
			Handle:           "acme",
			TargetRaw:        "*.acme.com",
			TargetNormalized: "*.acme.com",
			Category:         "wildcard",
			InScope:          true,
			IsBBP:            true,
			Description:      "api, v2",
			Source:           "raw",
		},
	}
}

func TestExportJSON(t *testing.T) {
	var fnErr error
	out := captureStdout(t, func() { fnErr = exportJSON(sampleExportEntries()) })
	if fnErr != nil {
		t.Fatalf("exportJSON: %v", fnErr)
	}
	var data ExportData
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if data.Count != 1 || len(data.Entries) != 1 {
		t.Fatalf("count = %d entries = %d, want 1", data.Count, len(data.Entries))
	}
	if data.Entries[0].TargetRaw != "*.acme.com" {
		t.Fatalf("target = %q", data.Entries[0].TargetRaw)
	}
}

func TestExportCSVQuotesDescription(t *testing.T) {
	var fnErr error
	out := captureStdout(t, func() { fnErr = exportCSV(sampleExportEntries()) })
	if fnErr != nil {
		t.Fatalf("exportCSV: %v", fnErr)
	}
	r := csv.NewReader(strings.NewReader(out))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv: %v\n%s", err, out)
	}
	if len(records) != 2 {
		t.Fatalf("rows = %d, want header + 1", len(records))
	}
	if records[0][0] != "program_url" {
		t.Fatalf("header[0] = %q", records[0][0])
	}
	if records[1][8] != "api, v2" {
		t.Fatalf("description = %q, want quoted comma preserved", records[1][8])
	}
	if records[1][9] != "raw" {
		t.Fatalf("source = %q", records[1][9])
	}
}

func TestNormalizeExportFormat(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"json", "json", true},
		{"JSON", "json", true},
		{" csv ", "csv", true},
		{"xml", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		got, err := normalizeExportFormat(tc.in)
		if tc.ok {
			if err != nil || got != tc.want {
				t.Errorf("normalizeExportFormat(%q) = %q, %v, want %q", tc.in, got, err, tc.want)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "unknown format") {
			t.Errorf("normalizeExportFormat(%q) = %q, %v, want unknown format", tc.in, got, err)
		}
	}
}

func TestExportUnknownFormat(t *testing.T) {
	exportCmd.SetArgs([]string{})
	if err := exportCmd.Flags().Set("format", "xml"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = exportCmd.Flags().Set("format", "json") })
	err := runExport(exportCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("error = %v, want unknown format", err)
	}
}

func TestExportCSVEmptyHasHeader(t *testing.T) {
	var fnErr error
	out := captureStdout(t, func() { fnErr = exportCSV(nil) })
	if fnErr != nil {
		t.Fatalf("exportCSV(nil): %v", fnErr)
	}
	r := csv.NewReader(strings.NewReader(out))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv: %v", err)
	}
	if len(records) != 1 || records[0][0] != "program_url" {
		t.Fatalf("want header-only CSV, got %#v", records)
	}
}
