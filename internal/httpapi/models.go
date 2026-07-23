package httpapi

import "github.com/jimeh/airplan/internal/httpapi/generated"

// REST wire models are aliases to the checked-in OpenAPI output. Keeping the
// public adapter names here avoids leaking the generated package throughout
// the domain layer while making schema drift a compile-time failure.
type (
	Health                    = generated.Health
	UploadLimits              = generated.UploadLimits
	Capabilities              = generated.Capabilities
	DocumentMetadata          = generated.DocumentMetadata
	CollectionMetadata        = generated.CollectionMetadata
	TargetRequest             = generated.TargetRequest
	GetUploadRequest          = generated.GetUploadRequest
	FileResult                = generated.FileResult
	UploadResult              = generated.UploadResult
	InspectedObject           = generated.InspectedObject
	UploadInspection          = generated.UploadInspection
	DeleteResult              = generated.DeleteResult
	ManifestRecord            = generated.ManifestRecord
	ManifestList              = generated.ManifestList
	RemoteUpload              = generated.RemoteUpload
	StorageList               = generated.StorageList
	SyncRequest               = generated.SyncRequest
	SyncFailure               = generated.SyncFailure
	SyncResult                = generated.SyncResult
	PurgePreviewRequest       = generated.PurgePreviewRequest
	PurgeCandidate            = generated.PurgeCandidate
	PurgePreview              = generated.PurgePreview
	PurgeRequest              = generated.PurgeRequest
	PurgeItemResult           = generated.PurgeItemResult
	PurgeResult               = generated.PurgeResult
	Problem                   = generated.Problem
	HealthStatus              = generated.HealthStatus
	CapabilitiesUploadFormats = generated.CapabilitiesUploadFormats
	CapabilitiesAPIVersion    = generated.CapabilitiesAPIVersion
	UploadResultKind          = generated.UploadResultKind
	DeleteResultKind          = generated.DeleteResultKind
	DocumentMetadataFormat    = generated.DocumentMetadataFormat
	ManifestRecordKind        = generated.ManifestRecordKind
	ManifestRecordType        = generated.ManifestRecordType
	RemoteUploadKind          = generated.RemoteUploadKind
	PurgePreviewRequestSource = generated.PurgePreviewRequestSource
	SyncFailureOperation      = generated.SyncFailureOperation
	UploadInspectionKind      = generated.UploadInspectionKind
	UploadInspectionState     = generated.UploadInspectionState
)
