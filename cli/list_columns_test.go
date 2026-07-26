package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/jimeh/airplan/airplan"
)

func TestListColumnRegistryIsConsistent(t *testing.T) {
	names := map[string]bool{}
	headers := map[string]bool{}
	for _, column := range listColumnRegistry {
		if column.name == "" || column.header == "" {
			t.Fatalf("column %+v has an empty name or header", column)
		}
		if names[column.name] {
			t.Fatalf("duplicate column name %q", column.name)
		}
		if headers[column.header] {
			t.Fatalf("duplicate column header %q", column.header)
		}
		names[column.name] = true
		headers[column.header] = true
		if column.local == listColumnUnavailable &&
			column.remote == listColumnUnavailable {
			t.Fatalf("column %q is unavailable in both modes", column.name)
		}
	}
}

// TestListColumnsRenderValuesForEveryMode fails when a column is registered as
// available in a mode but that mode's renderer has no case for it, which would
// otherwise print a silent dash.
func TestListColumnsRenderValuesForEveryMode(t *testing.T) {
	record := airplan.ManifestRecord{
		Type: "upload",
		Time: time.Date(2026, 7, 8, 14, 3, 11, 0, time.UTC),
		Key:  deleteDirA + "/plan.html",
		MarkerKey: deleteDirA + "/" +
			airplan.MarkerFilename,
		URL:     "https://plans.example.com/" + deleteDirA + "/plan.html",
		Bucket:  "plans",
		Profile: "work",
		Format:  "md",
		Kind:    "document",
		Slug:    "plan",
		Title:   "Work plan",
		Repo:    "https://github.com/acme/service",
		Bytes:   18432,

		MarkerVersion: airplan.MarkerVersion,
	}
	upload := airplan.RemoteUpload{
		Dir:          deleteDirA,
		MarkerKey:    deleteDirA + "/" + airplan.MarkerFilename,
		Kind:         airplan.UploadKindDocument,
		Slug:         "plan",
		Key:          deleteDirA + "/plan.html",
		URL:          "https://plans.example.com/" + deleteDirA + "/plan.html",
		Objects:      2,
		Bytes:        18532,
		LastModified: time.Date(2026, 7, 8, 14, 3, 11, 0, time.UTC),
	}

	for _, name := range listColumnNames(listModeLocal) {
		if got := localListCell(name, record); got == "-" || got == "" {
			t.Errorf("localListCell(%q) = %q, want a rendered value",
				name, got)
		}
	}
	for _, name := range listColumnNames(listModeRemote) {
		if got := remoteListCell(name, upload); got == "-" || got == "" {
			t.Errorf("remoteListCell(%q) = %q, want a rendered value",
				name, got)
		}
	}
}

// TestListColumnsRenderDashForMissingValues covers legacy history: optional
// fields render a dash instead of an empty cell, and a pre-marker key yields
// no upload directory.
func TestListColumnsRenderDashForMissingValues(t *testing.T) {
	record := airplan.ManifestRecord{
		Type:  "upload",
		Time:  time.Date(2026, 7, 8, 14, 3, 11, 0, time.UTC),
		Key:   "legacy/plan.html",
		URL:   "https://plans.example.com/legacy/plan.html",
		Bytes: 42,
	}
	for _, name := range []string{
		"kind", "title", "slug", "dir", "format", "repo", "bucket",
	} {
		if got := localListCell(name, record); got != "-" {
			t.Errorf("localListCell(%q) = %q, want -", name, got)
		}
	}
	if got := localListCell("state", record); got != "legacy" {
		t.Errorf("localListCell(state) = %q, want legacy", got)
	}
	if got := localListCell("profile", record); got != "<root>" {
		t.Errorf("localListCell(profile) = %q, want <root>", got)
	}
}

func TestSelectListColumnsRejectsUnknownAndWrongMode(t *testing.T) {
	tests := []struct {
		name string
		mode listMode
		spec string
		want string
	}{
		{"unknown local", listModeLocal, "nope", "unknown list column"},
		{"unknown remote", listModeRemote, "nope", "unknown list column"},
		{
			"remote-only locally", listModeLocal, "objects",
			"only available with --remote",
		},
		{
			"local-only remotely", listModeRemote, "profile",
			"not available with --remote",
		},
		{
			"local-only remotely additive", listModeRemote, "+title",
			"not available with --remote",
		},
		{"mixed forms", listModeLocal, "+dir,title", "cannot mix"},
		{"mixed forms reversed", listModeLocal, "title,-dir", "cannot mix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := selectListColumns(
				tt.mode,
				listColumnRequest{spec: tt.spec, changed: true},
				nil,
			)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
