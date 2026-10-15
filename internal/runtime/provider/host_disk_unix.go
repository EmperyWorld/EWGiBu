//go:build unix

package provider

import (
	"context"
	"syscall"
)

func diskAvailable(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

// DiskAvailableBytes implements ReadOnly.
func (h Host) DiskAvailableBytes(ctx context.Context, path string) (uint64, error) {
	_ = ctx
	_ = h
	return diskAvailable(path)
}
