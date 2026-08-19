package airplan

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUpgradeDocumentMigratesV3WithoutVersionsMetadata(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("u", 26)
	created := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	source := []byte("# Original\n\nBody.\n")
	oldPage := []byte("<html>old</html>")
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir, CreatedAt: created,
		Kind: UploadKindDocument, Slug: "plan", Format: "md", Title: "Original",
		Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(oldPage)), ContentType: pageContentType},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, marker)
	store.set(dir+"/plan.html", oldPage)
	store.set(dir+"/plan.md", source)
	store.set(dir+"/"+ProtectedFilename, []byte(`{"protected":true}`))
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := store.client(t, manifest)

	plan, err := client.PlanUpgradeDocument(context.Background(), dir,
		UpgradeDocumentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != UpgradeStateUpgradeable || plan.CurrentMarkerVersion != 3 || store.puts != 0 {
		t.Fatalf("plan = %+v, puts = %d", plan, store.puts)
	}
	result, err := client.UpgradeDocument(context.Background(), *plan)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Upgraded || result.Result.URL == "" {
		t.Fatalf("result = %+v", result)
	}
	if len(store.putKeys) != 2 || store.putKeys[0] != dir+"/"+MarkerFilename ||
		store.putKeys[1] != dir+"/plan.html" {
		t.Fatalf("put order = %q", store.putKeys)
	}
	gotMarker, ok := store.get(dir + "/" + MarkerFilename)
	if !ok {
		t.Fatal("marker missing")
	}
	decoded, err := DecodeUploadMarker(gotMarker, dir)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Version != MarkerVersion || decoded.Producer.Name != "airplan" ||
		decoded.Render == nil || decoded.Render.Generation != RendererGeneration {
		t.Fatalf("marker = %+v", decoded)
	}
	if got, _ := store.get(dir + "/plan.md"); string(got) != string(source) {
		t.Fatal("source changed")
	}
	if _, exists := store.get(dir + "/.airplan-versions.json"); exists {
		t.Fatal("standalone upgrade created versions metadata")
	}
	if _, exists := store.get(dir + "/" + ProtectedFilename); !exists {
		t.Fatal("protection was removed")
	}
	records, warnings, err := ReadManifest(manifest)
	if err != nil || len(warnings) != 0 || len(records) != 1 || records[0].Type != "upgrade" || records[0].CreatedAt != created {
		t.Fatalf("manifest = %+v, warnings = %v, err = %v", records, warnings, err)
	}
}

func TestUpgradeDocumentRepairsBundlePagesEntryLastWithoutIdentityMarkerPut(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	result, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry:         PageInput{Reader: strings.NewReader("# Entry\n\n[Details](docs/details.md)\n"), Path: "plan.md"},
		Pages:         []PageInput{{Reader: strings.NewReader("# Details\n"), Path: "docs/details.md"}},
		RepositoryURL: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	markerBody, ok := store.get(result.MarkerKey)
	if !ok {
		t.Fatal("marker missing")
	}
	marker, err := DecodeUploadMarker(markerBody, result.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(marker.Pages) != 2 {
		t.Fatalf("pages = %+v", marker.Pages)
	}
	entryKey := result.ID + "/" + marker.Entrypoint
	childKey := result.ID + "/" + marker.Pages[1].Page
	store.set(entryKey, []byte("stale entry"))
	store.set(childKey, []byte("stale child"))
	store.mu.Lock()
	store.puts = 0
	store.putKeys = nil
	store.putAttempts = 0
	store.mu.Unlock()

	plan, err := client.PlanUpgradeDocument(context.Background(), result.URL, UpgradeDocumentOptions{})
	if err != nil || plan.State != UpgradeStateUpgradeable || plan.Reason != "rendered page requires repair" {
		t.Fatalf("plan = %+v, error = %v", plan, err)
	}
	payloads, err := client.spoolUpgradePayloads(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	defer payloads.cleanup()
	if err := client.materializeUpgradeSpooled(context.Background(), plan, payloads); err != nil {
		t.Fatal(err)
	}
	defer plan.newBundle.cleanup()
	if !bytes.Equal(plan.markerBody, plan.newMarker) {
		t.Fatal("repair-only upgrade produced a different desired marker")
	}
	upgraded, err := client.UpgradeDocument(context.Background(), *plan)
	if err != nil || !upgraded.Upgraded {
		t.Fatalf("upgrade = %+v, error = %v", upgraded, err)
	}
	store.mu.Lock()
	putKeys := append([]string(nil), store.putKeys...)
	store.mu.Unlock()
	if len(putKeys) != 2 || putKeys[0] != childKey || putKeys[1] != entryKey {
		t.Fatalf("put order = %q, want child then entry with no marker PUT", putKeys)
	}
}

func TestUpgradeManifestEventPreservesRevisionAndCompleteProjection(t *testing.T) {
	store := newUpgradeStore(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := store.client(t, manifest)
	dir := strings.Repeat("v", 26)
	chain := strings.Repeat("c", 26)
	page := []byte("<html>revised</html>")
	source := []byte("# Revised\n")
	diff := []byte("--- revision-1/plan.md\n+++ revision-2/plan.md\n")
	marker := UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Title: "Revised",
		Repo:     "https://github.com/jimeh/airplan",
		Producer: Producer{Name: "airplan", Version: "0.8.0"},
		Render: &RenderRecipe{
			Generation: RendererGeneration,
			Template:   RenderTemplate{Kind: "builtin"}, MermaidURL: DefaultMermaidURL,
			Themes: themeRecipePtr(defaultThemeBundle()),
		},
		Revision: &RevisionDescriptor{
			ChainID: chain, Number: 2,
			PreviousURL: "https://plans.example.com/" + strings.Repeat("p", 26) + "/plan.html",
		},
		Entrypoint: "plan.html",
		Pages: []MarkerPage{{
			Path: "plan.md", Page: "plan.html", Source: "plan.md",
			Format: "md", Title: "Revised", Lang: "",
		}},
		Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)), ContentType: pageContentType, SHA256: contentSHA256(page)},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType, SHA256: contentSHA256(source)},
			{Name: DiffFilename, Role: MarkerRoleDiff, Bytes: int64(len(diff)), ContentType: diffContentType, SHA256: contentSHA256(diff)},
		},
	}
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	result := Result{
		ID: dir, URL: "https://plans.example.com/" + dir + "/plan.html",
		Key: dir + "/plan.html", SourceKey: dir + "/plan.md", Bucket: "plans",
		Bytes: int64(len(page)), ContentType: pageContentType, Title: marker.Title,
		CreatedAt: marker.CreatedAt, MarkerVersion: MarkerVersion,
		MarkerKey: dir + "/" + MarkerFilename, Format: "md",
		Kind: string(UploadKindDocument), Slug: "plan", RepositoryURL: marker.Repo,
		RevisionChainID: chain, Revision: 2, LatestRevision: 3,
	}
	client.recordUpgrade(context.Background(), &result, markerBody)
	records, warnings, err := ReadManifest(manifest)
	if err != nil || len(warnings) != 0 || len(records) != 1 {
		t.Fatalf("records = %+v, warnings = %v, err = %v", records, warnings, err)
	}
	record := records[0]
	if record.Type != "upgrade" || record.RevisionChainID != chain ||
		record.Revision != 2 || record.LatestRevision != 3 || record.Objects != 4 ||
		record.TotalBytes <= record.Bytes || record.SourceKey != result.SourceKey ||
		record.Repo != marker.Repo || record.Title != marker.Title {
		t.Fatalf("upgrade projection = %+v", record)
	}
}

func TestPlanUpgradeLegacyMarkerWithUndeclaredSourceSize(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("z", 26)
	page := []byte("old")
	source := []byte("# Legacy plan\n")
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Format: "md",
		Page: "plan.html", Source: "plan.md", Title: "Legacy",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, marker)
	store.set(dir+"/plan.html", page)
	store.set(dir+"/plan.md", source)
	client := store.client(t, "")
	plan, err := client.PlanUpgradeDocument(
		context.Background(), dir, UpgradeDocumentOptions{},
	)
	if err != nil || plan.State != UpgradeStateUpgradeable ||
		plan.sourceSizes["plan.md"] != int64(len(source)) ||
		plan.sourceDigests["plan.md"] != contentSHA256(source) {
		t.Fatalf("legacy upgrade plan = %+v, %v", plan, err)
	}
	result, err := client.UpgradeDocument(context.Background(), *plan)
	if err != nil || !result.Upgraded {
		t.Fatalf("legacy upgrade result = %+v, %v", result, err)
	}
}

func TestPlanUpgradeV5ThemeMismatchRequiresForce(t *testing.T) {
	store := newUpgradeStore(t)
	first, err := store.client(t, "").Upload(context.Background(), Input{
		Reader: strings.NewReader("# Plan\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	client := store.clientWithThemes(t, "", "solarized-light", "one-dark")

	plan, err := client.PlanUpgradeDocument(
		context.Background(), first.URL, UpgradeDocumentOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != UpgradeStateIneligible ||
		!strings.Contains(plan.Reason, "themes do not match") ||
		!strings.Contains(plan.Reason, "upgrade --force") {
		t.Fatalf("theme mismatch plan = %+v", plan)
	}
	forced, err := client.PlanUpgradeDocument(
		context.Background(), first.URL, UpgradeDocumentOptions{Force: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if forced.State != UpgradeStateUpgradeable ||
		!strings.Contains(forced.Reason, "forced") {
		t.Fatalf("forced theme mismatch plan = %+v", forced)
	}
}

func TestUpgradeSourceReadLimitUsesDefaultForLegacyUndeclaredSize(t *testing.T) {
	marker := &UploadMarker{Objects: []MarkerObject{{
		Name: "plan.md", Role: MarkerRoleSource,
	}}}
	if got := upgradeSourceReadLimit(marker); got != DefaultMaxInputSize {
		t.Fatalf("legacy source read limit = %d, want %d", got, DefaultMaxInputSize)
	}
	marker.Objects[0].Bytes = 123
	if got := upgradeSourceReadLimit(marker); got != 123 {
		t.Fatalf("declared source read limit = %d, want 123", got)
	}
}

func TestPlanUpgradePropagatesTransientRevisionDiffReadFailure(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.failGetKey = second.ID + "/" + DiffFilename
	store.mu.Unlock()
	plan, err := client.PlanUpgradeDocument(
		context.Background(), second.URL, UpgradeDocumentOptions{},
	)
	if err == nil || plan != nil || !strings.Contains(err.Error(), "get object") {
		t.Fatalf("transient diff read plan = %+v, %v", plan, err)
	}
}

func TestRevisionPreviousUsesContainingPageKeyWithDeletedIntermediate(t *testing.T) {
	cfg := &Config{PublicBaseURL: "https://plans.example.com"}
	when := time.Now().UTC().Truncate(time.Second)
	dir1, dir3 := strings.Repeat("a", 26), strings.Repeat("c", 26)
	url1 := "https://plans.example.com/" + dir1 + "/plan.html"
	metadata := VersionsMetadata{
		Schema: "airplan-versions", Version: 1, ChainID: strings.Repeat("b", 26),
		CurrentRevision: 3, LatestRevision: 3, LastAssignedRevision: 3,
		Revisions: []VersionsRevision{
			{Number: 1, URL: url1, CreatedAt: when},
			{Number: 2, Deleted: true, DeletedAt: when.Add(time.Second)},
			{Number: 3, URL: "https://plans.example.com/" + dir3 + "/plan.html", CreatedAt: when.Add(2 * time.Second), DiffURL: "https://plans.example.com/" + dir3 + "/" + DiffFilename},
		},
	}
	pageKey := dir3 + "/plan.html"
	body, err := EncodeVersionsMetadata(metadata, cfg, pageKey)
	if err != nil {
		t.Fatal(err)
	}
	marker := &UploadMarker{Revision: &RevisionDescriptor{
		ChainID: metadata.ChainID, Number: 3, PreviousURL: url1,
	}}
	got, err := revisionPrevious(body, cfg, pageKey, marker,
		[]byte("--- revision-1/plan.md\n+++ revision-3/plan.md\n"))
	if err != nil || got != 1 {
		t.Fatalf("previous revision = %d, %v; want 1", got, err)
	}
}

func TestRevisionPreviousUsesDiffHeaderAfterPredecessorTombstone(t *testing.T) {
	cfg := &Config{PublicBaseURL: "https://plans.example.com"}
	when := time.Now().UTC().Truncate(time.Second)
	dir2, dir4 := strings.Repeat("b", 26), strings.Repeat("d", 26)
	url2 := "https://plans.example.com/" + dir2 + "/plan.html"
	metadata := VersionsMetadata{
		Schema: "airplan-versions", Version: 1, ChainID: strings.Repeat("c", 26),
		CurrentRevision: 4, LatestRevision: 4, LastAssignedRevision: 4,
		Revisions: []VersionsRevision{
			{Number: 1, Deleted: true, DeletedAt: when},
			{Number: 2, Deleted: true, DeletedAt: when.Add(time.Second)},
			{Number: 3, Deleted: true, DeletedAt: when.Add(2 * time.Second)},
			{Number: 4, URL: "https://plans.example.com/" + dir4 + "/plan.html", CreatedAt: when.Add(3 * time.Second), DiffURL: "https://plans.example.com/" + dir4 + "/" + DiffFilename},
		},
	}
	pageKey := dir4 + "/plan.html"
	body, err := EncodeVersionsMetadata(metadata, cfg, pageKey)
	if err != nil {
		t.Fatal(err)
	}
	marker := &UploadMarker{Revision: &RevisionDescriptor{
		ChainID: metadata.ChainID, Number: 4, PreviousURL: url2,
	}}
	got, err := revisionPrevious(body, cfg, pageKey, marker,
		[]byte("--- revision-2/plan.md\n+++ revision-4/plan.md\n@@ -1 +1 @@\n-old\n+new\n"))
	if err != nil || got != 2 {
		t.Fatalf("previous revision = %d, %v; want 2", got, err)
	}
}

func TestUpgradeRetryRepairsEqualLengthPageAfterMarkerFirstInterruption(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("i", 26)
	source := []byte("# Plan\n")
	bundle, err := RenderDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: bytes.NewReader(source), Path: "plan.md", Format: "md",
			Title: "Plan",
		},
		Slug: "plan", Title: "Plan", MaxPageSize: -1,
		MaxTotalPageSize: -1,
	}, DocumentRenderOptions{
		RenderInputOptions: RenderInputOptions{
			IncludeSource: true, MermaidURL: DefaultMermaidURL,
		},
		MaxGeneratedPageSize: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPage := bundle.Pages[0].HTML
	oldPage := []byte(strings.Repeat("x", len(wantPage)))
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Title: "Plan", Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(oldPage)), ContentType: pageContentType},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, marker)
	store.set(dir+"/plan.html", oldPage)
	store.set(dir+"/plan.md", source)
	client := store.client(t, "")
	plan, err := client.PlanUpgradeDocument(context.Background(), dir,
		UpgradeDocumentOptions{})
	if err != nil || plan.State != UpgradeStateUpgradeable {
		t.Fatalf("initial plan = %+v, error = %v", plan, err)
	}
	store.failPutAttempt = 2
	if _, err := client.UpgradeDocument(context.Background(), *plan); err == nil {
		t.Fatal("page write interruption unexpectedly succeeded")
	}
	strandedMarkerBody, _ := store.get(dir + "/" + MarkerFilename)
	strandedMarker, err := DecodeUploadMarker(strandedMarkerBody, dir)
	if err != nil {
		t.Fatal(err)
	}
	if strandedMarker.PageSHA256 != contentSHA256(wantPage) {
		t.Fatalf("stranded marker page digest = %q", strandedMarker.PageSHA256)
	}
	if got, _ := store.get(dir + "/plan.html"); !bytes.Equal(got, oldPage) {
		t.Fatal("failed page write changed the old page")
	}
	store.failPutAttempt = 0
	retry, err := client.PlanUpgradeDocument(context.Background(), dir,
		UpgradeDocumentOptions{})
	if err != nil || retry.State != UpgradeStateUpgradeable ||
		retry.Reason != "rendered page requires repair" {
		t.Fatalf("retry plan = %+v, error = %v", retry, err)
	}
	result, err := client.UpgradeDocument(context.Background(), *retry)
	if err != nil || !result.Upgraded {
		t.Fatalf("retry result = %+v, error = %v", result, err)
	}
	if got, _ := store.get(dir + "/plan.html"); !bytes.Equal(got, wantPage) {
		t.Fatal("retry did not repair the rendered page")
	}
	if _, exists := store.get(dir + "/.airplan-versions.json"); exists {
		t.Fatal("upgrade retry created versions metadata")
	}
}

func TestUpgradePlanningDoesNotRender(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("w", 26)
	page := []byte("old")
	source := []byte("# Plan\n")
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Title: "Plan", Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)), ContentType: pageContentType, SHA256: contentSHA256(page)},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, marker)
	store.set(dir+"/plan.html", page)
	store.set(dir+"/plan.md", source)
	templatePath := filepath.Join(t.TempDir(), "broken-at-execution.tmpl")
	if err := os.WriteFile(templatePath, []byte(`{{call .Title}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := New(context.Background(), &Config{
		Endpoint: store.server.URL, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		DisableManifest: true, ProducerVersion: "0.8.0",
		MermaidURL: DefaultMermaidURL, Template: templatePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := client.PlanUpgradeDocument(context.Background(), dir, UpgradeDocumentOptions{})
	if err != nil || plan.State != UpgradeStateUpgradeable || store.puts != 0 {
		t.Fatalf("plan = %+v, puts = %d, err = %v", plan, store.puts, err)
	}
	if _, err := client.UpgradeDocument(context.Background(), *plan); err == nil {
		t.Fatal("execution unexpectedly rendered the invalid template")
	}
	if store.puts != 0 {
		t.Fatalf("render failure performed %d writes", store.puts)
	}
}

func TestUpgradeExecutionRejectsTemplateParseErrorBeforeWrites(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("p", 26)
	seedV3UpgradeDocument(t, store, dir)
	templatePath := filepath.Join(t.TempDir(), "invalid.tmpl")
	if err := os.WriteFile(templatePath, []byte(`{{`), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := New(context.Background(), &Config{
		Endpoint: store.server.URL, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		DisableManifest: true, ProducerVersion: "0.8.0", Template: templatePath,
	})
	if err != nil {
		t.Fatalf("New returned deferred template error: %v", err)
	}
	plan, err := client.PlanUpgradeDocument(context.Background(), dir,
		UpgradeDocumentOptions{})
	if err != nil || plan.State != UpgradeStateUpgradeable || store.puts != 0 {
		t.Fatalf("plan = %+v, puts = %d, error = %v", plan, store.puts, err)
	}
	if _, err := client.UpgradeDocument(context.Background(), *plan); err == nil {
		t.Fatal("execution accepted an invalid configured template")
	}
	if store.puts != 0 {
		t.Fatalf("template parse failure performed %d writes", store.puts)
	}
}

func TestPlanUpgradeProducerVersionOrdering(t *testing.T) {
	for _, test := range []struct {
		name     string
		current  string
		want     UpgradeState
		wantText string
	}{
		{"older", "0.7.9", UpgradeStateUpgradeable, "producer release is older"},
		{"older prerelease", "0.8.0-rc.1", UpgradeStateUpgradeable, "producer release is older"},
		{"equal leading v", "v0.8.0", UpgradeStateCurrent, "already current"},
		{"newer", "99.0.0", UpgradeStateIneligible, "newer airplan release"},
		{"development", "dev", UpgradeStateCurrent, "already current"},
		{"unknown", "nightly-2026-08-15", UpgradeStateCurrent, "already current"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newUpgradeStore(t)
			dir := strings.Repeat(string(rune('a'+len(test.name)%20)), 26)
			seedCurrentUpgradeDocument(t, store, dir, test.current,
				documentRenderRecipe(&Config{MermaidURL: DefaultMermaidURL}, ""))
			plan, err := store.client(t, "").PlanUpgradeDocument(
				context.Background(), dir, UpgradeDocumentOptions{},
			)
			if err != nil || plan.State != test.want ||
				!strings.Contains(plan.Reason, test.wantText) {
				t.Fatalf("plan = %+v, error = %v", plan, err)
			}
		})
	}
}

func TestUpgradeForceReplacesStoredCustomTemplateWithBuiltin(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("r", 26)
	storedRecipe := documentRenderRecipe(&Config{MermaidURL: DefaultMermaidURL},
		strings.Repeat("a", 64))
	seedV4UpgradeDocument(t, store, dir, "0.8.0", storedRecipe)
	client := store.client(t, "")
	refused, err := client.PlanUpgradeDocument(context.Background(), dir,
		UpgradeDocumentOptions{})
	if err != nil || refused.State != UpgradeStateIneligible {
		t.Fatalf("non-force plan = %+v, error = %v", refused, err)
	}
	forced, err := client.PlanUpgradeDocument(context.Background(), dir,
		UpgradeDocumentOptions{Force: true})
	if err != nil || forced.State != UpgradeStateUpgradeable {
		t.Fatalf("force plan = %+v, error = %v", forced, err)
	}
	if _, err := client.UpgradeDocument(context.Background(), *forced); err != nil {
		t.Fatal(err)
	}
	body, _ := store.get(dir + "/" + MarkerFilename)
	marker, err := DecodeUploadMarker(body, dir)
	if err != nil {
		t.Fatal(err)
	}
	if marker.Render == nil || marker.Render.Template.Kind != "builtin" ||
		marker.Render.Template.SHA256 != "" {
		t.Fatalf("replacement recipe = %+v", marker.Render)
	}
}

func TestPlanUpgradeClassifiesConflictingMarkersInvalid(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("d", 26)
	seedV3UpgradeDocument(t, store, dir)
	collection, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindCollection,
		Objects: []MarkerObject{
			{Name: "index.html", Role: MarkerRolePage, Bytes: 1, ContentType: pageContentType},
			{Name: "file.txt", Role: MarkerRoleFile, Bytes: 1, ContentType: "text/plain"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+CollectionMarkerFilename, collection)
	plan, err := store.client(t, "").PlanUpgradeDocument(
		context.Background(), dir, UpgradeDocumentOptions{},
	)
	if err != nil || plan.State != UpgradeStateInvalid {
		t.Fatalf("plan = %+v, error = %v", plan, err)
	}
}

func TestPlanUpgradeRejectsOversizedFetchedPage(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("o", 26)
	page := bytes.Repeat([]byte("x"), int(maxUpgradePageSize)+1)
	source := []byte("# Plan\n")
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)), ContentType: pageContentType},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, marker)
	store.set(dir+"/plan.html", page)
	store.set(dir+"/plan.md", source)

	_, err = store.client(t, "").PlanUpgradeDocument(
		context.Background(), dir, UpgradeDocumentOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds the maximum upgrade size") {
		t.Fatalf("error = %v, want oversized-page rejection", err)
	}
	if store.puts != 0 {
		t.Fatalf("oversized planning performed %d writes", store.puts)
	}
}

func TestPlanUpgradeAcceptsV6EntryAboveLegacyPageLimitWithoutSpooling(t *testing.T) {
	spoolRoot := t.TempDir()
	t.Setenv("TMPDIR", spoolRoot)
	t.Setenv("TMP", spoolRoot)
	t.Setenv("TEMP", spoolRoot)
	store := newUpgradeStore(t)
	dir := strings.Repeat("l", 26)
	page := bytes.Repeat([]byte("x"), int(maxUpgradePageSize)+1)
	source := []byte("# Plan\n")
	markerBody, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Producer: Producer{Name: "airplan", Version: "0.8.0"},
		Render:     documentRenderRecipe(&Config{MermaidURL: DefaultMermaidURL}, ""),
		Entrypoint: "plan.html",
		Pages: []MarkerPage{{
			Path: "plan.md", Page: "plan.html", Source: "plan.md",
			Format: "md", Lang: "",
		}},
		Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)), ContentType: pageContentType, SHA256: contentSHA256(page)},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType, SHA256: contentSHA256(source)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, markerBody)
	store.mu.Lock()
	store.objects[dir+"/plan.html"] = page
	store.etags[dir+"/plan.html"]++
	store.mu.Unlock()
	store.set(dir+"/plan.md", source)

	plan, err := store.client(t, "").PlanUpgradeDocument(
		context.Background(), dir, UpgradeDocumentOptions{},
	)
	if err != nil || plan.State != UpgradeStateCurrent {
		t.Fatalf("plan = %+v, error = %v", plan, err)
	}
	if plan.pageSizes["plan.html"] != int64(len(page)) ||
		plan.pageDigests["plan.html"] != contentSHA256(page) ||
		plan.newBundle != nil || plan.newMarker != nil {
		t.Fatalf("plan retained unexpected execution state: %+v", plan)
	}
	assertNoUpgradeSpools(t, spoolRoot)
}

func TestUpgradeExecutionUsesPrivateSpoolsAndCleansUp(t *testing.T) {
	spoolRoot := t.TempDir()
	t.Setenv("TMPDIR", spoolRoot)
	t.Setenv("TMP", spoolRoot)
	t.Setenv("TEMP", spoolRoot)
	store := newUpgradeStore(t)
	dir := strings.Repeat("q", 26)
	seedV3UpgradeDocument(t, store, dir)
	client := store.client(t, "")
	plan, err := client.PlanUpgradeDocument(
		context.Background(), dir, UpgradeDocumentOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	payloads, err := client.spoolUpgradePayloads(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(payloads.spool.dir)
		if statErr != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("spool directory = %v, error = %v", info, statErr)
		}
		entries, readErr := os.ReadDir(payloads.spool.dir)
		if readErr != nil || len(entries) != 2 {
			t.Fatalf("spool entries = %v, error = %v", entries, readErr)
		}
		for _, entry := range entries {
			info, statErr := entry.Info()
			if statErr != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("spool file %q = %v, error = %v", entry.Name(), info, statErr)
			}
		}
	}
	payloads.cleanup()
	assertNoUpgradeSpools(t, spoolRoot)

	store.failPutAttempt = store.putAttempts + 1
	if _, err := client.UpgradeDocument(context.Background(), *plan); err == nil {
		t.Fatal("upgrade storage failure unexpectedly succeeded")
	}
	assertNoUpgradeSpools(t, spoolRoot)
	store.failPutAttempt = 0

	result, err := client.UpgradeDocument(context.Background(), *plan)
	if err != nil || !result.Upgraded {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	assertNoUpgradeSpools(t, spoolRoot)

	currentDir := strings.Repeat("j", 26)
	seedCurrentUpgradeDocument(t, store, currentDir, "0.8.0",
		documentRenderRecipe(&Config{MermaidURL: DefaultMermaidURL}, ""))
	current, err := client.PlanUpgradeDocument(
		context.Background(), currentDir, UpgradeDocumentOptions{},
	)
	if err != nil || current.State != UpgradeStateCurrent {
		t.Fatalf("current plan = %+v, error = %v", current, err)
	}
	if result, err := client.UpgradeDocument(context.Background(), *current); err != nil || result.Upgraded {
		t.Fatalf("current result = %+v, error = %v", result, err)
	}
	assertNoUpgradeSpools(t, spoolRoot)
}

func assertNoUpgradeSpools(t *testing.T, root string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "airplan-document-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("upgrade spools remain: %q", matches)
	}
}

func TestBulkUpgradeRejectsInvalidConcurrency(t *testing.T) {
	client := newUpgradeStore(t).client(t, filepath.Join(t.TempDir(), "manifest.jsonl"))
	if _, err := client.PlanBulkUpgrade(context.Background(),
		BulkUpgradeOptions{Concurrency: -1}); err == nil {
		t.Fatal("planning accepted negative concurrency")
	}
	if _, err := client.ExecuteBulkUpgrade(context.Background(),
		BulkUpgradeRequest{Concurrency: 33}); err == nil {
		t.Fatal("execution accepted concurrency above 32")
	}
}

func TestUpgradeDocumentRejectsStalePlan(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("v", 26)
	page := []byte("old")
	source := []byte("# Plan\n")
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)), ContentType: pageContentType},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, marker)
	store.set(dir+"/plan.html", page)
	store.set(dir+"/plan.md", source)
	client := store.client(t, "")
	plan, err := client.PlanUpgradeDocument(context.Background(), dir, UpgradeDocumentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, marker)
	_, err = client.UpgradeDocument(context.Background(), *plan)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v", err)
	}
	if got, _ := store.get(dir + "/plan.html"); string(got) != string(page) {
		t.Fatal("page changed after marker conflict")
	}
}

func TestUpgradeDocumentRechecksSourceIdentityImmediatelyBeforeMutation(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("i", 26)
	seedV3UpgradeDocument(t, store, dir)
	client := store.client(t, "")
	plan, err := client.PlanUpgradeDocument(
		context.Background(), dir, UpgradeDocumentOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceKey := dir + "/plan.md"
	store.mu.Lock()
	store.replaceBeforeGetKey = sourceKey
	store.replaceBeforeGetAttempt = store.getKeyAttempts[sourceKey] + 3
	store.replaceBeforeGetBody = []byte("# Changed during render\n")
	store.mu.Unlock()
	_, err = client.UpgradeDocument(context.Background(), *plan)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want source identity conflict", err)
	}
	if store.puts != 0 {
		t.Fatalf("source identity conflict performed %d writes", store.puts)
	}
}

func TestUpgradeDocumentReplansSerializedState(t *testing.T) {
	t.Run("fresh current no-ops after reads", func(t *testing.T) {
		store := newUpgradeStore(t)
		dir := strings.Repeat("c", 26)
		seedCurrentUpgradeDocument(t, store, dir, "0.8.0",
			documentRenderRecipe(&Config{MermaidURL: DefaultMermaidURL}, ""))
		client := store.client(t, "")
		plan, err := client.PlanUpgradeDocument(context.Background(), dir,
			UpgradeDocumentOptions{})
		if err != nil || plan.State != UpgradeStateCurrent {
			t.Fatalf("plan = %+v, error = %v", plan, err)
		}
		serialized := roundTripUpgradePlan(t, *plan)
		getsBefore := store.getAttempts
		result, err := client.UpgradeDocument(context.Background(), serialized)
		if err != nil || result.Upgraded || result.State != UpgradeStateCurrent {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
		if store.getAttempts == getsBefore || store.puts != 0 {
			t.Fatalf("gets = %d -> %d, puts = %d", getsBefore, store.getAttempts, store.puts)
		}
	})

	t.Run("serialized upgradeable applies exact fresh plan", func(t *testing.T) {
		store := newUpgradeStore(t)
		dir := strings.Repeat("s", 26)
		seedV3UpgradeDocument(t, store, dir)
		client := store.client(t, "")
		plan, err := client.PlanUpgradeDocument(context.Background(), dir,
			UpgradeDocumentOptions{})
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.UpgradeDocument(context.Background(),
			roundTripUpgradePlan(t, *plan))
		if err != nil || !result.Upgraded || store.puts != 2 {
			t.Fatalf("result = %+v, puts = %d, error = %v", result, store.puts, err)
		}
	})

	t.Run("fabricated current missing target refuses after reads", func(t *testing.T) {
		store := newUpgradeStore(t)
		_, err := store.client(t, "").UpgradeDocument(context.Background(),
			UpgradeDocumentPlan{
				Target: strings.Repeat("m", 26), State: UpgradeStateCurrent,
			})
		if err == nil || !strings.Contains(err.Error(), "missing") ||
			store.getAttempts == 0 || store.puts != 0 {
			t.Fatalf("gets = %d, puts = %d, error = %v", store.getAttempts, store.puts, err)
		}
	})

	t.Run("fresh ineligible refuses fabricated current", func(t *testing.T) {
		store := newUpgradeStore(t)
		dir := strings.Repeat("n", 26)
		page := []byte("<html></html>")
		marker, err := EncodeUploadMarker(UploadMarker{
			Schema: MarkerSchema, Version: 3, Directory: dir,
			CreatedAt: time.Now().UTC(), Kind: UploadKindDocument,
			Slug: "plan", Format: "md", Objects: []MarkerObject{{
				Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)),
				ContentType: pageContentType,
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		store.set(dir+"/"+MarkerFilename, marker)
		store.set(dir+"/plan.html", page)
		_, err = store.client(t, "").UpgradeDocument(context.Background(),
			UpgradeDocumentPlan{Target: dir, State: UpgradeStateCurrent})
		if err == nil || !strings.Contains(err.Error(), "ineligible") || store.puts != 0 {
			t.Fatalf("puts = %d, error = %v", store.puts, err)
		}
	})

	t.Run("fresh invalid refuses fabricated current", func(t *testing.T) {
		store := newUpgradeStore(t)
		dir := strings.Repeat("d", 26)
		seedV3UpgradeDocument(t, store, dir)
		collection, err := EncodeUploadMarker(UploadMarker{
			Schema: MarkerSchema, Version: 3, Directory: dir,
			CreatedAt: time.Now().UTC().Truncate(time.Second),
			Kind:      UploadKindCollection, Objects: []MarkerObject{
				{Name: "index.html", Role: MarkerRolePage, Bytes: 1, ContentType: pageContentType},
				{Name: "file.txt", Role: MarkerRoleFile, Bytes: 1, ContentType: "text/plain"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		store.set(dir+"/"+CollectionMarkerFilename, collection)
		_, err = store.client(t, "").UpgradeDocument(context.Background(),
			UpgradeDocumentPlan{Target: dir, State: UpgradeStateCurrent})
		if err == nil || !strings.Contains(err.Error(), "invalid") || store.puts != 0 {
			t.Fatalf("puts = %d, error = %v", store.puts, err)
		}
	})
}

func TestBulkUpgradeKeepsOrderAndContinuesAfterConflict(t *testing.T) {
	store := newUpgradeStore(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := store.client(t, manifest)
	dirs := []string{strings.Repeat("x", 26), strings.Repeat("y", 26)}
	markers := make([][]byte, len(dirs))
	for index, dir := range dirs {
		page := []byte("old")
		source := []byte("# Plan\n")
		marker, err := EncodeUploadMarker(UploadMarker{
			Schema: MarkerSchema, Version: 3, Directory: dir,
			CreatedAt: time.Now().UTC().Add(time.Duration(index) * time.Second).Truncate(time.Second),
			Kind:      UploadKindDocument, Slug: "plan", Format: "md",
			Objects: []MarkerObject{
				{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)), ContentType: pageContentType},
				{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		markers[index] = marker
		store.set(dir+"/"+MarkerFilename, marker)
		store.set(dir+"/plan.html", page)
		store.set(dir+"/plan.md", source)
		if err := appendManifestRecord(context.Background(), manifest, ManifestRecord{
			Type: "upload", Time: time.Now().UTC().Add(time.Duration(index) * time.Second),
			CreatedAt: time.Now().UTC(), Bucket: "plans", MarkerKey: dir + "/" + MarkerFilename,
			Key: dir + "/plan.html", SourceKey: dir + "/plan.md",
			URL:  "https://plans.example.com/" + dir + "/plan.html",
			Kind: string(UploadKindDocument), Format: "md", Slug: "plan",
			Bytes: int64(len(page)), MarkerVersion: 3,
		}); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := client.PlanBulkUpgrade(context.Background(), BulkUpgradeOptions{Concurrency: 2})
	if err != nil || len(plan.Items) != 2 {
		t.Fatalf("plan = %+v, err = %v", plan, err)
	}
	store.set(dirs[1]+"/"+MarkerFilename, markers[1])
	result, err := client.ExecuteBulkUpgrade(context.Background(), BulkUpgradeRequest{
		Items: plan.Items, Concurrency: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Upgraded != 1 || result.Failed != 1 || len(result.Items) != 2 {
		t.Fatalf("result = %+v", result)
	}
	for index, item := range result.Items {
		if item.Plan.Target != plan.Items[index].Target {
			t.Fatalf("item %d target = %q, want %q", index, item.Plan.Target, plan.Items[index].Target)
		}
	}
}

func seedV3UpgradeDocument(t *testing.T, store *upgradeStore, dir string) {
	t.Helper()
	page := []byte("old")
	source := []byte("# Plan\n")
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)), ContentType: pageContentType},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, marker)
	store.set(dir+"/plan.html", page)
	store.set(dir+"/plan.md", source)
}

func seedV4UpgradeDocument(
	t *testing.T, store *upgradeStore, dir, producer string, recipe *RenderRecipe,
) {
	t.Helper()
	page := []byte("old")
	source := []byte("# Plan\n")
	v4Recipe := *recipe
	v4Recipe.Generation = 2
	v4Recipe.Themes = nil
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 4, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Producer: Producer{Name: "airplan", Version: producer},
		Render: &v4Recipe, Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)), ContentType: pageContentType, SHA256: contentSHA256(page)},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, marker)
	store.set(dir+"/plan.html", page)
	store.set(dir+"/plan.md", source)
}

func seedCurrentUpgradeDocument(
	t *testing.T, store *upgradeStore, dir, producer string, recipe *RenderRecipe,
) {
	t.Helper()
	page := []byte("old")
	source := []byte("# Plan\n")
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Producer: Producer{Name: "airplan", Version: producer},
		Render: recipe, Entrypoint: "plan.html",
		Pages: []MarkerPage{{Path: "plan.md", Page: "plan.html", Source: "plan.md", Format: "md", Lang: ""}},
		Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)), ContentType: pageContentType, SHA256: contentSHA256(page)},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType, SHA256: contentSHA256(source)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, marker)
	store.set(dir+"/plan.html", page)
	store.set(dir+"/plan.md", source)
}

func roundTripUpgradePlan(t *testing.T, plan UpgradeDocumentPlan) UpgradeDocumentPlan {
	t.Helper()
	body, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var decoded UpgradeDocumentPlan
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

type upgradeStore struct {
	t                       *testing.T
	server                  *httptest.Server
	mu                      sync.Mutex
	objects                 map[string][]byte
	etags                   map[string]int
	puts                    int
	putKeys                 []string
	putAttempts             int
	failPutAttempt          int
	failPutKey              string
	failPutSuffix           string
	commitPutThenFailKey    string
	cancelOnPutKey          string
	cancelOnPut             func()
	failDeleteKeys          bool
	failMarkerDelete        bool
	failGetKey              string
	failGetKeyOnce          string
	pauseGetKey             string
	pauseGetReached         chan struct{}
	pauseGetRelease         chan struct{}
	pauseGetUsed            bool
	pauseGetAfterCommit     bool
	failHeadKey             string
	getAttempts             int
	getKeyAttempts          map[string]int
	replaceBeforeGetKey     string
	replaceBeforeGetAttempt int
	replaceBeforeGetBody    []byte
	conditionalBarrier      chan struct{}
	conditionalBarrierCount int
	pauseIfNoneKey          string
	pauseIfNoneReached      chan struct{}
	pauseIfNoneRelease      chan struct{}
	pauseIfNoneUsed         bool
	pauseIfMatchKey         string
	pauseIfMatchReached     chan struct{}
	pauseIfMatchRelease     chan struct{}
	pauseIfMatchUsed        bool
	pauseListPrefix         string
	pauseListAttempt        int
	listAttempts            int
	pauseListReached        chan struct{}
	pauseListRelease        chan struct{}
	pauseListUsed           bool
	ifMatchBarrier          chan struct{}
	ifMatchBarrierCount     int
}

func newUpgradeStore(t *testing.T) *upgradeStore {
	s := &upgradeStore{
		t: t, objects: map[string][]byte{}, etags: map[string]int{},
		getKeyAttempts: map[string]int{},
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.server.Close)
	return s
}

func (s *upgradeStore) client(t *testing.T, manifest string) *Client {
	return s.clientWithThemes(t, manifest, "", "")
}

func (s *upgradeStore) clientWithThemes(
	t *testing.T, manifest, lightTheme, darkTheme string,
) *Client {
	client, err := New(context.Background(), &Config{
		Endpoint: s.server.URL, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		ManifestPath: manifest, DisableManifest: manifest == "",
		ProducerVersion: "0.8.0", MermaidURL: DefaultMermaidURL,
		LightTheme: lightTheme, DarkTheme: darkTheme,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (s *upgradeStore) set(key string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = append([]byte(nil), body...)
	s.etags[key]++
}

func (s *upgradeStore) get(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, ok := s.objects[key]
	return append([]byte(nil), body...), ok
}

func (s *upgradeStore) handle(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/plans/")
	if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" {
		s.mu.Lock()
		s.listAttempts++
		var listRelease chan struct{}
		pauseAttempt := s.pauseListAttempt
		if pauseAttempt == 0 {
			pauseAttempt = 1
		}
		if s.pauseListReached != nil &&
			r.URL.Query().Get("prefix") == s.pauseListPrefix &&
			s.listAttempts == pauseAttempt && !s.pauseListUsed {
			s.pauseListUsed = true
			close(s.pauseListReached)
			listRelease = s.pauseListRelease
		}
		s.mu.Unlock()
		if listRelease != nil {
			<-listRelease
		}
	}
	if r.Method == http.MethodPut && r.Header.Get("If-None-Match") == "*" {
		s.mu.Lock()
		var targetedRelease chan struct{}
		if s.pauseIfNoneReached != nil && key == s.pauseIfNoneKey &&
			!s.pauseIfNoneUsed {
			s.pauseIfNoneUsed = true
			close(s.pauseIfNoneReached)
			targetedRelease = s.pauseIfNoneRelease
		}
		s.mu.Unlock()
		if targetedRelease != nil {
			<-targetedRelease
		}

		s.mu.Lock()
		barrier := s.conditionalBarrier
		if barrier != nil {
			s.conditionalBarrierCount++
			if s.conditionalBarrierCount == 2 {
				close(barrier)
			}
		}
		s.mu.Unlock()
		if barrier != nil {
			select {
			case <-barrier:
			case <-time.After(5 * time.Second):
				s.t.Error("conditional-write barrier never released")
			}
		}
	}
	if r.Method == http.MethodPut && r.Header.Get("If-Match") != "" {
		s.mu.Lock()
		var targetedRelease chan struct{}
		if s.pauseIfMatchReached != nil && key == s.pauseIfMatchKey &&
			!s.pauseIfMatchUsed {
			s.pauseIfMatchUsed = true
			close(s.pauseIfMatchReached)
			targetedRelease = s.pauseIfMatchRelease
		}
		barrier := s.ifMatchBarrier
		if barrier != nil {
			s.ifMatchBarrierCount++
			if s.ifMatchBarrierCount == 2 {
				close(barrier)
			}
		}
		s.mu.Unlock()
		if targetedRelease != nil {
			<-targetedRelease
		}
		if barrier != nil {
			select {
			case <-barrier:
			case <-time.After(5 * time.Second):
				s.t.Error("conditional-update barrier never released")
			}
		}
	}
	if r.Method == http.MethodGet && r.URL.Query().Get("list-type") != "2" {
		s.mu.Lock()
		var targetedRelease chan struct{}
		if s.pauseGetReached != nil && key == s.pauseGetKey && !s.pauseGetUsed &&
			(!s.pauseGetAfterCommit || s.commitPutThenFailKey == "") {
			s.pauseGetUsed = true
			close(s.pauseGetReached)
			targetedRelease = s.pauseGetRelease
		}
		s.mu.Unlock()
		if targetedRelease != nil {
			<-targetedRelease
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("list-type") == "2" {
			prefix := r.URL.Query().Get("prefix")
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<?xml version="1.0"?><ListBucketResult><IsTruncated>false</IsTruncated>`)
			for objectKey, objectBody := range s.objects {
				if strings.HasPrefix(objectKey, prefix) {
					_, _ = fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size>"+
						"<LastModified>2026-08-15T12:00:00Z</LastModified></Contents>",
						objectKey, len(objectBody))
				}
			}
			_, _ = io.WriteString(w, `</ListBucketResult>`)
			return
		}
		s.getAttempts++
		s.getKeyAttempts[key]++
		if key == s.replaceBeforeGetKey &&
			s.getKeyAttempts[key] == s.replaceBeforeGetAttempt {
			s.objects[key] = append([]byte(nil), s.replaceBeforeGetBody...)
			s.etags[key]++
		}
		if s.failGetKey == key {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if s.failGetKeyOnce == key {
			s.failGetKeyOnce = ""
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, ok := s.objects[key]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_ = xml.NewEncoder(w).Encode(struct {
				Code string `xml:"Code"`
			}{Code: "NoSuchKey"})
			return
		}
		w.Header().Set("ETag", `"etag-`+string(rune('0'+s.etags[key]))+`"`)
		_, _ = w.Write(body)
	case http.MethodHead:
		if s.failHeadKey == key {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body, ok := s.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("ETag", `"etag-`+string(rune('0'+s.etags[key]))+`"`)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		ifMatch := r.Header.Get("If-Match")
		want := `"etag-` + string(rune('0'+s.etags[key])) + `"`
		if ifMatch != "" && ifMatch != want {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		if r.Header.Get("If-None-Match") == "*" {
			if _, exists := s.objects[key]; exists {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.putAttempts++
		if s.cancelOnPutKey == key && s.cancelOnPut != nil {
			s.cancelOnPut()
			s.cancelOnPutKey = ""
			s.cancelOnPut = nil
		}
		if s.failPutKey == key ||
			(s.failPutSuffix != "" && strings.HasSuffix(key, s.failPutSuffix)) {
			s.failPutKey = ""
			s.failPutSuffix = ""
			// A non-precondition, non-retryable response models an ordinary
			// storage failure without turning the fixture into an append conflict.
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if s.failPutAttempt == s.putAttempts {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		s.objects[key] = body
		s.etags[key]++
		s.puts++
		s.putKeys = append(s.putKeys, key)
		if s.commitPutThenFailKey == key {
			s.commitPutThenFailKey = ""
			// Model an ambiguous response: storage committed the write, but the
			// caller only receives a non-precondition failure.
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("ETag", `"updated"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodPost:
		if s.failDeleteKeys {
			s.failDeleteKeys = false
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var request struct {
			Objects []struct {
				Key string `xml:"Key"`
			} `xml:"Object"`
		}
		if err := xml.NewDecoder(r.Body).Decode(&request); err != nil {
			s.t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, object := range request.Objects {
			delete(s.objects, object.Key)
			delete(s.etags, object.Key)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0"?><DeleteResult></DeleteResult>`)
	case http.MethodDelete:
		if s.failMarkerDelete {
			s.failMarkerDelete = false
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		delete(s.objects, key)
		delete(s.etags, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
