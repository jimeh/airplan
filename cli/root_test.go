package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"testing"

	"github.com/jimeh/airplan/airplan"
)

func TestResolveVersion(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v0.1.2"}}
	for _, tt := range []struct {
		name    string
		stamped string
		info    *debug.BuildInfo
		ok      bool
		want    string
	}{
		{"ldflag wins", "0.2.0", info, true, "0.2.0"},
		{"module version", "dev", info, true, "0.1.2"},
		{"pseudo-version", "dev", &debug.BuildInfo{Main: debug.Module{
			Version: "v0.0.0-20260710011458-abcdef123456",
		}}, true, "0.0.0-20260710011458-abcdef123456"},
		{"dirty local build", "dev", &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: "true"},
			},
		}, true, "dev"},
		{"unavailable", "dev", nil, false, "dev"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.stamped, tt.info, tt.ok); got != tt.want {
				t.Fatalf("resolveVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandAliasesAndQoLShorthands(t *testing.T) {
	root := newRootCmd()
	list, _, err := root.Find([]string{"list"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Aliases) != 1 || list.Aliases[0] != "ls" {
		t.Fatalf("list aliases = %v, want [ls]", list.Aliases)
	}
	for _, tt := range []struct {
		command   string
		flag      string
		shorthand string
	}{
		{"list", "remote", "r"},
		{"purge", "remote", "r"},
		{"preview", "output", "o"},
		{"get", "output", "o"},
	} {
		cmd, _, findErr := root.Find([]string{tt.command})
		if findErr != nil {
			t.Fatal(findErr)
		}
		flag := cmd.Flags().Lookup(tt.flag)
		if flag == nil {
			t.Errorf("%s missing --%s flag", tt.command, tt.flag)
			continue
		}
		if flag.Shorthand != tt.shorthand {
			t.Errorf("%s --%s shorthand = %q, want %q",
				tt.command, tt.flag, flag.Shorthand, tt.shorthand)
		}
	}

	setListState(t)
	root = newRootCmd()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"ls", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "[]\n" {
		t.Fatalf("ls output = %q", stdout.String())
	}

	root = newRootCmd()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetArgs([]string{"list", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Aliases:", "list, ls", "-r, --remote"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("list help missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRootJSONOutputShape(t *testing.T) {
	t.Run("with source_url", func(t *testing.T) {
		fake := newFakeS3(t)
		stdout, stderr, err := executeRoot(t, fake,
			"# Launch\n\nBody\n",
			"-j",
			"-s", "launch",
			"-",
		)
		if err != nil {
			t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
		}

		assertOneJSONLine(t, stdout)
		if strings.Contains(stdout, "warning") {
			t.Fatalf("stdout contains warning: %q", stdout)
		}
		// stderr may carry warnings but must never carry the result;
		// the fallback-URL warning case is covered separately in
		// TestRootWarningsGoToStderr.
		_ = stderr

		var got rootJSON
		fields := decodeJSONLine(t, stdout, &got)
		assertJSONFieldOrder(t, stdout,
			[]string{
				"url", "key", "source_url", "bucket",
				"bytes", "content_type",
			},
		)
		if _, ok := fields["source_url"]; !ok {
			t.Fatal("source_url omitted for markdown input with source")
		}
		if got.SourceURL == "" {
			t.Fatal("source_url is empty")
		}
		assertJSONResult(t, fake, got)
	})

	t.Run("omits source_url under no-source", func(t *testing.T) {
		fake := newFakeS3(t)
		stdout, stderr, err := executeRoot(t, fake,
			"# Launch\n\nBody\n",
			"--no-source",
			"--json",
			"--slug", "launch",
			"-",
		)
		if err != nil {
			t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
		}

		assertOneJSONLine(t, stdout)

		var got rootJSON
		fields := decodeJSONLine(t, stdout, &got)
		assertJSONFieldOrder(t, stdout,
			[]string{"url", "key", "bucket", "bytes", "content_type"},
		)
		if _, ok := fields["source_url"]; ok {
			t.Fatalf("source_url present under --no-source: %s", stdout)
		}
		if got.SourceURL != "" {
			t.Fatalf("SourceURL = %q, want empty", got.SourceURL)
		}
		assertJSONResult(t, fake, got)
	})
}

func TestRootUploadsFileCollectionWithOrderedOutput(t *testing.T) {
	fake := newFakeS3(t)
	dir := t.TempDir()
	shot := filepath.Join(dir, "shot one.png")
	demo := filepath.Join(dir, "demo.webm")
	if err := os.WriteFile(shot, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(demo, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := executeRoot(t, fake, "", shot, demo)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 || !strings.HasSuffix(lines[0], "/shot%20one.png") ||
		!strings.HasSuffix(lines[1], "/demo.webm") ||
		!strings.HasSuffix(lines[2], "/index.html") {
		t.Fatalf("stdout = %q", stdout)
	}
	puts := fake.uploads()
	if len(puts) != 4 || !strings.HasSuffix(puts[0].path,
		"/"+airplan.CollectionMarkerFilename) ||
		!strings.HasSuffix(puts[3].path, "/index.html") {
		t.Fatalf("PUTs = %+v", puts)
	}

	fake = newFakeS3(t)
	stdout, _, err = executeRoot(t, fake, "", "--json", shot, demo)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		URL   string                       `json:"url"`
		Files []struct{ Name, URL string } `json:"files"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 2 || got.Files[0].Name != "shot one.png" ||
		!strings.HasSuffix(got.URL, "/index.html") {
		t.Fatalf("json = %s", stdout)
	}
}

func TestRootCollectionRejectsDocumentFlagsBeforeUpload(t *testing.T) {
	fake := newFakeS3(t)
	path := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(path, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := executeRoot(t, fake, "", "--slug", "shot", path)
	if err == nil || stdout != "" || len(fake.uploads()) != 0 ||
		!strings.Contains(err.Error(), "only valid for document") {
		t.Fatalf("stdout=%q err=%v puts=%v", stdout, err, fake.uploads())
	}
}

func TestOpenCollectionInputsRejectsInvalidListBeforeOpening(t *testing.T) {
	if _, _, err := openCollectionInputs([]string{"missing", "-"}); err == nil || !strings.Contains(err.Error(), "named files") {
		t.Fatalf("dash error = %v", err)
	}
	tooMany := make([]string, airplan.MaxCollectionFiles+1)
	for i := range tooMany {
		tooMany[i] = "missing"
	}
	if _, _, err := openCollectionInputs(tooMany); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("limit error = %v", err)
	}
}

func TestRootURLOutputWithoutJSON(t *testing.T) {
	fake := newFakeS3(t)
	stdout, stderr, err := executeRoot(t, fake,
		"# URL only\n",
		"--no-source",
		"--slug", "url-only",
		"-",
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}

	if strings.Count(stdout, "\n") != 1 {
		t.Fatalf("stdout = %q, want one line", stdout)
	}
	if strings.Contains(stdout, "{") || strings.Contains(stdout, "warning") {
		t.Fatalf("stdout contains non-URL content: %q", stdout)
	}
	if !strings.HasPrefix(stdout, "https://plans.example.com/") {
		t.Fatalf("stdout = %q, want public URL", stdout)
	}
}

func TestRootUploadsFileWithFilenameInference(t *testing.T) {
	fake := newFakeS3(t)
	input := filepath.Join(t.TempDir(), "example.go")
	if err := os.WriteFile(input, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeRoot(t, fake, "", "--json", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	var got rootJSON
	decodeJSONLine(t, stdout, &got)
	if !strings.HasSuffix(got.URL, "/example.html") ||
		!strings.HasSuffix(got.SourceURL, "/example.go") {
		t.Fatalf("result = %+v", got)
	}
	puts := fake.uploads()
	if len(puts) != 3 || !bytes.Contains(puts[2].body, []byte("chroma")) {
		t.Fatalf("uploads = %+v", puts)
	}
}

func TestRootUsesFrontMatterTitle(t *testing.T) {
	fake := newFakeS3(t)
	stdout, stderr, err := executeRoot(t, fake,
		"---\ntitle: Release plan\n---\n# Draft heading\n",
		"--repo", "none", "-",
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.HasPrefix(stdout, "https://plans.example.com/") {
		t.Fatalf("stdout = %q", stdout)
	}
	uploads := fake.uploads()
	if len(uploads) != 3 {
		t.Fatalf("uploads = %d, want marker, source, page", len(uploads))
	}
	if !bytes.Contains(uploads[0].body, []byte(`"title":"Release plan"`)) ||
		!bytes.Contains(uploads[2].body, []byte(`<title>Release plan</title>`)) {
		t.Fatalf("frontmatter title missing from marker/page: %#v", uploads)
	}
}

func TestRootNonexistentFileFailsBeforeUpload(t *testing.T) {
	fake := newFakeS3(t)
	missing := filepath.Join(t.TempDir(), "missing.md")

	stdout, _, err := executeRoot(t, fake, "", missing)
	if err == nil || stdout != "" || len(fake.uploads()) != 0 {
		t.Fatalf("stdout = %q, error = %v, uploads = %d",
			stdout, err, len(fake.uploads()))
	}
}

func TestRootWarningsGoToStderr(t *testing.T) {
	fake := newFakeS3(t)
	stdout, stderr, err := executeRootNoPublicBase(t, fake,
		"# Warnings\n",
		"--no-source",
		"--slug", "warn-routing",
		"-",
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}

	if strings.Contains(stdout, "airplan: warning") ||
		strings.Contains(stdout, "public_base_url") {
		t.Fatalf("stdout contains warning text: %q", stdout)
	}
	if !strings.Contains(stderr, "airplan: warning: public_base_url") {
		t.Fatalf("stderr = %q, want public_base_url warning", stderr)
	}
}

func TestRootLangReachesRenderedPage(t *testing.T) {
	fake := newFakeS3(t)
	stdout, stderr, err := executeRoot(t, fake,
		"package main\n",
		"--format", "txt",
		"--lang", "go",
		"--no-source",
		"--slug", "stdin-code",
		"-",
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.HasPrefix(stdout, "https://plans.example.com/") {
		t.Fatalf("stdout = %q, want public URL", stdout)
	}

	uploads := fake.uploads()
	if len(uploads) != 2 {
		t.Fatalf("got %d uploads, want marker and page", len(uploads))
	}
	page := string(uploads[1].body)
	if !strings.Contains(page, `class="chroma"`) {
		t.Fatal("rendered page body missing chroma class")
	}
	if !strings.Contains(page, `<span class="kn">package</span>`) {
		t.Fatalf("--lang go did not highlight package keyword:\n%s", page)
	}
}

func TestRootEmptyMermaidURLFlagResetsEnvironment(t *testing.T) {
	fake := newFakeS3(t)
	isolateEnv(t)
	customURL := "https://assets.example.test/custom-mermaid.mjs"
	t.Setenv("AIRPLAN_MERMAID_URL", customURL)

	cmd := newRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetIn(strings.NewReader("```mermaid\ngraph TD\n  A --> B\n```\n"))
	cmd.SetArgs([]string{
		"--endpoint", fake.server.URL,
		"--bucket", "plans",
		"--public-base-url", "https://plans.example.com",
		"--mermaid-url", "",
		"--no-source",
		"-",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	uploads := fake.uploads()
	if len(uploads) != 2 {
		t.Fatalf("got %d uploads, want marker and page", len(uploads))
	}
	page := string(uploads[1].body)
	if strings.Contains(page, customURL) {
		t.Fatal("empty Mermaid URL flag retained inherited custom URL")
	}
	if !strings.Contains(page, "cdn.jsdelivr.net/npm/mermaid@") {
		t.Fatal("empty Mermaid URL flag did not restore built-in default")
	}
}

func TestRootOpenFlagInvokesLauncher(t *testing.T) {
	var opened string
	oldOpenBrowser := openBrowser
	openBrowser = func(url string) error {
		opened = url
		return nil
	}
	t.Cleanup(func() {
		openBrowser = oldOpenBrowser
	})

	fake := newFakeS3(t)
	stdout, stderr, err := executeRoot(t, fake,
		"# Open\n",
		"--no-source",
		"-o",
		"--slug", "open",
		"-",
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}

	want := strings.TrimSpace(stdout)
	if opened != want {
		t.Fatalf("opened URL = %q, want %q", opened, want)
	}
}

type rootJSON struct {
	URL         string `json:"url"`
	Key         string `json:"key"`
	SourceURL   string `json:"source_url"`
	Bucket      string `json:"bucket"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
}

type fakeS3 struct {
	server *httptest.Server
	mu     sync.Mutex
	puts   []fakePut
}

type fakePut struct {
	path string
	body []byte
}

func newFakeS3(t *testing.T) *fakeS3 {
	t.Helper()

	fake := &fakeS3{}
	fake.server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)

			fake.mu.Lock()
			fake.puts = append(fake.puts, fakePut{
				path: r.URL.Path,
				body: body,
			})
			fake.mu.Unlock()

			w.WriteHeader(http.StatusOK)
		},
	))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeS3) uploads() []fakePut {
	f.mu.Lock()
	defer f.mu.Unlock()

	puts := make([]fakePut, len(f.puts))
	copy(puts, f.puts)
	return puts
}

func executeRoot(
	t *testing.T,
	fake *fakeS3,
	stdin string,
	args ...string,
) (string, string, error) {
	t.Helper()
	return executeRootWithPublicBase(t, fake, stdin,
		"https://plans.example.com", args...)
}

func executeRootNoPublicBase(
	t *testing.T,
	fake *fakeS3,
	stdin string,
	args ...string,
) (string, string, error) {
	t.Helper()
	return executeRootWithPublicBase(t, fake, stdin, "", args...)
}

func executeRootWithPublicBase(
	t *testing.T,
	fake *fakeS3,
	stdin string,
	publicBaseURL string,
	args ...string,
) (string, string, error) {
	t.Helper()
	isolateEnv(t)

	fullArgs := []string{
		"--endpoint", fake.server.URL,
		"--bucket", "plans",
	}
	if publicBaseURL != "" {
		fullArgs = append(fullArgs, "--public-base-url", publicBaseURL)
	}
	fullArgs = append(fullArgs, args...)

	cmd := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(fullArgs)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func isolateEnv(t *testing.T) {
	t.Helper()

	t.Setenv("AIRPLAN_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AIRPLAN_PROFILE", "")
	t.Setenv("AIRPLAN_TIMEOUT", "")
	t.Setenv("AIRPLAN_ENDPOINT", "")
	t.Setenv("AIRPLAN_BUCKET", "")
	t.Setenv("AIRPLAN_REGION", "")
	t.Setenv("AIRPLAN_PUBLIC_BASE_URL", "")
	t.Setenv("AIRPLAN_KEY_PREFIX", "")
	t.Setenv("AIRPLAN_TEMPLATE", "")
	t.Setenv("AIRPLAN_COLLECTION_TEMPLATE", "")
	t.Setenv("AIRPLAN_MERMAID_URL", "")
	t.Setenv("AIRPLAN_REPO", "")
	t.Setenv("AIRPLAN_NO_EXTERNAL_ASSETS", "")
	t.Setenv("AIRPLAN_ACCESS_KEY_ID", "test-access-key-id")
	t.Setenv("AIRPLAN_SECRET_ACCESS_KEY", "test-secret-access-key")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

func assertOneJSONLine(t *testing.T, stdout string) {
	t.Helper()

	if strings.Count(stdout, "\n") != 1 {
		t.Fatalf("stdout = %q, want exactly one line", stdout)
	}
	if !strings.HasPrefix(stdout, "{") ||
		!strings.HasSuffix(stdout, "}\n") {
		t.Fatalf("stdout = %q, want one JSON object", stdout)
	}
}

func decodeJSONLine(
	t *testing.T,
	stdout string,
	dest *rootJSON,
) map[string]json.RawMessage {
	t.Helper()

	line := strings.TrimSuffix(stdout, "\n")
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		t.Fatalf("json.Unmarshal fields: %v\nstdout: %s", err, stdout)
	}
	if err := json.Unmarshal([]byte(line), dest); err != nil {
		t.Fatalf("json.Unmarshal result: %v\nstdout: %s", err, stdout)
	}
	return fields
}

func assertJSONFieldOrder(
	t *testing.T,
	stdout string,
	fields []string,
) {
	t.Helper()

	prev := -1
	for _, field := range fields {
		idx := strings.Index(stdout, `"`+field+`":`)
		if idx == -1 {
			t.Fatalf("field %q missing from stdout: %s", field, stdout)
		}
		if idx <= prev {
			t.Fatalf("field %q out of order in stdout: %s", field, stdout)
		}
		prev = idx
	}
}

func assertJSONResult(t *testing.T, fake *fakeS3, got rootJSON) {
	t.Helper()

	if got.URL != "https://plans.example.com/"+got.Key {
		t.Fatalf("URL = %q, key = %q", got.URL, got.Key)
	}
	if got.Bucket != "plans" {
		t.Fatalf("bucket = %q, want plans", got.Bucket)
	}
	if got.ContentType != "text/html; charset=utf-8" {
		t.Fatalf("content_type = %q", got.ContentType)
	}

	uploads := fake.uploads()
	if len(uploads) == 0 {
		t.Fatal("no uploads captured")
	}
	page := uploads[len(uploads)-1]
	if page.path != "/plans/"+got.Key {
		t.Fatalf("page path = %q, want /plans/%s", page.path, got.Key)
	}
	if got.Bytes != int64(len(page.body)) {
		t.Fatalf("bytes = %d, want %d", got.Bytes, len(page.body))
	}
}
