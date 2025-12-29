//go:build linux

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

	atime := time.Unix(stat.Atim.Sec, stat.Atim.Nsec).UTC()
	mtime := time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec).UTC()
	ctime := time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec).UTC()
	return &atime, &mtime, &ctime, nil
}
