package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run bbscope as a daemon with scheduled polling",
	Long: `Run bbscope as a background daemon that polls platforms on a schedule.

The daemon will continuously poll configured platforms at the specified interval
and update the database with any scope changes.

Examples:
  # Poll all configured platforms every hour
  bbscope daemon --interval 1h --db

  # Poll specific platforms every 30 minutes with AI normalization
  bbscope daemon --interval 30m --platforms h1,bc --db --ai

  # Run in foreground with debug logging
  bbscope daemon --interval 15m --db -l debug`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get configuration
		interval, _ := cmd.Flags().GetDuration("interval")
		platformsFlag, _ := cmd.Flags().GetString("platforms")
		useDB, _ := cmd.Flags().GetBool("db")
		useAI, _ := cmd.Flags().GetBool("ai")
		pidFile, _ := cmd.Flags().GetString("pid-file")

		if interval < 1*time.Minute {
			return fmt.Errorf("interval must be at least 1 minute")
		}

		// Write PID file if requested
		if pidFile != "" {
			pid := os.Getpid()
			if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", pid)), 0600); err != nil {
				return fmt.Errorf("failed to write PID file: %w", err)
			}
			defer os.Remove(pidFile)
			utils.Log.Infof("PID file written: %s (PID: %d)", pidFile, pid)
		}

		// Set up signal handling for graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			sig := <-sigChan
			utils.Log.Infof("Received signal %s, shutting down gracefully...", sig)
			cancel()
		}()

		// Open database if needed
		var db *storage.DB
		if useDB {
			dbURL := viper.GetString("db_url")
			if dbURL == "" {
				return fmt.Errorf("database URL not configured. Set 'db_url' in ~/.bbscope.yaml")
			}

			var err error
			db, err = storage.Open(dbURL)
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()
		}

		// Parse platforms filter
		var platforms []string
		if platformsFlag != "" && platformsFlag != "all" {
			platforms = parsePlatforms(platformsFlag)
		}

		utils.Log.Infof("Starting daemon with interval: %s", interval)
		if len(platforms) > 0 {
			utils.Log.Infof("Polling platforms: %v", platforms)
		} else {
			utils.Log.Info("Polling all configured platforms")
		}

		// Run initial poll immediately
		utils.Log.Info("Running initial poll...")
		if err := runScheduledPoll(ctx, cmd, db, platforms, useDB, useAI); err != nil {
			utils.Log.Errorf("Initial poll failed: %v", err)
		}

		// Set up ticker for scheduled polling
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		pollCount := 1
		for {
			select {
			case <-ticker.C:
				pollCount++
				utils.Log.Infof("Starting scheduled poll #%d...", pollCount)
				if err := runScheduledPoll(ctx, cmd, db, platforms, useDB, useAI); err != nil {
					utils.Log.Errorf("Poll #%d failed: %v", pollCount, err)
				}
				utils.Log.Infof("Poll #%d completed", pollCount)

			case <-ctx.Done():
				utils.Log.Info("Daemon stopped")
				return nil
			}
		}
	},
}

// runScheduledPoll executes a single polling cycle
func runScheduledPoll(ctx context.Context, cmd *cobra.Command, db *storage.DB, platforms []string, useDB, useAI bool) error {
	// This will be refactored to use shared polling logic from poll.go
	// For now, it's a placeholder that calls the poll command
	
	// TODO: Extract common polling logic from poll.go into a shared function
	// that can be used by both poll and daemon commands
	
	utils.Log.Info("Polling logic to be implemented")
	return nil
}

// parsePlatforms converts a comma-separated string into a slice of platform names
func parsePlatforms(input string) []string {
	if input == "" {
		return nil
	}
	
	platforms := splitAndTrim(input, ",")
	return platforms
}

// splitAndTrim splits a string by delimiter and trims whitespace
func splitAndTrim(s, delimiter string) []string {
	var result []string
	parts := splitString(s, delimiter)
	for _, part := range parts {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitString(s, delimiter string) []string {
	// Simple string split implementation
	if delimiter == "" {
		return []string{s}
	}
	
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == delimiter[0] {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}
	
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}
	
	return s[start:end]
}

func init() {
	rootCmd.AddCommand(daemonCmd)

	daemonCmd.Flags().Duration("interval", 1*time.Hour, "Polling interval (e.g., 30m, 1h, 2h)")
	daemonCmd.Flags().String("platforms", "all", "Comma-separated list of platforms to poll (e.g., h1,bc,it)")
	daemonCmd.Flags().Bool("db", false, "Save results to database")
	daemonCmd.Flags().Bool("ai", false, "Use AI normalization")
	daemonCmd.Flags().String("pid-file", "", "Write process ID to file (optional)")
}
