package airplan

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const markerTestDir = "abcdefghijklmnopqrstuvwxyz"

var markerTestTime = time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)

func TestUploadMarkerV3DocumentRoundTrip(t *testing.T) {
	t.Parallel()

	marker := validDocumentMarker()
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"page":`) ||
		strings.Contains(string(body), `"page_bytes":`) ||
		strings.Contains(string(body), `"source":`) {
		t.Fatalf("marker contains legacy fields: %s", body)
	}

	got, err := DecodeUploadMarker(body, markerTestDir)
	if err != nil {
		t.Fatal(err)
	}
	want := marker
	want.Page = "launch-plan.html"
	want.PageBytes = 1234
	want.Source = "launch-plan.md"
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("marker = %+v, want %+v", *got, want)
	}
}

func TestUploadMarkerV3CollectionRoundTrip(t *testing.T) {
	t.Parallel()

	marker := validCollectionMarker()
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeUploadMarkerForName(
		body, markerTestDir, CollectionMarkerFilename,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := marker
	want.Page = "index.html"
	want.PageBytes = 900
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("marker = %+v, want %+v", *got, want)
	}
}

func TestMarkerFilenameForKind(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		kind UploadKind
		name string
		ok   bool
	}{
		{UploadKindDocument, MarkerFilename, true},
		{UploadKindCollection, CollectionMarkerFilename, true},
		{"future", "", false},
	} {
		name, ok := MarkerFilenameForKind(tt.kind)
		if name != tt.name || ok != tt.ok {
			t.Fatalf("MarkerFilenameForKind(%q) = %q, %v; want %q, %v",
				tt.kind, name, ok, tt.name, tt.ok)
		}
	}
}

func TestDecodeUploadMarkerV3FilenameMustMatchKind(t *testing.T) {
	t.Parallel()

	document, err := EncodeUploadMarker(validDocumentMarker())
	if err != nil {
		t.Fatal(err)
	}
	collection, err := EncodeUploadMarker(validCollectionMarker())
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name     string
		body     []byte
		basename string
	}{
		{"document under collection name", document, CollectionMarkerFilename},
		{"collection under document name", collection, MarkerFilename},
		{"unsupported marker name", document, ".ownership.json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeUploadMarkerForName(
				tt.body, markerTestDir, tt.basename,
			)
			assertMarkerCode(t, err, MarkerErrorInvalidFields)
		})
	}
}

func TestDecodeUploadMarkerValidation(t *testing.T) {
	t.Parallel()

	valid := `{"schema":"airplan-upload","version":1,` +
		`"directory":"` + markerTestDir + `",` +
		`"created_at":"2026-07-11T09:00:00Z",` +
		`"format":"md","page":"plan.html","source":"plan.md"}`

	tests := []struct {
		name string
		body string
		dir  string
		code MarkerErrorCode
	}{
		{name: "unknown field allowed", body: strings.TrimSuffix(valid, "}") +
			`,"future":{"nested":true}}`, dir: markerTestDir},
		{
			name: "duplicate field", body: strings.TrimSuffix(valid, "}") +
				`,"page":"other.html"}`, dir: markerTestDir,
			code: MarkerErrorMalformedJSON,
		},
		{
			name: "duplicate unknown nested field",
			body: strings.TrimSuffix(valid, "}") +
				`,"future":{"x":1,"x":2}}`, dir: markerTestDir,
			code: MarkerErrorMalformedJSON,
		},
		{
			name: "unsupported version",
			body: strings.Replace(valid, `"version":1`, `"version":4`, 1),
			dir:  markerTestDir, code: MarkerErrorUnsupportedVersion,
		},
		{
			name: "missing required", body: `{"version":1}`,
			dir: markerTestDir, code: MarkerErrorInvalidFields,
		},
		{
			name: "wrong directory", body: valid,
			dir:  "bbbbbbbbbbbbbbbbbbbbbbbbbb",
			code: MarkerErrorInvalidFields,
		},
		{
			name: "non UTC", body: strings.Replace(valid,
				`2026-07-11T09:00:00Z`, `2026-07-11T10:00:00+01:00`, 1),
			dir: markerTestDir, code: MarkerErrorInvalidFields,
		},
		{
			name: "unsafe page", body: strings.Replace(valid,
				`plan.html`, `../plan.html`, 1), dir: markerTestDir,
			code: MarkerErrorInvalidFields,
		},
		{
			name: "source mismatch", body: strings.Replace(valid,
				`plan.md`, `other.md`, 1), dir: markerTestDir,
			code: MarkerErrorInvalidFields,
		},
		{
			name: "HTML source", body: strings.Replace(valid,
				`"format":"md"`, `"format":"html"`, 1), dir: markerTestDir,
			code: MarkerErrorInvalidFields,
		},
		{
			name: "empty optional source", body: strings.Replace(valid,
				`plan.md`, ``, 1), dir: markerTestDir,
			code: MarkerErrorInvalidFields,
		},
		{
			name: "trailing JSON", body: valid + `{}`, dir: markerTestDir,
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
			assertMarkerCode(t, err, tt.code)
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

func TestUploadMarkerV3DocumentValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*UploadMarker)
	}{
		{"missing kind", func(m *UploadMarker) { m.Kind = "" }},
		{"missing slug", func(m *UploadMarker) { m.Slug = "" }},
		{"invalid slug", func(m *UploadMarker) { m.Slug = "has space" }},
		{"page slug mismatch", func(m *UploadMarker) {
			m.Objects[0].Name = "other.html"
		}},
		{"missing format", func(m *UploadMarker) { m.Format = "" }},
		{"unknown format", func(m *UploadMarker) { m.Format = "pdf" }},
		{"missing page", func(m *UploadMarker) { m.Objects = m.Objects[1:] }},
		{"duplicate page", func(m *UploadMarker) {
			m.Objects = append(m.Objects, MarkerObject{
				Name: "other.html", Role: MarkerRolePage, Bytes: 1,
				ContentType: "text/html; charset=utf-8",
			})
		}},
		{"file object", func(m *UploadMarker) {
			m.Objects[1].Role = MarkerRoleFile
		}},
		{"HTML source", func(m *UploadMarker) { m.Format = "html" }},
		{"markdown source mismatch", func(m *UploadMarker) {
			m.Objects[1].Name = "other.md"
		}},
		{"text HTML source", func(m *UploadMarker) {
			m.Format = "txt"
			m.Objects[1].Name = "launch-plan.html"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			marker := validDocumentMarker()
			tt.mutate(&marker)
			_, err := EncodeUploadMarker(marker)
			assertMarkerCode(t, err, MarkerErrorInvalidFields)
		})
	}
}

func TestUploadMarkerV3CollectionValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*UploadMarker)
	}{
		{"slug", func(m *UploadMarker) { m.Slug = "files" }},
		{"format", func(m *UploadMarker) { m.Format = "html" }},
		{"wrong page", func(m *UploadMarker) {
			m.Objects[0].Name = "gallery.html"
		}},
		{"source", func(m *UploadMarker) {
			m.Objects[1].Role = MarkerRoleSource
		}},
		{"no files", func(m *UploadMarker) { m.Objects = m.Objects[:1] }},
		{"negative file bytes", func(m *UploadMarker) { m.Objects[1].Bytes = -1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			marker := validCollectionMarker()
			tt.mutate(&marker)
			_, err := EncodeUploadMarker(marker)
			assertMarkerCode(t, err, MarkerErrorInvalidFields)
		})
	}

	zeroByte := validCollectionMarker()
	zeroByte.Objects[1].Bytes = 0
	if _, err := EncodeUploadMarker(zeroByte); err != nil {
		t.Fatalf("zero-byte collection file: %v", err)
	}

	tooMany := validCollectionMarker()
	tooMany.Objects = tooMany.Objects[:1]
	for i := 0; i < MaxCollectionFiles+1; i++ {
		tooMany.Objects = append(tooMany.Objects, MarkerObject{
			Name: fmt.Sprintf("file-%03d.bin", i), Role: MarkerRoleFile,
			Bytes: 1, ContentType: "application/octet-stream",
		})
	}
	_, err := EncodeUploadMarker(tooMany)
	assertMarkerCode(t, err, MarkerErrorInvalidFields)
}

func TestUploadMarkerV3ObjectValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*MarkerObject)
	}{
		{"empty name", func(o *MarkerObject) { o.Name = "" }},
		{"dot name", func(o *MarkerObject) { o.Name = "." }},
		{"dot dot name", func(o *MarkerObject) { o.Name = ".." }},
		{"slash", func(o *MarkerObject) { o.Name = "sub/file.png" }},
		{"backslash", func(o *MarkerObject) { o.Name = `sub\file.png` }},
		{"control", func(o *MarkerObject) { o.Name = "file\n.png" }},
		{"document marker collision", func(o *MarkerObject) {
			o.Name = MarkerFilename
		}},
		{"collection marker collision", func(o *MarkerObject) {
			o.Name = CollectionMarkerFilename
		}},
		{"unknown role", func(o *MarkerObject) { o.Role = "thumbnail" }},
		{"missing content type", func(o *MarkerObject) { o.ContentType = "" }},
		{"unnormalized content type", func(o *MarkerObject) {
			o.ContentType = "IMAGE/PNG"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			marker := validCollectionMarker()
			tt.mutate(&marker.Objects[1])
			_, err := EncodeUploadMarker(marker)
			assertMarkerCode(t, err, MarkerErrorInvalidFields)
		})
	}

	duplicate := validCollectionMarker()
	duplicate.Objects = append(duplicate.Objects, duplicate.Objects[1])
	_, err := EncodeUploadMarker(duplicate)
	assertMarkerCode(t, err, MarkerErrorInvalidFields)

	zeroPage := validCollectionMarker()
	zeroPage.Objects[0].Bytes = 0
	_, err = EncodeUploadMarker(zeroPage)
	assertMarkerCode(t, err, MarkerErrorInvalidFields)

	wrongPageType := validCollectionMarker()
	wrongPageType.Objects[0].ContentType = "text/plain; charset=utf-8"
	_, err = EncodeUploadMarker(wrongPageType)
	assertMarkerCode(t, err, MarkerErrorInvalidFields)
}

func TestDecodeUploadMarkerV3StrictFields(t *testing.T) {
	t.Parallel()

	body, err := EncodeUploadMarker(validDocumentMarker())
	if err != nil {
		t.Fatal(err)
	}
	valid := string(body)
	tests := []string{
		strings.Replace(valid, `"kind":"document",`, "", 1),
		strings.Replace(valid, `"slug":"launch-plan",`, "", 1),
		strings.Replace(valid, `"objects":[`, `"objects":null,"unused":[`, 1),
		strings.Replace(valid, `"bytes":1234`, `"future":true`, 1),
		strings.TrimSuffix(valid, "}") + `,"page":"launch-plan.html"}`,
		strings.TrimSuffix(valid, "}") + `,"slug":"launch-plan"}`,
	}
	for _, invalid := range tests {
		_, err := DecodeUploadMarker([]byte(invalid), markerTestDir)
		if code, ok := MarkerCode(err); !ok ||
			(code != MarkerErrorInvalidFields && code != MarkerErrorMalformedJSON) {
			t.Fatalf("error = %v, want invalid marker", err)
		}
	}

	withUnknownObjectField := strings.Replace(valid,
		`"content_type":"text/html; charset=utf-8"`,
		`"content_type":"text/html; charset=utf-8","future":true`, 1)
	if _, err := DecodeUploadMarker(
		[]byte(withUnknownObjectField), markerTestDir,
	); err != nil {
		t.Fatalf("unknown object field: %v", err)
	}
}

func TestDecodeUploadMarkerLegacyNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		version   int
		pageBytes int64
		repo      string
	}{
		{"v1", 1, 0, ""},
		{"v2", 2, 42, "https://github.com/acme/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marker := UploadMarker{
				Schema: MarkerSchema, Version: tt.version,
				Directory: markerTestDir, CreatedAt: markerTestTime,
				Format: "md", Page: "plan.html", PageBytes: tt.pageBytes,
				Source: "plan.md", Title: "Plan", Repo: tt.repo,
			}
			body, err := EncodeUploadMarker(marker)
			if err != nil {
				t.Fatal(err)
			}
			got, err := DecodeUploadMarker(body, markerTestDir)
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != UploadKindDocument || got.Slug != "plan" ||
				got.Page != "plan.html" || got.Source != "plan.md" ||
				len(got.Objects) != 2 {
				t.Fatalf("normalized marker = %+v", got)
			}
			if page := got.Objects[0]; page.Role != MarkerRolePage ||
				page.Bytes != tt.pageBytes ||
				page.ContentType != "text/html; charset=utf-8" {
				t.Fatalf("normalized page = %+v", page)
			}
			if source := got.Objects[1]; source.Role != MarkerRoleSource ||
				source.Bytes != 0 ||
				source.ContentType != "text/markdown; charset=utf-8" {
				t.Fatalf("normalized source = %+v", source)
			}
		})
	}
}

func TestDecodeUploadMarkerV1RejectsV2Fields(t *testing.T) {
	t.Parallel()

	validV1 := `{"schema":"airplan-upload","version":1,` +
		`"directory":"abcdefghijklmnopqrstuvwxyz",` +
		`"created_at":"2026-07-21T09:00:00Z","format":"html",` +
		`"page":"plan.html"`
	for _, extension := range []string{
		`,"page_bytes":42`,
		`,"page_bytes":0`,
		`,"page_bytes":{"future":true}`,
		`,"repo":"https://github.com/acme/repo"`,
		`,"repo":""`,
		`,"repo":["future","metadata"]`,
	} {
		_, err := DecodeUploadMarker(
			[]byte(validV1+extension+`}`), markerTestDir,
		)
		assertMarkerCode(t, err, MarkerErrorInvalidFields)
	}

	validV2 := `{"schema":"airplan-upload","version":2,` +
		`"directory":"abcdefghijklmnopqrstuvwxyz",` +
		`"created_at":"2026-07-21T09:00:00Z","format":"html",` +
		`"page":"plan.html","page_bytes":42,` +
		`"repo":"https://github.com/acme/repo"}`
	for _, invalidV2 := range []string{
		strings.Replace(validV2, `"page_bytes":42`,
			`"page_bytes":{"future":true}`, 1),
		strings.Replace(validV2, `"repo":"https://github.com/acme/repo"`,
			`"repo":["future","metadata"]`, 1),
	} {
		_, err := DecodeUploadMarker([]byte(invalidV2), markerTestDir)
		assertMarkerCode(t, err, MarkerErrorInvalidFields)
	}
}

func TestDecodeLegacyMarkerRequiresDocumentFilename(t *testing.T) {
	t.Parallel()

	marker := UploadMarker{
		Schema: MarkerSchema, Version: 2, Directory: markerTestDir,
		CreatedAt: markerTestTime, Format: "html", Page: "plan.html",
		PageBytes: 42,
	}
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeUploadMarkerForName(
		body, markerTestDir, CollectionMarkerFilename,
	)
	assertMarkerCode(t, err, MarkerErrorInvalidFields)
}

func TestUploadMarkerRepositoryValidation(t *testing.T) {
	t.Parallel()

	for _, repo := range []string{
		"git@github.com:acme/repo.git",
		"auto",
	} {
		marker := validDocumentMarker()
		marker.Repo = repo
		_, err := EncodeUploadMarker(marker)
		assertMarkerCode(t, err, MarkerErrorInvalidFields)
	}
}

func TestEncodeUploadMarkerOversized(t *testing.T) {
	t.Parallel()

	marker := validCollectionMarker()
	marker.Title = strings.Repeat("a", MaxMarkerSize)
	_, err := EncodeUploadMarker(marker)
	assertMarkerCode(t, err, MarkerErrorOversized)
}

func validDocumentMarker() UploadMarker {
	return UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion,
		Directory: markerTestDir, CreatedAt: markerTestTime,
		Kind: UploadKindDocument, Slug: "launch-plan", Format: "md",
		Objects: []MarkerObject{
			{
				Name: "launch-plan.html", Role: MarkerRolePage, Bytes: 1234,
				ContentType: "text/html; charset=utf-8",
			},
			{
				Name: "launch-plan.md", Role: MarkerRoleSource, Bytes: 321,
				ContentType: "text/markdown; charset=utf-8",
			},
		},
		Title: "Launch plan", Repo: "https://github.com/acme/airplan",
	}
}

func validCollectionMarker() UploadMarker {
	return UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion,
		Directory: markerTestDir, CreatedAt: markerTestTime,
		Kind: UploadKindCollection,
		Objects: []MarkerObject{
			{
				Name: "index.html", Role: MarkerRolePage, Bytes: 900,
				ContentType: "text/html; charset=utf-8",
			},
			{
				Name: "screenshot.png", Role: MarkerRoleFile, Bytes: 456,
				ContentType: "image/png",
			},
		},
		Title: "Feature evidence",
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
