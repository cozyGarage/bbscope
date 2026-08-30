package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cozyGarage/bbscope/v2/pkg/pollrun"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// pollEventBuffer bounds the channel that carries poll events into the Bubble
// Tea loop. Polling continues to make progress if the UI falls behind; a full
// buffer only slows the workers rather than dropping updates.
const pollEventBuffer = 64

// maxPollChangeLines limits how much of a large poll's change list is retained
// for display.
const maxPollChangeLines = 100

// PollEvents is the set of callbacks a PollFunc reports through.
type PollEvents struct {
	OnPlatform func(platform string)
	OnProgress func(pollrun.Progress)
	OnChanges  func([]storage.Change)
}

// PollFunc runs one poll, reporting progress through the supplied callbacks.
// It is injectable so tests can drive the polling view without network access
// or credentials.
type PollFunc func(ctx context.Context, events PollEvents) error

// defaultPollFunc polls every configured platform and persists the results,
// which is the same path `bbscope poll --db` takes.
func defaultPollFunc(db *storage.DB) PollFunc {
	return func(ctx context.Context, events PollEvents) error {
		pollers, err := pollrun.BuildPollers(ctx, "", nil)
		if err != nil {
			return err
		}
		if len(pollers) == 0 {
			return fmt.Errorf("no platforms configured; set credentials with 'bbscope config set <key>'")
		}

		return pollrun.Run(ctx, pollers, pollrun.Options{
			DB: db,
			OnPlatformStart: func(platform string) {
				events.OnPlatform(platform)
			},
			OnProgress: func(p pollrun.Progress) {
				events.OnProgress(p)
			},
			OnChanges: func(_ context.Context, changes []storage.Change) {
				events.OnChanges(changes)
			},
		})
	}
}

// pollingState tracks an in-flight poll.
type pollingState struct {
	active   bool
	platform string
	progress pollrun.Progress
	changes  []storage.Change
	finished bool
	err      error

	// events carries messages from the polling goroutine into Update.
	events chan tea.Msg
}

// polling messages
type (
	pollPlatformMsg string
	pollProgressMsg pollrun.Progress
	pollChangesMsg  []storage.Change
	pollDoneMsg     struct{ err error }
)

// startPolling kicks off a poll and begins draining its event channel.
func (m Model) startPolling() (tea.Model, tea.Cmd) {
	if m.polling.active {
		return m, nil
	}
	if m.pollFunc == nil {
		m.polling.err = fmt.Errorf("polling is not available in this build")
		m.polling.finished = true
		return m, nil
	}

	events := make(chan tea.Msg, pollEventBuffer)
	m.polling = pollingState{active: true, events: events}
	m.err = nil

	ctx := m.ctx
	pollFunc := m.pollFunc

	go func() {
		defer close(events)

		// send drops the event rather than blocking forever once the context is
		// done, so a quit during a poll cannot deadlock the worker.
		send := func(msg tea.Msg) {
			select {
			case events <- msg:
			case <-ctx.Done():
			}
		}

		err := pollFunc(ctx, PollEvents{
			OnPlatform: func(platform string) { send(pollPlatformMsg(platform)) },
			OnProgress: func(p pollrun.Progress) { send(pollProgressMsg(p)) },
			OnChanges:  func(c []storage.Change) { send(pollChangesMsg(c)) },
		})
		send(pollDoneMsg{err: err})
	}()

	return m, waitForPollEvent(events)
}

// waitForPollEvent blocks in a command goroutine until the next poll event
// arrives. Each received event schedules another wait, which is how a streaming
// source is adapted to Bubble Tea's one-message-per-command model.
func waitForPollEvent(events chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *Model) updatePolling(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case pollPlatformMsg:
		m.polling.platform = string(msg)
		m.polling.progress = pollrun.Progress{}
		return waitForPollEvent(m.polling.events), true

	case pollProgressMsg:
		m.polling.progress = pollrun.Progress(msg)
		return waitForPollEvent(m.polling.events), true

	case pollChangesMsg:
		if len(m.polling.changes) < maxPollChangeLines {
			m.polling.changes = append(m.polling.changes, msg...)
		}
		return waitForPollEvent(m.polling.events), true

	case pollDoneMsg:
		m.polling.active = false
		m.polling.finished = true
		m.polling.err = msg.err
		// A finished poll changes the stored data the dashboard summarizes.
		return tea.Batch(loadStatsCmd(m.db), loadRecentChangesCmd(m.db)), true
	}
	return nil, false
}

// renderPolling draws live progress, or the outcome once the poll finishes.
func (m Model) renderPolling() string {
	title := m.styles.Title.Render("⚡ Live Polling")

	var lines []string

	switch {
	case m.polling.active:
		lines = append(lines, m.styles.Subtitle.Render("Polling platforms..."))
		if m.polling.platform != "" {
			lines = append(lines, "", m.renderPollProgress())
		}
	case m.polling.finished && m.polling.err != nil:
		lines = append(lines, m.styles.Error.Render("Poll failed: "+m.polling.err.Error()))
	case m.polling.finished:
		lines = append(lines, m.styles.ChangeAdded.Render("Poll complete."))
	default:
		lines = append(lines, m.styles.Subtitle.Render("Press 'p' to start polling"))
	}

	if len(m.polling.changes) > 0 {
		lines = append(lines, "", m.styles.Subtitle.Render(
			fmt.Sprintf("Changes detected (%d)", len(m.polling.changes))))
		for i, c := range m.polling.changes {
			if i >= 10 {
				lines = append(lines, m.styles.Subtitle.Render("..."))
				break
			}
			lines = append(lines, m.renderChangeLine(c))
		}
	} else if m.polling.finished && m.polling.err == nil {
		lines = append(lines, m.styles.Change.Render("No scope changes."))
	}

	return m.styles.Border.Render(lipgloss.JoinVertical(lipgloss.Left,
		append([]string{title, ""}, lines...)...))
}

// renderPollProgress draws a simple bar plus a completed/total counter.
func (m Model) renderPollProgress() string {
	p := m.polling.progress
	label := m.styles.StatLabel.Render(m.polling.platform)

	if p.Total <= 0 {
		return fmt.Sprintf("%s  %s", label, m.styles.Subtitle.Render("listing programs..."))
	}

	const barWidth = 30
	filled := p.Completed * barWidth / p.Total
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	return fmt.Sprintf("%s  %s  %s",
		label,
		m.styles.ChangeAdded.Render(bar),
		m.styles.StatValue.Render(fmt.Sprintf("%d/%d", p.Completed, p.Total)),
	)
}
