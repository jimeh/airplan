package airplan

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRenderDocumentSpooledUsesPrivateFilesAndCleansUp(t *testing.T) {
	bundle, err := renderDocumentSpooled(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("# Plan\n"), Path: "plan.md",
			Format: "md",
		},
		MaxPageSize: -1, MaxTotalPageSize: -1,
	}, DocumentRenderOptions{
		RenderInputOptions:   RenderInputOptions{IncludeSource: true},
		MaxGeneratedPageSize: -1,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := bundle.spool.dir
	if len(bundle.Pages) != 1 || bundle.Pages[0].Source != nil ||
		bundle.Pages[0].HTML != nil {
		t.Fatalf("spooled bundle retained payload bytes: %+v", bundle.Pages)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); runtime.GOOS != "windows" && got != 0o700 {
		t.Fatalf("spool directory mode = %#o, want 0700", got)
	}
	for _, name := range []string{
		bundle.Pages[0].sourceFile.path,
		bundle.Pages[0].htmlFile.path,
	} {
		info, statErr := os.Stat(name)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
			t.Fatalf("spool file %q mode = %#o, want 0600", name, got)
		}
	}
	if source, readErr := bundle.Pages[0].sourceBody(context.Background()); readErr != nil || string(source) != "# Plan\n" {
		t.Fatalf("spooled source = %q, error = %v", source, readErr)
	}
	bundle.cleanup()
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spool directory remains after cleanup: %v", err)
	}
}

func TestMaterializeRenderedDocumentRemovesStagingAfterDigestFailure(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "preview")
	body := []byte("bad")
	err := materializeRenderedDocument(context.Background(), destination,
		&RenderedDocumentBundle{Pages: []RenderedBundlePage{{
			PagePath: "plan.html", HTML: []byte("page"),
		}}}, []preparedAsset{{
			AssetInput: AssetInput{
				Reader: bytes.NewReader(body), Path: "asset.bin",
				Size: int64(len(body)),
			},
			digest: contentSHA256([]byte("different")),
		}},
	)
	if err == nil || !strings.Contains(err.Error(), "changed after preflight") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed preview destination exists: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".airplan-preview-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("preview staging remains after failure: %v", matches)
	}
}
