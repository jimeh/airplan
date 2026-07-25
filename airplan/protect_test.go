package airplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jimeh/airplan/internal/httpapi"
)

func TestProtectUploadWritesSentinelAndManifest(t *testing.T) {
	fake := newProtectStorage(t)
	fake.addUploadV1(t, testDir, "plan.html")
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newProtectTestClient(t, fake.server.URL, manifest)

	res, err := client.ProtectUpload(context.Background(),
		"https://plans.example.com/"+testDir+"/plan.html", "README demo link")
	if err != nil {
		t.Fatal(err)
	}
	sentinelKey := testDir + "/" + ProtectedFilename
	if res.ID != testDir || res.SentinelKey != sentinelKey ||
		res.PageKey != testDir+"/plan.html" || !res.Protected ||
		res.Kind != UploadKindDocument || res.Reason != "README demo link" ||
		res.ProtectedAt.IsZero() {
		t.Fatalf("result = %+v", res)
	}

	body, ok := fake.object(sentinelKey)
	if !ok {
		t.Fatal("sentinel object was not written")
	}
	var sentinel protectionSentinel
	if err := json.Unmarshal(body, &sentinel); err != nil {
		t.Fatal(err)
	}
	if sentinel.Schema != ProtectionSchema ||
		sentinel.Version != ProtectionVersion ||
		sentinel.Reason != "README demo link" {
		t.Fatalf("sentinel = %+v", sentinel)
	}

	records, warnings, err := ReadManifest(manifest)
	if err != nil || len(warnings) != 0 || len(records) != 1 {
		t.Fatalf("manifest = %+v, warnings = %v, error = %v",
			records, warnings, err)
	}
	rec := records[0]
	if rec.Type != "protect" || rec.Key != testDir+"/plan.html" ||
		rec.MarkerKey != testDir+"/"+MarkerFilename || rec.Bucket != "plans" ||
		rec.Profile != "work" || rec.ProtectReason != "README demo link" {
		t.Fatalf("protect record = %+v", rec)
	}

	// Idempotent: protecting an already protected upload succeeds.
	if _, err := client.ProtectUpload(context.Background(),
		testDir, ""); err != nil {
		t.Fatal(err)
	}
}

func TestProtectUploadValidatesReasonBeforeAnyRequest(t *testing.T) {
	fake := newProtectStorage(t)
	fake.addUploadV1(t, testDir, "plan.html")
	client := newProtectTestClient(t, fake.server.URL, "")

	// The bound counts runes, not bytes: 256 two-byte runes are accepted.
	if _, err := client.ProtectUpload(context.Background(), testDir,
		strings.Repeat("é", MaxProtectReasonRunes)); err != nil {
		t.Fatalf("256-rune reason rejected: %v", err)
	}
	before := len(fake.operationOrder())
	_, err := client.ProtectUpload(context.Background(), testDir,
		strings.Repeat("é", MaxProtectReasonRunes+1))
	if err == nil || !strings.Contains(err.Error(), "exceeds 256 characters") {
		t.Fatalf("error = %v, want reason bound", err)
	}
	_, err = client.ProtectUpload(context.Background(), testDir,
		string([]byte{0xff, 0xfe}))
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("error = %v, want UTF-8 rejection", err)
	}
	// Reasons are echoed to terminals, so control characters are rejected.
	_, err = client.ProtectUpload(context.Background(), testDir,
		"keep\x1b[31m")
	if err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("error = %v, want control-character rejection", err)
	}
	if len(fake.operationOrder()) != before {
		t.Fatalf("invalid reasons reached storage: %v", fake.operationOrder())
	}
}

func TestProtectUploadRequiresMarkerAndDeclaredTarget(t *testing.T) {
	t.Run("missing marker", func(t *testing.T) {
		// Pre-marker legacy uploads have no ownership marker and can never
		// be protected.
		fake := newProtectStorage(t)
		fake.addObject(testDir+"/plan.html", []byte("page"), time.Now())
		client := newProtectTestClient(t, fake.server.URL, "")
		_, err := client.ProtectUpload(context.Background(), testDir, "")
		if err == nil || !strings.Contains(err.Error(),
			"ownership marker is missing") {
			t.Fatalf("error = %v, want missing-marker failure", err)
		}
	})
	t.Run("undeclared sibling", func(t *testing.T) {
		fake := newProtectStorage(t)
		fake.addUploadV1(t, testDir, "plan.html")
		client := newProtectTestClient(t, fake.server.URL, "")
		_, err := client.ProtectUpload(context.Background(),
			testDir+"/other.txt", "")
		if err == nil || !strings.Contains(err.Error(),
			`protect target "`+testDir+`/other.txt"`) {
			t.Fatalf("error = %v, want protect target rejection", err)
		}
		if _, ok := fake.object(testDir + "/" + ProtectedFilename); ok {
			t.Fatal("sentinel written for rejected target")
		}
	})
	t.Run("conflicting markers", func(t *testing.T) {
		fake := newProtectStorage(t)
		fake.addUploadV1(t, testDir, "plan.html")
		fake.addObject(testDir+"/"+CollectionMarkerFilename,
			[]byte(`{}`), time.Now())
		client := newProtectTestClient(t, fake.server.URL, "")
		_, err := client.ProtectUpload(context.Background(), testDir, "")
		assertMarkerCode(t, err, MarkerErrorConflictingMarkers)
		if _, ok := fake.object(testDir + "/" + ProtectedFilename); ok {
			t.Fatal("sentinel written despite marker conflict")
		}
		if _, err := client.UnprotectUpload(
			context.Background(), testDir,
		); err == nil {
			t.Fatal("unprotect accepted conflicting ownership markers")
		}
	})
}

func TestProtectUploadHonorsKeyPrefix(t *testing.T) {
	fake := newProtectStorage(t)
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: testDir,
		CreatedAt: time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC),
		Format:    "html", Page: "plan.html", Title: "Plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	fake.addObject("team/"+testDir+"/"+MarkerFilename, marker, when)
	fake.addObject("team/"+testDir+"/plan.html", []byte("page"), when)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client, err := New(context.Background(), &Config{
		Endpoint: fake.server.URL, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		ManifestPath: manifest, Profile: "work", KeyPrefix: "team",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.ProtectUpload(context.Background(),
		"team/"+testDir+"/plan.html", "keep")
	if err != nil {
		t.Fatal(err)
	}
	sentinelKey := "team/" + testDir + "/" + ProtectedFilename
	if res.SentinelKey != sentinelKey ||
		res.PageKey != "team/"+testDir+"/plan.html" {
		t.Fatalf("result = %+v", res)
	}
	if _, ok := fake.object(sentinelKey); !ok {
		t.Fatal("sentinel not written under the key prefix")
	}
	records, _, err := ReadManifest(manifest)
	if err != nil || len(records) != 1 ||
		records[0].MarkerKey != "team/"+testDir+"/"+MarkerFilename {
		t.Fatalf("manifest = %+v, error = %v", records, err)
	}
	if _, err := client.UnprotectUpload(
		context.Background(), "team/"+testDir,
	); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.object(sentinelKey); ok {
		t.Fatal("sentinel still present after unprotect")
	}
}

func TestProtectUploadCollection(t *testing.T) {
	fake := newProtectStorage(t)
	when := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: testDir,
		CreatedAt: when, Kind: UploadKindCollection,
		Objects: []MarkerObject{
			{
				Name: "index.html", Role: MarkerRolePage, Bytes: 8,
				ContentType: pageContentType,
			},
			{
				Name: "shot.png", Role: MarkerRoleFile, Bytes: 3,
				ContentType: "image/png",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fake.addObject(testDir+"/"+CollectionMarkerFilename, marker, when)
	fake.addObject(testDir+"/index.html", []byte("overview"), when)
	fake.addObject(testDir+"/shot.png", []byte("png"), when)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newProtectTestClient(t, fake.server.URL, manifest)

	res, err := client.ProtectUpload(context.Background(),
		testDir+"/shot.png", "evidence")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != UploadKindCollection ||
		res.PageKey != testDir+"/index.html" ||
		res.MarkerKey != testDir+"/"+CollectionMarkerFilename {
		t.Fatalf("result = %+v", res)
	}
	if _, ok := fake.object(testDir + "/" + ProtectedFilename); !ok {
		t.Fatal("sentinel not written for collection")
	}
	if _, err := client.UnprotectUpload(
		context.Background(), testDir,
	); err != nil {
		t.Fatal(err)
	}
	records, warnings, err := ReadManifest(manifest)
	if err != nil || len(warnings) != 0 || len(records) != 2 ||
		records[0].MarkerKey != testDir+"/"+CollectionMarkerFilename {
		t.Fatalf("manifest = %+v, warnings = %v, error = %v",
			records, warnings, err)
	}
}

func TestUnprotectUploadDeletesSentinelAndIsIdempotent(t *testing.T) {
	fake := newProtectStorage(t)
	fake.addUploadV1(t, testDir, "plan.html")
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newProtectTestClient(t, fake.server.URL, manifest)

	if _, err := client.ProtectUpload(
		context.Background(), testDir, "keep",
	); err != nil {
		t.Fatal(err)
	}
	res, err := client.UnprotectUpload(context.Background(), testDir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Protected || res.Reason != "" || !res.ProtectedAt.IsZero() {
		t.Fatalf("result = %+v", res)
	}
	if _, ok := fake.object(testDir + "/" + ProtectedFilename); ok {
		t.Fatal("sentinel still present after unprotect")
	}
	// Unprotecting an unprotected upload succeeds.
	if _, err := client.UnprotectUpload(context.Background(), testDir); err != nil {
		t.Fatal(err)
	}

	records, _, err := ReadManifest(manifest)
	if err != nil || len(records) != 3 || records[0].Type != "protect" ||
		records[1].Type != "unprotect" || records[2].Type != "unprotect" {
		t.Fatalf("manifest = %+v, error = %v", records, err)
	}
	if records[1].ProtectReason != "" {
		t.Fatalf("unprotect record carries a reason: %+v", records[1])
	}
}

func TestManifestWriterNeverEmitsDerivedProtection(t *testing.T) {
	fake := newProtectStorage(t)
	fake.addUploadV1(t, testDir, "plan.html")
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newProtectTestClient(t, fake.server.URL, manifest)
	if _, err := client.ProtectUpload(
		context.Background(), testDir, "keep",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UnprotectUpload(
		context.Background(), testDir,
	); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"protected"`, `"protected_at"`} {
		if strings.Contains(string(raw), field) {
			t.Fatalf("writer emitted derived field %s: %s", field, raw)
		}
	}
	// The derived fields exist only on reduced upload records.
	protected := ManifestRecord{
		Type: "upload", Time: time.Now().UTC(), Key: testDir + "/plan.html",
		URL:    "https://plans.example.com/" + testDir + "/plan.html",
		Bucket: "plans", Bytes: 1, MarkerVersion: MarkerVersion,
		Protected: true, ProtectedAt: time.Now().UTC(),
	}
	line, err := json.Marshal(protected)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(line), `"protected":true`) ||
		!strings.Contains(string(line), `"protected_at"`) {
		t.Fatalf("reduced record JSON = %s", line)
	}
}

func TestManifestUploadsProtectionReduction(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	pageKey := testDir + "/plan.html"
	markerKey := testDir + "/" + MarkerFilename
	upload := ManifestRecord{
		Type: "upload", Time: when, Key: pageKey, MarkerKey: markerKey,
		URL:    "https://plans.example.com/" + pageKey,
		Bucket: "plans", Bytes: 1, MarkerVersion: MarkerVersion,
	}
	protect := func(at time.Time, reason string) ManifestRecord {
		return ManifestRecord{
			Type: "protect", Time: at, Key: pageKey, MarkerKey: markerKey,
			Bucket: "plans", ProtectReason: reason,
		}
	}
	unprotect := ManifestRecord{
		Type: "unprotect", Time: when.Add(time.Hour), Key: pageKey,
		MarkerKey: markerKey, Bucket: "plans",
	}
	tombstone := ManifestRecord{
		Type: "delete", Time: when.Add(2 * time.Hour), Key: pageKey,
		MarkerKey: markerKey, Bucket: "plans", Reason: "deleted",
	}

	t.Run("latest protect wins", func(t *testing.T) {
		got := ManifestUploads([]ManifestRecord{
			upload,
			protect(when.Add(time.Minute), "first"),
			unprotect,
			protect(when.Add(2*time.Hour), "second"),
		})
		if len(got) != 1 || !got[0].Protected ||
			got[0].ProtectReason != "second" ||
			!got[0].ProtectedAt.Equal(when.Add(2*time.Hour)) {
			t.Fatalf("reduced = %+v", got)
		}
	})
	t.Run("unprotect clears", func(t *testing.T) {
		got := ManifestUploads([]ManifestRecord{
			upload, protect(when.Add(time.Minute), "keep"), unprotect,
		})
		if len(got) != 1 || got[0].Protected || got[0].ProtectReason != "" {
			t.Fatalf("reduced = %+v", got)
		}
	})
	t.Run("delete clears protection", func(t *testing.T) {
		later := upload
		later.Time = when.Add(3 * time.Hour)
		got := ManifestUploads([]ManifestRecord{
			upload, protect(when.Add(time.Minute), "keep"), tombstone, later,
		})
		if len(got) != 1 || got[0].Protected {
			t.Fatalf("reduced = %+v", got)
		}
	})
	t.Run("upload does not clear protection", func(t *testing.T) {
		reimported := upload
		reimported.Time = when.Add(3 * time.Hour)
		got := ManifestUploads([]ManifestRecord{
			upload, protect(when.Add(time.Minute), "keep"), reimported,
		})
		if len(got) != 1 || !got[0].Protected ||
			got[0].ProtectReason != "keep" {
			t.Fatalf("reduced = %+v", got)
		}
	})
	t.Run("protection without active upload stays hidden", func(t *testing.T) {
		got := ManifestUploads([]ManifestRecord{
			protect(when.Add(time.Minute), "keep"),
		})
		if len(got) != 0 {
			t.Fatalf("reduced = %+v", got)
		}
	})
	t.Run("stale derived fields never survive reduction", func(t *testing.T) {
		forged := upload
		forged.Protected = true
		forged.ProtectReason = "forged"
		got := ManifestUploads([]ManifestRecord{forged})
		if len(got) != 1 || got[0].Protected || got[0].ProtectReason != "" {
			t.Fatalf("reduced = %+v", got)
		}
	})
	t.Run("legacy uploads are never protected", func(t *testing.T) {
		legacy := upload
		legacy.MarkerVersion = 0
		legacy.MarkerKey = ""
		got := ManifestUploads([]ManifestRecord{
			legacy, protect(when.Add(time.Minute), "keep"),
		})
		if len(got) != 1 || got[0].Protected {
			t.Fatalf("reduced = %+v", got)
		}
	})
}

func TestReadManifestValidatesProtectionRecords(t *testing.T) {
	pageKey := testDir + "/plan.html"
	markerKey := testDir + "/" + MarkerFilename
	line := func(fields string) string {
		return `{"type":"protect","time":"2026-07-21T12:00:00Z",` +
			`"key":"` + pageKey + `","marker_key":"` + markerKey + `",` +
			`"bucket":"plans"` + fields + "}\n"
	}
	for _, tt := range []struct {
		name string
		line string
		want int
	}{
		{name: "valid protect", line: line(`,"protect_reason":"keep"`), want: 1},
		{name: "valid unprotect", line: strings.Replace(
			line(""), `"protect"`, `"unprotect"`, 1), want: 1},
		{
			name: "missing bucket",
			line: strings.Replace(line(""), `,"bucket":"plans"`, "", 1),
		},
		{
			name: "missing marker_key",
			line: strings.Replace(line(""),
				`"marker_key":"`+markerKey+`",`, "", 1),
		},
		{
			name: "marker_key outside page directory",
			line: strings.Replace(line(""), markerKey,
				strings.Repeat("z", 26)+"/"+MarkerFilename, 1),
		},
		{
			name: "non-UTC time",
			line: strings.Replace(line(""),
				"2026-07-21T12:00:00Z", "2026-07-21T12:00:00+01:00", 1),
		},
		{
			name: "oversized reason",
			line: line(`,"protect_reason":"` +
				strings.Repeat("x", MaxProtectReasonRunes+1) + `"`),
		},
		{
			name: "control characters in reason",
			line: line(`,"protect_reason":"keep\u001b[31m"`),
		},
		{
			name: "unprotect with reason",
			line: strings.Replace(line(`,"protect_reason":"keep"`),
				`"protect"`, `"unprotect"`, 1),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.jsonl")
			if err := os.WriteFile(path, []byte(tt.line), 0o600); err != nil {
				t.Fatal(err)
			}
			records, warnings, err := ReadManifest(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != tt.want {
				t.Fatalf("records = %+v, warnings = %v, want %d records",
					records, warnings, tt.want)
			}
			if tt.want == 0 && len(warnings) != 1 {
				t.Fatalf("warnings = %v, want one skip warning", warnings)
			}
		})
	}
}

func TestDeleteUploadRefusesProtected(t *testing.T) {
	valid, err := encodeProtectionSentinel(
		time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC), "README demo link",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name       string
		body       []byte
		failStatus int
		wantReason string
	}{
		{name: "valid body", body: valid, wantReason: "README demo link"},
		{name: "empty body", body: []byte{}},
		{name: "malformed body", body: []byte(`{"schema":`)},
		{
			name: "oversized body",
			body: []byte(`{"reason":"` +
				strings.Repeat("x", MaxMarkerSize) + `"}`),
		},
		{
			name: "sentinel read failure fails closed",
			body: valid, failStatus: http.StatusInternalServerError,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := newProtectStorage(t)
			fake.addUploadV1(t, testDir, "plan.html")
			sentinelKey := testDir + "/" + ProtectedFilename
			fake.addObject(sentinelKey, tt.body, time.Now())
			if tt.failStatus != 0 {
				fake.setGetFailure(sentinelKey, tt.failStatus)
			}
			client := newProtectTestClient(t, fake.server.URL, "")

			_, err := client.DeleteUpload(context.Background(), testDir)
			var protectedErr *UploadProtectedError
			if !errors.As(err, &protectedErr) {
				t.Fatalf("error = %v, want UploadProtectedError", err)
			}
			if protectedErr.Reason != tt.wantReason ||
				protectedErr.Target != testDir {
				t.Fatalf("protected error = %+v", protectedErr)
			}
			if !strings.Contains(err.Error(), "airplan unprotect "+testDir) ||
				!strings.Contains(err.Error(), "--force") {
				t.Fatalf("error text = %q", err)
			}
			if calls := fake.deleteCalls(); calls != 0 {
				t.Fatalf("delete calls = %d, want 0", calls)
			}
		})
	}
}

func TestDeleteUploadForceOrdersPayloadSentinelMarker(t *testing.T) {
	fake := newProtectStorage(t)
	fake.addUploadV1(t, testDir, "plan.html")
	sentinelKey := testDir + "/" + ProtectedFilename
	fake.addObject(sentinelKey, nil, time.Now())
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newProtectTestClient(t, fake.server.URL, manifest)

	res, err := client.DeleteUploadWithOptions(
		context.Background(), testDir, DeleteOptions{Force: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	markerKey := testDir + "/" + MarkerFilename
	if len(res.Keys) != 3 || res.Keys[1] != sentinelKey ||
		res.Keys[2] != markerKey {
		t.Fatalf("keys = %v, want payload, sentinel, marker order", res.Keys)
	}
	var mutations []string
	for _, op := range fake.operationOrder() {
		if strings.HasPrefix(op, "post-delete") ||
			strings.HasPrefix(op, "delete:") {
			mutations = append(mutations, op)
		}
	}
	want := []string{
		"post-delete", "delete:" + sentinelKey, "delete:" + markerKey,
	}
	if strings.Join(mutations, ",") != strings.Join(want, ",") {
		t.Fatalf("mutations = %v, want %v", mutations, want)
	}
	records, _, err := ReadManifest(manifest)
	if err != nil || len(records) != 1 || records[0].Type != "delete" {
		t.Fatalf("manifest = %+v, error = %v", records, err)
	}
}

func TestPurgeReportsDeleteTimeProtectedAsSkipped(t *testing.T) {
	fake := newProtectStorage(t)
	fake.addUploadV1(t, testDir, "plan.html")
	fake.addObject(testDir+"/"+ProtectedFilename, nil, time.Now())
	client := newProtectTestClient(t, fake.server.URL, "")

	result, err := client.Purge(context.Background(), PurgeRequest{
		UploadIDs: []string{testDir},
	})
	if err != nil {
		t.Fatalf("protected skip must not fail purge: %v", err)
	}
	if len(result.Items) != 1 || !result.Items[0].Protected ||
		result.Items[0].Error != "" || result.Items[0].Deleted != nil {
		t.Fatalf("items = %+v", result.Items)
	}
	if calls := fake.deleteCalls(); calls != 0 {
		t.Fatalf("delete calls = %d, want 0", calls)
	}
}

func TestPlanPurgeManifestSourceFiltersProtected(t *testing.T) {
	when := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	otherDir := strings.Repeat("b", 26)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	upload := func(dir string) ManifestRecord {
		return ManifestRecord{
			Type: "upload", Time: when, Key: dir + "/plan.html",
			MarkerKey: dir + "/" + MarkerFilename,
			URL:       "https://plans.example.com/" + dir + "/plan.html",
			Bucket:    "plans", Profile: "work", Bytes: 1,
			MarkerVersion: MarkerVersion,
		}
	}
	for _, rec := range []ManifestRecord{
		upload(testDir), upload(otherDir),
		{
			Type: "protect", Time: when.Add(time.Minute),
			Key:       testDir + "/plan.html",
			MarkerKey: testDir + "/" + MarkerFilename, Bucket: "plans",
			Profile: "work", ProtectReason: "keep",
		},
	} {
		if err := appendManifestRecord(
			context.Background(), manifest, rec,
		); err != nil {
			t.Fatal(err)
		}
	}
	client := newProtectTestClient(t, "https://unused.example.com", manifest)

	plan, err := client.PlanPurge(context.Background(), PurgePlanOptions{
		Source: UploadSourceManifest, All: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].UploadID != otherDir {
		t.Fatalf("candidates = %+v", plan.Candidates)
	}
	if len(plan.Protected) != 1 || plan.Protected[0].UploadID != testDir ||
		!plan.Protected[0].Record.Protected {
		t.Fatalf("protected = %+v", plan.Protected)
	}
}

func TestPlanPurgeStorageSourceFiltersProtected(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	otherDir := strings.Repeat("b", 26)
	fake := newProtectStorage(t)
	fake.addUploadV1(t, testDir, "plan.html")
	fake.addUploadV1(t, otherDir, "plan.html")
	sentinel, err := encodeProtectionSentinel(when, "keep")
	if err != nil {
		t.Fatal(err)
	}
	fake.addObject(testDir+"/"+ProtectedFilename, sentinel, when)
	client := newProtectTestClient(t, fake.server.URL, "")

	plan, err := client.PlanPurge(context.Background(), PurgePlanOptions{
		Source: UploadSourceStorage, All: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].UploadID != otherDir {
		t.Fatalf("candidates = %+v", plan.Candidates)
	}
	if len(plan.Protected) != 1 || plan.Protected[0].UploadID != testDir ||
		plan.Protected[0].Record.ProtectReason != "keep" {
		t.Fatalf("protected = %+v", plan.Protected)
	}
}

func TestListRemoteDetectsProtectionSentinel(t *testing.T) {
	fake := newProtectStorage(t)
	fake.addUploadV1(t, testDir, "plan.html")
	fake.addObject(testDir+"/"+ProtectedFilename, nil, time.Now())
	otherDir := strings.Repeat("b", 26)
	fake.addUploadV1(t, otherDir, "plan.html")
	client := newProtectTestClient(t, fake.server.URL, "")

	uploads, err := client.ListRemote(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byDir := map[string]RemoteUpload{}
	for _, upload := range uploads {
		byDir[upload.Dir] = upload
	}
	if !byDir[testDir].Protected || byDir[otherDir].Protected {
		t.Fatalf("uploads = %+v", uploads)
	}
	// The sentinel is an ordinary extra object: it counts toward objects
	// and bytes but never affects completeness.
	if byDir[testDir].Objects != byDir[otherDir].Objects+1 {
		t.Fatalf("objects = %d vs %d",
			byDir[testDir].Objects, byDir[otherDir].Objects)
	}
}

func TestInspectUploadReportsProtection(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	valid, err := encodeProtectionSentinel(when, "README demo link")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name       string
		body       []byte
		wantAt     time.Time
		wantReason string
	}{
		{
			name: "valid body", body: valid,
			wantAt: when, wantReason: "README demo link",
		},
		{name: "empty body", body: []byte{}},
		{name: "malformed body", body: []byte(`{"schema":`)},
		{
			name: "oversized body",
			body: []byte(`{"reason":"` +
				strings.Repeat("x", MaxMarkerSize) + `"}`),
		},
		{
			name: "out-of-bound reason dropped",
			body: []byte(`{"schema":"airplan-protection","version":1,` +
				`"created_at":"2026-07-21T12:00:00Z","reason":"` +
				strings.Repeat("x", MaxProtectReasonRunes+1) + `"}`),
			wantAt: when,
		},
		{
			// encoding/json would substitute U+FFFD for the invalid bytes,
			// so the whole advisory body is dropped instead.
			name: "invalid UTF-8 body dropped",
			body: []byte(`{"schema":"airplan-protection","version":1,` +
				`"created_at":"2026-07-21T12:00:00Z","reason":"a` +
				string([]byte{0xff, 0xfe}) + `"}`),
		},
		{
			name: "control-character reason dropped",
			body: []byte(`{"schema":"airplan-protection","version":1,` +
				`"created_at":"2026-07-21T12:00:00Z",` +
				`"reason":"keep\u001b[31m"}`),
			wantAt: when,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := newProtectStorage(t)
			fake.addUploadV1(t, testDir, "plan.html")
			fake.addObject(testDir+"/"+ProtectedFilename, tt.body, when)
			client := newProtectTestClient(t, fake.server.URL, "")

			inspection, err := client.InspectUpload(
				context.Background(), testDir,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !inspection.Protected {
				t.Fatalf("inspection = %+v, want protected", inspection)
			}
			if inspection.State != UploadComplete {
				t.Fatalf("state = %q, sentinel must not affect completeness",
					inspection.State)
			}
			if inspection.ProtectReason != tt.wantReason {
				t.Fatalf("reason = %q, want %q",
					inspection.ProtectReason, tt.wantReason)
			}
			wantAt := tt.wantAt
			if wantAt.IsZero() {
				// The listing timestamp is the advisory fallback.
				wantAt = when
			}
			if !inspection.ProtectedAt.Equal(wantAt) {
				t.Fatalf("protected_at = %v, want %v",
					inspection.ProtectedAt, wantAt)
			}
		})
	}
}

func TestInspectUploadWithoutSentinelIsUnprotected(t *testing.T) {
	fake := newProtectStorage(t)
	fake.addUploadV1(t, testDir, "plan.html")
	client := newProtectTestClient(t, fake.server.URL, "")

	inspection, err := client.InspectUpload(context.Background(), testDir)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Protected || !inspection.ProtectedAt.IsZero() ||
		inspection.ProtectReason != "" {
		t.Fatalf("inspection = %+v, want unprotected", inspection)
	}
}

func TestProtectedDeleteErrorTextMatchesAcrossBackends(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newProtectStorage(t)
	fake.addUploadV1(t, testDir, "plan.html")
	sentinel, err := encodeProtectionSentinel(when, "README demo link")
	if err != nil {
		t.Fatal(err)
	}
	fake.addObject(testDir+"/"+ProtectedFilename, sentinel, when)

	s3Client := newProtectTestClient(t, fake.server.URL, "")
	_, s3Err := s3Client.DeleteUpload(context.Background(), testDir)

	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(
		&HTTPOperations{Client: newProtectTestClient(t, fake.server.URL, "")},
		httpapi.Options{Token: token},
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	remote, err := New(context.Background(), &Config{
		Backend: BackendAirplan, APIURL: server.URL, APIToken: token,
		Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, httpErr := remote.DeleteUpload(context.Background(), testDir)

	// The advisory reason survives the REST round trip, so the actionable
	// delete error reads identically under either backend (SPEC.md §11).
	var protectedErr *UploadProtectedError
	if !errors.As(httpErr, &protectedErr) ||
		protectedErr.Reason != "README demo link" {
		t.Fatalf("HTTP error = %v, want protected error with reason", httpErr)
	}
	if s3Err == nil || httpErr.Error() != s3Err.Error() {
		t.Fatalf("error text differs across backends:\ns3   = %q\nhttp = %q",
			s3Err, httpErr)
	}
}

// protectStorage is a minimal in-memory S3 fake covering list, get, put,
// batch delete, and single delete, recording operation order.
type protectStorage struct {
	server     *httptest.Server
	mu         sync.Mutex
	objects    map[string][]byte
	modified   map[string]time.Time
	getFail    map[string]int
	operations []string
	deletes    int
}

func newProtectStorage(t *testing.T) *protectStorage {
	t.Helper()
	fake := &protectStorage{
		objects:  make(map[string][]byte),
		modified: make(map[string]time.Time),
		getFail:  make(map[string]int),
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *protectStorage) addUploadV1(t *testing.T, dir, page string) {
	t.Helper()
	body, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC),
		Format:    "html", Page: page, Title: "Plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	f.addObject(dir+"/"+MarkerFilename, body, when)
	f.addObject(dir+"/"+page, []byte("0123456789"), when)
}

func (f *protectStorage) addObject(key string, body []byte, when time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = append([]byte(nil), body...)
	f.modified[key] = when.UTC()
}

func (f *protectStorage) object(key string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.objects[key]
	return append([]byte(nil), body...), ok
}

func (f *protectStorage) setGetFailure(key string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getFail[key] = status
}

func (f *protectStorage) record(operation string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.operations = append(f.operations, operation)
}

func (f *protectStorage) operationOrder() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.operations...)
}

func (f *protectStorage) deleteCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deletes
}

var deletedKeyPattern = regexp.MustCompile(`<Key>([^<]+)</Key>`)

func (f *protectStorage) handle(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/plans/")
	switch {
	case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
		f.record("list")
		f.handleList(w, r)
	case r.Method == http.MethodGet:
		f.record("get:" + key)
		f.mu.Lock()
		status := f.getFail[key]
		body, ok := f.objects[key]
		f.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w,
				`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
			return
		}
		_, _ = w.Write(body)
	case r.Method == http.MethodPut:
		f.record("put:" + key)
		body, _ := io.ReadAll(r.Body)
		f.addObject(key, body, time.Now())
	case r.Method == http.MethodPost:
		f.record("post-delete")
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.deletes++
		for _, match := range deletedKeyPattern.FindAllStringSubmatch(
			string(body), -1,
		) {
			delete(f.objects, match[1])
			delete(f.modified, match[1])
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintln(w, `<?xml version="1.0"?><DeleteResult></DeleteResult>`)
	case r.Method == http.MethodDelete:
		f.record("delete:" + key)
		f.mu.Lock()
		delete(f.objects, key)
		delete(f.modified, key)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *protectStorage) handleList(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	f.mu.Lock()
	keys := make([]string, 0, len(f.objects))
	for key := range f.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	type listed struct {
		key      string
		size     int
		modified time.Time
	}
	items := make([]listed, 0, len(keys))
	for _, key := range keys {
		items = append(items, listed{key, len(f.objects[key]), f.modified[key]})
	}
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintln(w, `<?xml version="1.0"?><ListBucketResult>`+
		`<IsTruncated>false</IsTruncated>`)
	for _, item := range items {
		fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size>"+
			"<LastModified>%s</LastModified></Contents>",
			item.key, item.size, item.modified.Format(time.RFC3339))
	}
	fmt.Fprintln(w, `</ListBucketResult>`)
}

func newProtectTestClient(
	t *testing.T, endpoint, manifest string,
) *Client {
	t.Helper()
	disable := manifest == ""
	client, err := New(context.Background(), &Config{
		Endpoint: endpoint, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		ManifestPath: manifest, DisableManifest: disable, Profile: "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
