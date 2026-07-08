package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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
		if !strings.Contains(stderr, "airplan: warning:") {
			t.Fatalf("stderr = %q, want warning", stderr)
		}

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
	if len(uploads) != 1 {
		t.Fatalf("got %d uploads, want 1", len(uploads))
	}
	page := string(uploads[0].body)
	if !strings.Contains(page, `class="chroma"`) {
		t.Fatal("rendered page body missing chroma class")
	}
	if !strings.Contains(page, `<span class="kn">package</span>`) {
		t.Fatalf("--lang go did not highlight package keyword:\n%s", page)
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
		"--config", filepath.Join(t.TempDir(), "missing.toml"),
		"--endpoint", fake.server.URL,
		"--bucket", "plans",
		"--timeout", "0",
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

	t.Setenv("AIRPLAN_CONFIG", filepath.Join(t.TempDir(), "missing.toml"))
	t.Setenv("AIRPLAN_PROFILE", "")
	t.Setenv("AIRPLAN_TIMEOUT", "")
	t.Setenv("AIRPLAN_ENDPOINT", "")
	t.Setenv("AIRPLAN_BUCKET", "")
	t.Setenv("AIRPLAN_REGION", "")
	t.Setenv("AIRPLAN_PUBLIC_BASE_URL", "")
	t.Setenv("AIRPLAN_KEY_PREFIX", "")
	t.Setenv("AIRPLAN_TEMPLATE", "")
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
