package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jimeh/airplan/airplan"
)

func TestShowCommandHumanAndJSON(t *testing.T) {
	createdAt := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	body, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: airplan.MarkerVersion,
		Directory: deleteDirA, CreatedAt: createdAt, Format: "html",
		Page: "plan.html", Title: "Remote plan",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, jsonOutput := range []bool{false, true} {
		name := "human"
		if jsonOutput {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			isolateEnv(t)
			server := newFakeShowS3(t, body, createdAt)
			args := []string{
				"show", "--config", writeCLIConfig(t, server.URL), deleteDirA,
			}
			if jsonOutput {
				args = append(args, "--json")
			}
			stdout, stderr, err := executeCommand(t, "", "", args...)
			if err != nil {
				t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q", stderr)
			}
			if !jsonOutput {
				for _, want := range []string{
					"STATE", "complete", "DIRECTORY", deleteDirA,
					"TITLE", "Remote plan", "PAGE", "plan.html", "20 B",
				} {
					if !strings.Contains(stdout, want) {
						t.Fatalf("stdout missing %q:\n%s", want, stdout)
					}
				}
				return
			}

			var got struct {
				State string `json:"state"`
				Dir   string `json:"dir"`
				Page  struct {
					Key    string `json:"key"`
					Exists bool   `json:"exists"`
					Bytes  *int64 `json:"bytes"`
				} `json:"page"`
			}
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatal(err)
			}
			if got.State != "complete" || got.Dir != deleteDirA ||
				got.Page.Key != deleteDirA+"/plan.html" || !got.Page.Exists ||
				got.Page.Bytes == nil || *got.Page.Bytes != 20 {
				t.Fatalf("show JSON = %+v", got)
			}
		})
	}
}

func TestShowCommandWarnsForFallbackPublicURL(t *testing.T) {
	isolateEnv(t)
	createdAt := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	body, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: airplan.MarkerVersion,
		Directory: deleteDirA, CreatedAt: createdAt, Format: "html",
		Page: "plan.html",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := newFakeShowS3(t, body, createdAt)
	config := filepath.Join(t.TempDir(), "config.toml")
	data := fmt.Sprintf("endpoint = %q\nbucket = \"plans\"\ntimeout = \"0\"\n",
		server.URL)
	if err := os.WriteFile(config, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeCommand(
		t, "", "", "show", "--config", config, deleteDirA,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, server.URL+"/plans/"+deleteDirA+
		"/plan.html") || !strings.Contains(stderr, "public_base_url") {
		t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func newFakeShowS3(
	t *testing.T, markerBody []byte, when time.Time,
) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("list-type") == "2" {
				w.Header().Set("Content-Type", "application/xml")
				fmt.Fprintln(w, `<?xml version="1.0"?><ListBucketResult>`+
					`<IsTruncated>false</IsTruncated>`+
					fmt.Sprintf("<Contents><Key>%s/%s</Key><Size>%d</Size>"+
						"<LastModified>%s</LastModified></Contents>",
						deleteDirA, airplan.MarkerFilename, len(markerBody),
						when.Format(time.RFC3339))+
					fmt.Sprintf("<Contents><Key>%s/plan.html</Key><Size>20</Size>"+
						"<LastModified>%s</LastModified></Contents>",
						deleteDirA, when.Format(time.RFC3339))+
					`</ListBucketResult>`)
				return
			}
			if r.Method != http.MethodGet ||
				r.URL.Path != "/plans/"+deleteDirA+"/"+airplan.MarkerFilename {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			_, _ = w.Write(markerBody)
		},
	))
	t.Cleanup(server.Close)
	return server
}
