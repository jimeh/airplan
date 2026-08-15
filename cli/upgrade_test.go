package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jimeh/airplan/airplan"
)

func TestUpgradeCheckJSONKeepsStdoutMachineReadable(t *testing.T) {
	isolateEnv(t)
	dir := strings.Repeat("z", 26)
	page := []byte("old page")
	source := []byte("# Plan\n")
	marker, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Kind:      airplan.UploadKindDocument, Slug: "plan", Format: "md",
		Objects: []airplan.MarkerObject{
			{Name: "plan.html", Role: airplan.MarkerRolePage, Bytes: int64(len(page)), ContentType: "text/html; charset=utf-8"},
			{Name: "plan.md", Role: airplan.MarkerRoleSource, Bytes: int64(len(source)), ContentType: "text/markdown; charset=utf-8"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeRemoteS3(t, nil, nil, nil)
	fake.setMarker(dir+"/"+airplan.MarkerFilename, marker)
	fake.setMarker(dir+"/plan.html", page)
	fake.setMarker(dir+"/plan.md", source)
	stdout, stderr, err := executeCommand(t, "", "", "upgrade", "--check", "--json",
		"--config", writeCLIConfig(t, fake.server.URL), dir+"/plan.html")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	var plan airplan.UpgradeDocumentPlan
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("stdout = %q: %v", stdout, err)
	}
	if plan.State != airplan.UpgradeStateUpgradeable ||
		plan.CurrentMarkerVersion != 3 || plan.URL == "" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestUpgradeApplyPrintsOnlyStableURL(t *testing.T) {
	isolateEnv(t)
	fake := newFakeRemoteS3(t, nil, nil, nil)
	dir := strings.Repeat("a", 26)
	seedCLIUpgradeDocument(t, fake, "", dir)
	stdout, stderr, err := executeCommand(t, "", "", "upgrade",
		"--config", writeCLIConfig(t, fake.server.URL), dir+"/plan.html")
	if err != nil {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	want := "https://plans.example.com/" + dir + "/plan.html\n"
	if stdout != want || stderr != "" || fake.putCalls() != 2 {
		t.Fatalf("stdout = %q, stderr = %q, puts = %d", stdout, stderr, fake.putCalls())
	}
}

func TestUpdatePrintsOnlyNewRevisionURLAndJSONDescribesChain(t *testing.T) {
	isolateEnv(t)
	fake := newFakeRemoteS3(t, nil, nil, nil)
	dir := strings.Repeat("u", 26)
	seedCLICurrentDocument(t, fake, dir)
	config := writeCLIConfig(t, fake.server.URL)
	target := "https://plans.example.com/" + dir + "/plan.html"

	stdout, stderr, err := executeCommand(t, "# Plan\n\nRevised.\n", "",
		"update", "--json", "--config", config, target)
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	var result airplan.UpdateDocumentResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout = %q: %v", stdout, err)
	}
	if result.URL == "" || result.URL == target || result.Revision != 2 ||
		result.LatestRevision != 2 || result.PreviousURL != target ||
		result.DiffURL == "" || result.Unchanged {
		t.Fatalf("result = %+v", result)
	}
}

func TestUpdateAcceptsExplicitManifestWithDirectS3Backend(t *testing.T) {
	isolateEnv(t)
	fake := newFakeRemoteS3(t, nil, nil, nil)
	dir := strings.Repeat("m", 26)
	seedCLICurrentDocument(t, fake, dir)
	manifest := filepath.Join(t.TempDir(), "custom.jsonl")
	target := "https://plans.example.com/" + dir + "/plan.html"

	stdout, stderr, err := executeCommand(t, "# Plan\n\nRevised.\n", "",
		"--manifest", manifest, "update", "--config", writeCLIConfig(t, fake.server.URL),
		target)
	if err != nil || stderr != "" || stdout == "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	records, warnings, err := airplan.ReadManifest(manifest)
	if err != nil || len(warnings) != 0 || len(records) == 0 {
		t.Fatalf("manifest records = %+v, warnings = %v, error = %v", records, warnings, err)
	}
}

func TestBulkUpgradeJSONReportsFailureWithNonzeroStatus(t *testing.T) {
	isolateEnv(t)
	fake := newFakeRemoteS3(t, nil, nil, nil)
	dir := strings.Repeat("b", 26)
	record := seedCLIUpgradeDocument(t, fake, "", dir)
	writeDefaultManifest(t, []airplan.ManifestRecord{record})
	fake.setPutFailure(true)
	stdout, stderr, err := executeCommand(t, "", "", "upgrade", "--all", "--yes",
		"--json", "--config", writeCLIConfig(t, fake.server.URL))
	if err == nil || !strings.Contains(err.Error(), "one or more upgrades failed") {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	var result airplan.BulkUpgradeResult
	if decodeErr := json.Unmarshal([]byte(stdout), &result); decodeErr != nil {
		t.Fatalf("stdout = %q: %v", stdout, decodeErr)
	}
	if result.Failed != 1 || len(result.Items) != 1 || result.Items[0].Error == "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestBulkUpgradeAllProfilesRoutesAndClassifiesManifestDrift(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AIRPLAN_PROFILE", "removed-profile")
	work := newFakeRemoteS3(t, nil, nil, nil)
	home := newFakeRemoteS3(t, nil, nil, nil)
	workDir := strings.Repeat("w", 26)
	homeDir := strings.Repeat("h", 26)
	records := []airplan.ManifestRecord{
		seedCLIUpgradeDocument(t, work, "work", workDir),
		seedCLIUpgradeDocument(t, home, "home", homeDir),
		{
			Type: "upload", Time: time.Now().UTC(), CreatedAt: time.Now().UTC(),
			Key:       "ghost/" + strings.Repeat("g", 26) + "/plan.html",
			MarkerKey: "ghost/" + strings.Repeat("g", 26) + "/" + airplan.MarkerFilename,
			URL:       "https://plans.example.com/ghost/plan.html", Bucket: "plans",
			Profile: "ghost", Format: "md", Kind: string(airplan.UploadKindDocument),
			Slug: "plan", Bytes: 3, MarkerVersion: 3,
		},
		{
			Type: "upload", Time: time.Now().UTC(), CreatedAt: time.Now().UTC(),
			Key:       "drift/" + strings.Repeat("d", 26) + "/plan.html",
			MarkerKey: "drift/" + strings.Repeat("d", 26) + "/" + airplan.MarkerFilename,
			URL:       "https://plans.example.com/drift/plan.html", Bucket: "plans",
			Profile: "work", Format: "md", Kind: string(airplan.UploadKindDocument),
			Slug: "plan", Bytes: 3, MarkerVersion: 3,
		},
	}
	writeDefaultManifest(t, records)
	config := writeUpgradeProfilesConfig(t, work.server.URL, home.server.URL)
	stdout, stderr, err := executeCommand(t, "", "", "upgrade", "--all",
		"--all-profiles", "--dry-run", "--json", "--config", config)
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	var plan airplan.BulkUpgradePlan
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("stdout = %q: %v", stdout, err)
	}
	if len(plan.Items) != 4 || plan.Counts[airplan.UpgradeStateUpgradeable] != 2 ||
		plan.Counts[airplan.UpgradeStateInvalid] != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	profiles := map[string]airplan.UpgradeState{}
	for _, item := range plan.Items {
		profiles[item.Profile+"\x00"+item.Target] = item.State
	}
	if profiles["work\x00work/"+workDir+"/"+airplan.MarkerFilename] != airplan.UpgradeStateUpgradeable ||
		profiles["home\x00home/"+homeDir+"/"+airplan.MarkerFilename] != airplan.UpgradeStateUpgradeable {
		t.Fatalf("profile routing = %+v", profiles)
	}
	if work.putCalls() != 0 || home.putCalls() != 0 {
		t.Fatalf("dry run writes = work %d, home %d", work.putCalls(), home.putCalls())
	}
}

func TestBulkUpgradeAllProfilesRejectsMixedBackendsBeforeStorage(t *testing.T) {
	isolateEnv(t)
	fake := newFakeRemoteS3(t, nil, nil, nil)
	dir := strings.Repeat("x", 26)
	writeDefaultManifest(t, []airplan.ManifestRecord{
		seedCLIUpgradeDocument(t, fake, "work", dir),
	})
	path := filepath.Join(t.TempDir(), "config.toml")
	data := fmt.Sprintf(`
access_key_id = "test"
secret_access_key = "test"
public_base_url = "https://plans.example.com"

[profiles.work]
endpoint = %q
bucket = "plans"
key_prefix = "work"

[profiles.hosted]
backend = "airplan"
api_url = "https://airplan.example.com"
api_token = "01234567890123456789012345678901"
`, fake.server.URL)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := executeCommand(t, "", "", "upgrade", "--all",
		"--all-profiles", "--dry-run", "--config", path)
	if err == nil || !strings.Contains(err.Error(), "requires s3 profiles") ||
		stdout != "" || fake.putCalls() != 0 || fake.listCalls() != 0 {
		t.Fatalf("stdout = %q, puts = %d, lists = %d, error = %v",
			stdout, fake.putCalls(), fake.listCalls(), err)
	}
}

func TestBulkUpgradeApplyJSONAlwaysUsesResultSchema(t *testing.T) {
	for _, test := range []struct {
		name        string
		current     bool
		allProfiles bool
	}{
		{"empty", false, false},
		{"current only", true, false},
		{"root only all profiles", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			isolateEnv(t)
			fake := newFakeRemoteS3(t, nil, nil, nil)
			if test.current {
				dir := strings.Repeat("q", 26)
				writeDefaultManifest(t, []airplan.ManifestRecord{
					seedCLICurrentDocument(t, fake, dir),
				})
			}
			args := []string{"upgrade", "--all", "--yes", "--json"}
			if test.allProfiles {
				t.Setenv("AIRPLAN_PROFILE", "stale")
				args = append(args, "--all-profiles")
			}
			args = append(args, "--config", writeCLIConfig(t, fake.server.URL))
			stdout, stderr, err := executeCommand(t, "", "", args...)
			if err != nil || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
			}
			var result airplan.BulkUpgradeResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("stdout is not BulkUpgradeResult: %q: %v", stdout, err)
			}
			if result.Items == nil || len(result.Items) != 0 ||
				result.Upgraded != 0 || result.Failed != 0 {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestBulkUpgradeBareEnterAbortsCleanly(t *testing.T) {
	isolateEnv(t)
	fake := newFakeRemoteS3(t, nil, nil, nil)
	dir := strings.Repeat("e", 26)
	writeDefaultManifest(t, []airplan.ManifestRecord{
		seedCLIUpgradeDocument(t, fake, "", dir),
	})
	_, stderr, err := executeCommand(t, "\n", "", "upgrade", "--all",
		"--config", writeCLIConfig(t, fake.server.URL))
	if err != nil || !strings.Contains(stderr, "aborted") || fake.putCalls() != 0 {
		t.Fatalf("stderr = %q, puts = %d, error = %v", stderr, fake.putCalls(), err)
	}
}

func TestBulkUpgradeJSONDeclineEmitsEmptyResult(t *testing.T) {
	isolateEnv(t)
	fake := newFakeRemoteS3(t, nil, nil, nil)
	dir := strings.Repeat("j", 26)
	writeDefaultManifest(t, []airplan.ManifestRecord{
		seedCLIUpgradeDocument(t, fake, "", dir),
	})
	stdout, stderr, err := executeCommand(t, "\n", "", "upgrade", "--all",
		"--json", "--config", writeCLIConfig(t, fake.server.URL))
	if err != nil || !strings.Contains(stderr, "aborted") || fake.putCalls() != 0 {
		t.Fatalf("stdout = %q, stderr = %q, puts = %d, error = %v",
			stdout, stderr, fake.putCalls(), err)
	}
	var result airplan.BulkUpgradeResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not BulkUpgradeResult: %q: %v", stdout, err)
	}
	if result.Items == nil || len(result.Items) != 0 ||
		result.Upgraded != 0 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestRemoteBulkUpgradeFailureDoesNotDereferenceNilResult(t *testing.T) {
	isolateEnv(t)
	server := newBulkUpgradeBackend(t, false)
	stdout, stderr, err := executeCommand(t, "", "", "upgrade", "--all", "--yes",
		"--config", writeAirplanUpgradeConfig(t, server.URL, "0"))
	if err == nil || stdout != "" || !strings.Contains(stderr, "STATE\tCURRENT") ||
		strings.Contains(stderr, "upgraded ") {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
}

func TestBulkUpgradeConfirmationGetsFreshExecutionTimeout(t *testing.T) {
	isolateEnv(t)
	server := newBulkUpgradeBackend(t, true)
	cmd := newRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(&delayedReader{delay: 80 * time.Millisecond, body: []byte("y\n")})
	cmd.SetArgs([]string{
		"upgrade", "--all", "--config",
		writeAirplanUpgradeConfig(t, server.URL, "30ms"),
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout.String(), stderr.String(), err)
	}
}

func TestUpgradeFlagBoundariesFailBeforeConfiguration(t *testing.T) {
	isolateEnv(t)
	for _, test := range []struct {
		name string
		want string
		args []string
	}{
		{"all profiles rejects profile", "cannot be combined with --profile", []string{"upgrade", "--all", "--all-profiles", "--profile", "work"}},
		{"all profiles requires all", "--all-profiles requires --all", []string{"upgrade", "--all-profiles"}},
		{"bulk rejects target", "--all does not accept a target", []string{"upgrade", "--all", "target"}},
		{"single rejects dry run", "require --all", []string{"upgrade", "--dry-run", "target"}},
		{"bulk rejects force", "apply only to one target", []string{"upgrade", "--all", "--force"}},
		{"concurrency bounded", "between 1 and 32", []string{"upgrade", "--all", "--concurrency", "33"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stdout, _, err := executeCommand(t, "", "", test.args...)
			if err == nil || stdout != "" || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("stdout = %q, err = %v", stdout, err)
			}
		})
	}
}

func seedCLIUpgradeDocument(
	t *testing.T, fake *fakeRemoteS3, prefix, dir string,
) airplan.ManifestRecord {
	t.Helper()
	page := []byte("old")
	source := []byte("# Plan\n")
	marker, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Kind:      airplan.UploadKindDocument, Slug: "plan", Format: "md",
		Objects: []airplan.MarkerObject{
			{Name: "plan.html", Role: airplan.MarkerRolePage, Bytes: int64(len(page)), ContentType: "text/html; charset=utf-8"},
			{Name: "plan.md", Role: airplan.MarkerRoleSource, Bytes: int64(len(source)), ContentType: "text/markdown; charset=utf-8"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := ""
	if prefix != "" {
		base = prefix + "/"
	}
	fake.setMarker(base+dir+"/"+airplan.MarkerFilename, marker)
	fake.setMarker(base+dir+"/plan.html", page)
	fake.setMarker(base+dir+"/plan.md", source)
	profile := ""
	if prefix == "work" || prefix == "home" {
		profile = prefix
	}
	return airplan.ManifestRecord{
		Type: "upload", Time: time.Now().UTC(), CreatedAt: time.Now().UTC(),
		Key: base + dir + "/plan.html", SourceKey: base + dir + "/plan.md",
		MarkerKey: base + dir + "/" + airplan.MarkerFilename,
		URL:       "https://plans.example.com/" + base + dir + "/plan.html",
		Bucket:    "plans", Profile: profile, Format: "md",
		Kind: string(airplan.UploadKindDocument), Slug: "plan",
		Bytes: int64(len(page)), MarkerVersion: 3,
	}
}

func seedCLICurrentDocument(
	t *testing.T, fake *fakeRemoteS3, dir string,
) airplan.ManifestRecord {
	t.Helper()
	page := []byte("current")
	source := []byte("# Plan\n")
	marker, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: airplan.MarkerVersion,
		Directory: dir, CreatedAt: time.Now().UTC().Truncate(time.Second),
		Kind: airplan.UploadKindDocument, Slug: "plan", Format: "md",
		Producer: airplan.Producer{Name: "airplan", Version: buildVersion()},
		Render: &airplan.RenderRecipe{
			Generation: airplan.RendererGeneration,
			Template:   airplan.RenderTemplate{Kind: "builtin"},
			MermaidURL: airplan.DefaultMermaidURL,
		},
		Objects: []airplan.MarkerObject{
			{
				Name: "plan.html", Role: airplan.MarkerRolePage, Bytes: int64(len(page)),
				ContentType: "text/html; charset=utf-8", SHA256: sha256Hex(page),
			},
			{
				Name: "plan.md", Role: airplan.MarkerRoleSource, Bytes: int64(len(source)),
				ContentType: "text/markdown; charset=utf-8",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fake.setMarker(dir+"/"+airplan.MarkerFilename, marker)
	fake.setMarker(dir+"/plan.html", page)
	fake.setMarker(dir+"/plan.md", source)
	return airplan.ManifestRecord{
		Type: "upload", Time: time.Now().UTC(), CreatedAt: time.Now().UTC(),
		Key: dir + "/plan.html", SourceKey: dir + "/plan.md",
		MarkerKey: dir + "/" + airplan.MarkerFilename,
		URL:       "https://plans.example.com/" + dir + "/plan.html",
		Bucket:    "plans", Format: "md", Kind: string(airplan.UploadKindDocument),
		Slug: "plan", Bytes: int64(len(page)), MarkerVersion: airplan.MarkerVersion,
	}
}

func writeUpgradeProfilesConfig(t *testing.T, workEndpoint, homeEndpoint string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	data := fmt.Sprintf(`
access_key_id = "test"
secret_access_key = "test"
public_base_url = "https://plans.example.com"
timeout = "0"

[profiles.work]
endpoint = %q
bucket = "plans"
key_prefix = "work"

[profiles.home]
endpoint = %q
bucket = "plans"
key_prefix = "home"
`, workEndpoint, homeEndpoint)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newBulkUpgradeBackend(t *testing.T, succeed bool) *httptest.Server {
	t.Helper()
	dir := strings.Repeat("r", 26)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/upgrades/preview":
			_, _ = fmt.Fprintf(w, `{"items":[{"target":%q,"state":"upgradeable",`+
				`"target_marker_version":4,"target_producer_version":"0.8.0",`+
				`"target_renderer_generation":1,"marker_etag":"m",`+
				`"page_etag":"p","source_etag":"s"}],`+
				`"counts":{"upgradeable":1}}`, dir+"/plan.html")
		case "/api/v1/upgrades":
			if !succeed {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w,
					`{"type":"about:blank","title":"failure","status":500,`+
						`"code":"internal_error","detail":"boom"}`)
				return
			}
			_, _ = io.WriteString(w, `{"items":[],"upgraded":0,"failed":0}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeAirplanUpgradeConfig(t *testing.T, endpoint, timeout string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	data := fmt.Sprintf(`
backend = "airplan"
api_url = %q
api_token = "01234567890123456789012345678901"
timeout = %q
`, endpoint, timeout)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func sha256Hex(body []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(body))
}

type delayedReader struct {
	once  sync.Once
	delay time.Duration
	body  []byte
}

func (r *delayedReader) Read(p []byte) (int, error) {
	r.once.Do(func() { time.Sleep(r.delay) })
	if len(r.body) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.body)
	r.body = r.body[n:]
	return n, nil
}
