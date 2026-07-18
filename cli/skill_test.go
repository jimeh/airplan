package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jimeh/airplan/airplan"
)

func TestSkillCommandPrintsExactCanonicalSkill(t *testing.T) {
	want, err := os.ReadFile("../skills/airplan/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"skill"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got := stdout.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("stdout length = %d, want %d", len(got), len(want))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestSkillCommandNeedsNoConfigAndWorksFromAnyDirectory(t *testing.T) {
	badConfig := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(badConfig, []byte("not = [valid\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AIRPLAN_CONFIG", badConfig)

	t.Chdir(t.TempDir())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"skill"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), airplan.AgentSkill(); got != want {
		t.Fatalf("stdout length = %d, want %d", len(got), len(want))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestSkillCommandRejectsArguments(t *testing.T) {
	cmd := newSkillCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"extra"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}

func TestSkillCommandReturnsWriterErrors(t *testing.T) {
	wantErr := errors.New("write failed")
	cmd := newSkillCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetOut(errorWriter{err: wantErr})
	if err := cmd.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

var _ io.Writer = errorWriter{}
