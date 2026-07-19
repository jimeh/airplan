//go:build windows

package cli

import (
	"errors"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const replaceFileWriteThrough = 0x1

var replaceFileW = windows.NewLazySystemDLL("kernel32.dll").
	NewProc("ReplaceFileW")

func renameFileAtomic(oldPath, newPath string) error {
	oldPtr, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return renameFileError(oldPath, newPath, err)
	}
	newPtr, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return renameFileError(oldPath, newPath, err)
	}

	result, _, callErr := replaceFileW.Call(
		uintptr(unsafe.Pointer(newPtr)),
		uintptr(unsafe.Pointer(oldPtr)),
		0,
		replaceFileWriteThrough,
		0,
		0,
	)
	if result != 0 {
		return nil
	}
	if errors.Is(callErr, windows.ERROR_FILE_NOT_FOUND) {
		err = windows.MoveFileEx(oldPtr, newPtr,
			windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil {
			return nil
		}
		callErr = err
	}
	if callErr == syscall.Errno(0) {
		callErr = syscall.EINVAL
	}
	return renameFileError(oldPath, newPath, callErr)
}

func renameFileError(oldPath, newPath string, err error) error {
	return &os.LinkError{
		Op: "rename", Old: oldPath, New: newPath, Err: err,
	}
}
