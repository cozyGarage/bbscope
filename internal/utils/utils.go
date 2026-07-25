package utils

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
)

func AreSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Logger provides a logrus-compatible API using slog under the hood.
// This allows gradual migration without changing all call sites.
type Logger struct {
	handler slog.Handler
	level   slog.Level
}

// Log is the global logger instance
var Log = NewLogger()

// NewLogger creates a new Logger with default settings
func NewLogger() *Logger {
	l := &Logger{
		level: slog.LevelInfo,
	}
	l.updateHandler()
	return l
}

func (l *Logger) updateHandler() {
	opts := &slog.HandlerOptions{
		Level: l.level,
	}
	l.handler = slog.NewTextHandler(os.Stderr, opts)
	slog.SetDefault(slog.New(l.handler))
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level slog.Level) {
	l.level = level
	l.updateHandler()
}

// GetLevel returns the current logging level
func (l *Logger) GetLevel() slog.Level {
	return l.level
}

// Debug logs a debug message
func (l *Logger) Debug(args ...any) {
	slog.Debug(formatArgs(args...))
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...any) {
	slog.Debug(fmt.Sprintf(format, args...))
}

// Info logs an info message
func (l *Logger) Info(args ...any) {
	slog.Info(formatArgs(args...))
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...any) {
	slog.Info(fmt.Sprintf(format, args...))
}

// Warn logs a warning message
func (l *Logger) Warn(args ...any) {
	slog.Warn(formatArgs(args...))
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...any) {
	slog.Warn(fmt.Sprintf(format, args...))
}

// Error logs an error message
func (l *Logger) Error(args ...any) {
	slog.Error(formatArgs(args...))
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(args ...any) {
	slog.Error(formatArgs(args...))
	os.Exit(1)
}

// Fatalf logs a formatted fatal message and exits
func (l *Logger) Fatalf(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

// WithField returns a logger with an additional field (for compatibility)
func (l *Logger) WithField(key string, value any) *LogEntry {
	return &LogEntry{
		attrs: []slog.Attr{slog.Any(key, value)},
	}
}

// WithFields returns a logger with additional fields (for compatibility)
func (l *Logger) WithFields(fields map[string]any) *LogEntry {
	attrs := make([]slog.Attr, 0, len(fields))
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	return &LogEntry{attrs: attrs}
}

// LogEntry represents a log entry with fields
type LogEntry struct {
	attrs []slog.Attr
}

func (e *LogEntry) log(level slog.Level, msg string) {
	args := make([]any, len(e.attrs))
	for i, attr := range e.attrs {
		args[i] = attr
	}
	slog.Log(context.Background(), level, msg, args...)
}

func (e *LogEntry) Info(args ...any) { e.log(slog.LevelInfo, formatArgs(args...)) }
func (e *LogEntry) Infof(format string, args ...any) {
	e.log(slog.LevelInfo, fmt.Sprintf(format, args...))
}
func (e *LogEntry) Debug(args ...any) { e.log(slog.LevelDebug, formatArgs(args...)) }
func (e *LogEntry) Debugf(format string, args ...any) {
	e.log(slog.LevelDebug, fmt.Sprintf(format, args...))
}
func (e *LogEntry) Warn(args ...any) { e.log(slog.LevelWarn, formatArgs(args...)) }
func (e *LogEntry) Warnf(format string, args ...any) {
	e.log(slog.LevelWarn, fmt.Sprintf(format, args...))
}
func (e *LogEntry) Error(args ...any) { e.log(slog.LevelError, formatArgs(args...)) }
func (e *LogEntry) Errorf(format string, args ...any) {
	e.log(slog.LevelError, fmt.Sprintf(format, args...))
}

// Helper function for formatting variadic args
func formatArgs(args ...any) string {
	if len(args) == 0 {
		return ""
	}
	if len(args) == 1 {
		switch val := args[0].(type) {
		case string:
			return val
		case error:
			return val.Error()
		default:
			return fmt.Sprintf("%v", val)
		}
	}
	parts := make([]string, len(args))
	for i, arg := range args {
		switch val := arg.(type) {
		case string:
			parts[i] = val
		case error:
			parts[i] = val.Error()
		default:
			parts[i] = fmt.Sprintf("%v", val)
		}
	}
	return strings.Join(parts, " ")
}

// SetLogLevel sets the log level from a string (compatibility function)
func SetLogLevel(level string) {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warning", "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	case "fatal":
		slogLevel = slog.LevelError // slog doesn't have fatal, use error
	default:
		slog.Error("Bad log level string", "level", level)
		os.Exit(1)
	}
	Log.SetLevel(slogLevel)
}

// IsCIDR checks if a string is a valid CIDR range (x.x.x.x/xx)
func IsCIDR(cidr string) bool {
	if !strings.Contains(cidr, "/") {
		return false
	}
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}

// IsIP checks if a string is a valid IP address (IPv4 or IPv6)
func IsIP(ip string) bool {
	// Remove any surrounding square brackets for IPv6 addresses
	ip = strings.Trim(ip, "[]")

	// Parse the IP address
	parsedIP := net.ParseIP(ip)

	// If parsedIP is not nil, it's a valid IP address
	return parsedIP != nil
}

// IsIPRange checks if a string is a valid IP range in the format x.x.x.x-y.y.y.y
func IsIPRange(ipRange string) bool {
	parts := strings.Split(ipRange, "-")
	if len(parts) != 2 {
		return false
	}

	startIP := net.ParseIP(strings.TrimSpace(parts[0]))
	endIP := net.ParseIP(strings.TrimSpace(parts[1]))

	return startIP != nil && endIP != nil
}

// RedactURL redacts the password from a URL string for safe logging.
// Example: "postgres://user:secret@host/db" -> "postgres://user:****@host/db"
func RedactURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// If we can't parse it, return a generic redacted message
		return "[invalid URL]"
	}

	if parsed.User == nil {
		return rawURL // No credentials to redact
	}

	// Check if password exists
	if _, hasPassword := parsed.User.Password(); hasPassword {
		parsed.User = url.UserPassword(parsed.User.Username(), "****")
	}

	return parsed.String()
}
