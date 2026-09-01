package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run bbscope as a daemon with scheduled polling",
	Long: `Run bbscope as a background daemon that polls platforms on a schedule.

The daemon continuously polls configured platforms at the specified interval
and updates the database with any scope changes. It reuses the same poll
orchestration as 'bbscope poll --db'.

Examples:
  # Poll all configured platforms every hour
  bbscope daemon --interval 1h --db

  # Poll specific platforms every 30 minutes with AI normalization
  bbscope daemon --interval 30m --platforms h1,bc --db --ai

  # Run in foreground with debug logging
  bbscope daemon --interval 15m --db -l debug`,
	RunE: runDaemon,
}

func runDaemon(cmd *cobra.Command, args []string) error {
	interval, _ := cmd.Flags().GetDuration("interval")
	platformsFlag, _ := cmd.Flags().GetString("platforms")
	useDB, _ := cmd.Flags().GetBool("db")
	useAI, _ := cmd.Flags().GetBool("ai")
	pidFile, _ := cmd.Flags().GetString("pid-file")

	if interval < 1*time.Minute {
		return fmt.Errorf("interval must be at least 1 minute")
	}
	if !useDB {
		return fmt.Errorf("daemon requires --db so scope changes can be persisted")
	}

	if pidFile != "" {
		pid := os.Getpid()
		if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", pid)), 0600); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer os.Remove(pidFile)
		utils.Log.Infof("PID file written: %s (PID: %d)", pidFile, pid)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		sig := <-sigChan
		utils.Log.Infof("Received signal %s, shutting down gracefully...", sig)
		cancel()
	}()

	platformFilter := parsePlatformFilter(platformsFlag)
	proxyURL, _ := rootCmd.PersistentFlags().GetString("proxy")

	// pollCmd is never dispatched by cobra here, so its persistent flags are
	// never merged into its local FlagSet (that merge only happens inside
	// cobra's own ParseFlags during Execute()). Merge them explicitly so
	// runPollWithPollers's cmd.Flags() lookups (db, ai, category, ...) resolve
	// instead of silently returning zero values.
	pollCmd.Flags().AddFlagSet(pollCmd.PersistentFlags())
	_ = pollCmd.PersistentFlags().Set("db", strconv.FormatBool(useDB))
	_ = pollCmd.PersistentFlags().Set("ai", strconv.FormatBool(useAI))

	rebuildPollers := func() ([]platforms.PlatformPoller, error) {
		built, err := buildPollersFromConfig(ctx, proxyURL, platformFilter)
		if err != nil {
			return built, err
		}
		if len(built) == 0 {
			return nil, fmt.Errorf("no platforms to poll; configure credentials or adjust --platforms")
		}
		return built, nil
	}

	// Authenticate once and reuse pollers across ticks. If only some pollers
	// authenticate, keep the usable subset for this tick but retry the build on
	// the next one so a transient failure cannot remove a platform forever.
	var pollers []platforms.PlatformPoller
	rebuildPending := true

	runOnce := func() error {
		pollCmd.SetContext(ctx)
		var partialBuildErr error
		if rebuildPending || len(pollers) == 0 {
			rebuilt, buildErr := rebuildPollers()
			rebuildPending = buildErr != nil
			if len(rebuilt) == 0 {
				return buildErr
			}
			pollers = rebuilt
			if buildErr != nil {
				partialBuildErr = buildErr
				utils.Log.Warnf("Some pollers could not be built; using the available platforms and retrying next tick: %v", buildErr)
			}
		}
		err := runPollWithPollers(pollCmd, pollers)
		if err == nil {
			return partialBuildErr
		}
		if !looksLikeAuthError(err) {
			return errors.Join(partialBuildErr, err)
		}
		utils.Log.Warnf("Auth error during poll, re-authenticating: %v", err)
		rebuilt, buildErr := rebuildPollers()
		if buildErr != nil {
			pollers = rebuilt
			rebuildPending = true
			return fmt.Errorf("re-authenticate: %w", errors.Join(err, buildErr))
		}
		pollers = rebuilt
		rebuildPending = false
		return runPollWithPollers(pollCmd, pollers)
	}

	utils.Log.Infof("Starting bbscope daemon (interval: %s, platforms: %s, ai: %v)", interval, platformsFlag, useAI)

	utils.Log.Info("Running initial poll...")
	if err := runOnce(); err != nil {
		utils.Log.Errorf("Initial poll failed: %v", err)
	} else {
		utils.Log.Info("Initial poll completed")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	pollCount := 1
	for {
		select {
		case <-ticker.C:
			if ctx.Err() != nil {
				utils.Log.Info("Daemon stopped")
				return nil
			}
			pollCount++
			utils.Log.Infof("Starting scheduled poll #%d...", pollCount)
			if err := runOnce(); err != nil {
				utils.Log.Errorf("Poll #%d failed: %v", pollCount, err)
			} else {
				utils.Log.Infof("Poll #%d completed", pollCount)
			}
		case <-ctx.Done():
			utils.Log.Info("Daemon stopped")
			return nil
		}
	}
}

func init() {
	rootCmd.AddCommand(daemonCmd)

	daemonCmd.Flags().Duration("interval", 1*time.Hour, "Polling interval (e.g., 30m, 1h, 2h)")
	daemonCmd.Flags().String("platforms", "all", "Comma-separated list of platforms to poll (e.g., h1,bc,it,ywh,immunefi)")
	daemonCmd.Flags().Bool("db", false, "Save results to database (required)")
	daemonCmd.Flags().Bool("ai", false, "Use AI normalization")
	daemonCmd.Flags().String("pid-file", "", "Write process ID to file (optional)")
}

func looksLikeAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, needle := range []string{
		"401",
		"403",
		"unauthorized",
		"invalid auth",
		"auth failed",
		"authentication",
		"invalid auth token",
	} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
