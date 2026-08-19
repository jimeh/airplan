//go:build darwin

package airplan

import "golang.org/x/sys/unix"

func publishDirectoryNoReplace(source, destination string) error {
	return unix.RenamexNp(source, destination, unix.RENAME_EXCL)
}
