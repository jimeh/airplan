package airplan

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestParseFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Format
		wantErr bool
	}{
		{name: "markdown", input: "md", want: FormatMarkdown},
		{name: "html", input: "html", want: FormatHTML},
		{name: "empty", input: "", want: FormatUnknown, wantErr: true},
		{name: "case sensitive", input: "MD", want: FormatUnknown, wantErr: true},
		{name: "text", input: "txt", want: FormatText},
		{name: "unknown", input: "rst", want: FormatUnknown, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseFormat(tt.input)
			if got != tt.want {
				t.Fatalf("ParseFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseFormat(%q) error = %v, wantErr %v",
					tt.input, err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "md, html") {
				t.Fatalf("ParseFormat(%q) error = %q, want valid values",
					tt.input, err)
			}
		})
	}
}

func TestDetectFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
		data string
		want Format
	}{
		{
			name: "md extension",
			file: "plan.md",
			data: "<html>",
			want: FormatMarkdown,
		},
		{
			name: "md extension upper",
			file: "PLAN.MD",
			data: "<html>",
			want: FormatMarkdown,
		},
		{
			name: "markdown extension mixed",
			file: "plan.MarkDown",
			data: "<html>",
			want: FormatMarkdown,
		},
		{
			name: "html extension",
			file: "plan.html",
			data: "# plan",
			want: FormatHTML,
		},
		{
			name: "html extension upper",
			file: "PLAN.HTML",
			data: "# plan",
			want: FormatHTML,
		},
		{
			name: "htm extension mixed",
			file: "plan.HtM",
			data: "# plan",
			want: FormatHTML,
		},
		{
			name: "doctype sniff",
			file: "",
			data: "<!DOCTYPE HTML><html></html>",
			want: FormatHTML,
		},
		{
			name: "html sniff mixed case",
			file: "",
			data: "<Html><body></body></Html>",
			want: FormatHTML,
		},
		{
			name: "bom and unicode whitespace sniff",
			file: "",
			data: "\xef\xbb\xbf\u00a0\n\t<Html></Html>",
			want: FormatHTML,
		},
		{
			name: "plain markdown",
			file: "",
			data: "# Plan\n\nBody",
			want: FormatMarkdown,
		},
		{
			name: "markdown starting with div",
			file: "",
			data: "<div>not standalone html</div>",
			want: FormatMarkdown,
		},
		{
			name: "txt extension is text",
			file: "plan.txt",
			data: "<!doctype html>",
			want: FormatText,
		},
		{
			name: "source file extension is text",
			file: "main.go",
			data: "package main",
			want: FormatText,
		},
		{
			name: "json extension is text",
			file: "config.JSON",
			data: "{}",
			want: FormatText,
		},
		{
			name: "extensionless lexer name is text",
			file: "Makefile",
			data: "all:\n\tgo build\n",
			want: FormatText,
		},
		{
			name: "extensionless unrecognized name sniffs to md",
			file: "LICENSE",
			data: "MIT License",
			want: FormatMarkdown,
		},
		{
			name: "extensionless unrecognized name sniffs to html",
			file: "homepage",
			data: "<!doctype html>",
			want: FormatHTML,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := DetectFormat(tt.file, []byte(tt.data))
			if got != tt.want {
				t.Fatalf("DetectFormat(%q, %q) = %v, want %v",
					tt.file, tt.data, got, tt.want)
			}
			if got == FormatUnknown {
				t.Fatalf("DetectFormat(%q, %q) returned FormatUnknown",
					tt.file, tt.data)
			}
		})
	}
}

func TestInjectNoindex(t *testing.T) {
	t.Parallel()

	const tag = `<meta name="robots" content="noindex, nofollow">`

	tests := []struct {
		name       string
		input      string
		want       string
		wantResult NoindexResult
	}{
		{
			name:       "injects after head",
			input:      `<html><head><title>x</title></head></html>`,
			want:       `<html><head>` + tag + `<title>x</title></head></html>`,
			wantResult: NoindexInjected,
		},
		{
			name:  "injects after head with attributes",
			input: `<html><head lang="en"><title>x</title></head></html>`,
			want: `<html><head lang="en">` +
				tag + `<title>x</title></head></html>`,
			wantResult: NoindexInjected,
		},
		{
			name:       "injects after uppercase head",
			input:      `<html><HEAD><title>x</title></HEAD></html>`,
			want:       `<html><HEAD>` + tag + `<title>x</title></HEAD></html>`,
			wantResult: NoindexInjected,
		},
		{
			name:       "does not match header",
			input:      `<html><header>title</header><body>x</body></html>`,
			want:       `<html><header>title</header><body>x</body></html>`,
			wantResult: NoindexNoHead,
		},
		{
			name: "skips header before real head",
			input: `<html><header>title</header>` +
				`<head><title>x</title></head></html>`,
			want: `<html><header>title</header><head>` +
				tag + `<title>x</title></head></html>`,
			wantResult: NoindexInjected,
		},
		{
			name: "existing robots double quotes",
			input: `<html><head><meta name="robots" content="index">` +
				`</head></html>`,
			want: `<html><head><meta name="robots" content="index">` +
				`</head></html>`,
			wantResult: NoindexAlreadyPresent,
		},
		{
			name: "existing robots single quotes",
			input: `<html><head><meta name='robots' content='index'>` +
				`</head></html>`,
			want: `<html><head><meta name='robots' content='index'>` +
				`</head></html>`,
			wantResult: NoindexAlreadyPresent,
		},
		{
			name:       "existing robots unquoted uppercase",
			input:      `<html><head><meta NAME=ROBOTS></head></html>`,
			want:       `<html><head><meta NAME=ROBOTS></head></html>`,
			wantResult: NoindexAlreadyPresent,
		},
		{
			name: "existing robots flexible whitespace",
			input: "<html><head><meta \n\tNAME \r= 'ROBOTS' >" +
				"</head></html>",
			want: "<html><head><meta \n\tNAME \r= 'ROBOTS' >" +
				"</head></html>",
			wantResult: NoindexAlreadyPresent,
		},
		{
			name:       "no head",
			input:      `<html><body>x</body></html>`,
			want:       `<html><body>x</body></html>`,
			wantResult: NoindexNoHead,
		},
		{
			name:  "head self closing boundary",
			input: `<html><head/><body>x</body></html>`,
			want: `<html><head/>` +
				tag + `<body>x</body></html>`,
			wantResult: NoindexInjected,
		},
		{
			name: "commented robots does not suppress injection",
			input: `<!-- <meta name="robots" content="index"> -->` +
				`<html><head><title>x</title></head></html>`,
			want: `<!-- <meta name="robots" content="index"> -->` +
				`<html><head>` + tag + `<title>x</title></head></html>`,
			wantResult: NoindexInjected,
		},
		{
			name: "commented head is not splice target",
			input: `<!-- <head data-fake=">"> -->` +
				`<html><head lang=en><title>x</title></head></html>`,
			want: `<!-- <head data-fake=">"> -->` +
				`<html><head lang=en>` + tag +
				`<title>x</title></head></html>`,
			wantResult: NoindexInjected,
		},
		{
			name: "script lookalikes before head are inert",
			input: `<script>"<head><meta name='robots'>"</script>` +
				`<HEAD data-x=1></HEAD>`,
			want: `<script>"<head><meta name='robots'>"</script>` +
				`<HEAD data-x=1>` + tag + `</HEAD>`,
			wantResult: NoindexInjected,
		},
		{
			name: "template head before real head is inert",
			input: `<template><head><meta name=robots></head></template>` +
				`<head><title>x</title></head>`,
			want: `<template><head><meta name=robots></head></template>` +
				`<head>` + tag + `<title>x</title></head>`,
			wantResult: NoindexInjected,
		},
		{
			name: "noscript head before real head is inert",
			input: `<noscript><head><meta name=robots></head></noscript>` +
				`<head></head>`,
			want: `<noscript><head><meta name=robots></head></noscript>` +
				`<head>` + tag + `</head>`,
			wantResult: NoindexInjected,
		},
		{
			name: "style lookalike in head is inert",
			input: `<head><style>x::after{content:"<meta name=robots>"}` +
				`</style></head>`,
			want: `<head>` + tag +
				`<style>x::after{content:"<meta name=robots>"}</style></head>`,
			wantResult: NoindexInjected,
		},
		{
			name: "rcdata lookalikes in head are inert",
			input: `<head><title><meta name=robots></title>` +
				`<textarea><meta name=robots></textarea></head>`,
			want: `<head>` + tag + `<title><meta name=robots></title>` +
				`<textarea><meta name=robots></textarea></head>`,
			wantResult: NoindexInjected,
		},
		{
			name: "nested template robots meta is inert",
			input: `<head><template><template><meta name=robots>` +
				`</template></template></head>`,
			want: `<head>` + tag +
				`<template><template><meta name=robots>` +
				`</template></template></head>`,
			wantResult: NoindexInjected,
		},
		{
			name:  "noscript robots meta is inert",
			input: `<head><noscript><meta name=robots></noscript></head>`,
			want: `<head>` + tag +
				`<noscript><meta name=robots></noscript></head>`,
			wantResult: NoindexInjected,
		},
		{
			name:  "robots meta after head does not suppress injection",
			input: `<head></head><meta name=robots><body>x</body>`,
			want: `<head>` + tag +
				`</head><meta name=robots><body>x</body>`,
			wantResult: NoindexInjected,
		},
		{
			name:       "robots meta after body does not suppress injection",
			input:      `<head><body><meta name=robots></body>`,
			want:       `<head>` + tag + `<body><meta name=robots></body>`,
			wantResult: NoindexInjected,
		},
		{
			name: "entity encoded robots name is recognized",
			input: `<head><META content="all > none" NAME="ro&#98;ots">` +
				`</head>`,
			want: `<head><META content="all > none" NAME="ro&#98;ots">` +
				`</head>`,
			wantResult: NoindexAlreadyPresent,
		},
		{
			name:       "unclosed comment hides head lookalike",
			input:      `<!-- <head><meta name=robots>`,
			want:       `<!-- <head><meta name=robots>`,
			wantResult: NoindexNoHead,
		},
		{
			name:       "unclosed raw text hides head lookalike",
			input:      `<script><head><meta name=robots>`,
			want:       `<script><head><meta name=robots>`,
			wantResult: NoindexNoHead,
		},
		{
			name:       "unclosed quoted head has no splice point",
			input:      `<html><head data-x="oops<title>x</title>`,
			want:       `<html><head data-x="oops<title>x</title>`,
			wantResult: NoindexNoHead,
		},
		{
			name:       "unclosed head has no splice point",
			input:      `<html><head`,
			want:       `<html><head`,
			wantResult: NoindexNoHead,
		},
		{
			name:       "malformed content after head still injects",
			input:      `<html><head><script>"unterminated`,
			want:       `<html><head>` + tag + `<script>"unterminated`,
			wantResult: NoindexInjected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, result := InjectNoindex([]byte(tt.input))
			if result != tt.wantResult {
				t.Fatalf("InjectNoindex result = %v, want %v",
					result, tt.wantResult)
			}
			if string(got) != tt.want {
				t.Fatalf("InjectNoindex output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInjectNoindexByteExactness(t *testing.T) {
	t.Parallel()

	tag := []byte(`<meta name="robots" content="noindex, nofollow">`)
	prefix := []byte("<html>\r\n<head data-x=\"1\">")
	suffix := []byte("\n<title>Hi</title>\x00\n<body>\xff raw</body></html>")
	input := append(append([]byte{}, prefix...), suffix...)

	got, result := InjectNoindex(input)
	if result != NoindexInjected {
		t.Fatalf("InjectNoindex result = %v, want %v",
			result, NoindexInjected)
	}

	want := append(append(append([]byte{}, prefix...), tag...), suffix...)
	if !bytes.Equal(got, want) {
		t.Fatalf("InjectNoindex output = %q, want %q", got, want)
	}
	if !bytes.Equal(got[:len(prefix)], prefix) {
		t.Fatalf("prefix changed: got %q, want %q", got[:len(prefix)], prefix)
	}
	if !bytes.Equal(got[len(prefix)+len(tag):], suffix) {
		t.Fatalf("suffix changed: got %q, want %q",
			got[len(prefix)+len(tag):], suffix)
	}
}

func TestInjectNoindexPreservesBOMAndUnicode(t *testing.T) {
	t.Parallel()

	input := []byte("\xef\xbb\xbf<!doctype html>\r\n<head data-city=\"Malmö\">" +
		"\r\n<title>設計</title></head>")
	scan := scanHTMLHead(input)
	if scan.headEnd < 0 {
		t.Fatal("scanHTMLHead did not find head")
	}

	got, result := InjectNoindex(input)
	if result != NoindexInjected {
		t.Fatalf("InjectNoindex result = %v, want %v",
			result, NoindexInjected)
	}
	if !bytes.Equal(got[:scan.headEnd], input[:scan.headEnd]) {
		t.Fatal("bytes before insertion changed")
	}
	if !bytes.Equal(got[scan.headEnd+len(noindexMetaTag):],
		input[scan.headEnd:]) {
		t.Fatal("bytes after insertion changed")
	}
}

func TestHTMLTokenizerRawOffsetsPartitionInput(t *testing.T) {
	t.Parallel()

	inputs := [][]byte{
		[]byte(`<!doctype html><HEAD data-x=">"><title>x</title></HEAD>`),
		[]byte("<!-- fake <head> -->\r\n<head lang=\"en\">\r\n</head>"),
		[]byte(`<script>"<head>"</script><head><meta name=robots></head>`),
	}
	for _, input := range inputs {
		tokenizer := html.NewTokenizer(bytes.NewReader(input))
		offset := 0
		for {
			tokenType := tokenizer.Next()
			rawLen := len(tokenizer.Raw())
			if offset+rawLen > len(input) {
				t.Fatalf("raw offset %d exceeds input length %d",
					offset+rawLen, len(input))
			}
			offset += rawLen
			if tokenType == html.ErrorToken {
				break
			}
			if tokenType == html.StartTagToken ||
				tokenType == html.SelfClosingTagToken ||
				tokenType == html.EndTagToken {
				_ = tokenizer.Token()
			}
		}
		if offset != len(input) {
			t.Fatalf("raw tokens cover %d bytes, want %d", offset, len(input))
		}
	}
}

func FuzzInjectNoindex(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`<html><head></head><body>x</body></html>`),
		[]byte(`<!-- <head> --><head><meta name=robots></head>`),
		[]byte(`<script>"<head>"</script><head>`),
		[]byte(`<head><template><meta name=robots></template></head>`),
		[]byte("\xef\xbb\xbf\r\n<head data-x=\"é\">\x00\xff"),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		original := bytes.Clone(input)
		output, result := InjectNoindex(input)
		if !bytes.Equal(input, original) {
			t.Fatal("InjectNoindex mutated its input")
		}

		switch result {
		case NoindexNoHead, NoindexAlreadyPresent:
			if !bytes.Equal(output, original) {
				t.Fatalf("unchanged result %v modified input", result)
			}
		case NoindexInjected:
			scan := scanHTMLHead(original)
			if scan.headEnd < 0 || scan.headEnd > len(original) {
				t.Fatalf("invalid insertion offset %d", scan.headEnd)
			}
			if len(output) != len(original)+len(noindexMetaTag) {
				t.Fatalf("output length = %d, want %d",
					len(output), len(original)+len(noindexMetaTag))
			}
			if !bytes.Equal(output[:scan.headEnd], original[:scan.headEnd]) ||
				!bytes.Equal(output[scan.headEnd:scan.headEnd+len(noindexMetaTag)],
					[]byte(noindexMetaTag)) ||
				!bytes.Equal(output[scan.headEnd+len(noindexMetaTag):],
					original[scan.headEnd:]) {
				t.Fatal("injection did not preserve original bytes")
			}
		default:
			t.Fatalf("unknown NoindexResult %d", result)
		}
	})
}

func TestInjectNoindexQuotedTagEnd(t *testing.T) {
	t.Run("gt inside head attribute value", func(t *testing.T) {
		doc := `<html><head data-x=">"><title>x</title></head></html>`
		out, res := InjectNoindex([]byte(doc))
		if res != NoindexInjected {
			t.Fatalf("result = %v, want NoindexInjected", res)
		}
		want := `<html><head data-x=">">` +
			`<meta name="robots" content="noindex, nofollow">` +
			`<title>x</title></head></html>`
		if string(out) != want {
			t.Errorf("out = %q, want %q", out, want)
		}
	})

	t.Run("gt inside meta content value", func(t *testing.T) {
		doc := `<html><head><meta name="description" content="a > b">` +
			`</head></html>`
		out, res := InjectNoindex([]byte(doc))
		if res != NoindexInjected {
			t.Fatalf("result = %v, want NoindexInjected", res)
		}
		if !strings.Contains(string(out),
			`<head><meta name="robots" content="noindex, nofollow">`) {
			t.Errorf("meta not spliced after <head>: %q", out)
		}
	})

	t.Run("robots meta with gt in content", func(t *testing.T) {
		doc := `<html><head>` +
			`<meta content="all > none" name="robots"></head></html>`
		out, res := InjectNoindex([]byte(doc))
		if res != NoindexAlreadyPresent {
			t.Fatalf("result = %v, want NoindexAlreadyPresent", res)
		}
		if string(out) != doc {
			t.Error("document was modified")
		}
	})
}

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{"empty", "", false},
		{"plain text", "hello world\n", false},
		{"utf-8", "héllo ✨", false},
		{"nul byte", "PK\x03\x04\x00", true},
		{"png header", "\x89PNG\r\n\x1a\n\x00", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBinary([]byte(tt.data)); got != tt.want {
				t.Errorf("IsBinary(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}

	t.Run("nul past first 8KiB is not binary", func(t *testing.T) {
		data := append(bytes.Repeat([]byte("a"), 8192), 0)
		if IsBinary(data) {
			t.Error("NUL beyond the first 8 KiB should not flag binary")
		}
	})
}
