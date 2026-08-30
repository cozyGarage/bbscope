package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// browserLevel is the depth of the platform -> program -> target drill-down.
type browserLevel int

const (
	levelPlatforms browserLevel = iota
	levelPrograms
	levelTargets
)

func (l browserLevel) String() string {
	switch l {
	case levelPrograms:
		return "Programs"
	case levelTargets:
		return "Targets"
	default:
		return "Platforms"
	}
}

// browserItem is one row at any level. Only the fields relevant to the current
// level are populated.
type browserItem struct {
	name    string
	detail  string
	program storage.Program
}

func (i browserItem) Title() string       { return i.name }
func (i browserItem) Description() string { return i.detail }
func (i browserItem) FilterValue() string { return i.name + " " + i.detail }

// browserState holds the drill-down navigation state.
type browserState struct {
	level   browserLevel
	list    list.Model
	loading bool

	// Selection carried down the levels.
	platform string
	program  storage.Program

	// inScopeOnly filters out out-of-scope targets and, at the program level,
	// programs that have no in-scope targets left.
	inScopeOnly bool

	width  int
	height int
}

func newBrowserState() browserState {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)

	return browserState{level: levelPlatforms, list: l}
}

func (b *browserState) setSize(width, height int) {
	b.width = width
	b.height = height
	if height < 3 {
		height = 3
	}
	b.list.SetSize(width, height)
}

// browser messages
type (
	platformsMsg []browserItem
	programsMsg  []browserItem
	targetsMsg   []browserItem
)

// enterBrowser opens the browser at the platform level.
func (m Model) enterBrowser() (tea.Model, tea.Cmd) {
	m.browser.level = levelPlatforms
	m.browser.loading = true
	m.err = nil
	return m, loadPlatformsCmd(m.db)
}

// browserKeyPress handles keys while the browser is focused. It returns
// handled=false for keys the browser does not own, letting the global bindings
// see them.
func (m Model) browserKeyPress(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	// While the list's own filter prompt is open it must receive every
	// keystroke, including plain letters that are otherwise global shortcuts.
	if m.browser.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.browser.list, cmd = m.browser.list.Update(msg)
		return true, m, cmd
	}

	switch msg.String() {
	case "enter":
		model, cmd := m.browserDrillDown()
		return true, model, cmd

	case "esc", "backspace":
		if m.browser.level == levelPlatforms {
			// Nothing to go back to; let the global handler return to the
			// dashboard.
			return false, m, nil
		}
		model, cmd := m.browserGoBack()
		return true, model, cmd

	case "o":
		m.browser.inScopeOnly = !m.browser.inScopeOnly
		model, cmd := m.browserReload()
		return true, model, cmd
	}

	var cmd tea.Cmd
	m.browser.list, cmd = m.browser.list.Update(msg)
	return true, m, cmd
}

func (m Model) browserDrillDown() (tea.Model, tea.Cmd) {
	item, ok := m.browser.list.SelectedItem().(browserItem)
	if !ok {
		return m, nil
	}

	switch m.browser.level {
	case levelPlatforms:
		m.browser.platform = item.name
		m.browser.level = levelPrograms
		m.browser.loading = true
		return m, loadProgramsCmd(m.db, item.name, m.browser.inScopeOnly)

	case levelPrograms:
		m.browser.program = item.program
		m.browser.level = levelTargets
		m.browser.loading = true
		return m, loadTargetsCmd(m.db, item.program, m.browser.inScopeOnly)
	}

	// Targets are leaves.
	return m, nil
}

func (m Model) browserGoBack() (tea.Model, tea.Cmd) {
	switch m.browser.level {
	case levelTargets:
		m.browser.level = levelPrograms
		m.browser.loading = true
		return m, loadProgramsCmd(m.db, m.browser.platform, m.browser.inScopeOnly)
	case levelPrograms:
		m.browser.level = levelPlatforms
		m.browser.loading = true
		return m, loadPlatformsCmd(m.db)
	}
	return m, nil
}

// browserReload re-fetches the current level, used when a filter toggles.
func (m Model) browserReload() (tea.Model, tea.Cmd) {
	m.browser.loading = true
	switch m.browser.level {
	case levelPrograms:
		return m, loadProgramsCmd(m.db, m.browser.platform, m.browser.inScopeOnly)
	case levelTargets:
		return m, loadTargetsCmd(m.db, m.browser.program, m.browser.inScopeOnly)
	default:
		return m, loadPlatformsCmd(m.db)
	}
}

// updateBrowser consumes the browser's own message types.
func (m *Model) updateBrowser(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case platformsMsg:
		m.browser.loading = false
		return m.browser.list.SetItems(toListItems(msg)), true
	case programsMsg:
		m.browser.loading = false
		return m.browser.list.SetItems(toListItems(msg)), true
	case targetsMsg:
		m.browser.loading = false
		return m.browser.list.SetItems(toListItems(msg)), true
	}
	return nil, false
}

func toListItems(items []browserItem) []list.Item {
	out := make([]list.Item, 0, len(items))
	for _, i := range items {
		out = append(out, i)
	}
	return out
}

// loadPlatformsCmd builds the top level from per-platform statistics.
func loadPlatformsCmd(db *storage.DB) tea.Cmd {
	return func() tea.Msg {
		stats, err := db.GetStats(context.Background())
		if err != nil {
			return errMsg(fmt.Errorf("failed to load platforms: %w", err))
		}

		items := make([]browserItem, 0, len(stats))
		for _, s := range stats {
			items = append(items, browserItem{
				name: s.Platform,
				detail: fmt.Sprintf("%d programs · %d in scope · %d out of scope",
					s.ProgramCount, s.InScopeCount, s.OutOfScopeCount),
			})
		}
		return platformsMsg(items)
	}
}

// loadProgramsCmd lists one platform's programs, annotated with a target count
// so the user can see which are worth opening.
func loadProgramsCmd(db *storage.DB, platform string, inScopeOnly bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		programs, err := db.ListPrograms(ctx)
		if err != nil {
			return errMsg(fmt.Errorf("failed to load programs: %w", err))
		}

		entries, err := db.ListEntries(ctx, storage.ListOptions{
			Platform:   platform,
			IncludeOOS: !inScopeOnly,
		})
		if err != nil {
			return errMsg(fmt.Errorf("failed to load targets: %w", err))
		}

		counts := make(map[string]int, len(entries))
		for _, e := range entries {
			counts[e.ProgramURL]++
		}

		items := make([]browserItem, 0)
		for _, p := range programs {
			if !strings.EqualFold(p.Platform, platform) || p.Disabled || p.IsIgnored {
				continue
			}
			n := counts[p.URL]
			if inScopeOnly && n == 0 {
				continue
			}
			name := p.Handle
			if name == "" {
				name = p.URL
			}
			items = append(items, browserItem{
				name:    name,
				detail:  fmt.Sprintf("%d targets · %s", n, p.URL),
				program: p,
			})
		}

		sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })
		return programsMsg(items)
	}
}

// loadTargetsCmd lists one program's targets.
func loadTargetsCmd(db *storage.DB, program storage.Program, inScopeOnly bool) tea.Cmd {
	return func() tea.Msg {
		entries, err := db.ListEntries(context.Background(), storage.ListOptions{
			Platform:      program.Platform,
			ProgramFilter: program.URL,
			IncludeOOS:    !inScopeOnly,
		})
		if err != nil {
			return errMsg(fmt.Errorf("failed to load targets: %w", err))
		}

		items := make([]browserItem, 0, len(entries))
		for _, e := range entries {
			items = append(items, browserItem{
				name:   e.TargetNormalized,
				detail: targetDetail(e),
			})
		}
		return targetsMsg(items)
	}
}

// targetDetail summarizes an entry for the list's second line.
func targetDetail(e storage.Entry) string {
	parts := []string{e.Category}
	if e.InScope {
		parts = append(parts, "in scope")
	} else {
		parts = append(parts, "out of scope")
	}
	if e.IsBBP {
		parts = append(parts, "bounty")
	}
	if e.Description != "" {
		parts = append(parts, e.Description)
	}
	return strings.Join(parts, " · ")
}

// renderBrowser draws the current level with a breadcrumb.
func (m Model) renderBrowser() string {
	title := m.styles.Title.Render("📂 Browse")

	crumbs := []string{"platforms"}
	if m.browser.level >= levelPrograms && m.browser.platform != "" {
		crumbs = append(crumbs, m.browser.platform)
	}
	if m.browser.level == levelTargets {
		name := m.browser.program.Handle
		if name == "" {
			name = m.browser.program.URL
		}
		crumbs = append(crumbs, name)
	}

	header := m.styles.Subtitle.Render(strings.Join(crumbs, " / "))
	if m.browser.inScopeOnly {
		header += m.styles.Subtitle.Render("   [in-scope only]")
	}

	var body string
	switch {
	case m.browser.loading:
		body = m.styles.Subtitle.Render("Loading...")
	case len(m.browser.list.Items()) == 0:
		body = m.styles.Change.Render("Nothing to show at this level.")
	default:
		body = m.browser.list.View()
	}

	return m.styles.Border.Render(lipgloss.JoinVertical(lipgloss.Left, title, header, "", body))
}
