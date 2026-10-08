package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type contextKey string

const (
	// RequestIDKey is the context key used for request tracking
	RequestIDKey contextKey = "request_id"
)

// Logger is a wrapper around *slog.Logger with contextual helpers.
type Logger struct {
	*slog.Logger
}

// New creates a new structured logger configured for the specified environment and log level.
func New(env string, levelStr string) *Logger {
	return NewWithOutput(env, levelStr, os.Stdout)
}

// NewWithOutput creates a structured logger writing to the provided writer.
func NewWithOutput(env string, levelStr string, w io.Writer) *Logger {
	var level slog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production", "staging", "uat":
		handler = slog.NewJSONHandler(w, opts)
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}

// WithContext returns a logger decorated with context attributes such as request_id.
func (l *Logger) WithContext(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return l.Logger
	}

	reqID, ok := ctx.Value(RequestIDKey).(string)
	if ok && reqID != "" {
		return l.Logger.With(slog.String("request_id", reqID))
	}

	return l.Logger
}
