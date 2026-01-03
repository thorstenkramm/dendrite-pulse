package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"golang.org/x/text/unicode/norm"

	jsonapi "github.com/thorstenkramm/dendrite-pulse/internal/api"
)

var errPayloadTooLarge = errors.New("payload too large")

const (
	headerETag        = "ETag"
	headerIfMatch     = "If-Match"
	headerIfNoneMatch = "If-None-Match"
	headerLastMod     = "Last-Modified"
)

func (h Handler) createFile(c echo.Context) error {
	root, rel, preconditions, err := h.parseUploadContext(c)
	if err != nil {
		return err
	}
	if preconditions.ifMatch != "" {
		return echo.NewHTTPError(http.StatusBadRequest, "If-Match is not supported for POST")
	}

	reader, err := requireMultipartReader(c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	folderRel, folderDesc, err := h.resolveUploadFolder(ctx, root, rel)
	if err != nil {
		return err
	}

	state := &createUploadState{
		root:       root,
		folderRel:  folderRel,
		folderDesc: folderDesc,
	}
	defer func() {
		if state.unlock != nil {
			state.unlock()
		}
	}()

	if err := processMultipartFile(reader, func(part *multipart.Part) error {
		return h.handleCreatePart(state, part, preconditions)
	}); err != nil {
		cleanupTemp(state.tempPath)
		return err
	}
	if state.tempPath == "" || state.relPath == "" {
		cleanupTemp(state.tempPath)
		return echo.NewHTTPError(http.StatusBadRequest, "file part required")
	}

	if err := os.Rename(state.tempPath, state.targetPath); err != nil {
		cleanupTemp(state.tempPath)
		return toHTTPError(fmt.Errorf("store upload: %w", err))
	}

	desc, err := h.svc.Describe(ctx, root.Virtual, state.relPath)
	if err != nil {
		return toHTTPError(err)
	}

	return writeUploadResponse(c, http.StatusCreated, desc, state.wroteBytes)
}

func (h Handler) putFile(c echo.Context) error {
	root, rel, preconditions, err := h.parseUploadContext(c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	target, err := h.resolvePutTarget(ctx, root, rel)
	if err != nil {
		return err
	}

	unlock := h.svc.locker.lock(target.targetPath)
	defer unlock()

	info, exists, err := statPath(target.targetPath)
	if err != nil {
		return toHTTPError(fmt.Errorf("stat target: %w", err))
	}
	if err := validatePutTarget(info, exists, preconditions); err != nil {
		return err
	}

	result, err := h.receivePutPayload(c, target.folderDesc.AbsolutePath, target.fileName)
	if err != nil {
		cleanupTemp(result.tempPath)
		return err
	}

	if err := os.Rename(result.tempPath, target.targetPath); err != nil {
		cleanupTemp(result.tempPath)
		return toHTTPError(fmt.Errorf("store upload: %w", err))
	}

	status := http.StatusOK
	if !exists {
		status = http.StatusCreated
	}

	desc, err := h.svc.Describe(ctx, root.Virtual, target.relPath)
	if err != nil {
		return toHTTPError(err)
	}

	return writeUploadResponse(c, status, desc, result.wroteBytes)
}

type createUploadState struct {
	root       Root
	folderRel  string
	folderDesc Descriptor
	relPath    string
	targetPath string
	tempPath   string
	wroteBytes int64
	unlock     func()
}

type putTarget struct {
	fileName   string
	relPath    string
	targetPath string
	folderDesc Descriptor
}

type uploadResult struct {
	tempPath   string
	wroteBytes int64
}

func (h Handler) handleCreatePart(state *createUploadState, part *multipart.Part, pre preconditions) error {
	normalized, err := normalizeFilename(part.FileName())
	if err != nil {
		return err
	}

	relPath := path.Join(state.folderRel, normalized)
	state.relPath = relPath
	state.targetPath = filepath.Join(state.root.Source, filepath.FromSlash(relPath))

	if state.unlock == nil {
		state.unlock = h.svc.locker.lock(state.targetPath)
	}

	info, exists, err := statPath(state.targetPath)
	if err != nil {
		return toHTTPError(fmt.Errorf("stat target: %w", err))
	}
	if err := validateCreateTarget(info, exists, pre); err != nil {
		return err
	}

	tempPath, wroteBytes, err := writeTempFile(
		state.folderDesc.AbsolutePath,
		part,
		h.svc.fileMode,
		h.svc.maxUploadBytes,
	)
	if err != nil {
		cleanupTemp(tempPath)
		return mapUploadError(err)
	}

	state.tempPath = tempPath
	state.wroteBytes = wroteBytes
	return nil
}

func (h Handler) parseUploadContext(c echo.Context) (Root, string, preconditions, error) {
	root, rel, err := parseVirtualPath(c, h.svc.Roots())
	if err != nil {
		return Root{}, "", preconditions{}, err
	}

	conds, err := parsePreconditions(c)
	if err != nil {
		return Root{}, "", preconditions{}, err
	}

	return root, rel, conds, nil
}

func (h Handler) resolvePutTarget(ctx context.Context, root Root, rel string) (putTarget, error) {
	folderRel, fileName := splitRelPath(rel)
	if fileName == "" {
		return putTarget{}, echo.NewHTTPError(http.StatusBadRequest, "file name required")
	}

	normalized, err := normalizeFilename(fileName)
	if err != nil {
		return putTarget{}, err
	}

	folderRel, folderDesc, err := h.resolveUploadFolder(ctx, root, folderRel)
	if err != nil {
		return putTarget{}, err
	}

	relPath := path.Join(folderRel, normalized)
	targetPath := filepath.Join(root.Source, filepath.FromSlash(relPath))

	return putTarget{
		fileName:   normalized,
		relPath:    relPath,
		targetPath: targetPath,
		folderDesc: folderDesc,
	}, nil
}

// resolveUploadFolder validates and resolves a folder path for uploads, preventing traversal.
func (h Handler) resolveUploadFolder(ctx context.Context, root Root, rel string) (string, Descriptor, error) {
	folderRel, err := cleanRelativePath(rel)
	if err != nil {
		return "", Descriptor{}, toHTTPError(err)
	}

	folderDesc, err := h.svc.Describe(ctx, root.Virtual, folderRel)
	if err != nil {
		return "", Descriptor{}, toHTTPError(err)
	}
	if folderDesc.TargetKind != kindFolder {
		return "", Descriptor{}, echo.NewHTTPError(http.StatusNotFound, "target folder not found")
	}

	return folderRel, folderDesc, nil
}

func (h Handler) receivePutPayload(c echo.Context, folderAbs, expectedName string) (uploadResult, error) {
	if isMultipartRequest(c.Request().Header.Get(echo.HeaderContentType)) {
		reader, err := multipartReader(c)
		if err != nil {
			return uploadResult{}, err
		}

		var result uploadResult
		err = processMultipartFile(reader, func(part *multipart.Part) error {
			normalized, err := normalizeFilename(part.FileName())
			if err != nil {
				return err
			}
			if normalized != expectedName {
				return echo.NewHTTPError(http.StatusBadRequest, "multipart filename does not match URL")
			}

			tempPath, wroteBytes, err := writeTempFile(
				folderAbs,
				part,
				h.svc.fileMode,
				h.svc.maxUploadBytes,
			)
			if err != nil {
				cleanupTemp(tempPath)
				return mapUploadError(err)
			}

			result = uploadResult{tempPath: tempPath, wroteBytes: wroteBytes}
			return nil
		})
		if err != nil {
			cleanupTemp(result.tempPath)
			return uploadResult{}, err
		}

		return result, nil
	}

	if c.Request().ContentLength > h.svc.maxUploadBytes {
		return uploadResult{}, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "payload too large")
	}

	tempPath, wroteBytes, err := writeTempFile(
		folderAbs,
		c.Request().Body,
		h.svc.fileMode,
		h.svc.maxUploadBytes,
	)
	if err != nil {
		cleanupTemp(tempPath)
		return uploadResult{}, mapUploadError(err)
	}

	return uploadResult{tempPath: tempPath, wroteBytes: wroteBytes}, nil
}

func requireMultipartReader(c echo.Context) (*multipart.Reader, error) {
	if !isMultipartRequest(c.Request().Header.Get(echo.HeaderContentType)) {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "multipart/form-data required")
	}
	return multipartReader(c)
}

func multipartReader(c echo.Context) (*multipart.Reader, error) {
	reader, err := c.Request().MultipartReader()
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid multipart body")
	}
	return reader, nil
}

func processMultipartFile(reader *multipart.Reader, handle func(part *multipart.Part) error) error {
	var found bool
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid multipart body")
		}
		if part.FileName() == "" {
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}
		if found {
			_ = part.Close()
			return echo.NewHTTPError(http.StatusBadRequest, "multiple file parts are not allowed")
		}
		found = true

		if err := handle(part); err != nil {
			_ = part.Close()
			return err
		}
		_ = part.Close()
	}

	if !found {
		return echo.NewHTTPError(http.StatusBadRequest, "file part required")
	}
	return nil
}

func validateCreateTarget(info os.FileInfo, exists bool, pre preconditions) error {
	if !exists {
		return nil
	}
	if !info.Mode().IsRegular() {
		return conflictOrPrecondition(pre.ifNoneMatch, "target already exists")
	}
	return conflictOrPrecondition(pre.ifNoneMatch, "file already exists")
}

func validatePutTarget(info os.FileInfo, exists bool, pre preconditions) error {
	if exists && !info.Mode().IsRegular() {
		return conflictOrPrecondition(pre.ifNoneMatch || pre.ifMatch != "", "target already exists")
	}

	if pre.ifNoneMatch && exists {
		return echo.NewHTTPError(http.StatusPreconditionFailed, "file already exists")
	}

	if pre.ifMatch != "" {
		if !exists {
			return echo.NewHTTPError(http.StatusPreconditionFailed, "file does not exist")
		}
		etag, err := etagForInfo(info)
		if err != nil {
			return toHTTPError(fmt.Errorf("stat target: %w", err))
		}
		if etag != pre.ifMatch {
			return echo.NewHTTPError(http.StatusPreconditionFailed, "etag does not match")
		}
	}

	return nil
}

func conflictOrPrecondition(hasPrecondition bool, message string) error {
	if hasPrecondition {
		return echo.NewHTTPError(http.StatusPreconditionFailed, message)
	}
	return echo.NewHTTPError(http.StatusConflict, message)
}

func writeUploadResponse(c echo.Context, status int, desc Descriptor, sizeBytes int64) error {
	if desc.Metadata.SizeBytes == nil {
		desc.Metadata.SizeBytes = &sizeBytes
	}
	resp := jsonapi.SingleResponse[Resource]{Data: resourceFrom(desc)}
	if desc.Metadata.ETag != "" {
		c.Response().Header().Set(headerETag, desc.Metadata.ETag)
	}
	if desc.Metadata.ModifiedAt != nil {
		c.Response().Header().Set(headerLastMod, desc.Metadata.ModifiedAt.UTC().Format(http.TimeFormat))
	}
	c.Response().Header().Set(echo.HeaderContentType, jsonapi.ContentType)

	if err := c.JSON(status, resp); err != nil {
		return fmt.Errorf("write upload response: %w", err)
	}
	return nil
}

func isMultipartRequest(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && strings.EqualFold(mediaType, "multipart/form-data")
}

// normalizeFilename applies NFKC normalization and rejects path separators or traversal tokens.
func normalizeFilename(name string) (string, error) {
	normalized := norm.NFKC.String(name)
	if normalized == "" || normalized == "." || normalized == ".." {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid filename")
	}
	if strings.Contains(normalized, "/") || strings.Contains(normalized, "\\") {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid filename")
	}
	return normalized, nil
}

func splitRelPath(rel string) (string, string) {
	if rel == "" {
		return "", ""
	}
	dir := path.Dir(rel)
	if dir == "." {
		dir = ""
	}
	return dir, path.Base(rel)
}

type preconditions struct {
	ifMatch     string
	ifNoneMatch bool
}

func parsePreconditions(c echo.Context) (preconditions, error) {
	var out preconditions

	ifMatch := strings.TrimSpace(c.Request().Header.Get(headerIfMatch))
	if ifMatch != "" {
		if strings.Contains(ifMatch, ",") {
			return out, echo.NewHTTPError(http.StatusBadRequest, "multiple If-Match values are not supported")
		}
		if ifMatch == "*" {
			return out, echo.NewHTTPError(http.StatusBadRequest, "If-Match wildcard is not supported")
		}
		out.ifMatch = ifMatch
	}

	ifNoneMatch := strings.TrimSpace(c.Request().Header.Get(headerIfNoneMatch))
	if ifNoneMatch != "" {
		if ifNoneMatch != "*" {
			return out, echo.NewHTTPError(http.StatusBadRequest, "If-None-Match must be '*'")
		}
		out.ifNoneMatch = true
	}

	if out.ifMatch != "" && out.ifNoneMatch {
		return out, echo.NewHTTPError(http.StatusBadRequest, "If-Match and If-None-Match cannot be combined")
	}

	return out, nil
}

func statPath(path string) (os.FileInfo, bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("stat path: %w", err)
}

func etagForInfo(info os.FileInfo) (string, error) {
	if info == nil {
		return "", fmt.Errorf("missing file info")
	}
	return weakETag(info), nil
}

func writeTempFile(dir string, reader io.Reader, mode os.FileMode, maxBytes int64) (string, int64, error) {
	// #nosec G304 -- dir is derived from validated file roots.
	temp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return "", 0, fmt.Errorf("create temp file: %w", err)
	}

	tempPath := temp.Name()
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return tempPath, 0, fmt.Errorf("set temp file mode: %w", err)
	}

	written, err := copyWithLimit(temp, reader, maxBytes)
	if err != nil {
		_ = temp.Close()
		return tempPath, written, err
	}

	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return tempPath, written, fmt.Errorf("sync temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return tempPath, written, fmt.Errorf("close temp file: %w", err)
	}

	return tempPath, written, nil
}

func cleanupTemp(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

func copyWithLimit(dst io.Writer, src io.Reader, maxBytes int64) (int64, error) {
	limited := &io.LimitedReader{R: src, N: maxBytes + 1}
	written, err := io.Copy(dst, limited)
	if err != nil {
		return written, fmt.Errorf("stream upload: %w", err)
	}
	if written > maxBytes {
		return written, errPayloadTooLarge
	}
	return written, nil
}

func mapUploadError(err error) error {
	if errors.Is(err, errPayloadTooLarge) {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "payload too large")
	}
	return toHTTPError(err)
}
