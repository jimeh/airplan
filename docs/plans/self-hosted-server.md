# Airplan Backend, REST API, and MCP Server Implementation Plan

Status: implemented in [PR #63](https://github.com/jimeh/airplan/pull/63)\
Scope: single-user self-hosted Airplan server, REST API, and MCP transports  
Repository baseline: Airplan spec 0.26.0

## 1. Goal

Extend Airplan from a direct S3 CLI/library into a transport-neutral client
with two accurately named backends:

- `s3`: the current behavior, where the process renders content, accesses S3,
  and owns a local manifest.
- `airplan`: an HTTP transport that asks a self-hosted Airplan server to invoke
  the same operation service that direct `s3` mode calls in-process. The server
  owns the S3 credentials and rendering configuration and uses its normal local
  manifest.

The public Go library remains the single operation API used by the CLI and by
the local stdio MCP server. Callers should not need to know which backend was
selected after configuration is resolved.

The same binary also gains:

- `airplan serve`: one single-user HTTP process exposing the REST API and MCP
  Streamable HTTP endpoint.
- `airplan mcp`: a local stdio MCP server that uses either backend through the
  normal Go client.

The REST API must support the complete backend-sensitive management lifecycle
from the first release: upload, inspect, get, delete, manifest list, direct S3
list, purge, and manifest sync.

## 2. Settled decisions

1. Use schema-first HTTP rather than gRPC.
2. Check in an OpenAPI 3.0.3 document as the authoritative REST wire contract.
   OpenAPI 3.2.0 is current, but stable `oapi-codegen` supports 3.0; do not base
   a production contract on its experimental 3.1/3.2 parser.
3. Generate the Go REST models, strict server interface, and client from that
   one document.
4. Use streaming `multipart/form-data` for document and collection uploads.
5. Use the official Model Context Protocol Go SDK.
6. Support MCP stdio and the current Streamable HTTP transport. Do not add the
   deprecated HTTP+SSE transport.
7. Use one static bearer token for both REST and HTTP MCP in the initial
   single-user server. OAuth, accounts, roles, and token issuance are deferred.
8. Keep purge safe and interactive by splitting candidate planning from
   execution. The server never prompts; the CLI prompts between those calls.
9. Keep server state file-backed. No database, web UI, background worker,
   multi-replica coordination, or multi-user model is included.
10. `airplan serve` must use an `s3` profile. It must reject an `airplan`
    profile to prevent accidental proxy chains and loops.
11. `airplan serve` is an extension of the local CLI experience, not a separate
    state silo. By default it uses the same platform manifest path as ordinary
    local commands running as the same OS user.

## 3. Non-goals

- Multi-user authentication, registration, invitations, RBAC, or audit UI.
- OAuth/OIDC login or the MCP OAuth authorization flow.
- A management web UI.
- A relational database or remotely coordinated manifest.
- Horizontal server replicas.
- Resumable/chunk-session uploads or automatic ambiguous-request retries.
- gRPC, gRPC-Web, ConnectRPC, or a second API description language.
- Template/configuration management over REST or MCP.
- Remote filesystem access through MCP.
- Compatibility with the deprecated MCP HTTP+SSE transport.

## 4. Behavioral model

### 4.1 Backend selects the operation transport

Backend selection occurs once during config resolution. The selected backend
chooses how the public client reaches the Airplan operation service.

| Operation               | `s3` backend            | `airplan` backend        |
| ----------------------- | ----------------------- | ------------------------ |
| Upload/render           | Local process           | Airplan server           |
| S3 credentials          | Local process           | Airplan server only      |
| Manifest                | Local filesystem        | Server filesystem        |
| `list`                  | Local manifest          | Server manifest          |
| `list --remote`         | Direct local S3 listing | Server direct S3 listing |
| `show`, `get`, `delete` | Direct local S3         | REST API                 |
| `purge`                 | Local plan/delete       | Server plan/delete       |
| `sync`                  | S3 to local manifest    | S3 to server manifest    |

The S3-backed operation service owns manifest access wherever that service is
running. Direct `s3` calls reach a local service in-process. The `airplan`
transport reaches a service in `airplan serve` over HTTP. REST and hosted MCP
are adapters around that same service; they do not reimplement operations.

An `airplan` HTTP client must not also append upload/delete results to its own
machine's manifest. The remote service records them. However, `airplan serve`
running locally as the same OS user defaults to the ordinary platform manifest
path. Consequently, direct local commands and requests handled by the server
share one history. Existing writes serialize with a file lock and one append;
sync additionally locks, rereads, reduces, and appends. Ordinary reads remain
lock-free and tolerate an incomplete tail by warning and skipping it. This is
safe same-host process coordination, not mutual exclusion for readers:

```text
airplan serve --profile work
airplan --profile work list
```

The second command reads the same manifest and shows uploads made through the
server API. The server's manifest-list API also sees direct local uploads made
with the same profile and manifest path, but scopes its response to the
server's configured profile, bucket, and key prefix; it never exposes unrelated
profiles that happen to share the file.

The manifest must live on persistent storage. If it is lost, `sync` rebuilds
active history from marker-managed S3 uploads. Human-friendly metadata not
present in old markers has the same recovery limits as direct mode today.

### 4.2 Profile meaning

For an `airplan` profile, the CLI profile chooses the server connection. It
does not filter the server manifest by a coincidentally named S3 profile.

```toml
[profiles.shared]
backend = "airplan"
api_url = "https://airplan.example.com"
api_token = "..."
```

These commands all operate against the selected server and that server's one
configured S3 profile, bucket, and prefix:

```text
airplan --profile shared list
airplan --profile shared list --remote
airplan --profile shared sync
airplan --profile shared purge --older-than 30d
```

For `s3`, preserve existing local-manifest profile scoping and profile
inference behavior wherever it still applies.

### 4.3 Single-server constraint

The server remains deliberately single instance. The write-lock and
torn-tail-tolerant read behavior allows the local CLI and one server process to
coordinate safely on one host and manifest path; it does not make separate
replicas with separate files safe. Document this rather than adding a
distributed lock or database prematurely.

## 5. Configuration contract

### 5.1 Backend fields

Add these root/profile settings:

```toml
backend = "s3" # default when omitted
api_url = "https://airplan.example.com"
api_token = "..."
```

And environment variables:

```text
AIRPLAN_BACKEND
AIRPLAN_API_URL
AIRPLAN_API_TOKEN
```

Rules:

- Missing `backend` resolves to `s3`, keeping every existing config valid.
- `backend = "s3"` applies the existing endpoint, bucket, credential, public
  URL, rendering, and timeout requirements.
- `backend = "airplan"` requires an absolute HTTP(S) `api_url` and an API
  token. HTTPS is required unless the URL host is loopback.
- S3 fields inherited into an `airplan` profile are inactive, not errors. This
  permits an existing root-level S3 config plus a remote profile.
- Likewise, API fields inherited into an `s3` profile are inactive. Warn only
  when an inactive secret/endpoint was explicitly set in the selected profile,
  not merely inherited.
- Ambient AWS credentials must not be loaded for an `airplan` backend.
- API tokens must be redacted from config inspection, logs, and errors exactly
  as S3 secrets are.

Update `FileConfig`, `Settings`, `Config`, provenance reporting, generated
`schema/airplan.schema.json`, config tests, README examples, and SPEC section 7
together.

### 5.2 Global manifest selection

Promote the existing code-only `Config.ManifestPath` override into a root
persistent CLI option and process environment setting:

```text
airplan --manifest PATH <command>
AIRPLAN_MANIFEST=PATH airplan <command>
```

Rules:

- Resolution precedence is `--manifest` > `AIRPLAN_MANIFEST` >
  `DefaultManifestPath()` for every local S3-backed operation, including
  `airplan serve`.
- With `backend = "s3"`, the option selects the manifest used by upload,
  manifest list, inspect/delete reconciliation, purge, and sync.
- `airplan serve --manifest PATH` makes REST and hosted MCP operations use that
  exact manifest. Omitting it shares the ordinary CLI manifest for the same OS
  user and state environment.
- `airplan mcp --manifest PATH` applies when its selected backend is `s3`.
- An HTTP `airplan` client rejects an explicitly supplied `--manifest`: client
  callers cannot choose a remote server filesystem path. Configure the option
  on `airplan serve` instead. `AIRPLAN_MANIFEST` is a process-level local
  service default and is ignored when the resolved transport is HTTP, allowing
  one shell environment to use local and remote profiles.
- `--manifest` does not become a profile config key and is never sent over the
  REST or MCP protocols.
- Preserve existing code-level behavior: a relative path resolves against the
  caller's working directory. State that rule in SPEC and test it on Unix and
  Windows.

Add the flag once as a Cobra persistent root option so it is available to
upload, `list`, `show`, `get`, `delete`, `purge`, `sync`, `serve`, and `mcp`
without duplicating command definitions. For a local `s3` transport, these
backend-sensitive commands accept the resolved path consistently even when a
specific read-only mode such as `list --remote` does not access the manifest in
that invocation. Local-only commands that do not construct a backend client
(`preview`, `template`, `skill`, `config`, and `completion`) reject an
explicitly supplied flag as inapplicable. Auto-generated help may display the
persistent option but does not execute applicability validation.

Implement applicability in one root `PersistentPreRunE`/shared validator after
the final command and transport are known, rather than duplicating checks. The
validator distinguishes an explicitly changed flag from an inactive
`AIRPLAN_MANIFEST` default.

### 5.3 Server process configuration

Add:

```text
airplan serve \
  --profile storage \
  --listen 127.0.0.1:8080 \
  --token-file /run/secrets/airplan-token \
  --manifest /var/lib/airplan/manifest.jsonl
```

Server-only inputs:

- `--listen`, default `127.0.0.1:8080`.
- `--token-file`, with `AIRPLAN_SERVER_TOKEN` as the environment alternative.
- `--allowed-origin`, repeatable, for HTTP MCP Origin validation.
- `--temp-dir`, optional, for collection upload spooling.

The example's `--manifest` is the global option described above, not a
server-only flag. Without it, `serve` uses the same `DefaultManifestPath()` as
ordinary CLI operations. Containers or services running as a different OS user
must mount and explicitly select the desired persistent manifest when shared
local history is expected.

Do not overload client `api_token` as the server secret source. Refuse startup
when no server token exists, both token sources exist, the token is empty or
unreasonably short, the selected backend is not `s3`, or S3 validation fails.

Read the token once at startup. A token-file change requires restart in v1.
Recommend a random value of at least 32 bytes and a mode-0600 token file.

Default to loopback. Non-loopback binding should require an explicit
acknowledgement flag and documentation that TLS must terminate at a trusted
reverse proxy. The server itself need not manage certificates in v1.

## 6. Go library architecture

### 6.1 Operation service and transport boundary

Refactor the public `Client` into a stable facade:

```go
type Client struct {
    transport operationTransport
}

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
```

The exact private interface can be split into smaller internal capability
interfaces if that produces clearer files, but the transport boundary must
remain at Airplan operations rather than S3 object primitives.

Separate operation implementation from invocation transport:

```text
Client
├── backend=s3      → in-memory transport → operation service → S3 + manifest
└── backend=airplan → HTTP transport → REST server → same operation service
                                                   → S3 + manifest
```

- `operationService`: extracted from the current concrete `Client`; implements
  rendering, S3 access, marker reconciliation, manifest recording, and all
  management algorithms exactly once.
- `localTransport`: invokes that service in memory. Product
  `backend = "s3"` selects this path.
- `httpTransport`: wraps the generated OpenAPI client, streams inputs/results,
  and maps wire types/errors into existing public Go types. Product
  `backend = "airplan"` selects this path.
- REST handlers and hosted MCP handlers adapt requests directly to one
  `operationService`; they do not call a loopback client and do not contain a
  second copy of business logic.

The service's exact exported/internal shape may follow Go package constraints,
but it must be independently constructible by `airplan serve` with one
resolved `s3` config and manifest path. `New` validates product backend config,
then constructs either a local service/transport or the HTTP transport.

Split local service construction from storage readiness. The current `New`
eagerly creates S3 storage and checks credentials, but manifest-only operations
must remain config-free and must not contact S3. The revised local service:

- Resolves backend identity, profile metadata, and manifest path at
  construction without requiring complete S3 credentials.
- Lazily creates/checks S3 storage on the first operation that needs it, before
  reading upload input or mutating remote/local state.
- Lets manifest `ListManifest` and manifest-source `PlanPurge` dry runs execute
  without S3 construction.
- Defers storage initialization for an applied manifest purge until after the
  CLI has shown candidates and obtained confirmation.
- Exposes an explicit storage-readiness check that `airplan serve` calls during
  startup so a long-running server still fails fast on invalid credentials.

This intentionally changes the public `New` network-timing detail: construction
no longer proves S3 credentials. Document it in `IMPLEMENTATION.md`; observable
CLI error timing remains compatible because each S3-dependent operation checks
readiness at its start.

Existing exported `Client` method names and result types remain compatible.
Keep `RenderInput`, built-in templates, config inventory, and low-level
`ReadManifest` as local library functionality outside the transport interface.

`InspectRemoteUploads` remains available through both transports. The local
service preserves its optimized snapshot-aware implementation. The HTTP
transport performs bounded `InspectUpload` API calls using each supplied
`RemoteUpload.MarkerKey`, which already includes the server's key prefix, and
preserves input order. It must not send a bare upload ID through the
`url_or_key` field. `GetUpload` remains the existing buffering convenience
wrapper over the streaming `GetUploadTo` method.

### 6.2 Move remaining business logic into the library

Purge candidate selection currently lives substantially in `cli/purge.go`.
Move it into public library operations so the CLI, REST server, and MCP server
cannot diverge.

Make manifest listing scope explicit rather than relying on adapter-side
post-filtering:

```go
type ManifestScope string // "all" or "service"

type ListManifestOptions struct {
    Scope   ManifestScope
    Profile *string // optional exact local-list filter, including root ""
}
```

`all` preserves ordinary local list behavior and its optional profile filter.
`service` ignores caller profile filters and scopes to the operation service's
resolved profile, bucket, and key prefix. REST and hosted MCP always use
`service`; an HTTP client cannot request `all`.

Add types along these lines:

```go
type UploadSource string // "manifest" or "storage"

type PurgePlanOptions struct {
    Source        UploadSource
    CreatedBefore time.Time
    Slug          string
    All           bool
    Concurrency   int
}

type PurgeCandidate struct {
    UploadID  string
    Record    ManifestRecord
    Inspection *UploadInspection
    Warnings  []string
}

type PurgePlan struct {
    Candidates []PurgeCandidate
    Invalid    int
    Warnings   []string
}

type PurgeRequest struct {
    UploadIDs []string
}

type PurgeItemResult struct {
    UploadID string
    Deleted *DeleteResult
    Error string
}

type PurgeResult struct {
    Items []PurgeItemResult
}
```

Preserve these rules:

- A filter or explicit `All` is mandatory during planning.
- `All` satisfies the safety requirement but does not disable accompanying
  filters.
- For direct `s3` manifest planning only, the CLI preserves today's
  profile-only purge by mapping an explicitly selected profile with no other
  filter to `All: true`; backend scoping still limits candidates to that
  profile, bucket, and prefix. For storage/`--remote` planning and for an
  `airplan` profile, `--profile` only selects the connection and never counts as
  a deletion filter, so another filter or explicit `--all` is required.
- Manifest planning scopes records to the backend's configured profile,
  bucket, and key prefix.
- Storage planning performs one LIST snapshot, then bounded marker inspection.
- Invalid/conflicting markers are never deletion candidates.
- Execution receives explicit upload IDs, re-resolves and revalidates the current
  ownership marker, and deletes sequentially using `DeleteUpload` semantics.
- Partial failures do not prevent later explicit targets from being attempted.

The CLI remains responsible only for parsing `30d`/`2w`, presentation,
confirmation, and exit status.

### 6.3 Transport- and server-specific option restrictions

A uniform operation API does not imply that clients may control server-local
paths or policy:

- `SyncManifestOptions.ManifestPath` and global `--manifest` are valid only for
  local S3 service construction; reject client-side overrides for the HTTP
  transport.
- The Airplan server chooses template paths, source inclusion defaults,
  Mermaid policy, upload limits, and manifest path.
- Request-level document attributes such as format, title, slug, language, and
  canonical repository URL remain portable.
- Client `--max-size` and `--max-total-size` values remain portable lower
  bounds: the client preflights known sizes, sends its requested limits, and
  the server applies the stricter of client and server limits. A client cannot
  raise or disable the server's hard limits.
- Client-supplied remote-inspection concurrency remains valid from 1 through 64. The server validates it and may apply a lower configured/process cap; it
  never silently exceeds the requested value.
- Local repository discovery happens before an `airplan` upload and the client
  sends the resolved canonical URL. The server must never inspect its own
  working directory for repository context.

## 7. OpenAPI contract and generated code

### 7.1 Files and generation

Add:

```text
api/openapi.yaml                 authoritative REST contract
api/oapi-codegen.yaml            pinned generation configuration
internal/httpapi/api.gen.go      committed generated models/server/client
internal/httpapi/server.go       strict handler implementation
internal/httpapi/auth.go         bearer middleware
internal/httpapi/problems.go     RFC 9457 mapping
internal/httpapi/uploads.go      bounded multipart helpers
```

Use `oapi-codegen/v2` to generate:

- Models.
- Strict standard-library `net/http` server interface.
- Go client methods.

Use request-validation middleware explicitly; strict-server generation does
not fully validate requests or enforce security requirements. Implement bearer
authentication explicitly outside generated code.

Embed the exact checked-in schema and expose it at `GET /openapi.yaml`.
Integrate generation into `mise run generate` and drift detection into
`mise run generate:check`. Pin generator/tool versions in the existing mise
lock workflow and commit generated output.

Use `/api/v1` for the first API. `info.version` versions the wire contract, not
the running binary. Breaking wire changes require a new URL version; compatible
fields and operations remain within v1.

### 7.2 Endpoints

Public unauthenticated endpoints:

```text
GET /healthz
GET /openapi.yaml
```

Authenticated endpoints:

```text
GET    /api/v1/capabilities

POST   /api/v1/uploads/documents
POST   /api/v1/uploads/collections
POST   /api/v1/uploads/inspect
POST   /api/v1/uploads/get
POST   /api/v1/uploads/delete

GET    /api/v1/uploads
GET    /api/v1/storage/uploads

POST   /api/v1/sync
POST   /api/v1/purge/preview
POST   /api/v1/purge
```

Operation IDs should be stable and descriptive, for example
`uploadDocument`, `uploadCollection`, `inspectUpload`, `deleteUpload`,
`getUpload`, `listManifestUploads`, `listStorageUploads`, `syncManifest`,
`previewPurge`, and `executePurge`.

Do not combine manifest and storage list results behind a query switch: the
schemas and cost differ materially, and distinct generated methods are clearer.

### 7.3 Upload identity and target mapping

Expose the randomized upload directory component as an opaque `upload_id` in
upload, list, inspection, and purge-preview results. The public URL/key format
already places all modern upload objects under that directory.

The HTTP client's `airplan` profile does not know the server's public base URL,
bucket, endpoint, or key prefix, so it must not attempt to reproduce
`KeyFromURLOrKey` locally. Inspect, get, and delete accept a JSON
`TargetRequest` containing the existing public Go API's `url_or_key` value. The
server resolves it using its complete S3 config and applies the same scheme,
host/base-path, bucket, prefix, randomized-directory, marker, and
marker-declared-object validation as direct mode. Request bodies keep
capability URLs out of query/access logs.

This is constrained marker-managed target resolution, not generic S3 access:
the API cannot list or fetch an arbitrary key supplied outside a valid Airplan
upload. Retain compatibility with every target and marker version currently
supported by direct mode. Return the canonical `upload_id` and object metadata
in results where useful.

Purge uses only server-issued `upload_id` values from preview through
execution; raw S3 keys and URLs are never purge execution tokens.

### 7.4 Document upload

`POST /api/v1/uploads/documents` accepts streaming `multipart/form-data`:

- `metadata`: one `application/json` part.
- `document`: one binary part.

Metadata includes:

```json
{
  "name": "plan.md",
  "format": "md",
  "title": "Optional title",
  "slug": "optional-slug",
  "lang": "optional-language",
  "repository_url": "https://github.com/owner/repo"
}
```

Metadata also carries optional requested `max_size`; the server applies the
smaller of that value and its hard document limit. Omitting it selects the
server default. A negative or unlimited client value never disables the server
limit.

The generated client should accept an `io.Reader`. The `airplan` backend must
write multipart data through `io.Pipe` and `multipart.Writer`; it must not read
the complete source into memory.

The server applies the same document-size limit and text validation as direct
mode. It owns templates and server rendering policy.

### 7.5 Collection upload

`POST /api/v1/uploads/collections` accepts:

- `metadata`: JSON containing the optional title.
- Repeated `files` parts in CLI argument order.

Metadata may also request lower `max_size` and `max_total_size` values. Server
hard limits always win.

Preserve each sanitized basename and content type. Reject empty collections,
duplicate basenames, path traversal, malformed multipart bodies, excess part
counts, excess per-file size, and excess total size before publishing a marker.

The existing collection API requires exact sizes and `io.ReadSeeker`. Spool
each incoming member to a mode-0600 temporary file while counting and enforcing
limits, then call the normal collection operation. Clean every temporary file
on success, error, request cancellation, panic recovery, and graceful shutdown.
Do not buffer GiB-scale collections in memory.

### 7.6 Results

Return a superset of the existing CLI JSON result:

```json
{
  "id": "opaque-upload-id",
  "kind": "document",
  "url": "https://plans.example.com/.../plan.html",
  "key": "...",
  "source_url": "...",
  "source_key": "...",
  "bucket": "plans",
  "bytes": 12345,
  "content_type": "text/html",
  "title": "Plan",
  "created_at": "2026-07-23T12:00:00Z",
  "marker_version": 3,
  "repository_url": "https://github.com/owner/repo",
  "warnings": []
}
```

Collection responses additionally include ordered member results. The CLI
continues to print only its established URL or JSON shape to stdout and sends
warnings to stderr.

### 7.7 Listing

`GET /api/v1/uploads` reads the shared server manifest without contacting S3,
then returns active/history records scoped to the server's configured S3
profile, bucket, and key prefix. This deliberately differs from an unfiltered
local `airplan list`, which may display all profiles in the shared file. The
scope prevents a remote bearer-token holder from learning capability URLs or
bucket metadata for unrelated local profiles. Return manifest parse warnings
separately from records.

`GET /api/v1/storage/uploads` performs the same one-snapshot marker-prefix
listing as direct `list --remote`. It returns `RemoteUpload`-equivalent data and
does not silently sync the manifest.

Keep v1 unpaginated to match existing CLI behavior. Record this limitation and
add response/item limits only if real manifests make it necessary; do not
invent pagination semantics that the current library cannot use.

### 7.8 Inspect, get, and delete

- Each operation accepts a `TargetRequest` JSON body with `url_or_key`; get also
  carries the existing source-selection option. This deliberately uses POST so
  capability URLs/keys are not placed in query strings and the server can do
  authoritative target resolution in one request.
- Inspection uses the server's existing exact marker resolution and object
  validation.
- Object download streams bytes from the POST response. Do not buffer the
  object.
- Restrict object names to marker-declared page/source/member objects.
- Preserve content type and a safe download filename.
- Deletion validates ownership, deletes payload objects first and the marker
  last, and appends the server manifest tombstone.
- A repeated delete preserves current missing-marker reconciliation behavior.

### 7.9 Sync

`POST /api/v1/sync` accepts:

```json
{
  "prune": true,
  "dry_run": false,
  "concurrency": 8
}
```

The server executes the current one-LIST, targeted-confirmation, lock/reread,
deterministic-append algorithm against its manifest.

Because sync can make valid progress and also report per-item failures, return
a normal result containing additions, tombstones, counts, failures, and
warnings. The wire may include `complete` as derived convenience sugar defined
as `len(failures) == 0`; it does not change the public Go
`SyncManifestResult`. Whole-request/auth/validation/storage failures use HTTP
errors. The `airplan` backend maps a non-empty failures array back into the
existing Go result plus aggregate error so CLI exit behavior is unchanged.

### 7.10 Purge

`POST /api/v1/purge/preview` accepts:

```json
{
  "source": "manifest",
  "created_before": "2026-06-23T12:00:00Z",
  "slug": "release-*",
  "all": false,
  "concurrency": 8
}
```

The response contains explicit opaque `upload_id` values and display metadata.
`--dry-run` ends after this call. Otherwise the CLI prints candidates and
prompts unless `--yes` was supplied.

`POST /api/v1/purge` accepts only explicit targets from the preview:

```json
{
  "upload_ids": ["opaque upload ID 1", "opaque upload ID 2"]
}
```

Execution is intentionally stateless: do not persist purge plans. Resolve each
ID inside the server's configured key prefix and revalidate its current marker
through normal deletion so a stale preview cannot bypass ownership checks.
Return every success/failure. The library returns the result plus an aggregate
error when any item failed.

### 7.11 Capabilities and health

`GET /api/v1/capabilities` returns only safe compatibility data: API version,
server version, supported operations, supported upload formats, configured
size limits, and marker versions. Never return endpoints, bucket names, key
prefixes, filesystem paths, credentials, token metadata, or raw config.

`GET /healthz` is a liveness check and does not perform S3 calls on every
request. Startup already validates storage. Future readiness can be separate if
needed.

### 7.12 Errors

Use RFC 9457 `application/problem+json` for all REST errors, with stable
extensions:

```json
{
  "type": "https://airplan.dev/problems/invalid-upload",
  "title": "Invalid upload",
  "status": 422,
  "detail": "...",
  "code": "invalid_upload",
  "request_id": "..."
}
```

Clients branch on `code`, never on `detail`. Map typed Airplan errors to stable
HTTP status/code pairs. Do not expose S3 response bodies, credentials,
filesystem locations, stack traces, or internal request dumps.

Do not automatically retry upload POST requests in v1. A timeout after server
commit is ambiguous and correct idempotency requires durable request records.
Document the behavior and consider an idempotency design separately.

## 8. HTTP server implementation

### 8.1 Process lifecycle

`airplan serve` should:

1. Resolve and validate one `s3` profile.
2. Resolve token, global/default manifest path, and server-only paths.
3. Construct one `operationService` with the resolved S3 config and manifest.
4. Explicitly run its storage-readiness check so credentials fail before
   listening.
5. Construct REST and MCP adapters around that same service.
6. Start one `net/http.Server`.
7. Handle SIGINT/SIGTERM with bounded graceful shutdown.
8. Stop accepting requests, cancel in-flight work after the grace period, and
   clean temporary upload files.

Use explicit `ReadHeaderTimeout`, idle timeout, header-size limit, and bounded
graceful shutdown. Do not set a short whole-request `WriteTimeout` that breaks
large uploads/downloads. Product operation contexts and reverse-proxy limits
remain authoritative.

### 8.2 Authentication

Describe bearer authentication in OpenAPI using an HTTP bearer security
scheme. Apply it before parsing request bodies.

- Accept exactly `Authorization: Bearer <token>`.
- Reject multiple/malformed authorization values.
- Compare fixed-size token digests with constant-time comparison.
- Return the same generic 401 for missing and incorrect tokens.
- Include `WWW-Authenticate: Bearer` without revealing why validation failed.
- Never place tokens in URLs, logs, problem details, metrics, or request IDs.

Both REST and `/mcp` use this middleware. Health and schema routes do not.

### 8.3 Request safety

- Apply a total request-body limit before multipart parsing.
- Enforce per-part header/count limits and existing per-file/total limits.
- Sanitize names with the existing collection basename rules.
- Use mode-0600 temp files in a controlled directory.
- Disable directory traversal and symlink following.
- Disable CORS by default.
- Generate every request ID on the server and ignore incoming request-ID
  values.
- Log method, route template, status, duration, and request ID to stderr; omit
  query/body/auth values and capability URLs.
- Propagate request cancellation through rendering, S3, manifest locks, and
  streaming responses.

## 9. CLI integration

### 9.1 Route through the public client

Make backend-sensitive commands call only `Client` methods. Remove direct
manifest reads and purge selection from `cli/` except local-only compatibility
helpers that genuinely do not depend on backend selection.

Commands:

- Root upload and `--files` work unchanged against either backend.
- `show`, `get`, and `delete` route through the selected backend.
- `list` calls `ListManifest`; `list --remote` calls `ListRemote`.
- `sync` calls `SyncManifest`.
- `purge` calls `PlanPurge`, handles display/confirmation, then calls `Purge`.

Preserve the stdout contract exactly. Configuration notices, server warnings,
partial failures, prompts, and summaries remain stderr.

Wire the persistent `--manifest` option into the resolved local service config
before constructing `Client`. All local manifest reads, writes, profile
inference, reconciliation, purge, and sync must use that one resolved path;
individual commands must not accidentally fall back to `DefaultManifestPath()`.

### 9.2 Config resolution for `list`

Today local `list` can run without configuration. Backend selection means it
must resolve enough config to know whether history is local or on a server.

Preserve compatibility as follows:

- If no config/profile/backend selection is available, assume `s3` and read the
  default local manifest without requiring complete S3 credentials.
- If config resolves an `airplan` profile, require only API completeness and
  list the server manifest.
- If config resolves `s3`, preserve existing all-profile local list behavior
  unless an explicit profile filters it.
- `list --remote` always requires a complete selected backend.

This intentionally removes today's error for `list --config PATH` without
`--remote`: the config is now meaningful because it can select an `airplan`
manifest. Record that observable change in SPEC and CLI tests.

This needs a partial/config-purpose resolution path rather than weakening
normal `Config.Validate`.

Test the central UX explicitly: start `airplan serve --profile work` without a
manifest override, upload through its API, then run
`airplan --profile work list` as the same user and observe that upload from the
shared default manifest. Isolate the test's platform state directory so it
never touches developer history. Repeat with one explicit `--manifest` path
supplied to both processes.

### 9.3 Management-target profile inference

`show`, `get`, and `delete` currently inspect manifest history to infer a
profile before full config resolution. Preserve that convenience without
letting a local manifest accidentally redirect an HTTP client:

1. Resolve the global manifest path using flag/environment/default precedence.
2. If `--profile` or `AIRPLAN_PROFILE` is explicit, resolve it directly and do
   not perform manifest inference. Reject an explicit `--manifest` immediately
   if that profile selects the HTTP transport.
3. Otherwise read the resolved local manifest and attempt existing exact target
   inference.
4. An inferred record may select only a configured profile whose backend
   resolves to `s3`. Ignore an inferred `airplan` profile and continue normal
   default/single-profile resolution; remote clients never create local records
   for that backend.
5. After final resolution, permit the explicit manifest path for local S3 and
   reject it for HTTP before making a network request.

This allows a shared-manifest upload handled by `serve --profile work` to infer
the same local `s3` profile for direct management on the server machine. Add
tests for explicit local/HTTP profiles, inferred S3 profiles, ignored inferred
HTTP profiles, custom manifest paths, and missing/removed profiles.

### 9.4 Flags owned by the server

When using `airplan`, reject local-only overrides that the server cannot honor,
such as filesystem template paths. Error before opening/uploading inputs.
Document which request-level flags remain portable. Do not silently ignore a
user-supplied flag.

## 10. MCP implementation

### 10.1 SDK and transports

Use `github.com/modelcontextprotocol/go-sdk`, pinned through normal Go module
policy.

Add:

```text
airplan mcp [--config PATH] [--profile NAME] [--manifest PATH]
```

This is stdio only. It constructs the same public `Client`; therefore it works
with either `s3` or `airplan` configuration. MCP protocol messages are the only
stdout content. Logs and warnings go to stderr.

`airplan serve` exposes the same management tool implementation at `/mcp` via
Streamable HTTP. It calls the server's operation service directly rather than
making loopback REST requests.

### 10.2 Tools

Expose a deliberately small operation set:

| Tool              | Transports  | Purpose                                       |
| ----------------- | ----------- | --------------------------------------------- |
| `upload_document` | stdio, HTTP | Upload supplied text content                  |
| `upload_files`    | stdio only  | Upload local named file paths as a collection |
| `list_uploads`    | stdio, HTTP | Manifest list or direct storage list          |
| `inspect_upload`  | stdio, HTTP | Validate and describe one upload              |
| `delete_upload`   | stdio, HTTP | Delete one marker-managed upload              |
| `sync_manifest`   | stdio, HTTP | Preview/apply storage-to-manifest sync        |
| `preview_purge`   | stdio, HTTP | Return explicit purge candidates              |
| `execute_purge`   | stdio, HTTP | Delete explicit reviewed targets              |

`list_uploads` uses a `source` enum for agent ergonomics because both variants
share one user intent and structured tool result. REST keeps separate methods
because its generated wire response types and operational cost differ.

Do not expose template dumping, skill dumping, config introspection, server
configuration, arbitrary S3 object operations, or filesystem browsing.

`upload_document` accepts content plus name/format/title/slug/lang/repository
URL, making it portable across transports. HTTP MCP v1 omits file collections:
MCP has no adopted client-to-server file upload primitive, and server-local
paths would be both misleading and unsafe. Stdio `upload_files` can safely use
paths on the same machine and then forward through either backend.

Return URLs as text content with structured output matching a JSON Schema.
Where supported, also return resource links for the public page. Keep warnings
in structured output rather than corrupting stdio framing.

### 10.3 MCP destructive-operation safety

- `delete_upload` names one explicit target.
- `preview_purge` never deletes.
- `execute_purge` requires the exact explicit `upload_id` array obtained from a
  preview; it does not accept URLs, keys, filters, or `all`.
- `sync_manifest` defaults to dry-run in MCP unless `apply: true` is explicit.
- Tool descriptions must state persistence and deletion effects plainly.

### 10.4 Streamable HTTP details

Implement the current MCP Streamable HTTP transport at the single `/mcp`
endpoint. Do not add legacy HTTP+SSE compatibility unless concrete client
evidence later requires it.

MCP requires Origin validation for Streamable HTTP. Configure an explicit
verifier; do not rely on the SDK's changing zero-value default. Reject every
present Origin not in the configured allowlist with 403. Accept absent Origin
for non-browser agent clients. Keep the default allowlist empty.

Use the same static bearer middleware as REST. This is a documented custom
single-user authentication strategy, not implementation of MCP's OAuth
authorization framework. Some remote MCP clients that cannot configure custom
headers will therefore be unsupported in v1; document that limitation rather
than pretending interoperability is universal.

Prefer stateless MCP handling unless the SDK requires sessions for a selected
feature. The proposed tools need no server-to-client requests, subscriptions,
or durable MCP session state.

## 11. Spec, documentation, and generated artifacts

This changes observable behavior and therefore updates `SPEC.md` in the same
PR. The implemented server logging contract targets spec version 0.28.0.

Update:

- `SPEC.md`: processing model, server/non-goals, backend config, CLI commands,
  API behavior, auth, manifest ownership, MCP tools/transports, output/errors.
- `IMPLEMENTATION.md`: operation facade, implementations, generated API,
  server lifecycle, multipart spooling, MCP SDK, and test architecture.
- `README.md`: direct and self-hosted examples, reverse-proxy/TLS guidance,
  token setup, backend profiles, CLI/MCP client configuration, global
  `--manifest`/`AIRPLAN_MANIFEST`, shared same-user history, server API scoping,
  persistent manifest volumes for services/containers, and the single-instance
  limitation.
- `SECURITY.md`: bearer token threat model, capability URLs, origin checks,
  TLS requirement, secret handling, and disclosure scope.
- `skills/airplan/SKILL.md`: note that normal CLI commands transparently use
  either backend; do not teach agents to handle S3 or API credentials.
- `schema/airplan.schema.json`: regenerate for new backend fields.
- Runtime completion documentation plus command-registration/completion tests
  for `serve` and `mcp`.

The OpenAPI source is embedded into the binary and served at runtime. A
separate release asset is optional and should not be added until a consumer
requires it.

## 12. Implementation sequence

Implement as one cohesive feature PR with reviewable commits. The repository's
spec-sync sensor treats core Go changes as contract-sensitive, making a long
series of partially observable PRs unnecessarily awkward.

### Phase 1: Contract and scaffolding

1. Update SPEC/IMPLEMENTATION drafts and config schema source types.
2. Add backend discriminant and purpose-specific config validation.
3. Add config compatibility/provenance/schema tests.
4. Add OpenAPI source, generator config, generated-code checks, and problem
   schemas without exposing a server command yet.

Exit: existing configs and direct commands pass unchanged; generated API is
reproducible.

### Phase 2: Library facade and parity operations

1. Extract current behavior into one operation service and in-memory transport
   without behavior changes.
2. Separate manifest-only construction from lazy S3 readiness while preserving
   per-operation and purge-confirmation error timing.
3. Add `ListManifest`, `PlanPurge`, and `Purge` public operations.
4. Move purge filtering/remote inspection logic out of `cli/`.
5. Add global `--manifest` plumbing and prove all local service operations use
   the resolved path.
6. Add operation contract tests against the in-memory transport.

Exit: the direct CLI has identical observable behavior using only the new
facade.

### Phase 3: REST server

1. Implement strict handlers, auth, problem mapping, request IDs, and health.
2. Implement bounded document/collection upload and temp-file cleanup.
3. Implement inspect/get/delete/list/storage-list/sync/purge handlers, including
   configured profile/bucket/prefix scoping for shared-manifest list.
4. Add `airplan serve`, startup validation, signal handling, and persistent
   manifest configuration.

Exit: the generated client passes complete API integration tests against an
in-process server backed by MinIO.

### Phase 4: `airplan` HTTP backend and CLI routing

1. Implement the `airplan` HTTP transport around the generated client.
2. Stream multipart uploads and downloads.
3. Map problems and partial results into typed public errors/results.
4. Route every backend-sensitive CLI command through `Client`.
5. Preserve stdout/stderr, prompt, exit-code, profile, and timeout contracts.

Exit: the same CLI behavior passes against both direct MinIO and an Airplan
server in the integration suite.

### Phase 5: MCP

1. Add the official Go SDK and shared tool handlers.
2. Add stdio `airplan mcp` with both backend test matrices.
3. Mount Streamable HTTP `/mcp` in `airplan serve`.
4. Add explicit Origin verification and bearer integration.
5. Add tool schemas, structured output, destructive-operation wording, and
   transport-specific `upload_files` registration.

Exit: official SDK clients exercise stdio over both backends and authenticated
Streamable HTTP against the server.

### Phase 6: Hardening and documentation

1. Complete limits, cancellation, temp cleanup, concurrency, logging, and
   secret-redaction tests.
2. Update docs, examples, completion coverage, generated config schema, and
   agent skill.
3. Run the full verification matrix and manually smoke-test a reverse-proxied
   deployment with real S3-compatible storage when credentials are available.

## 13. Verification strategy

### 13.1 Unit tests

- Backend/default/inheritance/env/provenance validation.
- Existing config files remain valid and resolve `s3`.
- Inactive backend settings and explicit-inactive warnings.
- Global `--manifest` inheritance, defaulting, command coverage, relative or
  absolute path rule, and HTTP-transport rejection.
- `AIRPLAN_MANIFEST` precedence and inactivity under the HTTP transport.
- Same-path multi-process locked append plus lock-free/torn-tail-tolerant read
  behavior for a local CLI and server process.
- Config-free local list and config-free manifest purge dry-run without S3
  construction or credential checks.
- Applied purge obtains confirmation before lazy storage initialization and any
  credential failure.
- Management-target profile inference ordering across explicit, inferred S3,
  ignored inferred HTTP, and default profiles.
- URL/key to upload-ID/object-name parsing.
- Purge planning filters, `all` interaction, profile/bucket/prefix scope,
  conflicts, invalid markers, ordering, and partial execution.
- Problem-code mappings and generated response variants.
- Bearer parsing and constant-time digest comparison behavior.
- Origin allowlist behavior including absent, allowed, malformed, and rejected
  origins.
- Multipart part count, duplicate names, traversal, truncation, size limits,
  cancellation, and cleanup.
- MCP tool input/output schemas and destructive defaults.
- Stdout remains protocol-only under stdio MCP.

### 13.2 Contract tests

- Generated files match `api/openapi.yaml`.
- Every REST request is validated against the embedded schema.
- The generated client can call every generated server operation.
- Every documented success/status/problem response has a concrete test.
- Unknown JSON fields follow the chosen API compatibility rule consistently.
- API token security is both documented in OpenAPI and enforced by middleware.
- Server manifest listing excludes records from other profiles, buckets, and
  key prefixes in a shared multi-profile manifest.

### 13.3 Transport parity tests

Define a shared suite and run it against:

1. The in-memory transport and operation service with existing fake/test
   storage.
2. The HTTP transport against an in-process REST adapter around that same
   operation service and storage.

Cover upload document, upload collection, inspect, page/source/member get,
delete, manifest list, storage list, sync dry/apply/prune, purge preview/apply,
warnings, cancellation, and typed failures.

### 13.4 MinIO integration

Extend the existing testcontainers round trip:

- Start MinIO and an Airplan HTTP server with an isolated manifest/temp dir.
- Separately start a server with the normal default manifest path under an
  isolated platform state directory, upload through HTTP, and verify a
  same-user direct CLI `list` sees the record.
- Verify an explicit global `--manifest` makes direct CLI and server operations
  share that selected file, while a remote `airplan` client rejects the option.
- Seed the shared manifest with unrelated profile/bucket/prefix records and
  verify local unfiltered list can see them while the server API cannot.
- Upload through an `airplan` profile without S3 credentials on the client.
- Verify rendered objects and marker bytes match direct behavior.
- Restart the server and prove manifest persistence.
- Delete the manifest, sync, and verify recovery.
- Compare `list` and `list --remote` semantics.
- Exercise local and storage purge previews plus confirmed deletion.
- Verify collections larger than memory-test thresholds are streamed/spooled.
- Cancel mid-upload and assert no leaked temp files or published ownership
  marker.

### 13.5 MCP integration

- Launch `airplan mcp` as a subprocess and connect with the official SDK client.
- Run it once with `s3`, once with `airplan`, and compare tool results.
- Assert stdout contains only MCP frames; capture warnings from stderr.
- Connect to `/mcp` using the official Streamable HTTP client with correct,
  missing, and incorrect tokens.
- Verify rejected Origins, unsupported legacy transport behavior, and no
  `upload_files` tool on HTTP.
- Preview and execute a purge using explicit targets.

### 13.6 CLI and platform verification

- Preserve existing golden/output tests for all direct commands.
- Duplicate relevant command tests for `airplan` profiles.
- Verify timeout/cancellation and warning placement.
- Verify non-interactive purge errors and interactive confirmation behavior.
- Run native Windows unit tests for config, generated client, stdio framing,
  path handling, temp cleanup, and atomic manifest behavior.
- Run race tests around manifest sync, concurrent uploads, and shutdown where
  practical.

### 13.7 Required commands

At minimum:

```text
mise run generate
mise run generate:check
mise run check
mise run check:spec-sync
mise run test-integration
mise run audit:deps
mise run release:snapshot
```

Use `mise run verify` as the final broad local gate. A real deployment smoke
test should use a reverse proxy with HTTPS, an isolated token, persistent
manifest directory, and a non-production S3 prefix. Never print or inspect
stored credentials during verification.

## 14. Acceptance criteria

The feature is complete when:

1. Every existing config without `backend` behaves as `s3`.
2. A client configured only with Airplan API URL/token can perform the complete
   supported lifecycle without S3 credentials.
3. CLI stdout, JSON output, warning placement, prompts, and nonzero partial
   failure behavior remain compatible.
4. The S3 operation service owns manifest access where it runs. A same-user
   local CLI and `airplan serve` share the default manifest, an explicit global
   `--manifest` selects the same alternate file for both, and a remote HTTP
   client does not write a second local manifest.
5. The server manifest API scopes the shared file to its configured profile,
   bucket, and prefix; it never exposes unrelated local history.
6. Config-free local list and manifest purge preview remain free of S3
   credential checks, while `serve` still validates storage before listening.
7. Manifest list and direct S3 list remain distinct and map to `list` and
   `list --remote`.
8. Purge is previewed before execution, and execution only accepts explicit
   marker-validated targets.
9. The checked-in OpenAPI document reproduces the generated Go client/server
   and is served byte-for-byte by the binary.
10. Document and collection uploads are bounded and streaming; collection temp
    files are always cleaned.
11. `airplan mcp` works over stdio with both backends, and `/mcp` works via
    authenticated Streamable HTTP with explicit Origin validation.
12. No endpoint or MCP tool exposes S3 credentials, server config, arbitrary
    objects, arbitrary filesystem paths, or template management.
13. The single-user, single-instance, static-token, reverse-proxy TLS, and MCP
    client-compatibility limitations are prominent in documentation.
14. `mise run verify` passes, plus targeted MCP and API integration coverage.

## 15. Risks and mitigations

### Large refactor changes direct behavior

Mitigation: extract one operation service plus in-memory transport first, run
existing tests unchanged, then add a shared transport parity suite before
routing the CLI to HTTP.

### Lazy storage initialization changes error timing

Mitigation: initialize storage at the beginning of every S3-dependent
operation, preserve config-free manifest-only paths, keep purge initialization
after confirmation, make `serve` call readiness before listening, and pin each
timing contract with CLI/library tests.

### Generated code buffers uploads or downloads

Mitigation: inspect generated signatures and require `io.Reader`/
`multipart.Reader` for upload requests plus an `io.Reader`-bodied response path
for `POST /uploads/get`; wrap with custom streaming helpers and memory-bound
tests. Do not accept a generator upgrade that silently changes either property.

### Server/client manifest semantics drift

Mitigation: the operation service owns manifest operations wholesale, both the
CLI and server resolve the same default/global path for local S3 execution, and
the server scopes API reads to its own profile/bucket/prefix. Shared tests run
the same applicable scenarios through in-memory and HTTP transports.

### Purge becomes less safe over HTTP

Mitigation: server-side preview, client-side prompt, explicit-target execution,
and marker revalidation immediately before every delete.

### Static bearer tokens limit MCP interoperability

Mitigation: state the limitation, test clients with configurable headers, and
keep auth middleware separable so OAuth can be added without changing tools or
the application service.

### Reverse proxies break long uploads or MCP streaming

Mitigation: document HTTP/1.1 and HTTP/2 proxy settings, buffering/timeouts,
body limits, and forwarded scheme/host behavior; add one real proxy smoke test.

### Server state is mistaken for horizontally scalable state

Mitigation: refuse to claim multi-replica support, document one active server
per manifest, and defer shared state until multi-user work is intentionally
designed.

## 16. Deferred follow-ups

- Persistent idempotency records and safe automatic upload retry.
- OAuth 2.1/MCP authorization and multiple tokens.
- Multi-user identities, quotas, RBAC, and audit history.
- Web management UI and a queryable catalog.
- Shared database/lock for multiple replicas.
- Browser-native upload client generated from OpenAPI.
- Resumable upload sessions for unreliable or very large transfers.
- HTTP MCP file transfer if MCP adopts a portable client-to-server mechanism.
- Optional legacy HTTP+SSE transport only if supported-client evidence warrants
  it.

## 17. Standards and implementation references

- OpenAPI 3.2.0 specification:
  <https://spec.openapis.org/oas/v3.2.0.html>
- OpenAPI 3.0.3 specification used for generation:
  <https://spec.openapis.org/oas/v3.0.3>
- `oapi-codegen`:
  <https://github.com/oapi-codegen/oapi-codegen>
- RFC 9457, Problem Details for HTTP APIs:
  <https://www.rfc-editor.org/rfc/rfc9457.html>
- MCP 2025-11-25 transports:
  <https://modelcontextprotocol.io/specification/2025-11-25/basic/transports>
- MCP 2025-11-25 authorization:
  <https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization>
- Official MCP Go SDK:
  <https://github.com/modelcontextprotocol/go-sdk>
