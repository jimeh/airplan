package airplan

import (
	"bytes"
	"strings"
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
		"asset changed: {\"before\":{\"content_type\":\"image/png\",\"bytes\":1,\"sha256\":\"" + strings.Repeat("a", 64) + "\"},\"after\":{\"content_type\":\"image/png\",\"bytes\":1,\"sha256\":\"" + strings.Repeat("b", 64) + "\"}}\n")
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
			"--- revision-2/plan.md\n+++ revision-3/plan.md\n" +
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

func TestParseRevisionDiffReportRetainsGenerationFourAssetSection(t *testing.T) {
	body := []byte("# airplan revisions: 2 -> 3\n" +
		"# assets/screenshot.png\n" +
		"asset changed: image/png, 10 bytes, sha256 old -> image/png, 11 bytes, sha256 new\n")
	report, err := parseRevisionDiffReport(body, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.PageSections) != 0 ||
		!bytes.Contains(report.AssetSections["assets/screenshot.png"], []byte("asset changed:")) {
		t.Fatalf("report sections = %#v, %#v", report.PageSections, report.AssetSections)
	}
}

func TestParseRevisionDiffReportDoesNotTreatHunkContentAsHeaders(t *testing.T) {
	body := []byte("# airplan revisions: 2 -> 3\n" + revisionDiffFormatHeader +
		"# airplan page: \"airplan guide.md\"\n" +
		"--- revision-2/airplan guide.md\n+++ revision-3/airplan guide.md\n" +
		"@@ -1 +1 @@\n--- revision-old/source.md\n++++ revision-new/source.md\n")
	report, err := parseRevisionDiffReport(body, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(report.PageSections["airplan guide.md"], []byte("revision-new")) {
		t.Fatalf("page sections = %#v", report.PageSections)
	}
}

func TestParseRevisionDiffReportRejectsMalformedFormatTwoSections(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, test := range []struct {
		name    string
		section string
	}{
		{"noncanonical descriptor", "# airplan page: \"README.md\"\npage added: {\"lang\":\"Markdown\",\"format\":\"md\"}\n"},
		{"unknown descriptor field", "# airplan page: \"README.md\"\npage added: {\"format\":\"md\",\"lang\":\"Markdown\",\"extra\":true}\n"},
		{"added page without source diff", "# airplan page: \"README.md\"\npage added: {\"format\":\"md\",\"lang\":\"Markdown\"}\n"},
		{"removed page without source diff", "# airplan page: \"README.md\"\npage removed: {\"format\":\"md\",\"lang\":\"Markdown\"}\n"},
		{"empty metadata", "# airplan page: \"README.md\"\npage metadata changed: {}\n"},
		{"bad hunk counts", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1 +1 @@\n-old\n"},
		{"trailing hunk junk", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1 +1 @@\n-old\n+new\njunk\n"},
		{"zero coordinate with content", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -0 +1 @@\n-old\n+new\n"},
		{"noncanonical coordinate", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -01 +1 @@\n-old\n+new\n"},
		{"explicit unit counts", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1,1 +1,1 @@\n-old\n+new\n"},
		{"overlapping hunks", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1 +1 @@\n-old\n+new\n@@ -1 +1 @@\n-again\n+again-new\n"},
		{"newline marker before content", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1 +1 @@\n\\ No newline at end of file\n-old\n+new\n"},
		{"duplicate newline marker", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1 +1 @@\n-old\n\\ No newline at end of file\n\\ No newline at end of file\n+new\n"},
		{"old line after old EOF", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1,2 +1 @@\n-old\n\\ No newline at end of file\n-old-after-eof\n+new\n"},
		{"new line after new EOF", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1 +1,2 @@\n-old\n+new\n\\ No newline at end of file\n+new-after-eof\n"},
		{"context after old EOF", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1,2 +1,2 @@\n-old\n\\ No newline at end of file\n context-after-eof\n+new\n"},
		{"context after both EOFs", "# airplan page: \"README.md\"\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1,2 +1,2 @@\n context\n\\ No newline at end of file\n another-context\n"},
		{"noncanonical asset", "# airplan asset: \"image.png\"\nasset added: {\"sha256\":\"" + digest + "\",\"bytes\":1,\"content_type\":\"image/png\"}\n"},
		{"asset trailing content", "# airplan asset: \"image.png\"\nasset added: {\"content_type\":\"image/png\",\"bytes\":1,\"sha256\":\"" + digest + "\"}\ntrailing\n"},
		{"malformed page order", "# airplan page order\nbefore: [\"README.md\",\"README.md\"]\nafter: [\"README.md\"]\n"},
		{"malformed metadata", "# airplan metadata\nbefore: {\"format\":\"md\",\"title\":\"Old\"}\nafter: {\"format\":\"md\",\"title\":\"New\",\"slug\":\"plan\"}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte("# airplan revisions: 2 -> 3\n" + revisionDiffFormatHeader + test.section)
			if _, err := parseRevisionDiffReport(body, "README.md"); err == nil {
				t.Fatalf("parse accepted malformed report:\n%s", body)
			}
		})
	}
}

func TestParseRevisionDiffReportRejectsUnknownExplicitFormat(t *testing.T) {
	body := []byte("# airplan revisions: 2 -> 3\n# airplan diff format: 3\n")
	if _, err := parseRevisionDiffReport(body, "README.md"); err == nil ||
		!bytes.Contains([]byte(err.Error()), []byte("format is unsupported")) {
		t.Fatalf("error = %v, want unsupported format", err)
	}
}

func TestParseRevisionDiffReportAcceptsExplicitNoChanges(t *testing.T) {
	body := []byte("# airplan revisions: 2 -> 3\n" + revisionDiffFormatHeader +
		"No textual changes.\n")
	report, err := parseRevisionDiffReport(body, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.PageSections) != 0 || len(report.AssetSections) != 0 {
		t.Fatalf("report sections = %#v, %#v", report.PageSections, report.AssetSections)
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

func TestParseRevisionDiffReportSupportsLegacyExplicitUnitCounts(t *testing.T) {
	body := []byte("--- revision-1/plan.md\n+++ revision-2/plan.md\n@@ -1,1 +1,1 @@\n-old\n+new\n")
	report, err := parseRevisionDiffReport(body, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(report.PageSections["README.md"], body) {
		t.Fatalf("legacy section = %q", report.PageSections["README.md"])
	}
}

func TestParseRevisionDiffReportSupportsDualEOFMarkers(t *testing.T) {
	body := []byte("# airplan revisions: 2 -> 3\n" + revisionDiffFormatHeader +
		"# airplan page: \"README.md\"\n" +
		"--- revision-2/README.md\n+++ revision-3/README.md\n" +
		"@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n")
	if _, err := parseRevisionDiffReport(body, "README.md"); err != nil {
		t.Fatal(err)
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
