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
		changes, err := db.ListRecentChanges(context.Background(), 10)
		if err != nil {
			return errMsg(fmt.Errorf("failed to load changes: %w", err))
		}

		return changesMsg(changes)
	}
}

// renderDashboard renders the dashboard view
func (m Model) renderDashboard() string {
	if !m.statsLoaded {
		return m.styles.Subtitle.Render("Loading dashboard...")
	}

	title := m.styles.Title.Render("🎯 bbscope Dashboard")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		m.renderStats(),
		"",
		m.renderRecentChanges(),
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
		changes = append(changes, m.renderChangeLine(change))
	}

	return title + "\n" + lipgloss.JoinVertical(lipgloss.Left, changes...)
}

// renderChangeLine formats one change for the dashboard or the polling view.
func (m Model) renderChangeLine(change storage.Change) string {
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

	return style.Render(line)
}

// renderHelp renders the help view
func (m Model) renderHelp() string {
	title := m.styles.Title.Render("❓ Help")

	helpText := `
Views:

  d  Dashboard: totals and the most recent changes
  b  Browse:    platforms -> programs -> targets
  s  Search:    find targets, descriptions or programs
  p  Poll:      fetch every configured platform and store the results
  ?  This help screen
  q  Quit

Browsing:

  ↑ / ↓ / j / k  Move through the list
  enter          Open the selected platform or program
  esc            Go back up a level
  o              Toggle the in-scope-only filter
  /              Filter the current list by text

Searching:

  Type a query and press enter. Matches cover target names,
  descriptions and program URLs, including targets that have
  since been removed. Press esc to leave the search box.
`

	return m.styles.Border.Render(lipgloss.JoinVertical(lipgloss.Left, title, m.styles.Change.Render(helpText)))
}

// Helper functions

// truncate shortens s to at most maxLen runes, using an ellipsis when there is
// room for one. It operates on runes so a multi-byte target is never cut
// mid-character, and it tolerates a maxLen too small for the ellipsis, which
// used to panic with a negative slice bound.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	const ellipsis = "..."
	if maxLen <= len(ellipsis) {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-len(ellipsis)]) + ellipsis
}

func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(duration.Hours()/24))
	}
}
