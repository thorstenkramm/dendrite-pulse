package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoaderPrecedence(t *testing.T) {
	v := viper.New()
	loader := NewLoader(v)
	configRoot := filepath.Join(t.TempDir(), "config-root")
	require.NoError(t, os.MkdirAll(configRoot, 0o750))
	envRoot := filepath.Join(t.TempDir(), "env-root")
	require.NoError(t, os.MkdirAll(envRoot, 0o750))
	flagRoot := filepath.Join(t.TempDir(), "flag-root")
	require.NoError(t, os.MkdirAll(flagRoot, 0o750))

	cfgPath := writeTempConfig(t, fmt.Sprintf(`
[main]
listen = "0.0.0.0"
port = 4000

[log]
level = "warn"
format = "json"
file = "/file/config.log"

[[file-root]]
virtual = "/cfg"
source = "%s"
`, configRoot))

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("listen", "", "")
	flags.Int("port", 0, "")
	flags.String("log-level", "", "")
	flags.String("log-file", "", "")
	flags.String("log-format", "", "")
	flags.StringArray("file-root", nil, "")
	require.NoError(t, flags.Parse([]string{
		"--port", "4002",
		"--log-level", "debug",
		"--file-root", "/flag:" + flagRoot,
	}))

	require.NoError(t, v.BindPFlag("main.listen", flags.Lookup("listen")))
	require.NoError(t, v.BindPFlag("main.port", flags.Lookup("port")))
	require.NoError(t, v.BindPFlag("log.level", flags.Lookup("log-level")))
	require.NoError(t, v.BindPFlag("log.file", flags.Lookup("log-file")))
	require.NoError(t, v.BindPFlag("log.format", flags.Lookup("log-format")))
	require.NoError(t, v.BindPFlag("file-root", flags.Lookup("file-root")))

	t.Setenv("DENDRITE_LOG_FILE", "/env/logfile")
	t.Setenv("DENDRITE_FILE_ROOT", "/env:"+envRoot)

	cfg, err := loader.Load(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0", cfg.Main.Listen)
	assert.Equal(t, 4002, cfg.Main.Port)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "/env/logfile", cfg.Log.File)
	assert.Equal(t, "json", cfg.Log.Format)
	require.Len(t, cfg.FileRoots, 1)
	assert.Equal(t, "/flag", cfg.FileRoots[0].Virtual)
	assert.Equal(t, flagRoot, cfg.FileRoots[0].Source)
}

func TestLoaderMissingConfigFileUsesDefaults(t *testing.T) {
	v := viper.New()
	loader := NewLoader(v)
	root := filepath.Join(t.TempDir(), "root")
	require.NoError(t, os.MkdirAll(root, 0o750))
	t.Setenv("DENDRITE_FILE_ROOT", "/env:"+root)

	cfg, err := loader.Load(filepath.Join(t.TempDir(), "missing.toml"))
	require.NoError(t, err)

	assert.Equal(t, defaultListen, cfg.Main.Listen)
	assert.Equal(t, defaultPort, cfg.Main.Port)
	assert.Equal(t, defaultLogLevel, cfg.Log.Level)
	assert.Equal(t, defaultLogFmt, cfg.Log.Format)
	assert.Equal(t, "", cfg.Log.File)
	require.Len(t, cfg.FileRoots, 1)
	assert.Equal(t, "/env", cfg.FileRoots[0].Virtual)
	assert.Equal(t, root, cfg.FileRoots[0].Source)
}

func TestLoaderValidatesConfig(t *testing.T) {
	v := viper.New()
	loader := NewLoader(v)
	root := filepath.Join(t.TempDir(), "root")
	require.NoError(t, os.MkdirAll(root, 0o750))
	cfgPath := writeTempConfig(t, fmt.Sprintf(`
[main]
listen = "not-an-ip"

[log]
level = "info"
format = "text"

[[file-root]]
virtual = "/root"
source = "%s"
`, root))

	_, err := loader.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid listen address")
}

func TestParseFileRootDefinitions(t *testing.T) {
	defs := []string{"/public:/var/www/public,/docs:/srv/docs", "/tmp:/tmp"}

	roots, err := parseFileRootDefinitions(defs)
	require.NoError(t, err)

	require.Len(t, roots, 3)
	assert.Equal(t, "/public", roots[0].Virtual)
	assert.Equal(t, "/var/www/public", roots[0].Source)
	assert.Equal(t, "/docs", roots[1].Virtual)
	assert.Equal(t, "/srv/docs", roots[1].Source)
	assert.Equal(t, "/tmp", roots[2].Virtual)
	assert.Equal(t, "/tmp", roots[2].Source)

	_, err = parseFileRootDefinitions([]string{"missing-delimiter"})
	require.Error(t, err)
}

func TestValidateFileRoots(t *testing.T) {
	base := Config{
		Main: MainConfig{Listen: "127.0.0.1", Port: 3000},
		Log:  LogConfig{Level: "info", Format: "text"},
	}

	t.Run("whitespace", func(t *testing.T) {
		cfg := base
		cfg.FileRoots = []FileRoot{{Virtual: "/space ", Source: "/tmp"}}
		err := Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "whitespace")
	})

	t.Run("multi-folder virtual", func(t *testing.T) {
		cfg := base
		cfg.FileRoots = []FileRoot{{Virtual: "/public/foo", Source: "/tmp"}}
		err := Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "single folder")
	})

	t.Run("non-existent source", func(t *testing.T) {
		cfg := base
		cfg.FileRoots = []FileRoot{{Virtual: "/public", Source: "/definitely/missing"}}
		err := Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stat source")
	})

	t.Run("duplicate virtual", func(t *testing.T) {
		dir := t.TempDir()
		cfg := base
		cfg.FileRoots = []FileRoot{
			{Virtual: "/public", Source: dir},
			{Virtual: "/public", Source: dir},
		}
		err := Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate virtual")
	})
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
