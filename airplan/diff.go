package airplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
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

var revisionHunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(?: .*)?$`)

type revisionPageDescriptor struct {
	Format string `json:"format"`
	Title  string `json:"title,omitempty"`
	Lang   string `json:"lang"`
}

type revisionChangedValue struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

type revisionPageMetadataChanges struct {
	Format *revisionChangedValue `json:"format,omitempty"`
	Title  *revisionChangedValue `json:"title,omitempty"`
	Lang   *revisionChangedValue `json:"lang,omitempty"`
}

type revisionAssetDescriptor struct {
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
	SHA256      string `json:"sha256"`
}

type revisionAssetChange struct {
	Before revisionAssetDescriptor `json:"before"`
	After  revisionAssetDescriptor `json:"after"`
}

type revisionBundleMetadata struct {
	Format string `json:"format"`
	Title  string `json:"title"`
	Slug   string `json:"slug"`
}

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
		if err := validateRevisionPageSection(body, "plan.md", previous, current, false, true, false); err != nil {
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
	lastGlobalRank := 0
	lastPath := ""
	seenPath := false
	sectionCount := 0
	noChanges := false
	for len(rest) > 0 {
		if bytes.HasPrefix(rest, []byte("No textual changes.\n")) {
			if len(rest) != len("No textual changes.\n") || sectionCount != 0 {
				return nil, errors.New("airplan: revision diff has trailing content")
			}
			noChanges = true
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
		if !explicit {
			if legacyKind := legacyRevisionDiffPathKind(section); legacyKind != "" {
				kind, logical = legacyKind, name
			}
		}
		switch kind {
		case "global":
			if _, exists := seenGlobals[name]; exists {
				return nil, fmt.Errorf("airplan: duplicate revision diff section %q", name)
			}
			seenGlobals[name] = struct{}{}
			if explicit {
				if seenPath {
					return nil, fmt.Errorf("airplan: revision diff global section %q follows path sections", name)
				}
				rank := revisionGlobalSectionRank(name)
				if rank <= lastGlobalRank {
					return nil, fmt.Errorf("airplan: revision diff global section %q is out of order", name)
				}
				lastGlobalRank = rank
				if err := validateExplicitRevisionGlobalSection(section, name); err != nil {
					return nil, err
				}
			}
		case "page":
			if _, exists := report.PageSections[logical]; exists {
				return nil, fmt.Errorf("airplan: duplicate revision diff page section %q", logical)
			}
			if explicit && seenPath && logical <= lastPath {
				return nil, fmt.Errorf("airplan: revision diff path section %q is out of order", logical)
			}
			if err := validateRevisionPageSection(section, logical, previous, current, true, explicit, !explicit); err != nil {
				return nil, err
			}
			report.PageSections[logical] = section
			seenPath, lastPath = true, logical
		case "asset":
			if _, exists := report.AssetSections[logical]; exists {
				return nil, fmt.Errorf("airplan: duplicate revision diff asset section %q", logical)
			}
			if explicit && seenPath && logical <= lastPath {
				return nil, fmt.Errorf("airplan: revision diff path section %q is out of order", logical)
			}
			if explicit {
				if err := validateExplicitRevisionAssetSection(section, logical); err != nil {
					return nil, err
				}
			}
			report.AssetSections[logical] = section
			seenPath, lastPath = true, logical
		}
		sectionCount++
		rest = rest[sectionEnd:]
	}
	if explicit && sectionCount == 0 && !noChanges {
		return nil, errors.New("airplan: revision diff has no sections")
	}
	return report, nil
}

func revisionSectionPayload(section []byte, kind, logical string) ([]byte, error) {
	lineEnd := bytes.IndexByte(section, '\n')
	if lineEnd < 0 {
		return nil, fmt.Errorf("airplan: revision diff %s section %q is incomplete", kind, logical)
	}
	return section[lineEnd+1:], nil
}

func revisionGlobalSectionRank(name string) int {
	switch name {
	case "airplan page order":
		return 1
	case "airplan asset order":
		return 2
	case "airplan metadata":
		return 3
	default:
		return 0
	}
}

func decodeCanonicalRevisionJSON(encoded []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON value")
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(canonical, encoded) {
		return errors.New("JSON is not canonical")
	}
	return nil
}

func validateExplicitRevisionGlobalSection(section []byte, name string) error {
	payload, err := revisionSectionPayload(section, "global", name)
	if err != nil {
		return err
	}
	if !bytes.HasSuffix(payload, []byte("\n")) {
		return fmt.Errorf("airplan: revision diff global section %q is incomplete", name)
	}
	lines := bytes.Split(bytes.TrimSuffix(payload, []byte("\n")), []byte("\n"))
	if len(lines) != 2 || !bytes.HasPrefix(lines[0], []byte("before: ")) || !bytes.HasPrefix(lines[1], []byte("after: ")) {
		return fmt.Errorf("airplan: revision diff global section %q has invalid content", name)
	}
	var orderValues [][]string
	var metadataValues []revisionBundleMetadata
	for _, line := range lines {
		encoded := line[bytes.Index(line, []byte(": "))+2:]
		if name == "airplan metadata" {
			var value revisionBundleMetadata
			if err := decodeCanonicalRevisionJSON(encoded, &value); err != nil ||
				value.Format != FormatMarkdown.String() || value.Slug == "" || SanitizeSlug(value.Slug) != value.Slug {
				return fmt.Errorf("airplan: revision diff global section %q has invalid metadata", name)
			}
			metadataValues = append(metadataValues, value)
			continue
		}
		var values []string
		if err := decodeCanonicalRevisionJSON(encoded, &values); err != nil || values == nil {
			return fmt.Errorf("airplan: revision diff global section %q has invalid order", name)
		}
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if ValidateBundlePath(value) != nil {
				return fmt.Errorf("airplan: revision diff global section %q has invalid path %q", name, value)
			}
			folded := strings.ToLower(value)
			if _, exists := seen[folded]; exists {
				return fmt.Errorf("airplan: revision diff global section %q has duplicate path %q", name, value)
			}
			seen[folded] = struct{}{}
		}
		orderValues = append(orderValues, values)
	}
	if (len(orderValues) == 2 && revisionPathOrderEqual(orderValues[0], orderValues[1])) ||
		(len(metadataValues) == 2 && metadataValues[0] == metadataValues[1]) {
		return fmt.Errorf("airplan: revision diff global section %q does not describe a change", name)
	}
	return nil
}

func validateExplicitRevisionAssetSection(section []byte, logical string) error {
	payload, err := revisionSectionPayload(section, "asset", logical)
	if err != nil {
		return err
	}
	if !bytes.HasSuffix(payload, []byte("\n")) || bytes.Count(payload, []byte("\n")) != 1 {
		return fmt.Errorf("airplan: revision diff asset section %q has invalid content", logical)
	}
	line := bytes.TrimSuffix(payload, []byte("\n"))
	for _, candidate := range []struct {
		prefix string
		value  any
	}{
		{"asset added: ", &revisionAssetDescriptor{}},
		{"asset removed: ", &revisionAssetDescriptor{}},
		{"asset changed: ", &revisionAssetChange{}},
	} {
		if !bytes.HasPrefix(line, []byte(candidate.prefix)) {
			continue
		}
		if err := decodeCanonicalRevisionJSON(line[len(candidate.prefix):], candidate.value); err != nil || !validRevisionAssetValue(candidate.value) {
			return fmt.Errorf("airplan: revision diff asset section %q has invalid summary", logical)
		}
		return nil
	}
	return fmt.Errorf("airplan: revision diff asset section %q has invalid content", logical)
}

func validRevisionAssetValue(value any) bool {
	valid := func(item revisionAssetDescriptor) bool {
		return validNormalizedContentType(item.ContentType) &&
			item.Bytes >= 0 && validLowerHexDigest(item.SHA256)
	}
	switch item := value.(type) {
	case *revisionAssetDescriptor:
		return valid(*item)
	case *revisionAssetChange:
		return valid(item.Before) && valid(item.After) && item.Before != item.After
	default:
		return false
	}
}

func validLowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
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
	section []byte, logical string, previous, current int, hasSectionHeader, strict, allowPlanMD bool,
) error {
	payload := section
	if hasSectionHeader {
		var err error
		payload, err = revisionSectionPayload(section, "page", logical)
		if err != nil {
			return err
		}
	}
	if len(payload) == 0 || payload[len(payload)-1] != '\n' {
		return fmt.Errorf("airplan: revision diff page section %q is incomplete", logical)
	}
	lines := strings.Split(strings.TrimSuffix(string(payload), "\n"), "\n")
	descriptor := false
	requiresDiff := false
	if strict && len(lines) > 0 {
		for _, candidate := range []string{"page added: ", "page removed: "} {
			if strings.HasPrefix(lines[0], candidate) {
				var value revisionPageDescriptor
				encoded := []byte(strings.TrimPrefix(lines[0], candidate))
				if err := decodeCanonicalRevisionJSON(encoded, &value); err != nil ||
					(value.Format != FormatMarkdown.String() && value.Format != FormatText.String()) || value.Lang == "" {
					return fmt.Errorf("airplan: revision diff page section %q has invalid descriptor", logical)
				}
				descriptor = true
				requiresDiff = true
				lines = lines[1:]
				break
			}
		}
		if !descriptor && strings.HasPrefix(lines[0], "page metadata changed: ") {
			var value revisionPageMetadataChanges
			encoded := []byte(strings.TrimPrefix(lines[0], "page metadata changed: "))
			if err := decodeCanonicalRevisionJSON(encoded, &value); err != nil ||
				!validRevisionPageMetadataChanges(value) {
				return fmt.Errorf("airplan: revision diff page section %q has invalid metadata", logical)
			}
			descriptor = true
			lines = lines[1:]
		}
	} else if !strict && len(lines) > 0 {
		for _, prefix := range []string{"page added: ", "page removed: ", "page metadata changed: "} {
			if strings.HasPrefix(lines[0], prefix) {
				descriptor = true
				lines = lines[1:]
				break
			}
		}
	}
	if len(lines) == 0 {
		if descriptor && !requiresDiff {
			return nil
		}
		return fmt.Errorf("airplan: revision diff page section %q has invalid content", logical)
	}
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "--- revision-") ||
		!strings.HasPrefix(lines[1], "+++ revision-") {
		return fmt.Errorf("airplan: revision diff page section %q has invalid content", logical)
	}
	expectedPaths := []string{logical}
	if allowPlanMD && logical != "plan.md" {
		expectedPaths = append(expectedPaths, "plan.md")
	}
	matchedHeaders := false
	for _, path := range expectedPaths {
		if lines[0] == fmt.Sprintf("--- revision-%d/%s", previous, path) &&
			lines[1] == fmt.Sprintf("+++ revision-%d/%s", current, path) {
			matchedHeaders = true
			break
		}
	}
	if !matchedHeaders {
		return fmt.Errorf("airplan: revision diff page section %q has mismatched unified headers", logical)
	}
	return validateRevisionHunks(lines[2:], logical, strict)
}

func validRevisionPageMetadataChanges(value revisionPageMetadataChanges) bool {
	if value.Format == nil && value.Title == nil && value.Lang == nil {
		return false
	}
	for _, changed := range []*revisionChangedValue{value.Format, value.Title, value.Lang} {
		if changed != nil && changed.Before == changed.After {
			return false
		}
	}
	if value.Format != nil {
		validFormat := func(format string) bool {
			return format == FormatMarkdown.String() || format == FormatText.String()
		}
		if !validFormat(value.Format.Before) || !validFormat(value.Format.After) {
			return false
		}
	}
	return true
}

func validateRevisionHunks(lines []string, logical string, strict bool) error {
	if len(lines) == 0 {
		return fmt.Errorf("airplan: revision diff page section %q has no unified hunks", logical)
	}
	previousOldEnd, previousNewEnd := -1, -1
	for index := 0; index < len(lines); {
		match := revisionHunkHeader.FindStringSubmatch(lines[index])
		if match == nil {
			return fmt.Errorf("airplan: revision diff page section %q has invalid unified hunk", logical)
		}
		oldStart, oldCount, err := revisionHunkRange(match[1], match[2], strict)
		if err != nil {
			return fmt.Errorf("airplan: revision diff page section %q has invalid unified hunk", logical)
		}
		newStart, newCount, err := revisionHunkRange(match[3], match[4], strict)
		if err != nil {
			return fmt.Errorf("airplan: revision diff page section %q has invalid unified hunk", logical)
		}
		if strict {
			if oldCount == 0 && newCount == 0 {
				return fmt.Errorf("airplan: revision diff page section %q has empty unified hunk", logical)
			}
			oldEnd, oldEndErr := revisionHunkEnd(oldStart, oldCount)
			newEnd, newEndErr := revisionHunkEnd(newStart, newCount)
			if oldEndErr != nil || newEndErr != nil ||
				(previousOldEnd >= 0 && (oldStart <= previousOldEnd || newStart <= previousNewEnd)) {
				return fmt.Errorf("airplan: revision diff page section %q has non-monotonic unified hunks", logical)
			}
			previousOldEnd, previousNewEnd = oldEnd, newEnd
		}
		index++
		oldSeen, newSeen := 0, 0
		newlineMarkerEligible := false
		for index < len(lines) && !strings.HasPrefix(lines[index], "@@ ") {
			line := lines[index]
			switch {
			case line == `\ No newline at end of file`:
				if strict && !newlineMarkerEligible {
					return fmt.Errorf("airplan: revision diff page section %q has misplaced newline marker", logical)
				}
				newlineMarkerEligible = false
			case strings.HasPrefix(line, " "):
				oldSeen++
				newSeen++
				newlineMarkerEligible = true
			case strings.HasPrefix(line, "+"):
				newSeen++
				newlineMarkerEligible = true
			case strings.HasPrefix(line, "-"):
				oldSeen++
				newlineMarkerEligible = true
			default:
				return fmt.Errorf("airplan: revision diff page section %q has trailing or invalid unified content", logical)
			}
			index++
		}
		if oldSeen != oldCount || newSeen != newCount {
			return fmt.Errorf("airplan: revision diff page section %q has mismatched unified hunk counts", logical)
		}
	}
	return nil
}

func revisionHunkRange(startEncoded, countEncoded string, strict bool) (int, int, error) {
	start, err := strconv.Atoi(startEncoded)
	if err != nil {
		return 0, 0, err
	}
	count := 1
	if countEncoded != "" {
		count, err = strconv.Atoi(countEncoded)
		if err != nil {
			return 0, 0, err
		}
	}
	if strict && (strconv.Itoa(start) != startEncoded ||
		(countEncoded != "" && strconv.Itoa(count) != countEncoded) ||
		(countEncoded != "" && count == 1) ||
		(start == 0 && count != 0)) {
		return 0, 0, errors.New("non-canonical unified range")
	}
	return start, count, nil
}

func revisionHunkEnd(start, count int) (int, error) {
	if count == 0 {
		return start, nil
	}
	maxInt := int(^uint(0) >> 1)
	if start > maxInt-count+1 {
		return 0, errors.New("unified range overflows")
	}
	return start + count - 1, nil
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
