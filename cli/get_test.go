package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jimeh/airplan/airplan"
)

func TestGetCommandWritesExactBytesToStdout(t *testing.T) {
	isolateEnv(t)
	objects := getCLIObjects(t, "source bytes\n")
	server := newFakeGetS3(t, objects)
	page := objects[deleteDirA+"/plan.html"]

	stdout, stderr, err := executeCommand(
		t, "", "", "get", "--config", writeCLIConfig(t, server.URL),
		deleteDirA+"/plan.html",
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" || !bytes.Equal([]byte(stdout), page) {
		t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestGetCommandWritesOutputFileAtomically(t *testing.T) {
	isolateEnv(t)
	objects := getCLIObjects(t, "source bytes\n")
	server := newFakeGetS3(t, objects)
	output := filepath.Join(t.TempDir(), "download.html")

	stdout, stderr, err := executeCommand(
		t, "", "", "get", "--output", output,
		"--config", writeCLIConfig(t, server.URL), deleteDirA,
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, objects[deleteDirA+"/plan.html"]) {
		t.Fatalf("output = %q", got)
	}
	temps, err := filepath.Glob(filepath.Join(
		filepath.Dir(output), ".download.html.tmp-*",
	))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary files = %v, error = %v", temps, err)
	}
}

func TestGetCommandFetchesSource(t *testing.T) {
	isolateEnv(t)
	objects := getCLIObjects(t, "source bytes\x00\n")
	server := newFakeGetS3(t, objects)

	stdout, stderr, err := executeCommand(
		t, "", "", "get", "--source",
		"--config", writeCLIConfig(t, server.URL), deleteDirA,
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" || !bytes.Equal(
		[]byte(stdout), objects[deleteDirA+"/plan.md"],
	) {
		t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestGetCommandRejectsUnmanagedTarget(t *testing.T) {
	isolateEnv(t)
	server := newFakeGetS3(t, map[string][]byte{
		deleteDirA + "/plan.html": []byte("unmanaged page"),
	})

	stdout, _, err := executeCommand(
		t, "", "", "get", "--config", writeCLIConfig(t, server.URL),
		deleteDirA+"/plan.html",
	)
	if err == nil || stdout != "" ||
		!strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("stdout = %q, error = %v", stdout, err)
	}
}

func getCLIObjects(t *testing.T, source string) map[string][]byte {
	t.Helper()
	marker, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: airplan.MarkerVersion,
		Directory: deleteDirA,
		CreatedAt: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC),
		Format:    "md", Page: "plan.html", Source: "plan.md", Title: "Plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{
		deleteDirA + "/" + airplan.MarkerFilename: marker,
		deleteDirA + "/plan.html":                 []byte("page\x00bytes\n"),
		deleteDirA + "/plan.md":                   []byte(source),
	}
}

func newFakeGetS3(
	t *testing.T, objects map[string][]byte,
) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			key := strings.TrimPrefix(r.URL.Path, "/plans/")
			body, ok := objects[key]
			if !ok {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w,
					`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			_, _ = w.Write(body)
		},
	))
	t.Cleanup(server.Close)
	return server
}
