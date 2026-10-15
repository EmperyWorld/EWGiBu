//go:build !unix

package provider

import (
	"context"
	"fmt"
)

// DiskAvailableBytes implements ReadOnly.
func (h Host) DiskAvailableBytes(ctx context.Context, path string) (uint64, error) {
	_ = ctx
	_ = h
	_ = path
	return 0, fmt.Errorf("disk probe unsupported on this platform")
}
