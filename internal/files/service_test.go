package files

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeSymlinkToFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")
	require.NoError(t, os.WriteFile(target, []byte("hello"), 0o600))
	link := filepath.Join(root, "link-note")
	require.NoError(t, os.Symlink("note.txt", link))

	svc := newTestService(t, root)

	desc, err := svc.Describe(t.Context(), "/public", "link-note")
	require.NoError(t, err)

	assert.Equal(t, "symlink", desc.Kind)
	assert.Equal(t, "file", desc.TargetKind)
	assert.Nil(t, desc.Metadata.SizeBytes, "symlink size must be nil")
	assert.Equal(t, "text/plain; charset=utf-8", desc.Metadata.MimeType)
}

func TestListDirectoryPreventsTraversal(t *testing.T) {
	root := t.TempDir()
	svc := newTestService(t, root)

	_, err := svc.Describe(t.Context(), "/public", "../etc/passwd")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOutsideRoot)
}

func TestListDirectorySortsByName(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o600))

	svc := newTestService(t, root)

	entries, err := svc.ListDirectory(t.Context(), "/public", "")
	require.NoError(t, err)

	require.Len(t, entries, 2)
	assert.Equal(t, "a.txt", entries[0].Metadata.Name)
	assert.Equal(t, "b.txt", entries[1].Metadata.Name)
}

func TestSymlinkOutsideRootRejected(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "leak")))

	svc := newTestService(t, root)

	_, err := svc.Describe(t.Context(), "/public", "leak")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOutsideRoot)
}

func newTestService(t *testing.T, root string) *Service {
	t.Helper()

	svc, err := NewService([]Root{{Virtual: "/public", Source: root}})
	require.NoError(t, err)
	return svc
}
