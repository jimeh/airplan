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
	previousLines, previousMissingNewline := revisionDiffLines(previous)
	currentLines, currentMissingNewline := revisionDiffLines(current)
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
	if previousMissingNewline && len(previousLines) > 0 {
		diff = annotateMissingFinalNewline(diff, "-", previousLines[len(previousLines)-1])
	}
	if currentMissingNewline && len(currentLines) > 0 {
		diff = annotateMissingFinalNewline(diff, "+", currentLines[len(currentLines)-1])
	}
	if len(diff) == 0 {
		return nil, errors.New("airplan: identical sources do not have a revision diff")
	}
	if len(diff) > maxSize {
		return nil, fmt.Errorf("airplan: generated revision diff is %d bytes; maximum is %d",
			len(diff), maxSize)
	}
	return []byte(diff), nil
}

func revisionDiffLines(source []byte) ([]string, bool) {
	if len(source) == 0 {
		return nil, false
	}
	missing := source[len(source)-1] != '\n'
	lines := strings.SplitAfter(string(source), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if missing {
		lines[len(lines)-1] += "\n"
	}
	return lines, missing
}

func annotateMissingFinalNewline(diff, prefix, line string) string {
	needle := prefix + line
	index := strings.LastIndex(diff, needle)
	if index < 0 {
		return diff
	}
	end := index + len(needle)
	return diff[:end] + "\\ No newline at end of file\n" + diff[end:]
}
