package main

import (
	"fmt"
	"os"

	"github.com/jimeh/airplan/airplan"
)

func main() {
	if len(os.Args) != 2 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: browser-fixture OUTPUT")
		os.Exit(2)
	}
	diff := "--- revision-1/plan.md\n+++ revision-2/plan.md\n@@ -1 +1 @@\n-Original\n+Revised\n"
	page, err := airplan.RenderMarkdown([]byte("# Browser revision\n\nRevised\n"), airplan.RenderOptions{
		Title: "Browser revision", Slug: "plan", SourceName: "plan.md",
		SourcePath: "./plan.md", NoExternalAssets: true,
		Revision: 2, RevisionCount: 2, PreviousRevision: 1,
		RevisionChainID: "ssssssssssssssssssssssssss",
		VersionsPath:    airplan.VersionsFilename,
		DiffPath:        "./" + airplan.DiffFilename,
		DiffText:        diff,
		Pages: []airplan.DocumentTemplatePage{
			{Path: "plan.md", Title: "Browser revision", URL: "plan.html", Current: true},
			{Path: "notes.md", Title: "Notes", URL: "notes.html"},
		},
		CurrentPage: airplan.DocumentTemplatePage{
			Path: "plan.md", Title: "Browser revision", URL: "plan.html", Current: true,
		},
		Entrypoint: "plan.html",
	})
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(os.Args[1], page, 0o644); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
