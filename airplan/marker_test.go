package airplan

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUploadMarkerRoundTrip(t *testing.T) {
	t.Parallel()

	dir := "abcdefghijklmnopqrstuvwxyz"
	createdAt := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	marker := UploadMarker{
		Schema:    MarkerSchema,
		Version:   MarkerVersion,
		Directory: dir,
		CreatedAt: createdAt,
		Format:    "md",
		Page:      "launch-plan.html",
		PageBytes: 1234,
		Source:    "launch-plan.md",
		Title:     "Launch plan",
		Repo:      "https://github.com/acme/airplan",
	}

	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeUploadMarker(body, dir)
	if err != nil {
		t.Fatal(err)
	}
	if *got != marker {
		t.Fatalf("marker = %+v, want %+v", *got, marker)
	}
}

func TestDecodeUploadMarkerValidation(t *testing.T) {
	t.Parallel()

	const dir = "abcdefghijklmnopqrstuvwxyz"
	valid := `{"schema":"airplan-upload","version":1,` +
		`"directory":"` + dir + `",` +
		`"created_at":"2026-07-11T09:00:00Z",` +
		`"format":"md","page":"plan.html","source":"plan.md"}`

	tests := []struct {
		name string
		body string
		dir  string
		code MarkerErrorCode
	}{
		{name: "unknown field allowed", body: strings.TrimSuffix(valid, "}") +
			`,"future":{"nested":true}}`, dir: dir},
		{
			name: "duplicate field", body: strings.TrimSuffix(valid, "}") +
				`,"page":"other.html"}`, dir: dir,
			code: MarkerErrorMalformedJSON,
		},
		{
			name: "duplicate unknown nested field",
			body: strings.TrimSuffix(valid, "}") +
				`,"future":{"x":1,"x":2}}`, dir: dir,
			code: MarkerErrorMalformedJSON,
		},
		{
			name: "unsupported version",
			body: strings.Replace(valid, `"version":1`, `"version":3`, 1),
			dir:  dir, code: MarkerErrorUnsupportedVersion,
		},
		{
			name: "missing required", body: `{"version":1}`, dir: dir,
			code: MarkerErrorInvalidFields,
		},
		{
			name: "wrong directory", body: valid,
			dir:  "bbbbbbbbbbbbbbbbbbbbbbbbbb",
			code: MarkerErrorInvalidFields,
		},
		{
			name: "non UTC", body: strings.Replace(valid,
				`2026-07-11T09:00:00Z`, `2026-07-11T10:00:00+01:00`, 1),
			dir: dir, code: MarkerErrorInvalidFields,
		},
		{
			name: "unsafe page", body: strings.Replace(valid,
				`plan.html`, `../plan.html`, 1), dir: dir,
			code: MarkerErrorInvalidFields,
		},
		{
			name: "source mismatch", body: strings.Replace(valid,
				`plan.md`, `other.md`, 1), dir: dir,
			code: MarkerErrorInvalidFields,
		},
		{
			name: "HTML source", body: strings.Replace(valid,
				`"format":"md"`, `"format":"html"`, 1), dir: dir,
			code: MarkerErrorInvalidFields,
		},
		{
			name: "empty optional source", body: strings.Replace(valid,
				`plan.md`, ``, 1), dir: dir,
			code: MarkerErrorInvalidFields,
		},
		{
			name: "trailing JSON", body: valid + `{}`, dir: dir,
			code: MarkerErrorMalformedJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecodeUploadMarker([]byte(tt.body), tt.dir)
			if tt.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				if got == nil {
					t.Fatal("nil marker")
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			code, ok := MarkerCode(err)
			if !ok || code != tt.code {
				t.Fatalf("code = %q, ok = %v, err = %v; want %q",
					code, ok, err, tt.code)
			}
		})
	}
}

func TestDecodeUploadMarkerSizeAndUTF8(t *testing.T) {
	t.Parallel()

	_, err := DecodeUploadMarker(make([]byte, MaxMarkerSize+1), "ignored")
	assertMarkerCode(t, err, MarkerErrorOversized)

	_, err = DecodeUploadMarker([]byte{'{', '"', 0xff}, "ignored")
	assertMarkerCode(t, err, MarkerErrorMalformedJSON)
}

func TestUploadMarkerV2RequiresPortableMetadata(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	base := UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC),
		Format:    "html", Page: "plan.html", PageBytes: 10,
	}
	for _, mutate := range []func(*UploadMarker){
		func(marker *UploadMarker) { marker.PageBytes = 0 },
		func(marker *UploadMarker) { marker.PageBytes = -1 },
		func(marker *UploadMarker) { marker.Repo = "git@github.com:acme/repo.git" },
		func(marker *UploadMarker) { marker.Repo = "auto" },
	} {
		marker := base
		mutate(&marker)
		if _, err := EncodeUploadMarker(marker); err == nil {
			t.Fatalf("EncodeUploadMarker(%+v) succeeded", marker)
		}
	}

	v1 := base
	v1.Version = 1
	v1.PageBytes = 0
	if _, err := EncodeUploadMarker(v1); err != nil {
		t.Fatalf("version 1 compatibility: %v", err)
	}
}

func TestUploadMarkerFormatFilenames(t *testing.T) {
	t.Parallel()

	dir := "abcdefghijklmnopqrstuvwxyz"
	base := UploadMarker{
		Schema:    MarkerSchema,
		Version:   MarkerVersion,
		Directory: dir,
		CreatedAt: time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC),
		Page:      "plan.html",
		PageBytes: 1,
	}

	tests := []struct {
		name   string
		format string
		source string
		ok     bool
	}{
		{name: "markdown source", format: "md", source: "plan.md", ok: true},
		{name: "markdown no source", format: "md", ok: true},
		{name: "HTML", format: "html", ok: true},
		{name: "text Go", format: "txt", source: "plan.go", ok: true},
		{name: "text no source", format: "txt", ok: true},
		{name: "text HTML collision", format: "txt", source: "plan.html"},
		{name: "unknown format", format: "pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marker := base
			marker.Format = tt.format
			marker.Source = tt.source
			_, err := EncodeUploadMarker(marker)
			if tt.ok && err != nil {
				t.Fatal(err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func assertMarkerCode(t *testing.T, err error, want MarkerErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	var markerErr *MarkerValidationError
	if !errors.As(err, &markerErr) || markerErr.Code != want {
		t.Fatalf("error = %v, want marker code %q", err, want)
	}
}
