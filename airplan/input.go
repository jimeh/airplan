package airplan

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
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
	// NoindexAlreadyPresent: the effective head has a robots meta
	// tag; author intent wins and nothing was changed.
	NoindexAlreadyPresent
	// NoindexNoHead: no explicit effective head start token was found;
	// nothing was changed and the caller should warn on stderr.
	NoindexNoHead
)

const noindexMetaTag = `<meta name="robots" content="noindex, nofollow">`

// InjectNoindex splices a robots noindex meta tag immediately after
// the first explicit head start token. It is a byte-level splice: the
// document is tokenized but never re-serialized, and every other byte
// is returned exactly as given (SPEC.md §4).
func InjectNoindex(doc []byte) ([]byte, NoindexResult) {
	scan := scanHTMLHead(doc)
	if scan.headEnd == -1 {
		return doc, NoindexNoHead
	}
	if scan.hasRobotsMeta {
		return doc, NoindexAlreadyPresent
	}

	out := make([]byte, 0, len(doc)+len(noindexMetaTag))
	out = append(out, doc[:scan.headEnd]...)
	out = append(out, noindexMetaTag...)
	out = append(out, doc[scan.headEnd:]...)

	return out, NoindexInjected
}

type htmlHeadScan struct {
	headEnd       int
	hasRobotsMeta bool
}

func scanHTMLHead(doc []byte) htmlHeadScan {
	result := htmlHeadScan{headEnd: -1}
	tokenizer := html.NewTokenizer(bytes.NewReader(doc))
	offset := 0
	templateDepth := 0

	for {
		tokenType := tokenizer.Next()
		// Raw must be measured before Token, which may normalize the
		// tokenizer's internal buffer. Raw token lengths partition the
		// original input and therefore give an exact splice offset.
		offset += len(tokenizer.Raw())
		if tokenType == html.ErrorToken {
			return result
		}
		if tokenType != html.StartTagToken &&
			tokenType != html.SelfClosingTagToken &&
			tokenType != html.EndTagToken {
			continue
		}

		token := tokenizer.Token()
		switch tokenType {
		case html.StartTagToken, html.SelfClosingTagToken:
			if token.Data == "template" {
				if tokenType == html.StartTagToken {
					templateDepth++
				}
				continue
			}

			if result.headEnd == -1 {
				if templateDepth == 0 && token.Data == "head" {
					result.headEnd = offset
				}
				continue
			}
			if templateDepth != 0 {
				continue
			}

			switch token.Data {
			case "body":
				return result
			case "meta":
				if tokenHasRobotsName(token) {
					result.hasRobotsMeta = true
					return result
				}
			}

		case html.EndTagToken:
			if token.Data == "template" {
				if templateDepth > 0 {
					templateDepth--
				}
				continue
			}
			if result.headEnd != -1 && templateDepth == 0 &&
				token.Data == "head" {
				return result
			}
		}
	}
}

func tokenHasRobotsName(token html.Token) bool {
	for _, attr := range token.Attr {
		if attr.Key == "name" &&
			asciiEqualFold([]byte(attr.Val), "robots") {
			return true
		}
	}

	return false
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
