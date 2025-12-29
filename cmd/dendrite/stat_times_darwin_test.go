//go:build darwin

package main

import (
	"os"
	"syscall"
	"time"
)

func statTimes(info os.FileInfo) (*time.Time, *time.Time, *time.Time, *time.Time) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, nil, nil, nil
	}

	atime := time.Unix(stat.Atimespec.Sec, stat.Atimespec.Nsec).UTC()
	mtime := time.Unix(stat.Mtimespec.Sec, stat.Mtimespec.Nsec).UTC()
	ctime := time.Unix(stat.Ctimespec.Sec, stat.Ctimespec.Nsec).UTC()
	btime := time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec).UTC()
	return &atime, &mtime, &ctime, &btime
}
