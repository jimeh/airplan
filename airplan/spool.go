package airplan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// payloadSpool owns mode-0600 immutable inputs and rendered outputs for one
// direct document operation. The containing directory is private as well and
// is removed deterministically by the operation that created it.
type payloadSpool struct {
	dir  string
	next int
}

type spooledPayload struct {
	path   string
	size   int64
	digest string
}

func newPayloadSpool() (*payloadSpool, error) {
	dir, err := os.MkdirTemp("", "airplan-document-*")
	if err != nil {
		return nil, fmt.Errorf("airplan: create document spool: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("airplan: secure document spool: %w", err)
	}
	return &payloadSpool{dir: dir}, nil
}

func (s *payloadSpool) put(kind string, body []byte) (spooledPayload, error) {
	return s.putReader(kind, bytes.NewReader(body), int64(len(body)))
}

func (s *payloadSpool) putReader(
	kind string, reader io.Reader, limit int64,
) (spooledPayload, error) {
	if s == nil || s.dir == "" {
		return spooledPayload{}, fmt.Errorf("airplan: %s spool is unavailable", kind)
	}
	if reader == nil {
		return spooledPayload{}, fmt.Errorf("airplan: %s spool reader is nil", kind)
	}
	name := filepath.Join(s.dir, fmt.Sprintf("%04d-%s", s.next, kind))
	s.next++
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return spooledPayload{}, fmt.Errorf("airplan: create %s spool: %w", kind, err)
	}
	hash := sha256.New()
	source := reader
	if limit > 0 {
		source = io.LimitReader(reader, limit+1)
	}
	size, writeErr := io.Copy(io.MultiWriter(file, hash), source)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(name)
		return spooledPayload{}, fmt.Errorf("airplan: write %s spool: %w", kind, writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(name)
		return spooledPayload{}, fmt.Errorf("airplan: close %s spool: %w", kind, closeErr)
	}
	if limit > 0 && size > limit {
		_ = os.Remove(name)
		return spooledPayload{}, fmt.Errorf(
			"airplan: %s exceeds maximum spool size of %s", kind, formatSize(limit),
		)
	}
	return spooledPayload{
		path: name, size: size, digest: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func (p spooledPayload) open() (*os.File, error) {
	file, err := os.Open(p.path)
	if err != nil {
		return nil, fmt.Errorf("airplan: open document spool: %w", err)
	}
	return file, nil
}

func (p spooledPayload) read(ctx context.Context) ([]byte, error) {
	file, err := p.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	body, err := readInput(ctx, file, p.size)
	if err != nil {
		return nil, fmt.Errorf("airplan: read document spool: %w", err)
	}
	if int64(len(body)) != p.size || contentSHA256(body) != p.digest {
		return nil, fmt.Errorf("airplan: document spool changed after preflight")
	}
	return body, nil
}

func (p spooledPayload) verify() (*os.File, error) {
	file, err := p.open()
	if err != nil {
		return nil, err
	}
	digest, err := hashExactReader(file, 0, p.size)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if digest != p.digest {
		_ = file.Close()
		return nil, fmt.Errorf("airplan: document spool changed after preflight")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func (s *payloadSpool) cleanup() {
	if s == nil || s.dir == "" {
		return
	}
	_ = os.RemoveAll(s.dir)
	s.dir = ""
}
