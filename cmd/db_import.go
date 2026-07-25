package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"

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

	count := 0
	ctx := context.Background()
	for _, e := range entries {
		// If it's a custom target (no platform ID), add as custom
		if e.Platform == "" || e.Platform == "custom" {
			_, err := db.AddCustomTarget(ctx, e.TargetRaw, e.Category, e.ProgramURL)
			if err != nil {
				fmt.Printf("Error adding custom target %s: %v\n", e.TargetRaw, err)
			} else {
				count++
			}
			continue
		}

		// For real platform data, import logic would be more complex
		// Currently storage doesn't expose a raw insert for full entities
		// Since we mostly care about importing custom/manually added targets or restoring backups
		// We'll stick to a simple strategy for now: log warning for non-custom
		// TODO: Implement full UpsertEntry for arbitrary data restoration
		fmt.Printf("Skipping non-custom entry: %s (%s)\n", e.TargetRaw, e.Platform)
	}

	fmt.Printf("Successfully imported %d targets.\n", count)
	return nil
}

func parseimportJSON(r io.Reader) ([]storage.Entry, error) {
	var data ExportData // reuse struct from export
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		// Try array fallback
		var entries []storage.Entry
		if err2 := json.NewDecoder(r).Decode(&entries); err2 == nil {
			return entries, nil
		}
		return nil, err
	}
	return data.Entries, nil
}

func parseimportCSV(r io.Reader) ([]storage.Entry, error) {
	reader := csv.NewReader(r)

	// Skip header
	if _, err := reader.Read(); err != nil {
		return nil, err
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

		if len(record) < 6 {
			continue
		}

		e := storage.Entry{
			ProgramURL:       record[0],
			Platform:         record[1],
			Handle:           record[2],
			TargetRaw:        record[3],
			TargetNormalized: record[4],
			Category:         record[5],
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func init() {
	dbCmd.AddCommand(importCmd)
	importCmd.Flags().String("file", "", "Input file (default stdin)")
	importCmd.Flags().String("format", "json", "Input format (json, csv)")
}
