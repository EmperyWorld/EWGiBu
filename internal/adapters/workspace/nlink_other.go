//go:build !unix

package workspace

import "os"

func nlinkOf(fi os.FileInfo) (uint64, bool) {
	_ = fi
	return 0, false
}
