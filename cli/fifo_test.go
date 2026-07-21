//go:build !windows

package cli

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSelectCollectionModeRejectsFIFOWithoutBlocking(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "capture.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := selectCollectionMode([]string{fifo}, &rootOptions{})
		result <- err
	}()

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("collection mode detection blocked opening a FIFO")
	}
}
