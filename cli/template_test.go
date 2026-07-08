package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTemplateCommandPrintsBuiltinTemplate(t *testing.T) {
	wantBytes, err := os.ReadFile("../airplan/assets/page.html.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	want := string(wantBytes)
	if !strings.HasSuffix(want, "\n") {
		want += "\n"
	}

	var out bytes.Buffer
	cmd := newTemplateCmd()
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != want {
		t.Errorf("output length = %d, want %d", len(got), len(want))
	}
}
