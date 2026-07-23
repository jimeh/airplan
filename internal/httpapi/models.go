package httpapi

import "time"

// Health is the unauthenticated liveness response.
type Health struct {
	Status string `json:"status"`
}

// UploadLimits reports server-enforced upload limits.
type UploadLimits struct {
	DocumentBytes        int64 `json:"document_bytes"`
	CollectionFileBytes  int64 `json:"collection_file_bytes"`
	CollectionTotalBytes int64 `json:"collection_total_bytes"`
}

// Capabilities reports safe wire-compatibility information.
type Capabilities struct {
	APIVersion     string       `json:"api_version"`
	ServerVersion  string       `json:"server_version"`
	Operations     []string     `json:"operations"`
	UploadFormats  []string     `json:"upload_formats"`
	Limits         UploadLimits `json:"limits"`
	MarkerVersions []int        `json:"marker_versions"`
}

// DocumentMetadata is the JSON metadata part of a document upload.
type DocumentMetadata struct {
	Name          string `json:"name"`
	Format        string `json:"format,omitempty"`
	Title         string `json:"title,omitempty"`
	Slug          string `json:"slug,omitempty"`
	Lang          string `json:"lang,omitempty"`
	RepositoryURL string `json:"repository_url,omitempty"`
	MaxSize       int64  `json:"max_size,omitempty"`
}

// CollectionMetadata is the JSON metadata part of a collection upload.
type CollectionMetadata struct {
	Title         string `json:"title,omitempty"`
	RepositoryURL string `json:"repository_url,omitempty"`
	MaxSize       int64  `json:"max_size,omitempty"`
	MaxTotalSize  int64  `json:"max_total_size,omitempty"`
}

// TargetRequest identifies one marker-managed upload without using a query.
type TargetRequest struct {
	URLOrKey string `json:"url_or_key"`
}

// GetUploadRequest selects a declared upload object.
type GetUploadRequest struct {
	URLOrKey string `json:"url_or_key"`
	Source   bool   `json:"source,omitempty"`
}

// FileResult describes one uploaded collection member.
type FileResult struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Key         string `json:"key"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
}

// UploadResult describes one completed document or collection upload.
type UploadResult struct {
	ID            string       `json:"id"`
	Kind          string       `json:"kind"`
	URL           string       `json:"url"`
	Key           string       `json:"key"`
	SourceURL     string       `json:"source_url,omitempty"`
	SourceKey     string       `json:"source_key,omitempty"`
	Bucket        string       `json:"bucket"`
	Bytes         int64        `json:"bytes"`
	ContentType   string       `json:"content_type"`
	Title         string       `json:"title,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	MarkerVersion int          `json:"marker_version"`
	RepositoryURL string       `json:"repository_url,omitempty"`
	Warnings      []string     `json:"warnings"`
	Files         []FileResult `json:"files"`
}

// InspectedObject describes one marker-declared payload object.
type InspectedObject struct {
	Key           string `json:"key"`
	URL           string `json:"url,omitempty"`
	Exists        bool   `json:"exists"`
	Bytes         int64  `json:"bytes"`
	ExpectedBytes int64  `json:"expected_bytes"`
	ExpectedKnown bool   `json:"expected_known"`
}

// UploadInspection is one validated remote marker inspection.
type UploadInspection struct {
	State         string             `json:"state"`
	ID            string             `json:"id"`
	MarkerKey     string             `json:"marker_key"`
	Objects       int                `json:"objects"`
	Bytes         int64              `json:"bytes"`
	CreatedAt     *time.Time         `json:"created_at,omitempty"`
	Format        string             `json:"format,omitempty"`
	Kind          string             `json:"kind,omitempty"`
	Title         string             `json:"title,omitempty"`
	RepositoryURL string             `json:"repository_url,omitempty"`
	MarkerVersion int                `json:"marker_version"`
	Page          *InspectedObject   `json:"page,omitempty"`
	Source        *InspectedObject   `json:"source,omitempty"`
	Files         []*InspectedObject `json:"files"`
	Warnings      []string           `json:"warnings"`
	Error         string             `json:"error,omitempty"`
}

// DeleteResult describes one completed or reconciled deletion.
type DeleteResult struct {
	ID        string   `json:"id"`
	Keys      []string `json:"keys"`
	PageKey   string   `json:"page_key"`
	MarkerKey string   `json:"marker_key"`
	Kind      string   `json:"kind"`
	Warnings  []string `json:"warnings"`
}

// ManifestRecord is one scoped manifest history record.
type ManifestRecord struct {
	Type          string    `json:"type"`
	Time          time.Time `json:"time"`
	Key           string    `json:"key"`
	SourceKey     string    `json:"source_key,omitempty"`
	MarkerKey     string    `json:"marker_key,omitempty"`
	URL           string    `json:"url,omitempty"`
	Bucket        string    `json:"bucket,omitempty"`
	Format        string    `json:"format,omitempty"`
	Kind          string    `json:"kind,omitempty"`
	Slug          string    `json:"slug,omitempty"`
	Title         string    `json:"title,omitempty"`
	RepositoryURL string    `json:"repository_url,omitempty"`
	Bytes         int64     `json:"bytes,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	MarkerVersion int       `json:"marker_version,omitempty"`
}

// ManifestList is the scoped, file-backed manifest response.
type ManifestList struct {
	Records  []ManifestRecord `json:"records"`
	Warnings []string         `json:"warnings"`
}

// RemoteUpload is one upload directory found in a storage LIST snapshot.
type RemoteUpload struct {
	ID           string    `json:"id"`
	MarkerKey    string    `json:"marker_key"`
	Kind         string    `json:"kind"`
	Conflict     bool      `json:"conflict"`
	Slug         string    `json:"slug,omitempty"`
	Key          string    `json:"key,omitempty"`
	URL          string    `json:"url,omitempty"`
	Keys         []string  `json:"keys"`
	Objects      int       `json:"objects"`
	Bytes        int64     `json:"bytes"`
	LastModified time.Time `json:"last_modified"`
}

// StorageList is one direct storage inventory response.
type StorageList struct {
	Uploads  []RemoteUpload `json:"uploads"`
	Warnings []string       `json:"warnings"`
}

// SyncRequest controls storage-to-manifest reconciliation.
type SyncRequest struct {
	Prune       bool `json:"prune,omitempty"`
	DryRun      bool `json:"dry_run,omitempty"`
	Concurrency int  `json:"concurrency,omitempty"`
}

// SyncFailure is one targeted request failure during reconciliation.
type SyncFailure struct {
	MarkerKey string `json:"marker_key"`
	Operation string `json:"operation"`
	Error     string `json:"error"`
}

// SyncResult includes all valid progress and per-item failures.
type SyncResult struct {
	Added      []ManifestRecord `json:"added_records"`
	Tombstoned []ManifestRecord `json:"tombstone_records"`
	Unchanged  int              `json:"unchanged"`
	Incomplete int              `json:"incomplete"`
	Invalid    int              `json:"invalid"`
	Retained   int              `json:"retained"`
	Failures   []SyncFailure    `json:"failures"`
	Warnings   []string         `json:"warnings"`
	Complete   bool             `json:"complete"`
}

// PurgePreviewRequest selects candidates without deleting them.
type PurgePreviewRequest struct {
	Source        string     `json:"source"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`
	Slug          string     `json:"slug,omitempty"`
	All           bool       `json:"all,omitempty"`
	Concurrency   int        `json:"concurrency,omitempty"`
}

// PurgeCandidate is an explicit reviewed deletion candidate.
type PurgeCandidate struct {
	UploadID   string            `json:"upload_id"`
	Record     ManifestRecord    `json:"record"`
	Inspection *UploadInspection `json:"inspection,omitempty"`
	Warnings   []string          `json:"warnings"`
}

// PurgePreview is a non-mutating purge plan.
type PurgePreview struct {
	Candidates []PurgeCandidate `json:"candidates"`
	Invalid    int              `json:"invalid"`
	Warnings   []string         `json:"warnings"`
}

// PurgeRequest contains only explicit IDs returned by a preview.
type PurgeRequest struct {
	UploadIDs []string `json:"upload_ids"`
}

// PurgeItemResult is one attempted explicit deletion.
type PurgeItemResult struct {
	UploadID string        `json:"upload_id"`
	Deleted  *DeleteResult `json:"deleted,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// PurgeResult reports every attempted deletion.
type PurgeResult struct {
	Items []PurgeItemResult `json:"items"`
}
