package airplan

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Format identifies the input document format (SPEC.md §2).
type Format int

// Input formats.
const (
	FormatUnknown Format = iota
	FormatMarkdown
	FormatHTML
	FormatText
)

// String returns "md", "html", "txt", or "unknown".
func (f Format) String() string {
	switch f {
	case FormatMarkdown:
		return "md"
	case FormatHTML:
		return "html"
	case FormatText:
		return "txt"
	default:
		return "unknown"
	}
}

// ParseFormat parses a --format flag value: "md", "html", or "txt"
// (SPEC.md §2, §6).
func ParseFormat(s string) (Format, error) {
	switch s {
	case "md":
		return FormatMarkdown, nil
	case "html":
		return FormatHTML, nil
	case "txt":
		return FormatText, nil
	default:
		return FormatUnknown, fmt.Errorf(
			"airplan: invalid format %q (valid: md, html, txt)", s,
		)
	}
}

// DetectFormat applies the detection order of SPEC.md §2: known
// md/html extensions; any other extension → text; an extensionless
// filename the highlighter recognizes (Makefile, Dockerfile, …) →
// text; else content sniffing (leading <!doctype or <html,
// case-insensitive, after whitespace/BOM → html; anything else → md).
// Bare stdin defaulting to markdown is load-bearing — it is the
// primary agent path. A --format override is applied by the caller
// before this runs.
func DetectFormat(name string, data []byte) Format {
	if name != "" {
		switch ext := strings.ToLower(filepath.Ext(name)); ext {
		case ".md", ".markdown":
			return FormatMarkdown
		case ".html", ".htm":
			return FormatHTML
		case "":
			if matchesLexerFilename(filepath.Base(name)) {
				return FormatText
			}
		default:
			return FormatText
		}
	}

	data = trimSniffPrefix(data)
	if asciiHasPrefixFold(data, "<!doctype") ||
		asciiHasPrefixFold(data, "<html") {
		return FormatHTML
	}

	return FormatMarkdown
}

// IsBinary reports whether data looks like binary rather than text:
// a NUL byte within the first 8 KiB, git's binary heuristic
// (SPEC.md §2).
func IsBinary(data []byte) bool {
	n := min(len(data), 8192)
	return bytes.IndexByte(data[:n], 0) >= 0
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
	if hasRobotsMeta(doc) {
		return doc, NoindexAlreadyPresent
	}

	end := findHeadTagEnd(doc)
	if end == -1 {
		return doc, NoindexNoHead
	}

	const tag = `<meta name="robots" content="noindex, nofollow">`
	out := make([]byte, 0, len(doc)+len(tag))
	out = append(out, doc[:end]...)
	out = append(out, tag...)
	out = append(out, doc[end:]...)

	return out, NoindexInjected
}

func trimSniffPrefix(data []byte) []byte {
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		data = data[3:]
	}

	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			break
		}
		if !unicode.IsSpace(r) {
			break
		}
		data = data[size:]
	}

	return data
}

func findHeadTagEnd(doc []byte) int {
	for i := 0; i < len(doc); i++ {
		if !asciiHasPrefixFold(doc[i:], "<head") {
			continue
		}

		next := i + len("<head")
		if next < len(doc) && !isTagBoundary(doc[next]) {
			continue
		}

		return findTagEnd(doc, next)
	}

	return -1
}

func hasRobotsMeta(doc []byte) bool {
	for i := 0; i < len(doc); i++ {
		if !asciiHasPrefixFold(doc[i:], "<meta") {
			continue
		}

		next := i + len("<meta")
		if next < len(doc) && !isTagBoundary(doc[next]) {
			continue
		}

		end := findTagEnd(doc, next)
		if end == -1 {
			return false
		}

		if metaTagHasRobotsName(doc[next : end-1]) {
			return true
		}

		i = end - 1
	}

	return false
}

// findTagEnd returns the index just past the '>' that closes the tag
// being scanned from start, honoring single- and double-quoted
// attribute values so a '>' inside a quoted attribute doesn't end the
// tag early. Returns -1 when the tag never closes.
func findTagEnd(doc []byte, start int) int {
	var quote byte
	for i := start; i < len(doc); i++ {
		b := doc[i]
		switch {
		case quote != 0:
			if b == quote {
				quote = 0
			}
		case b == '"' || b == '\'':
			quote = b
		case b == '>':
			return i + 1
		}
	}

	return -1
}

func metaTagHasRobotsName(attrs []byte) bool {
	for i := 0; i < len(attrs); {
		i = skipHTMLSpace(attrs, i)
		if i >= len(attrs) {
			return false
		}

		if attrs[i] == '/' {
			i++
			continue
		}

		nameStart := i
		for i < len(attrs) && isAttrNameByte(attrs[i]) {
			i++
		}
		if nameStart == i {
			i++
			continue
		}

		attrName := attrs[nameStart:i]
		i = skipHTMLSpace(attrs, i)
		if i >= len(attrs) || attrs[i] != '=' {
			continue
		}

		i++
		i = skipHTMLSpace(attrs, i)
		value, next := readAttrValue(attrs, i)
		i = next

		if asciiEqualFold(attrName, "name") &&
			asciiEqualFold(value, "robots") {
			return true
		}
	}

	return false
}

func readAttrValue(attrs []byte, start int) ([]byte, int) {
	if start >= len(attrs) {
		return nil, start
	}

	switch attrs[start] {
	case '\'', '"':
		quote := attrs[start]
		valueStart := start + 1
		i := valueStart
		for i < len(attrs) && attrs[i] != quote {
			i++
		}
		if i >= len(attrs) {
			return attrs[valueStart:], i
		}
		return attrs[valueStart:i], i + 1
	default:
		i := start
		for i < len(attrs) && !isHTMLSpace(attrs[i]) && attrs[i] != '/' {
			i++
		}
		return attrs[start:i], i
	}
}

func skipHTMLSpace(data []byte, start int) int {
	for start < len(data) && isHTMLSpace(data[start]) {
		start++
	}

	return start
}

func isTagBoundary(b byte) bool {
	return b == '>' || b == '/' || isHTMLSpace(b)
}

func isHTMLSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}

func isAttrNameByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '-' || b == '_' || b == ':'
}

func asciiHasPrefixFold(data []byte, prefix string) bool {
	if len(data) < len(prefix) {
		return false
	}

	for i := 0; i < len(prefix); i++ {
		if asciiLower(data[i]) != asciiLower(prefix[i]) {
			return false
		}
	}

	return true
}

func asciiEqualFold(data []byte, s string) bool {
	if len(data) != len(s) {
		return false
	}

	for i := 0; i < len(s); i++ {
		if asciiLower(data[i]) != asciiLower(s[i]) {
			return false
		}
	}

	return true
}

func asciiLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}

	return b
}
