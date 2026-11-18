// Package logger builds the process logger. Assignment payloads are never logged.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// New returns a JSON logger writing to stdout.
func New(level string) *slog.Logger {
	return newWith(os.Stdout, level)
}

func newWith(w io.Writer, level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: parseLevel(level)}))
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
