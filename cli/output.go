package cli

import (
	"io"
	"os"
	"path/filepath"
)

// writeFileAtomic writes contents to path via a temporary file in the
// same directory and an atomic rename. mode is applied to the file
// before any bytes are written; callers choose it per command —
// preview keeps shareable 0644 pages, get keeps downloads user-only
// (SPEC.md §9).
func writeFileAtomic(path string, contents []byte, mode os.FileMode) error {
	return writeFileAtomicWith(path, mode, func(w io.Writer) error {
		_, err := w.Write(contents)
		return err
	})
}

func writeFileAtomicWith(path string, mode os.FileMode,
	write func(io.Writer) error,
) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := write(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return renameFileAtomic(tmpPath, path)
}
