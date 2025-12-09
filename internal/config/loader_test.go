package config

import (
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
	cfgPath := writeTempConfig(t, `
[main]
listen = "0.0.0.0"
port = 4000

[log]
level = "warn"
format = "json"
file = "/file/config.log"
`)

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("listen", "", "")
	flags.Int("port", 0, "")
	flags.String("log-level", "", "")
	flags.String("log-file", "", "")
	flags.String("log-format", "", "")
	require.NoError(t, flags.Parse([]string{"--port", "4002", "--log-level", "debug"}))

	require.NoError(t, v.BindPFlag("main.listen", flags.Lookup("listen")))
	require.NoError(t, v.BindPFlag("main.port", flags.Lookup("port")))
	require.NoError(t, v.BindPFlag("log.level", flags.Lookup("log-level")))
	require.NoError(t, v.BindPFlag("log.file", flags.Lookup("log-file")))
	require.NoError(t, v.BindPFlag("log.format", flags.Lookup("log-format")))

	t.Setenv("DENDRITE_LOG_FILE", "/env/logfile")

	cfg, err := loader.Load(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0", cfg.Main.Listen)
	assert.Equal(t, 4002, cfg.Main.Port)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "/env/logfile", cfg.Log.File)
	assert.Equal(t, "json", cfg.Log.Format)
}

func TestLoaderMissingConfigFileUsesDefaults(t *testing.T) {
	v := viper.New()
	loader := NewLoader(v)

	cfg, err := loader.Load(filepath.Join(t.TempDir(), "missing.toml"))
	require.NoError(t, err)

	assert.Equal(t, defaultListen, cfg.Main.Listen)
	assert.Equal(t, defaultPort, cfg.Main.Port)
	assert.Equal(t, defaultLogLevel, cfg.Log.Level)
	assert.Equal(t, defaultLogFmt, cfg.Log.Format)
	assert.Equal(t, "", cfg.Log.File)
}

func TestLoaderValidatesConfig(t *testing.T) {
	v := viper.New()
	loader := NewLoader(v)
	cfgPath := writeTempConfig(t, `
[main]
listen = "not-an-ip"

[log]
level = "info"
format = "text"
`)

	_, err := loader.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid listen address")
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
