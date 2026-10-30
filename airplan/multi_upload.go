package airplan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

// DefaultMaxFiles is the default write-side limit on collection files and on
// user-supplied document items (entry, pages, and assets). SPEC.md §2 defines
// the count; readers never validate it.
const DefaultMaxFiles = 1000

// uploadConcurrency bounds preflight hashing and payload transfers within one
// multi-object operation, and therefore the number of Open-based inputs held
// open at once (SPEC.md §5).
const uploadConcurrency = 8

// maxFilesLimit resolves a MaxFiles value: zero selects DefaultMaxFiles and a
// negative value disables the check, reported as zero.
func maxFilesLimit(value int) int {
	switch {
	case value == 0:
		return DefaultMaxFiles
	case value < 0:
		return 0
	default:
		return value
	}
}

// runBounded runs task for every index in [0, count) on at most
// uploadConcurrency workers. The first failure cancels the context passed to
// outstanding tasks, stops scheduling new ones, and is returned once every
// started task has finished.
func runBounded(
	ctx context.Context, count int,
	task func(ctx context.Context, index int) error,
) error {
	if count <= 0 {
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		next  atomic.Int64
		once  sync.Once
		first error
		wg    sync.WaitGroup
	)
	for range min(uploadConcurrency, count) {
		wg.Go(func() {
			for {
				index := int(next.Add(1) - 1)
				if index >= count || ctx.Err() != nil {
					return
				}
				if err := task(ctx, index); err != nil {
					once.Do(func() {
						first = err
						cancel()
					})
					return
				}
			}
		})
	}
	wg.Wait()
	if first != nil {
		return first
	}
	return ctx.Err()
}

// validateInputSource enforces that exactly one of an input's Reader and Open
// fields is set. label names the input in errors.
func validateInputSource(reader io.ReadSeeker, open func() (io.ReadSeekCloser, error), label string) error {
	switch {
	case reader == nil && open == nil:
		return fmt.Errorf("airplan: %s reader is nil", label)
	case reader != nil && open != nil:
		return fmt.Errorf("airplan: %s must set only one of Reader and Open", label)
	}
	return nil
}

// acquireInput returns a reader for one input and a release function.
// Open-based inputs are opened for the caller alone and closed by release;
// Reader-based inputs belong to the caller of the library and stay open.
func acquireInput(
	reader io.ReadSeeker, open func() (io.ReadSeekCloser, error),
) (io.ReadSeeker, func(), error) {
	if open == nil {
		return reader, func() {}, nil
	}
	opened, err := open()
	if err != nil {
		return nil, func() {}, err
	}
	if opened == nil {
		return nil, func() {}, errors.New("open returned no reader")
	}
	return opened, func() { _ = opened.Close() }, nil
}

// lazyReader opens its source on the first Read and closes it at EOF, so
// inputs consumed one after another hold at most one descriptor at a time.
type lazyReader struct {
	open    func() (io.ReadCloser, error)
	current io.ReadCloser
	done    bool
}

func (r *lazyReader) Read(buffer []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	if r.current == nil {
		opened, err := r.open()
		if err != nil {
			return 0, err
		}
		r.current = opened
	}
	n, err := r.current.Read(buffer)
	if errors.Is(err, io.EOF) {
		closeErr := r.Close()
		if closeErr != nil && n == 0 {
			return 0, closeErr
		}
	}
	return n, err
}

// Close releases a partially consumed source. Later reads report EOF.
func (r *lazyReader) Close() error {
	r.done = true
	if r.current == nil {
		return nil
	}
	err := r.current.Close()
	r.current = nil
	return err
}

// LocalFile is a regular file validated once by StatLocalFile and reopened on
// demand, so callers can declare many inputs without holding a descriptor for
// each one.
type LocalFile struct {
	path  string
	label string
	info  os.FileInfo
}

// StatLocalFile validates that path names a regular file and records its
// identity and size for later Open calls. label names the input in errors,
// such as "collection input".
func StatLocalFile(path, label string) (*LocalFile, error) {
	// Stat rather than open: opening a FIFO for reading can block before the
	// configured operation timeout applies.
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("airplan: inspect %s %q: %w", label, path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("airplan: %s %q is not a regular file", label, path)
	}
	return &LocalFile{path: path, label: label, info: info}, nil
}

// Path returns the path given to StatLocalFile.
func (f *LocalFile) Path() string { return f.path }

// Size returns the byte count observed by StatLocalFile.
func (f *LocalFile) Size() int64 { return f.info.Size() }

// Open reopens the file. It fails when the path now names a different file or
// the file's size differs from the size observed by StatLocalFile.
func (f *LocalFile) Open() (io.ReadSeekCloser, error) {
	current, err := os.Stat(f.path)
	if err != nil {
		return nil, err
	}
	if !f.unchanged(current) {
		return nil, f.changedError()
	}
	file, err := os.Open(f.path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !f.unchanged(opened) {
		_ = file.Close()
		return nil, f.changedError()
	}
	return file, nil
}

// LazyReader returns a reader that opens the file on its first Read and closes
// it at EOF. Close releases a partially consumed file.
func (f *LocalFile) LazyReader() io.ReadCloser {
	return &lazyReader{open: func() (io.ReadCloser, error) { return f.Open() }}
}

func (f *LocalFile) unchanged(current os.FileInfo) bool {
	return current.Mode().IsRegular() && os.SameFile(f.info, current) &&
		current.Size() == f.info.Size()
}

func (f *LocalFile) changedError() error {
	return fmt.Errorf("airplan: %s %q changed after preflight", f.label, f.path)
}

// putVerifiedInput uploads one preflighted input after confirming that its
// bytes still match the preflight digest. Open-based inputs are held open only
// for the duration of the call. label names the input in errors, such as
// `asset "logo.png"`.
func (c *Client) putVerifiedInput(
	ctx context.Context, reader io.ReadSeeker,
	open func() (io.ReadSeekCloser, error), start int64, digest, label string,
	target streamObject,
) error {
	body, release, err := acquireInput(reader, open)
	if err != nil {
		return fmt.Errorf("airplan: open %s: %w", label, err)
	}
	defer release()
	actual, err := hashExactReader(body, start, target.Size)
	if err != nil {
		return fmt.Errorf("airplan: verify %s: %w", label, err)
	}
	if actual != digest {
		return fmt.Errorf("airplan: %s changed after preflight", label)
	}
	target.Body = body
	return c.st.putStream(ctx, target)
}

// encodeNewUploadMarker encodes the marker for a new upload or revision. An
// oversized marker is reported with its object count and limit so callers can
// tell that the upload declares too many objects (SPEC.md §5).
func encodeNewUploadMarker(marker UploadMarker) ([]byte, error) {
	body, err := EncodeUploadMarker(marker)
	if code, ok := MarkerCode(err); ok && code == MarkerErrorOversized {
		return nil, fmt.Errorf(
			"airplan: upload declares %d objects and its ownership marker exceeds the %s marker limit; upload fewer files: %w",
			len(marker.Objects),
			formatSize(int64(maxEncodedMarkerSize(marker.Version))), err,
		)
	}
	return body, err
}
