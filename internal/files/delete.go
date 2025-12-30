package files

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/labstack/echo/v4"
)

func (h Handler) deleteResource(c echo.Context) error {
	root, rel, err := parseVirtualPath(c, h.svc.Roots())
	if err != nil {
		return err
	}

	relClean, err := cleanRelativePath(rel)
	if err != nil {
		return toHTTPError(err)
	}
	if relClean == "" {
		return echo.NewHTTPError(http.StatusForbidden, "virtual root cannot be deleted")
	}

	relClean, err = normalizeDeletePath(relClean)
	if err != nil {
		return err
	}

	recursive, err := parseRecursiveParam(c)
	if err != nil {
		return err
	}

	ifMatch, err := parseMoveIfMatch(c)
	if err != nil {
		return err
	}

	absPath := filepath.Join(root.Source, filepath.FromSlash(relClean))
	unlock := h.svc.locker.lock(absPath)
	defer unlock()

	info, err := os.Lstat(absPath)
	if err != nil {
		return mapDeleteError(err, absPath)
	}

	if err := validateMoveIfMatch(info, ifMatch); err != nil {
		return err
	}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		if err := os.Remove(absPath); err != nil {
			return mapDeleteError(err, absPath)
		}
	case info.Mode().IsRegular():
		if err := os.Remove(absPath); err != nil {
			return mapDeleteError(err, absPath)
		}
	case info.IsDir():
		if err := deleteFolder(absPath, recursive); err != nil {
			return err
		}
	default:
		return echo.NewHTTPError(http.StatusConflict, "unsupported resource type")
	}

	return writeNoContent(c, http.StatusNoContent)
}

func normalizeDeletePath(rel string) (string, error) {
	folderRel, name := splitRelPath(rel)
	if name == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid path")
	}

	normalized, err := normalizeFilename(name)
	if err != nil {
		return "", err
	}

	if folderRel == "" {
		return normalized, nil
	}
	return path.Join(folderRel, normalized), nil
}

func parseRecursiveParam(c echo.Context) (bool, error) {
	raw := strings.TrimSpace(c.QueryParam("recursive"))
	if raw == "" {
		return false, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, echo.NewHTTPError(http.StatusBadRequest, "invalid recursive parameter")
	}

	return value, nil
}

func deleteFolder(absPath string, recursive bool) error {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return mapDeleteError(err, absPath)
	}

	if len(entries) == 0 {
		if err := os.Remove(absPath); err != nil {
			return mapDeleteError(err, absPath)
		}
		return nil
	}

	if !recursive {
		return echo.NewHTTPError(http.StatusConflict, "folder is not empty")
	}

	if err := os.RemoveAll(absPath); err != nil {
		slog.Error("recursive delete failed", "path", absPath, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "delete failed")
	}

	return nil
}

func mapDeleteError(err error, path string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
		return echo.NewHTTPError(http.StatusConflict, "folder is not empty")
	}
	if mapped := toHTTPError(err); mapped != nil {
		var httpErr *echo.HTTPError
		if errors.As(mapped, &httpErr) {
			return httpErr
		}
	}

	slog.Error("delete failed", "path", path, "error", err)
	return echo.NewHTTPError(http.StatusInternalServerError, "delete failed")
}
