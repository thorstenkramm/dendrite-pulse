//go:build !darwin && !linux

package files

import (
	"syscall"
	"time"
)

func extractTimes(stat *syscall.Stat_t) (time.Time, time.Time, time.Time, *time.Time) {
	var zero time.Time
	return zero, zero, zero, nil
}
