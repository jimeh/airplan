//go:build integration

package airplan

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jimeh/airplan/internal/httpapi"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

// assertDeclaredTotalsRoundTrip checks that an upload's marker-declared totals
// survive a real storage round trip: sync imports exactly what the marker
// declares, and backfill restores the same values for history that predates
// them, converging in one pass (SPEC.md §9).
func assertDeclaredTotalsRoundTrip(
	ctx context.Context, t *testing.T, syncClient *Client,
	synced *SyncManifestResult, res *Result,
) {
	t.Helper()

	var imported *ManifestRecord
	for index := range synced.Added {
		if synced.Added[index].MarkerKey == res.MarkerKey {
			imported = &synced.Added[index]
		}
	}
	if imported == nil {
		t.Fatalf("sync did not import %q: %+v", res.MarkerKey, synced.Added)
		return
	}
	inspection, err := syncClient.InspectUpload(ctx, res.MarkerKey)
	if err != nil {
		t.Fatal(err)
	}
	wantObjects, wantBytes, ok := MarkerDeclaredTotals(
		*inspection.marker, inspection.markerBytes,
	)
	if !ok {
		t.Fatalf("marker %q declares no totals", res.MarkerKey)
	}
	if imported.Objects != wantObjects || imported.TotalBytes != wantBytes {
		t.Fatalf("imported %d/%d, want %d/%d", imported.Objects,
			imported.TotalBytes, wantObjects, wantBytes)
	}

	path := syncClient.cfg.ManifestPath
	stripDeclaredTotals(t, path)
	backfilled, err := syncClient.SyncManifest(ctx, SyncManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(backfilled.Added) != 0 || len(backfilled.Enriched) == 0 {
		t.Fatalf("backfill = %+v, want enrichment only", backfilled)
	}
	records := manifestRecordsByMarker(t, path)
	if got := records[res.MarkerKey]; got.Objects != wantObjects ||
		got.TotalBytes != wantBytes {
		t.Fatalf("backfilled %d/%d, want %d/%d", got.Objects, got.TotalBytes,
			wantObjects, wantBytes)
	}
	second, err := syncClient.SyncManifest(ctx, SyncManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Enriched) != 0 {
		t.Fatalf("second sync enriched again: %+v", second.Enriched)
	}
}

// TestIntegrationRoundTrip uploads through the real pipeline against a
// live S3-compatible server (MinIO, managed by testcontainers) and
// verifies bytes and headers by fetching the objects back. Excluded
// from plain `go test ./...` by the integration build tag; run via
// `mise run test-integration`.
func TestIntegrationRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Minute,
	)
	defer cancel()

	// Keep the reviewed release tag and multi-platform digest together.
	// Refresh both, inspect the image labels, then run this test before
	// accepting a newer MinIO image.
	const minioImage = "minio/minio:RELEASE.2025-09-07T16-13-09Z@" +
		"sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
	minioC, err := tcminio.Run(ctx, minioImage)
	testcontainers.CleanupContainer(t, minioC)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := minioC.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Endpoint:        "http://" + endpoint,
		Bucket:          "airplan-test",
		Region:          "us-east-1",
		AccessKeyID:     minioC.Username,
		SecretAccessKey: minioC.Password,
		DisableManifest: true,
		Repository:      "https://github.com/jimeh/airplan",
	}

	st, err := newStorage(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(cfg.Bucket),
	})
	if err != nil {
		t.Fatal(err)
	}

	client, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	src := "# Integration Plan\n\nSome **bold** text.\n\n" +
		"```go\nfmt.Println(\"hi\")\n```\n"
	res, err := client.Upload(ctx, Input{
		Reader: strings.NewReader(src),
		Name:   "integration-plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.Title != "Integration Plan" {
		t.Errorf("Title = %q, want %q", res.Title, "Integration Plan")
	}
	if !strings.HasSuffix(res.Key, "/integration-plan.html") {
		t.Errorf("unexpected page key %q", res.Key)
	}
	if !strings.HasSuffix(res.SourceKey, "/integration-plan.md") {
		t.Errorf("unexpected source key %q", res.SourceKey)
	}

	page := getObject(ctx, t, st, res.Key)
	if page.contentType != "text/html; charset=utf-8" {
		t.Errorf("page Content-Type = %q", page.contentType)
	}
	if page.cacheControl != "no-store" {
		t.Errorf("page Cache-Control = %q", page.cacheControl)
	}
	if page.metaTitle != "Integration Plan" {
		t.Errorf("page x-amz-meta-title = %q", page.metaTitle)
	}
	if int64(len(page.body)) != res.Bytes {
		t.Errorf("page bytes = %d, Result.Bytes = %d",
			len(page.body), res.Bytes)
	}
	if !strings.Contains(string(page.body), "Some <strong>bold</strong>") {
		t.Error("page body missing rendered markdown")
	}
	if !strings.Contains(string(page.body),
		`href="./integration-plan.md" download`) {
		t.Error("page missing download link to sibling source")
	}
	if !strings.Contains(string(page.body),
		`class="raw" href="./integration-plan.md"`) {
		t.Error("page missing raw link to sibling source")
	}

	source := getObject(ctx, t, st, res.SourceKey)
	if string(source.body) != src {
		t.Error("source object bytes differ from input")
	}
	if source.contentType != "text/markdown; charset=utf-8" {
		t.Errorf("source Content-Type = %q", source.contentType)
	}
	if source.cacheControl != "no-store" {
		t.Errorf("source Cache-Control = %q", source.cacheControl)
	}

	dirPrefix, err := uploadDirPrefix(res.Key)
	if err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSuffix(dirPrefix, "/")
	dir = dir[strings.LastIndex(dir, "/")+1:]
	markerKey := dirPrefix + MarkerFilename
	markerObject := getObject(ctx, t, st, markerKey)
	if markerObject.contentType != "application/json" {
		t.Errorf("marker Content-Type = %q", markerObject.contentType)
	}
	if markerObject.cacheControl != "no-store" {
		t.Errorf("marker Cache-Control = %q", markerObject.cacheControl)
	}
	marker, err := DecodeUploadMarker(markerObject.body, dir)
	if err != nil {
		t.Fatal(err)
	}
	if marker.Page != "integration-plan.html" ||
		marker.Source != "integration-plan.md" ||
		marker.Title != "Integration Plan" || marker.Format != "md" ||
		marker.PageBytes != int64(len(page.body)) ||
		marker.Repo != "https://github.com/jimeh/airplan" {
		t.Fatalf("uploaded marker = %+v", marker)
	}

	remote, err := client.ListRemote(ctx)
	if err != nil {
		t.Fatal(err)
	}
	indexed := remoteByDir(t, remote, dir)
	wantBytes := int64(len(markerObject.body) + len(page.body) + len(source.body))
	if indexed.Slug != "integration-plan" || indexed.Objects != 3 ||
		indexed.Bytes != wantBytes {
		t.Fatalf("indexed upload = %+v, want 3 objects and %d bytes",
			indexed, wantBytes)
	}
	inspection, err := client.InspectUpload(ctx, res.URL)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != UploadComplete || inspection.Page == nil ||
		!inspection.Page.Exists || inspection.Source == nil ||
		!inspection.Source.Exists {
		t.Fatalf("complete inspection = %+v", inspection)
	}

	// Exercise real S3-compatible conditional upgrade writes. First prove a
	// stale marker ETag fails closed without touching the page, then re-plan and
	// migrate the same source-backed document from marker v3 to the current version.
	legacyMarker := *marker
	legacyMarker.Version = 3
	legacyMarker.Producer = Producer{}
	legacyMarker.Render = nil
	legacyMarker.Entrypoint = ""
	legacyMarker.Pages = nil
	legacyBody, err := EncodeUploadMarker(legacyMarker)
	if err != nil {
		t.Fatal(err)
	}
	putIntegrationObject(ctx, t, st, object{
		Key: markerKey, Body: legacyBody, ContentType: markerContentType,
	})
	stalePlan, err := client.PlanUpgradeDocument(ctx, res.URL, UpgradeDocumentOptions{})
	if err != nil || stalePlan.State != UpgradeStateUpgradeable {
		t.Fatalf("stale upgrade plan = %+v, %v", stalePlan, err)
	}
	concurrentMarker := legacyMarker
	concurrentMarker.Title = "Concurrent change"
	concurrentBody, err := EncodeUploadMarker(concurrentMarker)
	if err != nil {
		t.Fatal(err)
	}
	putIntegrationObject(ctx, t, st, object{
		Key: markerKey, Body: concurrentBody, ContentType: markerContentType,
	})
	if _, err := client.UpgradeDocument(ctx, *stalePlan); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale upgrade error = %v, want conflict", err)
	}
	if got := getObject(ctx, t, st, res.Key); !bytes.Equal(got.body, page.body) {
		t.Fatal("stale upgrade changed the page")
	}
	putIntegrationObject(ctx, t, st, object{
		Key: markerKey, Body: legacyBody, ContentType: markerContentType,
	})
	upgradePlan, err := client.PlanUpgradeDocument(ctx, res.URL, UpgradeDocumentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := client.UpgradeDocument(ctx, *upgradePlan)
	if err != nil || !upgraded.Upgraded || upgraded.Result.URL != res.URL {
		t.Fatalf("upgrade = %+v, %v", upgraded, err)
	}
	markerObject = getObject(ctx, t, st, markerKey)
	marker, err = DecodeUploadMarker(markerObject.body, dir)
	if err != nil || marker.Version != MarkerVersion ||
		marker.Producer.Name != "airplan" || marker.Render == nil {
		t.Fatalf("upgraded marker = %+v, %v", marker, err)
	}
	page = getObject(ctx, t, st, res.Key)
	if got := getObject(ctx, t, st, res.SourceKey); !bytes.Equal(got.body, source.body) {
		t.Fatal("upgrade changed source bytes")
	}
	if _, err := st.getBytes(ctx, dirPrefix+".airplan-versions.json", MaxMarkerSize); !errors.Is(err, errObjectNotFound) {
		t.Fatalf("standalone upgrade versions metadata error = %v", err)
	}
	testRevisionRoundTrip(ctx, t, client, st)

	collection, err := client.UploadFiles(ctx, FilesInput{Files: []FileInput{
		{Name: "shot.png", Reader: bytes.NewReader([]byte("png")), Size: 3},
		{Name: "demo.webm", Reader: bytes.NewReader([]byte("video")), Size: 5},
	}})
	if err != nil {
		t.Fatal(err)
	}
	collectionDirPrefix, err := uploadDirPrefix(collection.Key)
	if err != nil {
		t.Fatal(err)
	}
	collectionDir := strings.TrimSuffix(collectionDirPrefix, "/")
	collectionDir = collectionDir[strings.LastIndex(collectionDir, "/")+1:]
	collectionMarker := getObject(ctx, t, st, collection.MarkerKey)
	decodedCollection, err := DecodeUploadMarkerForName(
		collectionMarker.body, collectionDir, CollectionMarkerFilename,
	)
	if err != nil || decodedCollection.Kind != UploadKindCollection ||
		len(decodedCollection.Objects) != 3 {
		t.Fatalf("collection marker = %+v, %v", decodedCollection, err)
	}
	if got := getObject(ctx, t, st, collection.Files[1].Key); string(got.body) != "video" || got.contentType != "video/webm" {
		t.Fatalf("collection video = %+v", got)
	}
	collectionInspection, err := client.InspectUpload(ctx, collection.Files[0].URL)
	if err != nil || collectionInspection.State != UploadComplete || len(collectionInspection.Files) != 2 {
		t.Fatalf("collection inspection = %+v, %v", collectionInspection, err)
	}
	var downloaded bytes.Buffer
	if _, err := client.GetUploadTo(ctx, collection.Files[1].URL, GetOptions{}, &downloaded); err != nil || downloaded.String() != "video" {
		t.Fatalf("streamed collection member = %q, %v", downloaded.String(), err)
	}
	conflictDir := "zzzzzzzzzzzzzzzzzzzzzzzzzz"
	putIntegrationObject(ctx, t, st, object{
		Key:  conflictDir + "/" + MarkerFilename,
		Body: []byte(`{}`), ContentType: markerContentType,
	})
	putIntegrationObject(ctx, t, st, object{
		Key:  conflictDir + "/" + CollectionMarkerFilename,
		Body: []byte(`{}`), ContentType: markerContentType,
	})
	conflictInspection, err := client.InspectUpload(ctx, conflictDir)
	if err != nil || conflictInspection.Error != MarkerErrorConflictingMarkers {
		t.Fatalf("conflict inspection = %+v, %v", conflictInspection, err)
	}
	if _, err := client.DeleteUpload(ctx, conflictDir); err == nil {
		t.Fatal("delete accepted conflicting ownership markers")
	}
	if err := st.deleteKeys(ctx, []string{
		conflictDir + "/" + MarkerFilename,
		conflictDir + "/" + CollectionMarkerFilename,
	}); err != nil {
		t.Fatal(err)
	}

	partialDir := "bbbbbbbbbbbbbbbbbbbbbbbbbb"
	partialMarker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 1,
		Directory: partialDir,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Format:    "md",
		Page:      "missing.html",
		Source:    "missing.md",
		Title:     "Partial upload",
	})
	if err != nil {
		t.Fatal(err)
	}
	putIntegrationObject(ctx, t, st, object{
		Key: partialDir + "/" + MarkerFilename, Body: partialMarker,
		ContentType: "application/json",
	})

	unmarkedDir := "cccccccccccccccccccccccccc"
	putIntegrationObject(ctx, t, st, object{
		Key: unmarkedDir + "/unowned.html", Body: []byte("unowned"),
		ContentType: "text/html; charset=utf-8",
	})

	invalidDir := "dddddddddddddddddddddddddd"
	putIntegrationObject(ctx, t, st, object{
		Key:         invalidDir + "/" + MarkerFilename,
		Body:        []byte(`{"schema":"airplan-upload","version":99}`),
		ContentType: "application/json",
	})

	remote, err = client.ListRemote(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(remote) != 4 {
		t.Fatalf("remote uploads = %+v, want document, collection, partial, and invalid", remote)
	}
	if indexedCollection := remoteByDir(t, remote, collectionDir); indexedCollection.Kind != UploadKindCollection || !strings.HasSuffix(indexedCollection.Key, "/index.html") {
		t.Fatalf("indexed collection = %+v", indexedCollection)
	}
	remoteByDir(t, remote, partialDir)
	remoteByDir(t, remote, invalidDir)
	for _, upload := range remote {
		if upload.Dir == unmarkedDir {
			t.Fatal("markerless directory was remotely discoverable")
		}
	}

	inspection, err = client.InspectUpload(ctx, partialDir)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != UploadIncomplete || inspection.Page.Exists ||
		inspection.Source == nil || inspection.Source.Exists {
		t.Fatalf("partial inspection = %+v", inspection)
	}
	inspection, err = client.InspectUpload(ctx, invalidDir)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != UploadInvalid ||
		inspection.Error != MarkerErrorUnsupportedVersion {
		t.Fatalf("invalid inspection = %+v", inspection)
	}
	if _, err := client.DeleteUpload(ctx, invalidDir); err == nil {
		t.Fatal("delete accepted an invalid ownership marker")
	}
	getObject(ctx, t, st, invalidDir+"/"+MarkerFilename)

	syncCfg := *cfg
	syncCfg.DisableManifest = false
	syncCfg.ManifestPath = filepath.Join(t.TempDir(), "manifest.jsonl")
	syncCfg.Profile = "receiver"
	syncClient, err := New(ctx, &syncCfg)
	if err != nil {
		t.Fatal(err)
	}
	synced, err := syncClient.SyncManifest(ctx, SyncManifestOptions{Prune: true})
	if err != nil || len(synced.Added) != 2 || synced.Incomplete != 1 ||
		synced.Invalid != 1 {
		t.Fatalf("initial sync = %+v, %v", synced, err)
	}
	// Declared totals must survive a real storage round trip and match what the
	// uploading machine recorded, then be restorable by backfill (SPEC.md §9).
	assertDeclaredTotalsRoundTrip(ctx, t, syncClient, synced, res)

	deleted, err := client.DeleteUpload(ctx, res.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.Keys) != 3 || deleted.Keys[len(deleted.Keys)-1] != markerKey {
		t.Fatalf("delete operation order = %v", deleted.Keys)
	}
	objects, err := st.listKeys(ctx, dirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].Key != dirPrefix+VersionsFilename {
		t.Fatalf("unexpected objects remain after delete: %+v", objects)
	}
	if got := getObject(ctx, t, st, dirPrefix+VersionsFilename); !bytes.Equal(
		got.body, standaloneDeleteReservationBody,
	) {
		t.Fatalf("standalone delete tombstone = %q", got.body)
	}
	synced, err = syncClient.SyncManifest(ctx, SyncManifestOptions{Prune: true})
	if err != nil || len(synced.Tombstoned) != 1 {
		t.Fatalf("deletion sync = %+v, %v", synced, err)
	}
	putIntegrationObject(ctx, t, st, object{
		Key: markerKey, Body: markerObject.body,
		ContentType: markerContentType,
	})
	putIntegrationObject(ctx, t, st, object{
		Key: res.SourceKey, Body: source.body,
		ContentType: sourceContentType,
	})
	putIntegrationObject(ctx, t, st, object{
		Key: res.Key, Body: page.body, ContentType: pageContentType,
	})
	synced, err = syncClient.SyncManifest(ctx, SyncManifestOptions{Prune: true})
	if err != nil || len(synced.Added) != 1 {
		t.Fatalf("restoration sync = %+v, %v", synced, err)
	}
	if _, err := client.DeleteUpload(ctx, res.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteUpload(ctx, partialDir); err != nil {
		t.Fatal(err)
	}
	collectionDeleted, err := client.DeleteUpload(ctx, collection.Files[0].URL)
	if err != nil || collectionDeleted.Keys[len(collectionDeleted.Keys)-1] != collection.MarkerKey {
		t.Fatalf("collection delete = %+v, %v", collectionDeleted, err)
	}

	testPurgeProtectionLifecycle(ctx, t, client)

	remote, err = client.ListRemote(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(remote) != 1 || remote[0].Dir != invalidDir {
		t.Fatalf("remote uploads after deletes = %+v", remote)
	}

	testHTTPBackendRoundTrip(ctx, t, cfg)
}

// testPurgeProtectionLifecycle covers the purge-protection contract against
// real storage: protect → remote purge skips → delete refuses → forced
// delete succeeds, and protect → unprotect → purge deletes (SPEC.md §9).
func testPurgeProtectionLifecycle(
	ctx context.Context, t *testing.T, client *Client,
) {
	t.Helper()
	protectedDoc, err := client.Upload(ctx, Input{
		Reader: strings.NewReader("# Keep\n"), Name: "keep-plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	victim, err := client.Upload(ctx, Input{
		Reader: strings.NewReader("# Victim\n"), Name: "victim-plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	protection, err := client.ProtectUpload(
		ctx, protectedDoc.URL, "integration keep",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !protection.Protected ||
		protection.SentinelKey != protectedDoc.ID+"/"+ProtectedFilename {
		t.Fatalf("protection = %+v", protection)
	}

	inspection, err := client.InspectUpload(ctx, protectedDoc.URL)
	if err != nil || !inspection.Protected ||
		inspection.ProtectReason != "integration keep" {
		t.Fatalf("protected inspection = %+v, %v", inspection, err)
	}

	plan, err := client.PlanPurge(ctx, PurgePlanOptions{
		Source: UploadSourceStorage, All: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Protected) != 1 ||
		plan.Protected[0].UploadID != protectedDoc.ID {
		t.Fatalf("purge plan protected = %+v", plan.Protected)
	}
	ids := make([]string, 0, len(plan.Candidates))
	sawVictim := false
	for _, candidate := range plan.Candidates {
		if candidate.UploadID == protectedDoc.ID {
			t.Fatalf("protected upload selected as candidate: %+v", candidate)
		}
		sawVictim = sawVictim || candidate.UploadID == victim.ID
		ids = append(ids, candidate.UploadID)
	}
	if !sawVictim {
		t.Fatalf("victim missing from purge candidates: %+v", plan.Candidates)
	}
	if _, err := client.Purge(ctx, PurgeRequest{UploadIDs: ids}); err != nil {
		t.Fatal(err)
	}

	// A delete-time skip: purging the protected ID directly is refused by
	// the sentinel, reported as a skip rather than a failure.
	purged, err := client.Purge(ctx, PurgeRequest{
		UploadIDs: []string{protectedDoc.ID},
	})
	if err != nil || len(purged.Items) != 1 || !purged.Items[0].Protected {
		t.Fatalf("protected purge = %+v, %v", purged, err)
	}

	_, err = client.DeleteUpload(ctx, protectedDoc.URL)
	var protectedErr *UploadProtectedError
	if !errors.As(err, &protectedErr) ||
		protectedErr.Reason != "integration keep" {
		t.Fatalf("delete error = %v, want protected refusal", err)
	}
	forced, err := client.DeleteUploadWithOptions(
		ctx, protectedDoc.URL, DeleteOptions{Force: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(forced.Keys) < 3 ||
		forced.Keys[len(forced.Keys)-1] != protectedDoc.MarkerKey ||
		forced.Keys[len(forced.Keys)-2] != protection.SentinelKey {
		t.Fatalf("forced delete order = %v", forced.Keys)
	}

	// Protect then unprotect: ordinary purge deletes the upload again.
	cycled, err := client.Upload(ctx, Input{
		Reader: strings.NewReader("# Cycle\n"), Name: "cycle-plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ProtectUpload(ctx, cycled.URL, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UnprotectUpload(ctx, cycled.URL); err != nil {
		t.Fatal(err)
	}
	plan, err = client.PlanPurge(ctx, PurgePlanOptions{
		Source: UploadSourceStorage, All: true,
	})
	if err != nil || len(plan.Protected) != 0 {
		t.Fatalf("post-unprotect plan = %+v, %v", plan, err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].UploadID != cycled.ID {
		t.Fatalf("post-unprotect candidates = %+v", plan.Candidates)
	}
	purged, err = client.Purge(ctx, PurgeRequest{UploadIDs: []string{cycled.ID}})
	if err != nil || len(purged.Items) != 1 || purged.Items[0].Deleted == nil {
		t.Fatalf("post-unprotect purge = %+v, %v", purged, err)
	}
}

func testRevisionRoundTrip(
	ctx context.Context, t *testing.T, client *Client, st *storage,
) {
	t.Helper()
	first, err := client.Upload(ctx, Input{
		Reader: strings.NewReader("# Revision chain\n\nOne.\n"), Name: "chain.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ProtectUpload(ctx, first.URL, "keep first"); err != nil {
		t.Fatal(err)
	}
	second, err := client.UpdateDocument(ctx, UpdateDocumentInput{
		Target: first.URL,
		Input:  Input{Reader: strings.NewReader("# Revision chain\n\nTwo.\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := client.UpdateDocument(ctx, UpdateDocumentInput{
		Target: first.URL,
		Input:  Input{Reader: strings.NewReader("# Revision chain\n\nThree.\n")},
	})
	if err != nil || third.Revision != 3 || third.PreviousURL != second.URL {
		t.Fatalf("third revision = %+v, %v", third, err)
	}
	firstDoc, err := client.loadRevisionDocument(ctx, first.URL)
	if err != nil {
		t.Fatal(err)
	}
	claimBody, err := revisionCandidateCleanupClaimBody(firstDoc.versionsBody)
	if err != nil {
		t.Fatal(err)
	}
	metadataKey := firstDoc.dirPrefix + VersionsFilename
	if err := st.putConditional(ctx, object{
		Key: metadataKey, Body: claimBody, ContentType: markerContentType,
	}, firstDoc.versionsETag); err != nil {
		t.Fatalf("real conditional cleanup claim: %v", err)
	}
	claimedBody, claimedETag, _, err := st.getBytesWithETag(
		ctx, metadataKey, MaxVersionsMetadataSize,
	)
	if err != nil || bytes.Equal(claimedBody, firstDoc.versionsBody) ||
		claimedETag == firstDoc.versionsETag {
		t.Fatalf("cleanup claim body/etag = changed %t/%t, %v",
			!bytes.Equal(claimedBody, firstDoc.versionsBody),
			claimedETag != firstDoc.versionsETag, err)
	}
	if _, err := DecodeVersionsMetadata(claimedBody, client.cfg, firstDoc.pageKey); err != nil {
		t.Fatalf("cleanup claim metadata: %v", err)
	}
	if err := st.putConditional(ctx, object{
		Key: metadataKey, Body: firstDoc.versionsBody, ContentType: markerContentType,
	}, claimedETag); err != nil {
		t.Fatalf("restore metadata after cleanup claim proof: %v", err)
	}
	inspection, err := client.InspectUpload(ctx, first.URL)
	if err != nil || inspection.Revision != 1 || inspection.LatestRevision != 3 ||
		inspection.Versions == nil || !inspection.Protected {
		t.Fatalf("first revision inspection = %+v, %v", inspection, err)
	}
	var diff bytes.Buffer
	if _, err := client.GetUploadTo(ctx, third.ID, GetOptions{Diff: true}, &diff); err != nil ||
		!strings.Contains(diff.String(), "-Two.") || !strings.Contains(diff.String(), "+Three.") {
		t.Fatalf("third revision diff = %q, %v", diff.String(), err)
	}
	plan, err := client.PlanPurge(ctx, PurgePlanOptions{
		Source: UploadSourceStorage, All: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range plan.Candidates {
		if candidate.UploadID == first.ID || candidate.UploadID == second.ID ||
			candidate.UploadID == third.ID {
			t.Fatalf("default purge selected revision member: %+v", candidate)
		}
	}
	skipped, err := client.Purge(ctx, PurgeRequest{UploadIDs: []string{second.ID}})
	if err != nil || len(skipped.Items) != 1 || !skipped.Items[0].Versioned ||
		skipped.Items[0].Deleted != nil {
		t.Fatalf("delete-time versioned purge skip = %+v, %v", skipped, err)
	}
	if _, err := client.DeleteUpload(ctx, second.URL); err != nil {
		t.Fatal(err)
	}
	thirdDoc, err := client.loadRevisionDocument(ctx, third.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.tombstoneLinkedRevision(
		ctx, thirdDoc.dirPrefix, thirdDoc.marker,
	); err != nil {
		t.Fatal(err)
	}
	thirdObjects, err := client.st.listKeys(ctx, thirdDoc.dirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	thirdPayloads := make([]string, 0, len(thirdObjects))
	for _, object := range thirdObjects {
		if object.Key != thirdDoc.dirPrefix+MarkerFilename {
			thirdPayloads = append(thirdPayloads, object.Key)
		}
	}
	if err := client.st.deleteKeys(ctx, thirdPayloads); err != nil {
		t.Fatal(err)
	}
	// Simulate a marker-last interruption: the tombstone was propagated and
	// every payload was deleted, but the ownership marker survived. A retry
	// must finish the scoped delete even though local versions metadata is gone.
	if _, err := client.DeleteUpload(ctx, third.URL); err != nil {
		t.Fatalf("complete interrupted linked delete: %v", err)
	}
	fourth, err := client.UpdateDocument(ctx, UpdateDocumentInput{
		Target: first.URL,
		Input:  Input{Reader: strings.NewReader("# Revision chain\n\nFour.\n")},
	})
	if err != nil || fourth.Revision != 4 || fourth.PreviousURL != first.URL {
		t.Fatalf("post-tombstone revision = %+v, %v", fourth, err)
	}
	if _, err := client.UnprotectUpload(ctx, first.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteUpload(ctx, first.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteUpload(ctx, fourth.URL); err != nil {
		t.Fatalf("delete final revision: %v", err)
	}

	// This helper is part of a larger shared-bucket test. Remove the remaining
	// chain fixtures directly after public lifecycle behavior has been checked.
	for _, id := range []string{first.ID, second.ID, third.ID, fourth.ID} {
		objects, err := st.listKeys(ctx, id+"/")
		if err != nil {
			t.Fatal(err)
		}
		keys := make([]string, 0, len(objects))
		for _, object := range objects {
			keys = append(keys, object.Key)
		}
		if err := st.deleteKeys(ctx, keys); err != nil {
			t.Fatal(err)
		}
	}
}

func testHTTPBackendRoundTrip(
	ctx context.Context, t *testing.T, storageConfig *Config,
) {
	t.Helper()
	serverConfig := *storageConfig
	serverConfig.DisableManifest = false
	serverConfig.ManifestPath = filepath.Join(t.TempDir(), "server-manifest.jsonl")
	serverConfig.Profile = "server"
	serverConfig.Repository = "none"
	serverClient, err := New(ctx, &serverConfig)
	if err != nil {
		t.Fatal(err)
	}
	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(
		&HTTPOperations{Client: serverClient, ServerVersion: "integration"},
		httpapi.Options{Token: token},
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	remoteClient, err := New(ctx, &Config{
		Backend: BackendAirplan, APIURL: server.URL, APIToken: token,
		Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}

	document, err := remoteClient.Upload(ctx, Input{
		Reader: strings.NewReader("# HTTP Integration\n\nRemote body.\n"),
		Name:   "http-integration.md", MaxSize: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := remoteClient.ListManifest(ctx, ListManifestOptions{
		Scope: ManifestScopeService,
	})
	if err != nil || len(manifest.Records) != 1 ||
		manifest.Records[0].MarkerKey != document.MarkerKey {
		t.Fatalf("HTTP manifest = %+v, %v", manifest, err)
	}
	storage, err := remoteClient.ListRemote(ctx)
	if err != nil {
		t.Fatal(err)
	}
	remoteByDir(t, storage, document.ID)
	inspection, err := remoteClient.InspectUpload(ctx, document.URL)
	if err != nil || inspection.State != UploadComplete {
		t.Fatalf("HTTP inspection = %+v, %v", inspection, err)
	}
	var page bytes.Buffer
	if _, err := remoteClient.GetUploadTo(
		ctx, document.URL, GetOptions{}, &page,
	); err != nil || !strings.Contains(page.String(), "Remote body") {
		t.Fatalf("HTTP download = %q, %v", page.String(), err)
	}
	plan, err := remoteClient.PlanPurge(ctx, PurgePlanOptions{
		Source: UploadSourceManifest, All: true,
	})
	if err != nil || len(plan.Candidates) != 1 ||
		plan.Candidates[0].UploadID != document.ID {
		t.Fatalf("HTTP purge plan = %+v, %v", plan, err)
	}
	purged, err := remoteClient.Purge(ctx, PurgeRequest{
		UploadIDs: []string{document.ID},
	})
	if err != nil || len(purged.Items) != 1 ||
		purged.Items[0].Deleted == nil {
		t.Fatalf("HTTP purge = %+v, %v", purged, err)
	}
	revisionBase, err := remoteClient.Upload(ctx, Input{
		Reader: strings.NewReader("# HTTP revisions\n\nOne.\n"),
		Name:   "http-revisions.md", MaxSize: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := remoteClient.UpdateDocument(ctx, UpdateDocumentInput{
		Target: revisionBase.URL,
		Input: Input{
			Reader:  strings.NewReader("# HTTP revisions\n\nTwo.\n"),
			MaxSize: -1,
		},
	})
	if err != nil || revision.Revision != 2 || revision.PreviousURL != revisionBase.URL {
		t.Fatalf("HTTP revision update = %+v, %v", revision, err)
	}
	remoteInspection, err := remoteClient.InspectUpload(ctx, revisionBase.URL)
	if err != nil || remoteInspection.LatestRevision != 2 {
		t.Fatalf("HTTP revision inspection = %+v, %v", remoteInspection, err)
	}
	for _, id := range []string{revisionBase.ID, revision.ID} {
		objects, err := serverClient.st.listKeys(ctx, id+"/")
		if err != nil {
			t.Fatal(err)
		}
		keys := make([]string, 0, len(objects))
		for _, object := range objects {
			keys = append(keys, object.Key)
		}
		if err := serverClient.st.deleteKeys(ctx, keys); err != nil {
			t.Fatal(err)
		}
	}

	collection, err := remoteClient.UploadFiles(ctx, FilesInput{
		Files: []FileInput{
			{
				Name: "shot.png", Reader: bytes.NewReader([]byte("png")),
				Size: 3,
			},
			{
				Name: "notes.txt", Reader: bytes.NewReader([]byte("notes")),
				Size: 5,
			},
		},
		MaxSize: -1, MaxTotalSize: -1,
	})
	if err != nil || len(collection.Files) != 2 {
		t.Fatalf("HTTP collection = %+v, %v", collection, err)
	}
	var member bytes.Buffer
	if _, err := remoteClient.GetUploadTo(
		ctx, collection.Files[1].URL, GetOptions{}, &member,
	); err != nil || member.String() != "notes" {
		t.Fatalf("HTTP collection member = %q, %v", member.String(), err)
	}
	if _, err := remoteClient.DeleteUpload(
		ctx, collection.Files[0].URL,
	); err != nil {
		t.Fatal(err)
	}
}

type fetchedObject struct {
	body         []byte
	contentType  string
	cacheControl string
	metaTitle    string
}

func getObject(
	ctx context.Context,
	t *testing.T,
	st *storage,
	key string,
) fetchedObject {
	t.Helper()

	out, err := st.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(st.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	defer func() { _ = out.Body.Close() }()

	body, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}

	return fetchedObject{
		body:         body,
		contentType:  aws.ToString(out.ContentType),
		cacheControl: aws.ToString(out.CacheControl),
		metaTitle:    out.Metadata["title"],
	}
}

func putIntegrationObject(
	ctx context.Context,
	t *testing.T,
	st *storage,
	obj object,
) {
	t.Helper()
	if err := st.put(ctx, obj); err != nil {
		t.Fatal(err)
	}
}

func remoteByDir(
	t *testing.T,
	uploads []RemoteUpload,
	dir string,
) RemoteUpload {
	t.Helper()
	for _, upload := range uploads {
		if upload.Dir == dir {
			return upload
		}
	}
	t.Fatalf("remote upload %q not found in %+v", dir, uploads)
	return RemoteUpload{}
}
