package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thorstenkramm/dendrite-pulse/internal/api"
)

func TestListDirectoryHandler(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "report.txt"), []byte("report"), 0o600))
	subDir := filepath.Join(root, "Docs & Notes")
	require.NoError(t, os.MkdirAll(subDir, 0o750))

	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, api.ContentType, rec.Header().Get(echo.HeaderContentType))

	var resp Response
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	assert.Len(t, resp.Data, 2)
	assert.Equal(t, "files", resp.Data[0].Type)
	assert.NotEmpty(t, resp.Data[0].Attributes.ResourceKind)
	assert.Contains(t, resp.Links.Self, "/api/v1/files/public")
	assert.NotNil(t, resp.Meta)
	assert.Equal(t, 2, resp.Meta.TotalCount)
}

func TestFileDownloadHandler(t *testing.T) {
	root := t.TempDir()
	content := []byte("hello world")
	filePath := filepath.Join(root, "hello.txt")
	require.NoError(t, os.WriteFile(filePath, content, 0o600))

	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public/hello.txt", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get(echo.HeaderContentType))
	assert.Equal(t, content, rec.Body.Bytes())
}

func TestPathTraversalReturnsBadRequest(t *testing.T) {
	root := t.TempDir()
	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public/../etc/passwd", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDirectoryListingAndDownload_VirtualSlash(t *testing.T) {
	root := t.TempDir()
	fileName := "Wolfgarten Voißel.gpx"
	fileContent := []byte("content")
	require.NoError(t, os.WriteFile(filepath.Join(root, fileName), fileContent, 0o600))

	dirName := "00_Hasenaugengesicht"
	require.NoError(t, os.MkdirAll(filepath.Join(root, dirName), 0o750))

	svc, err := NewService([]Root{{Virtual: "/", Source: root}})
	require.NoError(t, err)

	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+url.PathEscape(dirName), nil)
	listRec := httptest.NewRecorder()
	e.ServeHTTP(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+url.PathEscape(fileName), nil)
	downloadRec := httptest.NewRecorder()
	e.ServeHTTP(downloadRec, downloadReq)
	require.Equal(t, http.StatusOK, downloadRec.Code)
	assert.Equal(t, fileContent, downloadRec.Body.Bytes())
}

func TestDirectoryListingAndDownload_PublicRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "report.txt"), []byte("report"), 0o600))

	svc, err := NewService([]Root{{Virtual: "/public", Source: root}})
	require.NoError(t, err)

	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/public", nil)
	listRec := httptest.NewRecorder()
	e.ServeHTTP(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/public/report.txt", nil)
	downloadRec := httptest.NewRecorder()
	e.ServeHTTP(downloadRec, downloadReq)
	require.Equal(t, http.StatusOK, downloadRec.Code)
	assert.Equal(t, "report", downloadRec.Body.String())
}

func TestNonExistingFileReturns404(t *testing.T) {
	root := t.TempDir()
	svc, err := NewService([]Root{{Virtual: "/public", Source: root}})
	require.NoError(t, err)

	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public/missing.txt", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDirectoryListingAndDownload_MultipleRoots(t *testing.T) {
	publicRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(publicRoot, "index.html"), []byte("<html></html>"), 0o600))

	docsRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(docsRoot, "readme.txt"), []byte("docs"), 0o600))

	roots := []Root{
		{Virtual: "/public", Source: publicRoot},
		{Virtual: "/Docs & Notes", Source: docsRoot},
	}
	svc, err := NewService(roots)
	require.NoError(t, err)

	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/Docs%20%26%20Notes", nil)
	listRec := httptest.NewRecorder()
	e.ServeHTTP(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/Docs%20%26%20Notes/readme.txt", nil)
	downloadRec := httptest.NewRecorder()
	e.ServeHTTP(downloadRec, downloadReq)
	require.Equal(t, http.StatusOK, downloadRec.Code)
	assert.Equal(t, "docs", downloadRec.Body.String())
}

func TestUnicodeFilenames(t *testing.T) {
	root := t.TempDir()
	// Create files with unicode names (Chinese, Korean, Arabic)
	require.NoError(t, os.WriteFile(filepath.Join(root, "文件.txt"), []byte("Chinese"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "파일.txt"), []byte("Korean"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ملف.txt"), []byte("Arabic"), 0o600))

	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	assert.Len(t, resp.Data, 3)
	// Verify all files are listed
	names := make([]string, 0, len(resp.Data))
	for _, r := range resp.Data {
		names = append(names, r.Attributes.Name)
	}
	assert.Contains(t, names, "文件.txt")
	assert.Contains(t, names, "파일.txt")
	assert.Contains(t, names, "ملف.txt")
}

func TestSpecialCharacterFilenames(t *testing.T) {
	root := t.TempDir()
	// Create files with special characters
	specialNames := []string{
		"file with spaces.txt",
		"file{with}braces.txt",
		"file(parens).txt",
		"file[brackets].txt",
		"file'quote'.txt",
	}

	for _, name := range specialNames {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("content"), 0o600))
	}

	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	assert.Len(t, resp.Data, len(specialNames))
}

func TestPagination(t *testing.T) {
	root := t.TempDir()
	// Create 10 files
	for i := 0; i < 10; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(root, fmt.Sprintf("file%02d.txt", i)), []byte("content"), 0o600))
	}

	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	// Test with limit=3
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public?page[limit]=3", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	assert.Len(t, resp.Data, 3)
	assert.Equal(t, 10, resp.Meta.TotalCount)
	assert.Equal(t, 0, resp.Meta.Offset)
	assert.Equal(t, 3, resp.Meta.Limit)
	assert.NotNil(t, resp.Links.Next)
	assert.Nil(t, resp.Links.Prev)

	// Test with offset=3
	req = httptest.NewRequest(http.MethodGet, "/api/v1/files/public?page[limit]=3&page[offset]=3", nil)
	rec = httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Len(t, resp.Data, 3)
	assert.Equal(t, 3, resp.Meta.Offset)
	assert.NotNil(t, resp.Links.Prev)
	assert.NotNil(t, resp.Links.Next)
}

func TestPaginationLimitExceeded(t *testing.T) {
	root := t.TempDir()
	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public?page[limit]=501", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSorting(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "zebra.txt"), []byte("z"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("a"), 0o600))

	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	// Test ascending sort (default)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public?sort=name", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "alpha.txt", resp.Data[0].Attributes.Name)
	assert.Equal(t, "zebra.txt", resp.Data[1].Attributes.Name)

	// Test descending sort
	req = httptest.NewRequest(http.MethodGet, "/api/v1/files/public?sort=-name", nil)
	rec = httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "zebra.txt", resp.Data[0].Attributes.Name)
	assert.Equal(t, "alpha.txt", resp.Data[1].Attributes.Name)
}

func TestSortingInvalidField(t *testing.T) {
	root := t.TempDir()
	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public?sort=invalid_field", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSortingMultiFieldRejected(t *testing.T) {
	root := t.TempDir()
	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public?sort=name,size_bytes", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDownloadAttachment(t *testing.T) {
	root := t.TempDir()
	content := []byte("download me")
	require.NoError(t, os.WriteFile(filepath.Join(root, "download.txt"), content, 0o600))

	svc := newTestService(t, root)
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/public/download.txt?download=1", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "download.txt")
}

func TestRootSlashSpecialCase(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("content"), 0o600))

	// Create service with virtual "/" (root slash)
	svc, err := NewService([]Root{{Virtual: "/", Source: root}})
	require.NoError(t, err)

	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	// GET /api/v1/files should return contents of root directly
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	// Should list the file directly, not a virtual folder
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "file.txt", resp.Data[0].Attributes.Name)
}

func TestListRootsWithMultipleRoots(t *testing.T) {
	root1 := t.TempDir()
	root2 := t.TempDir()

	svc, err := NewService([]Root{
		{Virtual: "/public", Source: root1},
		{Virtual: "/private", Source: root2},
	})
	require.NoError(t, err)

	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	// Should list virtual folders
	assert.Len(t, resp.Data, 2)
	names := []string{resp.Data[0].Attributes.Name, resp.Data[1].Attributes.Name}
	assert.Contains(t, names, "public")
	assert.Contains(t, names, "private")
}

func jsonAPIError(err error, c echo.Context) {
	var he *echo.HTTPError
	if !errors.As(err, &he) {
		he = echo.NewHTTPError(http.StatusInternalServerError, "unexpected error")
	}
	if !c.Response().Committed {
		_ = c.JSON(he.Code, map[string]interface{}{
			"errors": []map[string]string{
				{
					"status": http.StatusText(he.Code),
					"detail": fmt.Sprint(he.Message),
				},
			},
		})
	}
}
