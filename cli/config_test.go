package cli

import (
	"bytes"
	"io"
	"strings"
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

func TestConfigSchemaCmdRejectsArguments(t *testing.T) {
	cmd := newConfigCmd()
	cmd.SetArgs([]string{"schema", "extra"})
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown command") &&
		!strings.Contains(err.Error(), "accepts 0 arg") {
		t.Fatalf("error = %v, want argument rejection", err)
	}
}
