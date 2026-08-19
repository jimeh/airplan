package airplan

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const markerTestDir = "abcdefghijklmnopqrstuvwxyz"

var markerTestTime = time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)

func TestUploadMarkerV6DocumentRoundTrip(t *testing.T) {
	t.Parallel()

	marker := validDocumentMarker()
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"page", "page_bytes", "source"} {
		if _, exists := fields[legacy]; exists {
			t.Fatalf("marker contains legacy top-level field %q: %s", legacy, body)
		}
	}

	got, err := DecodeUploadMarker(body, markerTestDir)
	if err != nil {
		t.Fatal(err)
	}
	want := marker
	want.Page = "launch-plan.html"
	want.PageBytes = 1234
	want.PageSHA256 = strings.Repeat("a", 64)
	want.Source = "launch-plan.md"
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("marker = %+v, want %+v", *got, want)
	}
}

func TestUploadMarkerV5DocumentRoundTrip(t *testing.T) {
	t.Parallel()

	marker := validDocumentMarkerV5()
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeUploadMarker(body, markerTestDir)
	if err != nil {
		t.Fatal(err)
	}
	want := marker
	want.Page = "launch-plan.html"
	want.PageBytes = 1234
	want.PageSHA256 = strings.Repeat("a", 64)
	want.Source = "launch-plan.md"
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("marker = %+v, want %+v", *got, want)
	}
}

func TestUploadMarkerV6CollectionRoundTrip(t *testing.T) {
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
	want.PageSHA256 = strings.Repeat("b", 64)
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("marker = %+v, want %+v", *got, want)
	}
}

func TestUploadMarkerV5CollectionRoundTrip(t *testing.T) {
	t.Parallel()

	marker := validCollectionMarkerV5()
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
	want.PageSHA256 = strings.Repeat("b", 64)
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("marker = %+v, want %+v", *got, want)
	}
}

func TestUploadMarkerV3RemainsReadableWithoutProvenance(t *testing.T) {
	t.Parallel()
	marker := validDocumentMarkerV3()
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeUploadMarker(body, markerTestDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 3 || got.Producer.Name != "" || got.Render != nil {
		t.Fatalf("legacy marker = %+v", got)
	}
}

func TestUploadMarkerV3IgnoresV4ProvenanceFields(t *testing.T) {
	t.Parallel()
	marker := validDocumentMarkerV3()
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"producer"`) ||
		strings.Contains(string(body), `"render"`) ||
		strings.Contains(string(body), `"sha256"`) {
		t.Fatalf("v3 encoder emitted v4 provenance: %s", body)
	}
	withUnknownFields := strings.Replace(string(body),
		`"content_type":"text/html; charset=utf-8"`,
		`"content_type":"text/html; charset=utf-8","sha256":{"future":true}`, 1)
	withUnknownFields = strings.TrimSuffix(withUnknownFields, "}") +
		`,"producer":{"name":"airplan","version":"99.0.0"},` +
		`"render":{"generation":99,"template":{"kind":"builtin"}}}`
	got, err := DecodeUploadMarker([]byte(withUnknownFields), markerTestDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Producer != (Producer{}) || got.Render != nil ||
		got.PageSHA256 != "" || got.Objects[0].SHA256 != "" {
		t.Fatalf("v3 unknown provenance became trusted: %+v", got)
	}
}

func TestUploadMarkerV5RequiresStringLowercasePageSHA(t *testing.T) {
	t.Parallel()
	body, err := EncodeUploadMarker(validDocumentMarkerV5())
	if err != nil {
		t.Fatal(err)
	}
	encodedSHA := `"sha256":"` + strings.Repeat("a", 64) + `"`
	for _, test := range []struct {
		name        string
		replacement string
	}{
		{"object value", `"sha256":{"future":true}`},
		{"uppercase hex", `"sha256":"` + strings.Repeat("A", 64) + `"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			modified := strings.Replace(string(body), encodedSHA, test.replacement, 1)
			if _, err := DecodeUploadMarker([]byte(modified), markerTestDir); err == nil {
				t.Fatalf("v5 marker accepted %s sha256", test.name)
			}
		})
	}
}

func TestUploadMarkerV5RequiresApplicableProvenance(t *testing.T) {
	t.Parallel()
	missingProducer := validDocumentMarkerV5()
	missingProducer.Producer = Producer{}
	if _, err := EncodeUploadMarker(missingProducer); err == nil {
		t.Fatal("v5 marker without producer was accepted")
	}
	missingRender := validDocumentMarkerV5()
	missingRender.Render = nil
	if _, err := EncodeUploadMarker(missingRender); err == nil {
		t.Fatal("generated v5 document without render recipe was accepted")
	}
	missingThemes := validDocumentMarkerV5()
	missingThemes.Render.Themes = nil
	if _, err := EncodeUploadMarker(missingThemes); err == nil {
		t.Fatal("v5 marker without theme provenance was accepted")
	}
	missingPageSHA := validDocumentMarkerV5()
	missingPageSHA.Objects[0].SHA256 = ""
	if _, err := EncodeUploadMarker(missingPageSHA); err == nil {
		t.Fatal("v5 marker without generated-page checksum was accepted")
	}
	sourceSHA := validDocumentMarkerV5()
	sourceSHA.Objects[1].SHA256 = strings.Repeat("b", 64)
	if _, err := EncodeUploadMarker(sourceSHA); err == nil {
		t.Fatal("v5 marker with a source checksum was accepted")
	}
	authoredHTML := validDocumentMarkerV5()
	authoredHTML.Format = "html"
	authoredHTML.Slug = "page"
	authoredHTML.Objects = []MarkerObject{{
		Name: "page.html", Role: MarkerRolePage, Bytes: 5,
		ContentType: pageContentType, SHA256: strings.Repeat("a", 64),
	}}
	authoredHTML.Render = nil
	if _, err := EncodeUploadMarker(authoredHTML); err != nil {
		t.Fatalf("authored HTML marker rejected: %v", err)
	}
	authoredHTML.Render = documentRenderRecipe(&Config{MermaidURL: DefaultMermaidURL}, "")
	if _, err := EncodeUploadMarker(authoredHTML); err == nil {
		t.Fatal("authored HTML marker with render recipe was accepted")
	}
}

func TestUploadMarkerV5ValidatesThemeDefaultIDs(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"default_light", "default_dark"} {
		for _, value := range []string{
			"Tokyo-Night", "bad\x01theme", strings.Repeat("a", maxThemeIDLength+1),
		} {
			t.Run(field+"/"+fmt.Sprintf("%q", value), func(t *testing.T) {
				marker := validDocumentMarkerV5()
				if field == "default_light" {
					marker.Render.Themes.DefaultLight = value
				} else {
					marker.Render.Themes.DefaultDark = value
				}
				if _, err := EncodeUploadMarker(marker); err == nil ||
					!strings.Contains(err.Error(), field) {
					t.Fatalf("theme default %s=%q error = %v", field, value, err)
				}
			})
		}
	}

	marker := validDocumentMarkerV5()
	marker.Render.Themes.DefaultLight = "historical-custom-light"
	marker.Render.Themes.DefaultDark = "historical-custom-dark"
	if _, err := EncodeUploadMarker(marker); err != nil {
		t.Fatalf("historical custom theme IDs rejected: %v", err)
	}
}

func TestUploadMarkerV4RevisionRoundTripPreservesProvenance(t *testing.T) {
	marker := validDocumentMarkerV5()
	marker.Version = 4
	marker.Render.Generation = 2
	originalThemes := marker.Render.Themes
	marker.Revision = &RevisionDescriptor{
		ChainID: strings.Repeat("r", 26), Number: 2,
		PreviousURL: "https://plans.example.test/" + strings.Repeat("p", 26) + "/launch-plan.html",
	}
	marker.Objects = append(marker.Objects, MarkerObject{
		Name: DiffFilename, Role: MarkerRoleDiff, Bytes: 8, ContentType: diffContentType,
	})
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	if marker.Render.Themes != originalThemes || marker.Render.Themes == nil {
		t.Fatal("v4 encoding mutated the caller's theme-aware render recipe")
	}
	decoded, err := DecodeUploadMarker(body, markerTestDir)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Version != 4 || decoded.Producer != marker.Producer ||
		decoded.Render == nil || decoded.Render.Generation != 2 || decoded.Render.Themes != nil ||
		decoded.Revision == nil || decoded.Revision.Number != 2 || decoded.PageSHA256 == "" {
		t.Fatalf("v4 compatibility fields were stripped: %+v", decoded)
	}
}

func TestV5CompatibleDecoderRejectsV6BeforeRewrite(t *testing.T) {
	body, err := EncodeUploadMarker(validDocumentMarker())
	if err != nil {
		t.Fatal(err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		t.Fatal(err)
	}
	if header.Version <= 5 {
		t.Fatalf("fixture version = %d, want newer than v5", header.Version)
	}
	decodeV5 := func(version int) error {
		if version < 1 || version > 5 {
			return errors.New("unsupported marker version")
		}
		return nil
	}
	if err := decodeV5(header.Version); err == nil {
		t.Fatal("v5-compatible decoder accepted v6")
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

	document, err := EncodeUploadMarker(validDocumentMarkerV3())
	if err != nil {
		t.Fatal(err)
	}
	collection, err := EncodeUploadMarker(validCollectionMarkerV3())
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
			body: strings.Replace(valid, `"version":1`, `"version":7`, 1),
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
			marker := validDocumentMarkerV3()
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
			marker := validCollectionMarkerV3()
			tt.mutate(&marker)
			_, err := EncodeUploadMarker(marker)
			assertMarkerCode(t, err, MarkerErrorInvalidFields)
		})
	}

	zeroByte := validCollectionMarkerV3()
	zeroByte.Objects[1].Bytes = 0
	if _, err := EncodeUploadMarker(zeroByte); err != nil {
		t.Fatalf("zero-byte collection file: %v", err)
	}

	tooMany := validCollectionMarkerV3()
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
		{"protection sentinel collision", func(o *MarkerObject) {
			o.Name = ProtectedFilename
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
			marker := validCollectionMarkerV3()
			tt.mutate(&marker.Objects[1])
			_, err := EncodeUploadMarker(marker)
			assertMarkerCode(t, err, MarkerErrorInvalidFields)
		})
	}

	duplicate := validCollectionMarkerV3()
	duplicate.Objects = append(duplicate.Objects, duplicate.Objects[1])
	_, err := EncodeUploadMarker(duplicate)
	assertMarkerCode(t, err, MarkerErrorInvalidFields)

	zeroPage := validCollectionMarkerV3()
	zeroPage.Objects[0].Bytes = 0
	_, err = EncodeUploadMarker(zeroPage)
	assertMarkerCode(t, err, MarkerErrorInvalidFields)

	wrongPageType := validCollectionMarkerV3()
	wrongPageType.Objects[0].ContentType = "text/plain; charset=utf-8"
	_, err = EncodeUploadMarker(wrongPageType)
	assertMarkerCode(t, err, MarkerErrorInvalidFields)
}

func TestUploadMarkerV4ReservesControlPrefix(t *testing.T) {
	t.Parallel()

	marker := validDocumentMarkerV5()
	marker.Version = 4
	marker.Objects[1].Name = ".airplan-user-document.txt"
	_, err := EncodeUploadMarker(marker)
	assertMarkerCode(t, err, MarkerErrorInvalidFields)
}

func TestDecodeUploadMarkerV3StrictFields(t *testing.T) {
	t.Parallel()

	body, err := EncodeUploadMarker(validDocumentMarkerV3())
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

func TestUploadMarkerV6NestedBundleRoundTrip(t *testing.T) {
	t.Parallel()

	marker := validBundleMarkerV6()
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeUploadMarker(body, markerTestDir)
	if err != nil {
		t.Fatal(err)
	}
	want := marker
	want.Page = marker.Entrypoint
	want.PageBytes = marker.Objects[0].Bytes
	want.PageSHA256 = marker.Objects[0].SHA256
	want.Source = marker.Pages[0].Source
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("marker = %+v, want %+v", *got, want)
	}
	if got.Pages[1].Lang != "python" {
		t.Fatalf("text-page language = %q, want explicit python", got.Pages[1].Lang)
	}
}

func TestUploadMarkerV6TextDocumentPageRules(t *testing.T) {
	t.Parallel()

	marker := validDocumentMarker()
	marker.Slug = "notes"
	marker.Format = "txt"
	marker.Entrypoint = "notes.html"
	marker.Pages = []MarkerPage{{
		Path: "notes.txt", Page: "notes.html", Source: "notes.txt",
		Format: "txt", Title: "Notes", Lang: "text",
	}}
	marker.Objects = []MarkerObject{
		{Name: "notes.html", Role: MarkerRolePage, Bytes: 20, ContentType: pageContentType, SHA256: strings.Repeat("a", 64)},
		{Name: "notes.txt", Role: MarkerRoleSource, Bytes: 5, ContentType: textContentType, SHA256: strings.Repeat("b", 64)},
		{Name: "evidence.bin", Role: MarkerRoleAsset, Bytes: 3, ContentType: "application/octet-stream", SHA256: strings.Repeat("c", 64)},
	}
	if _, err := EncodeUploadMarker(marker); err != nil {
		t.Fatalf("single-entry text marker with asset rejected: %v", err)
	}

	marker.Pages = append(marker.Pages, MarkerPage{
		Path: "details.md", Page: "details.html", Source: "details.md",
		Format: "md", Title: "Details", Lang: "",
	})
	marker.Objects = append(marker.Objects,
		MarkerObject{Name: "details.html", Role: MarkerRolePage, Bytes: 20, ContentType: pageContentType, SHA256: strings.Repeat("d", 64)},
		MarkerObject{Name: "details.md", Role: MarkerRoleSource, Bytes: 5, ContentType: sourceContentType, SHA256: strings.Repeat("e", 64)},
	)
	_, err := EncodeUploadMarker(marker)
	if err == nil || !strings.Contains(err.Error(), "text documents must contain exactly one managed page") {
		t.Fatalf("error = %v", err)
	}
}

func TestUploadMarkerV6ValidatesNestedPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*UploadMarker)
	}{
		{"traversal", func(m *UploadMarker) { m.Objects[4].Name = "images/../flow.svg" }},
		{"empty segment", func(m *UploadMarker) { m.Objects[4].Name = "images//flow.svg" }},
		{"backslash", func(m *UploadMarker) { m.Objects[4].Name = `images\flow.svg` }},
		{"reserved nested segment", func(m *UploadMarker) {
			m.Objects[4].Name = "images/.airplan-private/flow.svg"
		}},
		{"case-folded object collision", func(m *UploadMarker) {
			m.Objects = append(m.Objects, MarkerObject{
				Name: "IMAGES/FLOW.SVG", Role: MarkerRoleAsset, Bytes: 1,
				ContentType: "image/svg+xml", SHA256: strings.Repeat("f", 64),
			})
		}},
		{"case-folded page path collision", func(m *UploadMarker) {
			m.Pages[0].Path = "EXAMPLES/SERVER.GO"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			marker := validBundleMarkerV6()
			test.mutate(&marker)
			_, err := EncodeUploadMarker(marker)
			assertMarkerCode(t, err, MarkerErrorInvalidFields)
		})
	}
}

func TestUploadMarkerV6RequiresEveryPayloadDigest(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name   string
		marker UploadMarker
	}{
		{"document bundle", validBundleMarkerV6()},
		{"collection", validCollectionMarker()},
		{"revision", validRevisionMarkerV6()},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			for index, object := range fixture.marker.Objects {
				index, object := index, object
				t.Run(object.Name, func(t *testing.T) {
					marker := fixture.marker
					marker.Objects = append([]MarkerObject(nil), marker.Objects...)
					marker.Objects[index].SHA256 = ""
					_, err := EncodeUploadMarker(marker)
					assertMarkerCode(t, err, MarkerErrorInvalidFields)
					if !strings.Contains(err.Error(), object.Name) {
						t.Fatalf("error = %v, want object name %q", err, object.Name)
					}
				})
			}
		})
	}
}

func TestUploadMarkerVersionsBeforeV6RejectBundleFields(t *testing.T) {
	t.Parallel()

	for _, version := range []int{3, 4, 5} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			marker := validDocumentMarkerV5()
			marker.Version = version
			marker.Entrypoint = "launch-plan.html"
			marker.Pages = []MarkerPage{{
				Path: "README.md", Page: "launch-plan.html",
				Source: "launch-plan.md", Format: "md", Lang: "",
			}}
			_, err := EncodeUploadMarker(marker)
			assertMarkerCode(t, err, MarkerErrorInvalidFields)
		})
	}
}

func TestUploadMarkerV6MaximumEncodedSize(t *testing.T) {
	t.Parallel()

	marker := validDocumentMarker()
	marker.Title = ""
	for index := 0; index < MaxDocumentItems-1; index++ {
		marker.Objects = append(marker.Objects, MarkerObject{
			Name: fmt.Sprintf("artifacts/group-%02d/evidence-%02d.bin", index, index),
			Role: MarkerRoleAsset, Bytes: int64(index),
			ContentType: "application/octet-stream", SHA256: strings.Repeat("d", 64),
		})
	}
	base, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	const titleFieldOverhead = len(`,"title":""`)
	marker.Title = strings.Repeat("x", MaxMarkerSize-len(base)-titleFieldOverhead)
	exact, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	if len(exact) != MaxMarkerSize {
		t.Fatalf("encoded marker size = %d, want %d", len(exact), MaxMarkerSize)
	}
	if _, err := DecodeUploadMarker(exact, markerTestDir); err != nil {
		t.Fatalf("decode marker at size limit: %v", err)
	}

	marker.Title += "x"
	_, err = EncodeUploadMarker(marker)
	assertMarkerCode(t, err, MarkerErrorOversized)
	_, err = DecodeUploadMarker(append(exact, ' '), markerTestDir)
	assertMarkerCode(t, err, MarkerErrorOversized)
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
	marker := validDocumentMarkerV5()
	marker.Version = MarkerVersion
	marker.Entrypoint = "launch-plan.html"
	marker.Pages = []MarkerPage{{
		Path: "README.md", Page: "launch-plan.html", Source: "launch-plan.md",
		Format: "md", Title: "Launch plan", Lang: "",
	}}
	marker.Objects[1].SHA256 = strings.Repeat("b", 64)
	return marker
}

func validDocumentMarkerV5() UploadMarker {
	return UploadMarker{
		Schema: MarkerSchema, Version: 5,
		Directory: markerTestDir, CreatedAt: markerTestTime,
		Kind: UploadKindDocument, Slug: "launch-plan", Format: "md",
		Objects: []MarkerObject{
			{
				Name: "launch-plan.html", Role: MarkerRolePage, Bytes: 1234,
				ContentType: "text/html; charset=utf-8", SHA256: strings.Repeat("a", 64),
			},
			{
				Name: "launch-plan.md", Role: MarkerRoleSource, Bytes: 321,
				ContentType: "text/markdown; charset=utf-8",
			},
		},
		Title: "Launch plan", Repo: "https://github.com/acme/airplan",
		Producer: Producer{Name: "airplan", Version: "dev"},
		Render:   documentRenderRecipe(&Config{MermaidURL: DefaultMermaidURL}, ""),
	}
}

func validDocumentMarkerV3() UploadMarker {
	marker := validDocumentMarkerV5()
	marker.Version = 3
	marker.Producer = Producer{}
	marker.Render = nil
	marker.Objects[0].SHA256 = ""
	return marker
}

func validBundleMarkerV6() UploadMarker {
	marker := validDocumentMarker()
	marker.Pages = append(marker.Pages, MarkerPage{
		Path: "examples/server.go", Page: "examples/server.go.html",
		Source: "examples/server.go", Format: "txt", Title: "server.go",
		Lang: "python",
	})
	marker.Objects = append(marker.Objects,
		MarkerObject{
			Name: "examples/server.go.html", Role: MarkerRolePage, Bytes: 850,
			ContentType: pageContentType, SHA256: strings.Repeat("c", 64),
		},
		MarkerObject{
			Name: "examples/server.go", Role: MarkerRoleSource, Bytes: 120,
			ContentType: "text/plain; charset=utf-8", SHA256: strings.Repeat("d", 64),
		},
		MarkerObject{
			Name: "images/flow.svg", Role: MarkerRoleAsset, Bytes: 456,
			ContentType: "image/svg+xml", SHA256: strings.Repeat("e", 64),
		},
	)
	return marker
}

func validRevisionMarkerV6() UploadMarker {
	marker := validBundleMarkerV6()
	marker.Revision = &RevisionDescriptor{
		ChainID: strings.Repeat("r", 26), Number: 2,
		PreviousURL: "https://plans.example.test/" +
			strings.Repeat("p", 26) + "/launch-plan.html",
	}
	marker.Objects = append(marker.Objects, MarkerObject{
		Name: DiffFilename, Role: MarkerRoleDiff, Bytes: 80,
		ContentType: diffContentType, SHA256: strings.Repeat("f", 64),
	})
	return marker
}

func validCollectionMarker() UploadMarker {
	marker := validCollectionMarkerV5()
	marker.Version = MarkerVersion
	marker.Objects[1].SHA256 = strings.Repeat("c", 64)
	return marker
}

func validCollectionMarkerV5() UploadMarker {
	return UploadMarker{
		Schema: MarkerSchema, Version: 5,
		Directory: markerTestDir, CreatedAt: markerTestTime,
		Kind: UploadKindCollection,
		Objects: []MarkerObject{
			{
				Name: "index.html", Role: MarkerRolePage, Bytes: 900,
				ContentType: "text/html; charset=utf-8", SHA256: strings.Repeat("b", 64),
			},
			{
				Name: "screenshot.png", Role: MarkerRoleFile, Bytes: 456,
				ContentType: "image/png",
			},
		},
		Title:    "Feature evidence",
		Producer: Producer{Name: "airplan", Version: "dev"},
		Render:   collectionRenderRecipe(&Config{}, ""),
	}
}

func validCollectionMarkerV3() UploadMarker {
	marker := validCollectionMarkerV5()
	marker.Version = 3
	marker.Producer = Producer{}
	marker.Render = nil
	marker.Objects[0].SHA256 = ""
	return marker
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
