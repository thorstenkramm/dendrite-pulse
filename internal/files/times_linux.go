//go:build linux

package files

import (
	"syscall"
	"time"
)

func extractTimes(stat *syscall.Stat_t) (time.Time, time.Time, time.Time, *time.Time) {
	accessed := time.Unix(stat.Atim.Sec, stat.Atim.Nsec).UTC()
	modified := time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec).UTC()
	changed := time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec).UTC()
	return accessed, modified, changed, nil
}
