// Package config loads and validates dendrite-pulse configuration.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// Validation errors that callers can check with errors.Is.
var (
	// ErrInvalidListenAddress indicates the listen address is not a valid IP.
	ErrInvalidListenAddress = errors.New("invalid listen address")

	// ErrInvalidPort indicates the port number is out of range.
	ErrInvalidPort = errors.New("invalid port")

	// ErrInvalidLogLevel indicates an unsupported log level.
	ErrInvalidLogLevel = errors.New("invalid log level")

	// ErrInvalidLogFormat indicates an unsupported log format.
	ErrInvalidLogFormat = errors.New("invalid log format")

	// ErrNoFileRoots indicates no file roots were configured.
	ErrNoFileRoots = errors.New("no file roots configured")
)

// Config represents application configuration.
type Config struct {
	Main      MainConfig `mapstructure:"main"`
	Log       LogConfig  `mapstructure:"log"`
	FileRoots []FileRoot `mapstructure:"file-root"`
}

// FileRoot maps a virtual folder to a source directory.
type FileRoot struct {
	Virtual string `mapstructure:"virtual"`
	Source  string `mapstructure:"source"`
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
		return fmt.Errorf("%w: %s", ErrInvalidListenAddress, cfg.Main.Listen)
	}
	if cfg.Main.Port < 1 || cfg.Main.Port > 65535 {
		return fmt.Errorf("%w: %d", ErrInvalidPort, cfg.Main.Port)
	}

	level := strings.ToLower(cfg.Log.Level)
	switch level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("%w: %s", ErrInvalidLogLevel, cfg.Log.Level)
	}

	format := strings.ToLower(cfg.Log.Format)
	switch format {
	case "text", "json":
	default:
		return fmt.Errorf("%w: %s", ErrInvalidLogFormat, cfg.Log.Format)
	}

	return validateFileRoots(cfg.FileRoots)
}

func validateFileRoots(roots []FileRoot) error {
	if len(roots) == 0 {
		return ErrNoFileRoots
	}

	seenVirtuals := make(map[string]struct{})
	for i, root := range roots {
		if strings.TrimSpace(root.Virtual) != root.Virtual || strings.TrimSpace(root.Source) != root.Source {
			return fmt.Errorf("file root %d: leading or trailing whitespace is not allowed", i)
		}
		if root.Virtual == "" {
			return fmt.Errorf("file root %d: virtual cannot be empty", i)
		}
		if root.Source == "" {
			return fmt.Errorf("file root %d: source cannot be empty", i)
		}
		if !strings.HasPrefix(root.Virtual, "/") {
			return fmt.Errorf("file root %d: virtual must start with '/'", i)
		}
		// Virtual must be "/" or a single folder like "/public" (exactly one slash at the start)
		if root.Virtual != "/" && strings.Count(root.Virtual, "/") != 1 {
			return fmt.Errorf("file root %d: virtual must be '/' or a single folder (e.g. '/public')", i)
		}
		// Reject colons in virtual and source paths
		if strings.Contains(root.Virtual, ":") {
			return fmt.Errorf("file root %d: virtual path cannot contain a colon", i)
		}
		if strings.Contains(root.Source, ":") {
			return fmt.Errorf("file root %d: source path cannot contain a colon", i)
		}
		if !filepath.IsAbs(root.Source) || !strings.HasPrefix(root.Source, "/") {
			return fmt.Errorf("file root %d: source must be an absolute path starting with '/': %s", i, root.Source)
		}

		info, err := os.Stat(root.Source)
		if err != nil {
			return fmt.Errorf("file root %d: stat source %s: %w", i, root.Source, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("file root %d: source is not a directory: %s", i, root.Source)
		}

		if _, exists := seenVirtuals[root.Virtual]; exists {
			return fmt.Errorf("file root %d: duplicate virtual path: %s", i, root.Virtual)
		}
		seenVirtuals[root.Virtual] = struct{}{}
	}

	return nil
}
