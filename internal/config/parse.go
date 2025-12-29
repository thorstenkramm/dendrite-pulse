package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/inhies/go-bytesize"
)

// ParseMaxUploadSize parses the configured max upload size string into bytes.
func ParseMaxUploadSize(value string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("max upload size is empty")
	}
	if isDecimalDigits(trimmed) {
		parsed, err := strconv.ParseUint(trimmed, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse max upload size: %w", err)
		}
		if parsed == 0 {
			return 0, fmt.Errorf("max upload size must be positive")
		}
		if parsed > uint64(math.MaxInt64) {
			return 0, fmt.Errorf("max upload size is too large")
		}
		// #nosec G115 -- parsed is bounded by MaxInt64 above.
		return int64(parsed), nil
	}

	parsed, err := bytesize.Parse(trimmed)
	if err != nil {
		return 0, fmt.Errorf("parse max upload size: %w", err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("max upload size must be positive")
	}
	if parsed > bytesize.ByteSize(math.MaxInt64) {
		return 0, fmt.Errorf("max upload size is too large")
	}
	// #nosec G115 -- parsed is bounded by MaxInt64 above.
	return int64(parsed), nil
}

// ParseFileMode parses an octal file mode string.
func ParseFileMode(value string) (os.FileMode, error) {
	if strings.TrimSpace(value) == "" {
		return 0, fmt.Errorf("file mode is empty")
	}
	parsed, err := strconv.ParseUint(value, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("parse file mode: %w", err)
	}
	return os.FileMode(parsed), nil
}

func isDecimalDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}
