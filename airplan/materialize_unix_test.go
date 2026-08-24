//go:build unix

package airplan

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestMaterializeRenderedDocumentNormalizesNestedDirectoryModes(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "preview")
	assetBody := []byte("asset")
	assets := []preparedAsset{{
		AssetInput: AssetInput{
			Reader: bytes.NewReader(assetBody), Path: "assets/images/asset.bin",
			Size: int64(len(assetBody)),
		},
		digest: contentSHA256(assetBody),
	}}

	err := func() error {
		previousUmask := syscall.Umask(0o077)
		defer syscall.Umask(previousUmask)
		return materializeRenderedDocument(context.Background(), destination,
			&RenderedDocumentBundle{Pages: []RenderedBundlePage{{
				PagePath: "docs/guides/plan.html", HTML: []byte("page"),
			}}}, assets,
		)
	}()
	if err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]os.FileMode{
		destination:                                                 0o755,
		filepath.Join(destination, "docs"):                          0o755,
		filepath.Join(destination, "docs", "guides"):                0o755,
		filepath.Join(destination, "docs", "guides", "plan.html"):   0o644,
		filepath.Join(destination, "assets"):                        0o755,
		filepath.Join(destination, "assets", "images"):              0o755,
		filepath.Join(destination, "assets", "images", "asset.bin"): 0o644,
	} {
		info, statErr := os.Stat(name)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("mode for %q = %#o, want %#o", name, got, want)
		}
	}
}
