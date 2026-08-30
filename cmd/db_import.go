package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import data from file",
	Long: `Import data from a JSON or CSV file.

Reads data from a file (or stdin) and upserts it into the database.
Supports backups created by 'bbscope db export'.

Examples:
  # Import from JSON backup
  bbscope db import --file backup.json

  # Import from CSV
  bbscope db import --file targets.csv --format csv

  # Import from stdin
  cat backup.json | bbscope db import --format json`,
	RunE: runImport,
}

func runImport(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	format, _ := cmd.Flags().GetString("format")

	// Open input
	var input io.Reader
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer f.Close()
		input = f
	} else {
		input = os.Stdin
	}

	// Open database
	dbURL, err := GetDBConnectionString()
	if err != nil {
		return err
	}

	db, err := storage.Open(dbURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Parse and import
	var entries []storage.Entry
	if format == "json" {
		entries, err = parseimportJSON(input)
	} else if format == "csv" {
		entries, err = parseimportCSV(input)
	} else {
		return fmt.Errorf("unknown format: %s", format)
	}

	if err != nil {
		return fmt.Errorf("failed to parse import data: %w", err)
	}

	fmt.Printf("Importing %d entries...\n", len(entries))

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	imported, failed := importEntries(ctx, db, entries)

	fmt.Printf("Successfully imported %d targets.\n", imported)
	if failed > 0 {
		cmd.SilenceUsage = true
		return fmt.Errorf("%d of %d entries could not be imported", failed, len(entries))
	}
	return nil
}

// importEntries restores entries into the database, returning how many were
// imported and how many failed.
//
// Custom targets go through AddCustomTarget. Platform entries are grouped by
// program and replayed through the same BuildEntries/UpsertProgramEntries path
// a poll uses, so a backup restores as real platform data rather than being
// skipped. Change logging is suppressed: restoring a backup is not a scope
// change, and recording one "added" event per target would swamp the log.
func importEntries(ctx context.Context, db *storage.DB, entries []storage.Entry) (imported, failed int) {
	type programKey struct {
		url      string
		platform string
		handle   string
	}
	// Preserve the order programs first appear so output is deterministic.
	var order []programKey
	grouped := map[programKey][]storage.TargetItem{}

	for _, e := range entries {
		if e.Platform == "" || e.Platform == "custom" {
			if _, err := db.AddCustomTarget(ctx, e.TargetRaw, e.Category, e.ProgramURL); err != nil {
				fmt.Fprintf(os.Stderr, "Error adding custom target %s: %v\n", e.TargetRaw, err)
				failed++
			} else {
				imported++
			}
			continue
		}

		key := programKey{url: e.ProgramURL, platform: e.Platform, handle: e.Handle}
		if _, seen := grouped[key]; !seen {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], storage.TargetItem{
			URI:         e.TargetRaw,
			Category:    e.Category,
			Description: e.Description,
			InScope:     e.InScope,
			IsBBP:       e.IsBBP,
		})
	}

	for _, key := range order {
		items := grouped[key]
		built, err := storage.BuildEntries(key.url, key.platform, key.handle, items)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error building entries for %s: %v\n", key.url, err)
			failed += len(items)
			continue
		}
		if _, err := db.UpsertProgramEntriesWithOptions(
			ctx, key.url, key.platform, key.handle, built,
			storage.UpsertOptions{SkipChangeLog: true},
		); err != nil {
			fmt.Fprintf(os.Stderr, "Error importing program %s: %v\n", key.url, err)
			failed += len(items)
			continue
		}
		imported += len(items)
	}

	return imported, failed
}

// importEntry decodes a single exported entry.
//
// storage.Entry now carries snake_case json tags, but earlier versions of
// `db export` had no tags at all and emitted Go field names in PascalCase.
// Backups written by those versions must still restore, so a PascalCase
// fallback runs when the current shape yields nothing.
type importEntry struct {
	storage.Entry
}

func (e *importEntry) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &e.Entry); err != nil {
		return err
	}
	if e.TargetRaw != "" || e.TargetNormalized != "" {
		return nil
	}

	var legacy struct {
		ProgramURL       string
		Platform         string
		Handle           string
		TargetNormalized string
		TargetRaw        string
		Category         string
		Description      string
		InScope          bool
		IsBBP            bool
		Source           string
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	if legacy.TargetRaw == "" && legacy.TargetNormalized == "" {
		return nil
	}

	e.Entry = storage.Entry{
		ProgramURL:       legacy.ProgramURL,
		Platform:         legacy.Platform,
		Handle:           legacy.Handle,
		TargetNormalized: legacy.TargetNormalized,
		TargetRaw:        legacy.TargetRaw,
		Category:         legacy.Category,
		Description:      legacy.Description,
		InScope:          legacy.InScope,
		IsBBP:            legacy.IsBBP,
		Source:           legacy.Source,
	}
	return nil
}

func unwrapImportEntries(in []importEntry) []storage.Entry {
	out := make([]storage.Entry, 0, len(in))
	for _, e := range in {
		out = append(out, e.Entry)
	}
	return out
}

func parseimportJSON(r io.Reader) ([]storage.Entry, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// The export wrapper object.
	var data struct {
		Entries []importEntry `json:"entries"`
	}
	if err := json.Unmarshal(raw, &data); err == nil && data.Entries != nil {
		return unwrapImportEntries(data.Entries), nil
	}

	// Fallback: bare JSON array of entries
	var entries []importEntry
	if err2 := json.Unmarshal(raw, &entries); err2 == nil {
		return unwrapImportEntries(entries), nil
	}
	return nil, fmt.Errorf("invalid JSON import: expected export object or entry array")
}

// parseimportCSV reads the CSV shape produced by `db export --format csv`.
//
// Columns are located by header name rather than by position, so the reader
// tolerates column reordering and files that omit optional columns. Reading a
// fixed six columns previously discarded in_scope, is_bbp, description and
// source, silently importing every target as though it were in scope.
func parseimportCSV(r io.Reader) ([]storage.Entry, error) {
	reader := csv.NewReader(r)
	// Records are not all the same width once optional columns are omitted.
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, errors.New("empty CSV input: expected a header row")
		}
		return nil, err
	}

	index := make(map[string]int, len(header))
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(name))] = i
	}
	for _, required := range []string{"program_url", "platform", "target_raw", "category"} {
		if _, ok := index[required]; !ok {
			return nil, fmt.Errorf("CSV is missing the %q column", required)
		}
	}

	field := func(record []string, name string) string {
		i, ok := index[name]
		if !ok || i >= len(record) {
			return ""
		}
		return record[i]
	}

	var entries []storage.Entry
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// A bool column that is absent should not silently mean false for
		// in_scope, which would import every target as out of scope.
		inScope := true
		if raw := field(record, "in_scope"); raw != "" {
			inScope = parseCSVBool(raw)
		}

		entries = append(entries, storage.Entry{
			ProgramURL:       field(record, "program_url"),
			Platform:         field(record, "platform"),
			Handle:           field(record, "handle"),
			TargetRaw:        field(record, "target_raw"),
			TargetNormalized: field(record, "target_normalized"),
			Category:         field(record, "category"),
			Description:      field(record, "description"),
			InScope:          inScope,
			IsBBP:            parseCSVBool(field(record, "is_bbp")),
			Source:           field(record, "source"),
		})
	}
	return entries, nil
}

func parseCSVBool(s string) bool {
	v, err := strconv.ParseBool(strings.TrimSpace(s))
	return err == nil && v
}

func init() {
	dbCmd.AddCommand(importCmd)
	importCmd.Flags().String("file", "", "Input file (default stdin)")
	importCmd.Flags().String("format", "json", "Input format (json, csv)")
}
