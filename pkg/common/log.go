package common

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"
)

// Logger writes structured diagnostics to stderr, never to stdout, so JSON
// reports stay clean (FR-030).
type Logger struct {
	*slog.Logger
	timing io.Writer
}

// NewLogger builds a stderr-backed structured logger. When timingEnabled is
// set, Timing lines are written to stderr and never to stdout.
func NewLogger(timingEnabled bool) *Logger {
	tw := io.Discard
	if timingEnabled {
		tw = os.Stderr
	}
	return &Logger{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		timing: tw,
	}
}

// Timing writes a timestamped timing line to stderr when --timing is enabled.
func (l *Logger) Timing(format string, args ...any) {
	if l.timing == io.Discard {
		return
	}
	ts := time.Now().Format(time.RFC3339Nano)
	_, _ = fmt.Fprintf(l.timing, "%s %s\n", ts, fmt.Sprintf(format, args...))
}

// ColorEnabled reports whether ANSI color should be used: the NO_COLOR env var
// is unset AND the output stream is a terminal.
func ColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
