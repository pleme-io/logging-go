package logging

import (
	"fmt"
	"log/slog"
	"strings"
)

// ParseLevel parses a level name into a [slog.Level]. Accepted values are
// "debug", "info", "warn" (alias "warning"), and "error", case-insensitive and
// surrounding whitespace trimmed — matching the level vocabulary used by the
// Rust tracing stack. An empty or unrecognised value returns an error and the
// info level, so callers can fall back deterministically.
func ParseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("logging: unknown level %q", name)
	}
}
