// Package logging centralizes slog setup and context helpers.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

type ctxKey struct{}

// ContextWithLogger stores a slog logger in the context.
func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext retrieves the slog logger from the context, if available.
func FromContext(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return nil
	}
	if val := ctx.Value(ctxKey{}); val != nil {
		if lgr, ok := val.(*slog.Logger); ok {
			return lgr
		}
	}
	return nil
}

// NewLogger creates a slog logger configured for the given file/format/level.
// If logFile is "-", logs go to stdout without timestamps. For any other path,
// the file is created/appended and timestamps are kept.
func NewLogger(logFile, format, level string) (*slog.Logger, func() error, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, nil, err
	}

	var (
		writer io.Writer
		closer func() error
	)

	if logFile == "-" {
		writer = os.Stdout
	} else {
		// Path is user-supplied by design.
		//nolint:gosec
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		writer = f
		closer = f.Close
	}

	handlerOpts := slog.HandlerOptions{
		Level: lvl,
	}

	// Drop timestamp when writing to stdout.
	if logFile == "-" {
		handlerOpts.ReplaceAttr = func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return attr
		}
	}

	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(writer, &handlerOpts)
	case "text":
		handler = slog.NewTextHandler(writer, &handlerOpts)
	default:
		return nil, nil, fmt.Errorf("invalid log format: %s", format)
	}

	logger := slog.New(handler)
	return logger, closer, nil
}

func parseLevel(level string) (slog.Leveler, error) {
	switch strings.ToLower(level) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return nil, fmt.Errorf("invalid log level: %s", level)
	}
}
