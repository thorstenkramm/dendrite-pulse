package config

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Loader loads dendrite-pulse configuration using a shared Viper instance.
type Loader struct {
	v *viper.Viper
}

// NewLoader constructs a loader. A new Viper instance is created when v is nil.
func NewLoader(v *viper.Viper) *Loader {
	if v == nil {
		v = viper.New()
	}
	return &Loader{v: v}
}

// Load resolves configuration with precedence: defaults < config file < env < flags.
func (l *Loader) Load(configPath string) (Config, error) {
	v := l.v
	v.SetConfigType("toml")
	v.SetConfigFile(configPath)

	v.SetDefault("main.listen", defaultListen)
	v.SetDefault("main.port", defaultPort)
	v.SetDefault("log.level", defaultLogLevel)
	v.SetDefault("log.format", defaultLogFmt)

	v.SetEnvPrefix("DENDRITE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		var cfg Config
		var notFound viper.ConfigFileNotFoundError
		switch {
		case errors.As(err, &notFound):
		case errors.Is(err, fs.ErrNotExist):
		default:
			return cfg, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	settings := v.AllSettings()
	delete(settings, "file-root")

	if err := decodeSettings(settings, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal config: %w", err)
	}

	roots, err := decodeFileRoots(v.Get("file-root"))
	if err != nil {
		return cfg, err
	}
	if len(roots) > 0 {
		cfg.FileRoots = roots
	}

	if err := Validate(cfg); err != nil {
		return cfg, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// Load is a convenience wrapper that uses a fresh Viper instance.
func Load(configPath string) (Config, error) {
	return NewLoader(viper.New()).Load(configPath)
}

func parseFileRootDefinitions(defs []string) ([]FileRoot, error) {
	var roots []FileRoot

	for i, def := range defs {
		if def == "" {
			return nil, fmt.Errorf("file root %d: empty definition", i)
		}

		entries := strings.Split(def, ",")
		for j, entry := range entries {
			if entry == "" {
				return nil, fmt.Errorf("file root %d entry %d: empty definition", i, j)
			}

			parts := strings.SplitN(entry, ":", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("file root %d entry %d: expected format virtual:source", i, j)
			}

			virtual := parts[0]
			source := parts[1]

			if virtual == "" || source == "" {
				return nil, fmt.Errorf("file root %d entry %d: virtual and source must be non-empty", i, j)
			}

			roots = append(roots, FileRoot{
				Virtual: virtual,
				Source:  source,
			})
		}
	}

	return roots, nil
}

func decodeSettings(settings map[string]interface{}, cfg *Config) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "mapstructure",
		Result:  cfg,
	})
	if err != nil {
		return fmt.Errorf("init decoder: %w", err)
	}
	if err := decoder.Decode(settings); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	return nil
}

func decodeFileRoots(raw interface{}) ([]FileRoot, error) {
	if raw == nil {
		return nil, nil
	}

	if s, ok := raw.(string); ok && s != "" {
		return parseFileRootDefinitions([]string{s})
	}

	if defs, ok := raw.([]string); ok && len(defs) > 0 {
		return parseFileRootDefinitions(defs)
	}

	var roots []FileRoot
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "mapstructure",
		Result:  &roots,
	})
	if err != nil {
		return nil, fmt.Errorf("init file root decoder: %w", err)
	}
	if err := decoder.Decode(raw); err != nil {
		return nil, fmt.Errorf("decode file roots: %w", err)
	}

	return roots, nil
}
