package airplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jimeh/airplan/internal/httpapi"
)

func TestWireVersionsMetadataPreservesSchemaAndVersion(t *testing.T) {
	wire := wireVersionsMetadata(VersionsMetadata{Schema: "future", Version: 7})
	if string(wire.Schema) != "future" || int(wire.Version) != 7 {
		t.Fatalf("wire metadata = %+v", wire)
	}
}

func TestSupportedMarkerVersionsAdvertiseV1ThroughV6(t *testing.T) {
	if got, want := supportedMarkerVersions(), []int{1, 2, 3, 4, 5, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("supported marker versions = %v, want %v", got, want)
	}
}

func TestHTTPAPIUpdatesDocumentThroughOperationFacade(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	operations := &HTTPOperations{Client: client, ServerVersion: "test"}
	result, err := operations.UpdateDocument(context.Background(), httpapi.UpdateDocumentUpload{
		Metadata: httpapi.UpdateDocumentMetadata{Target: first.URL, Name: "plan.md"},
		Document: strings.NewReader("two\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != 2 || result.LatestRevision != 2 || result.Unchanged || result.DiffURL == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestHTTPAPIUploadsDocumentBundleThroughOperationFacade(t *testing.T) {
	store := newUpgradeStore(t)
	operations := &HTTPOperations{Client: store.client(t, ""), ServerVersion: "test"}
	result, err := operations.UploadDocument(context.Background(), httpapi.DocumentUpload{
		Metadata: httpapi.DocumentMetadata{
			Name: "README.md", Format: "md",
			Pages:  []httpapi.DocumentPageDescriptor{{Path: "src/main.go", Format: "txt"}},
			Assets: []httpapi.DocumentAssetDescriptor{{Path: "images/flow.svg", Size: 5, ContentType: "image/svg+xml"}},
		},
		Document: strings.NewReader("# Entry\n"),
		Pages: []httpapi.DocumentPage{{
			DocumentPageDescriptor: httpapi.DocumentPageDescriptor{Path: "src/main.go", Format: "txt"},
			Reader:                 strings.NewReader("package main\n"), Size: 13,
		}},
		Assets: []httpapi.DocumentAsset{{
			DocumentAssetDescriptor: httpapi.DocumentAssetDescriptor{Path: "images/flow.svg", Size: 5, ContentType: "image/svg+xml"},
			Reader:                  bytes.NewReader([]byte("asset")),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 || result.Pages[1].Path != "src/main.go" ||
		len(result.Assets) != 1 || result.Assets[0].Path != "images/flow.svg" {
		t.Fatalf("bundle result = %+v", result)
	}
}

func TestHTTPAPIRejectsInvalidBundleAsUnprocessableUpload(t *testing.T) {
	store := newUpgradeStore(t)
	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(
		&HTTPOperations{Client: store.client(t, ""), ServerVersion: "test"},
		httpapi.Options{Token: token},
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	apiClient, err := httpapi.NewClient(server.URL, token, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = apiClient.UploadDocumentBundle(context.Background(), httpapi.DocumentUpload{
		Metadata: httpapi.DocumentMetadata{
			Name:   "notes.md",
			Assets: []httpapi.DocumentAssetDescriptor{{Path: "notes.html", Size: 5}},
		},
		Document: strings.NewReader("# Notes\n"),
		Assets: []httpapi.DocumentAsset{{
			DocumentAssetDescriptor: httpapi.DocumentAssetDescriptor{
				Path: "notes.html", Size: 5,
			},
			Reader: strings.NewReader("asset"),
		}},
	})
	var problem *httpapi.ProblemError
	if !errors.As(err, &problem) ||
		problem.Problem.Status != http.StatusUnprocessableEntity ||
		problem.Problem.Code != "invalid_upload" {
		t.Fatalf("error = %v, want 422 invalid_upload", err)
	}
	if store.puts != 0 {
		t.Fatalf("storage PUTs = %d, want 0", store.puts)
	}
}

func TestHTTPAPIDocumentBundleMatchesDirectPipeline(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	asset := []byte("asset")
	direct, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("# Entry\n\n[Page](docs/page.md)\n"),
			Path:   "README.md",
		},
		Pages: []PageInput{{
			Reader: strings.NewReader("# Page\n"), Path: "docs/page.md",
		}},
		Assets: []AssetInput{{
			Reader: bytes.NewReader(asset), Path: "images/data.bin",
			Size: int64(len(asset)), ContentType: "application/octet-stream",
		}},
		RepositoryURL: "none",
	})
	if err != nil {
		t.Fatal(err)
	}

	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(
		&HTTPOperations{Client: client, ServerVersion: "test"},
		httpapi.Options{Token: token},
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	apiClient, err := httpapi.NewClient(server.URL, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := apiClient.UploadDocumentBundle(context.Background(), httpapi.DocumentUpload{
		Metadata: httpapi.DocumentMetadata{
			Name:  "README.md",
			Pages: []httpapi.DocumentPageDescriptor{{Path: "docs/page.md"}},
			Assets: []httpapi.DocumentAssetDescriptor{{
				Path: "images/data.bin", Size: int64(len(asset)),
				ContentType: "application/octet-stream",
			}},
		},
		Document: strings.NewReader("# Entry\n\n[Page](docs/page.md)\n"),
		Pages: []httpapi.DocumentPage{{
			DocumentPageDescriptor: httpapi.DocumentPageDescriptor{Path: "docs/page.md"},
			Reader:                 strings.NewReader("# Page\n"),
			Size:                   int64(len("# Page\n")),
		}},
		Assets: []httpapi.DocumentAsset{{
			DocumentAssetDescriptor: httpapi.DocumentAssetDescriptor{
				Path: "images/data.bin", Size: int64(len(asset)),
				ContentType: "application/octet-stream",
			},
			Reader: bytes.NewReader(asset),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(remote.Pages) != len(direct.Pages) ||
		len(remote.Assets) != len(direct.Assets) {
		t.Fatalf("direct = %+v, REST = %+v", direct, remote)
	}
	directMarker := decodeStoredMarker(t, store, direct.MarkerKey, direct.ID)
	remoteMarker := decodeStoredMarker(t, store, remote.MarkerKey, remote.ID)
	directMarker.Directory, remoteMarker.Directory = "", ""
	directMarker.CreatedAt, remoteMarker.CreatedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(directMarker, remoteMarker) {
		t.Fatalf("direct marker = %+v\nREST marker = %+v", directMarker, remoteMarker)
	}
}

func decodeStoredMarker(
	t *testing.T, store *upgradeStore, key, directory string,
) *UploadMarker {
	t.Helper()
	body, ok := store.get(key)
	if !ok {
		t.Fatalf("marker %q is missing", key)
	}
	marker, err := DecodeUploadMarker(body, directory)
	if err != nil {
		t.Fatal(err)
	}
	return marker
}

func TestHTTPAPIUpdateRefusesIneligibleAndMissingTargetsWithTypedStatuses(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	html, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("<h1>HTML</h1>"), Name: "page.html", Format: "html",
	})
	if err != nil {
		t.Fatal(err)
	}
	operations := &HTTPOperations{Client: client, ServerVersion: "test"}
	for _, test := range []struct {
		name   string
		target string
		status int
		code   string
	}{
		{"ineligible", html.URL, http.StatusUnprocessableEntity, "invalid_target"},
		{"missing", strings.Repeat("z", 26), http.StatusNotFound, "upload_not_found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := operations.UpdateDocument(context.Background(),
				httpapi.UpdateDocumentUpload{
					Metadata: httpapi.UpdateDocumentMetadata{Target: test.target},
					Document: strings.NewReader("# Revised\n"),
				})
			var problem *httpapi.ProblemError
			if !errors.As(err, &problem) || problem.Problem.Status != test.status ||
				problem.Problem.Code != test.code || strings.Contains(problem.Problem.Detail, test.target) {
				t.Fatalf("problem = %+v, error = %v", problem, err)
			}
		})
	}
}

func TestHTTPAPIPlansDocumentUpgradeThroughOperationFacade(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("r", 26)
	page, source := []byte("old"), []byte("# Plan\n")
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Kind:      UploadKindDocument, Slug: "plan", Format: "md",
		Objects: []MarkerObject{
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
	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(&HTTPOperations{
		Client: client, ServerVersion: "test",
	}, httpapi.Options{Token: token})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/upgrades/plan",
		bytes.NewBufferString(`{"target":"`+dir+`/plan.html"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var result httpapi.UpgradeDocumentPlan
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if string(result.State) != "upgradeable" || result.CurrentMarkerVersion != 3 || store.puts != 0 {
		t.Fatalf("result = %+v, puts = %d", result, store.puts)
	}
}

func TestHTTPAPIExecuteUpgradeReplansFabricatedCurrentState(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(&HTTPOperations{
		Client: client, ServerVersion: "test",
	}, httpapi.Options{Token: token})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"target":"` + strings.Repeat("m", 26) +
		`","state":"current","target_marker_version":6,` +
		`"target_producer_version":"0.8.0","target_renderer_generation":1}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/upgrades/execute",
		bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound || store.getAttempts == 0 || store.puts != 0 {
		t.Fatalf("status = %d, gets = %d, puts = %d, body = %s",
			recorder.Code, store.getAttempts, store.puts, recorder.Body.String())
	}
	var problem httpapi.Problem
	if err := json.NewDecoder(recorder.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "upload_not_found" {
		t.Fatalf("problem = %+v", problem)
	}
}

func TestWireBulkUpgradeFailedItemOmitsResult(t *testing.T) {
	wire := httpapi.BulkUpgradeItemResult{
		Plan: wireUpgradePlan(UpgradeDocumentPlan{
			Target: "dir/plan.html", State: UpgradeStateUpgradeable,
			TargetMarkerVersion:      MarkerVersion,
			TargetProducerVersion:    "0.8.0",
			TargetRendererGeneration: RendererGeneration,
		}),
		Error: "upgrade failed",
	}
	body, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"result"`) {
		t.Fatalf("failed item emitted a zero result: %s", body)
	}
}

func TestHTTPAPIManifestListScopesSharedManifest(t *testing.T) {
	manifestPath := t.TempDir() + "/manifest.jsonl"
	now := time.Now().UTC().Truncate(time.Second)
	for _, record := range []ManifestRecord{
		manifestUploadRecord(now, "work", "plans", "team/current", "a"),
		manifestUploadRecord(now, "work", "plans", "team/current", "e"),
		manifestUploadRecord(now, "home", "plans", "team/current", "b"),
		manifestUploadRecord(now, "work", "archive", "team/current", "c"),
		manifestUploadRecord(now, "work", "plans", "team/old", "d"),
	} {
		if err := appendManifestRecord(context.Background(), manifestPath, record); err != nil {
			t.Fatal(err)
		}
	}
	records, _, err := ReadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	records[1].MarkerKey = ""
	if err := rewriteManifest(t, manifestPath, records); err != nil {
		t.Fatal(err)
	}
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, Endpoint: "http://127.0.0.1",
		Bucket: "plans", KeyPrefix: "team/current", Profile: "work",
		ManifestPath: manifestPath, Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(&HTTPOperations{
		Client: client, ServerVersion: "test",
	}, httpapi.Options{Token: token})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/uploads", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var result httpapi.ManifestList
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 || result.Records[0].Title != "a" ||
		result.Records[1].Title != "e" {
		t.Fatalf("records = %+v", result.Records)
	}
}

// TestHTTPAPIManifestListOrdersByRecordTime covers the REST manifest endpoint,
// which lists through the service scope (SPEC.md §9).
func TestHTTPAPIManifestListOrdersByRecordTime(t *testing.T) {
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, Endpoint: "http://127.0.0.1",
		Bucket: "plans", Profile: "work",
		ManifestPath: writeUnorderedManifest(t), Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(&HTTPOperations{
		Client: client, ServerVersion: "test",
	}, httpapi.Options{Token: token})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/uploads", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var result httpapi.ManifestList
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 0, len(result.Records))
	for _, record := range result.Records {
		titles = append(titles, record.Title)
	}
	if len(titles) != 3 || titles[0] != "a" || titles[1] != "b" ||
		titles[2] != "c" {
		t.Fatalf("titles = %q, want oldest first", titles)
	}
}

func TestPlanPurgeScopesPrefixWithoutStorageConfig(t *testing.T) {
	manifestPath := t.TempDir() + "/manifest.jsonl"
	now := time.Now().UTC().Truncate(time.Second)
	for _, record := range []ManifestRecord{
		manifestUploadRecord(now, "work", "plans", "team/current", "a"),
		manifestUploadRecord(now, "work", "plans", "team/old", "b"),
	} {
		if err := appendManifestRecord(
			context.Background(), manifestPath, record,
		); err != nil {
			t.Fatal(err)
		}
	}
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, KeyPrefix: "team/current", Profile: "work",
		ManifestPath: manifestPath, Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := client.PlanPurge(context.Background(), PurgePlanOptions{
		Source: UploadSourceManifest, All: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].Record.Title != "a" {
		t.Fatalf("candidates = %+v", plan.Candidates)
	}
}

func TestHTTPOperationsGetUploadStreamsStorageBody(t *testing.T) {
	dir := "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3,
		Directory: dir, CreatedAt: time.Now().UTC(),
		Kind: UploadKindDocument, Slug: "plan", Format: "md",
		Objects: []MarkerObject{{
			Name: "plan.html", Role: MarkerRolePage,
			Bytes: 7, ContentType: "text/html; charset=utf-8",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	started := make(chan struct{})
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		switch r.URL.Path {
		case "/plans/" + dir + "/" + MarkerFilename:
			w.Header().Set("Content-Type", markerContentType)
			_, _ = w.Write(marker)
		case "/plans/" + dir + "/" + CollectionMarkerFilename:
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w,
				`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
		case "/plans/" + dir + "/plan.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(started)
			<-release
			_, _ = io.WriteString(w, "payload")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		server.Close()
	})
	client := &Client{
		cfg: &Config{
			Backend: BackendS3, Endpoint: server.URL,
			Bucket: "plans", Repository: "none",
		},
		st: newTestStorage(t, server.URL),
	}
	result := make(chan httpapi.Download, 1)
	errs := make(chan error, 1)
	go func() {
		download, getErr := (&HTTPOperations{Client: client}).GetUpload(
			context.Background(), httpapi.GetUploadRequest{URLOrKey: dir},
		)
		if getErr != nil {
			errs <- getErr
			return
		}
		result <- download
	}()

	select {
	case <-started:
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("storage body request did not start")
	}
	var download httpapi.Download
	select {
	case download = <-result:
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		releaseOnce.Do(func() { close(release) })
		t.Fatal("GetUpload buffered the storage body")
	}
	releaseOnce.Do(func() { close(release) })
	body, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := download.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if string(body) != "payload" || !strings.HasSuffix(download.Key, "plan.html") {
		t.Fatalf("download = %q, key = %q", body, download.Key)
	}
}

func rewriteManifest(
	t *testing.T, path string, records []ManifestRecord,
) error {
	t.Helper()
	var body strings.Builder
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		body.Write(encoded)
		body.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(body.String()), 0o600)
}

func TestServerSafeWarningsHideTemplatePath(t *testing.T) {
	const privatePath = "/private/airplan/template.html"
	warnings := serverSafeWarnings([]string{
		"custom template ignored for HTML input — HTML input is used as-is " +
			"(note: the template also failed to load: read " + privatePath + ")",
	})
	if len(warnings) != 1 || strings.Contains(warnings[0], privatePath) ||
		!strings.Contains(warnings[0], "configured template") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestHostedAdapterSafetyAndRequiredArrays(t *testing.T) {
	for _, test := range []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "", want: "none"},
		{input: "none", want: "none"},
		{
			input: "git@github.com:acme/airplan.git",
			want:  "https://github.com/acme/airplan",
		},
		{input: "auto", wantErr: true},
		{input: "file:///private/repo", wantErr: true},
	} {
		got, err := hostedRepositoryURL(test.input)
		if (err != nil) != test.wantErr || got != test.want {
			t.Fatalf("hostedRepositoryURL(%q) = %q, %v; want %q, error=%t",
				test.input, got, err, test.want, test.wantErr)
		}
	}
	warnings := serverSafeWarnings([]string{
		"manifest not recorded: /private/manifest.jsonl: denied",
	})
	if len(warnings) != 1 || strings.Contains(warnings[0], "/private") {
		t.Fatalf("warnings = %v", warnings)
	}
	if got := serverSafeItemError("delete /private/object: denied"); strings.Contains(got, "/private") || got == "" {
		t.Fatalf("item error = %q", got)
	}

	upload := wireUploadResult(&Result{}, nil)
	deleted := wireDeleteResult(&DeleteResult{})
	if upload.Warnings == nil || upload.Files == nil ||
		deleted.Keys == nil || deleted.Warnings == nil {
		t.Fatalf("required arrays are nil: upload=%+v delete=%+v", upload, deleted)
	}
	if string(deleted.Kind) != string(UploadKindDocument) {
		t.Fatalf("legacy delete kind = %q", deleted.Kind)
	}
}

func TestAPIOperationErrorClassifiesProtectedUpload(t *testing.T) {
	got := apiOperationError(&UploadProtectedError{Target: "dir"})
	var problem *httpapi.ProblemError
	if !errors.As(got, &problem) ||
		problem.Problem.Status != http.StatusConflict ||
		problem.Problem.Code != "upload_protected" {
		t.Fatalf("problem = %+v, error = %v", problem, got)
	}
}

func TestAPIOperationErrorClassifiesInvalidProtectReason(t *testing.T) {
	fake := newProtectStorage(t)
	client := newProtectTestClient(
		t, fake.server.URL, t.TempDir()+"/manifest.jsonl",
	)
	_, err := client.ProtectUpload(
		context.Background(), testDir,
		strings.Repeat("x", MaxProtectReasonRunes+1),
	)
	got := apiOperationError(err)
	var problem *httpapi.ProblemError
	if !errors.As(got, &problem) ||
		problem.Problem.Status != http.StatusUnprocessableEntity ||
		problem.Problem.Code != "invalid_protect_reason" {
		t.Fatalf("problem = %+v, error = %v", problem, got)
	}
}

func TestWireProtectionResultRequiredArrays(t *testing.T) {
	wire := wireProtectionResult(&ProtectionResult{
		ID: "dir", Protected: true,
	})
	if wire.Warnings == nil || !wire.Protected || wire.ProtectedAt != nil {
		t.Fatalf("wire = %+v", wire)
	}
	at := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	wire = wireProtectionResult(&ProtectionResult{ProtectedAt: at})
	if wire.ProtectedAt == nil || !wire.ProtectedAt.Equal(at) {
		t.Fatalf("wire = %+v", wire)
	}
}

func TestAPIOperationErrorDoesNotClassifyStorageText(t *testing.T) {
	storageErr := errors.New("bucket not found: InvalidAccessKeyId")
	if got := apiOperationError(storageErr); !errors.Is(got, storageErr) {
		t.Fatalf("error = %v, want original internal error", got)
	}
}

func TestAPIOperationErrorClassifiesInvalidTarget(t *testing.T) {
	got := apiOperationError(&invalidTargetError{
		err: errors.New("airplan: invalid target"),
	})
	var problem *httpapi.ProblemError
	if !errors.As(got, &problem) ||
		problem.Problem.Status != http.StatusUnprocessableEntity ||
		problem.Problem.Code != "invalid_target" {
		t.Fatalf("problem = %+v, error = %v", problem, got)
	}
}

func TestAPIOperationErrorClassifiesUpgradeConflict(t *testing.T) {
	got := apiOperationError(ErrConflict)
	var problem *httpapi.ProblemError
	if !errors.As(got, &problem) ||
		problem.Problem.Status != http.StatusConflict ||
		problem.Problem.Code != "upgrade_conflict" {
		t.Fatalf("problem = %+v, error = %v", problem, got)
	}
}

func TestAPIOperationErrorClassifiesRevisionConflict(t *testing.T) {
	got := apiOperationError(&revisionAppendConflictError{err: ErrConflict})
	var problem *httpapi.ProblemError
	if !errors.As(got, &problem) ||
		problem.Problem.Status != http.StatusConflict ||
		problem.Problem.Code != "revision_conflict" {
		t.Fatalf("problem = %+v, error = %v", problem, got)
	}
}

func TestAPIOperationErrorClassifiesRevisionHistoryCapacity(t *testing.T) {
	got := apiOperationError(errors.Join(
		ErrRevisionHistoryFull, errors.New("capacity detail"),
	))
	var problem *httpapi.ProblemError
	if !errors.As(got, &problem) ||
		problem.Problem.Status != http.StatusUnprocessableEntity ||
		problem.Problem.Code != "revision_history_full" ||
		strings.Contains(problem.Problem.Detail, "capacity detail") {
		t.Fatalf("problem = %+v, error = %v", problem, got)
	}
}

func TestAPIOperationErrorClassifiesInvalidDocumentInput(t *testing.T) {
	got := apiOperationError(invalidDocumentInput(errors.New(
		"schema-valid generated path collision detail",
	)))
	var problem *httpapi.ProblemError
	if !errors.As(got, &problem) ||
		problem.Problem.Status != http.StatusUnprocessableEntity ||
		problem.Problem.Code != "invalid_upload" ||
		strings.Contains(problem.Problem.Detail, "collision detail") {
		t.Fatalf("problem = %+v, error = %v", problem, got)
	}
}

func TestAPIOperationErrorClassifiesUpdateRefusals(t *testing.T) {
	tests := []struct {
		kind   updateRefusalKind
		status int
		code   string
	}{
		{updateRefusalMissing, http.StatusNotFound, "upload_not_found"},
		{updateRefusalInvalidTarget, http.StatusUnprocessableEntity, "invalid_target"},
		{updateRefusalInvalidUpload, http.StatusUnprocessableEntity, "invalid_upload"},
	}
	for _, test := range tests {
		got := apiOperationError(&updateRefusalError{
			kind: test.kind, err: errors.New("target detail must not escape"),
		})
		var problem *httpapi.ProblemError
		if !errors.As(got, &problem) || problem.Problem.Status != test.status ||
			problem.Problem.Code != test.code ||
			strings.Contains(problem.Problem.Detail, "target detail") {
			t.Fatalf("kind %q problem = %+v, error = %v", test.kind, problem, got)
		}
	}
	storageErr := errors.New("post-commit storage failure")
	if got := apiOperationError(storageErr); !errors.Is(got, storageErr) {
		t.Fatalf("unexpected storage error was remapped: %v", got)
	}
}

func TestAPIOperationErrorClassifiesUpgradeRefusals(t *testing.T) {
	for _, test := range []struct {
		state  UpgradeState
		status int
		code   string
	}{
		{UpgradeStateMissing, http.StatusNotFound, "upload_not_found"},
		{UpgradeStateInvalid, http.StatusUnprocessableEntity, "invalid_upload"},
		{UpgradeStateIneligible, http.StatusUnprocessableEntity, "invalid_target"},
	} {
		t.Run(string(test.state), func(t *testing.T) {
			got := apiOperationError(&upgradeRefusalError{
				state: test.state, reason: "test refusal",
			})
			var problem *httpapi.ProblemError
			if !errors.As(got, &problem) ||
				problem.Problem.Status != test.status ||
				problem.Problem.Code != test.code {
				t.Fatalf("problem = %+v, error = %v", problem, got)
			}
		})
	}
}

func TestHostedEndpointsRejectSemanticInvalidTargets(t *testing.T) {
	const dir = "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3,
		Directory: dir, CreatedAt: time.Now().UTC(),
		Kind: UploadKindDocument, Slug: "plan", Format: "md",
		Objects: []MarkerObject{{
			Name: "plan.html", Role: MarkerRolePage,
			Bytes: 7, ContentType: "text/html; charset=utf-8",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	storageServer := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("list-type") == "2" {
				writeListXML(t, w, []objectInfo{
					{Key: dir + "/" + MarkerFilename, Size: int64(len(marker))},
					{Key: dir + "/plan.html", Size: 7},
				})
				return
			}
			switch r.URL.Path {
			case "/plans/" + dir + "/" + MarkerFilename:
				w.Header().Set("Content-Type", markerContentType)
				_, _ = w.Write(marker)
			default:
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w,
					`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
			}
		},
	))
	t.Cleanup(storageServer.Close)
	client := &Client{
		cfg: &Config{
			Backend: BackendS3, Endpoint: storageServer.URL,
			Bucket: "plans", Repository: "none",
		},
		st: newTestStorage(t, storageServer.URL),
	}
	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(
		&HTTPOperations{Client: client}, httpapi.Options{Token: token},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		endpoint string
		target   string
	}{
		{endpoint: "/api/v1/uploads/inspect", target: dir + "/nested/file"},
		{endpoint: "/api/v1/uploads/get", target: dir + "/other.html"},
		{endpoint: "/api/v1/uploads/delete", target: dir + "/other.html"},
		{endpoint: "/api/v1/uploads/protect", target: dir + "/other.html"},
		{endpoint: "/api/v1/uploads/unprotect", target: dir + "/other.html"},
	} {
		t.Run(test.endpoint, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{
				"url_or_key": test.target,
			})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(
				http.MethodPost, test.endpoint, strings.NewReader(string(body)),
			)
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, body = %s",
					recorder.Code, recorder.Body.String())
			}
			var problem httpapi.Problem
			if err := json.NewDecoder(recorder.Body).Decode(&problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != "invalid_target" {
				t.Fatalf("problem = %+v", problem)
			}
		})
	}
}

func TestHostedDeleteClassifiesMissingMarkerReconciliation(t *testing.T) {
	const dir = "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	storageServer := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w,
				`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
		},
	))
	t.Cleanup(storageServer.Close)
	const token = "01234567890123456789012345678901"
	for _, test := range []struct {
		name       string
		target     string
		withRecord bool
		wantStatus int
		wantCode   string
	}{
		{
			name: "no active record", target: dir,
			wantStatus: http.StatusNotFound, wantCode: "upload_not_found",
		},
		{
			name: "unrecorded payload", target: dir + "/other.html",
			withRecord: true, wantStatus: http.StatusUnprocessableEntity,
			wantCode: "invalid_target",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifestPath := t.TempDir() + "/manifest.jsonl"
			if test.withRecord {
				err := appendManifestRecord(context.Background(), manifestPath,
					ManifestRecord{
						Type: "upload", Time: time.Now().UTC(),
						Key:    dir + "/plan.html",
						URL:    "https://plans.example/" + dir + "/plan.html",
						Bucket: "plans", Bytes: 1,
						Kind:          string(UploadKindDocument),
						MarkerVersion: MarkerVersion,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
			}
			client := &Client{
				cfg: &Config{
					Backend: BackendS3, Endpoint: storageServer.URL,
					Bucket: "plans", Repository: "none",
					ManifestPath: manifestPath,
				},
				st: newTestStorage(t, storageServer.URL),
			}
			handler, err := httpapi.NewHandler(
				&HTTPOperations{Client: client},
				httpapi.Options{Token: token},
			)
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(map[string]string{
				"url_or_key": test.target,
			})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(
				http.MethodPost, "/api/v1/uploads/delete",
				strings.NewReader(string(body)),
			)
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s",
					recorder.Code, recorder.Body.String())
			}
			var problem httpapi.Problem
			if err := json.NewDecoder(recorder.Body).Decode(&problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != test.wantCode {
				t.Fatalf("problem = %+v", problem)
			}
		})
	}
}

func manifestUploadRecord(
	when time.Time, profile, bucket, prefix, title string,
) ManifestRecord {
	dir := "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	if title != "a" {
		dir = string(title[0]) + dir[1:]
	}
	key := BuildKey(prefix, dir, title+".html")
	return ManifestRecord{
		Type: "upload", Time: when, Key: key,
		MarkerKey: BuildKey(prefix, dir, MarkerFilename),
		URL:       "https://plans.example/" + key, Bucket: bucket,
		Profile: profile, Format: "md", Kind: string(UploadKindDocument),
		Slug: title, Title: title, Bytes: 10, MarkerVersion: MarkerVersion,
	}
}
