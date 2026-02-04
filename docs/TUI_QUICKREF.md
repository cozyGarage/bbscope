# bbscope TUI - Quick Reference

## Launch

```bash
bbscope tui
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `d` | Dashboard (stats + recent changes) |
| `p` | Start live polling |
| `s` | Search targets |
| `?` | Help screen |
| `q` | Quit |
| `Ctrl+C` | Emergency exit |

## Views

### Dashboard (`d`)
- Program count, target count, 24h changes
- Last 5 scope changes (color-coded)
- Auto-refreshes after polling

### Live Polling (`p`)
- Trigger platform polling
- Real-time status updates
- Returns to dashboard when done

### Search (`s`)
- Placeholder (ready for implementation)
- Future: fuzzy search, filtering, clipboard

### Help (`?`)
- All keyboard shortcuts
- Navigation tips

## Implementation Files

```
cmd/tui.go           - Command entry point
pkg/tui/tui.go       - Main model & logic (260 lines)
pkg/tui/views.go     - Rendering & async ops (270 lines)
pkg/tui/tui_test.go  - Unit tests (50 lines)
```

## Dependencies Added

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles`
- `github.com/charmbracelet/lipgloss`
- `github.com/atotto/clipboard`

## Testing

```bash
# Unit tests
go test -v ./pkg/tui/...

# Build & run
go build -o bbscope .
./bbscope tui
```

## Next Steps

1. **Integration**: Wire up real polling to `cmd/poll.go`
2. **Search**: Implement fuzzy target search
3. **Clipboard**: Add copy-to-clipboard feature
4. **Progress**: Per-platform progress bars

## Styling

Minimal/clean design:
- Rounded borders (subtle gray)
- High contrast text
- Green for additions, red for removals
- No gradients or heavy decorations
