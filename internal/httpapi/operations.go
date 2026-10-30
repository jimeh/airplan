package httpapi

import (
	"context"
	"io"
)

// DocumentUpload carries the streaming document request to the operation
// adapter. The Reader is valid only for the duration of UploadDocument.
type DocumentUpload struct {
	Metadata     DocumentMetadata
	Document     io.Reader
	DocumentSize int64
	Pages        []DocumentPage
	Assets       []DocumentAsset
}

// DocumentPage is one managed page bound to its ordered metadata descriptor.
type DocumentPage struct {
	DocumentPageDescriptor
	Reader io.Reader
	Size   int64
}

// DocumentAsset is one exact-size, seekable asset bound to its descriptor.
// Exactly one of Reader and Open is set. Open-based assets are opened only
// while they are read and start at offset zero.
type DocumentAsset struct {
	DocumentAssetDescriptor
	Reader io.ReadSeeker
	Open   func() (io.ReadSeekCloser, error)
	Start  int64
}

type CreateDocumentRevisionUpload struct {
	Metadata     CreateDocumentRevisionMetadata
	Document     io.Reader
	DocumentSize int64
	Pages        []DocumentPage
	Assets       []DocumentAsset
}

// UpdateDocumentUpload is the compatibility name for revision multipart data.
type UpdateDocumentUpload = CreateDocumentRevisionUpload

// CollectionFile is one collection member. Exactly one of Reader and Open is
// set; Open-based members are opened only while they are read.
type CollectionFile struct {
	Name        string
	ContentType string
	Size        int64
	Reader      io.ReadSeeker
	Open        func() (io.ReadSeekCloser, error)
}

// CollectionUpload carries a fully validated collection to the operation
// adapter. Members are valid only for the duration of UploadCollection.
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
	CreateDocumentRevision(context.Context, CreateDocumentRevisionUpload) (DocumentRevisionResult, error)
	UploadCollection(context.Context, CollectionUpload) (UploadResult, error)
	PlanDocumentUpgrade(context.Context, UpgradePlanRequest) (UpgradeDocumentPlan, error)
	ExecuteDocumentUpgrade(context.Context, UpgradeDocumentPlan) (UpgradeDocumentResult, error)
	PlanBulkUpgrade(context.Context, BulkUpgradeOptions) (BulkUpgradePlan, error)
	ExecuteBulkUpgrade(context.Context, BulkUpgradeRequest) (BulkUpgradeResult, error)
	InspectUpload(context.Context, TargetRequest) (UploadInspection, error)
	GetUpload(context.Context, GetUploadRequest) (Download, error)
	DeleteUpload(context.Context, DeleteRequest) (DeleteResult, error)
	ProtectUpload(context.Context, ProtectRequest) (ProtectionResult, error)
	UnprotectUpload(context.Context, TargetRequest) (ProtectionResult, error)
	ListManifestUploads(context.Context) (ManifestList, error)
	ListStorageUploads(context.Context) (StorageList, error)
	SyncManifest(context.Context, SyncRequest) (SyncResult, error)
	PreviewPurge(context.Context, PurgePreviewRequest) (PurgePreview, error)
	ExecutePurge(context.Context, PurgeRequest) (PurgeResult, error)
}
