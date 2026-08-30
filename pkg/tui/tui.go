package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// ViewMode represents the current active view in the TUI
type ViewMode int

const (
	ViewDashboard ViewMode = iota
	ViewPolling
	ViewSearch
	ViewHelp
)

// Model represents the main application state
type Model struct {
	db       *storage.DB
	ctx      context.Context
	cancel   context.CancelFunc
	width    int
	height   int
	view     ViewMode
	quitting bool
	err      error

	// Dashboard state
	stats         Stats
	recentChanges []storage.Change
	statsLoaded   bool
	changesLoaded bool

	// Polling state
	pollingActive bool
	pollingStatus string

	// Styling
	styles Styles
}

// Stats represents dashboard statistics
type Stats struct {
	ProgramCount int
	TargetCount  int
	ChangesCount int
	LastUpdated  time.Time
}

// Styles contains all lipgloss styles for the TUI
type Styles struct {
	Border        lipgloss.Style
	Title         lipgloss.Style
	Subtitle      lipgloss.Style
	Stat          lipgloss.Style
	StatLabel     lipgloss.Style
	StatValue     lipgloss.Style
	Change        lipgloss.Style
	ChangeAdded   lipgloss.Style
	ChangeRemoved lipgloss.Style
	Help          lipgloss.Style
	Error         lipgloss.Style
}

// NewModel creates a new TUI model with minimal/clean styling
func NewModel(db *storage.DB) Model {
	ctx, cancel := context.WithCancel(context.Background())

	return Model{
		db:     db,
		ctx:    ctx,
		cancel: cancel,
		view:   ViewDashboard,
		styles: newStyles(),
	}
}

// newStyles creates minimal/clean styles
func newStyles() Styles {
	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255"))

	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	statLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	statValue := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255"))

	change := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	changeAdded := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42"))

	changeRemoved := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	return Styles{
		Border:        border,
		Title:         title,
		Subtitle:      subtitle,
		StatLabel:     statLabel,
		StatValue:     statValue,
		Stat:          statLabel,
		Change:        change,
		ChangeAdded:   changeAdded,
		ChangeRemoved: changeRemoved,
		Help:          help,
		Error:         errorStyle,
	}
}

// Init initializes the TUI application
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadStatsCmd(m.db),
		loadRecentChangesCmd(m.db),
	)
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case statsMsg:
		m.stats = Stats(msg)
		m.statsLoaded = true
		m.err = nil
		return m, nil

	case changesMsg:
		m.recentChanges = []storage.Change(msg)
		m.changesLoaded = true
		m.err = nil
		return m, nil

	case pollingStatusMsg:
		m.pollingStatus = string(msg)
		return m, nil

	case pollingCompleteMsg:
		m.pollingActive = false
		return m, tea.Batch(
			loadStatsCmd(m.db),
			loadRecentChangesCmd(m.db),
		)

	case errMsg:
		m.err = error(msg)
		return m, nil
	}

	return m, nil
}

// handleKeyPress processes keyboard input
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit

	case "d":
		m.view = ViewDashboard
		return m, nil

	case "p":
		if !m.pollingActive {
			m.view = ViewPolling
			m.pollingActive = true
			return m, startPollingCmd(m.db, m.ctx)
		}
		return m, nil

	case "s":
		m.view = ViewSearch
		return m, nil

	case "?":
		m.view = ViewHelp
		return m, nil
	}

	return m, nil
}

// View renders the current view
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var content string

	switch m.view {
	case ViewDashboard:
		content = m.renderDashboard()
	case ViewPolling:
		content = m.renderPolling()
	case ViewSearch:
		content = m.renderSearch()
	case ViewHelp:
		content = m.renderHelp()
	}

	if m.err != nil {
		errLine := m.styles.Subtitle.Render("Error: " + m.err.Error())
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", errLine)
	}

	// Add footer
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, content, footer)
}

// renderFooter renders the bottom help bar
func (m Model) renderFooter() string {
	helps := []string{
		"[d]ashboard",
		"[p]oll",
		"[s]earch",
		"[?]help",
		"[q]uit",
	}

	helpText := lipgloss.JoinHorizontal(lipgloss.Left, helps...)
	return "\n" + m.styles.Help.Render(helpText)
}
