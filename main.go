// Command airplan renders and uploads AI/LLM plan documents, with a
// local preview path that does not require object storage. Behavior is
// defined by SPEC.md.
package main

import (
	"os"

	"github.com/jimeh/airplan/cli"
)

func main() {
	os.Exit(cli.Execute())
}
