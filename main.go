// Command airplan uploads AI/LLM agent plan files (markdown or HTML)
// to S3-compatible object storage under a randomized, unguessable URL
// path and prints the resulting URL. Behavior is defined by SPEC.md.
package main

import (
	"os"

	"github.com/jimeh/airplan/cli"
)

func main() {
	os.Exit(cli.Execute())
}
