# airplan — Implementation Notes

How _our_ implementation of [SPEC.md](SPEC.md) is built: language,
dependencies, code structure, repo deliverables, phasing, and
testing. Behavior is defined exclusively by the spec; nothing here
may contradict it. Targets spec version 0.44.0.

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
- **Node/Bun/Python as the application runtime**: runtime dependency or
  heavyweight bundles; fails the "single static binary, fast startup"
  constraint. Bun and Node are development-only tools and are not shipped.

**Web build tooling.** Bun owns package installation, the lockfile, TypeScript
and CSS builds, and package scripts. Maintained browser assets and Playwright
tests are strict TypeScript under `web/` and `tests/browser/`. TypeScript 7,
Oxlint with its type-aware backend, Oxfmt, and Stylelint are package-local and
pinned by `bun.lock`. Bun emits deterministic readable and minified browser
assets; only the generated JavaScript and CSS under
`airplan/assets/generated/` are embedded in the Go binary. Node remains
available solely as Playwright's supported runtime. `bunfig.toml` applies a
seven-day minimum release age to every direct and transitive dependency with
no standing exclusions. Its isolated linker keeps transitive package contents
under `node_modules/.bun/`, outside Go's `./...` package discovery boundary.

Production JavaScript applies Bun's syntax and identifier minification while
retaining printer whitespace. This keeps unexpected Go-template delimiters
visible to a fail-closed byte check instead of rewriting executable source.
CSS may separate adjacent structural braces, but a token-aware pass preserves
strings, comments, and escapes so delimiter-like authored content is rejected.

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
                        rendering, document-bundle graph validation,
                        link rewriting, ownership markers, key/slug
                        generation, collection preflight/rendering,
                        streaming S3 upload/get, list/show/delete,
                        manifest history, URL assembly; embeds assets
                        via go:embed; Mermaid is the sole conditional
                        runtime asset
api/openapi.yaml        authoritative REST wire contract (embedded)
api/oapi-codegen.yaml   deterministic generator configuration
internal/httpapi/       generated REST models/client/server plus auth,
                        problem mapping, request safety, and adapters
internal/cmd/preparecontainer/
                        validates GoReleaser metadata and creates the
                        deterministic multi-platform image context
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

document, err := client.UploadDocument(ctx, airplan.DocumentInput{
    Entry: airplan.PageInput{Reader: readme, Path: "README.md"},
    Pages: []airplan.PageInput{{
        Reader: design, Path: "docs/design.md",
    }},
    Assets: []airplan.AssetInput{{
        Reader: diagram, Path: "images/flow.svg", Size: diagramSize,
        ContentType: "image/svg+xml",
    }},
})
// document.Result remains the entry summary.
// document.Pages and document.Assets expose the ordered bundle result.

files, err := client.UploadFiles(ctx, airplan.FilesInput{
    Files: []airplan.FileInput{{
        Name: "demo.webm", Reader: recording, Size: recordingSize,
    }},
})
// files.Files[0].URL is direct; files.URL is the overview.

revision, err := client.CreateDocumentRevision(ctx,
    airplan.CreateDocumentRevisionInput{
        Target: existingURL, Document: replacement,
    })
// UpdateDocument remains the deprecated one-entry compatibility wrapper.

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
- Themes: `ResolveThemeBundleWithOptions` validates the fixed registry plus
  every sorted global custom entry before selecting the configured ordered
  subset. It normalizes hex colors, hashes the selected order and defaults,
  and produces safe semantic CSS plus lightweight browser metadata once per
  resolved config. Mermaid palettes are separate and injected only for pages
  containing diagrams, with GitHub Light retained as an internal print target.
  Go owns the immutable domain, deterministic color mixing, Chroma styles, and
  marker recipe; TypeScript owns reader mode/slot state and interaction only.
- Highlighting: Chroma emits class-based markup. Built-ins select exact
  registered styles and custom themes either select `chroma:<name>` or build a
  semantic style with `chroma.NewStyleBuilder`. Generated rules are compacted,
  and themes with byte-identical Chroma output share declaration blocks through
  grouped selectors. Every selector remains scoped to one exact theme ID, with
  a separate fixed GitHub print stylesheet.
- Mermaid: a stateless Goldmark node renderer intercepts only exact
  `mermaid` fences ahead of Chroma and emits escaped source containers. The
  built-in template bakes a document-specific Mermaid module into its
  conditional script, imports the generated exact module URL, initializes
  Mermaid's `base` theme with explicit variables, and serially renders only the
  two assigned slot themes plus fixed GitHub Light print output at startup.
  Theme-ID caches, a latest-request guard, and per-theme failures make lazy
  switching race-safe while the synchronous print path uses prepared output.
  After successful rendering that
  module creates one native dialog viewer and enhances each rendered block
  with an explicit open control. The viewer clones the displayed SVG, rewrites
  every ID and local reference (including embedded styles and ARIA IDREFs), and
  keeps an open clone synchronized with theme swaps while retaining its
  transform. The pin manifest under `internal/deps` generates exported
  constants; a networked updater observes a 72-hour minimum age, stays within
  the current major, verifies jsDelivr, and refreshes generated/rendered
  artifacts. Dependency-only updates do not alter this document or SPEC.md.
- Built-in document and collection templates share authored TypeScript and CSS
  for base styling, early theme selection, runtime theme behavior, and
  theme-control markup. `web/build.ts` uses explicit Bun browser targets and
  formats to generate readable and minified page-specific bundles. A semantic
  manifest selects build-time `@primer/octicons` inputs. The same generator
  emits Go template definitions and named TypeScript constants, so Bun drops
  icons unused by each browser bundle while authored templates and scripts
  contain no copied SVG markup. It replaces exactly one inert Mermaid URL
  sentinel with the existing Go template action, rejects template delimiters
  from all other generated content, and produces no runtime network dependency
  beyond conditional Mermaid loading. A generalized bake step expands readable assets for
  `airplan template [document|collection]` and minified assets for executable
  built-ins. Both commands therefore emit complete, standalone, reusable
  templates without exposing internal bake markers.
- Built-in multi-page navigation remains server-rendered. Rendering derives a
  directory tree from the ordered page list while preserving that flat list for
  previous/next sequencing. Directories are static visual groups, and every page
  item is a normal anchor to a standalone document. CSS opts same-origin
  navigations into
  cross-document View Transitions under the no-reduced-motion media query;
  there is no click interception, fetch/replace controller, or client-side
  history. Each destination runs the existing one-shot initializer. Linked
  pages add page-local diff projection, entry-only complete reports, and
  marker-validated same-logical-page revision selection. Collapsed Pages uses
  a native top-anchored popover while Contents retains its bottom-sheet dialog;
  the document toolbar becomes sticky when the rails collapse, with Pages
  pinned left. Wide rail headings sit outside their scrollable lists, and the
  complete-diff view moves its document/raw links above the report while hiding
  compact Pages navigation. This page structure uses
  `RendererGeneration` 14.
- Document templates: Go `html/template`. Canonical template data exposes the
  raw source string, rendered and highlighted `template.HTML`, Chroma's
  `template.CSS`, safe theme CSS/catalog metadata, structured headings/ToC
  entries, format metadata,
  title, slug, indexing intent, frontmatter, repository context, source
  names/paths, ordered bundle pages, directory-grouped page navigation,
  current-page identity, entrypoint, and assets. Document-specific CSS and JS
  cover the page grid, Pages and On this
  page rails, narrow navigation dialog, previous/next controls, prose, source
  view, table of contents, copy controls, and Mermaid integration. Custom
  templates receive the additive bundle data but no injected navigation or
  transition assets.
- Document bundles: `bundle.go` owns the public `PageInput`, `AssetInput`,
  `DocumentInput`, result, and rendering types plus portable logical-path and
  generated-object collision checks. One preflight resolves every format and
  page name before rendering. A Goldmark AST transformer rewrites only ordinary
  links whose normalized target exactly matches the managed-source map. Relative
  URLs are calculated from each rendered page, so local output-directory
  previews and hosted pages use the same navigation model.
- Collection rendering uses a separate embedded `html/template` and stable
  `CollectionTemplateData` / `CollectionTemplateFile` surface. Preflight
  validates names and limits, resolves deterministic MIME/media kinds, creates
  already-escaped relative paths, and executes only the applicable collection
  template. The built-in template provides collection-specific responsive
  image, video, audio, and generic-file presentation, links image previews to
  their direct members, and uses the document toolbar's canonical shared
  geometry and interaction styling. Custom document and collection templates
  remain independently configurable.
- Local rendering: `RenderDocument` owns bundle path, count, aggregate-source,
  and aggregate-generated-HTML limits, renders every page against one page map,
  and returns `RenderedDocumentBundle`. `MaterializeDocument` and the direct
  upload, revision, and upgrade paths use one private mode-0600 payload spool;
  output-directory preview publishes from that spool through a private sibling
  staging directory. Platform-specific no-replace rename primitives publish
  that mode-0755 root only when the destination is still absent; published files
  are mode 0644. Filesystems that do not support the required no-replace rename
  operation reject publication rather than using a check-then-rename fallback.
  Both temporary layers are removed on success or failure, so the
  full allowed bundle need not remain in memory.
  Upgrade planning itself streams remote page and source bodies through bounded
  hashing and retains only size, digest, and ETag metadata. Execution replans,
  streams the exact generation into its private spool, and re-hashes sources
  immediately before the first conditional mutation.
  `RenderInput` remains the one-entry wrapper and owns read limits, binary and invalid
  UTF-8 rejection,
  format detection, title/slug resolution, template execution, and
  noindex handling. `RenderCollection` owns the equivalent collection
  preflight and local overview rendering. Explicit HTML is tokenized with
  `x/net/html`; raw token
  lengths locate the original head boundary while normalized tokens identify
  in-head robots metadata. Injection splices only the original byte slice and
  never serializes the token stream. `Client.UploadDocument` adds source, asset,
  and page storage, URLs, and manifest recording; `Client.Upload` wraps it.
  `airplan preview` stops after the applicable render or materialization path.
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
- Ownership markers: writers emit one v6 schema for all uploads. Documents use
  `.airplan.json`; collections use `.airplan-collection.json`. `kind` plus a
  normalized declared-object array describes pages, sources, assets, diffs, and
  files; document slug/format fields remain conditional. V6 adds the entrypoint
  and ordered managed-page descriptors, permits validated nested object paths,
  requires exact SHA-256 for every payload, and raises the marker ceiling to
  256 KiB. Every v4+ marker records
  producer provenance. Generated document and collection pages additionally
  record the renderer generation and either the built-in template identity or
  a SHA-256 digest of the custom template; the digest is computed from the same
  single file read that is parsed for rendering. Version 5 and 6 recipes also
  record the two default IDs and canonical catalog SHA-256; the full custom
  source is intentionally not duplicated. Every v4/v5 page object records the SHA-256 of
  its exact uploaded bytes; v6 records it for all payloads. Authored HTML omits
  render provenance. Version-specific decoding normalizes v1-v3 into the
  internal object model and preserves v4/v5 provenance; the version bump makes
  older readers reject v6 before rewriting it. Authored object path segments
  beginning `.airplan-` are
  reserved for Airplan control objects and rejected before upload. A centralized
  concurrent two-key resolver proves exactly one marker exists without adding
  LIST permission to targeted reads. Kind/name mismatches and dual markers
  fail closed. Marker-first upload and marker-last deletion preserve the
  remote ownership boundary.
- Built-in Markdown pages contain a dormant revision-discovery bootstrap. It
  fetches `.airplan-versions.json` relative to the page with `no-store` cache
  semantics and a fresh per-load query nonce. A 404 is the expected standalone
  case. Ordinary uploads therefore gain revision readiness without uploading a
  versions object; revision linkage can add that object later without replacing
  the page.
- `versions.go`, `diff.go`, and the revision operation implement linked
  Markdown history. `CreateDocumentRevision` is the bundle-aware public entry;
  the CLI, REST, and MCP update aliases delegate to it, while the deprecated Go
  `UpdateDocument` method retains its established one-entry engine. A canonical
  revision input is always the complete replacement bundle. Both engines share
  one prerequisite state machine for upgrades, interrupted promotion and page
  repair, latest traversal, and chain validation. Descriptor order, metadata,
  and v6 digests drive canonical no-op comparison before large unchanged assets
  are rendered or uploaded when possible.
  Markers carry immutable chain descriptors and diff inventory, while the
  separately versioned 64 KiB metadata index is conditionally replicated to
  each live member. Capacity is preflighted before candidate upload and exposed
  as `ErrRevisionHistoryFull`. The old latest metadata write is the append
  serialization point; ambiguous write responses are reconciled from both the
  serialization object and candidate replica. Existing-chain rollback first
  takes the same ETag cleanup claim as orphan collection, while first-link
  reconciliation
  idempotently completes the conditional creation. Delete reservations rebase
  from live survivors after interrupted operations, repairing missing member
  replicas and first-promotion state, while final-member deletion uses an
  invalid transition reservation to exclude new and stale appenders. The
  deleted member's control body remains as a markerless durable receipt for
  marker-last recovery, including when local history missed the first-link
  projection and must recover chain identity from the receipt. Adjacent exact
  source bytes are diffed with pure-Go go-difflib into one structured canonical
  report. Its versioned envelope uses explicit JSON-encoded page and asset
  section paths, avoiding collisions between logical paths and Airplan-owned
  bundle headings. Deterministic bundle-level, page-descriptor, page-source,
  order, and asset sections serialize the immutable stored bytes; the in-memory
  model projects one page-local section into each changed page and the complete
  report into the entry only. Chroma highlights each eligible projection once.
  Upgrade parses that envelope strictly before mutation, binds nested unified
  headers to the section path and envelope revisions, accepts the earlier
  unversioned structured form, and maps legacy one-page diffs wholly to the
  entry. Binary asset changes remain deterministic metadata summaries.
  Replicated revision indexes continue to store entry URLs only; the browser
  resolves child targets by fetching and validating the selected revision's v6
  marker, with its validated entry URL as the failure fallback.
- Collection storage: `UploadFiles` accepts known-size `io.ReadSeeker` members,
  hashes each member during preflight, rewinds it, hashes the transmitted bytes
  again, and fails before publishing the overview when a caller changes a
  reader. It limits each reader to its declaration and uploads the overview
  last. `GetUploadTo`
  streams large payloads to CLI stdout or atomic file output; the older
  `GetUpload` wrapper remains available for in-memory callers.
- Remote discovery: `ListRemote` recognizes both exact marker names, exposes
  their untrusted kind hint, selects exact collection `index.html`, and marks
  dual-name groups as conflicts without fetching markers or heading payloads.
  Its LIST snapshot retains per-key sizes for batch inspection.
  `InspectUpload` validates the selected marker and exact sizes for every
  normalized declared object, then downloads and hashes v6 payloads against
  their marker digests. LIST-backed inspection used by sync and purge, plus
  purge execution's revision guard, stays existence-and-size based and never
  expands into payload downloads. Targeted get and delete probe both markers
  and authorize only pages, document sources, collection files, or the existing
  marker.
- Manifest sync: `SyncManifest` reduces local history chronologically, compares
  the scoped active view to one remote LIST snapshot, and uses a shared bounded
  worker pool for marker GETs and targeted absence confirmation. Imports and
  tombstones are sorted, then the manifest is locked, reread, and rechecked
  before whole-line appends. Definite object-not-found is the only pruning
  signal; failures retain local state and return partial progress. Active v4
  Markdown records without revision identity are re-inspected so a promotion by
  another writer appends a revision-aware `link` projection.
- Remote deletion: the marker must decode and authorize the supplied direct
  target. Payload objects are removed with batched `DeleteObjects`, then the
  marker is removed in a separate final `DeleteObject`. Invalid and markerless
  directories are outside airplan's remote management authority.
- `--older-than` durations: small custom parser for `d`/`w` units —
  Go's stdlib `time.ParseDuration` has no days. `ParseTimeFilter` first checks
  strict zoned RFC 3339 syntax, including fraction and numeric-offset ranges,
  then tries exact offset-less layouts in the caller's local zone; it rejects
  the fractional-second extensions Go accepts for layouts that omit them.
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
  to use Airplan for explicitly requested document, document-bundle, or file
  sharing and for visual
  evidence explicitly called for by authorized PR or issue work. It reviews
  captures for sensitive material, uploads related evidence in one JSON-mode
  collection, distinguishes bundle pages/assets from peer-file collections,
  selects inline versus local-file MCP tools by payload shape and size,
  distinguishes `files[].url` from the overview `url`, and never
  invents or reuses partial results. It briefly identifies optional Markdown
  rendering features without encouraging decorative overuse, and still
  prohibits opportunistic uploads. This file remains the single canonical
  source. `skills/embed.go`
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

The official `ghcr.io/jimeh/airplan` image reuses the exact GoReleaser Linux
executables rather than compiling in Docker. After release-asset verification,
`preparecontainer` reads `dist/artifacts.json`, requires one amd64 and one
arm64 binary for the Airplan build, validates their ELF machine types, and
compares their bytes with the matching release archives. It normalizes the
paths to `linux/$GOARCH/airplan` in a small, seven-day workflow artifact.

The release Dockerfile selects that normalized binary with
`TARGETPLATFORM`, copies it into a digest-pinned distroless Debian 13 non-root
base, and contains no `RUN` instruction. Buildx therefore resolves both base
manifests and copies both static executables without QEMU or target-platform
execution. The executable resolves server environment fallbacks directly;
there is no shell entrypoint. `XDG_CONFIG_HOME=/etc` makes
`/etc/airplan/config.toml` an optional default without forcing an explicit
config path. The manifest is explicitly placed under the owned, declared
`/var/lib/airplan` volume.

Container publication is a separate job downstream of immutable GitHub
release verification. It restores the prevalidated context and pushes the
amd64/arm64 index canonically by digest without a user-facing tag. It verifies
the runnable platform set, exact child-image binary bytes against the preserved
GoReleaser context, SBOM attachment, OCI labels, version, numeric non-root
runtime, shell-free configuration, writable temp and state paths,
environment-only and file-based server startup, MinIO upload, graceful
shutdown, and manifest persistence. GitHub provides the one authoritative
provenance attestation because Buildx provenance is disabled; verification
requires this repository's release workflow as its signer. Buildx still
generates the image SBOM. A retry that reuses an existing exact-version digest
verifies its attestation but does not generate a duplicate.

Only after verification does the workflow assign the unprefixed exact version
tag and, after querying the latest GitHub release at the mutation boundary,
possibly `latest`. An existing exact tag is trusted only after its provenance,
release identity, platforms, and runtime all validate; a conflicting digest is
never overwritten by the workflow. A constant package-scoped job concurrency
group with `queue: max` serializes workflow-owned GHCR mutations across release
versions without replacing queued publications. GHCR does not enforce
immutable tags against external writers, so digest references remain the
immutable deployment boundary. Failures after GitHub release publication
create one deduplicated issue and are independently rerunnable. The first
package publication requires the accepted one-time manual change to public
visibility.

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
2. Preflight the complete document graph or every open collection file before
   generating a random directory. Hash and spool managed sources, validate and
   rewind assets, resolve generated names, then render and spool every managed
   page with separate aggregate limits.
3. Resolve repository context, build the complete normalized v6 object set with
   a SHA-256 for every payload plus producer and applicable render provenance,
   including the resolved theme recipe, and encode the kind-specific marker.
4. Put the marker first. Documents put sources and assets, non-entry pages, and
   the entry page last. Collections stream files in argument order and put
   `index.html` last. Hash seekable payloads again while transmitting and reject
   a mismatch before publishing the entry or overview. A bounded worker pool
   uploads independent objects; the entry and collection overview remain final
   barriers.
5. Print no URL until all declared PUTs succeed. Then emit the document URL or
   ordered direct collection URLs plus overview, and best-effort append one v6
   manifest record.
6. Discover both marker names through one LIST snapshot. The basename supplies
   only a kind hint; `show` validates content and every declared size.
7. Targeted get/delete resolve exactly one marker without LIST. Get streams one
   declared object. Delete refuses purge-protected uploads unless forced,
   removes payloads and extras, then the sentinel when present, then the
   marker, then appends a tombstone. Protect/unprotect resolve the marker the
   same way, then PUT or DELETE the sentinel and append the matching manifest
   record.
8. Upgrade every source-backed managed page with marker-first/entry-last
   conditional writes. Planning records the observed marker and all page/source
   entity tags. Execution replans live state, claims one desired v6 marker,
   skips digest-matching pages, repairs non-entry mismatches, verifies them, and
   repairs the entry last. A failed run may leave a marker-owned mixed renderer
   generation; inspection reports mismatches and retry converges without
   rewriting matching pages or the already-desired marker. Success appends an
   `upgrade` manifest event while leaving source, asset, protection, and revision
   control objects untouched.
9. Sync complete normalized uploads into compact local history. Confirm every
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

- Unit: config/template precedence, mode selection, bundle logical paths,
  generated-page naming and collisions, managed-link rewriting, document item
  and aggregate limits, collection filename/MIME preflight, slug sanitization,
  format sniffing, key entropy/encoding properties, URL assembly, strict
  version-specific marker validation through v6, complete payload digests,
  dual-marker resolution, LIST-only kind grouping, inspection states and exact
  sizes, streaming get selection, delete request ordering, purge-protection
  sentinel detection and forced-delete ordering, manifest reduction
  and lock cancellation, sync reconciliation, and document-only slug filters.
- Golden files: single and multi-page Markdown fixtures → rendered HTML
  snapshots plus v6 simple document, bundle, HTML-with-assets, no-source,
  collection, and revision markers
  (`testdata/`, `GOLDEN_UPDATE=1` convention).
- Web: strict TypeScript compiler projects cover browser and Playwright code;
  Oxlint runs ordinary and type-aware rules; Stylelint checks authored CSS;
  Bun unit tests cover pure tooling policy; and generation checks rebuild into
  a temporary directory and byte-compare the committed asset set.
- Browser: Chromium document-bundle fixtures cover page rails, active state,
  native full-document lifecycle, ordinary and no-JavaScript navigation,
  back/forward/fragments, narrow fallback navigation, reduced-motion transition
  opt-out, destination Mermaid initialization, source/changes controls, and
  printing. Collection fixtures cover image/video/audio and generic
  cards, direct and copy links, no-JavaScript behavior, hostile-looking names,
  narrow/wide layouts, all eleven built-ins, arbitrary cross-variant slot
  assignments, custom/no-JavaScript themes, storage migration, Mermaid
  cache/race/failure behavior, and fixed printing. Computed-style checks
  enforce shared toolbar geometry and transition-free theme changes across
  built-in document and collection pages.
- Integration: MinIO in a container (CI service / testcontainers);
  document-bundle and mixed collection round trips, byte/header preservation,
  native checksum comparison where supported, changed-reader rejection,
  marker bytes, remote kind discovery and conflicts, complete / incomplete /
  invalid inspection states, large streaming fetches, markerless invisibility,
  invalid delete rejection, cross-manifest sync, confirmed-absence tombstones,
  restoration, and successful marker-last deletion. The image release
  tag and multi-platform digest are immutable-pinned together in
  `airplan/integration_test.go`.
- Container: GoReleaser artifact-selection and ELF fixtures are unit tested.
  Docker CI builds both Linux platforms without QEMU, loads the native image,
  checks its runtime configuration, and runs environment-only and mounted-file
  MinIO server lifecycles across container replacement and persistent state.
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

The operation boundary is document and collection upload, document revision
creation, inspect, get, delete, protect/unprotect,
manifest list, storage
list, purge planning/execution, document-upgrade planning/execution, bulk
upgrade preview/execution, and sync—not S3 object primitives. The local
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
profile, bucket, and key prefix. Direct local list filters by resolved profile
unless `--all-profiles` requests the separate all-history scope.
Bulk upgrade follows the same service scope. CLI `--all-profiles` explicitly
constructs one client per configured profile, merges plans by bucket and marker
key, then routes each selected item back to its owning profile; it is never an
implicit expansion performed by one client or by the server. Profile inventory
is independent of the active selector/default, and mixed S3/hosted inventories
are rejected before any remote mutation. Its planning timeout ends before
confirmation and execution receives a fresh timeout context afterward.

Upgrade planning compares reproducible v4+ template recipes without parsing or
rendering. V5 and v6 additionally compare theme identity. A forced template or theme
mismatch records the currently configured recipe;
execution surfaces deferred template load/parse errors before its marker-first
write. Raw manifest upgrade events retain their append time, while reduction
projects required `created_at` into the active record's time so list and purge
age semantics remain tied to original creation.

### OpenAPI and REST adapter

`api/openapi.yaml` is OpenAPI 3.0.3 and is embedded byte-for-byte for
`GET /openapi.yaml`. `oapi-codegen` produces committed models, client methods,
and strict server interfaces in `internal/httpapi/generated`. The generator is
invoked by `go generate` and checked by the repository generated-file gate.
Authentication and request validation are explicit layers because generated
strict handlers do not enforce either policy by themselves.

The OpenAPI validator checks every route. For multipart upload and revision routes,
only generic request-body validation is disabled because kin-openapi would
buffer the complete body; method, path, authentication, and media type remain
validated before the generated strict handler. The bounded streaming adapter
then validates ordered metadata/document/pages/assets parts, names/counts, JSON
metadata shape and enums, logical paths, and all requested/server size limits
while spooling.

The REST adapter:

- accepts one static bearer token, compares fixed-size digests in constant
  time, and rejects authentication before body parsing;
- maps typed failures to RFC 9457 problems with stable codes, generic
  code-selected detail, and request IDs;
- replaces internal warning and per-item error detail with stable hosted
  messages before serialization;
- bounds total bodies, the 256 KiB metadata part, multipart parts, per-page and
  per-asset bytes, aggregate source/HTML/asset bytes, and complete requests;
- streams document uploads and revisions through multipart readers and object downloads
  through response writers;
- spools managed pages, assets, and collection members to temporary files, mode
  0600 where POSIX permission bits exist, because core asset and collection APIs
  need exact sizes and seekable readers;
- removes all spooled files on completion, failure, cancellation, or shutdown;
- resolves request targets against server-side storage configuration so the
  HTTP client never reconstructs capability keys with incomplete knowledge;
- exposes manifest and storage listing as distinct generated operations; and
- implements purge as a stateless preview followed by explicit upload-ID
  execution with fresh ownership-marker validation; and
- implements upgrade as an entity-tag-bound plan followed by marker-first and
  entry-last conditional execution, plus a bulk preview whose apply request must
  contain the exact reviewed plan items.

The canonical `/api/v1/uploads/documents/revisions` operation and deprecated
`/api/v1/uploads/documents/update` operation use the same generated canonical
schemas and one adapter. Frozen legacy metadata retains `max_size`; the
canonical model adds `max_page_size`, aggregate page, asset, and total limits,
and ordered descriptors. Conflicting old and new per-page limit spellings fail.
Capabilities advertise bundle support, canonical revision-route support, every
logical limit, the metadata limit, and the complete request envelope.

`airplan serve` constructs one operation service, readiness-checks storage,
mounts REST and hosted MCP on one `net/http.Server`, and handles SIGINT/SIGTERM
with bounded graceful shutdown. It sets header, idle, and header-size limits
but no short whole-request write timeout that would truncate large transfers.
The process is deliberately single-instance and relies on persistent
file-backed manifest state.

Serve-only observability uses `log/slog` with one text logger on stderr.
Request-ID and completion middleware wraps both transports. IDs are generated
by the server, reused by nested middleware, and never accepted from request
headers. Route names and methods are allowlisted before logging so unmatched or
unexpected request metadata cannot disclose a capability key or another
client-controlled value. Bearer validation returns a closed set of safe
rejection reasons after fixed-size digest comparison. REST and MCP use the same
validator and generic wire response.

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
Partial sync, purge, and bulk-upgrade results survive error mapping so the CLI
preserves its existing output and exit behavior.

Global `--manifest` resolution happens before client construction. Local S3
and server construction receive that path; the HTTP transport rejects an
explicit flag and ignores `AIRPLAN_MANIFEST`. Client-supplied filesystem
templates and other server-owned local policy overrides are likewise rejected
before input is opened.

Before streaming a bundle, the transport requires the advertised bundle
capability. Before a revision, it chooses the canonical route only when the
server advertises it; otherwise an existing one-entry request uses the legacy
route. It never probes with a body and retries after 404 because input streams
are not generally replayable.

### MCP adapters

`github.com/modelcontextprotocol/go-sdk` provides both transports.
`airplan mcp` uses `StdioTransport` and builds the normal client,
so its selected backend may be local S3 or HTTP. Protocol frames are the only
stdout output. The server uses the SDK Streamable HTTP handler at `/mcp` and
passes the operation service directly.

Tool registration and handlers are shared. Inline `upload_document`,
`new_document_revision`, and compatibility `update_document` accept UTF-8 page
content plus strict-base64 assets. The compatibility and canonical revision
tools share one schema and handler. Decoding enforces a 32 MiB aggregate asset
limit independently of the Streamable HTTP body ceiling.

With `LocalFiles`, stdio also registers `upload_document_files`,
`new_document_revision_files`, and collection `upload_files` because client and
tool process share a filesystem. HTTP omits all local-file tools because
server-local paths are not a portable file-transfer mechanism. Tool result structs
provide the generated JSON Schemas and keep warnings inside structured output.
Partial sync, purge, and bulk-upgrade errors set `IsError` without returning a
Go handler error so the SDK retains the structured progress result. Sync defaults to dry-run,
purge preview has no mutation path, and purge execution accepts only explicit
upload IDs. Document and bulk upgrade tools likewise preview by default;
mutation requires `apply: true`, and bulk mutation accepts only the exact
previewed plan items. Each handler derives a fresh configured-timeout context from the
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
