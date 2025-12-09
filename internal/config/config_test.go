package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateListenAddress(t *testing.T) {
	tests := []struct {
		name    string
		listen  string
		wantErr string
	}{
		{"valid IPv4", "127.0.0.1", ""},
		{"valid IPv6", "::1", ""},
		{"valid 0.0.0.0", "0.0.0.0", ""},
		{"hostname rejected", "localhost", "invalid listen address: localhost"},
		{"empty rejected", "", "invalid listen address: "},
		{"malformed IP", "192.168.1.999", "invalid listen address: 192.168.1.999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Main: MainConfig{Listen: tt.listen, Port: 3000},
				Log:  LogConfig{Level: "info", Format: "text"},
			}
			err := Validate(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr string
	}{
		{"valid port", 3000, ""},
		{"minimum valid", 1, ""},
		{"maximum valid", 65535, ""},
		{"zero rejected", 0, "invalid port: 0"},
		{"exceeds max", 65536, "invalid port: 65536"},
		{"negative rejected", -1, "invalid port: -1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Main: MainConfig{Listen: "127.0.0.1", Port: tt.port},
				Log:  LogConfig{Level: "info", Format: "text"},
			}
			err := Validate(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		wantErr string
	}{
		{"debug", "debug", ""},
		{"info", "info", ""},
		{"warn", "warn", ""},
		{"error", "error", ""},
		{"uppercase INFO", "INFO", ""},
		{"invalid verbose", "verbose", "invalid log level: verbose"},
		{"empty rejected", "", "invalid log level: "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Main: MainConfig{Listen: "127.0.0.1", Port: 3000},
				Log:  LogConfig{Level: tt.level, Format: "text"},
			}
			err := Validate(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateLogFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr string
	}{
		{"text", "text", ""},
		{"json", "json", ""},
		{"uppercase JSON", "JSON", ""},
		{"invalid xml", "xml", "invalid log format: xml"},
		{"empty rejected", "", "invalid log format: "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Main: MainConfig{Listen: "127.0.0.1", Port: 3000},
				Log:  LogConfig{Level: "info", Format: tt.format},
			}
			err := Validate(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoadConvenienceWrapper(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.toml")
	require.NoError(t, err)

	assert.Equal(t, defaultListen, cfg.Main.Listen)
	assert.Equal(t, defaultPort, cfg.Main.Port)
	assert.Equal(t, defaultLogLevel, cfg.Log.Level)
	assert.Equal(t, defaultLogFmt, cfg.Log.Format)
}

func TestNewLoaderWithNilViper(t *testing.T) {
	loader := NewLoader(nil)
	require.NotNil(t, loader)

	cfg, err := loader.Load("/nonexistent/path/config.toml")
	require.NoError(t, err)
	assert.Equal(t, defaultListen, cfg.Main.Listen)
}
