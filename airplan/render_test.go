package airplan

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func TestRenderMarkdownGolden(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "basic.md"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := RenderMarkdown(src, RenderOptions{
		Title:      "Refactor auth",
		Slug:       "refactor-auth",
		SourcePath: "./refactor-auth.md",
	})
	if err != nil {
		t.Fatal(err)
	}

	golden := filepath.Join("testdata", "basic.html")
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("rendered output differs from %s (run with -update "+
			"to refresh)", golden)
	}
}

func TestRenderMarkdownPageFeatures(t *testing.T) {
	src := []byte("# Hi\n\nsome *text*\n")

	t.Run("noindex default", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi"})
		if !strings.Contains(out,
			`<meta name="robots" content="noindex, nofollow">`) {
			t.Error("missing noindex meta tag")
		}
	})

	t.Run("indexable omits robots meta", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi", Indexable: true})
		if strings.Contains(out, `name="robots"`) {
			t.Error("robots meta present despite Indexable")
		}
	})

	t.Run("download link from SourcePath", func(t *testing.T) {
		out := render(t, src, RenderOptions{
			Title: "Hi", SourcePath: "./plan.md",
		})
		if !strings.Contains(out, `href="./plan.md" download`) {
			t.Error("missing download anchor")
		}
	})

	t.Run("no download link without SourcePath", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi"})
		if strings.Contains(out, "download") {
			t.Error("unexpected download anchor")
		}
	})

	t.Run("title is escaped", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "a <b> & c"})
		if !strings.Contains(out, "<title>a &lt;b&gt; &amp; c</title>") {
			t.Error("title not escaped")
		}
	})

	t.Run("standalone: no external refs", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi"})
		for _, frag := range []string{"<link", "<script src", "@import"} {
			if strings.Contains(out, frag) {
				t.Errorf("page references external asset: %s", frag)
			}
		}
	})

	t.Run("dark palette present", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi"})
		if strings.Count(out, "prefers-color-scheme: dark") < 2 {
			t.Error("expected dark palettes for page and syntax CSS")
		}
	})
}

func render(t *testing.T, src []byte, opts RenderOptions) string {
	t.Helper()
	out, err := RenderMarkdown(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"simple", "# Hello World\n\nbody\n", "Hello World"},
		{"not first line", "intro\n\n# Later Title\n", "Later Title"},
		{"h2 only", "## Not a title\n", ""},
		{"empty", "", ""},
		{"inline markup", "# Fix `auth` *now*\n", "Fix auth now"},
		{"setext", "Big Title\n=========\n", "Big Title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractTitle([]byte(tt.src)); got != tt.want {
				t.Errorf("ExtractTitle(%q) = %q, want %q",
					tt.src, got, tt.want)
			}
		})
	}
}

func TestResolveTitle(t *testing.T) {
	withH1 := []byte("# From H1\n")
	noH1 := []byte("plain text\n")

	tests := []struct {
		name     string
		explicit string
		src      []byte
		filename string
		slug     string
		want     string
	}{
		{"explicit wins", "Given", withH1, "plan.md", "slug", "Given"},
		{"h1 next", "", withH1, "plan.md", "slug", "From H1"},
		{"filename next", "", noH1, "auth-plan.md", "slug", "auth-plan"},
		{"slug last", "", noH1, "", "my-slug", "my-slug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveTitle(tt.explicit, tt.src, tt.filename, tt.slug)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
