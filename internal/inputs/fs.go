package inputs

import (
	"io"
	"io/fs"
)

// FileInfo is the subset of file metadata needed for safe input resolution.
type FileInfo interface {
	Name() string
	Size() int64
	Mode() fs.FileMode
	IsDir() bool
	Nlink() uint64 // hard-link count; 0 if unknown
}

// DirEntry is one directory entry from ReadDir.
type DirEntry interface {
	Name() string
	IsDir() bool
	Type() fs.FileMode
	Info() (FileInfo, error)
}

// FS is a rooted filesystem used only for declared workspace inputs.
// Lstat and ReadDir must not follow symlinks. Open must not follow a symlink
// at the named path. FollowSymlink resolves a link chain and returns a
// workspace-relative target, or an error if any target leaves the workspace.
type FS interface {
	Lstat(rel string) (FileInfo, error)
	ReadDir(rel string) ([]DirEntry, error)
	Open(rel string) (io.ReadCloser, error)
	ReadFile(rel string) ([]byte, error)
	FollowSymlink(rel string) (string, error)
}
