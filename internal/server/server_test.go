package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thorstenkramm/dendrite-pulse/internal/api"
	"github.com/thorstenkramm/dendrite-pulse/internal/ping"
)

func TestPingHandler(t *testing.T) {
	e := buildRouter()

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
	e := buildRouter()

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
	e := buildRouter()

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
		errCh <- Run(ctx, ":0")
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
