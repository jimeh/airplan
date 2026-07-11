package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jimeh/airplan/airplan"
)

func TestPurgeCommandFilters(t *testing.T) {
	now := time.Now().UTC()
	records := []airplan.ManifestRecord{
		uploadRecord(deleteDirA, "alpha", "work", now.Add(-60*24*time.Hour)),
		uploadRecord(deleteDirB, "beta", "home", now.Add(-45*24*time.Hour)),
		uploadRecord(deleteDirC, "alpha-new", "work", now.Add(-time.Hour)),
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "older-than",
			args: []string{"purge", "--older-than", "30d", "--dry-run"},
			want: []string{"alpha.html", "beta.html"},
		},
		{
			name: "slug",
			args: []string{"purge", "--slug", "alpha*", "--dry-run"},
			want: []string{"alpha.html", "alpha-new.html"},
		},
		{
			name: "profile",
			// Unified semantics: --profile selects the connection
			// profile too, so it must exist in the config file.
			args: []string{
				"purge", "--profile", "home", "--dry-run",
				"--config", writeProfilesConfig(t),
			},
			want: []string{"beta.html"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateEnv(t)
			writeDefaultManifest(t, records)

			stdout, stderr, err := executeCommand(t, "", "", tt.args...)
			if err != nil {
				t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			for _, want := range tt.want {
				if !strings.Contains(stderr, want) {
					t.Fatalf("stderr = %q, want %q", stderr, want)
				}
			}
			for _, rec := range records {
				name := filepath.Base(rec.Key)
				if contains(tt.want, name) {
					continue
				}
				if strings.Contains(stderr, name) {
					t.Fatalf("stderr = %q, did not want %q", stderr, name)
				}
			}
		})
	}
}

func TestPurgeRequiresFilterOrAll(t *testing.T) {
	isolateEnv(t)
	writeDefaultManifest(t, nil)

	stdout, stderr, err := executeCommand(t, "", "", "purge")
	if err == nil {
		t.Fatal("Execute error = nil, want error")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty before Execute handles error", stderr)
	}
	if !strings.Contains(err.Error(), "requires at least one filter") {
		t.Fatalf("error = %v, want filter requirement", err)
	}
}

func TestPurgeRemoteRequiresFilterBeyondProfile(t *testing.T) {
	tests := [][]string{
		{"purge", "--remote"},
		{"purge", "--remote", "--profile", "work"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			isolateEnv(t)

			stdout, stderr, err := executeCommand(t, "", "", args...)
			if err == nil {
				t.Fatal("Execute error = nil, want error")
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty before Execute handles error",
					stderr)
			}
			if !strings.Contains(err.Error(), "requires at least one filter") {
				t.Fatalf("error = %v, want filter requirement", err)
			}
		})
	}
}

func TestPurgeRemoteOlderThanDeletesOnlyOldUploads(t *testing.T) {
	isolateEnv(t)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour).Truncate(time.Second)
	newer := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	oldKey := deleteDirA + "/old.html"
	newKey := deleteDirB + "/new.html"
	fake := newFakeRemoteS3(t, []remoteFakeObject{
		{
			key:  deleteDirA + "/" + airplan.MarkerFilename,
			size: 100, lastModified: old,
		},
		{key: oldKey, size: 10, lastModified: old},
		{key: deleteDirA + "/old.md", size: 5, lastModified: old},
		{
			key:  deleteDirB + "/" + airplan.MarkerFilename,
			size: 100, lastModified: newer,
		},
		{key: newKey, size: 10, lastModified: newer},
	}, nil, nil)

	stdout, stderr, err := executeCommand(t, "", "",
		"purge", "--remote", "--older-than", "30d", "--yes",
		"--config", writeCLIConfig(t, fake.server.URL))
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "purged 1 uploads (0 failed)") {
		t.Fatalf("stderr = %q, want purge summary", stderr)
	}
	if fake.deleteCalls() != 1 {
		t.Fatalf("delete calls = %d, want 1", fake.deleteCalls())
	}

	records, warnings, err := airplan.ReadManifest("")
	if err != nil || len(warnings) != 0 {
		t.Fatalf("ReadManifest: %v %v", err, warnings)
	}
	if len(records) != 1 || records[0].Type != "delete" ||
		records[0].Key != oldKey {
		t.Fatalf("tombstone = %+v, want old upload only", records)
	}
}

func TestPurgeRemoteDeletesIncompleteAndSkipsInvalid(t *testing.T) {
	isolateEnv(t)
	when := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	incompleteMarkerKey := deleteDirA + "/" + airplan.MarkerFilename
	invalidMarkerKey := deleteDirB + "/" + airplan.MarkerFilename
	fake := newFakeRemoteS3(t, []remoteFakeObject{
		{key: incompleteMarkerKey, size: 100, lastModified: when},
		{key: invalidMarkerKey, size: 10, lastModified: when},
	}, nil, nil)
	body, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: airplan.MarkerVersion,
		Directory: deleteDirA, CreatedAt: when, Format: "md",
		Page: "missing.html", Source: "missing.md", Title: "Incomplete",
	})
	if err != nil {
		t.Fatal(err)
	}
	fake.setMarker(incompleteMarkerKey, body)
	fake.setMarker(invalidMarkerKey, []byte(`{"schema":`))

	stdout, stderr, err := executeCommand(t, "", "",
		"purge", "--remote", "--slug", "missing", "--yes",
		"--config", writeCLIConfig(t, fake.server.URL))
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" ||
		!strings.Contains(stderr, "skipped 1 invalid remote marker") ||
		!strings.Contains(stderr, "purged 1 uploads (0 failed)") ||
		fake.markerDeleteCalls() != 1 {
		t.Fatalf("stdout = %q, stderr = %q, marker deletes = %d",
			stdout, stderr, fake.markerDeleteCalls())
	}
}

func TestPurgeRemoteWarnsForFallbackPublicURL(t *testing.T) {
	isolateEnv(t)
	when := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	pageKey := deleteDirA + "/plan.html"
	fake := newFakeRemoteS3(t, []remoteFakeObject{
		{
			key:  deleteDirA + "/" + airplan.MarkerFilename,
			size: 100, lastModified: when,
		},
		{key: pageKey, size: 10, lastModified: when},
	}, nil, nil)
	config := filepath.Join(t.TempDir(), "config.toml")
	data := "endpoint = \"" + fake.server.URL + "\"\n" +
		"bucket = \"plans\"\n" +
		"timeout = \"0\"\n"
	if err := os.WriteFile(config, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeCommand(t, "", "",
		"purge", "--remote", "--all", "--dry-run", "--config", config)
	if err != nil {
		t.Fatal(err)
	}
	wantURL := fake.server.URL + "/plans/" + pageKey
	if stdout != "" || !strings.Contains(stderr, "public_base_url") ||
		!strings.Contains(stderr, wantURL) || fake.deleteCalls() != 0 {
		t.Fatalf("stdout = %q, stderr = %q, deletes = %d",
			stdout, stderr, fake.deleteCalls())
	}
}

func TestPurgeDryRunDeletesNothing(t *testing.T) {
	isolateEnv(t)
	writeDefaultManifest(t, []airplan.ManifestRecord{
		uploadRecord(deleteDirA, "alpha", "work",
			time.Now().Add(-60*24*time.Hour)),
	})

	stdout, stderr, err := executeCommand(t, "", "", "purge", "--all",
		"--dry-run")
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "alpha.html") {
		t.Fatalf("stderr = %q, want dry-run candidate", stderr)
	}

	records, warnings, err := airplan.ReadManifest("")
	if err != nil || len(warnings) != 0 {
		t.Fatalf("ReadManifest: %v %v", err, warnings)
	}
	if got := airplan.ActiveUploads(records); len(got) != 1 {
		t.Fatalf("active uploads = %d, want 1", len(got))
	}
}

func TestPurgeYesDeletesAndTombstones(t *testing.T) {
	isolateEnv(t)
	writeDefaultManifest(t, []airplan.ManifestRecord{
		uploadRecord(deleteDirA, "alpha", "work",
			time.Now().Add(-60*24*time.Hour)),
		uploadRecord(deleteDirB, "beta", "home",
			time.Now().Add(-45*24*time.Hour)),
	})
	fake := newFakeDeleteS3(t, map[string][]string{
		deleteDirA + "/": {deleteDirA + "/alpha.html", deleteDirA + "/alpha.md"},
		deleteDirB + "/": {deleteDirB + "/beta.html"},
	}, nil)

	stdout, stderr, err := executeCommand(t, "", "", "purge", "--all",
		"--yes", "--config", writeCLIConfig(t, fake.server.URL))
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "purged 2 uploads (0 failed)") {
		t.Fatalf("stderr = %q, want purge summary", stderr)
	}
	if fake.deleteCalls() != 2 {
		t.Fatalf("delete calls = %d, want 2", fake.deleteCalls())
	}

	records, warnings, err := airplan.ReadManifest("")
	if err != nil || len(warnings) != 0 {
		t.Fatalf("ReadManifest: %v %v", err, warnings)
	}
	if got := airplan.ActiveUploads(records); len(got) != 0 {
		t.Fatalf("active uploads = %+v, want none", got)
	}
}

func TestPurgeConfirmationAbort(t *testing.T) {
	isolateEnv(t)
	writeDefaultManifest(t, []airplan.ManifestRecord{
		uploadRecord(deleteDirA, "alpha", "work",
			time.Now().Add(-60*24*time.Hour)),
	})
	fake := newFakeDeleteS3(t, map[string][]string{
		deleteDirA + "/": {deleteDirA + "/alpha.html"},
	}, nil)

	stdout, stderr, err := executeCommand(t, "n\n", "", "purge", "--all",
		"--config", writeCLIConfig(t, fake.server.URL))
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "Delete 1 uploads? [y/N] ") ||
		!strings.Contains(stderr, "aborted") {
		t.Fatalf("stderr = %q, want prompt and abort", stderr)
	}
	if fake.deleteCalls() != 0 {
		t.Fatalf("delete calls = %d, want 0", fake.deleteCalls())
	}

	records, _, err := airplan.ReadManifest("")
	if err != nil {
		t.Fatal(err)
	}
	if got := airplan.ActiveUploads(records); len(got) != 1 {
		t.Fatalf("active uploads = %d, want 1", len(got))
	}
}

func TestPurgeConfirmationEOFErrors(t *testing.T) {
	isolateEnv(t)
	writeDefaultManifest(t, []airplan.ManifestRecord{
		uploadRecord(deleteDirA, "alpha", "work", time.Now()),
	})
	fake := newFakeDeleteS3(t, map[string][]string{
		deleteDirA + "/": {deleteDirA + "/alpha.html"},
	}, nil)

	stdout, stderr, err := executeCommand(t, "", "", "purge", "--all",
		"--config", writeCLIConfig(t, fake.server.URL))
	if err == nil || !strings.Contains(err.Error(), "rerun with --yes") {
		t.Fatalf("error = %v, want --yes guidance", err)
	}
	if stdout != "" || !strings.Contains(stderr, "Delete 1 uploads?") ||
		fake.deleteCalls() != 0 {
		t.Fatalf("stdout = %q, stderr = %q, deletes = %d",
			stdout, stderr, fake.deleteCalls())
	}
}

func TestPurgePartialFailureLeavesFailedUploadActive(t *testing.T) {
	isolateEnv(t)
	writeDefaultManifest(t, []airplan.ManifestRecord{
		uploadRecord(deleteDirA, "alpha", "work",
			time.Now().Add(-60*24*time.Hour)),
		uploadRecord(deleteDirB, "beta", "home",
			time.Now().Add(-45*24*time.Hour)),
		uploadRecord(deleteDirC, "gamma", "work",
			time.Now().Add(-40*24*time.Hour)),
	})
	fake := newFakeDeleteS3(t, map[string][]string{
		deleteDirA + "/": {deleteDirA + "/alpha.html"},
		deleteDirB + "/": {deleteDirB + "/beta.html"},
		deleteDirC + "/": {deleteDirC + "/gamma.html"},
	}, map[string]bool{deleteDirB: true})

	stdout, stderr, err := executeCommand(t, "", "", "purge", "--all",
		"--yes", "--config", writeCLIConfig(t, fake.server.URL))
	if err == nil {
		t.Fatal("Execute error = nil, want partial failure")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "airplan: error: delete") ||
		!strings.Contains(stderr, "purged 2 uploads (1 failed)") {
		t.Fatalf("stderr = %q, want error line and summary", stderr)
	}

	records, warnings, err := airplan.ReadManifest("")
	if err != nil || len(warnings) != 0 {
		t.Fatalf("ReadManifest: %v %v", err, warnings)
	}
	active := airplan.ActiveUploads(records)
	if len(active) != 1 || active[0].Key != deleteDirB+"/beta.html" {
		t.Fatalf("active uploads = %+v, want failed beta only", active)
	}
}

func uploadRecord(
	dir string,
	slug string,
	profile string,
	when time.Time,
) airplan.ManifestRecord {
	key := dir + "/" + slug + ".html"
	return airplan.ManifestRecord{
		Type:          "upload",
		Time:          when.UTC().Truncate(time.Second),
		Key:           key,
		URL:           "https://plans.example.com/" + key,
		Bucket:        "plans",
		Profile:       profile,
		Title:         slug + " title",
		Bytes:         10,
		MarkerVersion: airplan.MarkerVersion,
	}
}

func writeDefaultManifest(t *testing.T, records []airplan.ManifestRecord) {
	t.Helper()

	path, err := airplan.DefaultManifestPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	var lines []string
	for _, rec := range records {
		data, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(data))
	}
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func contains(values []string, needle string) bool {
	for _, v := range values {
		if v == needle {
			return true
		}
	}
	return false
}

// writeProfilesConfig writes a config defining the profiles the
// filter fixtures reference, so the unified --profile flag resolves.
func writeProfilesConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := "endpoint = \"https://example.com\"\n" +
		"bucket = \"plans\"\n\n" +
		"[profiles.home]\n" +
		"[profiles.work]\n"
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPurgeAllStillAppliesFilters: --all satisfies the filter
// requirement but must not bypass filters given alongside it —
// "purge --all --slug alpha*" deletes alpha uploads only.
func TestPurgeAllStillAppliesFilters(t *testing.T) {
	now := time.Now().UTC()
	records := []airplan.ManifestRecord{
		uploadRecord(deleteDirA, "alpha", "work", now.Add(-time.Hour)),
		uploadRecord(deleteDirB, "beta", "home", now.Add(-time.Hour)),
	}
	isolateEnv(t)
	writeDefaultManifest(t, records)

	stdout, stderr, err := executeCommand(t, "", "",
		"purge", "--all", "--slug", "alpha*", "--dry-run")
	if err != nil {
		t.Fatalf("Execute: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "alpha.html") {
		t.Errorf("stderr missing alpha candidate: %q", stderr)
	}
	if strings.Contains(stderr, "beta.html") {
		t.Errorf("--all bypassed --slug filter: %q", stderr)
	}
}

func TestPurgeSkipsOtherBucketsAndPrefixes(t *testing.T) {
	isolateEnv(t)
	now := time.Now().UTC()
	current := uploadRecord(deleteDirA, "current", "", now)
	current.Key = "team/current/" + current.Key
	current.URL = "https://plans.example.com/" + current.Key
	otherPrefix := uploadRecord(deleteDirB, "other-prefix", "", now)
	otherPrefix.Key = "team/old/" + otherPrefix.Key
	otherPrefix.URL = "https://plans.example.com/" + otherPrefix.Key
	otherBucket := uploadRecord(deleteDirC, "other-bucket", "", now)
	otherBucket.Key = "team/current/" + otherBucket.Key
	otherBucket.URL = "https://plans.example.com/" + otherBucket.Key
	otherBucket.Bucket = "archive"
	writeDefaultManifest(t, []airplan.ManifestRecord{
		current, otherPrefix, otherBucket,
	})

	config := filepath.Join(t.TempDir(), "config.toml")
	data := "endpoint = \"https://example.com\"\n" +
		"bucket = \"plans\"\n" +
		"key_prefix = \"team/current\"\n"
	if err := os.WriteFile(config, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeCommand(t, "", "",
		"purge", "--all", "--dry-run", "--config", config)
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" || !strings.Contains(stderr, "current.html") ||
		strings.Contains(stderr, "other-prefix.html") ||
		strings.Contains(stderr, "other-bucket.html") ||
		!strings.Contains(stderr, "skipped 1 upload(s) recorded for other buckets") ||
		!strings.Contains(stderr, "skipped 1 upload(s) recorded for other key prefixes") {
		t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
	}
}
