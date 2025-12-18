// Package files provides filesystem access scoped to configured roots.
package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ErrRootNotFound indicates the requested virtual root does not exist.
var ErrRootNotFound = errors.New("file root not found")

// ErrOutsideRoot indicates a path resolves outside its configured root.
var ErrOutsideRoot = errors.New("path escapes configured root")

// Root maps a virtual folder to a source directory.
type Root struct {
	Virtual string
	Source  string
}

// Service exposes file operations scoped to configured roots.
type Service struct {
	roots   map[string]Root
	ordered []Root
}

const (
	kindFile    = "file"
	kindFolder  = "folder"
	kindSymlink = "symlink"
)

// NewService creates a new Service.
func NewService(roots []Root) (*Service, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("no file roots provided")
	}

	ordered := make([]Root, 0, len(roots))
	rootMap := make(map[string]Root, len(roots))
	for _, r := range roots {
		if _, exists := rootMap[r.Virtual]; exists {
			return nil, fmt.Errorf("duplicate file root: %s", r.Virtual)
		}
		resolvedSource, err := filepath.EvalSymlinks(r.Source)
		if err != nil {
			return nil, fmt.Errorf("resolve file root %s: %w", r.Virtual, err)
		}
		normalized := Root{
			Virtual: r.Virtual,
			Source:  filepath.Clean(resolvedSource),
		}
		ordered = append(ordered, normalized)
		rootMap[r.Virtual] = normalized
	}

	return &Service{
		roots:   rootMap,
		ordered: ordered,
	}, nil
}

// Descriptor describes a resolved filesystem entry.
type Descriptor struct {
	Root         Root
	VirtualPath  string
	RelPath      string
	Name         string
	Kind         string // file, folder, symlink
	TargetKind   string // after resolving symlinks
	AbsolutePath string // resolved target path (for file or directory)
	LinkPath     string // symlink path; equals AbsolutePath when not a symlink
	Metadata     Metadata
}

// Metadata captures file attributes.
type Metadata struct {
	Name           string
	VirtualPath    string
	ResourceKind   string
	SizeBytes      *int64
	PermissionMode string
	User           string
	Group          string
	UserID         int
	GroupID        int
	MimeType       string
	AccessedAt     *time.Time
	ModifiedAt     *time.Time
	ChangedAt      *time.Time
	BornAt         *time.Time
}

// HasSingleRootSlash returns true if there's exactly one root and its virtual path is "/".
func (s *Service) HasSingleRootSlash() bool {
	return len(s.ordered) == 1 && s.ordered[0].Virtual == "/"
}

// ListRoots returns descriptors for all configured roots.
func (s *Service) ListRoots(ctx context.Context) ([]Descriptor, error) {
	descs := make([]Descriptor, 0, len(s.ordered))
	for _, root := range s.ordered {
		desc, err := s.describe(ctx, root, "")
		if err != nil {
			return nil, err
		}
		descs = append(descs, desc)
	}
	return descs, nil
}

// Describe resolves a single path beneath a virtual root.
func (s *Service) Describe(ctx context.Context, virtual, rel string) (Descriptor, error) {
	root, ok := s.lookupRoot(virtual)
	if !ok {
		return Descriptor{}, fmt.Errorf("%w: %s", ErrRootNotFound, virtual)
	}
	return s.describe(ctx, root, rel)
}

// Roots returns configured roots.
func (s *Service) Roots() []Root {
	out := make([]Root, len(s.ordered))
	copy(out, s.ordered)
	return out
}

// ListDirectory lists entries within a directory.
func (s *Service) ListDirectory(ctx context.Context, virtual, rel string) ([]Descriptor, error) {
	root, ok := s.lookupRoot(virtual)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRootNotFound, virtual)
	}

	relClean, err := cleanRelativePath(rel)
	if err != nil {
		return nil, err
	}

	parentDesc, err := s.describe(ctx, root, relClean)
	if err != nil {
		return nil, err
	}
	if parentDesc.TargetKind != kindFolder {
		return nil, fmt.Errorf("not a directory: %s", parentDesc.VirtualPath)
	}

	entries, err := os.ReadDir(parentDesc.AbsolutePath)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	descs := make([]Descriptor, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled: %w", ctx.Err())
		default:
		}

		childRel := path.Join(relClean, entry.Name())
		desc, err := s.describe(ctx, root, childRel)
		if err != nil {
			return nil, err
		}
		descs = append(descs, desc)
	}

	return descs, nil
}

func (s *Service) describe(_ context.Context, root Root, rel string) (Descriptor, error) {
	relClean, err := cleanRelativePath(rel)
	if err != nil {
		return Descriptor{}, err
	}

	virtualPath := joinVirtual(root.Virtual, relClean)

	absPath := filepath.Join(root.Source, filepath.FromSlash(relClean))
	info, err := os.Lstat(absPath)
	if err != nil {
		return Descriptor{}, fmt.Errorf("stat %s: %w", virtualPath, err)
	}

	kind := classify(info)

	desc := Descriptor{
		Root:        root,
		RelPath:     relClean,
		Name:        entryName(root, relClean),
		Kind:        kind,
		TargetKind:  kind,
		LinkPath:    absPath,
		VirtualPath: virtualPath,
	}

	var targetInfo os.FileInfo
	switch kind {
	case kindSymlink:
		resolved, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			return Descriptor{}, fmt.Errorf("resolve symlink %s: %w", virtualPath, err)
		}
		if err := ensureWithinRoot(root.Source, resolved); err != nil {
			return Descriptor{}, err
		}
		tInfo, err := os.Stat(resolved)
		if err != nil {
			return Descriptor{}, fmt.Errorf("stat symlink target %s: %w", virtualPath, err)
		}
		desc.AbsolutePath = resolved
		desc.TargetKind = classify(tInfo)
		targetInfo = tInfo
	default:
		desc.AbsolutePath = absPath
		targetInfo = info
	}

	desc.Metadata = metadataFromInfo(desc, targetInfo)

	return desc, nil
}

func (s *Service) lookupRoot(virtual string) (Root, bool) {
	if !strings.HasPrefix(virtual, "/") {
		virtual = "/" + virtual
	}
	root, ok := s.roots[virtual]
	return root, ok
}

func classify(info os.FileInfo) string {
	switch mode := info.Mode(); {
	case mode.IsDir():
		return kindFolder
	case mode&os.ModeSymlink != 0:
		return kindSymlink
	default:
		return kindFile
	}
}

func cleanRelativePath(rel string) (string, error) {
	if hasTraversal(rel) {
		return "", fmt.Errorf("%w: %s", ErrOutsideRoot, rel)
	}

	cleaned := path.Clean("/" + rel)
	if cleaned == "/" {
		return "", nil
	}
	if strings.HasPrefix(cleaned, "/../") || cleaned == "/.." {
		return "", fmt.Errorf("%w: %s", ErrOutsideRoot, rel)
	}
	return strings.TrimPrefix(cleaned, "/"), nil
}

func hasTraversal(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return false
}

func joinVirtual(virtual, rel string) string {
	if rel == "" {
		return virtual
	}
	return path.Join(virtual, rel)
}

func entryName(root Root, rel string) string {
	if rel == "" {
		if root.Virtual == "/" {
			return "/"
		}
		return strings.TrimPrefix(root.Virtual, "/")
	}
	return path.Base(rel)
}

func metadataFromInfo(desc Descriptor, info os.FileInfo) Metadata {
	mode := info.Mode().Perm()
	sizeBytes := pointerSize(info, desc.Kind)

	uid, gid, userName, groupName := ownership(info)
	accessed, modified, changed, born := fileTimes(info)

	mimeType := mimeFor(desc.TargetKind, desc.AbsolutePath)

	return Metadata{
		Name:           desc.Name,
		VirtualPath:    desc.VirtualPath,
		ResourceKind:   desc.Kind,
		SizeBytes:      sizeBytes,
		PermissionMode: fmt.Sprintf("%04o", mode),
		User:           userName,
		Group:          groupName,
		UserID:         uid,
		GroupID:        gid,
		MimeType:       mimeType,
		AccessedAt:     accessed,
		ModifiedAt:     modified,
		ChangedAt:      changed,
		BornAt:         born,
	}
}

func pointerSize(info os.FileInfo, kind string) *int64 {
	if kind == kindFile {
		size := info.Size()
		return &size
	}
	return nil
}

func ownership(info os.FileInfo) (int, int, string, string) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, "", ""
	}

	uid := int(stat.Uid)
	gid := int(stat.Gid)

	userName := strconvOrEmpty(uid)
	groupName := strconvOrEmpty(gid)

	if u, err := user.LookupId(fmt.Sprintf("%d", uid)); err == nil {
		userName = u.Username
	}
	if g, err := user.LookupGroupId(fmt.Sprintf("%d", gid)); err == nil {
		groupName = g.Name
	}

	return uid, gid, userName, groupName
}

func fileTimes(info os.FileInfo) (*time.Time, *time.Time, *time.Time, *time.Time) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, nil, nil, nil
	}

	accessed, modified, changed, born := extractTimes(stat)

	return toPtr(accessed), toPtr(modified), toPtr(changed), born
}

func toPtr(t time.Time) *time.Time {
	return &t
}

func ensureWithinRoot(root, target string) error {
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrOutsideRoot
	}
	return nil
}

func mimeFor(kind, absPath string) string {
	if kind == kindFolder {
		return "inode/directory"
	}
	if kind == kindSymlink {
		return "inode/symlink"
	}

	// #nosec G304 -- absPath is validated to be within the configured root.
	f, err := os.Open(absPath)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}

	return http.DetectContentType(buf[:n])
}

func strconvOrEmpty(v int) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%d", v)
}
