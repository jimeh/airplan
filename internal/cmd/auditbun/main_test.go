package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const validIgnore = `{"advisory": "GHSA-vfj7-8cjw-p6xm", "package": "braces",` +
	` "reason": "no patched release", "expires": "2026-11-16"}`

func writeIgnores(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ignores.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadIgnores(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []string
		wantErr string
	}{
		{
			name:    "one ignore",
			content: `{"ignores": [` + validIgnore + `]}`,
			want:    []string{"GHSA-vfj7-8cjw-p6xm"},
		},
		{
			name: "numeric advisory",
			content: `{"ignores": [{"advisory": "1240992", "package": "braces",` +
				` "reason": "no patched release", "expires": "2026-11-16"}]}`,
			want: []string{"1240992"},
		},
		{
			name:    "no ignores",
			content: `{"ignores": []}`,
		},
		{
			name: "advisory with trailing text",
			content: `{"ignores": [{"advisory":` +
				` "GHSA-vfj7-8cjw-p6xm,GHSA-aaaa-bbbb-cccc",` +
				` "package": "braces", "reason": "no patched release",` +
				` "expires": "2026-11-16"}]}`,
			wantErr: "invalid advisory",
		},
		{
			name: "advisory with leading text",
			content: `{"ignores": [{"advisory": "x1240992",` +
				` "package": "braces", "reason": "no patched release",` +
				` "expires": "2026-11-16"}]}`,
			wantErr: "invalid advisory",
		},
		{
			name:    "duplicate advisory",
			content: `{"ignores": [` + validIgnore + `, ` + validIgnore + `]}`,
			wantErr: "listed more than once",
		},
		{
			name: "missing package",
			content: `{"ignores": [{"advisory": "GHSA-vfj7-8cjw-p6xm",` +
				` "reason": "no patched release", "expires": "2026-11-16"}]}`,
			wantErr: "names no package",
		},
		{
			name: "blank reason",
			content: `{"ignores": [{"advisory": "GHSA-vfj7-8cjw-p6xm",` +
				` "package": "braces", "reason": " ",` +
				` "expires": "2026-11-16"}]}`,
			wantErr: "gives no reason",
		},
		{
			name: "missing expiry",
			content: `{"ignores": [{"advisory": "GHSA-vfj7-8cjw-p6xm",` +
				` "package": "braces", "reason": "no patched release"}]}`,
			wantErr: "invalid expiry",
		},
		{
			name: "expiry with time",
			content: `{"ignores": [{"advisory": "GHSA-vfj7-8cjw-p6xm",` +
				` "package": "braces", "reason": "no patched release",` +
				` "expires": "2026-11-16T00:00:00Z"}]}`,
			wantErr: "invalid expiry",
		},
		{
			name: "unknown field",
			content: `{"ignores": [{"advisory": "GHSA-vfj7-8cjw-p6xm",` +
				` "package": "braces", "reason": "no patched release",` +
				` "expire": "2026-11-16"}]}`,
			wantErr: "unknown field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ignores, err := loadIgnores(writeIgnores(t, tt.content))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, entry := range ignores {
				got = append(got, entry.Advisory)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("advisories = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckExpiry(t *testing.T) {
	t.Parallel()

	path := writeIgnores(t, `{"ignores": [`+validIgnore+`]}`)
	ignores, err := loadIgnores(path)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		now     time.Time
		wantErr bool
	}{
		{
			name: "last moment before expiry",
			now:  time.Date(2026, 11, 15, 23, 59, 59, 0, time.UTC),
		},
		{
			name:    "start of expiry date",
			now:     time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC),
			wantErr: true,
		},
		{
			// 2026-11-15T20:00-08:00 is 2026-11-16T04:00Z.
			name: "expiry date reached in UTC but not locally",
			now: time.Date(
				2026, 11, 15, 20, 0, 0, 0, time.FixedZone("PST", -8*60*60),
			),
			wantErr: true,
		},
		{
			name:    "long after expiry",
			now:     time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := checkExpiry(path, ignores, tt.now)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an expiry error")
			}
			for _, want := range []string{
				"GHSA-vfj7-8cjw-p6xm", "braces", "2026-11-16",
				"no patched release", path,
			} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	beforeExpiry := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	afterExpiry := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		extra       []string
		now         time.Time
		auditStatus int
		auditErr    error
		want        int
		wantAudit   []string
		wantStderr  string
	}{
		{
			name:  "audit passes",
			extra: []string{"--audit-level=high"},
			now:   beforeExpiry,
			wantAudit: []string{
				"audit", "--audit-level=high", "--ignore=GHSA-vfj7-8cjw-p6xm",
			},
		},
		{
			name:        "audit status is returned",
			now:         beforeExpiry,
			auditStatus: 3,
			want:        3,
			wantAudit:   []string{"audit", "--ignore=GHSA-vfj7-8cjw-p6xm"},
		},
		{
			name:       "audit cannot start",
			now:        beforeExpiry,
			auditErr:   errors.New("bun not found"),
			want:       1,
			wantAudit:  []string{"audit", "--ignore=GHSA-vfj7-8cjw-p6xm"},
			wantStderr: "bun not found",
		},
		{
			name:       "expired ignore skips the audit",
			extra:      []string{"--audit-level=high"},
			now:        afterExpiry,
			want:       1,
			wantStderr: "expired on 2026-11-16",
		},
		{
			name:       "ignore flag with value is rejected",
			extra:      []string{"--ignore=GHSA-aaaa-bbbb-cccc"},
			now:        beforeExpiry,
			want:       2,
			wantStderr: "--ignore=GHSA-aaaa-bbbb-cccc is not accepted",
		},
		{
			name:       "bare ignore flag is rejected",
			extra:      []string{"--ignore", "GHSA-aaaa-bbbb-cccc"},
			now:        beforeExpiry,
			want:       2,
			wantStderr: "--ignore is not accepted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeIgnores(t, `{"ignores": [`+validIgnore+`]}`)
			var gotAudit []string
			var stderr strings.Builder
			got := run(
				append([]string{path}, tt.extra...), tt.now, &stderr,
				func(args []string) (int, error) {
					gotAudit = args
					return tt.auditStatus, tt.auditErr
				},
			)
			if got != tt.want {
				t.Errorf("status = %d, want %d", got, tt.want)
			}
			if !slices.Equal(gotAudit, tt.wantAudit) {
				t.Errorf("audit args = %v, want %v", gotAudit, tt.wantAudit)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf(
					"stderr %q does not contain %q", stderr.String(),
					tt.wantStderr,
				)
			}
		})
	}
}

func TestRunWithoutArguments(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	got := run(nil, time.Now(), &stderr, func([]string) (int, error) {
		t.Error("audit ran without an ignore file")
		return 0, nil
	})
	if got != 2 || !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("status = %d, stderr = %q", got, stderr.String())
	}
}

// TestCommittedIgnoresAreValid keeps a malformed edit to the repository's
// ignore file out of the audit gate. It does not check expiry, which only the
// audit itself enforces.
func TestCommittedIgnoresAreValid(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "deps", "bun-audit-ignores.json")
	if _, err := loadIgnores(path); err != nil {
		t.Fatal(err)
	}
}
