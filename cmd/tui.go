package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
	"github.com/cozyGarage/bbscope/v2/pkg/tui"

)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive terminal UI",
	Long: `Launch an interactive terminal user interface for browsing
and managing bug bounty scopes.

Features:
  - Dashboard with real-time statistics
  - Live polling progress view
  - Interactive search interface
  - Minimal, clean design`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get database URL from config
		dbURL := viper.GetString("db_url")
		if dbURL == "" {
			return fmt.Errorf("database URL not configured. Set 'db_url' in ~/.bbscope.yaml")
		}

		// Open database connection
		db, err := storage.Open(dbURL)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		// Create and run TUI
		model := tui.NewModel(db)
		program := tea.NewProgram(model, tea.WithAltScreen())

		if _, err := program.Run(); err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
