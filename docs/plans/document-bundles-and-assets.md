# Document Bundles, Assets, and Multi-Page Navigation Implementation Plan

Status: proposed

Scope: document uploads containing an entry document, supporting assets, and
additional Markdown or source-code pages across the Go library, CLI, REST API,
MCP, local preview, management operations, revisions, upgrades, and built-in
page UI

Repository baseline: Airplan v0.10.0, spec 0.40.0

Follow-up: [Revision-aware page navigation and diff UX](revision-aware-navigation-and-diffs.md)
supersedes this plan's entry-only revision switching and bundle-wide per-page
Changes presentation. The storage model, browser-native navigation, and other
bundle decisions remain unchanged.

Later contract: SPEC.md 0.57.0 supersedes this plan's explicit-role-only local
CLI and MCP input decisions. Named file sets now infer document kind, entry,
pages, and assets, while the marker and structured API model remains explicit.

## 1. Goal

Let one Airplan document upload represent a small, self-contained body of work:

- exactly one entry document;
- zero or more additional rendered pages; and
- zero or more supporting assets such as images, SVGs, recordings, stylesheets,
  or downloads.

The simple case remains simple. Uploading one Markdown, text, or HTML file keeps
the current behavior and result URL. A caller opts into the richer model by
explicitly identifying supporting pages and assets. Existing multiple-positional
file invocations remain collections.

For Markdown entry documents, Airplan provides navigation among declared pages.
Built-in pages opt ordinary same-origin navigations into browser-native
cross-document View Transitions. The browser still loads a fresh standalone
HTML document and owns its history, scripts, focus, and scroll behavior. Browsers
without cross-document transition support use the same links as normal page
navigations.

This design calls the richer shape a **document bundle**, but it remains
`kind: "document"`. It is an extension of the document lifecycle, not a third
upload kind alongside documents and collections.

## 2. Product model and terminology

The document domain model gains four explicit concepts:

1. **Document** is the ownership, lifecycle, and sharing unit represented by one
   upload directory and ownership marker.
2. **Entry document** is the page whose URL Airplan prints and opens. Every
   document has exactly one.
3. **Managed page** is a declared Markdown or UTF-8 text/source file rendered by
   Airplan into its own standalone HTML page. The entry is also a managed page
   when its format is Markdown or text.
4. **Asset** is an uploaded byte-for-byte dependency or download that Airplan
   does not render or place in page navigation.

A **document bundle** is a document with at least one non-entry managed page or
asset. The term is useful in APIs and implementation discussion, but it does
not appear as a new marker kind or list filter.

Collections retain their current meaning: unordered peer files with a generated
overview. A collection has no entry source document, page graph, source-aware
link rewriting, revision semantics, or Airplan-managed inter-page navigation.

Do not create a `CONTEXT.md` for these terms until the feature ships. This plan
records proposed language; the implemented domain contract belongs in SPEC.md
and IMPLEMENTATION.md.

## 3. Settled product decisions

1. Keep the two public upload kinds, `document` and `collection`.
2. Preserve current mode selection. Multiple unqualified positional files still
   select collection mode. Pages and assets require explicit role flags or
   structured API fields.
3. The entry document may be Markdown, UTF-8 text, or authored HTML.
4. Additional Airplan-managed pages require a Markdown entry document. Each
   additional page may be Markdown or UTF-8 text/source. Airplan renders source
   code through the current text-document renderer and language detection.
5. An authored HTML entry may declare assets, including other HTML files, but
   Airplan does not render, rewrite, or provide navigation among HTML pages.
   Supplying a managed page with an HTML entry is an error.
6. Assets are opaque bytes. Airplan determines or accepts their content type,
   uploads them unchanged, and does not generate thumbnails, transcode media,
   scan archives, or inspect their internal references.
7. The entry file's directory is the local bundle root for CLI operations.
   Every declared page and asset must resolve beneath it. Recursive directory
   discovery and paths outside that root are out of scope for the first version.
8. All user-visible page and asset paths are explicit, stable, bundle-relative
   paths. Airplan preserves safe nested directory structure rather than
   flattening basenames.
9. A Markdown link is rewritten only when its resolved path exactly identifies
   a declared managed source. External URLs, fragments, undeclared paths, asset
   links, and raw authored HTML attributes are left unchanged.
10. Plain-text CLI output remains the entry URL and nothing else. JSON results
    add structured `pages` and `assets` arrays.
11. Creating a document revision means complete replacement. The caller
    supplies the entire new bundle; omitted pages and assets are removals.
    Airplan does not inherit unspecified objects from the previous revision.
12. The built-in template opts into browser-native cross-document View
    Transitions using CSS. Custom document templates receive additive bundle
    data but no injected markup, CSS, or JavaScript, and therefore use ordinary
    browser navigation unless the custom template opts in itself.
13. Each rendered page remains a full standalone document at a real object URL.
    Browser-native transitions are a progressive visual effect, not a
    single-page application routing contract.
14. Assets are not duplicated in the pages navigation. Authors link or embed
    them from their content; management and inspection surfaces expose the asset
    inventory.
15. All writes use the new marker generation once the feature lands, including
    single-file documents and collections. Optional bundle fields express
    feature use; marker versions do not fork by upload shape.
16. Markdown managed pages replace their final source extension with `.html`.
    Text and source-code pages retain their complete source filename and append
    `.html`, so `main.c` and `main.h` become `main.c.html` and `main.h.html`.
17. The initial document limit is 100 user-supplied items total, including the
    entry, pages, and assets. Do not split it into separate page and asset count
    limits until operational evidence calls for different policy.
18. Built-in multi-page documents include compact previous and next controls
    after the current page content as well as the Pages rail.
19. Inline MCP assets have a 32 MiB decoded aggregate limit. REST and local-file
    MCP tools remain the documented route for larger assets.
20. **Create document revision** is the canonical domain operation. The CLI
    command is `new-revision`; `update` remains a compatibility alias without a
    warning. Public Go, REST, and MCP interfaces gain revision-named entry
    points while their existing update-named interfaces remain compatibility
    wrappers or aliases. This plan does not schedule removal of any compatibility
    name. `upgrade` keeps its current name because it re-renders an upload in
    place without creating a revision.
21. Marker v6 records a SHA-256 digest for every declared payload object,
    including document pages, sources, assets, revision diffs, collection index
    pages, and collection members. Native S3 checksums provide optional
    verification but do not replace the provider-independent marker digest.
22. Marker v6 increases the encoded marker limit from 64 KiB to 256 KiB. The
    100-item document limit remains independent from this byte limit.
23. Managed-page sources have a 100 MiB aggregate limit in addition to the
    existing 10 MiB per-page limit. Generated HTML has a separate 100 MiB
    aggregate ceiling, and upload paths spool page sources and output rather
    than retaining the complete set in memory. Asset bytes retain their separate
    2 GiB aggregate limit.
24. In-place bundle upgrades are resumable rather than failure-atomic. A failed
    attempt may leave a temporary mixture of renderer generations at stable page
    URLs. A retry compares desired and stored page digests, skips matching pages,
    repairs mismatches, and writes the entry page last.

## 4. CLI experience

Add repeatable `--page` and `--asset` flags to upload and `new-revision`
commands:

```console
airplan README.md \
  --page docs/design.md \
  --page examples/server.go \
  --asset images/flow.svg \
  --asset recordings/demo.mp4
```

The first positional input remains the entry document. This invocation is a
document because the supporting roles are explicit. By contrast, the existing
invocation remains a collection:

```console
airplan README.md LICENSE
```

Explicit roles avoid unstable content classification. SVG, CSS, JavaScript,
JSON, and source files can reasonably be either readable pages or assets; file
extensions alone cannot express the author's intent.

### 4.1 Input rules

- `--page` and `--asset` are repeatable and mutually exclusive for any one
  normalized path.
- The entry must be a named regular file when either flag is present. Bundled
  stdin is rejected because it has no unambiguous local root. Existing
  single-document stdin remains supported.
- Symlinks may be followed only when their fully resolved targets remain under
  the fully resolved bundle root. Directory inputs, sockets, devices, and other
  non-regular files are rejected before upload.
- The logical path is the declared file path relative to the entry directory,
  normalized with `/` separators. The entry's logical path is its basename.
- Reject absolute paths, empty segments, `.` or `..` segments, backslashes,
  NULs and control characters, reserved `.airplan-*` segments, duplicate
  normalized paths, and case-folded collisions. The conservative collision rule
  produces portable bundles even when authoring and serving filesystems differ.
- URL construction escapes each path segment independently and preserves `/` as
  hierarchy.
- Preflight validates all paths, formats, sizes, and generated-object
  collisions before the first remote write.

### 4.2 Limits

Retain `--max-size` as the per-managed-page text limit, with the current 10 MiB
default. Add:

- `--max-total-page-size`, default 100 MiB across entry and supporting managed
  source bytes;
- `--max-asset-size`, default 1 GiB per asset; and
- extend the existing `--max-total-size` flag to document mode, with its current
  2 GiB default applied across asset bytes.

The aggregate page limit bounds the source and rendered-page memory retained by
the first implementation. Apply the same 100 MiB ceiling separately to total
generated HTML bytes. Upload and output-directory preview spool validated source
and rendered pages into mode `0600` temporary files, so they do not retain the
whole permitted total in memory. The asset total remains stream-oriented and
matches the collection default. On the CLI, `0` disables a client-side limit,
matching the existing size-flag convention. At the Go library boundary, a
negative value disables that limit. A remote server may still impose a lower
advertised ceiling.

Limit errors identify the logical path and the effective limit. Size checks do
not consume non-seekable readers silently; the public bundle API requires exact
asset sizes and seekable asset readers.

### 4.3 Preview

Keep current one-page preview behavior, including stdout and `-o FILE`. Add
`--output-dir DIR` for any preview containing pages or assets:

```console
airplan preview README.md \
  --page docs/design.md \
  --asset images/flow.svg \
  --output-dir ./preview
```

`--output-dir` and `-o` are mutually exclusive. A bundle preview writes the
same relative layout used in storage, excluding the ownership marker and other
remote-management metadata. It includes rendered pages, source files when
source inclusion is enabled, and byte-identical assets.

Stage output in a sibling temporary directory, then rename it into place. Refuse
any existing destination by default, including an empty directory, so the final
rename has portable semantics on Windows and POSIX systems. This prevents a
failed preview from leaving a plausible but incomplete bundle and makes the
preview usable for real-browser navigation tests.

### 4.4 Open and management behavior

- `--open` opens only the entry URL.
- `list` continues to report `kind=document`. Wide and JSON output may add page
  and asset counts without changing the default compact columns.
- CLI `show` and the existing Go, REST, and MCP inspection operations expose the
  entry, ordered pages, and assets. This feature does not add a CLI `inspect`
  command.
- `get TARGET` defaults to the entry page only when `TARGET` identifies the
  upload directory or ownership marker. An explicit declared child URL or key
  continues to fetch that exact page, source, asset, diff, or collection member.
  `get TARGET --source` retrieves the entry source from a directory or marker
  target.
- Add mutually exclusive `get TARGET --page PATH` and
  `get TARGET --asset PATH`. `--page PATH --source` retrieves that page's
  original source. An asset has no source/rendered distinction. Reject these
  selectors, like the existing `--source` and `--diff` selectors, when `TARGET`
  already names an explicit child.
- A direct URL to a declared child page or asset resolves the owning marker and
  remains a valid target for show, delete, protect, and other targeted
  operations. Lifecycle mutations always affect the whole document bundle.
- Delete, purge, protect, unprotect, sync, and bulk upgrade treat the marker's
  complete object inventory as one owned unit.

### 4.5 New revision command

Rename the canonical revision command from `update` to `new-revision`:

```console
airplan new-revision <url|key> README.md \
  --page docs/design.md \
  --asset images/flow.svg
```

The name states both material behaviors: Airplan creates a distinct upload and
links it as the next revision. It does not mutate the target upload or patch one
member of its bundle.

Keep `update` as a Cobra alias that invokes the exact same command object:

```go
Use:     "new-revision <url|key> [markdown-file]",
Aliases: []string{"update"},
```

Help, examples, error messages, shell completions, README instructions, and the
shipped skill use `new-revision` as the canonical spelling. Continue accepting
`airplan update` with identical flags, stdout, stderr, no-op behavior, and exit
status. Do not print a deprecation warning initially; warning on stderr would
add noise to otherwise valid existing agent workflows. This plan does not
schedule removal of the alias.

## 5. Storage layout and marker schema

Use safe relative logical paths directly below the random upload directory. A
representative bundle is:

```text
<id>/.airplan.json
<id>/readme.html
<id>/readme.md
<id>/docs/design.html
<id>/docs/design.md
<id>/examples/server.go
<id>/examples/server.go.html
<id>/images/flow.svg
<id>/recordings/demo.mp4
```

The entry keeps the existing slug-derived object naming. Its logical path can be
`README.md` while its stored page and source are `readme.html` and `readme.md`.
Supporting page and asset objects preserve their validated logical paths. This
distinction keeps the current entry URL contract without applying slug
normalization to nested bundle members.

### 5.1 Marker v6

Introduce ownership marker schema version 6. Version 5 readers must continue to
fail closed on a v6 marker rather than partially managing unfamiliar objects.
Every new write emits v6 after the rollout, including simple documents and
collections, following the existing one-current-writer-version policy.

The generic `objects` array remains the authoritative deletion and ownership
inventory. Extend it to:

- allow validated relative object names rather than basenames only;
- add the `asset` role;
- allow multiple `page` and `source` objects for documents; and
- require a lowercase hexadecimal SHA-256 digest for every payload object.

The digest requirement covers document pages, sources, assets and revision
diffs, plus collection index pages and members. It gives revision comparison,
repair, inspection, and upgrade planning one provider-independent content
identity. Native S3 checksums may verify transport or avoid a content download
when the provider returns a full-object SHA-256, but Airplan does not require
that optional provider feature and never substitutes an ETag for SHA-256.

Add document-specific fields conceptually equivalent to:

```json
{
  "entrypoint": "readme.html",
  "pages": [
    {
      "path": "README.md",
      "page": "readme.html",
      "source": "readme.md",
      "format": "md",
      "title": "Project plan",
      "lang": ""
    },
    {
      "path": "examples/server.go",
      "page": "examples/server.go.html",
      "source": "examples/server.go",
      "format": "txt",
      "title": "server.go",
      "lang": "go"
    }
  ]
}
```

`pages` is ordered, includes the entry when it is managed, and pairs each
logical source path with its rendered object. Persist the normalized highlight
language, including an empty value when no language applies, so revision
comparison and upgrade rendering reproduce a text page exactly. `source` is
omitted under `no_source`. For authored HTML entries, `entrypoint` identifies
the authored HTML page and `pages` is empty. Assets need no parallel descriptor
array in the marker because their logical paths, role, digest, size, and content
type are already present in `objects`.

Keep the top-level title, slug, format, and legacy in-memory `Page`/`Source`
compatibility view tied to the entry. Internal decoders should derive those
compatibility fields from `entrypoint` and the entry page descriptor rather
than assuming the first object with a matching role.

Increase the encoded marker limit to 256 KiB. New readers allow up to that limit
before decoding so they can identify marker v6, while v5 and older markers remain
readable. Initially cap a document at 100 user-supplied items total, including
the entry, pages, and assets. Validate the encoded marker size before storage in
addition to the count limit.

### 5.2 Object naming and generated collisions

A managed Markdown path maps to a sibling `.html` path by replacing its final
extension. A managed text or source-code path maps by appending `.html` to the
complete source path. Extensionless text follows the same append rule:

| Source path          | Format   | Rendered path             |
| -------------------- | -------- | ------------------------- |
| `guide.md`           | Markdown | `guide.html`              |
| `notes.markdown`     | Markdown | `notes.html`              |
| `src/main.c`         | text     | `src/main.c.html`         |
| `include/main.h`     | text     | `include/main.h.html`     |
| `examples/server.go` | text     | `examples/server.go.html` |
| `LICENSE`            | text     | `LICENSE.html`            |

The entry document continues to use the existing slug rules. This distinction
preserves familiar Markdown URLs while preventing common same-stem source pairs
such as `main.c` and `main.h` from competing for `main.html`.

Fail preflight when two sources still map to the same rendered path, when a
rendered path collides with an asset, or when any user path collides with a
reserved Airplan control object. Remaining real collisions include a text page
named `main.c` plus an asset named `main.c.html`, or `main.c` plus a Markdown
page named `main.c.md`; both would produce `main.c.html`.

Do not silently rename objects. Stable, explainable authored links matter more
than accepting an ambiguous bundle.

## 6. Markdown rendering and link rewriting

Build one validated page map before rendering any page. During Goldmark AST
processing, rewrite an ordinary link destination only when all of these are
true:

1. the destination is relative and has a path component;
2. resolving it against the current source page's logical directory produces a
   safe bundle-relative path;
3. that exact normalized path names a declared managed source; and
4. the destination is represented by a Markdown link node rather than raw HTML.

Replace only the path portion with the target rendered `.html` path relative to
the current rendered page. Preserve its query and fragment. Images continue to
point at assets. Absolute URLs, protocol-relative URLs, `mailto:`, same-page
fragments, undeclared local files, and raw HTML attributes remain byte-for-byte
under the existing renderer semantics.

This intentionally avoids guessing whether an undeclared `.md` link was a
mistake. It also preserves source bytes exactly: rewriting affects only rendered
HTML.

Every built-in rendered page receives the same ordered page navigation model,
the current logical page identity, and the entry URL. Rendering must not require
final public URLs; relative page URLs are sufficient and make local preview
identical to hosted behavior.

## 7. Built-in page navigation

### 7.1 Information architecture

Use the existing three-column document layout differently for bundles:

- left rail: **Pages**, with ordered titles and source-relative paths;
- center: the current rendered page; and
- right rail: **On this page**, using the current heading table of contents.

Single-page documents retain their current table-of-contents placement. The
bundle layout should reuse existing theme tokens, typography, borders, and
spacing. Page paths use the existing utility/monospace voice so the visual
identity feels like a technical notebook rather than a generic site sidebar.

The active page has both a non-color indicator and `aria-current="page"`.
Assets do not appear in the Pages rail. A compact previous and next footer
repeats page sequence controls after long content.

At narrow widths, expose Pages through a toolbar control and accessible dialog,
while retaining an ordinary no-JavaScript page list above the document. The
current narrow table-of-contents behavior remains. Enhancement may collapse the
fallback list after the dialog controller initializes successfully.

### 7.2 Browser-native cross-document transitions

All navigation items are ordinary `<a href>` links to complete HTML documents.
The built-in bundle template opts both ends of a same-origin navigation into a
cross-document View Transition:

```css
@media (prefers-reduced-motion: no-preference) {
  @view-transition {
    navigation: auto;
  }
}
```

The browser performs a real navigation. It loads a fresh document, executes its
scripts through the normal page lifecycle, changes the URL and history entry,
and applies native fragment, focus, and scroll behavior. The transition API
only controls how the browser presents the change between the old and new
documents.

Cross-document transitions require both documents to opt in and remain
same-origin. As of this plan, Chromium supports them from version 126 and Safari
from 18.2. Firefox supports same-document View Transitions but not the
cross-document `@view-transition` rule. Unsupported browsers ignore the CSS and
perform an ordinary navigation. See the
[Chrome cross-document transition documentation](https://developer.chrome.com/docs/web-platform/view-transitions/cross-document),
[Safari 18.2 release notes](https://webkit.org/blog/16301/webkit-features-in-safari-18-2/),
and [MDN compatibility data](https://github.com/mdn/browser-compat-data/blob/main/css/at-rules/view-transition.json).

Do not intercept clicks, fetch destination pages, replace DOM regions, call
`history.pushState`, or build a client-side page cache. Airplan does not need a
router, transition-safety classifier, renderer-identity handshake, or fallback
controller. Normal navigation is the only navigation path.

The CSS opt-in may animate another same-origin built-in document when both ends
opt in, including a linked revision or another bundle. That is acceptable
because the effect is cosmetic and the browser still performs the same complete
navigation. External links, downloads, reloads, and destinations that do not opt
in behave normally.

### 7.3 Page lifecycle and authored scripts

Markdown input remains trusted and may contain raw script elements. A fresh
document executes authored scripts and the Airplan page runtime through normal
browser loading, which preserves current semantics. Keep the existing one-shot
page initializer. Every destination initializes its own theme controls, source
and changes views, table of contents, copy buttons, print hooks, and Mermaid
rendering.

The full navigation resets transient page-local UI such as the Rendered/Source
selection. Theme preferences continue through their existing persistent
storage. Airplan does not copy runtime state between documents.

### 7.4 Motion and loading behavior

Start with a restrained root crossfade. Keep it short enough to read as
continuity rather than an interstitial. If visual testing shows that the page
rails move unnecessarily, give the Pages rail, document body, and On this page
rail stable, unique `view-transition-name` values and tune their pseudo-element
animations separately.

Only emit the opt-in under `prefers-reduced-motion: no-preference`. Browsers
without the cross-document rule, readers who request reduced motion, and
browsers that skip a transition for a particular navigation show the normal
page load without an Airplan-specific fallback.

The transition hides repaint discontinuity but does not remove network latency.
Do not add prefetching, prerendering, or an application cache initially.
Uploaded pages use `no-store`, and in-place upgrades can change bytes at stable
URLs. If real measurements show slow navigation, treat response caching and
speculative loading as a separate design with explicit freshness semantics.

### 7.5 Mermaid, printing, and renderer generation

Each destination page initializes Mermaid through the existing one-page
runtime. Navigation from a page without diagrams to one with diagrams needs no
special lazy-loader handoff because the destination loads its own script and
policy data.

Increment `RendererGeneration` when the bundle page structure and CSS contract
land, following the existing page-upgrade policy. The transition itself does
not require embedded bundle identity or runtime-generation metadata.

Print output includes only the loaded page. Existing before-print and
after-print disclosure handling initializes afresh on every page.

## 8. Go library contract

Introduce a first-class document-bundle API while retaining the current
single-input API as a compatibility wrapper. Proposed public shapes are:

```go
type PageInput struct {
    Reader io.Reader
    Path   string
    Format string
    Title  string
    Lang   string
}

type AssetInput struct {
    Reader      io.ReadSeeker
    Path        string
    Size        int64
    ContentType string
}

type DocumentInput struct {
    Entry            PageInput
    Pages            []PageInput
    Assets           []AssetInput
    Slug             string
    Title            string
    MaxPageSize      int64
    MaxTotalPageSize int64
    MaxAssetSize     int64
    MaxTotalSize     int64
    RepositoryURL    string
}

type PageResult struct {
    Path        string `json:"path"`
    Format      string `json:"format"`
    Title       string `json:"title,omitempty"`
    URL         string `json:"url"`
    Key         string `json:"key"`
    SourceURL   string `json:"source_url,omitempty"`
    SourceKey   string `json:"source_key,omitempty"`
    Bytes       int64  `json:"bytes"`
    SourceBytes int64  `json:"source_bytes,omitempty"`
}

type AssetResult struct {
    Path        string `json:"path"`
    URL         string `json:"url"`
    Key         string `json:"key"`
    Bytes       int64  `json:"bytes"`
    ContentType string `json:"content_type"`
}

type DocumentResult struct {
    Result
    Pages  []PageResult  `json:"pages,omitempty"`
    Assets []AssetResult `json:"assets,omitempty"`
}

func (c *Client) UploadDocument(
    ctx context.Context,
    in DocumentInput,
) (*DocumentResult, error)
```

`PageInput.Path` is the logical bundle-relative path and replaces filesystem
inference at the library boundary. CLI adapters are responsible for resolving
local paths, repository context, regular-file rules, and symlinks. Direct Go
callers may supply any reader after providing a valid logical path. The
100 MiB default for `MaxTotalPageSize` bounds all entry and supporting managed
source bytes; a negative value disables the client-side limit.

`Client.Upload(ctx, Input)` remains source-compatible and wraps its input as a
one-entry `DocumentInput`. Preserve `Result` fields as the entry-page summary so
existing callers and JSON consumers continue to get the same meaning. Existing
collection types and `UploadFiles` remain unchanged.

Add a canonical revision-creation operation for complete bundles:

```go
type CreateDocumentRevisionInput struct {
    Target   string
    Document DocumentInput
}

type DocumentRevisionResult struct {
    DocumentResult
    PreviousURL string `json:"previous_url,omitempty"`
    DiffURL     string `json:"diff_url,omitempty"`
    Unchanged   bool   `json:"unchanged"`
}

func (c *Client) CreateDocumentRevision(
    ctx context.Context,
    in CreateDocumentRevisionInput,
) (*DocumentRevisionResult, error)
```

Keep `UpdateDocument(UpdateDocumentInput)` and `UpdateDocumentResult` as the
existing one-entry compatibility API. Mark them deprecated in Go documentation
without changing runtime behavior, and implement the method through
`CreateDocumentRevision`. Do not add the previously proposed
`UpdateDocumentBundle` name; new bundle-aware callers should start with the
accurate canonical operation. Extend compatibility result JSON additively where
needed so existing consumers can observe page and asset results without losing
the established entry aliases.

Add a renderer counterpart such as:

```go
func RenderDocument(
    ctx context.Context,
    in DocumentInput,
    opts DocumentRenderOptions,
) (*RenderedDocumentBundle, error)
```

`RenderedDocumentBundle` contains every generated page and declared copy
operation without writing storage or the local manifest. The explicit render
API may retain its results, but rejects aggregate generated HTML above 100 MiB.
Upload and output-directory preview use the same internal renderer with a
temporary-file sink, preserving one implementation of link rewriting, page
identity, template data, and collision checks without retaining every page
body. Keep `RenderInput` as the single-page wrapper.

Extend the stable custom document template contract with additive `Pages`,
`CurrentPage`, `Entrypoint`, and `Assets` fields. Each path/URL field supplied to
templates must already be normalized or safely escaped for its context. Do not
inject built-in navigation assets into custom output.

## 9. Upload pipeline and failure semantics

The local and server-backed operation services use one logical pipeline:

1. Validate the document shape, path graph, counts, and per-page, aggregate-page,
   per-asset, and aggregate-asset limits.
2. Read, validate, hash, and spool text pages while enforcing the 100 MiB source
   total; determine all formats and navigation metadata.
3. Validate asset sizes and rewindability, then hash assets while preserving
   their starting offsets.
4. Resolve every generated object name and fail on collisions.
5. Render every managed page against the complete page map, hashing and spooling
   output while enforcing the separate 100 MiB generated HTML total.
6. Encode and size-check the v6 ownership marker.
7. Upload the marker first so every later partial object remains owned.
8. Upload sources and assets while hashing the exact transmitted bytes. Fail if
   a second-pass digest differs from the marker.
9. Upload non-entry rendered pages.
10. Upload the entry rendered page last.
11. Record the completed result in the local manifest.

Returning the entry URL only after the entry PUT means every declared dependency
already exists when the document becomes reachable through the reported URL.
As today, a failed operation can leave owned partial objects that sync or delete
can reconcile. Do not roll back by best-effort deletion, which can create a less
recoverable state.

Collection creation adopts the same digest invariant without changing its
public `FileInput` shape. Preflight reads and hashes every seekable member,
rewinds it, and records the digest in the marker. Upload hashes each member
again and fails before publishing `index.html` if the transmitted digest
differs. This adds one full local read per collection member but avoids temporary
copies and keeps the marker authoritative when a caller mutates a reader between
passes.

When supported by the configured store, pass the precomputed SHA-256 through
the native S3 checksum fields and compare the returned full-object checksum.
Provider checksum support is an extra integrity check. Marker digests and the
second local hash remain the portable contract.

Bound upload concurrency rather than launching one goroutine per object. Keep
the entry page as an explicit final barrier. Cancellation stops scheduling and
attempts to interrupt in-flight storage requests; arbitrary caller readers
retain the existing limitation that the caller may need to close them.

## 10. REST API and HTTP transport

Add `POST /api/v1/uploads/documents/revisions` as the canonical revision
operation under `/api/v1`. Keep
`POST /api/v1/uploads/documents/update` as a deprecated compatibility route to
the same operation. Both routes accept and return the same media types and
schemas. This is an additive request shape for capable clients and servers, not
a new resource family.

### 10.1 Multipart request

Extend document multipart bodies to contain, in order:

1. one JSON `metadata` part;
2. one `document` part for the entry;
3. zero or more `pages` parts; and
4. zero or more `assets` parts.

Metadata contains ordered descriptors for pages and assets. Each descriptor
provides the logical path and any format, title, language, or content type
needed to bind and validate its corresponding repeated part. Asset descriptors
also provide the exact declared size. Page readers remain arbitrary `io.Reader`
values at the Go boundary, so the server measures their exact sizes while
spooling. The metadata order is authoritative. Part filenames are diagnostic
only and must not override logical paths.

Reject missing, extra, out-of-order, or count-mismatched parts. Preserve strict
unknown-field handling in generated JSON models. Increase the metadata-part
limit from 64 KiB to 256 KiB, enforce it in client preflight, and advertise it
through capabilities. The server streams multipart parsing and spools each page
and asset to a mode `0600` temporary file, enforcing per-part, aggregate-page,
per-asset, aggregate-asset, and complete-request limits during the copy. Cleanup
all temporary files on every exit path.

The generated client continues using `io.Pipe` and `multipart.Writer` so large
assets are never assembled into one in-memory body. It emits exact declared
asset sizes and rewinds seekable asset readers deterministically. Existing
no-retry semantics remain because an arbitrary page stream cannot safely be
replayed after partial delivery.

### 10.2 OpenAPI models and results

Update `api/openapi.yaml`, then regenerate the strict server and client types.
Name the canonical operation `createDocumentRevision`, with
`CreateDocumentRevisionMetadata` and `DocumentRevisionResult` schemas. Extend
the canonical metadata with:

- entry metadata;
- ordered page descriptors;
- ordered asset descriptors;
- `max_page_size`;
- `max_total_page_size`;
- `max_asset_size`; and
- `max_total_size`.

Retain the existing `max_size` field in extended document-upload metadata and
as a deprecated alias for `max_page_size` in revision metadata, because v0.10.0
and older clients send it to the existing routes. Accept either spelling. If
both are present, require equal values and reject a conflicting request rather
than choosing silently. Preserve the other legacy update metadata fields,
including `target`, `name`, and `title`, in the shared revision schema.

Extend document upload, revision, show, and inspect results with `pages` and
`assets`. Preserve existing top-level result fields as entry aliases and keep
`files` specific to collections.

Retain the existing `updateDocument` operation on the compatibility route, mark
it `deprecated: true`, and point it at the same canonical schemas. Generated
clients therefore expose the new accurate method while old clients and raw HTTP
callers continue to work. The server routes both operation adapters into one
implementation; compatibility must not fork validation, limits, error mapping,
or revision behavior.

Update `internal/httpapi` model aliases, request parsing, operation adapters,
client encoding, error mapping, and generated-contract tests together. The
transport-parity tests should execute the same `DocumentInput` against direct
storage and an HTTP test server and compare logical outcomes.

### 10.3 Capability negotiation

Add a document-bundle capability to the server capability response, including:

- support for managed pages and assets;
- support for the canonical document-revision route;
- maximum total document item count;
- maximum per-page source, aggregate managed-source, aggregate generated-page,
  per-asset, and aggregate-asset bytes;
- maximum metadata-part bytes; and
- maximum complete multipart request bytes.

The server's complete request envelope must accommodate the advertised page and
asset totals plus multipart overhead. A client-valid bundle must not pass all
advertised logical limits and then receive an unexplained transport-level 413.

A new client sending a bundle checks this capability before opening or streaming
parts and reports a clear server-upgrade error when absent. A new client may
send the legacy one-document shape to an old server. Old clients continue to
upload single documents to new servers.

The HTTP transport chooses the canonical revision route only when the server
advertises it. Otherwise, it uses the legacy update route for an existing
single-entry request. Never probe the canonical route by streaming a multipart
body and retrying after a 404; arbitrary readers and partially delivered bodies
are not replay-safe. A bundle still requires the separate document-bundle
capability regardless of which route name reaches a new server.

The marker version change protects storage management compatibility, while API
capability negotiation protects request compatibility. No `/api/v2` is needed
unless implementation reveals a non-additive response or operation semantic
that cannot be represented safely in the existing schema.

## 11. MCP tools

MCP must support both hosted inline content and efficient local file upload.
One schema cannot serve both well because a hosted MCP server cannot read the
caller's filesystem, while base64 is unsuitable for large recordings.

### 11.1 Cross-transport inline tools

Extend `upload_document` with optional fields conceptually shaped as:

```json
{
  "content": "# Entry",
  "name": "README.md",
  "pages": [
    {
      "path": "docs/design.md",
      "content": "# Design",
      "format": "md",
      "title": "Design"
    }
  ],
  "assets": [
    {
      "path": "images/flow.svg",
      "content_base64": "...",
      "content_type": "image/svg+xml"
    }
  ]
}
```

Register `new_document_revision` as the canonical inline revision tool with the
same complete-bundle fields. Pages are UTF-8 strings; assets are strict standard
base64. Use the existing `url_or_key` field for the revision target so the
canonical and compatibility tools can share one schema without breaking
current callers. Reject invalid base64 before calling the operation service.

MCP has no transparent tool-alias mechanism, so continue registering
`update_document` as a compatibility tool with the same input schema, output
schema, and handler. Its description identifies it as the compatibility name
for `new_document_revision`. Both tools remain visible during this compatibility
period; this plan does not schedule removal of the old name.

Keep the streamable HTTP MCP request ceiling at its current value and make the
encoded JSON body limit authoritative. Also add a 32 MiB decoded aggregate
inline-asset limit to bound base64 decoding and allocation. These limits are
simultaneous: highly escaped or large text content can exhaust the encoded body
budget before the decoded-asset ceiling. Describe both in tool schemas and
errors. REST or the local file tools are the supported path for large media.

### 11.2 Local-file tools

When `MCPServerOptions.LocalFiles` is enabled, register:

- `upload_document_files` with `entry_path`, `page_paths`, and `asset_paths`;
  and
- `new_document_revision_files` with `url_or_key`, `entry_path`, `page_paths`,
  and `asset_paths`.

These tools apply the same local path and bundle-root rules as the CLI and avoid
base64 buffering. Like the existing `upload_files`, omit them from hosted MCP
because their paths would refer to the server's filesystem rather than the
caller's.

`new_document_revision_files` is new in this feature, so it needs no
update-named compatibility alias.

Do not overload inline fields to sometimes mean paths. Separate tool names make
the trust boundary visible to agents and keep hosted schemas honest.

All document MCP results expose structured entry, page, and asset data. Human
content leads with the entry URL and summarizes counts rather than printing
every asset URL unless requested through inspection. Update the shipped
`skills/airplan/SKILL.md` so agents choose inline tools for small generated
artifacts, local-file tools for screenshots and recordings, and collections for
peer files without a primary narrative.

## 12. Revisions, diffs, and no-op detection

Extend linked Markdown revisions from one source/page pair to a complete bundle.
The entry must remain Markdown. A revision resupplies the entire bundle and
creates one new upload directory when any of these change:

- a page's logical path, format, title, language, or source bytes;
- an asset's logical path, content type, size, or bytes;
- page or asset ordering where ordering is visible; or
- entry-level metadata that affects rendered output.

Compare normalized descriptors and stored SHA-256 digests before rendering or
uploading large unchanged assets where practical. A byte-identical logical
bundle is the existing successful no-op and consumes no revision number.

Keep one `.airplan-changes.diff` object per revision. Generate a deterministic
combined report containing:

- unified diff sections for added, removed, and changed UTF-8 managed sources;
- rename representation as remove plus add unless exact-digest rename detection
  is trivial and unambiguous; and
- concise binary asset summaries with path, content type, size, and digest
  changes.

Do not embed binary data. Preserve the current total and inline diff limits.
The built-in Changes view presents sections by logical path.

The replicated revision index continues to store each revision's entry URL, not
a page URL map. The settled follow-up in
[Revision-aware page navigation and diff UX](revision-aware-navigation-and-diffs.md)
uses exact logical source paths as cross-revision identity and validates the
selected revision's v6 marker on demand. A matching child performs an ordinary
navigation to the same logical page; an absent page or invalid/unavailable
marker falls back to the selected entry. This keeps versions metadata bounded.

## 13. In-place upgrades and concurrency

A document upgrade re-renders every managed page in the target bundle using its
stored source and current renderer. It preserves source and asset bytes, public
URLs, creation time, protection, and revision identity.

Upgrade eligibility requires source-backed managed pages and a marker whose
bundle graph is structurally complete. Extend planning and conditional-write
state to include every rendered page and source identity. Re-plan immediately
before execution, compare the marker and object ETags with the approved plan,
and use conditional writes rather than relying on a process-local lock.

Treat marker v6 as the desired state during an upgrade:

1. Render every managed page and calculate its desired digest.
2. Build the target marker with the current renderer generation and desired
   object digests. Conditionally write it once when it differs from the current
   marker. If a retry finds that exact desired marker already present, do not
   perform an identity PUT merely to obtain a new ETag.
3. Determine each stored non-entry page's actual digest. Use a provider's native
   checksum only when it is a full-object SHA-256; otherwise GET and hash the
   page. Skip pages that match. Replace each mismatch with `If-Match` against
   the ETag observed during planning.
4. Re-read the marker and verify that every non-entry page matches its desired
   digest.
5. Apply the same comparison to the entry page and write it last only when it
   differs.
6. Verify the marker, every rendered page, unchanged sources, protection state,
   and revision metadata before recording success.

Do not update the marker after each page. A failed attempt may leave the desired
marker with some old rendered objects and some new ones. This is an owned,
repairable partial state, not a completed upgrade. `inspect` reports the digest
mismatches, and the next attempt repeats planning but uploads only mismatched
pages. Entry-last ordering reduces exposure but does not promise an atomic
bundle-generation switch. Ordinary navigation can temporarily encounter mixed
renderer generations until repair completes; every individual page remains a
standalone document with a normal script lifecycle.

Conflicting writers with different desired markers lose the conditional marker
claim. Concurrent repairers for the same desired marker may divide successful
page writes, but conditional page ETags prevent either from overwriting an
unobserved change. A loser re-plans against the converged state.

Authored HTML with assets remains non-upgradeable because Airplan has no source
renderer to apply. `no_source` Markdown bundles retain the current inability to
upgrade or create linked revisions.

## 14. Security, privacy, and trust boundaries

- Preserve the current trusted-document boundary. Markdown raw HTML and URL
  destinations remain unsanitized.
- Rely on the browser's same-origin requirement for cross-document transitions.
  Airplan performs no script-driven destination fetch or DOM replacement.
- Treat SVG, HTML, JavaScript, and other active assets as trusted authored
  content. Serving them unchanged may execute content when directly opened or
  embedded according to browser rules. Document this rather than silently
  changing content types.
- Do not introduce a Content Security Policy as part of this feature. A useful
  CSP requires a separate compatibility design for raw HTML, authored scripts,
  Mermaid, themes, and custom templates.
- Never expose server-local paths through REST or hosted MCP results and errors.
- Use mode `0600` temporary files and close/unlink them on success,
  cancellation, parser failure, limit failure, and operation failure.
- Keep unguessable directory URLs as capability URLs. Child page and asset URLs
  share the same capability boundary as the entry.
- Continue emitting `no-store` for generated pages and management metadata.

## 15. Compatibility and migration

### 15.1 Compatibility promises

- `airplan FILE` behaves as before.
- `airplan FILE FILE...` remains a collection.
- `airplan new-revision` is canonical, while `airplan update` remains a
  behaviorally identical alias with no warning or scheduled removal.
- `Client.Upload(Input)` and `RenderInput` remain available.
- `Client.CreateDocumentRevision` is canonical, while
  `Client.UpdateDocument` remains a deprecated source-compatible wrapper for
  its existing one-entry input.
- Existing top-level result fields describe the entry page.
- Existing v5 and older markers remain readable and manageable.
- New code models an old single document as one entry page and zero assets.
- Old clients can upload single documents through a new server.
- New servers continue accepting the legacy REST `max_size` field and the MCP
  `update_document.url_or_key` field.
- New clients require advertised capability before sending a bundle to an old
  server.
- The canonical REST revision route and deprecated update route share one
  operation implementation and schema.
- MCP advertises both `new_document_revision` and the compatibility
  `update_document` tool with identical behavior.
- Every generated bundle page works through full browser navigation without
  JavaScript and in browsers without cross-document transition support.

Old Airplan binaries must not manage v6 markers. This is an intentional
fail-closed storage-format boundary. The SPEC.md version receives a minor bump
because the CLI, page behavior, APIs, and marker format all change observably.

### 15.2 Documentation updates in the implementation PR

Update together:

- SPEC.md, including its semantic version;
- IMPLEMENTATION.md;
- README upload, preview, REST, and MCP examples;
- `api/openapi.yaml` and generated API code;
- `schema/airplan.schema.json` only if configuration gains defaults or server
  limit keys;
- the shipped Airplan skill;
- shell completions, with `new-revision` canonical and `update` accepted as an
  alias; and
- live demo fixtures, if a stable multi-page demo can be maintained without
  deleting superseded uploads.

## 16. Implementation sequence

Implement in one cohesive feature branch, but keep the following dependency
order so each layer can be tested before the next one depends on it:

1. **Contract and fixtures**
   - Write SPEC.md vNext behavior for paths, roles, marker v6, CLI, result JSON,
     canonical revision naming, compatibility aliases, REST, MCP,
     browser-native transitions, revisions, and upgrades.
   - Add representative bundle fixtures covering nested Markdown, source code,
     SVG, image, and binary media paths.
2. **Core model and marker**
   - Add path validation, page/asset inputs and results, collision detection,
     mandatory payload digests, the 256 KiB marker limit, marker v6
     encoding/decoding, and old-marker compatibility projection.
   - Extend ownership resolution and management inventory before any producer
     can write v6.
3. **Rendering**
   - Add bundle preflight, AST link rewriting, custom-template data, complete
     page rendering, and standalone local materialization.
   - Implement deterministic Markdown replacement and non-Markdown append rules
     for rendered page names.
4. **Storage operation**
   - Implement upload ordering, preflight and transmitted-byte hashing, optional
     native S3 checksum verification, limits, cancellation, result mapping, and
     manifest recording for direct storage.
   - Add collection-member digests while retaining seekable streaming and
     detecting a reader that changes between the two passes.
   - Wrap existing single-document entry points around the new operation.
5. **CLI and preview**
   - Add explicit role flags, filesystem containment checks, output-directory
     preview, management selectors, help, completions, and JSON output.
   - Make `new-revision` canonical and keep `update` as a warning-free alias to
     the same Cobra command.
6. **REST transport**
   - Add the canonical revision route, deprecate but retain the update route,
     regenerate code, extend capabilities, implement streaming multipart
     parsing/client emission, raise metadata to 256 KiB, preserve frozen legacy
     request shapes, and prove direct/HTTP parity.
7. **MCP**
   - Add canonical revision tools, retain `update_document` as a compatibility
     tool, add local-file tools, enforce base64 limits, and update the shipped
     skill.
8. **Browser UI**
   - Add Pages and On this page rails, previous and next controls,
     cross-document View Transition CSS, named transition regions if visual
     testing justifies them, and reduced-motion behavior.
9. **Revisions and upgrades**
   - Add the canonical Go revision API and legacy wrapper, then extend no-op
     comparison, combined diffs, complete-bundle revision creation, upgrade
     planning, digest-based resumable repair, conditional writes, and
     bulk-management presentation.
10. **Documentation and demos**
    - Finish public examples, architecture documentation, golden fixtures, and
      any maintained live demo.

Do not publish v6-producing code before all lifecycle readers and destructive
operations understand the complete v6 object inventory. If work is split across
pull requests, keep bundle creation behind an unexported or disabled capability
until that invariant holds.

## 17. Verification strategy

Tests should target the failure modes introduced by the bundle graph, generated
page names, and cross-document transitions rather than multiplying every
existing single-document case.

### 17.1 Core and marker tests

- Table-test safe nested paths, traversal, separators, reserved names,
  case-folded duplicates, and generated `.html` collisions.
- Cover Markdown extension replacement, non-Markdown extension preservation,
  extensionless text, same-stem source pairs, and remaining real collisions.
- Golden-test v6 markers for a simple document, multi-page bundle, HTML plus
  assets, `no_source`, collection, and revision member.
- Prove old markers decode into the entry compatibility view and old-version
  encoders reject v6-only shapes.
- Verify the 100-item and 256 KiB marker boundaries, including 100 long
  descriptors with digests.
- Require and assert digests for every document and collection payload against
  exact uploaded bytes.
- Persist page language through marker encode/decode and cover a text page whose
  explicit language differs from filename detection.

### 17.2 Rendering and preview tests

- Cover cross-directory Markdown links, fragments and queries, source-code
  pages, undeclared links, images, external schemes, and raw HTML links.
- Golden-test every generated page, including active navigation and current-page
  table of contents.
- Prove source bytes and asset bytes remain unchanged.
- Verify custom template fields without assuming built-in scripts or styles.
- Materialize a preview and assert every relative page and asset target exists.
- Inject a mid-materialization error and prove the destination is not exposed as
  a completed bundle.
- Verify an existing empty or non-empty output directory is rejected on POSIX
  and Windows.

### 17.3 CLI, upload, and transport tests

- Verify help and generated completions present `new-revision` as canonical and
  accept `update` as an alias.
- Run canonical and alias CLI invocations against equivalent chains and assert
  identical flags, validation, stdout, stderr, exit status, no-op behavior, and
  revision results.
- Exercise `CreateDocumentRevision` and the legacy `UpdateDocument` wrapper
  against equivalent one-entry chains, plus the canonical bundle-only path.
- Observe marker-first, dependency-before-entry ordering with a recording
  storage fake.
- Fail each upload phase and prove the remaining partial state is marker-owned.
- Test per-page, 100 MiB aggregate-page, per-asset, aggregate-asset, count, and
  cancellation limits using many valid pages as well as one oversized page.
- Mutate a same-size seekable asset and collection member between the preflight
  and upload reads; assert the second digest fails the operation before entry or
  collection-index publication.
- Exercise multipart missing, extra, reordered, oversized, malformed-base64,
  and cleanup paths.
- Exercise the 256 KiB REST metadata boundary and prove the advertised complete
  request envelope accepts every bundle allowed by its logical limits.
- Confirm a large asset is streamed rather than buffered by the HTTP client and
  server.
- Compare direct and REST results for the same bundle.
- Exercise the canonical and deprecated REST routes against equivalent chains
  and assert the same schemas, validation errors, limits, no-op results, and
  revision state.
- Prove capability negotiation selects the route before streaming and that a
  new client uses the legacy route with an old server without replaying a body.
- Test new-client/old-server and old-client/new-server capability behavior.
- Replay byte-frozen v0.10.0 document-upload and update multipart requests
  containing `max_size` against the new server, and reject conflicting
  `max_size` and `max_page_size` values.
- Preserve exact-child `get` behavior and reject page, asset, source, or diff
  selectors when the target already identifies a declared child.

### 17.4 MCP tests

- Exercise inline pages and assets on stdio and streamable HTTP.
- Exercise `new_document_revision` and `update_document` against equivalent
  chains and assert identical schemas, results, errors, and no-op behavior.
- Replay the frozen v0.10.0 `update_document` schema with `url_or_key`, and
  assert the canonical tool uses the same target field.
- Confirm only `new_document_revision_files` is registered for the newly added
  local-file revision workflow.
- Verify the 32 MiB decoded asset ceiling and encoded request-body ceiling
  independently, including a request where escaped text reaches the encoded
  ceiling first.
- Confirm local file tools are registered only with `LocalFiles` and never read
  server-local paths in hosted mode.
- Assert tool results and errors do not leak local paths.

### 17.5 Revision and upgrade tests

- Cover page/asset add, remove, modify, reorder, collision, and unchanged
  bundles.
- Cover a language-only page change through revision comparison and upgrade.
- Assert deterministic text and binary diff sections and existing size limits.
- Simulate stale writers and mid-upgrade failures across multiple page ETags.
- Fail after each page write and prove the marker-owned partial state may mix
  generations, inspection identifies every mismatch, and a retry converges.
- Record PUTs during retry and prove matching pages and the marker are skipped,
  dependencies are repaired before the entry, and no page causes a per-object
  marker rewrite.
- Verify sync, delete, purge, protect, and inspect operate on every v6 object.

### 17.6 Browser tests

Use `mise run test:browser` against a generated preview bundle and cover desktop
and narrow viewports in light and dark themes:

- ordinary links create a fresh document and execute destination scripts;
- URLs, titles, active page state, table of contents, source view, and copy
  behavior are correct after each load;
- back, forward, fragment navigation, focus, and scroll use native browser
  behavior;
- the built-in bundle CSS opts into cross-document transitions only when the
  reader has not requested reduced motion;
- no-JavaScript navigation and narrow fallback page list;
- custom templates do not receive the built-in transition opt-in;
- authored scripts execute through the normal document lifecycle;
- navigation from a page without Mermaid to one with Mermaid;
- print disclosure behavior after navigation; and
- source and changes controls initialize correctly on every destination.

Use a document-lifetime sentinel to prove navigation creates a new document.
Visually inspect the root crossfade in current Chrome and Safari, including slow
network simulation, desktop and narrow layouts, and reduced motion. Confirm
that current Firefox performs an ordinary usable navigation without transition
artifacts. Headless assertions alone do not prove animation quality.

### 17.7 Handoff gates

During implementation, run focused Go and browser tests as each layer lands.
Before handoff run:

```console
mise run check
mise run test:browser
mise run test-integration
mise run generate:check
```

Run `mise run verify` when Docker and release tooling are available because the
marker, HTTP transport, generated assets, and public library all change. Native
Windows CI remains required evidence for path normalization and multipart temp
file behavior that cannot be reproduced fully on macOS.

For this plan-only change, Markdown formatting plus `git diff --check` is
proportionate verification; no runtime behavior changes.

## 18. Risks and mitigations

| Risk                                                                         | Mitigation                                                                                               |
| ---------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| A v6 writer creates objects an old destructive operation misses              | Make all readers inventory-safe before enabling v6 writes; old binaries fail closed                      |
| Nested user paths escape the upload directory or collide after normalization | Centralize one segment-aware validator used by CLI, library, API, marker decoding, and URL generation    |
| A browser lacks cross-document View Transitions                              | Ordinary links and complete standalone pages remain the only navigation path                             |
| A transition masks repaint but not slow network delivery                     | Keep normal loading semantics; measure before designing cache or prerender freshness rules               |
| Managed pages or assets exhaust memory                                       | Enforce a 100 MiB page-source total, stream assets, spool REST parts, and retain strict transport limits |
| Revision creation accidentally retains omitted files                         | Define revisions as complete replacement and compare complete normalized inventories                     |
| A seekable payload changes between preflight and upload                      | Hash both passes, fail before entry publication, and retain marker ownership for repair                  |
| Native S3 checksum behavior differs by provider or upload mode               | Keep marker SHA-256 canonical and accept only full-object SHA-256 as an optional fast path               |
| Canonical and compatibility names drift apart                                | Route every alias and wrapper through one operation and replay frozen legacy wire shapes                 |
| Revision metadata grows with every page in every revision                    | Store only entry URLs in the replicated revision index                                                   |
| An upgrade fails after replacing some child pages                            | Declare repairable mixed state, write the entry last, and skip digest-matching pages on retry            |
| Mobile navigation becomes inaccessible without JavaScript                    | Keep ordinary links and a visible fallback page list until enhancement initializes                       |

## 19. Explicit non-goals

- Automatic recursive directory upload.
- Reference-based role inference from document contents.
- Airplan-managed navigation among authored HTML pages.
- Rewriting raw HTML attributes, CSS URLs, JavaScript imports, or asset contents.
- Asset galleries, thumbnailing, media transcoding, virus scanning, or archive
  expansion.
- Client-side content replacement, custom history management, an application
  router, offline cache, service worker, or managed cross-upload navigation.
- Partial in-place editing of one page or asset in a document bundle.
- Page-level protection, deletion, retention, or revision history independent
  from the owning document.
- Changing the current trusted-content boundary or introducing CSP.
