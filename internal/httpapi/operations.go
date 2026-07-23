package httpapi

import (
	"context"
	"io"
)

// DocumentUpload carries the streaming document request to the operation
// adapter. The Reader is valid only for the duration of UploadDocument.
type DocumentUpload struct {
	Metadata DocumentMetadata
	Document io.Reader
}

// CollectionFile is one mode-0600 spooled collection member.
type CollectionFile struct {
	Name        string
	ContentType string
	Size        int64
	Reader      io.ReadSeeker
}

// CollectionUpload carries a fully validated collection to the operation
// adapter. Readers are valid only for the duration of UploadCollection.
type CollectionUpload struct {
	Metadata CollectionMetadata
	Files    []CollectionFile
}

// Download is a streaming marker-declared object response.
type Download struct {
	Body        io.ReadCloser
	Key         string
	Filename    string
	ContentType string
}

// Operations is the transport boundary between HTTP and Airplan's shared
// operation service. Implementations contain business logic; handlers only
// decode, bound, authenticate, and encode requests.
type Operations interface {
	Capabilities(context.Context) (Capabilities, error)
	UploadDocument(context.Context, DocumentUpload) (UploadResult, error)
	UploadCollection(context.Context, CollectionUpload) (UploadResult, error)
	InspectUpload(context.Context, TargetRequest) (UploadInspection, error)
	GetUpload(context.Context, GetUploadRequest) (Download, error)
	DeleteUpload(context.Context, TargetRequest) (DeleteResult, error)
	ListManifestUploads(context.Context) (ManifestList, error)
	ListStorageUploads(context.Context) (StorageList, error)
	SyncManifest(context.Context, SyncRequest) (SyncResult, error)
	PreviewPurge(context.Context, PurgePreviewRequest) (PurgePreview, error)
	ExecutePurge(context.Context, PurgeRequest) (PurgeResult, error)
}
