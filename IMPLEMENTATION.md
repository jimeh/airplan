# airplan — Implementation Notes

How _our_ implementation of [SPEC.md](SPEC.md) is built: language,
dependencies, code structure, repo deliverables, phasing, and
testing. Behavior is defined exclusively by the spec; nothing here
may contradict it. Targets spec version 0.28.0.

---

## 1. Language & Toolchain

**Go (1.26.4).** The exact minimum is declared by `go.mod` and pinned
in `mise.lock`. Rationale:

- Single static binary via `CGO_ENABLED=0`; trivial cross-compilation
  for the usual agent-host platforms (linux/amd64, linux/arm64,
  darwin/arm64, darwin/amd64, windows/amd64, windows/arm64).
- Cold-start is a few milliseconds — matters because the tool is
  invoked per-plan by agent harnesses.
- Mature ecosystem for exactly this job: `goldmark` (CommonMark + GFM
  markdown), `chroma` (syntax highlighting), `aws-sdk-go-v2`
  (S3-compatible client).
- Distribution via GoReleaser + Homebrew tap + `go install` covers
  every likely consumer.

Considered alternatives:

- **Rust**: equally good binary/startup story, slower to iterate for a
  tool this small; markdown-to-styled-HTML story (comrak + hand-rolled
  highlighting) is more work than goldmark + chroma.
- **Node/Bun/Python**: runtime dependency or heavyweight bundles;
  fails the "single static binary, fast startup" constraint.

## 2. Dependencies (deliberately few)

| Dependency                    | Purpose                               |
| ----------------------------- | ------------------------------------- |
| `yuin/goldmark` (+ GFM ext)   | markdown → HTML body                  |
| `alecthomas/chroma/v2`        | code block syntax highlighting        |
| `aws/aws-sdk-go-v2` (s3)      | uploads (SigV4, retries, checksums)   |
| `BurntSushi/toml`             | config file parsing                   |
| `gopkg.in/yaml.v3`            | YAML frontmatter parsing              |
| `spf13/cobra`                 | CLI: subcommands, flags, completion   |
| `invopop/jsonschema`          | config JSON Schema from Go structs    |
| `gofrs/flock`                 | cross-platform manifest file locking  |
| `golang.org/x/net/html`       | HTML5 tokenization for noindex splice |
| `oapi-codegen/v2`             | OpenAPI client/server code generation |
| `modelcontextprotocol/go-sdk` | MCP stdio and Streamable HTTP         |

Notes:

- Cobra, but not Viper. Cobra earns its keep with pflag-style
  long/short flags, built-in shell completion, and clean subcommand
  routing. Config resolution lives in the core `airplan` package
  with explicit flags > env > file precedence — Viper's magic isn't
  needed and obscures exactly the part that must be predictable.
- No `init()`-based command registration (the style cobra's docs
  push). Every command is a constructor — `newRootCmd()`,
  `newListCmd()`, … — returning a `*cobra.Command` with its flags
  bound locally; `main` stitches them together. No package-level
  command variables or globals, and constructors are directly
  testable with `cmd.SetArgs(...)` / `cmd.Execute()`.
- R2 compatibility: aws-sdk-go-v2 defaults to sending CRC32 request
  checksums, which older R2 deployments rejected. R2 supports CRC32
  now, but pin the SDK version tested against R2 and set
  `RequestChecksumCalculation: when_required` to stay safe.

## 3. Code Structure

The public surfaces are the CLI, the importable Go package, the REST API, and
MCP. The core library remains public; protocol adapters and generated wire
types live under `internal/`. The `main`
package sits at the repo root so
`go install github.com/jimeh/airplan@latest` installs a binary named
`airplan` with no `/cmd/...` suffix.

```
main.go                 package main — thin shim: cli.Execute()
cli/                    cobra command constructors (root, list, …);
                        flag parsing, output formatting, exit codes
airplan/                core library (public Go API): config
                        load/merge/validate, input reading + format
                        detection + noindex splice, markdown
                        rendering, ownership markers, key/slug
                        generation, collection preflight/rendering,
                        streaming S3 upload/get, list/show/delete,
                        manifest history, URL assembly; embeds assets
                        via go:embed; Mermaid is the sole conditional
                        runtime asset
api/openapi.yaml        authoritative REST wire contract (embedded)
api/oapi-codegen.yaml   deterministic generator configuration
internal/httpapi/       generated REST models/client/server plus auth,
                        problem mapping, request safety, and adapters
skills/embed.go         go:embed bridge for the canonical agent skill
schema/airplan.schema.json   generated config schema (committed)
skills/airplan/SKILL.md      agent skill: using airplan from harnesses
```

The core stays one cohesive package (`config.go`, `input.go`,
`render.go`, `keygen.go`, `storage.go`, …) — one import for
consumers; splitting into sub-packages adds ceremony without benefit
at this size. The import path `github.com/jimeh/airplan/airplan`
stutters, but the alternatives (`pkg/`, `lib/`) are worse.

`cli` contains no business logic: it parses flags, calls the core
package, and formats output per the spec's output contract. Anything
the CLI can do, a Go consumer can do by calling the core directly.

Public API sketch (kept deliberately small; best-effort stability
until v1.0, semver discipline after):

```go
cfg, err := airplan.LoadConfig(airplan.ConfigOptions{
    Path:    "",       // "" → XDG default
    Profile: "work",
})
client, err := airplan.New(ctx, cfg)
res, err := client.Upload(ctx, airplan.Input{
    Reader: file,
    Name:   "plan.md", // "" for stdin
})
// res.URL, res.Key, res.SourceURL, res.Bytes, res.ContentType,
// res.MarkerKey, res.Format, res.RepositoryURL

files, err := client.UploadFiles(ctx, airplan.FilesInput{
    Files: []airplan.FileInput{{
        Name: "demo.webm", Reader: recording, Size: recordingSize,
    }},
})
// files.Files[0].URL is direct; files.URL is the overview.

skill := airplan.AgentSkill() // exact canonical skills/airplan/SKILL.md

uploads, err := client.ListRemote(ctx) // one LIST traversal, no marker GETs
inspection, err := client.InspectUpload(ctx, uploads[0].MarkerKey)
fetched, err := client.GetUpload(ctx, inspection.Page.Key, airplan.GetOptions{})
_, err = client.GetUploadTo(ctx, inspection.Page.Key, airplan.GetOptions{}, dst)
deleted, err := client.DeleteUpload(ctx, inspection.MarkerKey)
synced, err := client.SyncManifest(ctx, airplan.SyncManifestOptions{
    Prune:       true,
    Concurrency: airplan.DefaultRemoteConcurrency,
})
```

## 4. Spec Requirements → Mechanisms

- Rendering: goldmark with GFM extensions (tables,
  strikethrough, task lists, autolinks), definition lists, footnotes,
  heading anchors,
  and a small local AST transformer/renderer for GitHub-style alerts.
  Alert parsing and HTML generation happen before template execution;
  the uploaded page needs CSS for presentation but no alert JavaScript.
  Unsafe rendering remains enabled so Markdown preserves authored raw
  HTML and link destinations; Markdown and explicit HTML input share the
  same trusted-content boundary.
- Frontmatter: a byte-oriented delimiter pass extracts exact leading YAML or
  TOML blocks before Goldmark sees the body. The native parsers validate a
  mapping root and extract only a string title; Chroma highlights the original
  block for the collapsed built-in presentation.
- Repository context: explicit remotes are normalized locally. Automatic
  discovery runs bounded `git` subprocesses with `Cmd.Dir`, checks file
  repository membership before the working-directory fallback, and accepts
  only GitHub.com origins. Resolution happens once for every input format so
  uploads persist canonical catalog metadata. A Goldmark AST transformer also
  turns Markdown references into links after parsing, where code, links,
  images, HTML, and autolinks can be excluded structurally.
- Columns: a strict line scanner indexes only complete supported Pandoc columns
  containers. Local Goldmark block parsers then build dedicated columns and
  column AST nodes, and a node renderer emits the fixed div markup. Goldmark
  parses all child Markdown in one document, preserving heading IDs and
  table-of-contents order; invalid structures remain ordinary Markdown nodes.
- Highlighting: chroma emitting class-based markup with generated system-theme
  media queries plus explicit-theme selector scopes (inline styles cannot
  switch light/dark). The spec's source view is chroma's markdown lexer run at
  render time.
- Mermaid: a stateless Goldmark node renderer intercepts only exact
  `mermaid` fences ahead of Chroma and emits escaped source containers. The
  built-in template conditionally imports the generated exact module URL,
  renders cached light and dark SVG variants with strict security, and swaps
  them synchronously for theme and print changes. The pin manifest under
  `internal/deps` generates exported constants; a networked updater observes a
  72-hour minimum age, stays within the current major, verifies jsDelivr, and
  refreshes generated/rendered artifacts. Dependency-only updates do not alter
  this document or SPEC.md.
- Built-in document and collection templates share source assets for base
  styling, early theme selection, runtime theme behavior, and theme-control
  markup. A generalized bake step expands shared and page-specific assets into
  each embedded layout before parsing. The source-friendly expansion retains
  asset comments for `airplan template [document|collection]`; executable
  expansion removes source-only comments so rendered output stays clean. Both
  commands therefore emit complete, standalone, reusable templates without
  exposing internal bake markers.
- Document templates: Go `html/template`. Canonical template data exposes the
  raw source string, rendered and highlighted `template.HTML`, Chroma's
  `template.CSS`, structured headings/ToC entries, format metadata,
  title, slug, indexing intent, frontmatter, repository context, and source
  names/paths. Document-specific CSS and JS cover the page grid, prose, source
  view, table of contents, copy controls, and Mermaid integration.
- Collection rendering uses a separate embedded `html/template` and stable
  `CollectionTemplateData` / `CollectionTemplateFile` surface. Preflight
  validates names and limits, resolves deterministic MIME/media kinds, creates
  already-escaped relative paths, and executes only the applicable collection
  template. The built-in template provides collection-specific responsive
  image, video, audio, and generic-file presentation, links image previews to
  their direct members, and uses the document toolbar's canonical shared
  geometry and interaction styling. Custom document and collection templates
  remain independently configurable.
- Local rendering: `RenderInput` owns read limits, binary and invalid
  UTF-8 rejection,
  format detection, title/slug resolution, template execution, and
  noindex handling. `RenderCollection` owns the equivalent collection
  preflight and local overview rendering. Explicit HTML is tokenized with
  `x/net/html`; raw token
  lengths locate the original head boundary while normalized tokens identify
  in-head robots metadata. Injection splices only the original byte slice and
  never serializes the token stream. `Client.Upload` adds source/page storage,
  URLs, and manifest recording; `airplan preview` stops after `RenderInput`.
  Preview and get file output rename a same-directory temporary file on Unix
  and use Windows `ReplaceFileW` (falling back to `MoveFileEx` for a new
  destination) so replacement matches the spec's atomicity contract on both
  families. Preview pages stay shareable at 0644; get downloads are written
  user-only at 0600 per SPEC.md §9.
- Public API boundaries: `New`, `RenderInput`, and every `Client` operation
  reject nil contexts; zero-value or nil clients return
  `ErrUninitializedClient`; and `PublicURL` reports a nil config as an error.
  Cancellation stops waiting for arbitrary input readers, but callers must
  still unblock or close a retained reader because Go cannot interrupt it.
- Key randomness: `crypto/rand` — never `math/rand` (spec requires a
  CSPRNG).
- Public URL assembly percent-encodes each object-key path segment;
  delete parsing uses `net/url` to recover the original UTF-8 key.
- Ownership markers: writers emit one v3 schema for all uploads. Documents use
  `.airplan.json`; collections use `.airplan-collection.json`. `kind` plus a
  normalized declared-object array describes pages, sources, and files;
  document slug/format fields remain conditional. Version-specific decoding
  normalizes v1/v2 into the same internal object model. A centralized
  concurrent two-key resolver proves exactly one marker exists without adding
  LIST permission to targeted reads. Kind/name mismatches and dual markers
  fail closed. Marker-first upload and marker-last deletion preserve the
  remote ownership boundary.
- Collection storage: `UploadFiles` accepts known-size `io.ReadSeeker` members,
  keeps inputs stable through preflight and sequential retryable PUTs, limits
  each reader to its declaration, and uploads the overview last. `GetUploadTo`
  streams large payloads to CLI stdout or atomic file output; the older
  `GetUpload` wrapper remains available for in-memory callers.
- Remote discovery: `ListRemote` recognizes both exact marker names, exposes
  their untrusted kind hint, selects exact collection `index.html`, and marks
  dual-name groups as conflicts without fetching markers or heading payloads.
  Its LIST snapshot retains per-key sizes for batch inspection.
  `InspectUpload` validates the selected marker and exact sizes for every
  normalized declared object. Targeted get and delete probe both markers and
  authorize only pages, document sources, collection files, or the existing
  marker.
- Manifest sync: `SyncManifest` reduces local history chronologically, compares
  the scoped active view to one remote LIST snapshot, and uses a shared bounded
  worker pool for marker GETs and targeted absence confirmation. Imports and
  tombstones are sorted, then the manifest is locked, reread, and rechecked
  before whole-line appends. Definite object-not-found is the only pruning
  signal; failures retain local state and return partial progress.
- Remote deletion: the marker must decode and authorize the supplied direct
  target. Payload objects are removed with batched `DeleteObjects`, then the
  marker is removed in a separate final `DeleteObject`. Invalid and markerless
  directories are outside airplan's remote management authority.
- `--older-than` durations: small custom parser for `d`/`w` units —
  Go's stdlib `time.ParseDuration` has no days.
- Manifest appends: `O_APPEND` open, whole line in one `Write` call,
  wrapped in context-aware `gofrs/flock` acquisition (flock on Unix,
  LockFileEx on Windows) per spec §9's concurrency and timeout rules.
  Records carry kind, document-only slug/format, portable marker metadata, and
  local connection context without duplicating collection inventories; readers
  discard malformed, oversized, and unsupported records completely and resume
  at the following newline. A latest-event state machine keyed by bucket and
  marker key makes tombstones reversible while retaining legacy key-only data.
- Purge: local candidates are constrained to the active bucket and key
  prefix before deletion. Remote candidates come from LIST, then marker
  inspection uses the same configurable 1-64 worker pool as sync; only valid
  non-conflicting markers grant deletion authority, marker `created_at` drives
  age filtering, slug filters apply only to documents, and deletion remains
  sequential.
- Config/state paths: `os.UserConfigDir` for config; a small helper
  for the state dir (`XDG_STATE_HOME` → `~/.local/state`,
  `%LocalAppData%` on Windows — Go stdlib has no state-dir
  function).

Config resolution derives provenance from the same definition metadata and
explicit inputs used by each precedence layer. `ResolveConfig` returns the
same `Config` as `LoadConfig` plus config-path, profile-selection, and
field-source metadata; `LoadConfig` delegates to it to keep one resolution
path. Field traces retain ordered source identities but no shadowed values,
avoiding duplicate credential material. `config show` is a thin table/JSON
formatter over that result and redacts both credential fields.
`ListConfigProfiles` shares strict config parsing, config-path selection,
dangling-default validation, and credential permission warnings, but bypasses
profile and field resolution entirely. `config profiles` only formats that
sorted inventory as a table or JSON array.

## 5. Config JSON Schema Generation

Generated from the core package's config structs via
`invopop/jsonschema`, struct tags carrying descriptions and defaults
— the schema cannot drift from the code that parses the file (a spec
requirement). The root level embeds the same profile struct that
`[profiles.*]` uses (alongside `default_profile` and the profiles
map), so root-level keys, inheritance merging, validation, and the
JSON Schema all fall out of one struct definition.

- A generated copy is committed at `schema/airplan.schema.json`;
  CI regenerates and fails on any diff (staleness check).
- GoReleaser attaches it to release assets alongside binaries.

## 6. Repo Deliverables (beyond the binary)

- Agent skill (`skills/airplan/SKILL.md`): teaches agent harnesses
  to use Airplan for explicitly requested document/file sharing and for visual
  evidence explicitly called for by authorized PR or issue work. It reviews
  captures for sensitive material, uploads related evidence in one JSON-mode
  collection, distinguishes `files[].url` from the overview `url`, and never
  invents or reuses partial results. It still prohibits opportunistic uploads.
  This file remains the single canonical source. `skills/embed.go`
  embeds it in the binary, the core package exposes the exact content
  through `AgentSkill`, and the thin `airplan skill` command writes it
  byte-for-byte without loading configuration or touching external
  state. The cached `mise run build` task tracks the skill tree as a
  source so edits invalidate the binary.
- README: R2 setup walkthrough (bucket, custom domain, token scoped
  to Object Read & Write on the one bucket), `#:schema` editor
  setup, installing the agent skill (copy into `.agents/skills/`
  or `.claude/skills/`, or reference from a plugin marketplace), and
  an optional belt-and-braces note: serving `X-Robots-Tag: noindex`
  via a Cloudflare Transform Rule on the custom domain (S3/R2 can't
  emit custom response headers themselves).
- Live demo automation: `.github/workflows/update-demos.yml` compares
  page and source origin bytes read through the storage API with the
  upload-mode render goldens, uploads only stale demos after pushes to
  `main`, and opens or updates a bot-owned README PR with GitHub App-signed
  commits. If the demo links on `main` are already current, the workflow closes
  any obsolete bot-owned demo-link PR and deletes its update branch. Manual
  runs may force fresh URLs. Published demo uploads are permanent and are
  never deleted by the workflow.

## 7. Distribution

GoReleaser: cross-platform archives, checksums, SPDX JSON SBOMs from
Syft, Homebrew tap (cask);
`airplan.schema.json` bundled into archives and published as a
standalone release asset (the `#:schema` URL). Shell completions are
generated at runtime by `airplan completion` rather than shipped. The
canonical agent skill is embedded in every binary and available at runtime
through `airplan skill`; it does not need a separate release asset.
Releases are cut by release-please from conventional commits. Merging
the release PR creates a remote tag and a notes-bearing draft, then passes
the tag and commit to the GoReleaser workflow. For production releases,
GoReleaser's OSS macOS notarization pipeline signs both Darwin executables
with the repository's Developer ID Application PKCS#12 identity, enables the
hardened runtime and secure timestamp, and waits for Apple notarization. It
does this before archives, archive SBOMs, checksums, and Homebrew cask hashes
are produced. Snapshot builds explicitly skip this stage and need no Apple
credentials. No entitlements are used.

The repository-scoped `MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD`, and
`MACOS_NOTARY_KEY` secrets provide the signing identity and Notary API key.
The repository variables `MACOS_NOTARY_KEY_ID` and
`MACOS_NOTARY_ISSUER_ID` identify that key. `MACOS_TEAM_ID` is a separate
repository variable used only to verify the finished signatures; it is not a
signing input. The automatic release-please call passes the three secrets
directly to the reusable release workflow, with no GitHub environment or
approval gate.

GoReleaser uploads archives, checksums, SBOMs, and the standalone schema into
the draft. The release workflow fails closed when any of the three Apple
secrets or three identity variables is empty, when the release commit is not
contained in `origin/main`, or when Apple rejects or times out. It records
GitHub SLSA provenance attestations and verifies the complete asset inventory
and GitHub SHA-256 digests. Native Apple Silicon and Intel jobs then download
the exact draft archives. GitHub restricts draft visibility to callers with
push access, so the download step receives a write-capable token even though
it only performs reads. The token is absent from the subsequent verification
step that executes the binary. That step verifies checksums, signature team
and authority, hardened runtime, timestamp, notarization, architecture, and
reported version. The online notarization-ticket check has a bounded retry for
transient Apple or network failures. Only after both jobs pass does the
workflow publish and verify the draft. Publication locks the existing tag and
assets and makes the release immutable.

GoReleaser OSS generates the Homebrew Cask without uploading it. The workflow
carries that exact file between jobs as an immutable, same-run workflow
artifact retained for seven days. After release publication and verification,
a separate downstream job mints a short-lived release bot token and atomically
replaces the tap's existing Cask through the GitHub Contents API. A failed Cask
write leaves the prior tap file in place and opens a Cask-specific issue. GitHub
can re-run that failed job without re-entering the successful, draft-only
release publication job. The workflow does not use GoReleaser Pro split/merge.

The signed executable remains inside the existing `.tar.gz`; Quill submits
the executable to Apple without changing the distribution format. Raw Mach-O
executables cannot carry stapled tickets, so the first Gatekeeper assessment
may require internet access. A future offline installation path would need a
staple-capable container such as a PKG. `go install` remains a fallback,
derives its version from Go build information, and produces a local executable
outside the project's signing and notarization pipeline.

## 8. Upload Lifecycle

1. Select document or collection mode and validate mode-specific flags before
   config-dependent storage work.
2. Render a document in memory, or preflight every open collection file and
   render its overview, before generating a random directory.
3. Resolve repository context, build the complete normalized v3 object set, and
   encode the kind-specific marker.
4. Put the marker first. Documents then put optional source and page;
   collections stream files in argument order and put `index.html` last.
5. Print no URL until all declared PUTs succeed. Then emit the document URL or
   ordered direct collection URLs plus overview, and best-effort append one v3
   manifest record.
6. Discover both marker names through one LIST snapshot. The basename supplies
   only a kind hint; `show` validates content and every declared size.
7. Targeted get/delete resolve exactly one marker without LIST. Get streams one
   declared object. Delete removes payloads and extras, then the marker, then
   appends a tombstone.
8. Sync complete normalized uploads into compact local history. Confirm every
   LIST-absent active marker with a targeted GET before local tombstoning.

Manifest reads retain pre-marker upload records as read-only legacy history.
Show, get, and delete share profile inference: exactly one requested URL or key
match in active, marker-managed history may select its recorded named profile
before config resolution. URL matches require the recorded public host;
explicit flag or environment profile selection remains authoritative. A typed
profile-mismatch error lets delete add a targeted retry hint when marker lookup
fails. Local list filters by recorded profile only when `--profile` was passed;
local purge instead filters candidates by the fully resolved active profile
before applying its user-supplied age and slug filters.

This ordering intentionally exposes interrupted creation as incomplete and
removes a directory from airplan's management surface only after payload
deletion has succeeded.

## 9. Testing Strategy

- Unit: config/template precedence, mode selection, collection filename/MIME
  preflight, size limits, slug sanitization, format sniffing, key
  entropy/encoding properties, URL assembly, strict v1-v3 marker validation,
  dual-marker resolution, LIST-only kind grouping, inspection states and exact
  sizes, streaming get selection, delete request ordering, manifest reduction
  and lock cancellation, sync reconciliation, and document-only slug filters.
- Golden files: markdown fixtures → rendered HTML snapshots
  (`testdata/`, `GOLDEN_UPDATE=1` convention).
- Browser: Chromium collection fixtures cover image/video/audio and generic
  cards, direct and copy links, no-JavaScript behavior, hostile-looking names,
  narrow/wide layouts, and light/dark themes for built-in and custom templates.
  Computed-style checks enforce shared toolbar geometry and transition-free
  theme changes across built-in document and collection pages.
- Integration: MinIO in a container (CI service / testcontainers);
  document and mixed collection round trips, byte/header preservation,
  marker bytes, remote kind discovery and conflicts, complete / incomplete /
  invalid inspection states, large streaming fetches, markerless invisibility,
  invalid delete rejection, cross-manifest sync, confirmed-absence tombstones,
  restoration, and successful marker-last deletion. The image release
  tag and multi-platform digest are immutable-pinned together in
  `airplan/integration_test.go`.
- Smoke (manual or tagged, needs creds): real R2 upload via a
  scoped token, fetched through the custom domain. Collection smoke coverage
  includes an image and short recording, external image embedding, video seek,
  copied absolute URLs, direct-member management, and whole-upload deletion.

## 10. Operation Transports, REST, and MCP

### Operation facade

`Client` is a stable public facade over an internal operation transport.
Product backend names describe where the same operation service runs:

```text
Client
├── backend=s3      → local transport → operation service → S3 + manifest
└── backend=airplan → HTTP transport  → REST adapter ──────┘
```

The operation boundary is upload, inspect, get, delete, manifest list, storage
list, purge planning/execution, and sync—not S3 object primitives. The local
transport calls the S3-backed service in memory. The HTTP transport wraps the
generated OpenAPI client and maps wire results and problems back to public
Airplan types. REST handlers and hosted MCP receive the same service directly;
they do not make loopback HTTP requests or reimplement selection/deletion
logic. Their upload adapters translate omitted repository context to `none`,
reject `auto`, and normalize explicit repository URLs before core rendering so
hosted requests never initiate Git discovery against server-local paths.

Local service construction resolves profile identity and manifest state before
it creates storage. Storage is initialized lazily at the start of each
S3-dependent operation. This preserves config-free manifest list and manifest
purge preview, and keeps credential failure before input consumption or state
mutation. `serve` calls the explicit readiness check during startup so a
long-running service fails before listening.

The service owns its manifest. A local S3 client and `serve` use the same
platform default or global override and coordinate appends with the existing
cross-platform file lock. HTTP clients do not write a second local manifest.
Service-scope list and purge operations filter the shared file by resolved
profile, bucket, and key prefix; the direct local all-profile list remains a
separate scope.

### OpenAPI and REST adapter

`api/openapi.yaml` is OpenAPI 3.0.3 and is embedded byte-for-byte for
`GET /openapi.yaml`. `oapi-codegen` produces committed models, client methods,
and strict server interfaces in `internal/httpapi/generated`. The generator is
invoked by `go generate` and checked by the repository generated-file gate.
Authentication and request validation are explicit layers because generated
strict handlers do not enforce either policy by themselves.

The OpenAPI validator checks every route. For the two multipart upload routes,
only generic request-body validation is disabled because kin-openapi would
buffer the complete body; method, path, authentication, and media type remain
validated before the generated strict handler. The bounded streaming adapter
then validates part names/counts, JSON metadata shape and enums, filenames, and
all requested/server size limits while spooling.

The REST adapter:

- accepts one static bearer token, compares fixed-size digests in constant
  time, and rejects authentication before body parsing;
- maps typed failures to RFC 9457 problems with stable codes and request IDs;
- replaces internal warning and per-item error detail with stable hosted
  messages before serialization;
- bounds total bodies, multipart parts, per-file bytes, and aggregate bytes;
- streams document uploads through multipart readers and object downloads
  through response writers;
- spools collection members to temporary files, mode 0600 where POSIX
  permission bits exist, because the core collection API needs exact sizes and
  seekable readers;
- removes all spooled files on completion, failure, cancellation, or shutdown;
- resolves request targets against server-side storage configuration so the
  HTTP client never reconstructs capability keys with incomplete knowledge;
- exposes manifest and storage listing as distinct generated operations; and
- implements purge as a stateless preview followed by explicit upload-ID
  execution with fresh ownership-marker validation.

`airplan serve` constructs one operation service, readiness-checks storage,
mounts REST and hosted MCP on one `net/http.Server`, and handles SIGINT/SIGTERM
with bounded graceful shutdown. It sets header, idle, and header-size limits
but no short whole-request write timeout that would truncate large transfers.
The process is deliberately single-instance and relies on persistent
file-backed manifest state.

Serve-only observability uses `log/slog` with one text logger on stderr.
Request-ID and completion middleware wraps both transports; route names are
allowlisted before logging so an unmatched URL cannot disclose a capability
key. Bearer validation returns a closed set of safe rejection reasons after
fixed-size digest comparison. REST and MCP use the same validator and generic
wire response.

The MCP SDK receives a logger through both `ServerOptions` and
`StreamableHTTPOptions`, but an adapter discards SDK messages and attributes
that could contain protocol data and emits only fixed lifecycle categories.
Receiving middleware records allowlisted protocol methods and registered tool
names without inspecting arguments or results. Hosted errors retain their
underlying Go cause in a private wrapper for classification while exposing
only the existing sanitized text to the SDK and client. The stdio constructor
does not install this serve logger, preserving stdout protocol purity.

### HTTP transport

The HTTP transport is selected only by `backend = "airplan"`. It never loads
the AWS credential chain. Upload bodies are produced with `io.Pipe` and
`multipart.Writer`, and get responses stream into the caller's writer. It adds
the configured bearer token to every authenticated request, does not retry
ambiguous upload POSTs, and converts problem codes into typed public failures.
Partial sync and purge results survive error mapping so the CLI preserves its
existing output and exit behavior.

Global `--manifest` resolution happens before client construction. Local S3
and server construction receive that path; the HTTP transport rejects an
explicit flag and ignores `AIRPLAN_MANIFEST`. Client-supplied filesystem
templates and other server-owned local policy overrides are likewise rejected
before input is opened.

### MCP adapters

`github.com/modelcontextprotocol/go-sdk` provides both transports.
`airplan mcp` uses `StdioTransport` and builds the normal client,
so its selected backend may be local S3 or HTTP. Protocol frames are the only
stdout output. The server uses the SDK Streamable HTTP handler at `/mcp` and
passes the operation service directly.

Tool registration and handlers are shared. HTTP omits `upload_files` because
server-local paths are not a portable file-transfer mechanism; stdio includes
it because client and tool process share a filesystem. Tool result structs
provide the generated JSON Schemas and keep warnings inside structured output.
Partial sync and purge errors set `IsError` without returning a Go handler error
so the SDK retains the structured progress result. Sync defaults to dry-run,
purge preview has no mutation path, and purge execution accepts only explicit
upload IDs. Each handler derives a fresh configured-timeout context from the
long-lived MCP session context.

Hosted MCP is wrapped by the REST bearer middleware. A dedicated Origin
verifier rejects every present Origin outside the configured allowlist and
allows absent Origin for non-browser clients. The default allowlist is empty.
The endpoint bounds POST bodies before the SDK's stateless body inspection. It
uses current Streamable HTTP only; no legacy HTTP+SSE adapter or OAuth
token-issuance implementation is installed.

### Additional test layers

- Generated contract tests ensure checked-in Go output matches OpenAPI and
  exercise each endpoint through the generated client.
- Operation contract tests run the same lifecycle against local and HTTP
  transports, including partial failures and cancellation.
- Server tests cover auth parsing, token redaction, Origin policy, request
  limits, multipart cleanup, request IDs, and scoped manifest visibility.
- MCP tests connect with the official SDK over stdio and Streamable HTTP,
  verify the transport-specific tool inventory, and keep stdio protocol-only.
- MinIO integration starts a real server, proves direct/HTTP parity and shared
  manifest persistence, restarts the server, and exercises sync and purge.
