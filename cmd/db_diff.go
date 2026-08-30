package cmd

import (
	"context"
	"fmt"
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
	format, _ := cmd.Flags().GetString("format")

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
		outputDiffJSON(filtered)
	case "csv":
		outputDiffCSV(filtered)
	default:
		outputDiffText(filtered, from, to)
	}
	return nil
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

func outputDiffJSON(changes []storage.Change) {
	// Simple JSON output
	fmt.Println("[")
	for i, change := range changes {
		fmt.Printf(`  {"type": "%s", "platform": "%s", "target": "%s", "category": "%s", "program": "%s", "time": "%s"}`,
			change.ChangeType,
			change.Platform,
			change.TargetNormalized,
			change.Category,
			change.ProgramURL,
			change.OccurredAt.Format(time.RFC3339),
		)
		if i < len(changes)-1 {
			fmt.Println(",")
		} else {
			fmt.Println()
		}
	}
	fmt.Println("]")
}

func outputDiffCSV(changes []storage.Change) {
	fmt.Println("type,platform,target,category,program,time")
	for _, change := range changes {
		fmt.Printf("%s,%s,%s,%s,%s,%s\n",
			change.ChangeType,
			change.Platform,
			csvEscape(change.TargetNormalized),
			change.Category,
			csvEscape(change.ProgramURL),
			change.OccurredAt.Format(time.RFC3339),
		)
	}
}

func csvEscape(s string) string {
	// Simple CSV escaping
	if containsComma(s) || containsQuote(s) {
		return fmt.Sprintf(`"%s"`, replaceQuotes(s))
	}
	return s
}

func containsComma(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			return true
		}
	}
	return false
}

func containsQuote(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			return true
		}
	}
	return false
}

func replaceQuotes(s string) string {
	result := ""
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			result += `""`
		} else {
			result += string(s[i])
		}
	}
	return result
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
