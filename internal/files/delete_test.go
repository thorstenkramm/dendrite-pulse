package files

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o600))

	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/target.txt", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	_, err := os.Stat(target)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	link := filepath.Join(root, "link.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o600))
	require.NoError(t, os.Symlink(target, link))

	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/link.txt", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	_, err := os.Lstat(link)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(target)
	require.NoError(t, err)
}

func TestDeleteFolderBehavior(t *testing.T) {
	root := t.TempDir()
	emptyDir := filepath.Join(root, "empty")
	require.NoError(t, os.MkdirAll(emptyDir, 0o750))
	fullDir := filepath.Join(root, "full")
	require.NoError(t, os.MkdirAll(fullDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(fullDir, "file.txt"), []byte("data"), 0o600))

	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	t.Run("empty folder without recursive", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/empty", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
		_, err := os.Stat(emptyDir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("non-empty folder without recursive", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/full", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusConflict, rec.Code)
		_, err := os.Stat(fullDir)
		require.NoError(t, err)
	})

	t.Run("non-empty folder with recursive", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/full?recursive=true", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
		_, err := os.Stat(fullDir)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestDeleteIfMatchMismatch(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o600))

	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/target.txt", nil)
	req.Header.Set(headerIfMatch, `W/"mismatch"`)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusPreconditionFailed, rec.Code)
	_, err := os.Stat(target)
	require.NoError(t, err)
}

func TestDeleteTopLevelForbidden(t *testing.T) {
	root := t.TempDir()
	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestDeleteInvalidRecursiveParam(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o600))

	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/target.txt?recursive=maybe", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeleteNonExistentFile(t *testing.T) {
	root := t.TempDir()

	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/missing.txt", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteWithMatchingIfMatch(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o600))

	info, err := os.Stat(target)
	require.NoError(t, err)
	etag := weakETag(info)

	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/target.txt", nil)
	req.Header.Set(headerIfMatch, etag)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	_, err = os.Stat(target)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteRecursiveNumericValues(t *testing.T) {
	root := t.TempDir()

	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	t.Run("recursive=1 deletes non-empty folder", func(t *testing.T) {
		dir := filepath.Join(root, "dir1")
		require.NoError(t, os.MkdirAll(dir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0o600))

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/dir1?recursive=1", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("recursive=0 rejects non-empty folder", func(t *testing.T) {
		dir := filepath.Join(root, "dir0")
		require.NoError(t, os.MkdirAll(dir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0o600))

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/dir0?recursive=0", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusConflict, rec.Code)
		_, err := os.Stat(dir)
		require.NoError(t, err)
	})
}

func TestDeleteEmptyFolderWithRecursive(t *testing.T) {
	root := t.TempDir()
	emptyDir := filepath.Join(root, "emptydir")
	require.NoError(t, os.MkdirAll(emptyDir, 0o750))

	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/emptydir?recursive=true", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	_, err := os.Stat(emptyDir)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteFolderIfMatch(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "folder")
	require.NoError(t, os.MkdirAll(dir, 0o750))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	etag := weakETag(info)

	svc := newDeleteService(t, []Root{{Virtual: "/public", Source: root}})
	e := newDeleteEcho(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/public/folder", nil)
	req.Header.Set(headerIfMatch, etag)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	_, err = os.Stat(dir)
	assert.True(t, os.IsNotExist(err))
}

func newDeleteService(t *testing.T, roots []Root) *Service {
	t.Helper()

	svc, err := NewService(roots, defaultOptions())
	require.NoError(t, err)
	return svc
}

func newDeleteEcho(svc *Service) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)
	return e
}
