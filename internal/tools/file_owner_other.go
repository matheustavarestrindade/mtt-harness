//go:build !unix

package tools

import "os"

func fileOwnerIDs(os.FileInfo) (uint32, uint32, bool) { return 0, 0, false }
