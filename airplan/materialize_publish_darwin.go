//go:build darwin

package airplan

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func publishDirectoryNoReplace(source, destination string) error {
	err := unix.RenamexNp(source, destination, unix.RENAME_EXCL)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOTSUP) {
		return fmt.Errorf("exclusive no-replace directory publication is unsupported by this filesystem: %w", err)
	}
	return err
}
