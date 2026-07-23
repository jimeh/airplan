# airplan — Tool Specification

**Spec version: 0.28.0**

Semantic versioning, applied to the spec itself: while below 1.0,
**minor** covers observable behavior changes — including breaking
pre-release corrections and backward-compatible additions — and
**patch** covers clarifications and editorial fixes. Once the
contract is deliberately declared stable at 1.0.0, **major** covers
breaking changes, **minor** covers backward-compatible additions,
and **patch** covers clarifications and compatible corrections. The
first implementation release does not by itself force spec 1.0.

`airplan` uploads AI/LLM agent documents and file collections to
S3-compatible object storage under randomized, unguessable URL paths and
prints the resulting public URLs. It can access storage directly or use a
single-user self-hosted Airplan HTTP server that owns the S3 credentials.
An agent can publish a plan as a readable page, or upload screenshots,
recordings, and other artifacts as one collection with a generated overview
page, then link the result from chat, an issue, or a pull request.

This document specifies **behavior only**: what the tool does, its
interfaces, and its on-the-wire and on-disk formats. It contains no
implementation detail; a conforming implementation can be built in any
language and remain fully compatible — same CLI, same config files,
same URLs, same page features, same manifest format. How _our_
implementation is built lives in [IMPLEMENTATION.md](IMPLEMENTATION.md).

Non-goals: no accounts, multi-user authorization, embedded manifest web UI,
background sync daemon, remotely coordinated database, horizontal server
replicas, recursive directory upload, media transcoding, thumbnail generation,
or archive expansion. Airplan is not a public catalog or general pastebin.

---

## 1. Processing Model & Output Contract

The selected backend changes how the same Airplan operation is invoked:

```
CLI or MCP request
  → backend=s3: invoke the operation service in this process
  → backend=airplan: invoke the same service through REST
  → select document or collection mode
  → preflight and render the primary HTML page
  → generate one random upload directory
  → PUT the kind-specific ownership marker
  → PUT source or collection files
  → PUT the primary HTML page last
  → append one manifest entry where the operation service runs
  → return and print public URL(s)
```

Upload output contract (critical for agent use):

- **stdout**: for a document, the final public page URL and nothing else. For a
  collection, one direct file URL per input in argument order followed by the
  overview URL. With `--json`, one JSON object and nothing else.
- **stderr**: all logs, warnings, progress, errors.
- **exit code**: 0 on success; non-zero on any failure. Never print a
  URL that wasn't successfully uploaded.

`get` writes the fetched object bytes and nothing else to stdout unless
`--output` selects a file.

---

## 2. Input Handling

`airplan [flags] [file ...]` — no file, or one `-`, reads a document from
stdin. Collections require one or more named regular files.

Airplan selects collection mode before rendering or storage mutation when any
of these conditions hold:

1. `--files` is set.
2. More than one path is supplied.
3. One named input has a recognized media or generic-binary extension. The
   deterministic set includes common image, video, audio, PDF, and archive
   formats; SVG is included even though it is text.
4. One named input contains a NUL byte in its first 8 KiB or those bytes are
   not valid UTF-8.

An explicit `--format md|html|txt` forces document mode and retains document
validation. Stdin is always document mode. `--files` and `--format` are
mutually exclusive.

### Document input

Three input formats: markdown (rendered, §3), HTML (uploaded as-is,
§4), and plain text (rendered as a highlighted code page, §3).
Named document inputs must be regular files. Streams remain supported through
stdin by omitting the path or passing `-`.

Format detection:

1. `--format md|html|txt` wins if given.
2. File extension: `.md`/`.markdown` → md; `.html`/`.htm` → html;
   **any other extension → text** (`.go`, `.py`, `.txt`, `.json`, …).
3. Extensionless filename recognized by the syntax highlighter's
   filename patterns (`Makefile`, `Dockerfile`, …) → text.
4. Otherwise — stdin, or an unrecognized extensionless name — sniff:
   leading `<!doctype` or `<html` (case-insensitive, after
   whitespace/BOM) → html, else md. Bare stdin defaulting to
   markdown is load-bearing: it is the primary agent path.

Binary rejection: input containing a NUL byte within its first 8 KiB
(git's binary heuristic) is rejected with an error before any upload,
regardless of detected or forced format. airplan uploads UTF-8 text
documents: input that is not valid UTF-8 is likewise rejected before
any upload, regardless of detected or forced format. There is no
bypass for either check. When input fails both checks, the invalid
UTF-8 error takes precedence over the binary-input error.

A zero-byte document is rejected before key generation or any upload.
Whitespace-only input remains valid: airplan does not reinterpret authored
text merely because it has no visible characters.

Size limit: input larger than the configured maximum — default
**10 MiB** — is rejected with an error before any upload. The whole
document is loaded into memory for rendering (md/text) or the noindex
splice (html), and a plan document over the default is invariably a
mistake — the wrong file, like a database dump. Implementations must
detect the overflow without buffering meaningfully past the limit.
`--max-size` sets the limit per invocation: a plain byte count, or an
integer with a `k`/`m`/`g` suffix (binary multiples) whose unit may have an
optional trailing `b`/`ib`; matching is case-insensitive (`10MB`, `512k`,
`1gib`). Unit tails without `k`/`m`/`g`, such as `10ib`, are invalid. `0`
removes the limit. There is deliberately no config key, so raising or removing
the guard stays a per-invocation decision.

### Collection input

Collection preflight completes before a random directory is generated or any
object is uploaded. It opens every input and keeps that exact file open through
upload, rejects directories and non-regular files, records the basename and
size, detects a content type, rejects duplicate basenames, enforces all limits,
renders the overview, and encodes the complete marker.

Member names are direct basenames, never paths. Empty names, `.`, `..`,
`.airplan.json`, `.airplan-collection.json`, `index.html`, names containing
slashes, backslashes, NUL, or control characters, and duplicate names are
rejected. A collection may contain up to 100 files. Zero-byte members are
valid.

`--title` sets the overview title. Without it, a one-file collection uses the
member basename; multiple files use `<first basename> and <N> more`.

The default collection limits are **1 GiB per file** and **2 GiB total**.
`--max-size` applies per member; `--max-total-size` applies to the sum. A value
of `0` removes the corresponding limit. Both use the size syntax documented
above. Collection files are uploaded from seekable readers with their known
sizes instead of being buffered wholly in memory. Growth after preflight cannot
append undeclared bytes; unexpected truncation fails the upload.

Content types use a deterministic extension mapping for common browser media,
including PNG, JPEG, GIF, WebP, AVIF, SVG, MP4, WebM, MOV, MP3, M4A, Ogg, WAV,
and PDF. Unknown extensions use conservative content sniffing, then
`application/octet-stream`. The original bytes are never sanitized, rewritten,
transcoded, expanded, or otherwise interpreted.

---

## 3. Markdown Rendering

Markdown input is rendered to an HTML page with embedded CSS and a system font
stack. Airplan-managed external loading is limited to optional features
described below.

- Markdown dialect: CommonMark plus GitHub Flavored Markdown
  extensions — tables, strikethrough, task lists, URL/email autolinks — plus
  definition lists, footnotes, heading anchors, and GitHub-style alerts.
  GFM autolinks retain balanced parentheses and exclude trailing punctuation.
  Alerts use the
  standard blockquote markers `NOTE`, `TIP`, `IMPORTANT`, `WARNING`,
  and `CAUTION`; they are converted to static HTML during
  rendering and may contain normal block Markdown. Unrecognized alert
  markers remain ordinary blockquotes.
- YAML frontmatter delimited by exact `---` lines and TOML frontmatter
  delimited by exact `+++` lines are recognized only at byte zero, after an
  optional UTF-8 BOM. The closing delimiter must match. Invalid, unclosed, or
  non-mapping frontmatter is an error; a missing, empty, or non-string `title`
  is ignored. Frontmatter is excluded from the rendered body, headings, and
  table of contents. The built-in page displays the exact block in a collapsed
  native details element with server-side syntax highlighting. Source view and
  the uploaded source remain byte-exact.
- A narrow subset of Pandoc fenced divs provides responsive columns. An outer
  delimiter is at least four colons followed only by `{.columns}`. It contains
  at least two direct child divs whose delimiter is at least three colons and
  shorter than the outer delimiter, followed by `{.column}` or a `width`
  attribute containing an integer or decimal percentage greater than zero and
  at most 100. Normal block Markdown is supported within each child. Unknown
  attributes, nesting, orphaned/unterminated delimiters, and invalid widths
  remain ordinary Markdown. Columns share available width equally unless
  weighted, prevent content overflow, and stack at narrow and print layouts.
- With repository context, plain-text references `#123`,
  `owner/other-repo#456`, and full 40-character hexadecimal commit IDs become
  links to the corresponding GitHub-compatible issue or commit. Matching uses
  strict token boundaries and never changes inline or fenced code, Mermaid
  source, existing links or images, raw HTML, or GFM URL/email autolinks.
- Trust boundary: raw inline/block HTML and link/image destinations are
  rendered as authored. Markdown and HTML input are trusted content and
  may execute active content when someone opens the resulting page.
  The original Markdown remains exact in source view and in the
  uploaded sibling.
- Fenced code blocks are syntax-highlighted at render time. The highlighting
  follows the selected page theme, with separate light and dark palettes;
  System follows `prefers-color-scheme`. Print always uses the light palette.
- An exact lowercase `mermaid` fenced code block is rendered as a Mermaid
  diagram. Its readable, HTML-escaped source remains the no-JavaScript and
  load-failure fallback and remains exact in source view. The built-in page
  loads Mermaid only when such a block exists and external assets are allowed,
  using an exact pinned ECMAScript module URL, strict security, explicit
  rendering, and Airplan-controlled light and dark theme variants. The
  selected page theme chooses the screen variant, while print always uses the
  light variant; per-diagram theme configuration cannot override these
  variants. Custom templates receive the Mermaid template data below but do
  not receive injected assets.
- Page styling: supports Light, System, and Dark themes. System follows
  `prefers-color-scheme`; support for both schemes is advertised through the
  standard document-level `color-scheme` hint. The page uses a centered
  document shell around 54rem
  wide, prose constrained to a readable measure around 78ch, comfortable
  line height, distinct heading/body/muted color roles, and section
  hierarchy carried primarily by type and spacing rather than repeated
  divider rules. Code blocks and tables may use the full shell width so an
  80-column source line fits without horizontal scrolling at the default
  font size. Inline and block code use separate subtle surfaces; block code
  has a quiet border and thin horizontal scrollbar. Print uses a compact
  10.5-point body with a 1.45 line height, removes screen-only content padding,
  tightens vertical spacing, and keeps headings with the following content
  when pagination permits. With scripting enabled, all `details` elements are
  expanded while printing and return to their prior open or closed state
  afterward. Print CSS also reveals closed disclosure content without scripting
  in browsers that support `::details-content`.
- A responsive table of contents is rendered from markdown headings:
  - H1, H2, and H3 headings are included. If an H1 is the first visible
    block in the document, it is treated as the document title and is
    the only heading omitted from the built-in table of contents. Later
    H1 headings remain top-level entries.
  - Heading links and hierarchy work without JavaScript. On wide
    screens the table of contents occupies a sticky rail beside the
    centered document; on narrow screens it moves above the document.
    As a progressive enhancement on layouts without the sticky rail, a
    compact control keeps the table of contents reachable after its
    inline version scrolls above the viewport.
  - In-page navigation scrolls smoothly by default. It becomes immediate
    when the reader requests reduced motion.
  - Scroll position highlighting is a progressive enhancement and
    respects `prefers-reduced-motion`. The table of contents is hidden
    in source view and omitted when fewer than two entries remain.
- `<title>` from `--title`, else a non-empty string frontmatter `title`, else
  first `<h1>`, else source filename, else the resolved slug (covers stdin
  input with no `<h1>`).
- `<meta name="robots" content="noindex, nofollow">` — belt and
  braces on top of URL unguessability; works regardless of what
  headers the CDN/domain serves. Omitted under `--indexable`.
- Baseline interactive niceties use a small amount of embedded vanilla JS with
  no framework. Mermaid's conditional module is the only airplan-managed
  external script:
  - Theme toggle: an icon-only Light/System/Dark segmented control with
    accessible names. System is the initial default. Light and Dark choices
    persist in browser storage and apply to other built-in pages on the same
    origin; choosing System clears that override. With scripting disabled,
    the page follows the system preference and does not show the control. The
    theme toggle follows the file controls. At wider sizes the rendered/source
    toggle aligns left while file controls align right, with the theme toggle
    at the far-right edge and a quiet divider separating it from the file
    controls. At narrow sizes the rendered/source and theme toggles share the
    first row at opposite edges, with available file controls clustered and
    left-aligned below. When no rendered/source toggle is available, the file
    controls instead occupy the first row opposite the theme toggle. Toolbar
    controls update immediately without color or background transitions when
    their active state or the page theme changes.
  - Rendered/source toggle: switch between the rendered plan and a
    syntax-highlighted view of the original markdown. The source is
    highlighted at render time, so no client-side highlighter
    ships. (Embedding the source roughly doubles page weight —
    irrelevant at plan-document sizes.) The controls use visible text
    labels, and source view identifies itself as “Markdown source”.
    The view toggle uses a subtle segmented treatment; adjacent file
    actions are borderless, with muted hover states and a clearly
    visible keyboard-focus outline.
  - "Copy markdown" button for the full original source. Raw text
    is recovered from the highlighted block's text content (the
    highlight markup must preserve it exactly), so the source is
    embedded once.
  - "Download markdown" button: a plain `<a download>` anchor to the
    sibling `.md` object (relative link, `./<slug>.md`). Being a
    plain anchor, it works even without JS; omitted when the source
    wasn't uploaded (`--no-source`).
  - "Raw" link: a plain anchor to the same sibling source without the
    `download` attribute, so the browser can open it directly. It has
    the same availability and no-JavaScript behavior as Download.
  - Per-code-block copy buttons on hover; always visible on touch
    devices, where hover doesn't exist.
  - Graceful degradation: with JS disabled the rendered view stays
    fully readable and controls are hidden. Controls are likewise
    hidden in print styles. Clipboard API needs a secure context,
    which https links satisfy.

### Plain-text input

Text input (§2) shares the markdown page machinery: the same
standalone page template, styling, and dark/light behavior, with the
body being the source rendered as one syntax-highlighted code block.
A shared source file reads like a one-file gist.

- The highlight language comes from `--lang` when given (a language
  name the highlighter knows: `go`, `python`, `json`, …), else from
  the source filename (extension or recognized special names like
  `Makefile`). When neither yields a lexer — a forced `--format txt`
  on stdin without `--lang`, an unknown extension, or an
  unrecognized `--lang` value — the block renders as unhighlighted
  plain text. (This is about the highlight language only; which
  inputs _become_ text format is decided solely by §2.)
- Title chain: `--title`, else the original source filename
  including its extension (`keygen.go`), else slug (no
  content-derived title — the document is never interpreted).
- The page shows the original filename as a header bar attached to
  the code block, so a shared file identifies itself. Omitted for
  stdin input, where no filename exists.
- The original file is uploaded alongside the page as
  `<random>/<slug>.<ext>` (`text/plain; charset=utf-8`, same cache
  headers), where `<ext>` is the source filename's extension —
  `txt` when there is none (stdin) or when it would collide with
  the page object (`html`/`htm`). The page's download anchor points
  at it, and the Raw anchor opens it without forcing a download.
  `--no-source` skips it, exactly as for markdown.

### Page templates & customization

Users can substitute the built-in page template with their own via
`template` in a profile, `AIRPLAN_TEMPLATE`, or `--template PATH`.
Applies to markdown and text input — HTML input is always uploaded
as-is (warn if combined).

Template data contract (the stable API custom templates code
against):

| Field                         | Type      | Meaning                              |
| ----------------------------- | --------- | ------------------------------------ |
| `.Title`                      | string    | resolved title                       |
| `.RenderedHTML`               | raw HTML  | rendered markdown or text page body  |
| `.SourceText`                 | string    | original unmodified source           |
| `.HighlightedSourceHTML`      | raw HTML  | syntax-highlighted original source   |
| `.SyntaxCSS`                  | raw CSS   | styles required by highlighted HTML  |
| `.Headings`                   | heading[] | all markdown headings                |
| `.TOC`                        | heading[] | built-in H1-H3 ToC entries           |
| `.Format`                     | string    | `md` or `txt`                        |
| `.Language`                   | string    | resolved source-highlight language   |
| `.SourceName`                 | string    | original basename; empty for stdin   |
| `.SourcePath`                 | string    | relative path to the uploaded source |
| `.Slug`                       | string    | resolved slug                        |
| `.Indexable`                  | boolean   | whether indexing is allowed          |
| `.HasMermaid`                 | boolean   | exact Mermaid fence was rendered     |
| `.NoExternalAssets`           | boolean   | managed external loads are disabled  |
| `.MermaidURL`                 | string    | resolved Mermaid module URL          |
| `.FrontMatterText`            | string    | exact complete frontmatter block     |
| `.FrontMatterFormat`          | string    | `yaml`, `toml`, or empty             |
| `.FrontMatterTitle`           | string    | usable frontmatter title or empty    |
| `.HighlightedFrontMatterHTML` | raw HTML  | highlighted frontmatter block        |
| `.RepositoryURL`              | string    | resolved canonical repository URL    |

Each heading has `.Level` (1–6), `.ID`, `.Text`, and `.IsTitle`.
`.IsTitle` is true only for a leading H1 that the built-in table of
contents omits. `.TOC` is structured data, not pre-rendered navigation
HTML, so custom templates retain control of markup and presentation.

`.SourcePath` is empty when the source isn't uploaded
(`--no-source`); templates must handle both cases.

A custom template takes full responsibility for the page: page styles,
noindex meta, and any interactivity. `.SyntaxCSS` is supplied because it
is coupled to the generated highlighting classes; the built-in page's
own CSS and JavaScript are baked directly into its template rather than
exposed as data. `airplan template` prints that exact, self-contained
built-in template to stdout. Saving the output and passing it back via
`--template` must work unchanged.

Portability boundary: the data contract above is
implementation-independent; the template _syntax_ is
implementation-defined, so user template files are not portable
across implementations.

### Collection overview pages

Every collection has a generated `index.html` primary page. The built-in page
is self-contained and has no Airplan-managed external resources. It preserves
input order, shows the collection title, repository context when present, file
count, and total member bytes, and presents each member according to its media
kind:

- images render at full available width and intrinsic aspect ratio in a
  rounded, overflow-clipped frame with lazy loading and filename-derived alt
  text; the preview itself links to the direct member URL, matching `Open`;
- video renders edge-to-edge in the same frame, while audio uses a bordered
  container; both use controls, `preload="metadata"`, and never autoplay;
- PDF, archive, text, and unknown members use compact file rows without empty
  preview panels;
- every member shows its filename, content type, human-readable size, and
  `Open`, `Download`, and `Copy link` actions;
- an overview copy action returns the absolute `index.html` URL;
- open and download links work without JavaScript, while copy buttons are a
  progressive enhancement;
- relative member URLs remain correct under custom domains and key prefixes;
- titles, filenames, types, and repository metadata are HTML-escaped;
- the layout supports narrow and wide viewports plus Light, System, and Dark
  themes using the same persisted `airplan-theme` preference as document
  pages; the collection toolbar and shared controls use the same structure,
  dimensions, spacing, and interaction styling as document pages; System
  follows `prefers-color-scheme`, while print uses Light;
- the page includes `noindex, nofollow` unless `--indexable` is set.

Unknown content remains a normal openable and downloadable file. The page does
not claim it is unsafe or broken merely because Airplan cannot preview it.

Users may replace the collection page through `collection_template` in config,
`AIRPLAN_COLLECTION_TEMPLATE`, or `--collection-template PATH`. This setting is
independent of the document `template` setting. Only the applicable template
is loaded and parsed, so a broken collection template does not block document
uploads and a broken document template does not block collections or as-is
HTML. Applicable template read, parse, execution, and empty-output failures
occur before any storage mutation.

Collection template data contract:

| Field            | Type   | Meaning                           |
| ---------------- | ------ | --------------------------------- |
| `.Title`         | string | resolved collection title         |
| `.Files`         | file[] | ordered collection members        |
| `.TotalBytes`    | int64  | sum of member sizes               |
| `.Indexable`     | bool   | whether indexing is allowed       |
| `.RepositoryURL` | string | resolved canonical repository URL |

Each file has `.Name`, `.Path`, `.ContentType`, `.Bytes`, and `.MediaKind`.
`.MediaKind` is `image`, `video`, `audio`, or `file`. `.Path` is an already
percent-encoded relative URL such as `./Screenshot%201.png`; templates must not
reconstruct URLs from `.Name`. The implementation-defined template function
`formatBytes` renders a byte count in human-readable binary units.

A custom template controls presentation only. It cannot rename members or
alter the marker, result, or uploaded inventory. Airplan still declares and
uploads every input even when the template omits its link. The template author
is responsible for page styles, noindex markup, accessibility, copy behavior,
JavaScript, and any external resources. `airplan template collection` prints
the exact reusable built-in template; `airplan template` and
`airplan template document` print the document template.

---

## 4. HTML Input

Uploaded as-is, with one deliberate exception: by default a
`<meta name="robots" content="noindex, nofollow">` tag is injected,
so HTML uploads get the same indexing protection as rendered
markdown pages.

Injection rules (privacy by default, applied conservatively):

- The tag is spliced immediately after the first explicit `<head …>`
  start token emitted by HTML tokenization outside inert `template`
  and `noscript` content. Head lookalikes in comments, raw-text, or
  RCDATA content do not count. This is a byte-level splice at the
  original token boundary: the document is never re-serialized, and
  every other byte is served exactly as uploaded.
- That head's metadata scope ends at the first effective `</head>` or
  `<body …>` token outside inert content, or at EOF. Only an effective
  `<meta>` start token in that scope, outside `template` and `noscript`
  content, whose parsed `name` attribute equals `robots` ASCII
  case-insensitively prevents injection. Normal HTML attribute parsing,
  including character-reference decoding, applies. Author intent in
  the effective head wins; meta lookalikes and metadata elsewhere do
  not weaken the privacy default.
- If tokenization finds no complete explicit effective head start
  token, a warning is printed to stderr and the file is uploaded
  unmodified. Once a valid splice point exists, malformed later markup
  does not prevent injection unless an effective robots meta was
  already recognized.
- `--indexable` disables injection entirely.

No DOM tree is built or repaired, and no other modification occurs.
HTML input never uploads a sibling source object: the uploaded object
already is the original file.

---

## 5. Upload Behavior

- Every new upload writes ownership marker version 3. Readers continue to
  manage versions 1 and 2, but writers never migrate or emit them. Marker
  versions describe wire-schema generations; `kind` distinguishes documents
  from collections. Older clients fail closed on new v3 uploads.

- The exact marker basename supplies an untrusted LIST-only kind hint:

  | Kind         | Marker basename            |
  | ------------ | -------------------------- |
  | `document`   | `.airplan.json`            |
  | `collection` | `.airplan-collection.json` |

  Existing v1/v2 uploads remain documents under `.airplan.json`. Marker
  content remains authoritative. A v3 marker whose `kind` disagrees with its
  basename is invalid. A directory containing both names has conflicting
  ownership declarations and grants no managed read or deletion authority.

- Markers are UTF-8 JSON uploaded with
  `Content-Type: application/json` and `Cache-Control: no-store`, and are at
  most 64 KiB. A v3 document marker is:

  ```json
  {
    "schema": "airplan-upload",
    "version": 3,
    "directory": "vq3nhk2p7r4wzt5c6ydjm3xhqd",
    "created_at": "2026-07-21T12:00:00Z",
    "kind": "document",
    "slug": "plan",
    "format": "md",
    "objects": [
      {
        "name": "plan.html",
        "role": "page",
        "bytes": 18432,
        "content_type": "text/html; charset=utf-8"
      },
      {
        "name": "plan.md",
        "role": "source",
        "bytes": 4096,
        "content_type": "text/markdown; charset=utf-8"
      }
    ],
    "title": "Refactor auth",
    "repo": "https://github.com/acme/service"
  }
  ```

  A v3 collection uses the same declared-object model:

  ```json
  {
    "schema": "airplan-upload",
    "version": 3,
    "directory": "vq3nhk2p7r4wzt5c6ydjm3xhqd",
    "created_at": "2026-07-21T12:00:00Z",
    "kind": "collection",
    "objects": [
      {
        "name": "index.html",
        "role": "page",
        "bytes": 9216,
        "content_type": "text/html; charset=utf-8"
      },
      {
        "name": "login.png",
        "role": "file",
        "bytes": 184320,
        "content_type": "image/png"
      }
    ],
    "title": "Login flow",
    "repo": "https://github.com/acme/service"
  }
  ```

- `schema`, `version`, `directory`, `created_at`, `kind`, and `objects` are
  required in v3. `schema` is exactly `airplan-upload`; `directory` matches the
  containing random directory; `created_at` is RFC 3339 UTC. `repo`, when
  present, is the canonical resolved HTTPS repository URL. Connection-local
  profile, endpoint, credentials, bucket, prefix, and public URL metadata are
  never stored.

- `objects` is non-empty, has unique safe direct basenames, and contains
  exactly one positive-size HTML `page`. Every object declares `name`, `role`,
  `bytes`, and a syntactically valid normalized `content_type`. A document has
  a required valid `slug`, required `format` (`md`, `html`, or `txt`), no
  `file` objects, and at most one positive-size `source` following the existing
  document filename rules. A collection omits `slug` and `format`, uses
  `index.html` as its page, declares no source, and contains one through 100
  `file` objects whose sizes may be zero. Unknown roles or kinds are invalid.

- Unknown marker fields are ignored for forward-compatible extensions.
  Duplicate field names, invalid UTF-8, malformed JSON, unsupported versions,
  unsafe or inconsistent filenames, invalid roles, sizes, content types, or
  repositories, and oversized markers are invalid. Unsupported markers remain
  visible to LIST-only discovery but cannot be inspected as valid, fetched,
  deleted, purged, or synced.

- Version 1 and 2 markers are decoded into the v3 declared-object model after
  their original wire rules validate. Version 1 omits `page_bytes` and `repo`;
  version 2 requires positive `page_bytes` and may include `repo`.

- Every payload uses `Cache-Control: no-store`. Primary pages use
  `Content-Type: text/html; charset=utf-8`; collection members use their
  declared content types. Airplan does not force browser-viewable media to
  download because direct image and video URLs are part of the intended use.
  `x-amz-meta-title` remains convenience metadata; the marker title is
  authoritative for remote management.

- Document order is `.airplan.json` → optional source → page. Collection
  order is `.airplan-collection.json` → files in argument order →
  `index.html`. Any PUT failure fails the command and writes no local upload
  record or stdout URL. Marker-first creation leaves interrupted uploads
  discoverable; page-last creation prevents an overview from appearing before
  its declared payloads. An upload is complete only when every declared object
  exists with its declared size. Extra unrecognized objects do not affect
  completeness.

- After complete storage, Airplan assembles result URLs and best-effort appends
  one manifest upload record. A manifest warning does not revoke an otherwise
  successful upload. Collection stdout remains withheld until the marker,
  every member, and the overview have all uploaded successfully.

- Bucket must **not** allow listing publicly; privacy rests on the
  key being unguessable. Documentation covers the R2 setup: public
  bucket via custom domain (listing is not exposed) or Workers
  route.
- Region defaults to `auto` (R2 convention); real AWS users set it.

---

## 6. CLI Interface

```
airplan [flags] [file ...]
```

No file, or one `-`, reads a document from stdin. Multiple paths are one
collection.

| Flag                      | Default        | Notes                              |
| ------------------------- | -------------- | ---------------------------------- |
| `--files`                 | off            | force named inputs into collection |
| `--format`                | auto           | document-only `md`\|`html`\|`txt`  |
| `--slug S`                | from filename  | document-only URL filename         |
| `--title T`               | from content   | document or collection title       |
| `--template P`            | built-in       | document template                  |
| `--collection-template P` | built-in       | collection overview template       |
| `--no-source`             | off            | document-only source suppression   |
| `--indexable`             | off            | omit noindex on the primary page   |
| `--no-external-assets`    | off            | document-only managed load control |
| `--mermaid-url URL`       | pinned URL     | document-only Mermaid module       |
| `--repo VALUE`            | `auto`         | `auto`, `none`, or repository URL  |
| `--max-size N`            | mode-specific  | 10MiB document; 1GiB per file      |
| `--max-total-size N`      | 2GiB           | collection total; 0 = no limit     |
| `--timeout D`             | 30s            | operation timeout; 0 = none        |
| `--lang L`                | from filename  | document text highlight language   |
| `--json`                  | off            | JSON object on stdout              |
| `--profile P`             | config default | named profile from config file     |
| `--config PATH`           | XDG default    | alternate config file              |
| `--manifest PATH`         | state default  | local S3 operation manifest        |
| `--open`                  | off            | open the primary page              |
| `--version`               |                |                                    |

Plus flag overrides for every connection setting (`--endpoint`,
`--bucket`, `--region`, `--public-base-url`, `--key-prefix`) for
one-off use.

Frequent flags get short forms: `-p` (`--profile`), `-s` (`--slug`),
`-t` (`--title`), `-j` (`--json`), and `-o` (`--open`). On subcommands,
`-r` is `--remote` for `list` and `purge`, while `-o` is `--output` for
`preview` and `get`. Connection overrides stay long-only.
`airplan completion bash|zsh|fish|powershell` emits shell completions.

If `--open` fails to launch a browser (common in headless/agent
environments), a warning goes to stderr and the exit code is
unaffected — the upload succeeded and the URL was already printed.

Flags explicitly used in the wrong mode fail before storage mutation.
`--format`, `--lang`, `--slug`, `--template`, `--no-source`,
`--no-external-assets`, and `--mermaid-url` are document-only. `--files`,
`--collection-template`, and `--max-total-size` are collection-only.
`--open` always opens the primary page: a document page or collection
overview, never an arbitrary member.

Released binaries report their release version under `--version`.
GoReleaser builds may stamp it directly; binaries installed through
the Go module path derive it from embedded Go build information.
Module pseudo-versions are reported without their leading `v`.
Unversioned local development builds, including dirty builds, report
`dev`.

Official macOS release archives contain native `amd64` or `arm64`
executables signed with a Developer ID Application identity, hardened
runtime enabled, and a secure timestamp. Apple must accept each executable
for notarization before its release is published. A raw executable cannot
carry a stapled notarization ticket, so its first Gatekeeper assessment may
require internet access to retrieve the ticket from Apple. This guarantee
does not cover `go install`, whose locally built executable is not Developer
ID-signed or Apple-notarized by the project.

Context-aware execution phases are bounded by a timeout — default **30
seconds** — so stalled input and storage operations fail with a clear error
instead of hanging the caller (often an agent harness) indefinitely. The clock
begins after config resolution; config loading itself is excluded because the
config may supply the timeout. Interactive confirmation time is also excluded.

Upload, preview, list, show, get, and delete each receive one timeout budget.
Local
purge starts one deletion budget after confirmation. Remote purge receives one
budget for listing and marker inspection, then a fresh deletion budget after
confirmation. This prevents human think time from consuming a network budget
and gives both remote phases the configured opportunity to finish. Operations
that share a phase share its deadline; a large sequential purge may therefore
complete partially and report the remaining items as failures for retry.

The timeout is configurable via `--timeout` / `AIRPLAN_TIMEOUT` / the
`timeout` config key (root or profile level), with the usual precedence (§7).
Values are Go-style duration strings (`30s`, `1m30s`) or a bare integer meaning
seconds; out-of-range values are errors and `0` disables the timeout.

Examples:

```sh
airplan plan.md
# → https://plans.example.com/vq3nhk2p7r4wzt5c6ydjm3xhqd/plan.html

airplan login.png demo.webm
# → https://plans.example.com/vq3n.../login.png
# → https://plans.example.com/vq3n.../demo.webm
# → https://plans.example.com/vq3n.../index.html

cat plan.md | airplan --slug refactor-auth
airplan --files README.md
airplan --json report.html
airplan --profile personal --open plan.md
```

`--json` output (single line, stable schema):

```json
{
  "url": "https://plans.example.com/vq3n.../plan.html",
  "key": "vq3nhk2p7r4wzt5c6ydjm3xhqd/plan.html",
  "source_url": "https://plans.example.com/vq3n.../plan.md",
  "bucket": "plans",
  "bytes": 18432,
  "content_type": "text/html; charset=utf-8"
}
```

`source_url` is omitted for HTML input and under `--no-source`.
`bytes` and `content_type` describe the uploaded page object (the
one `url` points at), not the markdown source.

Collection `--json` output remains one line and one object. `url`, `key`,
`bytes`, and `content_type` describe `index.html`; `files` maps members in
input order:

```json
{
  "url": "https://plans.example.com/vq3n.../index.html",
  "key": "vq3n.../index.html",
  "files": [
    {
      "name": "login.png",
      "url": "https://plans.example.com/vq3n.../login.png",
      "key": "vq3n.../login.png",
      "bytes": 184320,
      "content_type": "image/png"
    }
  ],
  "bucket": "plans",
  "bytes": 9216,
  "content_type": "text/html; charset=utf-8"
}
```

Errors: human-readable single-line message to stderr prefixed
`airplan:`; with `--json`, errors still go to stderr as text (stdout
stays reserved for the success object).

### Subcommands

```
airplan config schema
airplan config profiles [--config PATH] [--json]
airplan skill
airplan template [document|collection]
airplan preview [flags] [file ...]
airplan completion bash|zsh|fish|powershell
airplan list|ls [--remote] [--json]
airplan show [--json] <url|key>
airplan get [--output PATH] [--source] <url|key>
airplan delete <url|key>
airplan purge [--remote] [--older-than 30d]
              [--all] [--dry-run] [--yes] [--concurrency N]
airplan sync [--config PATH] [--profile NAME] [--concurrency N]
             [--no-prune] [--dry-run] [--json]
airplan serve [--listen ADDR] [--allow-non-loopback] [--token-file PATH]
              [--allowed-origin ORIGIN] [--temp-dir PATH]
airplan mcp
```

`config schema` prints the config file's JSON Schema (see §7).
`skill` prints the complete canonical airplan agent skill to stdout,
byte-for-byte, including its YAML frontmatter and trailing newline. It accepts
no arguments or command-specific flags and emits nothing to stderr on success.
It does not load configuration, inspect credentials, access storage or the
network, or write state, so it works with only the installed binary and from
any working directory. The same content is available through the public core
library API.
`template` prints a built-in template (see §3). With no argument or with
`document`, it prints the document template. `template collection` prints the
collection overview template.
`preview` runs input detection and page rendering locally, writing the
resulting HTML to stdout or to `--output PATH`. It supports the rendering
flags `--format`, `--lang`, `--slug`, `--title`, `--template`,
`--indexable`, `--no-external-assets`, `--mermaid-url`, `--repo`, and
`--max-size`,
plus `--config` and `--profile` for
resolving template settings. It does not validate S3 connection fields,
access the network, upload source, or write the manifest. Consequently
`.SourcePath` is empty in a preview, while markdown's embedded source
view remains available. HTML input receives the same conservative
noindex injection as an upload. `file` omitted or `-` reads stdin;
`--output -` is equivalent to the stdout default. An output path that
resolves to the input file is rejected without modifying the input. File output
is written completely to a temporary file beside the destination and then
atomically renamed into place; any failure before the rename leaves an existing
destination unchanged.

`preview --files` or multiple named inputs renders a collection overview. It
supports `--title`, `--collection-template`, `--indexable`, `--repo`,
`--max-size`, and `--max-total-size`, performs the same collection preflight,
and accesses neither storage nor the manifest. The output uses the same
relative member paths as an upload. Airplan does not copy or inline member
files for preview, so media resolves locally when the output is saved beside
the inputs; callers may stage files together when inputs came from different
directories.

`ls` is an exact non-destructive alias for `list`.

`list`/`purge` operate on the operation service's manifest by default, or
on its live bucket listing with `--remote`. With an `airplan` backend those
operations execute on the server. `show` inspects one remote
marker directory. `get` fetches only objects declared by a valid remote
ownership marker. `delete` takes an explicit URL or key, but it only
operates on a directory carrying a valid airplan ownership marker; it
therefore works on marker-managed uploads from any machine without
becoming a general-purpose bucket deletion command. See §9.
`sync` reconciles the selected remote marker inventory into the operation
service's manifest. It imports remotely present uploads and, by default,
tombstones uploads whose markers are confirmed absent. It never mutates
remote storage.

---

## 7. Configuration

Resolution precedence: **flags > env vars > selected profile >
root-level values > built-in defaults**. Config file location:
`$XDG_CONFIG_HOME/airplan/config.toml`
(`~/.config/airplan/config.toml`; platform-appropriate config
directory on Windows), overridable with `--config` /
`AIRPLAN_CONFIG`.

The platform-default config file is optional so environment variables and
flags can fully configure the tool. A path explicitly selected with `--config`
or `AIRPLAN_CONFIG` must exist; a missing explicit path is an error rather than
silently falling back to an empty configuration.

The global manifest path resolves as `--manifest PATH` then
`AIRPLAN_MANIFEST` then the platform `DefaultManifestPath()`. Relative paths
are relative to the invocation working directory. The result applies to every
local `s3` operation, `serve`, and stdio `mcp` when it selects `s3`. An
explicit `--manifest` is rejected for the HTTP `airplan` backend because a
client cannot choose a server filesystem path; `AIRPLAN_MANIFEST` is ignored
for that backend. Local-only commands that do not construct a backend client
reject an explicitly supplied `--manifest` as inapplicable.

All connection/behavior keys may be set at the root level of the
config file as well as inside profiles. Root-level keys are base
values every profile inherits; a profile overrides only what it
sets. The simplest config needs no profiles at all:

```toml
# ~/.config/airplan/config.toml — minimal single-bucket setup
backend         = "s3"
endpoint        = "https://<account-id>.r2.cloudflarestorage.com"
bucket          = "plans"
region          = "auto"
public_base_url = "https://plans.example.com"
```

With profiles (note: TOML requires root-level keys to appear before
the first `[profiles.*]` header):

```toml
# ~/.config/airplan/config.toml
# Root-level keys are shared base values; profiles override only
# what differs.
endpoint        = "https://<account-id>.r2.cloudflarestorage.com"
region          = "auto"
# template = "~/.config/airplan/my-template.html"  # optional
# collection_template = "~/.config/airplan/my-collection.html"
# repo = "auto"       # GitHub context: auto, none, or explicit URL
# no_source = true    # behavior defaults; flags override
# timeout = "30s"     # operation timeout; 0 = none
# indexable = true
# Credentials may live here, but env vars are preferred:
# access_key_id     = "..."
# secret_access_key = "..."
key_prefix      = ""          # optional, prepended to object keys
                              # (also scopes list/purge --remote;
                              # give each person one in a shared
                              # bucket)

default_profile = "work"

[profiles.work]
bucket          = "work-plans"
public_base_url = "https://plans.work.example.com"

[profiles.personal]
endpoint        = "https://s3.eu-west-2.amazonaws.com"
region          = "eu-west-2"
bucket          = "jimeh-plans"
public_base_url = "https://jimeh-plans.s3.eu-west-2.amazonaws.com"

[profiles.shared]
backend         = "airplan"
api_url         = "https://airplan.example.com"
api_token       = "..."
```

`backend` is `s3` when omitted. An `s3` profile uses the existing storage,
rendering, and manifest settings. An `airplan` profile requires an absolute
HTTP(S) `api_url` and `api_token`; HTTPS is required except for loopback hosts.
It sends operations to that server and never loads ambient AWS credentials or
writes a second client-side manifest. S3 settings inherited by an `airplan`
profile, and API settings inherited by an `s3` profile, are inactive. Explicit
inactive profile settings may produce a warning; inherited ones do not.

### Profile resolution

1. `--profile` / `AIRPLAN_PROFILE`, if given (error if it names a
   profile that doesn't exist).
2. Else `default_profile`, if set (error if dangling).
3. Else, if exactly one named profile exists, use it.
4. Else, if the root-level values — merged with environment
   variables and flag overrides, which sit above them in the
   precedence order — form a complete configuration, run on those.
   This keeps one-off `--endpoint`/`--bucket` invocations working
   against a config file that happens to define multiple profiles.
5. Else, error — listing the available profile names.

In every case the selected profile is merged over the root-level
values per the precedence above.

### Configured profile inventory

`airplan config profiles` lists the named Airplan profiles defined by
`[profiles.*]` in the selected config file. It does not include the root-level
configuration as a pseudo-profile or inspect profiles from the standard AWS
credential chain. Names are sorted lexicographically. The default table has
the exact columns `PROFILE` and `DEFAULT`; the latter is `yes` only for the
profile named by `default_profile` and `no` otherwise. It does not indicate an
active or inferred profile. A config with no named profiles writes no table
output. Empty names and names containing non-graphic Unicode characters are
rendered as Go-quoted strings in the table so each profile stays on one safe
terminal row; JSON retains the exact name.

`--json` / `-j` returns an array of objects with string `name` and boolean
`default` fields in the same order. An empty inventory is `[]`, not `null`.
The command accepts only `--config` and `--json`; in particular, `--profile`
and normal config override flags do not apply. Config path selection remains
explicit `--config`, then `AIRPLAN_CONFIG`, then the optional platform default.

Profile inventory parses the config file strictly and verifies that
`default_profile`, when present, names a defined profile. Malformed TOML,
unknown keys, a dangling default, and a missing explicitly selected path are
errors. The command does not perform active-profile resolution, merge or parse
other `AIRPLAN_*` values, validate config field values or completeness, resolve
credentials, access storage or the network, or write local state. Thus an
ambiguous, incomplete multi-profile config remains listable. Config permission
warnings go to stderr under the same rules as normal configuration loading.

Environment variables (highest-priority credential source in
practice, agent-harness friendly):

```
AIRPLAN_PROFILE
AIRPLAN_BACKEND
AIRPLAN_API_URL
AIRPLAN_API_TOKEN
AIRPLAN_ENDPOINT
AIRPLAN_BUCKET
AIRPLAN_REGION
AIRPLAN_ACCESS_KEY_ID
AIRPLAN_SECRET_ACCESS_KEY
AIRPLAN_PUBLIC_BASE_URL
AIRPLAN_KEY_PREFIX
AIRPLAN_TEMPLATE
AIRPLAN_COLLECTION_TEMPLATE
AIRPLAN_NO_EXTERNAL_ASSETS
AIRPLAN_MERMAID_URL
AIRPLAN_REPO
AIRPLAN_TIMEOUT
AIRPLAN_CONFIG
AIRPLAN_MANIFEST
```

For `s3`, credential fallback order is `AIRPLAN_*` env → profile file values →
standard AWS chain (`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`,
shared credentials file). The AWS chain fallback makes it work
out-of-the-box in environments already configured for S3. If exactly
one of `access_key_id` and `secret_access_key` is set after merging,
configuration fails instead of silently ignoring the partial pair and
falling back to ambient AWS credentials.

`endpoint` and `public_base_url` must be absolute HTTP(S) URLs with a
host and without user information, query, or fragment components.
Path prefixes are allowed. `key_prefix` may contain arbitrary UTF-8
path-segment text, but empty internal segments and `.` / `..`
segments are rejected because intermediaries can normalize them.
When public links are assembled, every object-key segment is
percent-encoded; delete URL parsing reverses that encoding.
`mermaid_url` must be valid UTF-8 and an absolute HTTPS URL with a host and
without user information or a fragment; paths and query strings are allowed.
It is validated even when external assets are disabled or a custom template is
used.

`repo` / `AIRPLAN_REPO` / `--repo` accepts `auto`, `none`, or an
explicit repository URL. Explicit HTTPS, `ssh://git@host/owner/repo`,
`ssh://git@host:PORT/owner/repo`, and `git@host:owner/repo` forms normalize to
`https://host/owner/repo`; an optional `.git` suffix is removed and an SSH
transport port is dropped. Credentials, HTTPS ports, query strings, fragments,
extra path segments, local paths, `file:` URLs, and `git:` URLs are rejected.
SSH URL user information must be exactly the username `git` with no password;
the SCP-like form likewise requires the `git@` prefix.
An explicit URL may name a GitHub Enterprise-compatible host and an invalid
value is an error.

`auto` performs quiet, local-only Git discovery of the `origin` remote and
accepts only `github.com`; it never contacts the remote. For any input file,
the file's repository wins. Only when the file directory is not within any Git
repository does discovery fall back to the invocation working directory. A
file inside a repository whose origin is absent, invalid, or unsupported does
not fall back. Stdin uses the invocation working directory. Discovery failure
is non-fatal. `none` performs no discovery. Markdown uses the result for
reference linking; all formats store it as marker and manifest metadata.

The CLI and upload client default repository context to `auto`. The direct
local-rendering API's zero-value repository option performs no discovery;
library callers opt in by passing `auto`. The lower-level renderer receives
only an already resolved canonical URL and never runs Git itself.

Unknown keys in the config file are an error naming the offending
key — typo protection, and it keeps the parser exactly in sync with
the published schema's `additionalProperties: false`.

If the config file contains credentials and is group- or
world-readable, a warning is printed to stderr.

Behavioral defaults: `template`, `collection_template`, `no_source`,
`indexable`, `no_external_assets`, `mermaid_url`, `repo`, and `timeout` may be
set at the root or profile level; their flags override the config
values.

`template` applies only to rendered documents. `collection_template` applies
only to collection overview pages. Configuring either does not cause it to be
loaded or validated during the other mode.

`no_external_assets` covers only airplan-managed view-time loads, including
Mermaid. It does not rewrite or block external content authored in trusted
Markdown, HTML, or custom templates. `mermaid_url` may point at another CDN or
self-hosted compatible module; an empty direct library option uses the built-in
exact pin.

`public_base_url` is strongly recommended whenever the endpoint URL
isn't itself publicly readable (always the case for R2). If unset,
the URL is assembled as `<endpoint>/<bucket>/<key>` (path-style) and
a warning is printed to stderr noting it may not be publicly
reachable.

Validation before an operation reports missing fields for the selected backend
and resolved profile. Local S3 services initialize storage lazily, so
manifest-only listing and purge preview can work without storage credentials;
every storage-dependent operation validates readiness before reading input or
mutating state. `serve` validates storage before it starts listening.

### Resolved config inspection

`airplan config show` prints the resolved configuration without accessing the
network, resolving the standard AWS credential chain, validating storage
completeness, or writing local state. It accepts `--config`, `--profile`, and
the same config override flags as an upload. Those flags describe the current
inspection invocation; flags from an earlier process cannot be observed.

The default table reports the selected config path, active profile, credential
mode, and every config field's resolved value and winning source. Sources are
one of a built-in default, root config key, selected-profile config key,
`AIRPLAN_*` environment variable, or explicit flag. Config-path and profile
rows likewise distinguish flag, environment, default path/profile, and
profile inference. Root-level selection made complete by any combination of
root config, environment, and flags is described as a complete root-level
resolution. Unset fields remain visible as `<unset>`.

`--json` returns one object with `config_file`, `profile`, `credential_mode`,
and `fields`. Each field object contains `value`, `set`, `sensitive`, and
`source`; each source contains stable `kind` and `name` strings plus optional
`path` and `profile`. Source kinds are `builtin`, `config_root`,
`config_profile`, `environment`, `override`, and `inferred`. Root profile
selection is represented by `name: null` and `root: true`. Field order is not
significant in JSON.

`access_key_id` and `secret_access_key` values are always redacted. The table
prints only `<set>` or `<unset>`; JSON always uses `value: null` together with
the `set` and `sensitive` booleans. When neither is explicitly configured,
credential mode reports the standard AWS chain without attempting to resolve
it. When both fields are configured, the human-readable credential mode is
`explicit access keys`. Endpoint values remain visible.

Incomplete endpoint, bucket, or credential settings are displayable because
inspection is diagnostic. Errors that prevent deterministic resolution still
fail the command, including malformed TOML, unknown keys, invalid parsed
environment values, a missing explicit config path, or an invalid/ambiguous
profile selection. Config load warnings go to stderr; inspection output goes
to stdout.

### Config JSON Schema

The config file format is described by a published JSON Schema that
must exactly match what the tool accepts (it may not drift from the
parsing code).

- `airplan config schema` prints it to stdout.
- The schema file ships with releases.
- Editors get validation/autocomplete via Taplo or Even Better TOML
  with a `#:schema` directive in the config file pointing at the
  released schema URL.

---

## 8. URL / Key Generation

Privacy model: **capability URL**. Anyone with the link can read the
plan; no one without it can find it. Requirements: enough entropy to
be unguessable at internet scale, URL-safe, robust to case-folding
(chat apps, email clients, and some proxies lowercase URLs).

Scheme:

```
# document
[<key_prefix>/]<random>/.airplan.json
[<key_prefix>/]<random>/<slug>.html
[<key_prefix>/]<random>/<slug>.md      (markdown input, unless
                                        --no-source)
[<key_prefix>/]<random>/<slug>.<ext>   (text input's original file,
                                        unless --no-source; <ext>
                                        per §3)

# collection
[<key_prefix>/]<random>/.airplan-collection.json
[<key_prefix>/]<random>/<member basename>  (one per input)
[<key_prefix>/]<random>/index.html
```

Each upload owns one random directory. Exactly one valid kind-specific marker
establishes Airplan's authority over everything under that directory; filename
shape without a marker never establishes ownership. Both marker names create a
conflict and grant no authority. Management commands treat the directory as
one deletion unit, so page, source or members, marker, extras, and any
partial-upload remnants never get separated.

- `<random>`: 16 bytes from a cryptographically secure random source
  (never a seeded PRNG), encoded lowercase base32 (RFC 4648
  alphabet, no padding) → 26 chars, **128 bits** of entropy.
  Lowercase-only sidesteps case-folding corruption that
  base62/base64 URLs suffer; 128 bits makes brute-force enumeration
  (even with no rate limiting) computationally absurd.
- `<slug>`: document-only human-readable filename portion so links look sane in
  chat and downloads name themselves. From `--slug`, else the source
  filename stem, else `plan`. Sanitized: lowercased, non
  `[a-z0-9-]` → `-`, collapsed, trimmed, max 64 chars; if
  sanitization leaves an empty string (e.g. an all-non-ASCII
  filename), fall back to `plan`. Contributes zero entropy by
  design — privacy never depends on it.
- Collection uploads have no slug. Their primary page is always `index.html`,
  and member basenames provide the human-readable direct URLs. Names are
  percent-encoded when assembled into public URLs but remain unencoded object
  key segments in storage, JSON, and manifest data.
- `.html` extension: helps any static host / CDN infer content type
  and makes saved files open correctly.

Example keys:

```
vq3nhk2p7r4wzt5c6ydjm3xhqd/.airplan.json
vq3nhk2p7r4wzt5c6ydjm3xhqd/refactor-auth.html
vq3nhk2p7r4wzt5c6ydjm3xhqd/refactor-auth.md
gaj4jmvi6dverjkoy6khas2ble/.airplan-collection.json
gaj4jmvi6dverjkoy6khas2ble/Screenshot 1.png
gaj4jmvi6dverjkoy6khas2ble/index.html
```

Final URL: `<public_base_url>/<key>`, with each key path segment
percent-encoded. The object key stored in S3 and exposed in JSON or
manifest records remains unencoded.

Explicitly rejected: hash-of-content keys (deduplication leaks
whether a document was already uploaded, and shorter hashes invite
truncation), sequential or timestamped keys (guessable), user-chosen
full paths (footgun).

---

## 9. History & Cleanup

No TTL / server-side lifecycle rules: on R2 they require bucket-admin
credentials to manage and even to verify, which conflicts with the
minimal object-scoped tokens agents should hold. Cleanup is instead
client-driven — off the local manifest, or off a live bucket listing
with `--remote` — using the same credentials as uploads (the
object-scoped token covers `GetObject`, `DeleteObject`, and
`ListObjectsV2`; public listing stays blocked either way).

### Local manifest

Every upload is recorded in
`$XDG_STATE_HOME/airplan/manifest.jsonl` (platform-appropriate state
directory on Windows), or the path selected by the global manifest option —
append-only JSONL, one record per line.
Deletions and remote-absence reconciliation append tombstone records;
the file is never rewritten in place. Uploads made on other machines
can be imported explicitly with `sync`; the manifest remains a local
projection rather than remote authority. A same-user local CLI and
`airplan serve` share the default path. Consequently, `airplan list` on the
server host sees uploads made through its API when both select the same S3
profile and manifest path. Containers and services must mount that file on
persistent storage and select it explicitly when their OS state directory
differs.

Record schema — exact field names are part of this spec, so two
conforming implementations can share a manifest:

```json
{"type":"upload","time":"2026-07-21T12:00:00Z",
 "key":"vq3n.../plan.html","source_key":"vq3n.../plan.md",
 "marker_key":"vq3n.../.airplan.json",
 "url":"https://plans.example.com/vq3n.../plan.html",
 "bucket":"plans","profile":"work","kind":"document",
 "slug":"plan","format":"md",
 "title":"Refactor auth","repo":"https://github.com/acme/service",
 "bytes":18432,"marker_version":3}
{"type":"upload","time":"2026-07-21T12:03:00Z",
 "key":"gaj4.../index.html",
 "marker_key":"gaj4.../.airplan-collection.json",
 "url":"https://plans.example.com/gaj4.../index.html",
 "bucket":"plans","profile":"work","kind":"collection",
 "title":"login.png and 1 more","bytes":9216,"marker_version":3}
{"type":"delete","time":"2026-07-09T09:12:44Z",
 "key":"vq3n.../plan.html","marker_key":"vq3n.../.airplan.json",
 "bucket":"plans","profile":"work","reason":"deleted"}
```

(Shown wrapped for readability; on disk each record is one line.)

- `time` is RFC 3339, UTC.
- `upload` records: `kind` is `document` or `collection`. `slug` and `format`
  are present for documents and omitted for collections. `source_key` is
  document-only and omitted for HTML or under `--no-source`. `key` and `url`
  identify the primary page; `bytes` describes that page, not collection
  payload bytes. `title` is omitted when empty; `profile` is omitted for
  root-level settings. `marker_key` is the exact kind-specific ownership key.
  `repo` preserves canonical repository metadata. The full collection
  inventory remains only in the remote marker.
- Current writers always include `marker_version: 3`; its absence identifies
  legacy pre-marker history. Readers infer `kind: document` and derive its
  slug from the page key for valid older records that omit those fields.
- New `delete` tombstones include `marker_key`, `bucket`, the receiving
  `profile`, and reason `deleted` or `remote_missing`. Their identity is
  `(bucket, marker_key)`. Legacy key-only tombstones remain valid.
- Manifest history reduces chronologically. The latest event for an upload
  identity wins, duplicate uploads collapse to their latest record, and a
  later upload reactivates an earlier tombstone. A legacy key-only tombstone
  hides matching preceding uploads but not a later upload event.
- Forward compatibility: readers ignore unknown fields and skip
  records with an unknown `type`. The record itself needs no schema
  version; `marker_version` describes the remote upload format.
  Readers retain an otherwise-valid upload with no `marker_version`
  as legacy history, but it never authorizes delete or purge. An
  unsupported nonzero `marker_version` is invalid and skipped with a
  warning. Marker versions 1, 2, and 3 are managed; pre-marker entries remain
  visible as read-only legacy history and are never pruned by `sync`.

Concurrent invocations are expected (parallel agents on one
machine) and must be safe:

- Each record is written as a single append — one write of the full
  line, trailing newline included — to a file opened in append
  mode.
- Appends are wrapped in an advisory file lock (`flock` /
  `LockFileEx` style). All writers are airplan, so advisory
  suffices; the lock removes reliance on append atomicity, which
  doesn't hold on network filesystems. Waiting for the lock is part
  of the invocation and must stop when its context or configured
  timeout expires; manifest locking can never create an unbounded
  wait.
- Readers tolerate a torn, malformed, or oversized line by skipping
  it with a warning on stderr — never by failing, never losing the
  rest of the file. Implementations may bound retained bytes per line,
  but must discard through the next newline before resuming.
- Never rewriting in place (tombstones, not deletion) means there
  is no read-modify-write cycle to race on.

### Commands

- `airplan list`: past uploads from the manifest (date, profile,
  management state, title, human-readable binary size, URL); `--json`
  for scripting with exact byte counts. Table state is `managed` for
  the supported `marker_version` and `legacy` when the field is absent.
  Both appear in history without warning; legacy entries remain
  ineligible for delete reconciliation and purge.
- With no resolvable configuration or backend selection, `list` assumes `s3`
  and reads the resolved local manifest without requiring storage credentials.
  Local S3 listing with no explicit profile shows every recorded profile; an
  explicit `--profile NAME` filters that exact profile, and `--profile=`
  selects root-level history. If configuration selects an `airplan` profile,
  `list` calls the server's manifest endpoint. `--config` is therefore valid
  for non-remote list because it can select the HTTP backend.
- `airplan list --remote`: cheaply discovers marker directories made from any
  machine. It performs only paginated bucket LIST operations beneath the
  active profile's `key_prefix`; it does not GET markers, HEAD pages, or trust
  marker content. It groups every returned object beneath an exact
  `[key_prefix/]<26-char lowercase base32>/` directory, then emits groups
  containing `.airplan.json`, `.airplan-collection.json`, or both. Payload
  filename shape without either marker is never evidence of visibility.
- Remote list rows have `DATE`, `KIND`, `OBJECTS`, `SIZE`, `SLUG`, `DIRECTORY`,
  and `URL` columns. `DATE` is the selected marker object's storage
  last-modified time. `OBJECTS` and `SIZE` count every object and byte
  recursively beneath the random directory, including the marker,
  nested keys, and unrecognized extras. `KIND` is `document` or `collection`
  from the exact marker basename, and remains an untrusted hint.
  `.airplan.json` retains the existing unambiguous direct-child HTML inference
  for `SLUG`, key, and URL. `.airplan-collection.json` leaves `SLUG` empty and
  selects an exact direct-child `index.html` as its key and URL even when other
  HTML members exist. With both marker names, `KIND` is `conflict` and page,
  slug, and URL inference is suppressed. No marker or page request is made.
  URL fallback without `public_base_url` emits the normal warning once.
  `DIRECTORY` is the 26-character random directory without
  `key_prefix`. Rows sort by marker last-modified time, then marker
  key.
- `list --remote --json` prints an array with one object per row. Its stable
  fields are `time`, `dir`, `marker_key`, `objects`, `bytes`, and `kind` when
  one marker kind is implied. `conflict` is true for dual-marker directories;
  `slug`, `key`, and `url` appear only when inferred. These entries describe
  marker-key presence and occupancy, not validated uploads.
  A malformed, oversized, or unsupported marker remains visible here
  because ordinary remote listing never reads it.
- `airplan show <url|key>` performs targeted inspection of one remote marker
  directory. The target may be its random directory, either marker name, or
  any direct child. `show` lists the directory, requires exactly one ownership
  marker, fetches and validates it, and reports every declared object's
  existence and size plus total directory object count and bytes. A valid
  marker is `complete` only when every declared object exists with its declared
  size; otherwise it is `incomplete`. Extra objects do not affect state. A
  present invalid marker, including a dual-marker conflict, produces a
  successful `invalid` inspection but grants no authority. A missing marker is
  an error. Storage, authentication, timeout, cancellation, and other request
  failures fail the command rather than becoming marker states.
- `show --json` emits one object. All states contain `state`, `dir`,
  `marker_key`, `objects`, and `bytes`. Valid states additionally
  contain `time`, `kind`, `marker_version`, `page`, `title` when non-empty,
  and `repo` when present; documents also expose `format` and optional
  `source`, while collections expose an ordered `files` array. Declared object
  entries contain `key`, `url`, `exists`, `expected_bytes`, and `bytes`, with
  `bytes` omitted when missing. An invalid result
  additionally contains `error`, a stable coarse code:
  `oversized`, `malformed_json`, `unsupported_version`, `invalid_fields`, or
  `conflicting_markers`; it never exposes untrusted marker fields. Human
  output presents the same information as a labeled detail block.
- `airplan get <url|key>` fetches one object from a marker-managed upload.
  Full URLs, bare keys, random directories, configured prefixes, and
  path-style endpoint URLs obey the same connection, bucket, and prefix
  rules as `delete`. Before returning bytes, `get` concurrently probes both
  exact marker keys, requires one to exist and the other to be confirmed
  absent, and validates the existing marker. This preserves object-read-only
  credentials without requiring LIST permission. A timeout, authorization
  failure, or ambiguous probe fails closed. A random-directory target selects
  the primary page, or the document source under `--source`; requesting source
  from a collection or source-less document is an error. An explicit declared
  page, document source, collection file, or existing marker fetches that exact
  object. Any undeclared child is rejected, as is `--source` with an explicit
  child. A missing selected object is an error naming its full key.
  Raw fetched bytes, with no added newline or other output, go to stdout by
  default. `--output PATH` writes the complete bytes to a temporary file
  beside the destination and atomically renames it into place; `--output -`
  is equivalent to stdout. Written files are user-only (0600 on POSIX
  systems); fetched bytes are not shared with other local users by
  default. Payload download streams to its destination so large recordings do
  not require whole-object buffering. `get` never writes the local manifest or
  changes remote storage.
- `airplan delete <url|key>` only deletes a marker-managed upload. The target
  may be the random directory, its existing marker, or any page, source, or
  collection file declared by the valid marker. Other siblings are rejected.
  Before any deletion, Airplan resolves exactly one marker by the same
  fail-closed dual probe as `get` and validates it. Missing, conflicting,
  malformed, oversized, unsupported, or inconsistent ownership touches no
  bucket objects. Native storage tooling is the escape hatch.
  Full URLs must use HTTP(S) and match the configured public base URL
  or endpoint by host and base path; HTTP and HTTPS variants of the
  same host are equivalent because the URL is parsed, not fetched. A
  path-style endpoint URL must contain the configured bucket as its
  exact bucket path segment — a missing or different bucket is an
  error. Bucket-only URL parsing is allowed only when neither
  connection URL is configured.
- A valid marker authorizes deletion of every object under its own
  random directory, including incomplete-upload remnants and
  unrecognized extra siblings. Deletion removes every non-marker
  object first. Only after all payload deletions succeed is the marker
  deleted in a separate final operation. Any payload or marker failure
  leaves the local upload untombstoned so retry can resume while the
  marker still establishes ownership. A successful marker deletion is
  followed by the append-only local tombstone.
- Before `show`, `get`, or `delete` resolves its connection, it consults local
  history for exactly one matching active, marker-managed manifest record.
  When neither
  `--profile` nor `AIRPLAN_PROFILE` is set and that record names a
  profile, the recorded profile overrides the general config default;
  stderr notes the selection. URL targets participate in this inference
  only when they are HTTP(S) URLs whose host matches the recorded public
  URL; URL query strings and fragments are ignored. With zero or multiple
  matching records, normal config resolution proceeds without inference.
  A collection history record may match any direct child beneath its recorded
  random directory after the same host checks. This selects connection context
  only; the remote marker must still declare the requested target before read
  or deletion authority exists.
  Explicit flag or environment selection always wins and is never silently
  changed. An inferred profile removed from the selected config is an
  actionable selection error. Missing, unreadable, or ambiguous
  history falls back to normal config resolution. Remote marker validation
  remains authoritative. For `delete`, if marker lookup then fails and the
  matching record names another
  profile, stderr warns that the mismatch may be the cause and identifies
  both `--profile` and `AIRPLAN_PROFILE` as retry mechanisms. When the record
  used root-level settings but named-profile resolution is active, the hint
  instead directs the user to a config path that resolves root-level settings.
- There is one narrow ensure-gone reconciliation path for a marker
  deletion that succeeded before its local tombstone could be written.
  When the marker is absent, airplan may append a tombstone without
  issuing any S3 deletion only if an active local upload record names
  the same page directory, has a supported `marker_version`, and
  matches the active bucket and profile. Invalid unrelated lines do not
  mask a complete matching record; they remain relevant when no such
  record can be established. If the manifest is missing, unreadable,
  lacks a complete matching record, or belongs to another connection,
  deletion fails. This exception repairs local history; it never grants
  authority to delete unmarked bucket objects.
- `airplan purge`: bulk delete driven by the manifest with filters —
  `--older-than 30d`, `--slug PATTERN`, `--profile P`. Durations
  accept `d`/`w` units. `--profile`/`-p` behaves as on every other
  command by selecting the connection profile. Local purge always
  considers only uploads recorded with the resolved active profile,
  whether it came from `--profile`, `AIRPLAN_PROFILE`,
  `default_profile`, single-profile inference, or root-level config.
  Thus a profile's uploads are only purged with that profile's
  connection and credentials.
  `--slug PATTERN` applies only to documents. Collections have no slug and are
  excluded even from `--slug '*'`; age, profile, scope, or `--all` select them.
  Member filenames and collection `index.html` are never reinterpreted as
  slugs.
  Requires at least one filter or an explicit `--all`. `--dry-run`
  previews; confirmation prompt unless `--yes`. EOF before an answer
  is an error that directs non-interactive callers to use `--yes`; an
  explicit negative answer remains a successful abort.
  Failed deletes are reported to stderr and left un-tombstoned so a
  re-run retries them. Purge only considers records with a supported
  `marker_version` under the active bucket and `key_prefix`;
  other-bucket and other-prefix records are skipped with a note.
  Every selected deletion still requires the marker, except for the
  local-only ensure-gone reconciliation above. Suitable for cron
  (`purge --older-than 30d --yes`).
- `purge --remote` starts from the same marker-key candidates as
  `list --remote`, but fetches and validates markers because it is a
  destructive operation. It may select both `complete` and
  `incomplete` uploads, using marker `created_at` for `--older-than`.
  `--slug` selects documents only and uses the marker-declared slug even if the
  page is missing. It never selects an invalid marker or marker conflict. Such
  a directory cannot
  be deleted by airplan; `show` can inspect it and native storage
  tooling must clean it. Marker-last deletion keeps an interrupted
  purge discoverable and retryable.
  Marker inspection is concurrent with a default limit of 8 and accepts
  `--concurrency N` from 1 through 64. The flag is rejected without
  `--remote`. Candidate order remains deterministic and confirmed
  deletions remain sequential.
  In a team bucket, each person sets their own `key_prefix`, which
  keeps `--remote` scoped to their own uploads.
- `airplan sync` reconciles the selected profile, bucket, and key prefix
  into the local manifest. One paginated LIST snapshot supplies remote
  marker candidates and object sizes. Missing local candidates have their
  markers fetched concurrently and are imported only when the supported
  marker validates and every declared object is present at its declared size.
  Imported v3 records retain kind, exact marker identity, primary page,
  document slug/format/source where applicable, title, and repository, but do
  not duplicate collection inventories. Imported profile,
  bucket, and public URL values come from the receiving machine's resolved
  connection, never the marker.
  By default, active scoped local records absent from LIST are considered for
  pruning, but airplan performs a targeted marker GET before appending a
  `remote_missing` tombstone. Only a definite not-found response confirms
  absence. A returned marker is retained regardless of its contents; timeout,
  authentication, transport, and ambiguous storage errors retain the record
  and fail the sync partially. `--no-prune` makes sync additive-only.
  `--concurrency N` defaults to 8 and accepts 1 through 64 across marker
  fetches and absence confirmations. `--dry-run` performs the same remote
  validation without locking or writing the manifest. Network inspection does
  not hold the manifest lock; before appending, sync locks, rereads, reduces,
  and rechecks local state, then writes deterministic newline-terminated
  records. Per-item failures do not discard successfully validated progress.
  Human output and warnings use stderr while stdout remains empty. `--json`
  emits exactly one object on stdout with deterministic `added_records`,
  `tombstone_records`, and `failures` arrays plus `unchanged`, `incomplete`,
  `invalid`, and `retained` counters. A partial failure exits nonzero after
  writing the result. Sync provides eventual active-inventory convergence;
  it neither uploads deletion history nor makes historical JSONL files
  identical across machines.
- The local manifest still matters: it remembers titles and profile
  context, and works offline. Remote listing is the cheap storage view;
  `show`, `get`, `delete`, and `purge --remote` read marker state when they
  need validated upload details, read authority, or deletion authority.

---

## 10. Security & Privacy Notes

- Unguessable ≠ private-forever: URLs shared into third-party chat
  tools may be scanned/prefetched by those tools, and objects stay
  in the bucket until deleted. `airplan purge --older-than 30d`
  (manual or cron) is the cleanup story; document both caveats
  prominently.
- Bucket policy: object-read via public domain only; no
  `ListBucket` on any public principal. R2 custom-domain setup gets
  this right by default — documentation covers verification steps.
- Credentials: recommend R2 API tokens scoped to a single bucket at
  the Object Read & Write level (covers upload, and the list/delete
  that management commands need — never bucket-admin); never log
  credentials; redact endpoint account IDs from error output where
  feasible.
- Key generation must use a cryptographically secure random source —
  never a seeded/insecure PRNG.
- Markdown rendering preserves raw HTML and link destinations, and HTML
  input is uploaded as authored. Both may execute active content, so
  only share documents from trusted sources.
- Collection members are uploaded byte-for-byte. HTML and SVG members may
  execute active content when opened, while media content types intentionally
  allow browsers and external proxies to render the originals. Only upload
  trusted artifacts.
- Screenshots and recordings may reveal tokens, usernames, private messages,
  browser chrome, or unrelated desktop content. Review captures before upload.
  Filenames are also exposed in direct public URLs and the overview page.
- The generated collection overview HTML-escapes authored metadata and builds
  relative URLs only from validated direct basenames. It never interpolates
  filenames, titles, content types, or repository data as raw HTML.

---

## 11. Backends, HTTP Server, REST API, and MCP

### Backends and operation ownership

Airplan has two product backends:

- `s3` invokes the S3-backed operation service in-process. That process owns
  rendering, storage access, and the selected local manifest.
- `airplan` transports the same operation API over HTTP to `airplan serve`.
  The server owns rendering, S3 credentials, and its selected manifest.

Backend-sensitive CLI operations have identical intent under either backend:
upload, `list`, `list --remote`, `show`, `get`, `delete`, purge preview and
execution, and `sync`. `list` means the operation service's manifest;
`list --remote` means a direct storage-marker listing. An HTTP client never
appends local upload or tombstone records. Server REST and hosted MCP adapters
invoke the server's operation service directly rather than calling loopback
HTTP or duplicating business rules.

For an `airplan` backend, request attributes such as format, title, slug,
language, repository URL, and lower size limits remain portable. Explicit S3
connection overrides and server-owned rendering policy flags are rejected
before input is opened. Inherited settings remain inactive as described in
§7; a client cannot choose the server's endpoint, bucket, key prefix,
templates, source policy, indexability, or Mermaid policy.
Raw REST and hosted MCP requests that omit repository context disable
repository discovery. Hosted requests reject `auto` and accept only `none` or
a normalizable explicit repository URL, so the server never falls back to
inspecting its own working directory or caller-named filesystem paths.
Document names are optional for stdin-style REST clients and contain at most
255 Unicode characters when present.

The server's manifest listing is scoped to its resolved S3 profile, bucket,
and key prefix even when its file also contains records for other local
profiles. The ordinary local S3 `list` without a profile remains an
all-profile view. `serve` requires an `s3` profile and rejects an `airplan`
profile, preventing proxy chains and loops.

### Server process

`airplan serve` runs one single-user HTTP server. Its server-specific options
are:

- `--listen`, default `127.0.0.1:8080`.
- `--allow-non-loopback`, the required acknowledgement for a non-loopback
  listener.
- `--token-file`, with `AIRPLAN_SERVER_TOKEN` as the alternative token source.
- repeatable `--allowed-origin` values for hosted MCP Origin validation.
- `--temp-dir` for bounded collection-upload spooling.
- `--log-level`, with `AIRPLAN_SERVER_LOG_LEVEL` as its fallback. An explicit
  flag wins. Accepted values are `error`, `warn`, `info`, `debug`, and `trace`;
  the default is `info`.

Exactly one non-empty server-token source is required. A token should contain
at least 32 random bytes; token files should be mode 0600. The token is read
once at startup. The server defaults to loopback. Binding to a non-loopback
address requires explicit acknowledgement, and TLS must terminate at a trusted
reverse proxy. The built-in server does not manage certificates.

Server logs are line-oriented text on stderr only. At `info`, the process
prints its existing listening line and otherwise remains quiet except for
server failures. The listening line is also present at `debug` and `trace` but
is suppressed at `warn` and `error`. `debug` adds completed REST and MCP
requests with transport, allowlisted method, safe route path, status, duration,
and a server-generated request ID; bearer rejection reasons; Origin and
size-limit rejections; and MCP tool name, outcome, duration, and safe failure
class. `trace` additionally adds request starts, MCP protocol method lifecycle,
and sanitized SDK lifecycle events. Trace is more verbose than debug and is
rendered as `TRACE`.

Authentication rejection reasons may distinguish missing, duplicate,
wrong-scheme, malformed-shape, and mismatched credentials in local debug logs,
while every client still receives the same generic authentication response.
Incoming request-ID values are ignored rather than reflected. No level logs raw
HTTP or MCP bodies, Authorization values, tool arguments or results, upload
content, capability URLs or keys, S3 response bodies, endpoints, buckets,
credentials, token metadata, or filesystem paths.

`serve` validates its S3 readiness before listening, uses bounded HTTP header
and idle timeouts, and shuts down gracefully on SIGINT or SIGTERM. It is a
single-instance service: only one active server may own a manifest. Local CLI
processes on the same host may share that file through its existing locked
append protocol, but separate replicas with independent files are unsupported.

### REST wire contract

`api/openapi.yaml` is the authoritative OpenAPI 3.0.3 contract. The exact
checked-in schema is embedded and returned by `GET /openapi.yaml`. Compatible
changes remain under `/api/v1`; breaking changes require a new URL version.
`GET /healthz` and `GET /openapi.yaml` are unauthenticated. The following
endpoints require `Authorization: Bearer <token>`:

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

Document and collection uploads use bounded streaming
`multipart/form-data`. The server applies the stricter of its hard limits and
portable client-requested limits. Collection members are spooled to temporary
files, mode 0600 on platforms with POSIX permission bits, so the existing
seekable collection API can be used without whole-collection buffering; all
temporary files are removed after success, failure, cancellation, or shutdown.

Inspect, get, and delete take `url_or_key` in a JSON request body. The server
resolves it against its complete S3 configuration and permits only objects
declared by exactly one valid Airplan ownership marker. Get streams its
response with the stored object's content type. Capability URLs are not placed
in query strings. Upload, list,
inspection, and purge-preview results expose the randomized directory as an
opaque `upload_id`.

Purge is two-phase. `/purge/preview` applies the source and filters without
deleting and returns explicit `upload_id` candidates. The CLI displays them
and performs confirmation. `/purge` accepts only an explicit array of those
IDs, re-resolves and revalidates every current marker, attempts targets
sequentially, and reports every success or failure. It accepts no URL, key,
filter, or implicit-all execution request.

REST errors use RFC 9457 `application/problem+json` with stable `code` and
`request_id` fields. Authentication is checked before request bodies are
parsed. Missing, malformed, and incorrect bearer credentials receive the same
generic 401 and `WWW-Authenticate: Bearer`. Tokens, capability URLs, request
bodies, S3 response bodies, filesystem paths, and credentials must not appear
in logs, error details, warnings, or per-item failure text. Hosted structured
results use stable generic messages where internal detail would otherwise be
exposed. Upload POSTs are not retried automatically because a
timeout after server commit is ambiguous without persistent idempotency state.

### MCP servers

`airplan mcp` is a stdio MCP server. It constructs the normal public client,
so it works with either backend. MCP frames are its only stdout content;
warnings and logs use stderr. `airplan serve` exposes the same tool
implementation at `/mcp` using MCP Streamable HTTP. Deprecated HTTP+SSE is not
supported.

The minimal tool set is:

| Tool              | Stdio | HTTP | Effect                             |
| ----------------- | ----- | ---- | ---------------------------------- |
| `upload_document` | yes   | yes  | Upload supplied text content       |
| `upload_files`    | yes   | no   | Upload local paths as a collection |
| `list_uploads`    | yes   | yes  | List manifest or storage records   |
| `inspect_upload`  | yes   | yes  | Validate one marker-managed upload |
| `delete_upload`   | yes   | yes  | Delete one explicit upload         |
| `sync_manifest`   | yes   | yes  | Preview or apply reconciliation    |
| `preview_purge`   | yes   | yes  | Return explicit purge candidates   |
| `execute_purge`   | yes   | yes  | Delete reviewed upload IDs         |

Hosted MCP omits file collection upload because MCP has no portable
client-to-server file upload and server-local paths are unsafe. No transport
exposes template dumping, configuration inspection, credentials, server
configuration, arbitrary S3 objects, or filesystem browsing.

`sync_manifest` defaults to dry-run unless `apply: true` is explicit.
`preview_purge` never deletes, and `execute_purge` accepts only explicit
`upload_id` values. Tool results are structured and warnings remain inside the
result rather than corrupting protocol framing. Partial sync or purge failures
set the MCP error indicator while retaining the structured progress result.
The configured operation timeout applies independently to each MCP tool call;
it does not limit the lifetime of a stdio MCP session.

The Streamable HTTP endpoint uses the same bearer token as REST. This is a
custom single-user mechanism, not MCP OAuth; clients unable to add an
Authorization header are unsupported. A present `Origin` header must exactly
match an allowed origin or receives 403. An absent Origin is accepted for
non-browser agent clients, and the default allowlist is empty. Streamable HTTP
POST bodies are limited to 61 MiB, enough for the maximum JSON-escaped default
document input plus bounded protocol metadata; oversized bodies receive 413.
