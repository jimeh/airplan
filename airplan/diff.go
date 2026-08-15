package airplan

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// GenerateRevisionDiff returns a deterministic three-context unified diff of
// the exact UTF-8 Markdown sources. Headers use revision identities rather
// than caller-local paths.
func GenerateRevisionDiff(previous, current []byte, previousRevision, currentRevision int) ([]byte, error) {
	return generateRevisionDiff(previous, current, previousRevision, currentRevision, MaxDiffSize)
}

func generateRevisionDiff(
	previous, current []byte, previousRevision, currentRevision, maxSize int,
) ([]byte, error) {
	if previousRevision <= 0 || currentRevision <= previousRevision {
		return nil, fmt.Errorf("airplan: ordered positive revision numbers are required")
	}
	newlineMarker := missingNewlineMarker(previous, current)
	previousLines := revisionDiffLines(previous, newlineMarker)
	currentLines := revisionDiffLines(current, newlineMarker)
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        previousLines,
		B:        currentLines,
		FromFile: fmt.Sprintf("revision-%d/plan.md", previousRevision),
		ToFile:   fmt.Sprintf("revision-%d/plan.md", currentRevision),
		Context:  3,
		Eol:      "\n",
	})
	if err != nil {
		return nil, fmt.Errorf("airplan: generate revision diff: %w", err)
	}
	// The dependency emits LF with Eol above; normalize defensively so output
	// remains byte-identical when callers provide CRLF source.
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	diff = strings.ReplaceAll(diff, newlineMarker+"\n", "\n\\ No newline at end of file\n")
	if len(diff) == 0 {
		return nil, errors.New("airplan: identical sources do not have a revision diff")
	}
	if len(diff) > maxSize {
		return nil, fmt.Errorf("airplan: generated revision diff is %d bytes; maximum is %d",
			len(diff), maxSize)
	}
	return []byte(diff), nil
}

func missingNewlineMarker(sources ...[]byte) string {
	marker := "\x00airplan-missing-final-newline\x00"
	for suffix := 1; ; suffix++ {
		found := false
		for _, source := range sources {
			if strings.Contains(string(source), marker) {
				found = true
				break
			}
		}
		if !found {
			return marker
		}
		marker = fmt.Sprintf("\x00airplan-missing-final-newline-%d\x00", suffix)
	}
}

func revisionDiffLines(source []byte, newlineMarker string) []string {
	if len(source) == 0 {
		return nil
	}
	missing := source[len(source)-1] != '\n'
	lines := strings.SplitAfter(string(source), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if missing {
		lines[len(lines)-1] += newlineMarker + "\n"
	}
	return lines
}
