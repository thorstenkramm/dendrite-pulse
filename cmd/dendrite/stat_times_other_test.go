//go:build !darwin && !linux

package main

import (
	"os"
	"time"
)

func statTimes(_ os.FileInfo) (*time.Time, *time.Time, *time.Time, *time.Time) {
	return nil, nil, nil, nil
}
