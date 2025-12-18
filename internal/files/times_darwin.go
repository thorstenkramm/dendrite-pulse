//go:build darwin

package files

import (
	"syscall"
	"time"
)

func extractTimes(stat *syscall.Stat_t) (time.Time, time.Time, time.Time, *time.Time) {
	accessed := time.Unix(stat.Atimespec.Sec, stat.Atimespec.Nsec).UTC()
	modified := time.Unix(stat.Mtimespec.Sec, stat.Mtimespec.Nsec).UTC()
	changed := time.Unix(stat.Ctimespec.Sec, stat.Ctimespec.Nsec).UTC()
	born := time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec).UTC()
	return accessed, modified, changed, &born
}
