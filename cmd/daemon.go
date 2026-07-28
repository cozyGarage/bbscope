package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run bbscope as a daemon with scheduled polling",
	Long: `Run bbscope as a background daemon that polls platforms on a schedule.

The daemon will continuously poll configured platforms at the specified interval
and update the database with any scope changes.

NOTE: Daemon polling is not implemented yet. Use cron or systemd with
'bbscope poll --db' for scheduled polling until this command is completed.

Examples:
  # Poll all configured platforms every hour
  bbscope daemon --interval 1h --db

  # Poll specific platforms every 30 minutes with AI normalization
  bbscope daemon --interval 30m --platforms h1,bc --db --ai

  # Run in foreground with debug logging
  bbscope daemon --interval 15m --db -l debug`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("daemon polling is not implemented yet; use 'bbscope poll --db' via cron or systemd for scheduled polling")
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)

	daemonCmd.Flags().Duration("interval", 1*time.Hour, "Polling interval (e.g., 30m, 1h, 2h)")
	daemonCmd.Flags().String("platforms", "all", "Comma-separated list of platforms to poll (e.g., h1,bc,it)")
	daemonCmd.Flags().Bool("db", false, "Save results to database")
	daemonCmd.Flags().Bool("ai", false, "Use AI normalization")
	daemonCmd.Flags().String("pid-file", "", "Write process ID to file (optional)")
}
