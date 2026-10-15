//go:build !unix

package workspace

import (
	"io"
	"os"
)

func openNoFollow(abs string) (io.ReadCloser, error) {
	// Best-effort: Lstat was already checked by callers for non-symlink.
	return os.Open(abs)
}
