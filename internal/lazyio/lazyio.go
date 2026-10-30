// Package lazyio provides a reader that opens its source on first use.
package lazyio

import (
	"errors"
	"io"
	"sync"
)

// ErrClosed reports a read from a Reader closed before its source reached EOF.
var ErrClosed = errors.New("lazy reader is closed")

// Reader opens its source on the first Read and closes it at EOF, so inputs
// consumed one after another hold at most one descriptor at a time. Close may
// run concurrently with an in-flight Read, as when a canceled operation stops
// waiting for its reading goroutine: a source opened after Close is closed
// immediately rather than leaked. Reads themselves are not concurrent-safe,
// as with any io.Reader.
type Reader struct {
	open func() (io.ReadCloser, error)

	mu      sync.Mutex
	current io.ReadCloser
	closed  bool
	eof     bool
}

// New returns a Reader over the source returned by open.
func New(open func() (io.ReadCloser, error)) *Reader {
	return &Reader{open: open}
}

func (r *Reader) Read(buffer []byte) (int, error) {
	r.mu.Lock()
	if r.closed {
		eof := r.eof
		r.mu.Unlock()
		if eof {
			return 0, io.EOF
		}
		return 0, ErrClosed
	}
	current := r.current
	r.mu.Unlock()
	if current == nil {
		opened, err := r.open()
		if err != nil {
			return 0, err
		}
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			_ = opened.Close()
			return 0, ErrClosed
		}
		r.current = opened
		r.mu.Unlock()
		current = opened
	}
	n, err := current.Read(buffer)
	if errors.Is(err, io.EOF) {
		closeErr := r.release(true)
		if closeErr != nil && n == 0 {
			return 0, closeErr
		}
	}
	return n, err
}

// Close releases a partially consumed source. Later reads report ErrClosed,
// or io.EOF when the source had already been read to its end.
func (r *Reader) Close() error {
	return r.release(false)
}

func (r *Reader) release(eof bool) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.eof = eof
	current := r.current
	r.current = nil
	r.mu.Unlock()
	if current == nil {
		return nil
	}
	return current.Close()
}
