package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	jsonapi "github.com/thorstenkramm/dendrite-pulse/internal/api"
	"github.com/thorstenkramm/dendrite-pulse/internal/config"
	"github.com/thorstenkramm/dendrite-pulse/internal/files"
	"github.com/thorstenkramm/dendrite-pulse/internal/server"
)

func TestNewRootCmd(t *testing.T) {
	// Reset viper state between tests
	viper.Reset()

	cmd := newRootCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "dendrite", cmd.Use)
	assert.Equal(t, "dendrite-pulse API server", cmd.Short)

	// Verify run subcommand exists
	runCmd, _, err := cmd.Find([]string{"run"})
	require.NoError(t, err)
	assert.Equal(t, "run", runCmd.Use)
}

func TestRootCmdFlags(t *testing.T) {
	viper.Reset()

	cmd := newRootCmd()

	// Test persistent flags exist
	portFlag := cmd.PersistentFlags().Lookup("port")
	require.NotNil(t, portFlag)
	assert.Equal(t, "3000", portFlag.DefValue)

	listenFlag := cmd.PersistentFlags().Lookup("listen")
	require.NotNil(t, listenFlag)
	assert.Equal(t, "127.0.0.1", listenFlag.DefValue)

	maxUploadFlag := cmd.PersistentFlags().Lookup("max-upload-size")
	require.NotNil(t, maxUploadFlag)
	assert.Equal(t, "2GB", maxUploadFlag.DefValue)

	fileModeFlag := cmd.PersistentFlags().Lookup("file-mode")
	require.NotNil(t, fileModeFlag)
	assert.Equal(t, "0600", fileModeFlag.DefValue)

	configFlag := cmd.PersistentFlags().Lookup("config")
	require.NotNil(t, configFlag)
	assert.Equal(t, "/etc/dendrite/dendrite.conf", configFlag.DefValue)

	logLevelFlag := cmd.PersistentFlags().Lookup("log-level")
	require.NotNil(t, logLevelFlag)
	assert.Equal(t, "info", logLevelFlag.DefValue)

	logFileFlag := cmd.PersistentFlags().Lookup("log-file")
	require.NotNil(t, logFileFlag)
	assert.Equal(t, "", logFileFlag.DefValue)

	logFormatFlag := cmd.PersistentFlags().Lookup("log-format")
	require.NotNil(t, logFormatFlag)
	assert.Equal(t, "text", logFormatFlag.DefValue)
}

func TestRunCmdConfigCheckFlag(t *testing.T) {
	viper.Reset()

	cmd := newRootCmd()
	runCmd, _, err := cmd.Find([]string{"run"})
	require.NoError(t, err)

	configCheckFlag := runCmd.Flags().Lookup("config-check")
	require.NotNil(t, configCheckFlag)
	assert.Equal(t, "false", configCheckFlag.DefValue)
}

func TestRunServerConfigCheck(t *testing.T) {
	viper.Reset()

	// Create a temporary config file
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	rootDir := filepath.Join(tmpDir, "root")
	require.NoError(t, os.MkdirAll(rootDir, 0o750))
	cfgContent := `
[main]
listen = "127.0.0.1"
port = 8080

[log]
level = "info"
format = "text"

[[file-root]]
virtual = "/public"
source = "` + rootDir + `"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0o600))

	// Set up viper with config-check mode
	viper.Set("config", cfgPath)
	viper.Set("config-check", true)

	// Capture stdout
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runServer(nil, nil)

	_ = w.Close()
	os.Stdout = oldStdout
	_, _ = buf.ReadFrom(r)

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Config OK:")
}

func TestRunServerInvalidConfig(t *testing.T) {
	viper.Reset()

	// Create an invalid config file
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "invalid.toml")
	rootDir := filepath.Join(tmpDir, "root")
	require.NoError(t, os.MkdirAll(rootDir, 0o750))
	cfgContent := `
[main]
listen = "not-an-ip"
port = 8080

[[file-root]]
virtual = "/public"
source = "` + rootDir + `"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0o600))

	viper.Set("config", cfgPath)
	viper.Set("config-check", false)

	err := runServer(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid listen address")
}

func TestEndToEndFileAPI(t *testing.T) {
	repoRoot := findRepoRoot(t)
	testrunDir, root1, root2 := setupTestRunDirs(t, repoRoot)
	populateTestRoots(t, repoRoot, testrunDir, root1, root2)
	cfgPath := writeTestConfig(t, repoRoot, root1, root2)
	baseURL, client := startTestServer(t, cfgPath)
	assertRootListing(t, client, baseURL)
	uploadLargeFile(t, testrunDir, baseURL, "/01-test-run", root1)
	compareDirectoryListings(t, client, baseURL, "/01-test-run", root1)
	compareDirectoryListings(t, client, baseURL, "/02-test-run", root2)

	fileEntries := append(
		collectFileEntries(t, "/01-test-run", root1),
		collectFileEntries(t, "/02-test-run", root2)...,
	)
	selected := pickRandomEntries(t, fileEntries, 15)
	validateFileEntries(t, client, baseURL, selected)
}

type fileEntry struct {
	RootVirtual string
	RootPath    string
	RelPath     string
	AbsPath     string
}

func setupTestRunDirs(t *testing.T, repoRoot string) (string, string, string) {
	t.Helper()

	testrunDir := filepath.Join(repoRoot, "testrun")
	require.NoError(t, os.RemoveAll(testrunDir))
	require.NoError(t, os.MkdirAll(testrunDir, 0o750))
	t.Cleanup(func() {
		_ = os.RemoveAll(testrunDir)
	})

	root1 := filepath.Join(testrunDir, "01-test-run")
	root2 := filepath.Join(testrunDir, "02-test-run")
	require.NoError(t, os.MkdirAll(root1, 0o750))
	require.NoError(t, os.MkdirAll(root2, 0o750))

	return testrunDir, root1, root2
}

func populateTestRoots(t *testing.T, repoRoot, testrunDir, root1, root2 string) {
	t.Helper()

	cacheDir := filepath.Join(testrunDir, ".fillfs-cache")
	_ = os.RemoveAll(cacheDir)

	runFillFS(t, repoRoot, root1, cacheDir)
	runFillFS(t, repoRoot, root2, cacheDir)
}

func uploadLargeFile(t *testing.T, testrunDir, baseURL, rootVirtual, rootPath string) {
	t.Helper()

	sourcePath := filepath.Join(testrunDir, "large-upload-source.bin")
	createLargeFile(t, sourcePath)

	// #nosec G304 -- path is generated in the test testrun directory.
	file, err := os.Open(sourcePath)
	require.NoError(t, err)
	defer func() {
		_ = file.Close()
	}()

	destName := "large.bin"
	uploadURL := baseURL + "/api/v1/files" + path.Join(rootVirtual, destName)
	req, err := http.NewRequest(http.MethodPut, uploadURL, file)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/octet-stream")

	uploadClient := &http.Client{Timeout: 10 * time.Minute}
	resp, err := uploadClient.Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected upload status %d: %s", resp.StatusCode, string(body))
	}

	var parsed jsonapi.SingleResponse[files.Resource]
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&parsed))
	require.NotNil(t, parsed.Data.Attributes.SizeBytes)

	targetPath := filepath.Join(rootPath, destName)
	info, err := os.Stat(targetPath)
	require.NoError(t, err)
	assert.Equal(t, info.Size(), *parsed.Data.Attributes.SizeBytes)
	assert.Equal(t, destName, parsed.Data.Attributes.Name)
	require.NotNil(t, parsed.Data.Attributes.ETag)
	assert.NotEmpty(t, *parsed.Data.Attributes.ETag)
}

func createLargeFile(t *testing.T, path string) {
	t.Helper()

	// #nosec G304 -- path is generated in test temp directory.
	file, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	const oneGiB = int64(1024 * 1024 * 1024)
	require.NoError(t, file.Truncate(oneGiB))
}

func startTestServer(t *testing.T, cfgPath string) (string, *http.Client) {
	t.Helper()

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)

	fileRoots := make([]files.Root, 0, len(cfg.FileRoots))
	for _, root := range cfg.FileRoots {
		fileRoots = append(fileRoots, files.Root{
			Virtual: root.Virtual,
			Source:  root.Source,
		})
	}

	maxUploadBytes, err := config.ParseMaxUploadSize(cfg.Main.MaxUploadSize)
	require.NoError(t, err)
	fileMode, err := config.ParseFileMode(cfg.Main.FileMode)
	require.NoError(t, err)

	fileSvc, err := files.NewService(fileRoots, files.Options{
		MaxUploadBytes: maxUploadBytes,
		FileMode:       fileMode,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, "127.0.0.1:24499", server.Config{
			FileService: fileSvc,
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatalf("server shutdown timeout")
		}
	})

	baseURL := "http://127.0.0.1:24499"
	waitForServer(t, baseURL, errCh)

	client := &http.Client{Timeout: 5 * time.Second}
	return baseURL, client
}

func assertRootListing(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()

	rootListing := fetchListing(t, client, baseURL, "/api/v1/files")
	require.Len(t, rootListing.Data, 2)

	rootNames := map[string]struct{}{
		"01-test-run": {},
		"02-test-run": {},
	}
	for _, item := range rootListing.Data {
		_, ok := rootNames[item.Attributes.Name]
		assert.True(t, ok, "unexpected root listed: %s", item.Attributes.Name)
	}
}

func pickRandomEntries(t *testing.T, entries []fileEntry, count int) []fileEntry {
	t.Helper()

	require.GreaterOrEqual(t, len(entries), count)

	selected := make([]fileEntry, 0, count)
	used := make(map[int]struct{}, count)

	for len(selected) < count {
		idx := cryptoRandIndex(t, len(entries))
		if _, ok := used[idx]; ok {
			continue
		}
		used[idx] = struct{}{}
		selected = append(selected, entries[idx])
	}

	return selected
}

func cryptoRandIndex(t *testing.T, upper int) int {
	t.Helper()

	if upper <= 0 {
		t.Fatalf("invalid max index: %d", upper)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(upper)))
	require.NoError(t, err)
	return int(n.Int64())
}

func validateFileEntries(t *testing.T, client *http.Client, baseURL string, entries []fileEntry) {
	t.Helper()

	listingCache := make(map[string]jsonapi.CollectionResponse[files.Resource])
	for _, entry := range entries {
		parentRel := path.Dir(entry.RelPath)
		if parentRel == "." {
			parentRel = ""
		}
		cacheKey := entry.RootVirtual + "|" + parentRel
		resp, ok := listingCache[cacheKey]
		if !ok {
			apiPath := apiPathFor(entry.RootVirtual, parentRel)
			resp = fetchListing(t, client, baseURL, apiPath)
			listingCache[cacheKey] = resp
		}

		resource, found := findResource(resp, path.Base(entry.RelPath))
		require.True(t, found, "missing file in listing: %s", entry.RelPath)
		validateFileAttributes(t, resource, entry)
	}
}
func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}

	t.Fatalf("failed to locate repo root")
	return ""
}

func writeTestConfig(t *testing.T, repoRoot, root1, root2 string) string {
	t.Helper()

	testDataDir := filepath.Join(repoRoot, "test_data")
	require.NoError(t, os.MkdirAll(testDataDir, 0o750))

	cfgPath := filepath.Join(testDataDir, "dendrite_test1.conf")
	cfg := fmt.Sprintf(`
[main]
listen = "127.0.0.1"
port = 24499
max_upload_size = "2GB"
file_mode = "0600"

[[file-root]]
virtual = "/01-test-run"
source = "%s"

[[file-root]]
virtual = "/02-test-run"
source = "%s"
`, root1, root2)

	require.NoError(t, os.WriteFile(cfgPath, []byte(strings.TrimSpace(cfg)+"\n"), 0o600))
	t.Cleanup(func() {
		_ = os.Remove(cfgPath)
		_ = os.Remove(testDataDir)
	})
	return cfgPath
}

func runFillFS(t *testing.T, repoRoot, dest, cacheDir string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"go",
		"run",
		"github.com/thorstenkramm/fillfs/cmd/fillfs",
		"--dest", dest,
		"--cache-dir", cacheDir,
		"--folders", "2",
		"--files-per-folder", "10",
		"--depths", "2",
		"--yes",
		"--wipe-dest",
	)
	cmd.Dir = repoRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fillfs failed: %v\n%s", err, string(output))
	}
}

func waitForServer(t *testing.T, baseURL string, errCh <-chan error) {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/api/v1/ping")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case err := <-errCh:
			t.Fatalf("server failed to start: %v", err)
		default:
		}
		time.Sleep(150 * time.Millisecond)
	}

	t.Fatalf("server did not start in time")
}

func compareDirectoryListings(
	t *testing.T,
	client *http.Client,
	baseURL string,
	rootVirtual string,
	rootPath string,
) {
	t.Helper()

	err := filepath.WalkDir(rootPath, func(dir string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(rootPath, dir)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", dir, err)
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}

		apiPath := apiPathFor(rootVirtual, rel)
		resp := fetchListing(t, client, baseURL, apiPath)

		expectedEntries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("read dir %s: %w", dir, err)
		}

		require.Len(t, resp.Data, len(expectedEntries))

		seen := make(map[string]files.Resource, len(resp.Data))
		for _, resource := range resp.Data {
			seen[resource.Attributes.Name] = resource
		}

		for _, fsEntry := range expectedEntries {
			resource, ok := seen[fsEntry.Name()]
			if !ok {
				t.Fatalf("missing entry %s in listing for %s", fsEntry.Name(), apiPath)
			}
			expectedType := expectedEntryType(dir, fsEntry.Name())
			assert.Equal(t, expectedType, resource.Attributes.ResourceKind)
		}

		return nil
	})
	require.NoError(t, err)
}

func collectFileEntries(t *testing.T, rootVirtual, rootPath string) []fileEntry {
	t.Helper()

	var entries []fileEntry
	err := filepath.WalkDir(rootPath, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat entry %s: %w", p, err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(rootPath, p)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", p, err)
		}
		rel = filepath.ToSlash(rel)

		entries = append(entries, fileEntry{
			RootVirtual: rootVirtual,
			RootPath:    rootPath,
			RelPath:     rel,
			AbsPath:     p,
		})
		return nil
	})
	require.NoError(t, err)
	return entries
}

func apiPathFor(rootVirtual, rel string) string {
	full := rootVirtual
	if rel != "" {
		full = path.Join(rootVirtual, rel)
	}
	if full == "/" {
		return "/api/v1/files"
	}

	parts := strings.Split(strings.TrimPrefix(full, "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return "/api/v1/files/" + strings.Join(parts, "/")
}

func fetchListing(
	t *testing.T,
	client *http.Client,
	baseURL string,
	apiPath string,
) jsonapi.CollectionResponse[files.Resource] {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+apiPath, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d for %s: %s", resp.StatusCode, apiPath, string(body))
	}

	var parsed jsonapi.CollectionResponse[files.Resource]
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&parsed))
	return parsed
}

func findResource(resp jsonapi.CollectionResponse[files.Resource], name string) (files.Resource, bool) {
	for _, resource := range resp.Data {
		if resource.Attributes.Name == name {
			return resource, true
		}
	}
	return files.Resource{}, false
}

func validateFileAttributes(t *testing.T, resource files.Resource, entry fileEntry) {
	t.Helper()

	info, err := os.Stat(entry.AbsPath)
	require.NoError(t, err)

	assert.Equal(t, "file", resource.Attributes.ResourceKind)
	assert.Equal(t, path.Base(entry.RelPath), resource.Attributes.Name)
	require.NotNil(t, resource.Attributes.SizeBytes)
	assert.Equal(t, info.Size(), *resource.Attributes.SizeBytes)
	assert.Equal(t, fmt.Sprintf("%04o", info.Mode().Perm()), resource.Attributes.PermissionMode)

	expectedMime := detectMime(entry.AbsPath)
	assert.Equal(t, expectedMime, resource.Attributes.MimeType)

	validateOwnership(t, resource, info)
	validateTimes(t, resource, info)
}

func validateOwnership(t *testing.T, resource files.Resource, info os.FileInfo) {
	t.Helper()

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}

	uid := int(stat.Uid)
	gid := int(stat.Gid)

	assert.Equal(t, uid, resource.Attributes.UserID)
	assert.Equal(t, gid, resource.Attributes.GroupID)

	if userName, err := user.LookupId(fmt.Sprintf("%d", uid)); err == nil && userName.Username != "" {
		assert.Equal(t, userName.Username, resource.Attributes.User)
	}
	if groupName, err := user.LookupGroupId(fmt.Sprintf("%d", gid)); err == nil && groupName.Name != "" {
		assert.Equal(t, groupName.Name, resource.Attributes.Group)
	}
}

func validateTimes(t *testing.T, resource files.Resource, info os.FileInfo) {
	t.Helper()

	atime, mtime, ctime, btime := statTimes(info)
	if atime != nil {
		assertTimeEqual(t, resource.Attributes.AccessedAt, *atime)
	}
	if mtime != nil {
		assertTimeEqual(t, resource.Attributes.ModifiedAt, *mtime)
	}
	if ctime != nil {
		assertTimeEqual(t, resource.Attributes.ChangedAt, *ctime)
	}
	if btime != nil {
		assertTimeEqual(t, resource.Attributes.BornAt, *btime)
	}
}

func assertTimeEqual(t *testing.T, value *string, expected time.Time) {
	t.Helper()

	if value == nil {
		t.Fatalf("missing expected timestamp")
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	require.NoError(t, err)
	assert.WithinDuration(t, expected.UTC(), parsed.UTC(), 3*time.Second)
}

func detectMime(path string) string {
	// #nosec G304 -- path originates from generated test data under the testrun directory.
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() {
		_ = file.Close()
	}()

	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}

	return http.DetectContentType(buf[:n])
}

func expectedEntryType(dir, name string) string {
	info, err := os.Lstat(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "symlink"
	}
	if info.IsDir() {
		return "folder"
	}
	return "file"
}
