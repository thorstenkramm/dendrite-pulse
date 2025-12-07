package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thorstenkramm/dendrite-pulse/internal/api"
	"github.com/thorstenkramm/dendrite-pulse/internal/logging"
	"github.com/thorstenkramm/dendrite-pulse/internal/ping"
)

func TestPingHandler(t *testing.T) {
	e := buildRouter(Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, api.ContentType, rec.Header().Get("Content-Type"))

	var resp ping.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Equal(t, "pong", resp.Data.Attributes.Message)
	assert.Equal(t, "ping", resp.Data.Type)
	assert.Equal(t, "ping", resp.Data.ID)
	assert.Equal(t, 1, resp.Meta.Page.CurrentPage)
	assert.Equal(t, "/api/v1/ping", resp.Links.Self)
}

func TestNotFoundHandler(t *testing.T) {
	e := buildRouter(Config{})

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, api.ContentType, rec.Header().Get("Content-Type"))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	require.Len(t, resp.Errors, 1)
	assert.Equal(t, "404", resp.Errors[0].Status)
	assert.Equal(t, http.StatusText(http.StatusNotFound), resp.Errors[0].Title)
	assert.Equal(t, http.StatusText(http.StatusNotFound), resp.Errors[0].Detail)
}

func TestMethodNotAllowed(t *testing.T) {
	e := buildRouter(Config{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, api.ContentType, rec.Header().Get("Content-Type"))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	require.Len(t, resp.Errors, 1)
	assert.Equal(t, "405", resp.Errors[0].Status)
	assert.Equal(t, http.StatusText(http.StatusMethodNotAllowed), resp.Errors[0].Title)
}

func TestRun_GracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, ":0", Config{})
	}()

	// Give the server time to start
	time.Sleep(50 * time.Millisecond)

	// Trigger graceful shutdown
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func TestSlogRequestLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := Config{
		Logger:      logger,
		LogRequests: true,
	}
	e := buildRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	req.Header.Set("User-Agent", "test-agent")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "new request")
	assert.Contains(t, logOutput, "path=/api/v1/ping")
	assert.Contains(t, logOutput, "method=GET")
	assert.Contains(t, logOutput, "user_agent=test-agent")
}

func TestSlogRequestLogger_WithRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := Config{
		Logger:      logger,
		LogRequests: true,
	}
	e := buildRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// RequestID middleware should have generated an ID
	logOutput := buf.String()
	assert.Contains(t, logOutput, "request_id=")
}

func TestSlogRequestLogger_ContextLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := Config{
		Logger:      logger,
		LogRequests: true,
	}
	e := buildRouter(cfg)

	// Add a test handler that checks for logger in context
	var ctxLogger *slog.Logger
	e.GET("/test-ctx", func(c echo.Context) error {
		ctxLogger = logging.FromContext(c.Request().Context())
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test-ctx", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotNil(t, ctxLogger, "logger should be available in request context")
}
