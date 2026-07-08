package airplan

import (
	"mime"
	"testing"
)

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
