package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// Messages for async operations
type statsMsg Stats
type changesMsg []storage.Change
type pollingStatusMsg string
type pollingCompleteMsg struct{}
type errMsg error

// loadStatsCmd loads statistics from database
func loadStatsCmd(db *storage.DB) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Get program count (active only)
		programs, err := db.ListPrograms(ctx)
		if err != nil {
			return errMsg(fmt.Errorf("failed to load programs: %w", err))
		}
		activePrograms := 0
		for _, p := range programs {
			if !p.Disabled && !p.IsIgnored {
				activePrograms++
			}
		}

		// Get target count (active programs only via ListOptions defaults)
		entries, err := db.ListEntries(ctx, storage.ListOptions{})
		if err != nil {
			return errMsg(fmt.Errorf("failed to load entries: %w", err))
		}

		// Get true 24h change count
		now := time.Now()
		changes, err := db.GetChangesBetween(ctx, now.Add(-24*time.Hour), now, "")
		if err != nil {
			return errMsg(fmt.Errorf("failed to load changes: %w", err))
		}

		return statsMsg{
			ProgramCount: activePrograms,
			TargetCount:  len(entries),
			ChangesCount: len(changes),
			LastUpdated:  now,
		}
	}
}

// loadRecentChangesCmd loads recent changes from database
func loadRecentChangesCmd(db *storage.DB) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		changes, err := db.ListRecentChanges(ctx, 10)
		if err != nil {
			return errMsg(fmt.Errorf("failed to load changes: %w", err))
		}

		return changesMsg(changes)
	}
}

// startPollingCmd starts the polling process
func startPollingCmd(db *storage.DB, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Note: This is a placeholder. In a real implementation,
		// we would trigger the actual platform polling here.
		// For now, we simulate it with a delay.
		time.Sleep(2 * time.Second)
		return pollingCompleteMsg{}
	}
}

// renderDashboard renders the dashboard view
func (m Model) renderDashboard() string {
	if !m.statsLoaded {
		return m.styles.Subtitle.Render("Loading dashboard...")
	}

	title := m.styles.Title.Render("🎯 bbscope Dashboard")

	// Stats section
	statsBox := m.renderStats()

	// Recent changes section
	changesBox := m.renderRecentChanges()

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		statsBox,
		"",
		changesBox,
	)

	return m.styles.Border.Render(content)
}

// renderStats renders the statistics section
func (m Model) renderStats() string {
	programStat := fmt.Sprintf("%s: %s",
		m.styles.StatLabel.Render("Programs"),
		m.styles.StatValue.Render(fmt.Sprintf("%d", m.stats.ProgramCount)),
	)

	targetStat := fmt.Sprintf("%s: %s",
		m.styles.StatLabel.Render("Targets"),
		m.styles.StatValue.Render(fmt.Sprintf("%d", m.stats.TargetCount)),
	)

	changesStat := fmt.Sprintf("%s: %s",
		m.styles.StatLabel.Render("Changes (24h)"),
		m.styles.StatValue.Render(fmt.Sprintf("%d", m.stats.ChangesCount)),
	)

	stats := lipgloss.JoinHorizontal(lipgloss.Top,
		programStat, "  │  ",
		targetStat, "  │  ",
		changesStat,
	)

	return m.styles.Subtitle.Render("📊 Overview\n") + stats
}

// renderRecentChanges renders the recent changes section
func (m Model) renderRecentChanges() string {
	title := m.styles.Subtitle.Render("🔔 Recent Changes")

	if len(m.recentChanges) == 0 {
		return title + "\n" + m.styles.Change.Render("No recent changes")
	}

	changes := make([]string, 0, len(m.recentChanges))
	for i, change := range m.recentChanges {
		if i >= 5 { // Limit to 5 changes
			break
		}

		var prefix string
		var style lipgloss.Style

		switch change.ChangeType {
		case "added":
			prefix = "+"
			style = m.styles.ChangeAdded
		case "removed":
			prefix = "-"
			style = m.styles.ChangeRemoved
		default:
			prefix = "~"
			style = m.styles.Change
		}

		// Format: + target    platform/program    time
		line := fmt.Sprintf("%s %-30s %-25s %s",
			prefix,
			truncate(change.TargetNormalized, 30),
			truncate(change.Platform+"/"+change.Handle, 25),
			formatTimeAgo(change.OccurredAt),
		)

		changes = append(changes, style.Render(line))
	}

	return title + "\n" + lipgloss.JoinVertical(lipgloss.Left, changes...)
}

// renderPolling renders the live polling view
func (m Model) renderPolling() string {
	title := m.styles.Title.Render("⚡ Live Polling")

	var content string
	if m.pollingActive {
		content = m.styles.Subtitle.Render("Polling platforms...\n\n") +
			m.styles.Change.Render(m.pollingStatus)
	} else {
		content = m.styles.Subtitle.Render("Press 'p' to start polling")
	}

	full := lipgloss.JoinVertical(lipgloss.Left, title, "", content)
	return m.styles.Border.Render(full)
}

// renderSearch renders the search interface
func (m Model) renderSearch() string {
	title := m.styles.Title.Render("🔍 Search")

	content := m.styles.Subtitle.Render("Search feature coming soon...")

	full := lipgloss.JoinVertical(lipgloss.Left, title, "", content)
	return m.styles.Border.Render(full)
}

// renderHelp renders the help view
func (m Model) renderHelp() string {
	title := m.styles.Title.Render("❓ Help")

	helpText := `
Keyboard Shortcuts:

  d - Dashboard view
  p - Live polling view
  s - Search targets
  ? - This help screen
  q - Quit application

Navigation:

  Arrow keys / hjkl - Navigate lists
  Enter - Select item
  Esc - Go back
`

	full := lipgloss.JoinVertical(lipgloss.Left, title, m.styles.Change.Render(helpText))
	return m.styles.Border.Render(full)
}

// Helper functions

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return "just now"
	} else if duration < time.Hour {
		mins := int(duration.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		return fmt.Sprintf("%dh ago", hours)
	} else {
		days := int(duration.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}
