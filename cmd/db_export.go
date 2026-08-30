package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export database content to file",
	Long: `Export database content to JSON or CSV format.

Exports all programs and targets from the database. Useful for backups,
external analysis, or migrating data to another instance.

Examples:
  # Export everything to JSON (stdout)
  bbscope db export > backup.json

  # Export to CSV
  bbscope db export --format csv > targets.csv

  # Export specified platform only
  bbscope db export --platform h1 --format json > h1.json`,
	RunE: runExport,
}

func normalizeExportFormat(format string) (string, error) {
	f := strings.ToLower(strings.TrimSpace(format))
	switch f {
	case "json", "csv":
		return f, nil
	default:
		return "", fmt.Errorf("unknown format: %s (use json or csv)", format)
	}
}

func runExport(cmd *cobra.Command, args []string) error {
	rawFormat, _ := cmd.Flags().GetString("format")
	format, err := normalizeExportFormat(rawFormat)
	if err != nil {
		return err
	}
	platform, _ := cmd.Flags().GetString("platform")

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

	// Fetch all entries
	opts := storage.ListOptions{
		Platform:        platform,
		IncludeOOS:      true, // Export everything by default
		IncludeIgnored:  true, // Backups must include ignored programs or a restore drops them
		IncludeDisabled: true, // Include soft-disabled programs in backups
	}

	ctx := context.Background()
	entries, err := db.ListEntries(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to list entries: %w", err)
	}

	switch format {
	case "json":
		return exportJSON(entries)
	case "csv":
		return exportCSV(entries)
	default:
		return fmt.Errorf("unknown format: %s (use json or csv)", format)
	}
}

type ExportData struct {
	Version    string          `json:"version"`
	ExportedAt time.Time       `json:"exported_at"`
	Count      int             `json:"count"`
	Entries    []storage.Entry `json:"entries"`
}

func exportJSON(entries []storage.Entry) error {
	data := ExportData{
		Version:    "1.0",
		ExportedAt: time.Now().UTC(),
		Count:      len(entries),
		Entries:    entries,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func exportCSV(entries []storage.Entry) error {
	writer := csv.NewWriter(os.Stdout)

	// Header
	header := []string{
		"program_url",
		"platform",
		"handle",
		"target_raw",
		"target_normalized",
		"category",
		"in_scope",
		"is_bbp",
		"description",
		"source",
		"base_target_raw",
		"base_category",
		"disabled",
		"is_ignored",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Rows
	for _, e := range entries {
		record := []string{
			e.ProgramURL,
			e.Platform,
			e.Handle,
			e.TargetRaw,
			e.TargetNormalized,
			e.Category,
			fmt.Sprintf("%t", e.InScope),
			fmt.Sprintf("%t", e.IsBBP),
			e.Description,
			e.Source,
			e.BaseTargetRaw,
			e.BaseCategory,
			fmt.Sprintf("%t", e.Disabled),
			fmt.Sprintf("%t", e.IsIgnored),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	writer.Flush()
	return writer.Error()
}

func init() {
	dbCmd.AddCommand(exportCmd)
	exportCmd.Flags().String("format", "json", "Output format (json, csv)")
	exportCmd.Flags().String("platform", "", "Filter by platform (optional)")
}
