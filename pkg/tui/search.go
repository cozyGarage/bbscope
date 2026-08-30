package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// maxSearchResults caps how many rows are rendered. SearchTargets can match a
// very large slice of the database and the view is not paginated.
const maxSearchResults = 200

// searchState holds the query box and its results.
type searchState struct {
	input     textinput.Model
	results   []storage.Entry
	query     string
	searching bool
	searched  bool

	width  int
	height int
}

func newSearchState() searchState {
	ti := textinput.New()
	ti.Placeholder = "target, description or program URL"
	ti.CharLimit = 200
	ti.Prompt = "› "

	return searchState{input: ti}
}

func (s *searchState) setSize(width, height int) {
	s.width = width
	s.height = height
	if width > 4 {
		s.input.Width = width - 4
	}
}

type searchResultsMsg struct {
	query   string
	results []storage.Entry
}

// enterSearch focuses the query box.
func (m Model) enterSearch() (tea.Model, tea.Cmd) {
	m.err = nil
	m.search.input.Focus()
	return m, textinput.Blink
}

// searchKeyPress routes keys while the search box has focus.
//
// It consumes every key it is given. Enter and Escape are the view's own
// bindings and everything else belongs to the text input; letting any key fall
// through to the global handler would mean typing a query fired the
// single-letter view shortcuts. Ctrl-C is intercepted before this is called.
func (m Model) searchKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		query := strings.TrimSpace(m.search.input.Value())
		if query == "" {
			return m, nil
		}
		m.search.searching = true
		m.search.query = query
		return m, searchCmd(m.db, query)

	case "esc":
		m.search.input.Blur()
		m.view = ViewDashboard
		return m, nil
	}

	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(msg)
	return m, cmd
}

// updateSearch consumes the search view's own message types, reporting whether
// it handled the message.
func (m *Model) updateSearch(msg tea.Msg) bool {
	if msg, ok := msg.(searchResultsMsg); ok {
		m.search.searching = false
		m.search.searched = true
		m.search.query = msg.query
		m.search.results = msg.results
		return true
	}
	return false
}

func searchCmd(db *storage.DB, query string) tea.Cmd {
	return func() tea.Msg {
		results, err := db.SearchTargets(context.Background(), query)
		if err != nil {
			return errMsg(fmt.Errorf("search failed: %w", err))
		}
		return searchResultsMsg{query: query, results: results}
	}
}

// renderSearch draws the query box and the result rows.
func (m Model) renderSearch() string {
	title := m.styles.Title.Render("🔍 Search")
	box := m.search.input.View()

	var body string
	switch {
	case m.search.searching:
		body = m.styles.Subtitle.Render("Searching...")
	case !m.search.searched:
		body = m.styles.Subtitle.Render("Type a query and press enter.")
	case len(m.search.results) == 0:
		body = m.styles.Change.Render(fmt.Sprintf("No matches for %q.", m.search.query))
	default:
		body = m.renderSearchResults()
	}

	return m.styles.Border.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", box, "", body))
}

func (m Model) renderSearchResults() string {
	shown := m.search.results
	truncated := false
	if len(shown) > maxSearchResults {
		shown = shown[:maxSearchResults]
		truncated = true
	}

	header := m.styles.Subtitle.Render(fmt.Sprintf("%d match(es) for %q", len(m.search.results), m.search.query))

	lines := make([]string, 0, len(shown)+2)
	lines = append(lines, header, "")
	for _, e := range shown {
		marker := "•"
		style := m.styles.Change
		if !e.InScope {
			marker = "×"
			style = m.styles.ChangeRemoved
		}
		if e.IsHistorical {
			marker = "~"
		}

		line := fmt.Sprintf("%s %-40s %-12s %s",
			marker,
			truncate(e.TargetNormalized, 40),
			truncate(e.Platform, 12),
			truncate(e.ProgramURL, 44),
		)
		lines = append(lines, style.Render(line))
	}

	if truncated {
		lines = append(lines, "", m.styles.Subtitle.Render(
			fmt.Sprintf("Showing the first %d results; narrow the query to see more.", maxSearchResults)))
	}

	return strings.Join(lines, "\n")
}
