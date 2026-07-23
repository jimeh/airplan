package airplan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

// operationTransport is the operation-level boundary behind Client. The local
// S3 implementation remains in Client's existing methods; an airplan backend
// supplies this transport and never constructs storage locally.
type operationTransport interface {
	Upload(context.Context, Input) (*Result, error)
	UploadFiles(context.Context, FilesInput) (*FilesResult, error)
	ListManifest(context.Context, ListManifestOptions) (*ManifestList, error)
	ListRemote(context.Context) ([]RemoteUpload, error)
	InspectUpload(context.Context, string) (*UploadInspection, error)
	InspectRemoteUploads(context.Context, []RemoteUpload, int) ([]RemoteInspectionResult, error)
	GetUploadTo(context.Context, string, GetOptions, io.Writer) (string, error)
	GetUpload(context.Context, string, GetOptions) (*GetResult, error)
	DeleteUpload(context.Context, string) (*DeleteResult, error)
	PlanPurge(context.Context, PurgePlanOptions) (*PurgePlan, error)
	Purge(context.Context, PurgeRequest) (*PurgeResult, error)
	SyncManifest(context.Context, SyncManifestOptions) (*SyncManifestResult, error)
}

// ManifestScope controls which records ListManifest returns.
type ManifestScope string

const (
	// ManifestScopeAll preserves local CLI history behavior.
	ManifestScopeAll ManifestScope = "all"
	// ManifestScopeService limits records to the configured service identity.
	ManifestScopeService ManifestScope = "service"
)

// ListManifestOptions controls manifest-backed listing.
type ListManifestOptions struct {
	Scope   ManifestScope
	Profile *string
}

// ManifestList contains active uploads and non-fatal parse warnings.
type ManifestList struct {
	Records  []ManifestRecord `json:"records"`
	Warnings []string         `json:"warnings,omitempty"`
}

// ListManifest lists active uploads through the selected backend.
func (c *Client) ListManifest(
	ctx context.Context, opts ListManifestOptions,
) (*ManifestList, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if c.remote != nil {
		if opts.Scope == ManifestScopeAll && opts.Profile != nil {
			return nil, errors.New(
				"airplan: remote manifest listing does not accept local profile filters",
			)
		}
		return c.remote.ListManifest(ctx, ListManifestOptions{
			Scope: ManifestScopeService,
		})
	}
	records, warnings, err := ReadManifest(c.cfg.ManifestPath)
	if err != nil {
		return nil, err
	}
	uploads := ManifestUploads(records)
	filtered := make([]ManifestRecord, 0, len(uploads))
	for _, rec := range uploads {
		switch opts.Scope {
		case "", ManifestScopeAll:
			if opts.Profile != nil && rec.Profile != *opts.Profile {
				continue
			}
		case ManifestScopeService:
			if rec.Profile != c.cfg.Profile {
				continue
			}
			if c.cfg.Bucket != "" && rec.Bucket != c.cfg.Bucket {
				continue
			}
			if c.cfg.KeyPrefix != "" &&
				!KeyMatchesPrefix(rec.MarkerKey, c.cfg.KeyPrefix) {
				continue
			}
		default:
			return nil, fmt.Errorf("airplan: invalid manifest scope %q", opts.Scope)
		}
		filtered = append(filtered, rec)
	}
	return &ManifestList{Records: filtered, Warnings: warnings}, nil
}

// UploadSource selects manifest or storage inventory for purge planning.
type UploadSource string

const (
	UploadSourceManifest UploadSource = "manifest"
	UploadSourceStorage  UploadSource = "storage"
)

// PurgePlanOptions controls safe, non-mutating purge planning.
type PurgePlanOptions struct {
	Source        UploadSource `json:"source"`
	CreatedBefore time.Time    `json:"created_before,omitempty"`
	Slug          string       `json:"slug,omitempty"`
	All           bool         `json:"all,omitempty"`
	Concurrency   int          `json:"concurrency,omitempty"`
}

// PurgeCandidate is one marker-managed upload eligible for deletion.
type PurgeCandidate struct {
	UploadID   string            `json:"upload_id"`
	Record     ManifestRecord    `json:"record"`
	Inspection *UploadInspection `json:"inspection,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
}

// PurgePlan is the result of non-mutating candidate selection.
type PurgePlan struct {
	Candidates []PurgeCandidate `json:"candidates"`
	Invalid    int              `json:"invalid"`
	Warnings   []string         `json:"warnings,omitempty"`
}

// PurgeRequest executes deletion of explicit server-issued upload IDs.
type PurgeRequest struct {
	UploadIDs []string `json:"upload_ids"`
}

// PurgeItemResult reports one attempted deletion.
type PurgeItemResult struct {
	UploadID string        `json:"upload_id"`
	Deleted  *DeleteResult `json:"deleted,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// PurgeResult reports every requested deletion in input order.
type PurgeResult struct {
	Items []PurgeItemResult `json:"items"`
}

// PlanPurge selects marker-managed uploads without deleting them.
func (c *Client) PlanPurge(
	ctx context.Context, opts PurgePlanOptions,
) (*PurgePlan, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if !opts.All && opts.CreatedBefore.IsZero() && opts.Slug == "" {
		return nil, errors.New(
			"airplan: purge requires at least one filter or explicit all",
		)
	}
	if opts.Source == "" {
		opts.Source = UploadSourceManifest
	}
	if opts.Slug != "" {
		if _, err := path.Match(opts.Slug, ""); err != nil {
			return nil, fmt.Errorf("airplan: invalid purge slug pattern: %w", err)
		}
	}
	if c.remote != nil {
		return c.remote.PlanPurge(ctx, opts)
	}

	plan := &PurgePlan{}
	switch opts.Source {
	case UploadSourceManifest:
		profile := c.cfg.Profile
		listed, err := c.ListManifest(ctx, ListManifestOptions{
			Scope: ManifestScopeAll, Profile: &profile,
		})
		if err != nil {
			return nil, err
		}
		plan.Warnings = append(plan.Warnings, listed.Warnings...)
		otherBuckets := 0
		otherPrefixes := 0
		for _, rec := range listed.Records {
			if c.cfg.Bucket != "" && rec.Bucket != c.cfg.Bucket {
				otherBuckets++
				continue
			}
			markerKey := rec.MarkerKey
			if markerKey == "" {
				markerKey = markerKeyForManifestRecord(rec)
			}
			if c.cfg.Bucket != "" &&
				!KeyMatchesPrefix(markerKey, c.cfg.KeyPrefix) {
				otherPrefixes++
				continue
			}
			if !IsSupportedMarkerVersion(rec.MarkerVersion) ||
				!purgeRecordMatches(rec, opts) {
				continue
			}
			id := uploadIDFromMarkerKey(markerKey)
			if id == "" {
				plan.Invalid++
				continue
			}
			plan.Candidates = append(plan.Candidates, PurgeCandidate{
				UploadID: id, Record: rec,
			})
		}
		if otherBuckets > 0 {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(
				"skipped %d upload(s) recorded for other buckets",
				otherBuckets,
			))
		}
		if otherPrefixes > 0 {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(
				"skipped %d upload(s) recorded for other key prefixes",
				otherPrefixes,
			))
		}
	case UploadSourceStorage:
		remote, err := c.ListRemote(ctx)
		if err != nil {
			return nil, err
		}
		inspections, err := c.InspectRemoteUploads(
			ctx, remote, opts.Concurrency,
		)
		if err != nil {
			return nil, err
		}
		for _, item := range inspections {
			if item.Err != nil {
				return nil, item.Err
			}
			inspection := item.Inspection
			if inspection == nil || inspection.State == UploadInvalid {
				plan.Invalid++
				continue
			}
			rec := manifestRecordFromInspection(c.cfg, inspection)
			if !purgeRecordMatches(rec, opts) {
				continue
			}
			plan.Candidates = append(plan.Candidates, PurgeCandidate{
				UploadID: item.Upload.Dir,
				Record:   rec, Inspection: inspection,
				Warnings: append([]string(nil), inspection.Warnings...),
			})
		}
	default:
		return nil, fmt.Errorf("airplan: invalid purge source %q", opts.Source)
	}
	if plan.Candidates == nil {
		plan.Candidates = []PurgeCandidate{}
	}
	return plan, nil
}

func purgeRecordMatches(rec ManifestRecord, opts PurgePlanOptions) bool {
	if !opts.CreatedBefore.IsZero() && !rec.Time.Before(opts.CreatedBefore) {
		return false
	}
	if opts.Slug == "" {
		return true
	}
	if rec.Kind == string(UploadKindCollection) {
		return false
	}
	slug := rec.Slug
	if slug == "" {
		slug, _ = pageSlug(path.Base(rec.Key))
	}
	matched, _ := path.Match(opts.Slug, slug)
	return matched
}

func manifestRecordFromInspection(
	cfg *Config, inspection *UploadInspection,
) ManifestRecord {
	rec := ManifestRecord{
		Type: "upload", Time: inspection.CreatedAt.UTC(),
		MarkerKey: inspection.MarkerKey, Bucket: cfg.Bucket,
		Profile: cfg.Profile, Format: inspection.Format,
		Kind: string(inspection.Kind), Title: inspection.Title,
		Repo: inspection.Repo, MarkerVersion: inspection.MarkerVersion,
	}
	if inspection.Page != nil {
		rec.Key = inspection.Page.Key
		rec.URL = inspection.Page.URL
		rec.Bytes = inspection.Page.Bytes
		if inspection.Kind == UploadKindDocument {
			rec.Slug, _ = pageSlug(path.Base(rec.Key))
		}
	}
	if inspection.Source != nil {
		rec.SourceKey = inspection.Source.Key
	}
	return rec
}

func uploadIDFromMarkerKey(markerKey string) string {
	parts := strings.Split(strings.Trim(markerKey, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	id := parts[len(parts)-2]
	if !isRandomDir(id) {
		return ""
	}
	return id
}

// Purge deletes explicit upload IDs sequentially and attempts every target.
func (c *Client) Purge(
	ctx context.Context, req PurgeRequest,
) (*PurgeResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if c.remote != nil {
		return c.remote.Purge(ctx, req)
	}
	if len(req.UploadIDs) == 0 {
		return nil, errors.New("airplan: purge requires at least one upload ID")
	}
	result := &PurgeResult{Items: make([]PurgeItemResult, 0, len(req.UploadIDs))}
	failed := 0
	for _, id := range req.UploadIDs {
		item := PurgeItemResult{UploadID: id}
		if !isRandomDir(id) {
			item.Error = "invalid upload ID"
			failed++
			result.Items = append(result.Items, item)
			continue
		}
		deleted, err := c.DeleteUpload(
			ctx, BuildKey(c.cfg.KeyPrefix, id, ""),
		)
		if err != nil {
			item.Error = err.Error()
			failed++
		} else {
			item.Deleted = deleted
		}
		result.Items = append(result.Items, item)
	}
	if failed > 0 {
		return result, fmt.Errorf(
			"airplan: purge failed: %d upload(s) failed", failed,
		)
	}
	return result, nil
}
