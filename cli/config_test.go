package cli

import (
	"bytes"
	"io"
	"testing"

	"github.com/jimeh/airplan/airplan"
)

func TestConfigSchemaCmd(t *testing.T) {
	want, err := airplan.ConfigSchema()
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newConfigCmd()
	cmd.SetArgs([]string{"schema"})
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("output differs from ConfigSchema()")
	}
}
