package airplan

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestUpgradePlanningDoesNotRender(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("w", 26)
	page := []byte("old")
	source := []byte("# Plan\n")
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Title: "Plan", Objects: []MarkerObject{
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
			seedV4UpgradeDocument(t, store, dir, test.current,
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
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second), Kind: UploadKindDocument,
		Slug: "plan", Format: "md", Producer: Producer{Name: "airplan", Version: producer},
		Render: recipe, Objects: []MarkerObject{
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

type upgradeStore struct {
	t       *testing.T
	server  *httptest.Server
	mu      sync.Mutex
	objects map[string][]byte
	etags   map[string]int
	puts    int
	putKeys []string
}

func newUpgradeStore(t *testing.T) *upgradeStore {
	s := &upgradeStore{t: t, objects: map[string][]byte{}, etags: map[string]int{}}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.server.Close)
	return s
}

func (s *upgradeStore) client(t *testing.T, manifest string) *Client {
	client, err := New(context.Background(), &Config{
		Endpoint: s.server.URL, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		ManifestPath: manifest, DisableManifest: manifest == "",
		ProducerVersion: "0.8.0", MermaidURL: DefaultMermaidURL,
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
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
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
	case http.MethodPut:
		ifMatch := r.Header.Get("If-Match")
		want := `"etag-` + string(rune('0'+s.etags[key])) + `"`
		if ifMatch != "" && ifMatch != want {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.t.Fatal(err)
		}
		s.objects[key] = body
		s.etags[key]++
		s.puts++
		s.putKeys = append(s.putKeys, key)
		w.Header().Set("ETag", `"updated"`)
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
