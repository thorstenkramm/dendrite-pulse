package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/labstack/echo/v4"
)

const (
	headerDestination = "Destination"
	headerOverwrite   = "Overwrite"
)

type moveRequest struct {
	destination string
	overwrite   bool
}

type moveRequestBody struct {
	Op        string `json:"op"`
	To        string `json:"to"`
	Overwrite *bool  `json:"overwrite,omitempty"`
}

type moveDestination struct {
	root       Root
	rel        string
	absPath    string
	folderDesc Descriptor
}

type moveContext struct {
	srcDesc   Descriptor
	srcPath   string
	dest      moveDestination
	ifMatch   string
	overwrite bool
}

func (h Handler) moveFile(c echo.Context) error {
	req, err := parseMoveHeaders(c)
	if err != nil {
		return err
	}
	return h.handleMove(c, req)
}

func (h Handler) patchFile(c echo.Context) error {
	req, err := parseMovePatchRequest(c)
	if err != nil {
		return err
	}
	return h.handleMove(c, req)
}

func (h Handler) handleMove(c echo.Context, req moveRequest) error {
	ctx, err := h.buildMoveContext(c, req)
	if err != nil {
		return err
	}
	return h.executeMove(c, ctx)
}

func (h Handler) buildMoveContext(c echo.Context, req moveRequest) (moveContext, error) {
	root, rel, err := parseVirtualPath(c, h.svc.Roots())
	if err != nil {
		return moveContext{}, err
	}
	if rel == "" {
		return moveContext{}, echo.NewHTTPError(http.StatusForbidden, "virtual root cannot be moved")
	}

	ifMatch, err := parseMoveIfMatch(c)
	if err != nil {
		return moveContext{}, err
	}

	destRoot, destRel, err := parseDestinationPath(req.destination, h.svc.Roots())
	if err != nil {
		return moveContext{}, err
	}
	if destRel == "" {
		return moveContext{}, echo.NewHTTPError(http.StatusForbidden, "destination cannot be virtual root")
	}

	ctx := c.Request().Context()
	srcDesc, err := h.svc.Describe(ctx, root.Virtual, rel)
	if err != nil {
		return moveContext{}, toHTTPError(err)
	}
	if srcDesc.Kind != kindFile {
		return moveContext{}, echo.NewHTTPError(http.StatusConflict, "only files can be moved")
	}

	dest, err := h.resolveMoveDestination(ctx, destRoot, destRel)
	if err != nil {
		return moveContext{}, err
	}
	if srcDesc.Root.Virtual == dest.root.Virtual && srcDesc.RelPath == dest.rel {
		return moveContext{}, echo.NewHTTPError(http.StatusConflict, "source and destination are the same")
	}

	srcPath := srcDesc.LinkPath
	if srcPath == "" {
		srcPath = srcDesc.AbsolutePath
	}

	return moveContext{
		srcDesc:   srcDesc,
		srcPath:   srcPath,
		dest:      dest,
		ifMatch:   ifMatch,
		overwrite: req.overwrite,
	}, nil
}

func (h Handler) executeMove(c echo.Context, ctx moveContext) error {
	unlock := lockPaths(h.svc.locker, ctx.srcPath, ctx.dest.absPath)
	defer unlock()

	srcInfo, err := os.Stat(ctx.srcPath)
	if err != nil {
		return toHTTPError(fmt.Errorf("stat source: %w", err))
	}
	if !srcInfo.Mode().IsRegular() {
		return echo.NewHTTPError(http.StatusConflict, "only files can be moved")
	}

	if err := validateMoveIfMatch(srcInfo, ctx.ifMatch); err != nil {
		return err
	}

	destInfo, destExists, err := statPath(ctx.dest.absPath)
	if err != nil {
		return toHTTPError(fmt.Errorf("stat destination: %w", err))
	}

	status, err := resolveMoveStatus(destInfo, destExists, ctx.overwrite)
	if err != nil {
		return err
	}

	if err := moveFileAtomic(ctx.srcPath, ctx.dest.absPath, ctx.dest.folderDesc.AbsolutePath, srcInfo); err != nil {
		return err
	}

	if status == http.StatusNoContent {
		return writeNoContent(c, status)
	}

	destDesc, err := h.svc.Describe(c.Request().Context(), ctx.dest.root.Virtual, ctx.dest.rel)
	if err != nil {
		return toHTTPError(err)
	}

	return writeUploadResponse(c, status, destDesc, srcInfo.Size())
}

func parseMoveHeaders(c echo.Context) (moveRequest, error) {
	dest := strings.TrimSpace(c.Request().Header.Get(headerDestination))
	if dest == "" {
		return moveRequest{}, echo.NewHTTPError(http.StatusBadRequest, "Destination header is required")
	}

	overwrite, err := parseOverwriteHeader(c.Request().Header.Get(headerOverwrite))
	if err != nil {
		return moveRequest{}, err
	}

	return moveRequest{destination: dest, overwrite: overwrite}, nil
}

func parseMovePatchRequest(c echo.Context) (moveRequest, error) {
	if !isJSONRequest(c.Request().Header.Get(echo.HeaderContentType)) {
		return moveRequest{}, echo.NewHTTPError(http.StatusBadRequest, "application/json required")
	}

	var body moveRequestBody
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return moveRequest{}, echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}

	if strings.TrimSpace(body.Op) != "move" {
		return moveRequest{}, echo.NewHTTPError(http.StatusBadRequest, "unsupported operation")
	}

	dest := strings.TrimSpace(body.To)
	if dest == "" {
		return moveRequest{}, echo.NewHTTPError(http.StatusBadRequest, "destination is required")
	}

	overwrite := false
	if body.Overwrite != nil {
		overwrite = *body.Overwrite
	}

	return moveRequest{destination: dest, overwrite: overwrite}, nil
}

func parseDestinationPath(raw string, roots []Root) (Root, string, error) {
	if strings.Contains(raw, "://") || strings.HasPrefix(raw, "//") {
		return Root{}, "", echo.NewHTTPError(http.StatusBadRequest, "destination must be API-relative")
	}
	if strings.Contains(raw, "?") || strings.Contains(raw, "#") {
		return Root{}, "", echo.NewHTTPError(http.StatusBadRequest, "destination must not include query or fragment")
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Root{}, "", echo.NewHTTPError(http.StatusBadRequest, "destination is required")
	}

	return parseVirtualPathRaw(raw, roots)
}

func (h Handler) resolveMoveDestination(ctx context.Context, root Root, rel string) (moveDestination, error) {
	folderRel, fileName := splitRelPath(rel)
	if fileName == "" {
		return moveDestination{}, echo.NewHTTPError(http.StatusBadRequest, "destination file name required")
	}

	normalized, err := normalizeFilename(fileName)
	if err != nil {
		return moveDestination{}, err
	}

	folderRel, folderDesc, err := h.resolveUploadFolder(ctx, root, folderRel)
	if err != nil {
		return moveDestination{}, err
	}

	destRel := path.Join(folderRel, normalized)
	destAbs := filepath.Join(root.Source, filepath.FromSlash(destRel))

	return moveDestination{
		root:       root,
		rel:        destRel,
		absPath:    destAbs,
		folderDesc: folderDesc,
	}, nil
}

func parseMoveIfMatch(c echo.Context) (string, error) {
	ifNoneMatch := strings.TrimSpace(c.Request().Header.Get(headerIfNoneMatch))
	if ifNoneMatch != "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "If-None-Match is not supported")
	}

	ifMatch := strings.TrimSpace(c.Request().Header.Get(headerIfMatch))
	if ifMatch == "" {
		return "", nil
	}
	if strings.Contains(ifMatch, ",") {
		return "", echo.NewHTTPError(http.StatusBadRequest, "multiple If-Match values are not supported")
	}
	if ifMatch == "*" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "If-Match wildcard is not supported")
	}

	return ifMatch, nil
}

func parseOverwriteHeader(value string) (bool, error) {
	if strings.TrimSpace(value) == "" {
		return false, nil
	}
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "T":
		return true, nil
	case "F":
		return false, nil
	default:
		return false, echo.NewHTTPError(http.StatusBadRequest, "Overwrite must be 'T' or 'F'")
	}
}

func validateMoveIfMatch(info os.FileInfo, ifMatch string) error {
	if ifMatch == "" {
		return nil
	}
	etag, err := etagForInfo(info)
	if err != nil {
		return toHTTPError(fmt.Errorf("stat source: %w", err))
	}
	if etag != ifMatch {
		return echo.NewHTTPError(http.StatusPreconditionFailed, "etag does not match")
	}
	return nil
}

func resolveMoveStatus(info os.FileInfo, exists bool, overwrite bool) (int, error) {
	if !exists {
		return http.StatusCreated, nil
	}
	if !info.Mode().IsRegular() {
		return 0, echo.NewHTTPError(http.StatusConflict, "destination is a directory or symlink")
	}
	if !overwrite {
		return 0, echo.NewHTTPError(http.StatusConflict, "destination already exists")
	}
	return http.StatusNoContent, nil
}

func writeNoContent(c echo.Context, status int) error {
	if err := c.NoContent(status); err != nil {
		return fmt.Errorf("write move response: %w", err)
	}
	return nil
}

func isJSONRequest(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

// lockPaths acquires locks for multiple paths in sorted order to prevent deadlocks.
// Returns an unlock function that releases locks in reverse order.
func lockPaths(locker *pathLocker, paths ...string) func() {
	unique := make(map[string]struct{}, len(paths))
	ordered := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, ok := unique[p]; ok {
			continue
		}
		unique[p] = struct{}{}
		ordered = append(ordered, p)
	}

	sort.Strings(ordered)

	unlocks := make([]func(), 0, len(ordered))
	for _, p := range ordered {
		unlocks = append(unlocks, locker.lock(p))
	}

	return func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
}

func moveFileAtomic(srcPath, destPath, destDir string, srcInfo os.FileInfo) error {
	if err := os.Rename(srcPath, destPath); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return toHTTPError(fmt.Errorf("rename file: %w", err))
	}

	if err := copyAcrossFilesystems(srcPath, destPath, destDir, srcInfo); err != nil {
		slog.Error(
			"cross-filesystem move failed",
			"source", srcPath,
			"destination", destPath,
			"error", err,
		)
		return echo.NewHTTPError(http.StatusInternalServerError, "cross-filesystem move failed")
	}

	return nil
}

func copyAcrossFilesystems(srcPath, destPath, destDir string, srcInfo os.FileInfo) error {
	temp, err := os.CreateTemp(destDir, ".move-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			cleanupTemp(tempPath)
		}
	}()

	if err := temp.Chmod(srcInfo.Mode().Perm()); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set temp file mode: %w", err)
	}

	// #nosec G304 -- srcPath is derived from validated file roots.
	srcFile, err := os.Open(srcPath)
	if err != nil {
		_ = temp.Close()
		return fmt.Errorf("open source file: %w", err)
	}
	defer func() {
		_ = srcFile.Close()
	}()

	if _, err := io.Copy(temp, srcFile); err != nil {
		_ = temp.Close()
		return fmt.Errorf("copy source file: %w", err)
	}

	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	removeTemp = false

	if err := os.Remove(srcPath); err != nil {
		return fmt.Errorf("remove source file: %w", err)
	}

	return nil
}
