package store

import (
	"os"
	"syscall"
)

type fileFP struct {
	size  int64
	mtime int64
	ino   uint64
}

func statFP(path string) (fileFP, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return fileFP{}, false
	}
	fp := fileFP{size: info.Size(), mtime: info.ModTime().UnixNano()}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		fp.ino = uint64(st.Ino)
	}
	return fp, true
}
