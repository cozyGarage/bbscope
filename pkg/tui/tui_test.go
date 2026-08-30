package tui

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cozyGarage/bbscope/v2/pkg/pollrun"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// key builds a KeyMsg for a single character, matching what Bubble Tea
// delivers for an ordinary keypress.
func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// send applies one message and returns the resulting model.
func send(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	return got
}

func TestNewModel(t *testing.T) {
	model := NewModel(nil)

	if model.view != ViewDashboard {
		t.Errorf("Expected initial view to be Dashboard, got %v", model.view)
	}
	if model.quitting {
		t.Error("Expected quitting to be false initially")
	}
	if model.cancel == nil {
		t.Error("Expected cancel function to be initialized")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"shorter than limit", "short", 10, "short"},
		{"exactly at limit", "exactly10!", 10, "exactly10!"},
		{"needs ellipsis", "this is a very long string", 10, "this is..."},
		{"empty", "", 5, ""},

		// These used to panic on s[:maxLen-3] with a negative bound.
		{"limit below ellipsis", "abcdef", 2, "ab"},
		{"limit of one", "abcdef", 1, "a"},
		{"limit of zero", "abcdef", 0, ""},
		{"negative limit", "abcdef", -1, ""},
		{"limit equals ellipsis", "abcdef", 3, "abc"},

		// Rune-aware, so a multi-byte target is never cut mid-character.
		{"multibyte", "日本語のターゲット", 5, "日本..."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncate(tc.input, tc.maxLen); got != tc.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.expected)
			}
		})
	}
}

func TestNewStyles(t *testing.T) {
	styles := newStyles()
	if !styles.Title.GetBold() {
		t.Error("Expected Title style to be bold")
	}
}

// TestViewSwitching covers the global shortcuts from the dashboard.
func TestViewSwitching(t *testing.T) {
	tests := []struct {
		key  string
		want ViewMode
	}{
		{"b", ViewBrowser},
		{"s", ViewSearch},
		{"/", ViewSearch},
		{"?", ViewHelp},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			m := NewModel(nil)
			got := send(t, m, key(tc.key))
			if got.view != tc.want {
				t.Errorf("key %q: view = %v, want %v", tc.key, got.view, tc.want)
			}
		})
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			m := send(t, NewModel(nil), key(k))
			if !m.quitting {
				t.Errorf("key %q should set quitting", k)
			}
			if !strings.Contains(m.View(), "Goodbye") {
				t.Error("a quitting model should render the goodbye message")
			}
		})
	}
}

// TestSearchInputCapturesLetterKeys is the regression this routing exists for:
// while the search box has focus, typing must not fire the global single-letter
// shortcuts. Typing "db" once jumped to the dashboard and then the browser.
func TestSearchInputCapturesLetterKeys(t *testing.T) {
	m := NewModel(nil)
	m = send(t, m, key("s"))
	if m.view != ViewSearch {
		t.Fatalf("expected the search view, got %v", m.view)
	}

	for _, r := range "dbpq" {
		m = send(t, m, key(string(r)))
		if m.view != ViewSearch {
			t.Fatalf("typing %q left the search view for %v", r, m.view)
		}
		if m.quitting {
			t.Fatalf("typing %q quit the application", r)
		}
	}

	if got := m.search.input.Value(); got != "dbpq" {
		t.Errorf("search box contains %q, want %q", got, "dbpq")
	}
}

// TestSearchCtrlCStillQuits confirms the one key that must always work.
func TestSearchCtrlCStillQuits(t *testing.T) {
	m := send(t, NewModel(nil), key("s"))
	m = send(t, m, key("ctrl+c"))
	if !m.quitting {
		t.Error("ctrl+c must quit even while the search box has focus")
	}
}

func TestSearchEscapeReturnsToDashboard(t *testing.T) {
	m := send(t, NewModel(nil), key("s"))
	m = send(t, m, key("esc"))
	if m.view != ViewDashboard {
		t.Errorf("esc from search should return to the dashboard, got %v", m.view)
	}
}

// TestSearchResultsRender covers the view that used to say "coming soon".
func TestSearchResultsRender(t *testing.T) {
	m := NewModel(nil)
	m.view = ViewSearch
	m = send(t, m, searchResultsMsg{
		query: "example",
		results: []storage.Entry{
			{TargetNormalized: "api.example.com", Platform: "h1", ProgramURL: "https://h1/p", InScope: true},
			{TargetNormalized: "old.example.com", Platform: "h1", ProgramURL: "https://h1/p", InScope: false},
		},
	})

	if m.search.searching {
		t.Error("results should clear the searching flag")
	}
	out := m.View()
	for _, want := range []string{"api.example.com", "old.example.com", "2 match(es)"} {
		if !strings.Contains(out, want) {
			t.Errorf("search view missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "coming soon") {
		t.Error("the search placeholder text is still being rendered")
	}
}

func TestSearchEmptyResults(t *testing.T) {
	m := NewModel(nil)
	m.view = ViewSearch
	m = send(t, m, searchResultsMsg{query: "nothing", results: nil})
	if !strings.Contains(m.View(), "No matches") {
		t.Errorf("expected a no-matches message:\n%s", m.View())
	}
}

// TestSearchResultsAreCapped guards the unpaginated result list against a query
// that matches most of the database.
func TestSearchResultsAreCapped(t *testing.T) {
	many := make([]storage.Entry, maxSearchResults+50)
	for i := range many {
		many[i] = storage.Entry{TargetNormalized: "t.example.com", Platform: "h1"}
	}

	m := NewModel(nil)
	m.view = ViewSearch
	m = send(t, m, searchResultsMsg{query: "e", results: many})

	out := m.View()
	if !strings.Contains(out, "Showing the first") {
		t.Errorf("expected a truncation notice for %d results:\n%s", len(many), out)
	}
	if got := strings.Count(out, "t.example.com"); got > maxSearchResults+1 {
		t.Errorf("rendered %d rows, want at most %d", got, maxSearchResults)
	}
}

// TestBrowserDrillDownAndBack walks the platform -> program -> target levels.
func TestBrowserDrillDownAndBack(t *testing.T) {
	m := NewModel(nil)
	m.view = ViewBrowser
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = send(t, m, platformsMsg{{name: "h1", detail: "2 programs"}})
	if m.browser.loading {
		t.Error("receiving platforms should clear the loading flag")
	}
	if got := len(m.browser.list.Items()); got != 1 {
		t.Fatalf("expected 1 platform item, got %d", got)
	}

	m = send(t, m, key("enter"))
	if m.browser.level != levelPrograms {
		t.Fatalf("enter on a platform should open the programs level, got %v", m.browser.level)
	}
	if m.browser.platform != "h1" {
		t.Errorf("selected platform = %q, want %q", m.browser.platform, "h1")
	}

	prog := storage.Program{Platform: "h1", Handle: "acme", URL: "https://h1/acme"}
	m = send(t, m, programsMsg{{name: "acme", detail: "3 targets", program: prog}})
	m = send(t, m, key("enter"))
	if m.browser.level != levelTargets {
		t.Fatalf("enter on a program should open the targets level, got %v", m.browser.level)
	}
	if m.browser.program.Handle != "acme" {
		t.Errorf("selected program = %q, want %q", m.browser.program.Handle, "acme")
	}

	m = send(t, m, targetsMsg{{name: "api.acme.com", detail: "url · in scope"}})
	if !strings.Contains(m.View(), "api.acme.com") {
		t.Errorf("target list not rendered:\n%s", m.View())
	}

	// Walk back up.
	m = send(t, m, key("esc"))
	if m.browser.level != levelPrograms {
		t.Errorf("esc from targets should return to programs, got %v", m.browser.level)
	}
	m = send(t, m, key("esc"))
	if m.browser.level != levelPlatforms {
		t.Errorf("esc from programs should return to platforms, got %v", m.browser.level)
	}
}

// TestBrowserEscapeAtTopLevelLeavesBrowser confirms the browser declines the
// key at its root so the global handler can act on it.
func TestBrowserEscapeAtTopLevelLeavesBrowser(t *testing.T) {
	m := NewModel(nil)
	m.view = ViewBrowser
	m.browser.level = levelPlatforms

	m = send(t, m, key("esc"))
	if m.view != ViewDashboard {
		t.Errorf("esc at the top browser level should return to the dashboard, got %v", m.view)
	}
}

func TestBrowserInScopeFilterToggles(t *testing.T) {
	m := NewModel(nil)
	m.view = ViewBrowser

	if m.browser.inScopeOnly {
		t.Fatal("the in-scope filter should start off")
	}
	m = send(t, m, key("o"))
	if !m.browser.inScopeOnly {
		t.Error("'o' should enable the in-scope filter")
	}
	if !strings.Contains(m.View(), "in-scope only") {
		t.Errorf("the active filter should be visible:\n%s", m.View())
	}
	m = send(t, m, key("o"))
	if m.browser.inScopeOnly {
		t.Error("'o' should toggle the in-scope filter back off")
	}
}

// TestBrowserFilterPromptCapturesKeys mirrors the search-box regression for the
// list's own text filter.
func TestBrowserFilterPromptCapturesKeys(t *testing.T) {
	m := NewModel(nil)
	m.view = ViewBrowser
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = send(t, m, platformsMsg{{name: "h1"}, {name: "bc"}})

	// "/" opens the list's filter prompt.
	m = send(t, m, key("/"))
	if m.browser.list.FilterState() != list.Filtering {
		t.Fatalf("expected the list filter to be active, got %v", m.browser.list.FilterState())
	}

	m = send(t, m, key("q"))
	if m.quitting {
		t.Error("typing 'q' into the browser filter must not quit")
	}
	if m.view != ViewBrowser {
		t.Errorf("typing into the filter left the browser for %v", m.view)
	}
}

// TestPollingRunsThroughInjectedPoller drives the whole polling lifecycle: the
// view used to be a two-second sleep that reported completion regardless.
func TestPollingRunsThroughInjectedPoller(t *testing.T) {
	m := NewModelWithPoller(nil, func(ctx context.Context, events PollEvents) error {
		events.OnPlatform("h1")
		events.OnProgress(pollrun.Progress{Platform: "h1", Completed: 1, Total: 2})
		events.OnChanges([]storage.Change{{
			ChangeType: "added", Platform: "h1", TargetNormalized: "new.example.com",
			OccurredAt: time.Now(),
		}})
		events.OnProgress(pollrun.Progress{Platform: "h1", Completed: 2, Total: 2})
		return nil
	})

	m.view = ViewPolling
	next, cmd := m.startPolling()
	m = next.(Model)
	if !m.polling.active {
		t.Fatal("startPolling should mark the poll active")
	}

	// Drain the event channel the way the Bubble Tea runtime would.
	for i := 0; i < 20 && cmd != nil; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
		if m.polling.finished {
			break
		}
	}

	if !m.polling.finished {
		t.Fatal("the poll never reported completion")
	}
	if m.polling.err != nil {
		t.Fatalf("unexpected poll error: %v", m.polling.err)
	}
	if m.polling.progress.Completed != 2 || m.polling.progress.Total != 2 {
		t.Errorf("final progress = %d/%d, want 2/2", m.polling.progress.Completed, m.polling.progress.Total)
	}
	if len(m.polling.changes) != 1 {
		t.Fatalf("expected 1 recorded change, got %d", len(m.polling.changes))
	}

	out := m.View()
	for _, want := range []string{"Poll complete", "new.example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("polling view missing %q:\n%s", want, out)
		}
	}
}

// TestPollingSurfacesFailure confirms a failed poll is reported rather than
// silently rendering as success.
func TestPollingSurfacesFailure(t *testing.T) {
	wantErr := errors.New("no platforms configured")
	m := NewModelWithPoller(nil, func(ctx context.Context, events PollEvents) error {
		return wantErr
	})
	m.view = ViewPolling

	next, cmd := m.startPolling()
	m = next.(Model)

	for i := 0; i < 10 && cmd != nil; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		var nextModel tea.Model
		nextModel, cmd = m.Update(msg)
		m = nextModel.(Model)
		if m.polling.finished {
			break
		}
	}

	if !errors.Is(m.polling.err, wantErr) {
		t.Fatalf("polling error = %v, want %v", m.polling.err, wantErr)
	}
	if !strings.Contains(m.View(), "Poll failed") {
		t.Errorf("a failed poll should say so:\n%s", m.View())
	}
}

// TestPollingIgnoresRestartWhileActive guards against a second poll being
// launched over the first.
func TestPollingIgnoresRestartWhileActive(t *testing.T) {
	var started atomic.Int32
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })

	m := NewModelWithPoller(nil, func(ctx context.Context, events PollEvents) error {
		started.Add(1)
		<-block
		return nil
	})

	next, _ := m.startPolling()
	m = next.(Model)
	next, _ = m.startPolling()
	m = next.(Model)

	if !m.polling.active {
		t.Fatal("the first poll should still be active")
	}
	// Allow the goroutine to be scheduled before counting.
	time.Sleep(20 * time.Millisecond)
	if got := started.Load(); got > 1 {
		t.Errorf("poll started %d times, want 1", got)
	}
}

func TestPollingWithoutPollFuncReportsError(t *testing.T) {
	m := NewModelWithPoller(nil, nil)
	next, _ := m.startPolling()
	m = next.(Model)

	if m.polling.err == nil {
		t.Fatal("a model with no poll implementation should report an error")
	}
}

// TestErrorMessageClearsLoadingFlags stops a failed load from leaving the view
// stuck on "Loading...".
func TestErrorMessageClearsLoadingFlags(t *testing.T) {
	m := NewModel(nil)
	m.browser.loading = true
	m.search.searching = true

	m = send(t, m, errMsg(errors.New("db is down")))

	if m.browser.loading || m.search.searching {
		t.Error("an error must clear the pending-load flags")
	}
	if !strings.Contains(m.View(), "db is down") {
		t.Errorf("the error should be rendered:\n%s", m.View())
	}
}

func TestWindowResizePropagatesToLists(t *testing.T) {
	m := send(t, NewModel(nil), tea.WindowSizeMsg{Width: 120, Height: 50})

	if m.width != 120 || m.height != 50 {
		t.Errorf("model size = %dx%d, want 120x50", m.width, m.height)
	}
	if m.browser.list.Width() != 120 {
		t.Errorf("browser list width = %d, want 120", m.browser.list.Width())
	}
	if m.browser.list.Height() != 50-chromeHeight {
		t.Errorf("browser list height = %d, want %d", m.browser.list.Height(), 50-chromeHeight)
	}
}

// TestTinyWindowDoesNotPanic covers a terminal small enough that the chrome
// exceeds the available height.
func TestTinyWindowDoesNotPanic(t *testing.T) {
	m := send(t, NewModel(nil), tea.WindowSizeMsg{Width: 10, Height: 2})
	m.view = ViewBrowser
	_ = m.View()
}

func TestHelpDocumentsImplementedKeys(t *testing.T) {
	m := NewModel(nil)
	m.view = ViewHelp
	out := m.View()

	for _, want := range []string{"Browse", "Search", "enter", "esc", "in-scope"} {
		if !strings.Contains(out, want) {
			t.Errorf("help text missing %q:\n%s", want, out)
		}
	}
}

func TestFormatTimeAgo(t *testing.T) {
	now := time.Now()
	tests := []struct {
		at   time.Time
		want string
	}{
		{now.Add(-10 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-50 * time.Hour), "2d ago"},
	}
	for _, tc := range tests {
		if got := formatTimeAgo(tc.at); got != tc.want {
			t.Errorf("formatTimeAgo(%v) = %q, want %q", tc.at, got, tc.want)
		}
	}
}

func TestTargetDetail(t *testing.T) {
	got := targetDetail(storage.Entry{Category: "url", InScope: true, IsBBP: true, Description: "main API"})
	for _, want := range []string{"url", "in scope", "bounty", "main API"} {
		if !strings.Contains(got, want) {
			t.Errorf("targetDetail missing %q, got %q", want, got)
		}
	}

	got = targetDetail(storage.Entry{Category: "url"})
	if !strings.Contains(got, "out of scope") {
		t.Errorf("expected an out-of-scope marker, got %q", got)
	}
}

func TestRenderPollProgressBar(t *testing.T) {
	m := NewModel(nil)
	m.polling.platform = "h1"

	m.polling.progress = pollrun.Progress{Platform: "h1", Total: 0}
	if !strings.Contains(m.renderPollProgress(), "listing programs") {
		t.Error("an unknown total should render a listing message rather than a bar")
	}

	m.polling.progress = pollrun.Progress{Platform: "h1", Completed: 5, Total: 10}
	out := m.renderPollProgress()
	if !strings.Contains(out, "5/10") {
		t.Errorf("progress counter missing from %q", out)
	}
	if !strings.Contains(out, "█") || !strings.Contains(out, "░") {
		t.Errorf("expected a partially filled bar, got %q", out)
	}
}
