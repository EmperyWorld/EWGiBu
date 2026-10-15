//go:build !unix

package state

import (
	"fmt"
	"os"
)

func flockExclusive(f *os.File) error {
	_ = f
	return fmt.Errorf("operation lock requires unix flock")
}

func flockUnlock(f *os.File) error {
	_ = f
	return nil
}
