package airplan

import "errors"

// Format identifies the input document format (SPEC.md §2).
type Format int

// Input formats.
const (
	FormatUnknown Format = iota
	FormatMarkdown
	FormatHTML
)

// String returns "md", "html", or "unknown".
func (f Format) String() string {
	switch f {
	case FormatMarkdown:
		return "md"
	case FormatHTML:
		return "html"
	default:
		return "unknown"
	}
}

// ParseFormat parses a --format flag value: "md" or "html"
// (SPEC.md §2, §6).
func ParseFormat(s string) (Format, error) {
	return FormatUnknown, errors.New("airplan: ParseFormat not implemented")
}

// DetectFormat applies the detection order of SPEC.md §2: file
// extension when name is non-empty and recognized, else content
// sniffing (leading <!doctype or <html, case-insensitive, after
// whitespace/BOM → html; anything else → md). A --format override is
// applied by the caller before this runs.
func DetectFormat(name string, data []byte) Format {
	return FormatUnknown
}

// NoindexResult reports what InjectNoindex did (SPEC.md §4).
type NoindexResult int

// InjectNoindex outcomes.
const (
	// NoindexInjected: the meta tag was spliced in after <head>.
	NoindexInjected NoindexResult = iota
	// NoindexAlreadyPresent: the document has a robots meta tag;
	// author intent wins and nothing was changed.
	NoindexAlreadyPresent
	// NoindexNoHead: no <head> tag was found; nothing was changed and
	// the caller should warn on stderr.
	NoindexNoHead
)

// InjectNoindex splices a
// <meta name="robots" content="noindex, nofollow"> tag immediately
// after the first <head …> tag, found by a case-insensitive scan. It
// is a byte-level splice: the document is never parsed or
// re-serialized, and every other byte is returned exactly as given
// (SPEC.md §4).
func InjectNoindex(doc []byte) ([]byte, NoindexResult) {
	return doc, NoindexNoHead
}
