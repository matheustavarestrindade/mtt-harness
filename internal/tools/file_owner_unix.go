//go:build unix

package tools

import (
	"os"
	"syscall"
)

func fileOwnerIDs(information os.FileInfo) (uint32, uint32, bool) {
	metadata, available := information.Sys().(*syscall.Stat_t)
	if !available {
		return 0, 0, false
	}
	return metadata.Uid, metadata.Gid, true
}
