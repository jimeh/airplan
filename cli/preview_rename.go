//go:build !windows

package cli

import "os"

func renameFileAtomic(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
