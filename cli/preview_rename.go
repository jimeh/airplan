//go:build !windows

package cli

import "os"

func renamePreviewAtomic(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
