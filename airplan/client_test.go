package airplan

import (
	"context"
	"errors"
	"io"
	"mime"
	"strings"
	"testing"
)

func TestReadInput(t *testing.T) {
	t.Run("under limit", func(t *testing.T) {
		data, err := readInput(strings.NewReader("hello"), 10)
		if err != nil || string(data) != "hello" {
			t.Errorf("data = %q, err = %v", data, err)
		}
	})

	t.Run("exactly at limit", func(t *testing.T) {
		data, err := readInput(strings.NewReader("0123456789"), 10)
		if err != nil || len(data) != 10 {
			t.Errorf("len = %d, err = %v", len(data), err)
		}
	})

	t.Run("over limit", func(t *testing.T) {
		_, err := readInput(strings.NewReader("0123456789x"), 10)
		if !errors.Is(err, ErrInputTooLarge) {
			t.Errorf("err = %v, want ErrInputTooLarge", err)
		}
	})

	t.Run("unlimited", func(t *testing.T) {
		data, err := readInput(strings.NewReader("0123456789x"), 0)
		if err != nil || len(data) != 11 {
			t.Errorf("len = %d, err = %v", len(data), err)
		}
	})
}

func TestUploadRejectsOversizedInput(t *testing.T) {
	// Oversized input must fail before any key generation or storage
	// access, so a Client without a live storage backend suffices.
	c := &Client{cfg: &Config{Bucket: "b"}}

	huge := io.LimitReader(zeroReader{}, MaxInputSize+1)
	_, err := c.Upload(context.Background(), Input{Reader: huge})
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("err = %v, want ErrInputTooLarge", err)
	}
}

// zeroReader yields zero bytes forever without allocating input data.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { return len(p), nil }

func TestTitleMetadata(t *testing.T) {
	if titleMetadata("") != nil {
		t.Error("empty title should produce nil metadata")
	}

	if got := titleMetadata("Refactor auth")["title"]; got != "Refactor auth" {
		t.Errorf("ASCII title changed: %q", got)
	}

	title := "Ünïcode Tïtle ✨"
	encoded := titleMetadata(title)["title"]
	for i := 0; i < len(encoded); i++ {
		if encoded[i] > 0x7e {
			t.Fatalf("encoded title contains non-ASCII byte: %q", encoded)
		}
	}
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != title {
		t.Errorf("round-trip = %q, want %q", decoded, title)
	}
}

func TestFilenameStem(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"plan.md", "plan"},
		{"/tmp/dir/refactor-auth.html", "refactor-auth"},
		{"noext", "noext"},
		{"archive.tar.gz", "archive.tar"},
	}
	for _, tt := range tests {
		if got := filenameStem(tt.in); got != tt.want {
			t.Errorf("filenameStem(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
