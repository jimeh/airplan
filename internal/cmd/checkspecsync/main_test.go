package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestIsContractSensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{path: "main.go", want: true},
		{path: "cli/root.go", want: true},
		{path: "airplan/render.go", want: true},
		{path: "cli/root_test.go", want: false},
		{path: "airplan/render_test.go", want: false},
		{path: "airplan/testdata/page.html", want: false},
		{path: "airplan/assets/page.js", want: true},
		{path: "airplan/assets/theme/style.css", want: true},
		{path: "schema/airplan.schema.json", want: true},
		{path: "tests/browser/page.spec.js", want: false},
		{path: "internal/cmd/genschema/main.go", want: false},
		{path: "README.md", want: false},
		{path: "SPEC.md", want: false},
		{path: ".github/workflows/ci.yml", want: false},
		{path: "go.sum", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := isContractSensitive(tt.path); got != tt.want {
				t.Fatalf("isContractSensitive(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}

func TestAnalyze(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		paths []string
		v     versions
		want  []string
	}{
		{
			name:  "unrelated docs are clean",
			paths: []string{"README.md", "docs/design.md"},
			v: versions{
				baseSpec: "0.19.1", currentSpec: "0.19.1",
				implementation: "0.19.1",
			},
		},
		{
			name:  "sensitive path without bump",
			paths: []string{"cli/café.go"},
			v: versions{
				baseSpec: "0.19.1", currentSpec: "0.19.1",
				implementation: "0.19.1",
			},
			want: []string{
				"contract-sensitive paths changed without a SPEC.md version bump: cli/café.go",
			},
		},
		{
			name:  "spec edited without version bump",
			paths: []string{"SPEC.md", "airplan/render.go"},
			v: versions{
				baseSpec: "0.19.1", currentSpec: "0.19.1",
				implementation: "0.19.1",
			},
			want: []string{
				"contract-sensitive paths changed without a SPEC.md version bump: airplan/render.go",
			},
		},
		{
			name:  "aligned bump is clean",
			paths: []string{"airplan/render.go", "SPEC.md", "IMPLEMENTATION.md"},
			v: versions{
				baseSpec: "0.19.1", currentSpec: "0.20.0",
				implementation: "0.20.0",
			},
		},
		{
			name:  "stale implementation target",
			paths: []string{"SPEC.md"},
			v: versions{
				baseSpec: "0.19.1", currentSpec: "0.20.0",
				implementation: "0.19.1",
			},
			want: []string{
				"IMPLEMENTATION.md targets spec version 0.19.1, but SPEC.md is version 0.20.0",
			},
		},
		{
			name: "multiple sensitive changes produce one sorted finding",
			paths: []string{
				"main.go",
				"README.md",
				"airplan/assets/page.js",
				"cli/removed.go",
				"internal/moved.go",
			},
			v: versions{
				baseSpec: "0.19.1", currentSpec: "0.19.1",
				implementation: "0.19.1",
			},
			want: []string{
				"contract-sensitive paths changed without a SPEC.md version bump: " +
					"airplan/assets/page.js, cli/removed.go, main.go",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := analyze(tt.paths, tt.v); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("analyze() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSplitPathsPreservesGitNULTerminatedNames(t *testing.T) {
	t.Parallel()

	want := []string{"cli/café.go", "airplan/line\nbreak.go"}
	got := splitPaths("cli/café.go\x00airplan/line\nbreak.go\x00")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitPaths() = %#v, want %#v", got, want)
	}
}

func TestParseVersions(t *testing.T) {
	t.Parallel()

	got, err := parseVersions(
		"**Spec version: 0.19.1**\n",
		"# Heading\n\n**Spec version: 0.20.0**\n",
		"Targets spec version 0.20.0.\n",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := versions{
		baseSpec: "0.19.1", currentSpec: "0.20.0", implementation: "0.20.0",
	}
	if got != want {
		t.Fatalf("parseVersions() = %#v, want %#v", got, want)
	}
}

func TestParseVersionsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		baseSpec       string
		currentSpec    string
		implementation string
		want           string
	}{
		{
			name: "missing base version", currentSpec: "**Spec version: 0.19.1**",
			implementation: "Targets spec version 0.19.1.",
			want:           "parse base SPEC.md version",
		},
		{
			name: "missing current version", baseSpec: "**Spec version: 0.19.1**",
			implementation: "Targets spec version 0.19.1.",
			want:           "parse current SPEC.md version",
		},
		{
			name: "duplicate current version", baseSpec: "**Spec version: 0.19.1**",
			currentSpec:    "**Spec version: 0.19.1**\n**Spec version: 0.20.0**",
			implementation: "Targets spec version 0.19.1.",
			want:           "expected exactly one declaration, found 2",
		},
		{
			name: "missing implementation target", baseSpec: "**Spec version: 0.19.1**",
			currentSpec: "**Spec version: 0.19.1**", want: "parse current IMPLEMENTATION.md version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseVersions(tt.baseSpec, tt.currentSpec, tt.implementation)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseVersions() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRunPolicyExitModes(t *testing.T) {
	t.Parallel()

	git := fakeGit{responses: map[string]string{
		"merge-base origin/main HEAD":                                         "abc123\n",
		"diff --name-only -z --no-renames --diff-filter=ACMRD abc123 HEAD --": "cli/root.go\x00",
		"show abc123:SPEC.md":                                                 "**Spec version: 0.19.1**",
		"show HEAD:SPEC.md":                                                   "**Spec version: 0.19.1**",
		"show HEAD:IMPLEMENTATION.md":                                         "Targets spec version 0.19.1.",
	}}

	for _, tt := range []struct {
		name       string
		env        map[string]string
		wantCode   int
		wantOutput string
	}{
		{
			name: "local finding fails", wantCode: 1,
			wantOutput: "spec sync warning: contract-sensitive paths",
		},
		{
			name: "warn only finding passes", env: map[string]string{
				"SPEC_SYNC_WARN_ONLY": "1",
			}, wantCode: 0, wantOutput: "spec sync warning: contract-sensitive paths",
		},
		{
			name: "github finding is annotated", env: map[string]string{
				"SPEC_SYNC_WARN_ONLY": "1", "GITHUB_ACTIONS": "true",
			}, wantCode: 0, wantOutput: "::warning title=SPEC sync::",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			getenv := func(key string) string { return tt.env[key] }
			if got := run(git, getenv, &stderr); got != tt.wantCode {
				t.Fatalf("run() = %d, want %d; stderr: %s", got, tt.wantCode, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantOutput) {
				t.Fatalf("stderr = %q, want substring %q", stderr.String(), tt.wantOutput)
			}
		})
	}
}

func TestRunOperationalErrorAlwaysFails(t *testing.T) {
	t.Parallel()

	git := fakeGit{err: errors.New("git unavailable")}
	var stderr bytes.Buffer
	getenv := func(key string) string {
		if key == "SPEC_SYNC_WARN_ONLY" {
			return "1"
		}
		return ""
	}
	if got := run(git, getenv, &stderr); got != 1 {
		t.Fatalf("run() = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "spec sync check failed") {
		t.Fatalf("stderr = %q, want operational error", stderr.String())
	}
}

type fakeGit struct {
	responses map[string]string
	err       error
}

func (f fakeGit) run(args ...string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	key := strings.Join(args, " ")
	response, ok := f.responses[key]
	if !ok {
		return "", errors.New("unexpected git command: " + key)
	}
	return response, nil
}
