package airplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
	return generateRevisionDiffForPath(
		previous, current, previousRevision, currentRevision, "plan.md", maxSize,
	)
}

func generateRevisionDiffForPath(
	previous, current []byte, previousRevision, currentRevision int,
	logicalPath string, maxSize int,
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
		FromFile: fmt.Sprintf("revision-%d/%s", previousRevision, logicalPath),
		ToFile:   fmt.Sprintf("revision-%d/%s", currentRevision, logicalPath),
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

// revisionDiffReport is the canonical adjacent bundle report plus its parsed
// page-local projections. Body is written unchanged to .airplan-changes.diff.
type revisionDiffReport struct {
	Previous      int
	Current       int
	Body          []byte
	PageSections  map[string][]byte
	AssetSections map[string][]byte
}

const revisionDiffFormatHeader = "# airplan diff format: 2\n"

func parseRevisionDiffReport(body []byte, entryPath string) (*revisionDiffReport, error) {
	if !bytes.HasPrefix(body, []byte("# airplan revisions: ")) {
		previous, current, err := revisionDiffRange(body)
		if err != nil {
			return nil, err
		}
		if entryPath == "" {
			return nil, errors.New("airplan: legacy revision diff entry path is empty")
		}
		report := &revisionDiffReport{
			Previous: previous, Current: current, Body: append([]byte(nil), body...),
			PageSections:  map[string][]byte{entryPath: append([]byte(nil), body...)},
			AssetSections: make(map[string][]byte),
		}
		if err := validateRevisionPageSection(body, "plan.md", previous, current, false); err != nil {
			return nil, err
		}
		return report, nil
	}
	previous, current, err := revisionDiffRange(body)
	if err != nil {
		return nil, err
	}
	report := &revisionDiffReport{
		Previous: previous, Current: current, Body: append([]byte(nil), body...),
		PageSections:  make(map[string][]byte),
		AssetSections: make(map[string][]byte),
	}
	headerEnd := bytes.IndexByte(body, '\n')
	if headerEnd < 0 {
		return nil, errors.New("airplan: revision diff range is missing a newline")
	}
	rest := body[headerEnd+1:]
	explicit := bytes.HasPrefix(rest, []byte(revisionDiffFormatHeader))
	if !explicit && bytes.HasPrefix(rest, []byte("# airplan diff format: ")) {
		return nil, errors.New("airplan: revision diff format is unsupported")
	}
	if explicit {
		rest = rest[len(revisionDiffFormatHeader):]
	}
	seenGlobals := make(map[string]struct{})
	for len(rest) > 0 {
		if bytes.HasPrefix(rest, []byte("No textual changes.\n")) {
			if len(rest) != len("No textual changes.\n") {
				return nil, errors.New("airplan: revision diff has trailing content")
			}
			break
		}
		if !bytes.HasPrefix(rest, []byte("# ")) {
			return nil, errors.New("airplan: revision diff section header is invalid")
		}
		lineEnd := bytes.IndexByte(rest, '\n')
		if lineEnd < 0 {
			return nil, errors.New("airplan: revision diff section header is incomplete")
		}
		name := string(rest[2:lineEnd])
		if name == "" {
			return nil, errors.New("airplan: revision diff section name is empty")
		}
		nextOffset := bytes.Index(rest[lineEnd+1:], []byte("\n# "))
		sectionEnd := len(rest)
		if nextOffset >= 0 {
			sectionEnd = lineEnd + 1 + nextOffset + 1
		}
		section := append([]byte(nil), rest[:sectionEnd]...)
		kind, logical, headerErr := parseRevisionDiffSectionHeader(name, explicit)
		if headerErr != nil {
			return nil, headerErr
		}
		if !explicit && kind == "global" {
			if legacyKind := legacyRevisionDiffPathKind(section); legacyKind != "" {
				kind, logical = legacyKind, name
			}
		}
		if explicit && (kind == "page" || kind == "asset") {
			if err := validateExplicitRevisionPathSection(section, kind, logical); err != nil {
				return nil, err
			}
		}
		switch kind {
		case "global":
			if _, exists := seenGlobals[name]; exists {
				return nil, fmt.Errorf("airplan: duplicate revision diff section %q", name)
			}
			seenGlobals[name] = struct{}{}
		case "page":
			if _, exists := report.PageSections[logical]; exists {
				return nil, fmt.Errorf("airplan: duplicate revision diff page section %q", logical)
			}
			if err := validateRevisionPageSection(section, logical, previous, current, true); err != nil {
				return nil, err
			}
			report.PageSections[logical] = section
		case "asset":
			if _, exists := report.AssetSections[logical]; exists {
				return nil, fmt.Errorf("airplan: duplicate revision diff asset section %q", logical)
			}
			if bytes.Contains(section, []byte("\n--- revision-")) ||
				bytes.Contains(section, []byte("\n+++ revision-")) {
				return nil, fmt.Errorf("airplan: revision diff asset section %q contains unified headers", logical)
			}
			report.AssetSections[logical] = section
		}
		rest = rest[sectionEnd:]
	}
	return report, nil
}

func validateExplicitRevisionPathSection(section []byte, kind, logical string) error {
	lineEnd := bytes.IndexByte(section, '\n')
	if lineEnd < 0 {
		return fmt.Errorf("airplan: revision diff %s section %q is incomplete", kind, logical)
	}
	payload := section[lineEnd+1:]
	prefixes := []string{"page added: ", "page removed: ", "page metadata changed: ", "--- revision-"}
	if kind == "asset" {
		prefixes = []string{"asset added: ", "asset removed: ", "asset changed: "}
	}
	for _, prefix := range prefixes {
		if bytes.HasPrefix(payload, []byte(prefix)) {
			return nil
		}
	}
	return fmt.Errorf("airplan: revision diff %s section %q has invalid content", kind, logical)
}

func legacyRevisionDiffPathKind(section []byte) string {
	lineEnd := bytes.IndexByte(section, '\n')
	if lineEnd < 0 {
		return ""
	}
	payload := section[lineEnd+1:]
	for _, prefix := range []string{
		"page added: ", "page removed: ", "page metadata changed: ",
		"--- revision-",
	} {
		if bytes.HasPrefix(payload, []byte(prefix)) {
			return "page"
		}
	}
	for _, prefix := range []string{"asset added: ", "asset removed: ", "asset changed: "} {
		if bytes.HasPrefix(payload, []byte(prefix)) {
			return "asset"
		}
	}
	return ""
}

func parseRevisionDiffSectionHeader(name string, explicit bool) (string, string, error) {
	switch name {
	case "airplan page order", "airplan asset order", "airplan metadata":
		return "global", "", nil
	}
	if explicit {
		for _, candidate := range []struct {
			prefix string
			kind   string
		}{{"airplan page: ", "page"}, {"airplan asset: ", "asset"}} {
			if !strings.HasPrefix(name, candidate.prefix) {
				continue
			}
			encoded := strings.TrimPrefix(name, candidate.prefix)
			var logical string
			if err := json.Unmarshal([]byte(encoded), &logical); err != nil {
				return "", "", fmt.Errorf("airplan: revision diff %s section path is invalid", candidate.kind)
			}
			canonical, _ := json.Marshal(logical)
			if string(canonical) != encoded || ValidateBundlePath(logical) != nil {
				return "", "", fmt.Errorf("airplan: revision diff %s section path %q is invalid", candidate.kind, logical)
			}
			return candidate.kind, logical, nil
		}
		return "", "", fmt.Errorf("airplan: revision diff section header %q is invalid", name)
	}
	if ValidateBundlePath(name) != nil {
		return "", "", fmt.Errorf("airplan: revision diff section path %q is invalid", name)
	}
	// Generation-4 structured reports did not distinguish page and asset
	// sections. Keep their path-keyed projection; current-page filtering drops
	// asset sections before rendering.
	return "page", name, nil
}

func validateRevisionPageSection(
	section []byte, logical string, previous, current int, hasSectionHeader bool,
) error {
	payload := section
	if hasSectionHeader {
		lineEnd := bytes.IndexByte(section, '\n')
		if lineEnd < 0 {
			return fmt.Errorf("airplan: revision diff page section %q is incomplete", logical)
		}
		payload = section[lineEnd+1:]
	}
	lines := strings.Split(strings.TrimSuffix(string(payload), "\n"), "\n")
	minus, plus := -1, -1
	for index, line := range lines {
		if strings.HasPrefix(line, "--- revision-") {
			if minus >= 0 {
				return fmt.Errorf("airplan: revision diff page section %q has duplicate old headers", logical)
			}
			minus = index
		}
		if strings.HasPrefix(line, "+++ revision-") {
			if plus >= 0 {
				return fmt.Errorf("airplan: revision diff page section %q has duplicate new headers", logical)
			}
			plus = index
		}
	}
	if minus < 0 && plus < 0 {
		return nil
	}
	if minus < 0 || plus != minus+1 ||
		lines[minus] != fmt.Sprintf("--- revision-%d/%s", previous, logical) ||
		lines[plus] != fmt.Sprintf("+++ revision-%d/%s", current, logical) {
		return fmt.Errorf("airplan: revision diff page section %q has mismatched unified headers", logical)
	}
	return nil
}

func revisionDiffRange(body []byte) (int, int, error) {
	line, _, _ := bytes.Cut(body, []byte("\n"))
	const prefix = "# airplan revisions: "
	if !bytes.HasPrefix(line, []byte(prefix)) {
		lines := strings.Split(string(body), "\n")
		parseLegacy := func(value, header string) (int, error) {
			value = strings.TrimSuffix(value, "\r")
			if !strings.HasPrefix(value, header) || !strings.HasSuffix(value, "/plan.md") {
				return 0, errors.New("airplan: legacy diff header is invalid")
			}
			number, parseErr := strconv.Atoi(strings.TrimSuffix(
				strings.TrimPrefix(value, header), "/plan.md",
			))
			if parseErr != nil || number <= 0 {
				return 0, errors.New("airplan: legacy diff revision is invalid")
			}
			return number, nil
		}
		for index := 0; index+1 < len(lines); index++ {
			if !strings.HasPrefix(lines[index], "--- revision-") ||
				!strings.HasPrefix(lines[index+1], "+++ revision-") {
				continue
			}
			previous, previousErr := parseLegacy(lines[index], "--- revision-")
			current, currentErr := parseLegacy(lines[index+1], "+++ revision-")
			if previousErr != nil || currentErr != nil || current <= previous {
				return 0, 0, errors.New("airplan: legacy diff revision range is invalid")
			}
			return previous, current, nil
		}
		return 0, 0, errors.New("airplan: revision diff range header is missing")
	}
	parts := strings.Split(string(line[len(prefix):]), " -> ")
	if len(parts) != 2 {
		return 0, 0, errors.New("airplan: revision diff range header is invalid")
	}
	previous, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, errors.New("airplan: revision diff previous revision is invalid")
	}
	current, err := strconv.Atoi(parts[1])
	if err != nil || previous <= 0 || current <= previous {
		return 0, 0, errors.New("airplan: revision diff range is invalid")
	}
	return previous, current, nil
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
