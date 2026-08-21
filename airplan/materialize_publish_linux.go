//go:build linux

package airplan

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func publishDirectoryNoReplace(source, destination string) error {
	err := unix.Renameat2(
		unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE,
	)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) ||
		errors.Is(err, unix.EOPNOTSUPP) {
		return fmt.Errorf("exclusive no-replace directory publication is unsupported by this filesystem: %w", err)
	}
	return err
}
