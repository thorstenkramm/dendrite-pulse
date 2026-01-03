package files

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	jsonapi "github.com/thorstenkramm/dendrite-pulse/internal/api"
)

func TestUploadCreateFilePost(t *testing.T) {
	root := t.TempDir()
	svc := newUploadService(t, root, 1024)
	e := newUploadEcho(svc)

	body, contentType := buildMultipartBody(t, "report.txt", []byte("hello"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/public", body)
	req.Header.Set(echo.HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.NotEmpty(t, rec.Header().Get(headerETag))
	assert.NotEmpty(t, rec.Header().Get(headerLastMod))

	var resp jsonapi.SingleResponse[Resource]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "/public/report.txt", resp.Data.Attributes.VirtualPath)
	assert.Equal(t, "report.txt", resp.Data.Attributes.Name)
	require.NotNil(t, resp.Data.Attributes.SizeBytes)
	assert.Equal(t, int64(5), *resp.Data.Attributes.SizeBytes)

	// #nosec G304 -- reading from test temp directory.
	data, err := os.ReadFile(filepath.Join(root, "report.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

func TestUploadPostConflictAndPrecondition(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "exists.txt"), []byte("old"), 0o600))
	svc := newUploadService(t, root, 1024)
	e := newUploadEcho(svc)

	tests := []struct {
		name       string
		headerName string
		headerVal  string
		wantStatus int
	}{
		{"conflict without precondition", "", "", http.StatusConflict},
		{"precondition failed", headerIfNoneMatch, "*", http.StatusPreconditionFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, contentType := buildMultipartBody(t, "exists.txt", []byte("new"))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/files/public", body)
			req.Header.Set(echo.HeaderContentType, contentType)
			if tt.headerName != "" {
				req.Header.Set(tt.headerName, tt.headerVal)
			}
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestUploadPutRawCreateAndReplace(t *testing.T) {
	root := t.TempDir()
	svc := newUploadService(t, root, 1024)
	e := newUploadEcho(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/files/public/item.txt", bytes.NewBufferString("first"))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	etag := rec.Header().Get(headerETag)
	require.NotEmpty(t, etag)

	req = httptest.NewRequest(http.MethodPut, "/api/v1/files/public/item.txt", bytes.NewBufferString("second"))
	req.Header.Set(headerIfMatch, etag)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// #nosec G304 -- reading from test temp directory.
	data, err := os.ReadFile(filepath.Join(root, "item.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("second"), data)
}

func TestUploadPutIfMatchMismatch(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "item.txt"), []byte("old"), 0o600))
	svc := newUploadService(t, root, 1024)
	e := newUploadEcho(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/files/public/item.txt", bytes.NewBufferString("new"))
	req.Header.Set(headerIfMatch, `W/"does-not-match"`)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusPreconditionFailed, rec.Code)
}

func TestUploadPutMultipartFilenameMismatch(t *testing.T) {
	root := t.TempDir()
	svc := newUploadService(t, root, 1024)
	e := newUploadEcho(svc)

	body, contentType := buildMultipartBody(t, "wrong.txt", []byte("data"))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/files/public/right.txt", body)
	req.Header.Set(echo.HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUploadMissingFolder(t *testing.T) {
	root := t.TempDir()
	svc := newUploadService(t, root, 1024)
	e := newUploadEcho(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/files/public/missing/file.txt", bytes.NewBufferString("data"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestUploadSizeLimit(t *testing.T) {
	root := t.TempDir()
	svc := newUploadService(t, root, 5)
	e := newUploadEcho(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/files/public/limit.txt", bytes.NewBufferString("123456"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestUploadInvalidPaths(t *testing.T) {
	root := t.TempDir()
	svc := newUploadService(t, root, 1024)
	e := newUploadEcho(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/files/public%2Fsecret/file.txt", bytes.NewBufferString("data"))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPut, "/api/v1/files/public%5Csecret/file.txt", bytes.NewBufferString("data"))
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUploadConcurrentWrites(t *testing.T) {
	root := t.TempDir()
	svc := newUploadService(t, root, 1024)
	e := newUploadEcho(svc)
	server := httptest.NewServer(e)
	t.Cleanup(server.Close)

	contents := [][]byte{
		[]byte("alpha"),
		[]byte("beta"),
		[]byte("gamma"),
		[]byte("delta"),
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(contents))
	for _, payload := range contents {
		wg.Add(1)
		go func(body []byte) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodPut, server.URL+"/api/v1/files/public/concurrent.txt", bytes.NewReader(body))
			if err != nil {
				errs <- err
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errs <- err
				return
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				errs <- fmt.Errorf("unexpected status %d", resp.StatusCode)
				return
			}
		}(payload)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	// #nosec G304 -- reading from test temp directory.
	data, err := os.ReadFile(filepath.Join(root, "concurrent.txt"))
	require.NoError(t, err)
	assert.Contains(t, contents, data)
}

func TestUploadPutIfMatchNonExistent(t *testing.T) {
	root := t.TempDir()
	svc := newUploadService(t, root, 1024)
	e := newUploadEcho(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/files/public/missing.txt", bytes.NewBufferString("data"))
	req.Header.Set(headerIfMatch, `W/"any-etag"`)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusPreconditionFailed, rec.Code)
}

func TestUploadPostRejectsIfMatch(t *testing.T) {
	root := t.TempDir()
	svc := newUploadService(t, root, 1024)
	e := newUploadEcho(svc)

	body, contentType := buildMultipartBody(t, "new.txt", []byte("data"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/public", body)
	req.Header.Set(echo.HeaderContentType, contentType)
	req.Header.Set(headerIfMatch, `W/"some-etag"`)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUploadFileMode(t *testing.T) {
	root := t.TempDir()
	svc, err := NewService([]Root{{Virtual: "/public", Source: root}}, Options{
		MaxUploadBytes: 1024,
		FileMode:       0o640,
	})
	require.NoError(t, err)
	e := newUploadEcho(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/files/public/mode.txt", bytes.NewBufferString("test"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	info, err := os.Stat(filepath.Join(root, "mode.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

func TestUploadContentLengthExceedsLimit(t *testing.T) {
	root := t.TempDir()
	svc := newUploadService(t, root, 10)
	e := newUploadEcho(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/files/public/big.txt", bytes.NewBufferString("small"))
	req.ContentLength = 1000
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func newUploadService(t *testing.T, root string, maxUpload int64) *Service {
	t.Helper()

	svc, err := NewService([]Root{{Virtual: "/public", Source: root}}, Options{
		MaxUploadBytes: maxUpload,
		FileMode:       0o600,
	})
	require.NoError(t, err)
	return svc
}

func newUploadEcho(svc *Service) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = jsonAPIError
	RegisterRoutes(e, svc)
	return e
}

func buildMultipartBody(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body, writer.FormDataContentType()
}
