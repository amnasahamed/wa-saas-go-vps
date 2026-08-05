// Package logging provides a single slog.Logger configured for the runtime
// environment. Local dev pretty-prints, prod emits JSON for CloudWatch.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a *slog.Logger configured for the given environment and level.
// In dev we use a text handler for readability; in staging/prod we use JSON.
func New(env, level string) *slog.Logger {
	lvl := parseLevel(level)

	opts := &slog.HandlerOptions{
		Level: lvl,
	}

	var handler slog.Handler
	if env == "dev" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler).With(
		slog.String("env", env),
		slog.String("service", os.Getenv("SERVICE_NAME")),
	)
	return logger
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
