package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenDocumentBundleUsesDeclaredEntryRootAndNames(t *testing.T) {
	root := t.TempDir()
	realEntry := filepath.Join(root, "real.md")
	declaredEntry := filepath.Join(root, "declared.md")
	member := filepath.Join(root, "docs", "member.md")
	if err := os.MkdirAll(filepath.Dir(member), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(realEntry, []byte("# Entry\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(member, []byte("# Member\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realEntry, declaredEntry); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	opened, err := openDocumentBundle(declaredEntry, []string{member}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.close()
	if opened.input.Entry.Path != "declared.md" ||
		len(opened.input.Pages) != 1 || opened.input.Pages[0].Path != "docs/member.md" {
		t.Fatalf("document paths = %+v", opened.input)
	}
}

func TestOpenDocumentBundleRejectsResolvedMemberEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	entry := filepath.Join(root, "plan.md")
	link := filepath.Join(root, "member.md")
	target := filepath.Join(outside, "member.md")
	if err := os.WriteFile(entry, []byte("# Entry\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("# Outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	_, err := openDocumentBundle(entry, []string{link}, nil)
	if err == nil || !strings.Contains(err.Error(), "resolves outside") {
		t.Fatalf("error = %v", err)
	}
}
