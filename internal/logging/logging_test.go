package logging

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextWithLogger_And_FromContext(t *testing.T) {
	t.Run("stores and retrieves logger", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		ctx := ContextWithLogger(context.Background(), logger)

		got := FromContext(ctx)
		assert.Equal(t, logger, got)
	})

	t.Run("returns nil for nil logger", func(t *testing.T) {
		ctx := ContextWithLogger(context.Background(), nil)
		got := FromContext(ctx)
		assert.Nil(t, got)
	})

	//nolint:staticcheck // Testing nil context handling intentionally
	t.Run("returns nil for nil context", func(t *testing.T) {
		got := FromContext(nil)
		assert.Nil(t, got)
	})

	t.Run("returns nil when no logger in context", func(t *testing.T) {
		got := FromContext(context.Background())
		assert.Nil(t, got)
	})
}

func TestNewLogger_StdoutWithoutTimestamp(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger, closer, err := NewLogger("-", "text", "info")
	require.NoError(t, err)
	assert.Nil(t, closer)

	logger.Info("test message")

	require.NoError(t, w.Close())
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "test message")
	assert.NotContains(t, output, "time=")
}

func TestNewLogger_FileWithTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	logger, closer, err := NewLogger(logFile, "text", "info")
	require.NoError(t, err)
	require.NotNil(t, closer)
	defer func() { _ = closer() }()

	logger.Info("test message")

	content, err := os.ReadFile(logFile) //nolint:gosec // test file path is safe
	require.NoError(t, err)

	assert.Contains(t, string(content), "test message")
	assert.Contains(t, string(content), "time=")
}

func TestNewLogger_JSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	logger, closer, err := NewLogger(logFile, "json", "info")
	require.NoError(t, err)
	defer func() { _ = closer() }()

	logger.Info("test message")

	content, err := os.ReadFile(logFile) //nolint:gosec // test file path is safe
	require.NoError(t, err)

	assert.Contains(t, string(content), `"msg":"test message"`)
}

func TestNewLogger_InvalidFormat(t *testing.T) {
	_, _, err := NewLogger("-", "invalid", "info")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log format")
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	_, _, err := NewLogger("-", "text", "invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestNewLogger_InvalidFilePath(t *testing.T) {
	_, _, err := NewLogger("/nonexistent/path/test.log", "text", "info")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open log file")
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
		wantErr  bool
	}{
		{"", slog.LevelInfo, false},
		{"info", slog.LevelInfo, false},
		{"INFO", slog.LevelInfo, false},
		{"debug", slog.LevelDebug, false},
		{"DEBUG", slog.LevelDebug, false},
		{"warn", slog.LevelWarn, false},
		{"WARN", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"ERROR", slog.LevelError, false},
		{"invalid", slog.LevelInfo, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseLevel(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestLogLevelFiltering(t *testing.T) {
	t.Run("debug level logs debug messages", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		logger.Debug("debug message")
		assert.True(t, strings.Contains(buf.String(), "debug message"))
	})

	t.Run("info level filters debug messages", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		logger.Debug("debug message")
		assert.False(t, strings.Contains(buf.String(), "debug message"))
	})
}
