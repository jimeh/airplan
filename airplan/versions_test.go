package airplan

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestVersionsMetadataRoundTripAndTombstoneHighWater(t *testing.T) {
	cfg := &Config{PublicBaseURL: "https://plans.example.com", KeyPrefix: "docs"}
	chain := strings.Repeat("a", 26)
	when := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	metadata := VersionsMetadata{
		Schema: "airplan-versions", Version: 1,
		ChainID: chain, CurrentRevision: 1, LatestRevision: 3,
		LastAssignedRevision: 3, Revisions: []VersionsRevision{
			{Number: 1, URL: "https://plans.example.com/docs/" + chain + "/plan.html", CreatedAt: when},
			{Number: 2, Deleted: true, DeletedAt: when},
			{
				Number: 3, URL: "https://plans.example.com/docs/bbbbbbbbbbbbbbbbbbbbbbbbbb/plan.html", CreatedAt: when,
				DiffURL: "https://plans.example.com/docs/bbbbbbbbbbbbbbbbbbbbbbbbbb/" + DiffFilename,
			},
		},
	}
	body, err := EncodeVersionsMetadata(metadata, cfg, "docs/"+chain+"/plan.html")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeVersionsMetadata(body, cfg, "docs/"+chain+"/plan.html")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.LastAssignedRevision != 3 || !decoded.Revisions[1].Deleted {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestVersionsMetadataRejectsMalformedWireBodies(t *testing.T) {
	cfg := &Config{PublicBaseURL: "https://plans.example.com"}
	for _, test := range []struct {
		name string
		body []byte
	}{
		{name: "malformed", body: []byte(`{"schema":`)},
		{name: "duplicate name", body: []byte(`{"schema":"airplan-versions","schema":"airplan-versions","version":1}`)},
		{name: "trailing value", body: []byte(`{} {}`)},
		{name: "oversized", body: bytes.Repeat([]byte(" "), MaxVersionsMetadataSize+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeVersionsMetadata(test.body, cfg, ""); err == nil {
				t.Fatal("invalid versions metadata accepted")
			}
		})
	}
}

func TestVersionsMetadataRejectsStructuralInconsistency(t *testing.T) {
	cfg := &Config{PublicBaseURL: "https://plans.example.com"}
	when := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	dir1 := strings.Repeat("b", 26)
	dir2 := strings.Repeat("c", 26)
	base := VersionsMetadata{
		Schema: "airplan-versions", Version: 1,
		ChainID: strings.Repeat("a", 26), CurrentRevision: 1,
		LatestRevision: 2, LastAssignedRevision: 2,
		Revisions: []VersionsRevision{
			{Number: 1, URL: "https://plans.example.com/" + dir1 + "/plan.html", CreatedAt: when},
			{
				Number: 2, URL: "https://plans.example.com/" + dir2 + "/plan.html", CreatedAt: when,
				DiffURL: "https://plans.example.com/" + dir2 + "/" + DiffFilename,
			},
		},
	}
	tests := []struct {
		name   string
		mutate func(*VersionsMetadata)
	}{
		{name: "empty", mutate: func(metadata *VersionsMetadata) { metadata.Revisions = nil }},
		{name: "duplicate number", mutate: func(metadata *VersionsMetadata) { metadata.Revisions[1].Number = 1 }},
		{name: "out of order", mutate: func(metadata *VersionsMetadata) {
			metadata.Revisions[0], metadata.Revisions[1] = metadata.Revisions[1], metadata.Revisions[0]
		}},
		{name: "missing assigned number", mutate: func(metadata *VersionsMetadata) {
			metadata.Revisions[1].Number = 3
			metadata.LatestRevision, metadata.LastAssignedRevision = 3, 3
		}},
		{name: "current missing", mutate: func(metadata *VersionsMetadata) { metadata.CurrentRevision = 3 }},
		{name: "latest inconsistent", mutate: func(metadata *VersionsMetadata) { metadata.LatestRevision = 1 }},
		{name: "high water below", mutate: func(metadata *VersionsMetadata) { metadata.LastAssignedRevision = 1 }},
		{name: "high water above", mutate: func(metadata *VersionsMetadata) { metadata.LastAssignedRevision = 3 }},
		{name: "cross scope URL", mutate: func(metadata *VersionsMetadata) {
			metadata.Revisions[0].URL = "https://evil.example/" + dir1 + "/plan.html"
		}},
		{name: "diff outside member", mutate: func(metadata *VersionsMetadata) {
			metadata.Revisions[1].DiffURL = "https://plans.example.com/" + dir1 + "/" + DiffFilename
		}},
		{name: "invalid tombstone", mutate: func(metadata *VersionsMetadata) {
			metadata.Revisions[1] = VersionsRevision{Number: 2, Deleted: true}
			metadata.CurrentRevision, metadata.LatestRevision = 1, 1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := base
			metadata.Revisions = append([]VersionsRevision(nil), base.Revisions...)
			test.mutate(&metadata)
			if _, err := EncodeVersionsMetadata(metadata, cfg, dir1+"/plan.html"); err == nil {
				t.Fatal("inconsistent versions metadata accepted")
			}
		})
	}
}

func TestVersionsMetadataRejectsNoncanonicalAndNonAirplanURLs(t *testing.T) {
	cfg := &Config{PublicBaseURL: "https://plans.example.com"}
	when := time.Now().UTC().Truncate(time.Second)
	base := VersionsMetadata{
		Schema: "airplan-versions", Version: 1,
		ChainID: strings.Repeat("a", 26), CurrentRevision: 1,
		LatestRevision: 2, LastAssignedRevision: 2,
		Revisions: []VersionsRevision{
			{Number: 1, URL: "https://plans.example.com/" + strings.Repeat("b", 26) + "/plan.html", CreatedAt: when},
			{
				Number: 2, URL: "https://plans.example.com/" + strings.Repeat("c", 26) + "/plan.html", CreatedAt: when,
				DiffURL: "https://plans.example.com/" + strings.Repeat("c", 26) + "/" + DiffFilename,
			},
		},
	}
	for _, invalidURL := range []string{
		"https://plans.example.com/not-an-upload/plan.html",
		"https://plans.example.com/" + strings.Repeat("b", 26) + "/%70lan.html",
	} {
		metadata := base
		metadata.Revisions = append([]VersionsRevision(nil), base.Revisions...)
		metadata.Revisions[0].URL = invalidURL
		if _, err := EncodeVersionsMetadata(metadata, cfg, ""); err == nil {
			t.Fatalf("invalid URL %q accepted", invalidURL)
		}
	}
	metadata := base
	metadata.Revisions = append([]VersionsRevision(nil), base.Revisions...)
	metadata.Revisions[0].Number = 2
	metadata.Revisions[1].Number = 3
	metadata.LatestRevision, metadata.LastAssignedRevision = 3, 3
	if _, err := EncodeVersionsMetadata(metadata, cfg, ""); err == nil {
		t.Fatal("revision list not beginning at 1 accepted")
	}
}

func TestMarkerV4RevisionDiffRole(t *testing.T) {
	dir := strings.Repeat("a", 26)
	marker := UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion,
		Directory: dir, CreatedAt: time.Now().UTC().Truncate(time.Second),
		Kind: UploadKindDocument, Slug: "plan", Format: "md",
		Producer: Producer{Name: "airplan", Version: "dev"},
		Render: &RenderRecipe{
			Generation: RendererGeneration,
			Template:   RenderTemplate{Kind: "builtin"}, MermaidURL: DefaultMermaidURL,
		},
		Revision: &RevisionDescriptor{
			ChainID: strings.Repeat("b", 26), Number: 2,
			PreviousURL: "https://plans.example.com/" + dir + "/plan.html",
		},
		Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: 10, ContentType: pageContentType, SHA256: strings.Repeat("a", 64)},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: 2, ContentType: sourceContentType},
			{Name: DiffFilename, Role: MarkerRoleDiff, Bytes: 3, ContentType: diffContentType},
		},
	}
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeUploadMarker(body, dir)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Revision == nil || decoded.Revision.Number != 2 {
		t.Fatalf("decoded = %#v", decoded)
	}
	marker.Revision = nil
	if _, err := EncodeUploadMarker(marker); err == nil {
		t.Fatal("standalone diff accepted")
	}
}
