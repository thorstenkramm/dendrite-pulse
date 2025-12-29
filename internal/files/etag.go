package files

import (
	"fmt"
	"os"
	"syscall"
)

func weakETag(info os.FileInfo) string {
	mtime := info.ModTime().UTC().UnixNano()
	size := info.Size()

	stat, ok := info.Sys().(*syscall.Stat_t)
	if ok && stat.Ino != 0 {
		return fmt.Sprintf(`W/"%d-%d-%d"`, stat.Ino, size, mtime)
	}

	return fmt.Sprintf(`W/"%d-%d"`, size, mtime)
}
