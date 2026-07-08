package airplan

import (
	"bytes"
	"strings"
	"testing"
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
		{name: "unknown", input: "txt", want: FormatUnknown, wantErr: true},
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
			file: "plan.txt",
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
			name: "unknown extension uses sniffing",
			file: "plan.txt",
			data: "<!doctype html>",
			want: FormatHTML,
		},
		{
			name: "unknown extension markdown",
			file: "plan.txt",
			data: "# Plan",
			want: FormatMarkdown,
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
