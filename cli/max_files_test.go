package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMaxFilesInputs(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, len(names))
	for index, name := range names {
		paths[index] = filepath.Join(dir, name)
		if err := os.WriteFile(paths[index], []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func TestRootMaxFilesLimitsCollections(t *testing.T) {
	paths := writeMaxFilesInputs(t, "a.png", "b.png", "c.png")

	fake := newFakeS3(t)
	_, _, err := executeRoot(t, fake, "",
		append([]string{"--max-files", "2"}, paths...)...)
	if err == nil || !strings.Contains(err.Error(), "collection has 3 files; maximum is 2") {
		t.Fatalf("error = %v, want file-count limit", err)
	}
	if puts := fake.uploads(); len(puts) != 0 {
		t.Fatalf("PUTs = %d, want none", len(puts))
	}

	fake = newFakeS3(t)
	stdout, _, err := executeRoot(t, fake, "",
		append([]string{"--max-files", "0"}, paths...)...)
	if err != nil {
		t.Fatal(err)
	}
	// One URL per member plus the overview, and marker plus objects in storage.
	if lines := strings.Count(stdout, "\n"); lines != 4 {
		t.Fatalf("stdout = %q, want four URLs", stdout)
	}
	if puts := fake.uploads(); len(puts) != 5 {
		t.Fatalf("PUTs = %d, want 5", len(puts))
	}
}

func TestRootMaxFilesLimitsDocumentBundles(t *testing.T) {
	paths := writeMaxFilesInputs(t, "README.md", "logo.png")
	fake := newFakeS3(t)
	_, _, err := executeRoot(t, fake, "",
		"--max-files", "1", "--entrypoint", paths[0], "--asset", paths[1])
	if err == nil || !strings.Contains(err.Error(), "document has 2 items; maximum is 1") {
		t.Fatalf("error = %v, want document item limit", err)
	}
	if puts := fake.uploads(); len(puts) != 0 {
		t.Fatalf("PUTs = %d, want none", len(puts))
	}
}

func TestRootMaxFilesFlagValidation(t *testing.T) {
	paths := writeMaxFilesInputs(t, "a.png", "b.png")
	single := writeMaxFilesInputs(t, "plan.md")
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "negative",
			args: append([]string{"--max-files", "-1"}, paths...),
			want: "--max-files must not be negative",
		},
		{
			name: "not an integer",
			args: append([]string{"--max-files", "many"}, paths...),
			want: "invalid argument",
		},
		{
			name: "single document",
			args: []string{"--max-files", "5", single[0]},
			want: "--max-files is only valid for document bundles or collections",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeS3(t)
			_, _, err := executeRoot(t, fake, "", tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if puts := fake.uploads(); len(puts) != 0 {
				t.Fatalf("PUTs = %d, want none", len(puts))
			}
		})
	}
}

func TestPreviewMaxFilesLimitsCollections(t *testing.T) {
	isolateEnv(t)
	paths := writeMaxFilesInputs(t, "a.png", "b.png")
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs(append([]string{"preview", "--max-files", "1"}, paths...))
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "collection has 2 files; maximum is 1") {
		t.Fatalf("error = %v, want file-count limit", err)
	}
}

func TestServeRejectsNonPositiveMaxFiles(t *testing.T) {
	isolateEnv(t)
	for _, value := range []string{"0", "-5"} {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs([]string{"serve", "--max-files", value})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "--max-files must be positive") {
			t.Fatalf("--max-files %s error = %v", value, err)
		}
	}
}
