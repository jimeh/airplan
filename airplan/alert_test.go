package airplan

import (
	"bytes"
	"strings"
	"testing"
)

func TestMarkdownAlerts(t *testing.T) {
	for _, kind := range []string{
		"NOTE",
		"TIP",
		"IMPORTANT",
		"WARNING",
		"CAUTION",
	} {
		t.Run(kind, func(t *testing.T) {
			src := []byte("> [!" + kind + "]\n> Alert body.\n")
			out := renderMarkdownBody(t, src)

			class := "markdown-alert-" + strings.ToLower(kind)
			if !strings.Contains(out, class) {
				t.Errorf("output missing alert class %q:\n%s", class, out)
			}
			if strings.Contains(out, "[!"+kind+"]") {
				t.Errorf("output retained alert marker:\n%s", out)
			}
			if !strings.Contains(out, "<p>Alert body.</p>") {
				t.Errorf("output missing alert body:\n%s", out)
			}
		})
	}
}

func TestMarkdownAlertSupportsBlockContent(t *testing.T) {
	src := []byte(strings.Join([]string{
		"> [!NOTE]",
		"> First paragraph.",
		">",
		"> - one",
		"> - two",
		">",
		"> ```go",
		"> package main",
		"> ```",
		"",
	}, "\n"))
	out := renderMarkdownBody(t, src)

	for _, fragment := range []string{
		`class="markdown-alert markdown-alert-note"`,
		"<p>First paragraph.</p>",
		"<ul>",
		"<li>one</li>",
		`<span class="kn">package</span>`,
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("alert output missing %q:\n%s", fragment, out)
		}
	}
}

func TestConsecutiveMarkdownAlerts(t *testing.T) {
	src := []byte(strings.Join([]string{
		"> [!NOTE]",
		"> One.",
		"",
		"> [!WARNING]",
		"> Two.",
		"",
		"> [!CAUTION]",
		"> Three.",
		"",
	}, "\n"))
	out := renderMarkdownBody(t, src)

	for _, kind := range []string{"note", "warning", "caution"} {
		class := "markdown-alert-" + kind
		if !strings.Contains(out, class) {
			t.Errorf("output missing consecutive alert %q:\n%s", class, out)
		}
	}
}

func TestUnknownMarkdownAlertRemainsBlockquote(t *testing.T) {
	out := renderMarkdownBody(t, []byte("> [!CUSTOM]\n> Body.\n"))
	if !strings.Contains(out, "<blockquote>") ||
		!strings.Contains(out, "[!CUSTOM]") {
		t.Errorf("unknown alert did not remain a blockquote:\n%s", out)
	}
	if strings.Contains(out, "markdown-alert") {
		t.Errorf("unknown alert rendered as an alert:\n%s", out)
	}
}

func renderMarkdownBody(t *testing.T, src []byte) string {
	t.Helper()
	var out bytes.Buffer
	if err := newMarkdown().Convert(src, &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
