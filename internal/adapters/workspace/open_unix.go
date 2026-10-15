//go:build unix

package workspace

import (
	"io"
	"os"
	"syscall"
)

func openNoFollow(abs string) (io.ReadCloser, error) {
	fd, err := syscall.Open(abs, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), abs), nil
}
