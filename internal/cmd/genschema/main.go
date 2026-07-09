package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jimeh/airplan/airplan"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: genschema OUTPUT")
		os.Exit(2)
	}

	out, err := airplan.ConfigSchema()
	if err != nil {
		fatal(err)
	}

	path := os.Args[1]
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "genschema: %s\n", err)
	os.Exit(1)
}
