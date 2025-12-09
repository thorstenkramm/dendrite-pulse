package config

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

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
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
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
	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal config: %w", err)
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
