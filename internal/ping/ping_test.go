package ping

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thorstenkramm/dendrite-pulse/internal/api"
)

func TestHandler(t *testing.T) {
	e := echo.New()
	RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, api.ContentType, rec.Header().Get("Content-Type"))

	var resp Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Equal(t, "pong", resp.Data.Attributes.Message)
	assert.Equal(t, "ping", resp.Data.Type)
	assert.Equal(t, "ping", resp.Data.ID)
}

func TestHandler_ResponseStructure(t *testing.T) {
	e := echo.New()
	RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Verify pagination meta
	assert.Equal(t, 1, resp.Meta.Page.CurrentPage)
	assert.Equal(t, 1, resp.Meta.Page.From)
	assert.Equal(t, 1, resp.Meta.Page.LastPage)
	assert.Equal(t, 1, resp.Meta.Page.PerPage)
	assert.Equal(t, 1, resp.Meta.Page.To)
	assert.Equal(t, 1, resp.Meta.Page.Total)

	// Verify links
	assert.Equal(t, "/api/v1/ping", resp.Links.Self)
	assert.Equal(t, "/api/v1/ping", resp.Links.First)
	assert.Equal(t, "/api/v1/ping", resp.Links.Last)
}

func TestRegisterRoutes(t *testing.T) {
	e := echo.New()
	RegisterRoutes(e)

	routes := e.Routes()
	var found bool
	for _, r := range routes {
		if r.Path == "/api/v1/ping" && r.Method == http.MethodGet {
			found = true
			break
		}
	}
	assert.True(t, found, "expected /api/v1/ping GET route to be registered")
}
