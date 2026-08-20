package airplan

import (
	"bytes"
	"testing"
)

func TestRevisionDiffRangeAcceptsBundleDiffs(t *testing.T) {
	for _, test := range []struct {
		name string
		diff string
	}{
		{
			name: "textual page change",
			diff: "# airplan revisions: 3 -> 4\n\n## page: README.md\n--- revision-3/README.md\n+++ revision-4/README.md\n",
		},
		{
			name: "asset only change",
			diff: "# airplan revisions: 3 -> 4\n\nNo textual page changes.\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			previous, current, err := revisionDiffRange([]byte(test.diff))
			if err != nil {
				t.Fatal(err)
			}
			if previous != 3 || current != 4 {
				t.Fatalf("range = %d -> %d, want 3 -> 4", previous, current)
			}
		})
	}
}

func TestParseRevisionDiffReportProjectsLogicalPages(t *testing.T) {
	body := []byte("# airplan revisions: 2 -> 3\n" +
		"# airplan page order\nbefore: [\"README.md\"]\nafter: [\"README.md\",\"server.go\"]\n" +
		"# README.md\npage metadata changed: {\"title\":{\"before\":\"Old\",\"after\":\"New\"}}\n" +
		"# server.go\npage added: {\"format\":\"txt\",\"title\":\"Server\",\"lang\":\"Go\"}\n" +
		"--- revision-2/server.go\n+++ revision-3/server.go\n@@ -0,0 +1 @@\n+package main\n")
	report, err := parseRevisionDiffReport(body, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if report.Previous != 2 || report.Current != 3 || !bytes.Equal(report.Body, body) {
		t.Fatalf("report identity = %+v", report)
	}
	if len(report.PageSections) != 2 ||
		!bytes.Contains(report.PageSections["README.md"], []byte("metadata changed")) ||
		!bytes.Contains(report.PageSections["server.go"], []byte("revision-3/server.go")) {
		t.Fatalf("page sections = %#v", report.PageSections)
	}
	if bytes.Contains(report.PageSections["README.md"], []byte("server.go")) {
		t.Fatal("entry projection contains unrelated page changes")
	}
}

func TestParseRevisionDiffReportExplicitSectionsDisambiguateAirplanPath(t *testing.T) {
	body := []byte("# airplan revisions: 2 -> 3\n" + revisionDiffFormatHeader +
		"# airplan page: \"airplan guide.md\"\n" +
		"--- revision-2/airplan guide.md\n+++ revision-3/airplan guide.md\n" +
		"@@ -1 +1 @@\n-old\n+new\n" +
		"# airplan asset: \"airplan screenshot.png\"\n" +
		"asset changed: image/png, 1 bytes, sha256 old -> image/png, 1 bytes, sha256 new\n")
	report, err := parseRevisionDiffReport(body, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(report.PageSections["airplan guide.md"], []byte("+new")) {
		t.Fatalf("page sections = %#v", report.PageSections)
	}
	if !bytes.Contains(report.AssetSections["airplan screenshot.png"], []byte("asset changed")) {
		t.Fatalf("asset sections = %#v", report.AssetSections)
	}
}

func TestParseRevisionDiffReportRejectsMismatchedNestedHeaders(t *testing.T) {
	for _, headers := range []string{
		"--- revision-2/other.md\n+++ revision-3/airplan guide.md\n",
		"--- revision-1/airplan guide.md\n+++ revision-3/airplan guide.md\n",
	} {
		body := []byte("# airplan revisions: 2 -> 3\n" + revisionDiffFormatHeader +
			"# airplan page: \"airplan guide.md\"\n" + headers +
			"@@ -1 +1 @@\n-old\n+new\n")
		if _, err := parseRevisionDiffReport(body, "README.md"); err == nil ||
			!bytes.Contains([]byte(err.Error()), []byte("mismatched unified headers")) {
			t.Fatalf("headers = %q, error = %v, want mismatched unified headers", headers, err)
		}
	}
}

func TestParseRevisionDiffReportRetainsGenerationFourAirplanPath(t *testing.T) {
	for _, logical := range []string{"airplan guide.md", "airplan metadata"} {
		body := []byte("# airplan revisions: 2 -> 3\n" +
			"# " + logical + "\n" +
			"--- revision-2/" + logical + "\n+++ revision-3/" + logical + "\n" +
			"@@ -1 +1 @@\n-old\n+new\n")
		report, err := parseRevisionDiffReport(body, "README.md")
		if err != nil {
			t.Fatal(err)
		}
		if len(report.PageSections[logical]) == 0 {
			t.Fatalf("path %q, page sections = %#v", logical, report.PageSections)
		}
	}
}

func TestParseRevisionDiffReportRejectsUnknownExplicitFormat(t *testing.T) {
	body := []byte("# airplan revisions: 2 -> 3\n# airplan diff format: 3\n")
	if _, err := parseRevisionDiffReport(body, "README.md"); err == nil ||
		!bytes.Contains([]byte(err.Error()), []byte("format is unsupported")) {
		t.Fatalf("error = %v, want unsupported format", err)
	}
}

func TestParseRevisionDiffReportSupportsLegacySinglePageDiff(t *testing.T) {
	body := []byte("--- revision-1/plan.md\n+++ revision-2/plan.md\n@@ -1 +1 @@\n-old\n+new\n")
	report, err := parseRevisionDiffReport(body, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(report.PageSections["README.md"], body) {
		t.Fatalf("legacy section = %q", report.PageSections["README.md"])
	}
}

func TestInlinePageDiffsAppliesLimitPerSection(t *testing.T) {
	exact := bytes.Repeat([]byte("x"), MaxInlineDiffSize)
	over := bytes.Repeat([]byte("y"), MaxInlineDiffSize+1)
	inline, changed := inlinePageDiffs(&revisionDiffReport{PageSections: map[string][]byte{
		"exact.md": exact,
		"over.md":  over,
	}}, []RenderedBundlePage{{Path: "exact.md"}, {Path: "over.md"}, {Path: "stable.md"}})
	if len(inline["exact.md"]) != MaxInlineDiffSize || inline["over.md"] != "" {
		t.Fatalf("inline lengths = exact %d, over %d", len(inline["exact.md"]), len(inline["over.md"]))
	}
	if !changed["exact.md"] || !changed["over.md"] || changed["stable.md"] {
		t.Fatalf("changed pages = %#v", changed)
	}
}
