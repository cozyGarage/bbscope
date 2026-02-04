# TUI Implementation Notes

## Architecture

The TUI follows the Elm architecture pattern (Model-View-Update):

1. **Model** - Application state (`pkg/tui/tui.go`)
2. **Update** - Message handling and state transitions
3. **View** - Rendering based on current state

## Message Flow

```
User Input → tea.KeyMsg → handleKeyPress() → State Update → View Render
                                                ↓
Database Query → tea.Cmd → Async Operation → Response Msg → State Update
```

## View Modes

```go
type ViewMode int

const (
    ViewDashboard  // Default view with stats
    ViewPolling    // Live polling progress
    ViewSearch     // Target search interface
    ViewHelp       // Keyboard shortcuts
)
```

## Async Operations

All database operations run asynchronously using `tea.Cmd`:

- `loadStatsCmd()` - Fetch program/target/change counts
- `loadRecentChangesCmd()` - Get last 10 scope changes
- `startPollingCmd()` - Trigger platform polling (placeholder)

## Styling System

Uses Lipgloss for terminal styling:

```go
type Styles struct {
    Border        lipgloss.Style  // Rounded borders
    Title         lipgloss.Style  // Bold white
    Subtitle      lipgloss.Style  // Gray
    StatValue     lipgloss.Style  // Bold white
    ChangeAdded   lipgloss.Style  // Green
    ChangeRemoved lipgloss.Style  // Red
    Help          lipgloss.Style  // Gray
    Error         lipgloss.Style  // Red
}
```

## Integration Points

### Database Queries

- `db.ListPrograms()` - Get all programs
- `db.ListEntries()` - Get all targets
- `db.ListRecentChanges(since, limit)` - Get changes

### Polling (TODO)

Need to integrate with:
- `platforms.PlatformPoller` interface
- `cmd/poll.go` logic
- Progress reporting mechanism

### Search (TODO)

Potential approaches:
- Full-text search in PostgreSQL
- In-memory fuzzy matching
- Filter by category/platform/scope status

## Error Handling

Errors are captured and displayed in red:

```go
case errMsg:
    m.err = error(msg)
    // Error shown in view
```

## Performance Considerations

- Database queries run async (non-blocking)
- Limited to recent data (last 10 changes, 24h stats)
- Could add pagination for large result sets
- Consider caching for frequently accessed data

## Future Enhancements

1. **Real-time Updates**: WebSocket or polling for live changes
2. **Filters**: Multi-select platform/category filters
3. **Bookmarks**: Save favorite programs
4. **Export**: Export filtered results
5. **Themes**: Dark/light mode toggle
