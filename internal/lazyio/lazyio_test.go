package lazyio

import (
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

type trackedSource struct {
	io.Reader
	closed atomic.Bool
}

func (s *trackedSource) Close() error {
	s.closed.Store(true)
	return nil
}

func TestReaderOpensOnFirstReadAndClosesAtEOF(t *testing.T) {
	source := &trackedSource{Reader: strings.NewReader("body")}
	opens := 0
	reader := New(func() (io.ReadCloser, error) {
		opens++
		return source, nil
	})
	if opens != 0 {
		t.Fatal("opened before the first read")
	}
	body, err := io.ReadAll(reader)
	if err != nil || string(body) != "body" {
		t.Fatalf("body = %q, %v", body, err)
	}
	if opens != 1 || !source.closed.Load() {
		t.Fatalf("opens = %d, closed = %t", opens, source.closed.Load())
	}
	if n, err := reader.Read(make([]byte, 1)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("read after EOF = %d, %v", n, err)
	}
}

// Close during an in-flight open must not leak the source the opener later
// returns, and the abandoned read must fail rather than report clean data.
func TestReaderCloseDuringOpenClosesLateSource(t *testing.T) {
	source := &trackedSource{Reader: strings.NewReader("body")}
	entered := make(chan struct{})
	release := make(chan struct{})
	reader := New(func() (io.ReadCloser, error) {
		close(entered)
		<-release
		return source, nil
	})
	result := make(chan error, 1)
	go func() {
		_, err := reader.Read(make([]byte, 4))
		result <- err
	}()
	<-entered
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-result; !errors.Is(err, ErrClosed) {
		t.Fatalf("in-flight read error = %v, want ErrClosed", err)
	}
	if !source.closed.Load() {
		t.Fatal("source opened after Close was leaked")
	}
}
