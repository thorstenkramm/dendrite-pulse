package files

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	jsonapi "github.com/thorstenkramm/dendrite-pulse/internal/api"
)

func TestMoveRenameFile(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.txt")
	require.NoError(t, os.WriteFile(sourcePath, []byte("hello"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	req.Header.Set(headerDestination, "/api/v1/files/public/renamed.txt")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.NotEmpty(t, rec.Header().Get(headerETag))

	var resp jsonapi.SingleResponse[Resource]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "/public/renamed.txt", resp.Data.Attributes.VirtualPath)

	_, err := os.Stat(sourcePath)
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	// #nosec G304 -- reading from test temp directory.
	data, err := os.ReadFile(filepath.Join(root, "renamed.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

func TestMoveAcrossRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootA, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{
		{Virtual: "/public", Source: rootA},
		{Virtual: "/docs", Source: rootB},
	})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	req.Header.Set(headerDestination, "/api/v1/files/docs/moved.txt")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	_, err := os.Stat(filepath.Join(rootA, "source.txt"))
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	// #nosec G304 -- reading from test temp directory.
	data, err := os.ReadFile(filepath.Join(rootB, "moved.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), data)
}

func TestMoveOverwriteBehavior(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "dest.txt"), []byte("old"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	t.Run("conflict by default", func(t *testing.T) {
		req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
		req.Header.Set(headerDestination, "/api/v1/files/public/dest.txt")
		rec := httptest.NewRecorder()

		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("overwrite replaces destination", func(t *testing.T) {
		req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
		req.Header.Set(headerDestination, "/api/v1/files/public/dest.txt")
		req.Header.Set(headerOverwrite, "T")
		rec := httptest.NewRecorder()

		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Empty(t, rec.Body.String())

		// #nosec G304 -- reading from test temp directory.
		data, err := os.ReadFile(filepath.Join(root, "dest.txt"))
		require.NoError(t, err)
		assert.Equal(t, []byte("new"), data)
	})
}

func TestMoveIfMatchMismatch(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	req.Header.Set(headerDestination, "/api/v1/files/public/renamed.txt")
	req.Header.Set(headerIfMatch, `W/"mismatch"`)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusPreconditionFailed, rec.Code)
}

func TestMoveDestinationParentMissing(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	req.Header.Set(headerDestination, "/api/v1/files/public/missing/target.txt")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMoveTopLevelForbidden(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public", nil)
	req.Header.Set(headerDestination, "/api/v1/files/public/renamed.txt")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	req = httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	req.Header.Set(headerDestination, "/api/v1/files/public")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestPatchMove(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	body := []byte(`{"op":"move","to":"/api/v1/files/public/renamed.txt","overwrite":false}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/files/public/source.txt", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestMoveMissingDestinationHeader(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMoveSameSourceAndDestination(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	req.Header.Set(headerDestination, "/api/v1/files/public/source.txt")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestMoveSourceNotFound(t *testing.T) {
	root := t.TempDir()

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/missing.txt", nil)
	req.Header.Set(headerDestination, "/api/v1/files/public/dest.txt")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMoveInvalidOverwriteHeader(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	req.Header.Set(headerDestination, "/api/v1/files/public/dest.txt")
	req.Header.Set(headerOverwrite, "INVALID")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMoveDestinationFullURLRejected(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	req.Header.Set(headerDestination, "http://example.com/api/v1/files/public/dest.txt")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMoveDestinationWithQueryRejected(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	req.Header.Set(headerDestination, "/api/v1/files/public/dest.txt?foo=bar")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMoveToDirectoryConflict(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "destdir"), 0o750))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	req := httptest.NewRequest("MOVE", "/api/v1/files/public/source.txt", nil)
	req.Header.Set(headerDestination, "/api/v1/files/public/destdir")
	req.Header.Set(headerOverwrite, "T")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestPatchMoveInvalidOp(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	body := []byte(`{"op":"rename","to":"/api/v1/files/public/renamed.txt"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/files/public/source.txt", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPatchMoveInvalidJSON(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	body := []byte(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/files/public/source.txt", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPatchMoveWithIfMatch(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.txt")
	require.NoError(t, os.WriteFile(sourcePath, []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	// First get the ETag
	info, err := os.Stat(sourcePath)
	require.NoError(t, err)
	etag := weakETag(info)

	body := []byte(`{"op":"move","to":"/api/v1/files/public/renamed.txt"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/files/public/source.txt", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, "application/json")
	req.Header.Set(headerIfMatch, etag)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestPatchMoveMissingContentType(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	body := []byte(`{"op":"move","to":"/api/v1/files/public/renamed.txt"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/files/public/source.txt", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPatchMoveMissingDestination(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600))

	svc := newMoveService(t, []Root{{Virtual: "/public", Source: root}})
	e := newMoveEcho(svc)

	body := []byte(`{"op":"move","to":""}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/files/public/source.txt", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func newMoveService(t *testing.T, roots []Root) *Service {
	t.Helper()

	svc, err := NewService(roots, defaultOptions())
	require.NoError(t, err)
	return svc
}

func newMoveEcho(svc *Service) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)
	return e
}
