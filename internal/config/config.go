// Package config loads and validates dendrite-pulse configuration.
package config

import (
	"fmt"
	"net"
	"strings"
)

// Config represents application configuration.
type Config struct {
	Main MainConfig `mapstructure:"main"`
	Log  LogConfig  `mapstructure:"log"`
}

// MainConfig covers network binding.
type MainConfig struct {
	Listen string `mapstructure:"listen"`
	Port   int    `mapstructure:"port"`
}

// LogConfig covers logging options.
type LogConfig struct {
	File   string `mapstructure:"file"`
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

const (
	defaultListen   = "127.0.0.1"
	defaultPort     = 3000
	defaultLogLevel = "info"
	defaultLogFmt   = "text"
)

// Validate validates configuration fields.
func Validate(cfg Config) error {
	if ip := net.ParseIP(cfg.Main.Listen); ip == nil {
		return fmt.Errorf("invalid listen address: %s", cfg.Main.Listen)
	}
	if cfg.Main.Port < 1 || cfg.Main.Port > 65535 {
		return fmt.Errorf("invalid port: %d", cfg.Main.Port)
	}

	level := strings.ToLower(cfg.Log.Level)
	switch level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log level: %s", cfg.Log.Level)
	}

	format := strings.ToLower(cfg.Log.Format)
	switch format {
	case "text", "json":
	default:
		return fmt.Errorf("invalid log format: %s", cfg.Log.Format)
	}

	return nil
}
