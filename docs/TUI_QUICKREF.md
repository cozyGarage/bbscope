# bbscope TUI - Quick Reference

## Launch

```bash
bbscope tui
```

Requires `db_url` to be configured; the TUI reads everything it displays from
the database.

## Keyboard Shortcuts

### Global

| Key | Action |
|-----|--------|
| `d` | Dashboard (stats + recent changes) |
| `b` | Browse platforms, programs and targets |
| `s` or `/` | Search targets |
| `p` | Poll every configured platform |
| `?` | Help screen |
| `q` | Quit |
| `Ctrl+C` | Quit from anywhere, including a focused text box |

Single-letter shortcuts are suppressed while a text box or list filter has
focus, so typing a query never triggers a view switch.

### Browse

| Key | Action |
|-----|--------|
| `↑` / `↓` / `j` / `k` | Move through the list |
| `Enter` | Open the selected platform or program |
| `Esc` / `Backspace` | Go back up a level (leaves the browser at the top) |
| `o` | Toggle the in-scope-only filter |
| `/` | Filter the current list by text |

### Search

| Key | Action |
|-----|--------|
| `Enter` | Run the query |
| `Esc` | Leave the search box |

## Views

### Dashboard (`d`)
- Program count, target count, 24h changes
- Last 5 scope changes (color-coded)
- Refreshes after a poll finishes

### Browse (`b`)
Three levels, with a breadcrumb showing where you are:

```
platforms  ->  programs  ->  targets
```

The platform level is built from `GetStats`, so each row shows its program and
in/out-of-scope counts. Programs show a target count. Targets show category,
scope status, bounty status and description. The in-scope filter (`o`) applies
to programs and targets and hides programs left with nothing in scope.

### Search (`s`)
Runs `SearchTargets`, which matches target names, descriptions and program
URLs, and includes targets that have since been removed (marked `~`). Results
are capped at 200 rows; narrow the query to see more.

### Poll (`p`)
Runs the same code path as `bbscope poll --db` through `pkg/pollrun`, showing a
per-platform progress bar and listing detected changes as they arrive. The
dashboard refreshes when the poll finishes.

## Implementation Files

```
cmd/tui.go            - Command entry point
pkg/tui/tui.go        - Model, view routing, key handling
pkg/tui/browser.go    - Platform/program/target drill-down
pkg/tui/search.go     - Query box and results
pkg/tui/polling.go    - Poll lifecycle and progress
pkg/tui/views.go      - Dashboard, help, shared rendering helpers
pkg/tui/tui_test.go   - Unit tests
```

## Dependencies

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles` (`list`, `textinput`)
- `github.com/charmbracelet/lipgloss`

## Testing

`NewModelWithPoller` injects a `PollFunc`, so the polling view can be driven in
tests without network access or credentials.

```bash
# Unit tests
go test ./pkg/tui/...

# Build & run
go build -o bbscope .
./bbscope tui
```

## Not Yet Implemented

- Copy-to-clipboard and bulk export of selected targets
- A visual diff of changes between two polls
- Editing `~/.bbscope.yaml` from within the TUI
- Choosing which platforms to poll from the polling view

## Styling

Minimal/clean design:
- Rounded borders (subtle gray)
- High contrast text
- Green for additions, red for removals
- No gradients or heavy decorations
