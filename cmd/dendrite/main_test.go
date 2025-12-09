package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	cfgContent := `
[main]
listen = "127.0.0.1"
port = 8080

[log]
level = "info"
format = "text"
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
	cfgContent := `
[main]
listen = "not-an-ip"
port = 8080
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0o600))

	viper.Set("config", cfgPath)
	viper.Set("config-check", false)

	err := runServer(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid listen address")
}
