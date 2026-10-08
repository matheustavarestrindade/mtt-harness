//go:build unix

package workspaceread

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// No-follow prevents a final-link race; nonblocking avoids waiting on a FIFO
// substituted for a regular file between directory enumeration and open.
func openEntry(root *os.Root, path string) (*os.File, error) {
	path = filepath.Clean(path)
	if !filepath.IsLocal(path) {
		return nil, fmt.Errorf("workspace path is not local")
	}
	parent, operationError := root.Open(".")
	if operationError != nil || path == "." {
		return parent, operationError
	}
	components := strings.Split(path, string(filepath.Separator))
	for index, component := range components {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if index < len(components)-1 {
			flags |= unix.O_DIRECTORY
		}
		descriptor, operationError := unix.Openat(int(parent.Fd()), component, flags, 0)
		parent.Close()
		if operationError != nil {
			return nil, operationError
		}
		parent = os.NewFile(uintptr(descriptor), component)
	}
	return parent, nil
}
