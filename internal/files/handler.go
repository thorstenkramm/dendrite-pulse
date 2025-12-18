package files

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/thorstenkramm/dendrite-pulse/internal/api"
)

const (
	defaultLimit = 200
	maxLimit     = 500
)

// RegisterRoutes wires file handlers.
func RegisterRoutes(e *echo.Echo, svc *Service) {
	h := Handler{svc: svc}

	files := e.Group("/api/v1/files")
	files.GET("", h.listRoots)
	files.GET("/*", h.getResource)
}

// Handler serves file and directory requests.
type Handler struct {
	svc *Service
}

func (h Handler) listRoots(c echo.Context) error {
	params, err := parseListParams(c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()

	// Special case: if there's a single root with virtual "/", list its contents directly
	if h.svc.HasSingleRootSlash() {
		entries, err := h.svc.ListDirectory(ctx, "/", "")
		if err != nil {
			return toHTTPError(err)
		}
		return sendCollectionJSON(c, entries, params)
	}

	roots, err := h.svc.ListRoots(ctx)
	if err != nil {
		return toHTTPError(err)
	}

	return sendCollectionJSON(c, roots, params)
}

func (h Handler) getResource(c echo.Context) error {
	root, rel, err := parseVirtualPath(c, h.svc.Roots())
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	desc, err := h.svc.Describe(ctx, root.Virtual, rel)
	if err != nil {
		return toHTTPError(err)
	}

	if desc.TargetKind == "folder" {
		params, err := parseListParams(c)
		if err != nil {
			return err
		}

		entries, err := h.svc.ListDirectory(ctx, root.Virtual, rel)
		if err != nil {
			return toHTTPError(err)
		}

		return sendCollectionJSON(c, entries, params)
	}

	return h.serveFile(c, desc)
}

func sendCollectionJSON(c echo.Context, entries []Descriptor, params ListParams) error {
	sortDescriptors(entries, params.SortField, params.Descending)
	resp := collectionResponse(c, entries, params)
	c.Response().Header().Set(echo.HeaderContentType, api.ContentType)
	if err := c.JSON(http.StatusOK, resp); err != nil {
		return fmt.Errorf("write collection response: %w", err)
	}
	return nil
}

func (h Handler) serveFile(c echo.Context, desc Descriptor) error {
	// Check for download=1 query param to force attachment download
	if c.QueryParam("download") == "1" {
		if err := c.Attachment(desc.AbsolutePath, desc.Metadata.Name); err != nil {
			return fmt.Errorf("serve attachment: %w", err)
		}
		return nil
	}

	ctype := desc.Metadata.MimeType
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	c.Response().Header().Set(echo.HeaderContentType, ctype)

	http.ServeFile(c.Response(), c.Request(), desc.AbsolutePath)
	return nil
}

func parseVirtualPath(c echo.Context, roots []Root) (Root, string, error) {
	raw := c.Request().URL.RawPath
	if raw == "" {
		raw = c.Request().URL.Path
	}

	const prefix = "/api/v1/files"
	if !strings.HasPrefix(raw, prefix) {
		return Root{}, "", echo.NewHTTPError(http.StatusBadRequest, "invalid path")
	}

	rest := strings.TrimPrefix(raw, prefix)

	if rest == "" {
		return Root{}, "", echo.NewHTTPError(http.StatusNotFound, "file path required")
	}

	if strings.HasSuffix(rest, "/") {
		return Root{}, "", echo.NewHTTPError(http.StatusNotFound, "trailing slash is not allowed")
	}

	rest = strings.TrimPrefix(rest, "/")

	decoded, err := url.PathUnescape(rest)
	if err != nil {
		return Root{}, "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid path: %v", err))
	}

	pathWithSlash := "/" + decoded

	root, rel, ok := matchRoot(pathWithSlash, roots)
	if !ok {
		return Root{}, "", echo.NewHTTPError(http.StatusNotFound, "file root not found")
	}

	return root, rel, nil
}

func matchRoot(requestPath string, roots []Root) (Root, string, bool) {
	sorted := make([]Root, len(roots))
	copy(sorted, roots)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Virtual) > len(sorted[j].Virtual)
	})

	for _, root := range sorted {
		if root.Virtual == "/" {
			rel := strings.TrimPrefix(requestPath, "/")
			return root, rel, true
		}

		if requestPath == root.Virtual {
			return root, "", true
		}
		prefix := root.Virtual + "/"
		if strings.HasPrefix(requestPath, prefix) {
			rel := strings.TrimPrefix(requestPath, prefix)
			return root, rel, true
		}
	}

	return Root{}, "", false
}

func collectionResponse(c echo.Context, entries []Descriptor, params ListParams) Response {
	total := len(entries)

	// Apply pagination
	start := params.Offset
	if start > total {
		start = total
	}
	end := start + params.Limit
	if end > total {
		end = total
	}

	paged := entries[start:end]
	data := make([]Resource, 0, len(paged))
	for _, entry := range paged {
		data = append(data, resourceFrom(entry))
	}

	// Build pagination links
	basePath := c.Request().URL.Path
	links := buildPaginationLinks(basePath, params, total)

	return Response{
		Meta: &PaginationMeta{
			TotalCount: total,
			Offset:     params.Offset,
			Limit:      params.Limit,
		},
		Data:  data,
		Links: links,
	}
}

func buildPaginationLinks(basePath string, params ListParams, total int) *PaginationLinks {
	buildURL := func(offset int) string {
		u := fmt.Sprintf("%s?page[offset]=%d&page[limit]=%d", basePath, offset, params.Limit)
		if params.SortField != "name" || params.Descending {
			sortPrefix := ""
			if params.Descending {
				sortPrefix = "-"
			}
			u += fmt.Sprintf("&sort=%s%s", sortPrefix, params.SortField)
		}
		return u
	}

	// Calculate last page offset
	lastOffset := 0
	if total > 0 {
		lastOffset = ((total - 1) / params.Limit) * params.Limit
	}

	links := &PaginationLinks{
		Self:  buildURL(params.Offset),
		First: buildURL(0),
		Last:  buildURL(lastOffset),
	}

	// Previous link
	if params.Offset > 0 {
		prevOffset := params.Offset - params.Limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		prev := buildURL(prevOffset)
		links.Prev = &prev
	}

	// Next link
	if params.Offset+params.Limit < total {
		next := buildURL(params.Offset + params.Limit)
		links.Next = &next
	}

	return links
}

func resourceFrom(desc Descriptor) Resource {
	attrs := Attributes{
		Name:           desc.Metadata.Name,
		ResourceKind:   desc.Metadata.ResourceKind,
		SizeBytes:      desc.Metadata.SizeBytes,
		PermissionMode: desc.Metadata.PermissionMode,
		User:           desc.Metadata.User,
		Group:          desc.Metadata.Group,
		UserID:         desc.Metadata.UserID,
		GroupID:        desc.Metadata.GroupID,
		MimeType:       desc.Metadata.MimeType,
		AccessedAt:     formatTime(desc.Metadata.AccessedAt),
		ModifiedAt:     formatTime(desc.Metadata.ModifiedAt),
		ChangedAt:      formatTime(desc.Metadata.ChangedAt),
		BornAt:         formatTime(desc.Metadata.BornAt),
	}

	return Resource{
		ID:         desc.Metadata.VirtualPath,
		Type:       "files",
		Attributes: attrs,
		Links: ResourceLinks{
			Self: path.Join("/api/v1/files", desc.Metadata.VirtualPath),
		},
	}
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func toHTTPError(err error) error {
	var httpErr *echo.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr
	}

	switch {
	case errors.Is(err, ErrRootNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "file root not found")
	case errors.Is(err, ErrOutsideRoot):
		return echo.NewHTTPError(http.StatusBadRequest, "path escapes configured root")
	case errors.Is(err, context.Canceled):
		return echo.NewHTTPError(http.StatusRequestTimeout, "request canceled")
	}

	if os.IsPermission(err) || errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) {
		return echo.NewHTTPError(http.StatusForbidden, "permission denied")
	}
	if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
		return echo.NewHTTPError(http.StatusNotFound, "file not found")
	}

	return err
}

// Response represents a JSON:API collection envelope for files.
type Response struct {
	Meta  *PaginationMeta  `json:"meta,omitempty"`
	Data  []Resource       `json:"data"`
	Links *PaginationLinks `json:"links,omitempty"`
}

// PaginationMeta contains pagination metadata.
type PaginationMeta struct {
	TotalCount int `json:"total_count"`
	Offset     int `json:"offset"`
	Limit      int `json:"limit"`
}

// PaginationLinks contains pagination links.
type PaginationLinks struct {
	Self  string  `json:"self"`
	First string  `json:"first"`
	Last  string  `json:"last"`
	Prev  *string `json:"prev"`
	Next  *string `json:"next"`
}

// Resource represents a single file or folder resource.
type Resource struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Attributes Attributes    `json:"attributes"`
	Links      ResourceLinks `json:"links,omitempty"`
}

// Attributes captures file metadata attributes.
type Attributes struct {
	Name           string  `json:"name"`
	ResourceKind   string  `json:"resource_kind"`
	SizeBytes      *int64  `json:"size_bytes"`
	PermissionMode string  `json:"permission_mode"`
	User           string  `json:"user"`
	Group          string  `json:"group"`
	UserID         int     `json:"user_id"`
	GroupID        int     `json:"group_id"`
	MimeType       string  `json:"mime_type"`
	AccessedAt     *string `json:"accessed_at"`
	ModifiedAt     *string `json:"modified_at"`
	ChangedAt      *string `json:"changed_at"`
	BornAt         *string `json:"born_at"`
}

// ResourceLinks contains resource links.
type ResourceLinks struct {
	Self string `json:"self"`
}

// ListParams holds pagination and sorting parameters.
type ListParams struct {
	Limit      int
	Offset     int
	SortField  string
	Descending bool
}

// validSortFields are the allowed sort field names.
var validSortFields = map[string]bool{
	"name":            true,
	"resource_kind":   true,
	"size_bytes":      true,
	"permission_mode": true,
	"user":            true,
	"group":           true,
	"user_id":         true,
	"group_id":        true,
	"mime_type":       true,
	"accessed_at":     true,
	"modified_at":     true,
	"changed_at":      true,
	"born_at":         true,
}

func parseListParams(c echo.Context) (ListParams, error) {
	params := ListParams{
		Limit:     defaultLimit,
		Offset:    0,
		SortField: "name",
	}

	// Parse page[limit]
	if limitStr := c.QueryParam("page[limit]"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			return params, echo.NewHTTPError(http.StatusBadRequest, "invalid page[limit]: must be a positive integer")
		}
		if limit > maxLimit {
			return params, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("page[limit] exceeds maximum of %d", maxLimit))
		}
		params.Limit = limit
	}

	// Parse page[offset]
	if offsetStr := c.QueryParam("page[offset]"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			return params, echo.NewHTTPError(http.StatusBadRequest, "invalid page[offset]: must be a non-negative integer")
		}
		params.Offset = offset
	}

	// Parse sort
	if sortParam := c.QueryParam("sort"); sortParam != "" {
		// Check for multi-field sort (comma-separated)
		if strings.Contains(sortParam, ",") {
			return params, echo.NewHTTPError(http.StatusBadRequest, "sorting by multiple fields is not supported")
		}

		field := sortParam
		if strings.HasPrefix(field, "-") {
			params.Descending = true
			field = strings.TrimPrefix(field, "-")
		}

		if !validSortFields[field] {
			return params, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid sort field: %s", field))
		}
		params.SortField = field
	}

	return params, nil
}

func sortDescriptors(entries []Descriptor, field string, descending bool) {
	sort.SliceStable(entries, func(i, j int) bool {
		var less bool
		switch field {
		case "name":
			less = entries[i].Metadata.Name < entries[j].Metadata.Name
		case "resource_kind":
			less = entries[i].Metadata.ResourceKind < entries[j].Metadata.ResourceKind
		case "size_bytes":
			less = comparePtrInt64(entries[i].Metadata.SizeBytes, entries[j].Metadata.SizeBytes)
		case "permission_mode":
			less = entries[i].Metadata.PermissionMode < entries[j].Metadata.PermissionMode
		case "user":
			less = entries[i].Metadata.User < entries[j].Metadata.User
		case "group":
			less = entries[i].Metadata.Group < entries[j].Metadata.Group
		case "user_id":
			less = entries[i].Metadata.UserID < entries[j].Metadata.UserID
		case "group_id":
			less = entries[i].Metadata.GroupID < entries[j].Metadata.GroupID
		case "mime_type":
			less = entries[i].Metadata.MimeType < entries[j].Metadata.MimeType
		case "accessed_at":
			less = comparePtrTime(entries[i].Metadata.AccessedAt, entries[j].Metadata.AccessedAt)
		case "modified_at":
			less = comparePtrTime(entries[i].Metadata.ModifiedAt, entries[j].Metadata.ModifiedAt)
		case "changed_at":
			less = comparePtrTime(entries[i].Metadata.ChangedAt, entries[j].Metadata.ChangedAt)
		case "born_at":
			less = comparePtrTime(entries[i].Metadata.BornAt, entries[j].Metadata.BornAt)
		default:
			less = entries[i].Metadata.Name < entries[j].Metadata.Name
		}
		if descending {
			return !less
		}
		return less
	})
}

func comparePtrInt64(a, b *int64) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil {
		return true // nil sorts before non-nil
	}
	if b == nil {
		return false
	}
	return *a < *b
}

func comparePtrTime(a, b *time.Time) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil {
		return true // nil sorts before non-nil
	}
	if b == nil {
		return false
	}
	return a.Before(*b)
}
