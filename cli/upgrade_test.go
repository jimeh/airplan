package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestUpgradeFlagBoundariesFailBeforeConfiguration(t *testing.T) {
	isolateEnv(t)
	for _, test := range []struct {
		name string
		want string
		args []string
	}{
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

func writeUpgradeProfilesConfig(t *testing.T, workEndpoint, homeEndpoint string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	data := fmt.Sprintf(`
default_profile = "work"
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
