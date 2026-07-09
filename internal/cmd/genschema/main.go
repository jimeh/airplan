package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jimeh/airplan/airplan"
)

// main writes the generated config schema, or verifies an existing copy.
func main() {
	checkMode := false
	path := ""
	switch {
	case len(os.Args) == 2:
		path = os.Args[1]
	case len(os.Args) == 3 && os.Args[1] == "--check":
		checkMode = true
		path = os.Args[2]
	default:
		fmt.Fprintln(os.Stderr, "usage: genschema [--check] OUTPUT")
		os.Exit(2)
	}

	out, err := airplan.ConfigSchema()
	if err != nil {
		fatal(err)
	}

	if checkMode {
		check(path, out)
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		fatal(err)
	}
}

// check compares the committed schema against freshly generated output.
func check(path string, want []byte) {
	got, err := os.ReadFile(path)
	if err != nil {
		fatal(err)
	}
	if !bytes.Equal(got, want) {
		fmt.Fprintf(os.Stderr,
			"%s is stale; run mise run generate\n", path)
		os.Exit(1)
	}
}

// fatal reports a generator error and exits unsuccessfully.
func fatal(err error) {
	fmt.Fprintf(os.Stderr, "genschema: %s\n", err)
	os.Exit(1)
}
