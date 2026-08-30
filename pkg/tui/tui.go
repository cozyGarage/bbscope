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
	ViewBrowser
	ViewSearch
	ViewPolling
	ViewHelp
)

// chromeHeight is the number of rows the header and footer occupy, subtracted
// from the terminal height when sizing scrollable lists.
const chromeHeight = 6

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

	browser  browserState
	search   searchState
	polling  pollingState
	pollFunc PollFunc

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

// NewModel creates a new TUI model with minimal/clean styling.
func NewModel(db *storage.DB) Model {
	return NewModelWithPoller(db, defaultPollFunc(db))
}

// NewModelWithPoller builds a model with an injectable poll implementation.
// Tests use it to drive the polling view without touching the network.
func NewModelWithPoller(db *storage.DB, pollFunc PollFunc) Model {
	ctx, cancel := context.WithCancel(context.Background())

	return Model{
		db:       db,
		ctx:      ctx,
		cancel:   cancel,
		view:     ViewDashboard,
		styles:   newStyles(),
		browser:  newBrowserState(),
		search:   newSearchState(),
		pollFunc: pollFunc,
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
		m.browser.setSize(msg.Width, msg.Height-chromeHeight)
		m.search.setSize(msg.Width, msg.Height-chromeHeight)
		return m, nil

	case statsMsg:
		m.stats = Stats(msg)
		m.statsLoaded = true
		return m, nil

	case changesMsg:
		m.recentChanges = []storage.Change(msg)
		return m, nil

	case errMsg:
		m.err = error(msg)
		m.browser.loading = false
		m.search.searching = false
		return m, nil
	}

	// Views own the message types they introduced.
	if cmd, handled := m.updateBrowser(msg); handled {
		return m, cmd
	}
	if m.updateSearch(msg) {
		return m, nil
	}
	if cmd, handled := m.updatePolling(msg); handled {
		return m, cmd
	}

	return m, nil
}

// handleKeyPress processes keyboard input.
//
// The active view gets first refusal, because a view with a text input or a
// filter prompt must be able to consume ordinary letters before they are read
// as global shortcuts. Typing "d" into the search box should not jump to the
// dashboard.
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m.quit()
	}

	switch m.view {
	case ViewSearch:
		// The search box consumes every remaining key.
		return m.searchKeyPress(msg)
	case ViewBrowser:
		if handled, model, cmd := m.browserKeyPress(msg); handled {
			return model, cmd
		}
	}

	switch msg.String() {
	case "q":
		return m.quit()

	case "d":
		m.view = ViewDashboard
		return m, tea.Batch(loadStatsCmd(m.db), loadRecentChangesCmd(m.db))

	case "b":
		m.view = ViewBrowser
		return m.enterBrowser()

	case "s", "/":
		m.view = ViewSearch
		return m.enterSearch()

	case "p":
		m.view = ViewPolling
		return m.startPolling()

	case "?":
		m.view = ViewHelp
		return m, nil

	case "esc":
		m.view = ViewDashboard
		return m, nil
	}

	return m, nil
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.quitting = true
	if m.cancel != nil {
		m.cancel()
	}
	return m, tea.Quit
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
	case ViewBrowser:
		content = m.renderBrowser()
	case ViewSearch:
		content = m.renderSearch()
	case ViewPolling:
		content = m.renderPolling()
	case ViewHelp:
		content = m.renderHelp()
	}

	if m.err != nil {
		errLine := m.styles.Error.Render("Error: " + m.err.Error())
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", errLine)
	}

	return lipgloss.JoinVertical(lipgloss.Left, content, m.renderFooter())
}

// renderFooter renders the bottom help bar, listing the keys the current view
// actually responds to.
func (m Model) renderFooter() string {
	var helps []string

	switch m.view {
	case ViewBrowser:
		helps = []string{"[↑↓] move", "[enter] open", "[esc] back", "[o] in-scope filter", "[/] filter", "[?] help", "[q] quit"}
	case ViewSearch:
		helps = []string{"[enter] search", "[esc] dashboard", "[?]help", "[ctrl+c] quit"}
	case ViewPolling:
		helps = []string{"[d]ashboard", "[esc] back", "[?]help", "[q]uit"}
	default:
		helps = []string{"[d]ashboard", "[b]rowse", "[s]earch", "[p]oll", "[?]help", "[q]uit"}
	}

	return "\n" + m.styles.Help.Render(lipgloss.JoinHorizontal(lipgloss.Left, helps...))
}
