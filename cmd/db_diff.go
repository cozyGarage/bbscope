package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare scope between two points in time",
	Long: `Compare scope between two timestamps to see what was added or removed.

This command queries the scope_changes table to show differences between
two points in time, helping you track scope evolution.

Examples:
  # Compare scope from Jan 1 to Feb 1
  bbscope db diff --from "2024-01-01" --to "2024-02-01"
  
  # Compare specific program
  bbscope db diff --program "hackerone.com/security" --from "2024-01-01"
  
  # Show only additions
  bbscope db diff --from "2024-01-01" --only-added
  
  # Show only removals
  bbscope db diff --from "2024-01-01" --only-removed
  
  # Export as JSON
  bbscope db diff --from "2024-01-01" --format json`,
	RunE: runDiff,
}

func runDiff(cmd *cobra.Command, args []string) error {
	// Parse flags
	fromStr, _ := cmd.Flags().GetString("from")
	toStr, _ := cmd.Flags().GetString("to")
	program, _ := cmd.Flags().GetString("program")
	onlyAdded, _ := cmd.Flags().GetBool("only-added")
	onlyRemoved, _ := cmd.Flags().GetBool("only-removed")
	rawFormat, _ := cmd.Flags().GetString("format")
	format, err := normalizeDataFormat(rawFormat, "text", "json", "csv")
	if err != nil {
		return err
	}

	// Parse timestamps
	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return fmt.Errorf("invalid --from date (use YYYY-MM-DD): %w", err)
	}

	to, err := parseDiffTo(toStr, time.Now())
	if err != nil {
		return err
	}

	if to.Before(from) {
		return fmt.Errorf("--to date must be after --from date")
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

	// Get changes between dates
	ctx := context.Background()
	changes, err := db.GetChangesBetween(ctx, from, to, program)
	if err != nil {
		return fmt.Errorf("failed to get changes: %w", err)
	}

	// Filter by type if requested
	filtered := make([]storage.Change, 0, len(changes))
	for _, change := range changes {
		if onlyAdded && change.ChangeType != "added" {
			continue
		}
		if onlyRemoved && change.ChangeType != "removed" {
			continue
		}
		filtered = append(filtered, change)
	}

	if len(filtered) == 0 {
		utils.Log.Info("No changes found in the specified time range")
		return nil
	}

	// Output based on format
	switch format {
	case "json":
		return outputDiffJSON(filtered)
	case "csv":
		return outputDiffCSV(filtered)
	default:
		outputDiffText(filtered, from, to)
		return nil
	}
}

// parseDiffTo resolves the --to flag. A bare YYYY-MM-DD parses to midnight, so
// treating it as the upper bound of an inclusive range would discard everything
// that happened during the day the user actually named; extend it to the last
// instant of that day instead. An empty flag means "up to now".
func parseDiffTo(toStr string, now time.Time) (time.Time, error) {
	if toStr == "" {
		return now, nil
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --to date (use YYYY-MM-DD): %w", err)
	}
	return to.AddDate(0, 0, 1).Add(-time.Nanosecond), nil
}

func outputDiffText(changes []storage.Change, from, to time.Time) {
	fmt.Printf("Scope changes from %s to %s:\n\n", from.Format("2006-01-02"), to.Format("2006-01-02"))

	added := 0
	removed := 0

	for _, change := range changes {
		var symbol string
		if change.ChangeType == "added" {
			symbol = "+"
			added++
		} else if change.ChangeType == "removed" {
			symbol = "-"
			removed++
		} else {
			symbol = "~"
		}

		fmt.Printf("%s [%s] %s (%s) - %s @ %s\n",
			symbol,
			change.OccurredAt.Format("2006-01-02 15:04"),
			change.TargetNormalized,
			change.Category,
			change.Platform,
			change.ProgramURL,
		)
	}

	fmt.Printf("\nSummary: %d added, %d removed, %d total\n", added, removed, len(changes))
}

// diffEntry is the JSON shape emitted by `db diff --format json`.
type diffEntry struct {
	Type     string `json:"type"`
	Platform string `json:"platform"`
	Target   string `json:"target"`
	Category string `json:"category"`
	Program  string `json:"program"`
	Time     string `json:"time"`
}

func outputDiffJSON(changes []storage.Change) error {
	// Built with encoding/json rather than Printf: a quote or backslash in a
	// target or program URL used to produce invalid JSON.
	out := make([]diffEntry, 0, len(changes))
	for _, change := range changes {
		out = append(out, diffEntry{
			Type:     change.ChangeType,
			Platform: change.Platform,
			Target:   change.TargetNormalized,
			Category: change.Category,
			Program:  change.ProgramURL,
			Time:     change.OccurredAt.Format(time.RFC3339),
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		return fmt.Errorf("encoding diff as JSON: %w", err)
	}
	return nil
}

func outputDiffCSV(changes []storage.Change) error {
	// encoding/csv rather than the previous hand-rolled escaper, which handled
	// commas and quotes but not embedded newlines.
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	records := make([][]string, 0, len(changes)+1)
	records = append(records, []string{"type", "platform", "target", "category", "program", "time"})
	for _, change := range changes {
		records = append(records, []string{
			change.ChangeType,
			change.Platform,
			change.TargetNormalized,
			change.Category,
			change.ProgramURL,
			change.OccurredAt.Format(time.RFC3339),
		})
	}

	if err := w.WriteAll(records); err != nil {
		return fmt.Errorf("writing diff as CSV: %w", err)
	}
	return w.Error()
}

func init() {
	dbCmd.AddCommand(diffCmd)

	diffCmd.Flags().String("from", "", "Start date (YYYY-MM-DD) (required)")
	diffCmd.Flags().String("to", "", "End date (YYYY-MM-DD) (default: now)")
	diffCmd.Flags().String("program", "", "Filter by program URL (optional)")
	diffCmd.Flags().Bool("only-added", false, "Show only additions")
	diffCmd.Flags().Bool("only-removed", false, "Show only removals")
	diffCmd.Flags().String("format", "text", "Output format: text, json, csv")

	if err := diffCmd.MarkFlagRequired("from"); err != nil {
		panic(err)
	}
}
