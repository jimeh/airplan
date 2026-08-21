package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jimeh/airplan/airplan"
)

func main() {
	if len(os.Args) != 2 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: browser-fixture OUTPUT")
		os.Exit(2)
	}
	outputPath := os.Args[1]
	entryURL := "plan.html"
	diff := "--- revision-1/plan.md\n+++ revision-2/plan.md\n@@ -1 +1 @@\n-Original\n+Revised\n"
	pages := []airplan.DocumentTemplatePage{
		{Path: "plan.md", Title: "Browser revision", URL: entryURL, Current: true},
		{Path: "notes.md", Title: "Notes", URL: "notes.html"},
	}
	page, err := airplan.RenderMarkdown([]byte("# Browser revision\n\nRevised\n"), airplan.RenderOptions{
		Title: "Browser revision", Slug: "plan", SourceName: "plan.md",
		SourcePath: "./plan.md", NoExternalAssets: true,
		Revision: 2, RevisionCount: 2, PreviousRevision: 1,
		RevisionChainID: "ssssssssssssssssssssssssss",
		VersionsPath:    airplan.VersionsFilename,
		DiffPath:        "./" + airplan.DiffFilename,
		DiffText:        diff,
		Pages:           pages,
		CurrentPage: airplan.DocumentTemplatePage{
			Path: "plan.md", Title: "Browser revision", URL: entryURL, Current: true,
		},
		Entrypoint: entryURL,
	})
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	notesPages := []airplan.DocumentTemplatePage{
		{Path: "plan.md", Title: "Browser revision", URL: entryURL},
		{Path: "notes.md", Title: "Notes", URL: "notes.html", Current: true},
	}
	notes, err := airplan.RenderMarkdown([]byte("# Notes\n\nSupporting revision fixture page.\n"), airplan.RenderOptions{
		Title: "Notes", Slug: "notes", SourceName: "notes.md",
		SourcePath: "./notes.md", NoExternalAssets: true,
		Revision: 2, RevisionCount: 2, PreviousRevision: 1,
		RevisionChainID: "ssssssssssssssssssssssssss",
		VersionsPath:    airplan.VersionsFilename,
		Pages:           notesPages,
		CurrentPage:     notesPages[1],
		Entrypoint:      entryURL,
	})
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputPath, page, 0o644); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(outputPath), "notes.html"), notes, 0o644); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
