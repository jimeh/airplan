# airplan — Tool Specification

**Spec version: 0.40.0**

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
- Fenced code blocks are syntax-highlighted at render time. Each catalog theme
  has an exactly scoped Chroma stylesheet. Built-ins use their curated Chroma
  style; custom themes either derive a coherent style from their semantic
  tokens or select an exact registered Chroma style. Print always uses Chroma's
  `github` style independently of the screen theme.
- An exact lowercase `mermaid` fenced code block is rendered as a Mermaid
  diagram. Its readable, HTML-escaped source remains the no-JavaScript and
  load-failure fallback and remains exact in source view. The built-in page
  loads Mermaid only when such a block exists and external assets are allowed,
  using an exact pinned ECMAScript module URL, strict security, explicit
  rendering, and `theme: "base"` with explicit variables derived from the
  resolved Airplan theme. At startup it serially renders and caches only the
  themes assigned to the two mode slots plus a fixed GitHub Light print result;
  newly selected themes are rendered lazily and stale asynchronous results are
  ignored. A failed theme retains the last valid SVG, or readable source when
  none has succeeded, without poisoning other themes. After a diagram renders
  successfully, the built-in page adds an
  explicit control that opens its currently displayed SVG in one reused native
  dialog. Inline diagrams remain passive. The viewer initially fits the
  diagram without enlarging it beyond its natural size, supports 5%–800% zoom
  through buttons, pointer-anchored wheel input, and `+`/`-`/`0` keys, and
  supports drag and arrow-key panning with a larger Shift step. Escape,
  backdrop, and close controls dismiss it and restore focus. Theme changes
  refresh an open viewer without resetting its zoom or pan. The control is
  always visible on touch input and otherwise appears on hover or focus; the
  control and dialog are hidden in print. No viewer is created when Mermaid
  loading or rendering fails, external assets are disabled, scripting is
  disabled, or native modal dialogs are unavailable. Custom templates receive
  the Mermaid template data below but do not receive injected assets.
- Page styling separates color mode from theme selection. Mode is System,
  Light, or Dark; System follows `prefers-color-scheme`. Light and Dark are
  independent slots, and either slot may contain a light- or dark-variant
  theme. The selected theme's declared variant controls `color-scheme`, native
  controls, and scrollbars. The page uses a centered
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
- Every built-in Markdown page contains dormant revision discovery. It
  requests the same-directory `.airplan-versions.json` path with
  `cache: no-store` and a unique per-page-load query nonce. HTTP 404 means the
  upload is standalone and is not an error. Other failures or invalid metadata
  disable only revision navigation. A regular upload does not create this
  control object; the linked-revision feature creates it only when revision 2
  is uploaded. Custom templates do not gain this bootstrap automatically.
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
  - Appearance settings: one icon-only accessible trigger opens a compact
    panel with a System/Light/Dark native mode control and independent Light
    mode theme and Dark mode theme selects. Both selects contain the identical
    catalog grouped into Light themes and Dark themes; variant never filters a
    slot. Escape and outside activation dismiss the panel, Escape restores
    focus, and narrow layouts keep it inset from viewport edges. With scripting
    disabled, uploader defaults follow the system preference and the panel is
    absent. The appearance trigger follows the file controls. At wider sizes the rendered/source
    toggle aligns left while file controls align right, with the theme toggle
    at the far-right edge behind a quiet divider. At 48rem and below the
    rendered/source and appearance controls share the first row at opposite edges,
    with available file controls clustered and left-aligned below. When no
    rendered/source toggle is available, the file controls instead occupy the
    first row opposite the appearance control. Toolbar controls update immediately
    without color or background transitions when their active state or the
    page theme changes. Reader state uses `airplan-color-mode`,
    `airplan-light-theme`, and `airplan-dark-theme`. When the new mode key is
    absent, `airplan-theme=light|dark` seeds it. Explicit new-mode writes mirror
    that legacy key; System removes both mode keys. Unknown stored theme IDs are
    retained but inactive on pages whose catalogs do not contain them.
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
  - Successfully rendered Mermaid diagrams expose the native zoom/pan viewer
    described above. Its near-full-viewport canvas follows the selected light
    or dark page theme, adapts its controls for narrow screens, and removes its
    control fade when reduced motion is requested.
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
| `.Revision`                   | integer   | this page's revision, or zero        |
| `.RevisionCount`              | integer   | greatest live revision when rendered |
| `.PreviousRevision`           | integer   | adjacent predecessor, or zero        |
| `.VersionsPath`               | string    | relative versions metadata path      |
| `.DiffPath`                   | string    | relative adjacent diff path          |
| `.DiffText`                   | string    | inline adjacent diff, or empty       |
| `.HighlightedDiffHTML`        | raw HTML  | highlighted inline adjacent diff     |
| `.ThemeCSS`                   | raw CSS   | validated semantic screen/print CSS  |
| `.ThemeCatalogJSON`           | safe JS   | validated browser catalog metadata   |
| `.DefaultLightTheme`          | string    | configured light-slot theme ID       |
| `.DefaultDarkTheme`           | string    | configured dark-slot theme ID        |

Each heading has `.Level` (1–6), `.ID`, `.Text`, and `.IsTitle`.
`.IsTitle` is true only for a leading H1 that the built-in table of
contents omits. `.TOC` is structured data, not pre-rendered navigation
HTML, so custom templates retain control of markup and presentation.

`.SourcePath` is empty when the source isn't uploaded
(`--no-source`); templates must handle both cases.

A custom template takes full responsibility for the page: page styles,
noindex meta, theme UI/runtime, and any interactivity. Theme fields are safe
opt-in data; Airplan never injects its theme assets into a custom template.
`.SyntaxCSS` is supplied because it is coupled to the generated highlighting classes; the built-in page's
own CSS and JavaScript are baked directly into its template rather than
exposed as data. `airplan template` prints a source-friendly, self-contained
representation of the built-in template to stdout. The executable may embed
an equivalent minified representation of those static assets. Saving the
printed output and passing it back via `--template` must work unchanged and
produce the same page behavior.

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
- the layout supports narrow and wide viewports plus the same mode, slot,
  catalog, persistence, and migration behavior as document pages; the
  collection toolbar and shared controls use the same structure,
  dimensions, spacing, and interaction styling as document pages; System
  follows `prefers-color-scheme`, while print is fixed GitHub Light;
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

- Every new upload writes ownership marker version 5. Readers continue to
  manage versions 1 through 4, but writers never emit them. Marker
  versions describe wire-schema generations; `kind` distinguishes documents
  from collections. Older clients fail closed on new v5 uploads.

- The exact marker basename supplies an untrusted LIST-only kind hint:

  | Kind         | Marker basename            |
  | ------------ | -------------------------- |
  | `document`   | `.airplan.json`            |
  | `collection` | `.airplan-collection.json` |

  Existing v1/v2 uploads remain documents under `.airplan.json`. Marker
  content remains authoritative. A modern marker whose `kind` disagrees with its
  basename is invalid. A directory containing both names has conflicting
  ownership declarations and grants no managed read or deletion authority.

- Markers are UTF-8 JSON uploaded with
  `Content-Type: application/json` and `Cache-Control: no-store`, and are at
  most 64 KiB. A v5 document marker is:

  ```json
  {
    "schema": "airplan-upload",
    "version": 5,
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
        "content_type": "text/html; charset=utf-8",
        "sha256": "5cd8d993f5ea9ad07d1290e673e32b6ae3e7078190cacceb5f3655071753b6e4"
      },
      {
        "name": "plan.md",
        "role": "source",
        "bytes": 4096,
        "content_type": "text/markdown; charset=utf-8"
      }
    ],
    "title": "Refactor auth",
    "repo": "https://github.com/acme/service",
    "producer": { "name": "airplan", "version": "0.8.0" },
    "render": {
      "generation": 3,
      "template": { "kind": "builtin" },
      "indexable": false,
      "no_external_assets": false,
      "mermaid_url": "https://cdn.jsdelivr.net/npm/mermaid@11.16.1/+esm",
      "themes": {
        "default_light": "github-light",
        "default_dark": "github-dark",
        "catalog_sha256": "6ef869946a1ccb9fc60de82d7643b0a5267d0a5e33c4ed0ba9b80ad98f18fb36"
      }
    }
  }
  ```

  A v5 collection uses the same declared-object model and records template
  kind `builtin_collection` or `custom_collection`:

  ```json
  {
    "schema": "airplan-upload",
    "version": 5,
    "directory": "vq3nhk2p7r4wzt5c6ydjm3xhqd",
    "created_at": "2026-07-21T12:00:00Z",
    "kind": "collection",
    "objects": [
      {
        "name": "index.html",
        "role": "page",
        "bytes": 9216,
        "content_type": "text/html; charset=utf-8",
        "sha256": "ec328863f41675b9216ed9c398c91162dbea954192fab7435048847a53a0a44b"
      },
      {
        "name": "login.png",
        "role": "file",
        "bytes": 184320,
        "content_type": "image/png"
      }
    ],
    "title": "Login flow",
    "repo": "https://github.com/acme/service",
    "producer": { "name": "airplan", "version": "0.8.0" },
    "render": {
      "generation": 3,
      "template": { "kind": "builtin_collection" },
      "indexable": false,
      "no_external_assets": false,
      "themes": {
        "default_light": "github-light",
        "default_dark": "github-dark",
        "catalog_sha256": "6ef869946a1ccb9fc60de82d7643b0a5267d0a5e33c4ed0ba9b80ad98f18fb36"
      }
    }
  }
  ```

- `schema`, `version`, `directory`, `created_at`, `kind`, `objects`, and
  `producer` are required in v4 and v5. `schema` is exactly `airplan-upload`;
  `producer.name` is `airplan`, and `producer.version` is the resolved release
  string or `dev`. `directory` matches the
  containing random directory; `created_at` is RFC 3339 UTC. `repo`, when
  present, is the canonical resolved HTTPS repository URL. Connection-local
  profile, endpoint, credentials, bucket, prefix, and public URL metadata are
  never stored. Generated pages also require `render`. Authored HTML has no
  render recipe. `generation` is a positive page-capability generation, not
  the marker or release version. Custom template recipes store kind `custom`
  or `custom_collection` and the lowercase SHA-256 of the exact template bytes;
  no local path or template source is stored. Version 5 generated-page recipes
  additionally require `themes`: the selected slot IDs and lowercase SHA-256
  of the canonical normalized catalog embedded in the page. Full custom theme
  definitions and local configuration paths are not stored. Version 4 recipes
  omit `themes`; current readers preserve their complete producer, render,
  revision, digest, and role data while upgrading them.

- `objects` is non-empty, has unique safe direct basenames, and contains
  exactly one positive-size HTML `page`. Every object declares `name`, `role`,
  `bytes`, and a syntactically valid normalized `content_type`. A document has
  a required valid `slug`, required `format` (`md`, `html`, or `txt`), no
  `file` objects, and at most one positive-size `source` following the existing
  document filename rules. A collection omits `slug` and `format`, uses
  `index.html` as its page, declares no source, and contains one through 100
  `file` objects whose sizes may be zero. Unknown roles or kinds are invalid.

  In v4 and v5, the page object additionally requires `sha256`, the lowercase
  64-hex-character SHA-256 of the exact uploaded page bytes. This durable
  content identity lets interrupted marker-first operations detect and repair
  an old page even when it has the same byte length as the replacement. Other
  objects omit it. Versions 1 through 3 treat `sha256` as an unknown field and
  never expose it as trusted marker data.

- Unknown marker fields are ignored for forward-compatible extensions.
  Duplicate field names, invalid UTF-8, malformed JSON, unsupported versions,
  unsafe or inconsistent filenames, invalid roles, sizes, content types, or
  repositories, and oversized markers are invalid. Unsupported markers remain
  visible to LIST-only discovery but cannot be inspected as valid, fetched,
  deleted, purged, or synced.

- Version 1 through 4 markers are decoded into the current declared-object model after
  their original wire rules validate. Version 1 omits `page_bytes` and `repo`;
  version 2 requires positive `page_bytes` and may include `repo`.

- Purge protection (§9) is declared by a sentinel object at the exact key
  `[key_prefix/]<dir>/.airplan-protected.json`. **Presence of the sentinel at
  that key is the entire protection contract**: an empty, malformed,
  oversized, or wrong-content-type body still means protected, so a partial
  write can never silently unprotect an upload. airplan writes the sentinel
  as UTF-8 JSON with `Content-Type: application/json` and
  `Cache-Control: no-store`:

  ```json
  {
    "schema": "airplan-protection",
    "version": 1,
    "created_at": "2026-07-25T12:00:00Z",
    "reason": "README demo link"
  }
  ```

  The body is advisory context only. `reason` is optional, at most 256
  Unicode characters, valid UTF-8, and free of control characters; readers
  drop reasons that violate any of these while the upload stays protected.
  Marker v4 and v5 reserve every direct basename beginning `.airplan-`, in addition
  to `.airplan.json`; collection member filenames and v4/v5 marker-declared object
  names must not use that namespace, or a member could forge a control object.
  The sole payload exception is `.airplan-changes.diff` with role `diff`, and
  only for a linked Markdown revision greater than 1; every other
  marker-declared `.airplan-` payload remains invalid.
  Existing v1-v3 marker validation retains its original filename rules. An upload
  created by an older build whose collection already declares this basename is
  invalid to protection-aware builds; it must be removed with the older client
  or directly through storage tooling after its ownership is verified. The
  sentinel is an ordinary extra object — it counts toward listed object and
  byte totals and never affects upload completeness.

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
`preview` and `get`, and `-A` is `list --all-profiles`. Connection overrides
stay long-only, as do `list`'s
table options `--columns`, `--wide`, and `--reverse` (§9); `-c` is already
`--config`.
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
Local purge starts one deletion budget after confirmation. Remote purge
receives one budget for listing and marker inspection, then a fresh deletion
budget after confirmation. Bulk upgrade likewise receives one planning budget
and a fresh execution budget after confirmation. This prevents human think
time from consuming a network budget and gives each phase the configured
opportunity to finish. Operations
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

### Document upgrades

`airplan upgrade <url|key>` upgrades one marker-managed, source-backed
Markdown document in place. Its page URL, source bytes, original `created_at`,
repository context, and purge-protection sentinel remain unchanged. The
operation re-renders the page and writes a complete marker v5 with producer,
theme identity,
render provenance. HTML, text, collections, and source-less Markdown are
ineligible. A marker produced by a newer renderer generation is never
implicitly downgraded. A custom-template document is eligible only when the
configured custom template has the marker-declared SHA-256. `--force` plus the
currently configured template choice explicitly authorizes replacing a stored
built-in or custom template recipe; `--template PATH` selects a custom
replacement and `--template=` selects the built-in template. Without force, a
template mismatch remains ineligible. Planning remains read-only even when a
configured template cannot be loaded or parsed; execution reports that error
before writing any object.

`upgrade --check` performs read-only classification. `--force` explicitly
re-renders an otherwise current target. Planning returns `upgradeable`,
`current`, `ineligible`, `invalid`, or `missing`, plus the current and target
marker, producer, and renderer versions. Execution always replans from live
storage before deciding to no-op or mutate. A fresh `current` classification
no-ops; `missing`, `invalid`, or `ineligible` refuses execution. A
still-upgradeable target must exactly match the submitted marker and page ETags,
and execution rechecks the source ETag and bytes before mutation. Planning fails
closed when the storage service omits a required ETag and rejects a fetched page
larger than 40 MiB rather than classifying truncated bytes.
It conditionally writes the marker first and page last; storage HTTP 409/412 is
a conflict that requires a new plan. The REST
API exposes the same condition as problem code `upgrade_conflict` with status 409. It verifies the
result before success and appends a manifest `upgrade` event. It never creates,
deletes, or modifies `.airplan-versions.json`.

Once schema, renderer, page-size repair, and force decisions are satisfied,
strictly comparable semantic producer versions provide a final classification:
an older producer is upgradeable, an equal producer is current, and a newer
producer is ineligible rather than implicitly downgraded. An optional leading
`v` is ignored. `dev` and other non-comparable producer strings are neutral and
therefore current unless another upgrade reason independently applies.

`airplan upgrade --all --dry-run` classifies active, deduplicated records from
the selected operation manifest. `--all` applies only the exact upgradeable
plans shown by that preview, with bounded `--concurrency` (default 4, range
1-32) and stable result ordering. Apply prompts on an interactive terminal;
non-interactive apply requires `--yes`. Independent failures do not cancel
later items, but any required upgrade failure makes the command non-zero.
Local S3 mode additionally accepts `--all-profiles`, which loads each named
profile from current configuration without requiring an active or default
profile and without letting `AIRPLAN_PROFILE` narrow that inventory. Every
participating profile must use the S3 backend; a mixed-backend inventory is
rejected before storage mutation. `--all-profiles` cannot be combined with an
explicit `--profile`. Records fail closed when profile
configuration is missing or has drifted. Hosted mode remains scoped to the
server's one configured S3 profile. With `--json`, bulk dry-run emits a
`BulkUpgradePlan`; apply always emits one `BulkUpgradeResult`, including when
the plan is empty or contains only current documents. Failed apply items retain
that parseable result on stdout and make the command non-zero.

### Linked Markdown revisions

`airplan update <url|key> [file|-]` accepts only a complete, marker-managed,
source-backed Markdown document. It resolves any supplied chain member to the
latest live revision, compares exact UTF-8 source bytes, and uploads a complete
ordinary document under a new random directory. A named input must resolve to
the existing document slug; stdin or an omitted name preserves it implicitly.
Local input size, emptiness, UTF-8, and binary validation completes before an
eligible predecessor is upgraded or repaired. Update may then apply the same
in-place upgrade machinery to an eligible older marker or renderer generation
before comparing source bytes; this prerequisite maintenance is part of the
requested update and remains independently recoverable.
Identical content is a
successful no-op returning the existing latest URL. An already-consistent
chain performs no storage or manifest writes; an identical-source retry may
first repair interrupted predecessor promotion or metadata replication.
Revision numbers are positive integers beginning at 1 and never reused.

A standalone upload has no `.airplan-versions.json`. Its first real update
conditionally creates that object in the predecessor and candidate, promotes
the predecessor marker to revision 1, and creates revision 2. Later updates
append linearly using the latest metadata ETag as the serialization point; a
stale concurrent writer receives a conflict and its unannounced candidate is
deleted. REST reports this as `revision_conflict` with status 409. After that
point, retries repair interrupted promotion or replication rather than
allocating another revision.

Cleanup of a marker-first candidate that has not been announced must serialize
against the same predecessor metadata ETag as append. For an existing chain,
cleanup conditionally rewrites semantically unchanged metadata by using an
alternate top-level JSON field order before deleting the candidate. The
same-size byte-distinct body changes content-derived S3 ETags; either that claim
wins or the append publishes first. A candidate whose assigned integer is
already at or below the chain high-water mark is durably known to have lost and
needs no additional claim.
A serialization request failure is ambiguous until a fresh bounded read of the
serialization object proves whether the intended revision URL was published.
An observed intended or monotonic successor body in either the serialization
object or the candidate's own replica continues post-commit repair. For an
existing chain, an observed older body permits rollback only after the updater
wins the same conditional cleanup claim used by third-party cleanup. A failed
read or cleanup claim leaves the discoverable candidate intact rather than
risking deletion of an announced revision. For a first link whose versions
object remains absent, the updater retries the idempotent conditional creation;
winning that retry completes the append rather than reporting a failure.
Semantically identical metadata with the cleanup claim's alternate field order
remains valid for no-op repair and verification.
A revision-2 candidate beside a still-live standalone predecessor fails closed
because no versions object exists on which cleanup can safely contend. A
standalone deletion reservation or missing predecessor is durable proof that
the first-link candidate lost. Candidate rollback uses a fresh bounded cleanup
context so cancellation of the update request does not by itself strand the
marker-first upload.

A first update and a concurrent standalone delete contend on a conditional
reservation at the predecessor's versions key. The delete reservation is not
valid versions metadata and remains as a tiny permanent deletion tombstone
after payload and marker removal. It keeps stale, preflighted updates from
subsequently winning `If-None-Match`; an update that observes it fails closed.
Marker-based listing and sync ignore the otherwise empty tombstone-only prefix.
These tombstones intentionally accumulate and Airplan lifecycle commands never
remove them. Thus exactly one transition can win, including when both
operations first observed the standalone marker.

Every linked v4 or v5 Markdown marker contains an immutable `revision` descriptor
with a 26-character lowercase RFC 4648 base32 `chain_id`, positive `number`,
and, after revision 1, the previous page URL. Revisions greater than 1 declare
exactly one `diff` object named `.airplan-changes.diff`, content type
`text/plain; charset=utf-8`; standalone documents and revision 1 declare none.
The adjacent unified diff has stable `revision-N/plan.md` headers, three context
lines, deterministic LF output, and preserves final-newline distinctions. It is
bounded to 32 MiB and generated before remote mutation. At most 512 KiB is
embedded for server-highlighted display; larger diffs retain the Changes view
and its raw sibling-object link without embedding the diff body in the page.
The immutable `--- revision-N/plan.md` header remains the authoritative
predecessor number when that predecessor has since become a metadata tombstone
and its URL is no longer present in the chain index; re-rendering must not infer
`current revision - 1` across an assignment gap.

`.airplan-versions.json` is a mutable no-store control object with schema
`airplan-versions`, version 1, chain ID, containing member's
`current_revision`, greatest live `latest_revision`, never-decreasing
`last_assigned_revision`, and a complete ordered `revisions` array containing
every assigned integer from 1 through that high-water mark. Live
entries carry number, canonical same-service page URL, UTC creation time, and,
after revision 1, a same-directory diff URL. Deleted entries are tombstones
carrying number, `deleted: true`, and UTC `deleted_at`. Every live member gets
the complete index with only `current_revision` differing. Bodies are limited
to 64 KiB. An append whose complete replicated index would exceed that bound is
refused before candidate upload with `ErrRevisionHistoryFull`; REST reports
`revision_history_full` (422). A missing object means standalone; invalid
metadata disables only revision navigation, not an otherwise complete payload.

The built-in page fetches metadata relative to itself with `no-store` and a
per-load nonce. Valid metadata turns the muted revision indicator above the
rendered content into the sole revision selector, adding a small downward
chevron while retaining its existing typography. Older pages use a
content-width stale-warning treatment labeled `Revision N of M`; the latest
page is labeled `Revision N (Latest)`. The whole indicator opens a native
select, including a one-option selector when deletion leaves one live chain
member; the toolbar contains no revision control and there are no previous,
next, or latest shortcut links.
Valid metadata also adds the server-highlighted Changes view and raw diff
link. It validates same-origin Airplan-shaped URLs and inserts metadata text
through DOM text APIs. Failure leaves the document, source, theme, ToC, print,
and Mermaid behavior intact. Anyone who can read one chain URL learns every
linked capability URL; this is intentional.

`get --diff` fetches the declared adjacent diff from a directory target and is
mutually exclusive with `--source`. `show` exposes revision identity, latest
revision/URL, diff state, validated versions metadata, and a separate advisory
metadata error. Manifest projections may carry `revision_chain_id`, `revision`,
and `latest_revision`; sync reconstructs them from v4/v5 markers and bounded
metadata reads.

Successful `update --json` output extends the six-field document-upload JSON
schema with `revision`, `latest_revision`, `previous_url`, `diff_url`, and
`unchanged`. It does not expose the library result's management or provenance
fields. A newly appended revision includes both URLs and `unchanged: false`;
an identical-source retry returns the current latest upload with
`unchanged: true`. A standalone identical-source result omits `revision`,
`latest_revision`, `previous_url`, and `diff_url` because no revision chain
exists yet.

Protection remains per directory. Purge skips linked revisions unless
`--include-versioned` explicitly acknowledges history removal; protection
still wins. Targeted delete conditionally tombstones the revision in every
surviving member before deleting payloads. The greatest remaining live revision
becomes latest while the assignment high-water mark never decreases.
An interrupted delete whose target still carries its invalid-current reservation
re-derives canonical state from a surviving member whether that replica still
lists the target live or already tombstoned, then completes propagation. An
announced member whose own replica is missing repairs it from its predecessor;
the predecessor's permanent linked-delete receipt remains authoritative after
its marker is gone. Revision 2 also completes any interrupted revision-1
marker/page promotion before deletion. Candidate classification applies only
when the target's own versions object is absent; a not-found while resolving any
other chain member fails closed without deleting target payloads. Marker-only
candidates that were never announced are removed without writing a revision
tombstone, so their unused integer can still be assigned by a later append.
Deleting the final live member conditionally replaces its
versions object with a strict invalid transition reservation before removing
the directory; this both excludes new updaters and makes already-preflighted
appends lose their stale ETag. With no survivor, the manifest delete tombstone
retains chain and revision identity and omits `latest_revision`.
The transition body uses schema `airplan-final-revision-delete-reservation`,
version 1, and exact `chain_id`, `revision`, `last_assigned_revision`, and UTC
second `deleted_at` fields. It is not valid versions metadata. Linked deletion
preserves either this final reservation or the deleted member's invalid-current
versions body through marker removal as a permanent receipt, just as standalone
deletion preserves its reservation. A marker-last retry can therefore prove the
revision identity after every payload is gone, even when local history missed
the first-link projection and still describes revision 1 as standalone.
Marker-based listing and sync ignore the resulting tombstone-only prefix.

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
airplan list|ls [--remote] [--json] [--columns SET] [--wide] [--reverse]
                [--newer-than X] [--older-than X] [--limit N]
                [--kind document|collection] [--slug PATTERN]
                [--protected|--no-protected]
                [-p NAME|--profile NAME|--profile=] [-A|--all-profiles]
airplan show [--json] <url|key>
airplan get [--output PATH] [--source|--diff] <url|key>
airplan delete [--force] <url|key>
airplan protect [--reason TEXT] <url|key>
airplan unprotect <url|key>
airplan upgrade [--check] [--force] [--template PATH] [--json] <url|key>
airplan upgrade --all [--dry-run] [--yes] [--concurrency N]
                [--all-profiles] [--json]
airplan update [--title TITLE] [--max-size SIZE] [--json] [--open] <url|key> [file|-]
airplan purge [--remote] [--older-than 30d|2026-01-01] [--include-versioned]
              [--all] [--dry-run] [--yes] [--concurrency N]
airplan sync [--config PATH] [--profile NAME] [--concurrency N]
             [--no-prune] [--dry-run] [--json]
airplan serve [--listen ADDR] [--allow-non-loopback] [--token-file PATH]
              [--allowed-origin ORIGIN] [--temp-dir PATH]
              [--log-level LEVEL]
airplan mcp
```

`config schema` prints the config file's JSON Schema (see §7).
`skill` prints the complete canonical airplan agent skill to stdout,
byte-for-byte, including its YAML frontmatter and trailing newline. It accepts
no arguments or command-specific flags and emits nothing to stderr on success.
It does not load configuration, inspect credentials, access storage or the
network, or write state, so it works with only the installed binary and from
any working directory. The skill identifies optional Markdown rendering
features and tells agents to use them only when they improve clarity. The same
content is available through the public core library API.
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

Local `list --wide` and explicit `--columns airplan,renderer` expose optional
manifest producer and renderer provenance as `AIRPLAN` and `RENDERER`. Older
records render a dash. These columns are unavailable to LIST-only `--remote`
output because it does not fetch marker bodies.

`list`/`purge` operate on the operation service's manifest by default, or
on its live bucket listing with `--remote`. With an `airplan` backend those
operations execute on the server. `show` inspects one remote
marker directory. For v4 and v5 markers its human and JSON output includes the
producer release and renderer generation when present. `get` fetches only objects declared by a valid remote
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
light_theme     = "github-light"
dark_theme      = "github-dark"
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
light_theme     = "solarized-light"
dark_theme      = "tokyo-night"

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
AIRPLAN_LIGHT_THEME
AIRPLAN_DARK_THEME
AIRPLAN_NO_EXTERNAL_ASSETS
AIRPLAN_MERMAID_URL
AIRPLAN_REPO
AIRPLAN_TIMEOUT
AIRPLAN_CONFIG
AIRPLAN_MANIFEST
AIRPLAN_SERVER_HOST
AIRPLAN_SERVER_PORT
AIRPLAN_SERVER_ALLOW_NON_LOOPBACK
AIRPLAN_SERVER_TOKEN
AIRPLAN_SERVER_TOKEN_FILE
AIRPLAN_SERVER_ALLOWED_ORIGINS
AIRPLAN_SERVER_TEMP_DIR
AIRPLAN_SERVER_LOG_LEVEL
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

### Theme catalog

`light_theme` and `dark_theme` select uploader defaults for the two reader
mode slots. They are profile-aware and may also be overridden by
`AIRPLAN_LIGHT_THEME` and `AIRPLAN_DARK_THEME`; there are no upload theme
flags. Either slot may name a theme of either variant. Defaults are
`github-light` and `github-dark`.

The reserved built-in catalog is `github-light`, `catppuccin-latte`,
`rose-pine-dawn`, `solarized-light`, `tokyo-night-day`, `github-dark`,
`catppuccin-mocha`, `rose-pine`, `solarized-dark`, `tokyo-night`, and
`one-dark`. The first five declare variant `light`; the last six declare
variant `dark`.

Custom themes are global entries available to every profile, not maps merged
inside profiles:

```toml
[themes.dracula-docs]
name = "Dracula Docs"
variant = "dark"
background = "#282a36"
foreground = "#f8f8f2"
muted = "#a8a8b2"
accent = "#bd93f9"
accent_foreground = "#282a36"
border = "#44475a"
surface = "#30323e"
surface_emphasis = "#44475a"
info = "#8be9fd"
success = "#50fa7b"
important = "#bd93f9"
warning = "#f1fa8c"
danger = "#ff5555"
syntax = "derived"
```

IDs are lowercase ASCII slugs no longer than 48 bytes and cannot shadow a
built-in. Names are non-empty valid UTF-8 of at most 80 Unicode characters.
`variant` is exactly `light` or `dark`. Every semantic color is required and
is normalized to lowercase `#rrggbb` or `#rrggbbaa`. `syntax` is omitted,
empty, `derived`, or exactly `chroma:<registered-style-name>`; omitted and
empty both derive highlighting from the semantic tokens. Chroma names are
validated against the registered list without fallback. Reserved/malformed
IDs, incomplete token sets, unknown keys, invalid colors or variants,
malformed syntax selectors, and selected IDs absent from the resolved catalog
are configuration errors before rendering or upload.

Catalog entries are canonicalized deterministically, custom IDs are sorted,
and the complete normalized catalog is SHA-256 hashed for render provenance.
Built-in pages embed semantic CSS and safe browser metadata for that immutable
catalog. Under `@media print`, page tokens, syntax, and prepared Mermaid output
are always GitHub Light regardless of mode, slot assignments, or custom themes.

If the config file contains credentials and is group- or
world-readable, a warning is printed to stderr.

Behavioral defaults: `template`, `collection_template`, `light_theme`,
`dark_theme`, `no_source`, `indexable`, `no_external_assets`, `mermaid_url`,
`repo`, and `timeout` may be set at the root or profile level. Applicable flags
override the config values; theme selection intentionally has no CLI flags.

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
 "created_at":"2026-07-21T12:00:00Z",
 "key":"vq3n.../plan.html","source_key":"vq3n.../plan.md",
 "marker_key":"vq3n.../.airplan.json",
 "url":"https://plans.example.com/vq3n.../plan.html",
 "bucket":"plans","profile":"work","kind":"document",
 "slug":"plan","format":"md",
 "title":"Refactor auth","repo":"https://github.com/acme/service",
 "bytes":18432,"objects":3,"total_bytes":19004,"marker_version":5,
 "producer_version":"0.8.0","renderer_version":3}
{"type":"upload","time":"2026-07-21T12:03:00Z",
 "created_at":"2026-07-21T12:03:00Z",
 "key":"gaj4.../index.html",
 "marker_key":"gaj4.../.airplan-collection.json",
 "url":"https://plans.example.com/gaj4.../index.html",
 "bucket":"plans","profile":"work","kind":"collection",
 "title":"login.png and 1 more","bytes":9216,"objects":4,
 "total_bytes":203512,"marker_version":5,
 "producer_version":"0.8.0","renderer_version":3}
{"type":"delete","time":"2026-07-09T09:12:44Z",
 "key":"vq3n.../plan.html","marker_key":"vq3n.../.airplan.json",
 "bucket":"plans","profile":"work","reason":"deleted"}
{"type":"protect","time":"2026-07-25T12:00:00Z",
 "key":"vq3n.../plan.html","marker_key":"vq3n.../.airplan.json",
 "bucket":"plans","profile":"work","protect_reason":"README demo link"}
{"type":"unprotect","time":"2026-07-26T09:00:00Z",
 "key":"vq3n.../plan.html","marker_key":"vq3n.../.airplan.json",
 "bucket":"plans","profile":"work"}
{"type":"upgrade","time":"2026-08-15T10:00:00Z",
 "created_at":"2026-07-21T12:00:00Z",
 "key":"vq3n.../plan.html","source_key":"vq3n.../plan.md",
 "marker_key":"vq3n.../.airplan.json",
 "url":"https://plans.example.com/vq3n.../plan.html",
 "bucket":"plans","profile":"work","kind":"document",
 "slug":"plan","format":"md","title":"Refactor auth",
 "bytes":19200,"marker_version":5,"producer_version":"0.8.0",
 "renderer_version":3}
{"type":"link","time":"2026-08-15T10:05:00Z",
 "created_at":"2026-07-21T12:00:00Z",
 "key":"vq3n.../plan.html","source_key":"vq3n.../plan.md",
 "marker_key":"vq3n.../.airplan.json",
 "url":"https://plans.example.com/vq3n.../plan.html",
 "bucket":"plans","profile":"work","kind":"document",
 "slug":"plan","format":"md","title":"Refactor auth",
 "bytes":19200,"objects":4,"total_bytes":20120,"marker_version":5,
 "producer_version":"0.8.0","renderer_version":3,
 "revision_chain_id":"d2x4...","revision":1,"latest_revision":2}
```

(Shown wrapped for readability; on disk each record is one line.)

- `time` is RFC 3339, UTC.
- `upload` records: `kind` is `document` or `collection`. `slug` and `format`
  are present for documents and omitted for collections. `source_key` is
  document-only and omitted for HTML or under `--no-source`. `key` and `url`
  identify the primary page; `bytes` describes that page, not collection
  payload bytes. `objects` counts the ownership marker plus every object the
  marker declares, and `total_bytes` sums their declared sizes; both are
  optional and absent together on history written before airplan recorded
  them, and on uploads whose marker cannot declare every counted size. Marker
  v3, v4, and v5 always qualify; marker v2 qualifies only when it declares no source;
  marker v1 and v2-with-source do not. When present, both fields are positive;
  a record with only one field or an explicit zero is invalid. They are
  additive: `bytes` keeps its own meaning and is never repurposed. `title` is
  omitted when empty; `profile` is omitted for
  root-level settings. `marker_key` is the exact kind-specific ownership key.
  `repo` preserves canonical repository metadata. The full collection
  inventory remains only in the remote marker.
- Current writers always include `marker_version: 5`; its absence identifies
  legacy pre-marker history. Readers infer `kind: document` and derive its
  slug from the page key for valid older records that omit those fields. The
  renderer-3 theme recipe remains in the remote v5 marker and is not duplicated
  in this local projection.
- New `delete` tombstones include `marker_key`, `bucket`, the receiving
  `profile`, and reason `deleted` or `remote_missing`. Their identity is
  `(bucket, marker_key)`. A linked-revision deletion also includes its
  `revision_chain_id`, deleted `revision`, and surviving `latest_revision`;
  these fields let reduction reject stale upload/link events whose local
  manifest append completed after the remote deletion. A marker-missing retry
  after an earlier manifest-write failure retains the chain and deleted
  revision but omits `latest_revision`, because that recovery path cannot
  reconstruct the surviving latest value safely. Legacy key-only tombstones
  remain valid.
- `protect` and `unprotect` records project remote purge-protection state
  (§9 protect/unprotect and purge) onto the same `(bucket, marker_key)`
  identity. Both require `key`, `marker_key`, and `bucket` — protection
  applies only to marker-managed identities, so pre-marker legacy history can
  never appear protected. `protect_reason` is an optional advisory note of at
  most 256 Unicode characters with no control characters, present on
  `protect` records only; it is a
  distinct field because `reason` is already a delete-tombstone enum.
- `upgrade` events carry a complete refreshed upload projection, an event
  `time`, required nonzero UTC preserved original `created_at`, and
  producer/renderer versions.
  Reduction treats them like upload refreshes without clearing protection or
  making the document appear newly created.
- `link` events carry the same complete projection as `upgrade`, plus
  `revision_chain_id`, `revision`, and `latest_revision`. They refresh every
  surviving member after append or tombstone propagation without changing its
  original `created_at`, object totals, or other orthogonal fields.
- Manifest history reduces chronologically. The latest event for an upload
  identity wins, duplicate uploads collapse to their latest record, and a
  later upload reactivates an earlier tombstone. A legacy key-only tombstone
  hides matching preceding uploads but not a later upload event. Linked
  revision numbers are immutable and never reused: a revision-aware delete
  tombstone permanently suppresses that revision even if a stale upload,
  upgrade, or link event is appended later. Chain-wide `latest_revision` is
  the highest advertised revision that does not have a revision-aware delete
  tombstone, so partial local history preserves known remote members while
  completed deletions cannot be resurrected by out-of-order manifest writes.
  Only records whose advertised latest is at least their own revision
  participate in or receive this normalization; an incomplete marker-only
  candidate retains `latest_revision: 0` and remains unannounced.
- Protection reduces alongside uploads: the latest `protect`/`unprotect`
  event wins, a `delete` clears the identity's protection, and an `upload`
  event does **not** clear it — a sync re-import of a still protected
  directory must not silently drop protection. Protection state with no
  active upload is simply not surfaced. Reduced upload records carry derived
  `protected`, `protected_at`, and `protect_reason` fields — surfaced by
  `list --json` and the REST manifest listing — but writers never persist
  the derived `protected`/`protected_at` fields on any record. The local
  projection exists so `purge --dry-run` is accurate offline; the remote
  sentinel (§5) remains authoritative.
- Forward compatibility: readers ignore unknown fields and skip
  records with an unknown `type`. The record itself needs no schema
  version; `marker_version` describes the remote upload format.
  Readers retain an otherwise-valid upload with no `marker_version`
  as legacy history, but it never authorizes delete or purge. An
  unsupported nonzero `marker_version` is invalid and skipped with a
  warning. Marker versions 1, 2, 3, and 4 are managed; pre-marker entries remain
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

- `airplan list`: past uploads from the manifest, by default date, kind, title,
  object count, human-readable binary size, and URL, plus protection state when
  relevant; `--json` for scripting with exact byte counts. Rows sort by record
  time, then ownership marker key, so
  local history reads in the same order as a remote listing even when `sync`
  appended imported uploads after later local ones. Clients reapply this order
  to manifest responses from older Airplan HTTP servers before filtering or
  limiting them. `--reverse` prints newest
  first, in the table and in `--json`. `KIND` is the record's kind, with
  legacy history reading as `document` under the record-schema inference above;
  it shows `-` only when no kind is known at all, as in a served record that
  declares none. `OBJECTS` and `SIZE` use the same column vocabulary as remote
  listing but report the local upload's marker-declared object count and total
  size; storage-observed remote values can differ as described below. Both
  read `-` for history that predates them, which `sync` fills in. The wide
  `PAGE SIZE` column always reports the primary page alone,
  and `bytes` keeps that meaning in `--json`. In local tables, `STATE` is
  `managed` for the supported `marker_version`, `legacy` when the field is
  absent, `protected` for a protected managed upload, and `legacy+protected`
  when both conditions apply. These states appear in history without warning;
  legacy entries remain ineligible for delete reconciliation and purge.
- Table columns are one vocabulary shared by local and remote listing, and
  always print in the canonical order `date`, `profile`, `state`, `kind`,
  `title`, `slug`, `objects`, `size`, `page-size`, `dir`, `format`, `airplan`,
  `renderer`, `revision`, `latest`, `repo`, `bucket`, `url`. Local listing
  offers every one of them; remote listing offers
  `date`, `state`, `kind`, `slug`, `objects`, `size`, `dir`, and `url`. Remote
  `STATE` is `protected` or `unprotected`; remote listing does not fetch marker
  bodies, so it cannot classify rows as managed or legacy. Three columns are
  automatic: `profile` when the printed rows span more than one profile,
  `state` when any row is legacy or protected or either protection filter was
  used, and `dir` when any row has no URL to identify it by. They appear in the
  default set only when their rule holds, so a column never occupies width
  without carrying information or confirming an explicit protection selection.
  `--columns date,title,url` selects an absolute set and suppresses automatic
  columns; `--columns +dir,-title` adjusts the mode's default set instead. The
  two forms do not mix, requested order does not change the canonical order,
  and repeated names collapse. An unknown name is an error listing the mode's
  valid names; a name valid only in the other mode is an error naming that
  mode rather than a silently blank column; so is a selection that leaves no
  columns at all. `--wide` prints every column the mode offers and combines
  with neither `--columns` nor `--json`. Column selection is presentation, so
  `--columns` and `--wide` are rejected with `--json`, while `--reverse`
  reorders both outputs.
- `list` filters are selection, not presentation, so they apply to both listing
  modes and to the table and `--json` alike. `--newer-than X` keeps uploads
  recorded at or after `X`; `--older-than X` keeps uploads recorded strictly
  before it, the same boundary `purge --older-than` uses, so the two bounds
  partition a timeline without overlap or gap. `--limit N` keeps the **N most
  recent** matches and still prints them oldest first; it applies after the
  other filters, `--limit 0` selects nothing, and a negative value is an error.
  `--kind document|collection` selects one kind, counting legacy history that
  omits `kind` as a document and never matching a remote dual-marker conflict,
  which declares no kind. `--slug PATTERN` is a glob over document slugs with
  the same meaning as `purge --slug`: collections are excluded even from
  `--slug '*'`, an upload with no known slug never matches, and a local record
  that omits `slug` uses the one derived from its page key. `--protected` keeps
  only protected uploads; `--no-protected` keeps only unprotected uploads, and
  the two flags are mutually exclusive. For remote listing the state comes
  from sentinel presence in the same LIST snapshot. Filters compose, and
  `--reverse` reorders whatever they selected. Supplying any string filter
  explicitly with an empty value is an error; omission alone means no filter.
  MCP requests preserve the same distinction, use `protected: true|false` for
  the two protection selections, and treat an explicit limit of zero as
  selecting nothing.
- `--newer-than` and `--older-than` accept exactly two forms. An age, as
  `purge --older-than` has always accepted, including `d` and `w` units:
  `7d`, `2w`, `36h`, `1h30m`. Or an absolute date: `2026`, `2026-07`,
  `2026-07-01`, `2026/07/01`, `2026-07-01 09:30`, `2026-07-01T09:30`,
  `2026-07-01T09:30:00`, or strict RFC 3339 with `Z` or a numeric offset.
  Zoned timestamps accept dot fractional seconds, reject comma fractions, and
  require offset hours and minutes within `00..23` and `00..59`; offset-less
  forms accept no fractional seconds beyond the exact layouts above. A value
  opening with four digits, alone or followed
  by `-` or `/`, is a date; everything else is an age. Dates and times without
  an offset resolve at **local** midnight or local wall-clock time, matching
  `docker` and `journalctl`; an explicit offset is honored as written. The
  manifest continues to record UTC — only the flag value is local. Because the
  same parser selects purge deletions, ambiguous input is refused rather than
  guessed: a slash date that does not lead with a four-digit year, such as
  `03/04/2026`, is an error naming the year-first form.
- Local S3 `list` defaults to the resolved active profile: an explicit
  `--profile` or `AIRPLAN_PROFILE`, `default_profile`, single-profile
  inference, or root-level configuration. It filters local history by that
  recorded profile without requiring storage credentials. `--profile NAME`
  selects that exact profile, while `--profile=` selects root-level history.
  `-A`/`--all-profiles` instead lists every recorded profile and can read local
  history without resolving an ambiguous or missing configuration profile. It
  cannot be combined with `--profile` or with `--remote`, whose scope is the
  selected profile's `key_prefix`. If configuration selects an `airplan`
  profile, `list` calls the server's manifest endpoint, which scopes records to
  the server's own identity; `--all-profiles` is rejected before client
  construction because cross-profile scope exists only for local S3 manifest
  history.
  `--config` is therefore valid
  for non-remote list because it can select the HTTP backend.
- `airplan list --remote`: cheaply discovers marker directories made from any
  machine. It performs only paginated bucket LIST operations beneath the
  active profile's `key_prefix`; it does not GET markers, HEAD pages, or trust
  marker content. It groups every returned object beneath an exact
  `[key_prefix/]<26-char lowercase base32>/` directory, then emits groups
  containing `.airplan.json`, `.airplan-collection.json`, or both. Payload
  filename shape without either marker is never evidence of visibility.
- Remote list rows have `DATE`, `KIND`, `SLUG`, `OBJECTS`, `SIZE`, and `URL`
  columns by default, plus `DIRECTORY` when a row has no inferable URL or under
  `--wide`. `DATE` is the selected marker object's storage
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
  key; `--reverse` prints newest first.
- Local and remote `OBJECTS` and `SIZE` are counted differently on purpose.
  Local values are **marker-declared**: the marker plus exactly the objects it
  lists, at the sizes it declares. Remote values are **storage-observed**:
  every object and byte beneath the random directory, including unrecognized
  extras. The same upload can therefore report different numbers in the two
  listings, and that divergence is diagnostic signal rather than an error —
  `show` inspects one directory and reports declared and actual sizes side by
  side, which is where the two are reconciled.
- `list --remote --json` prints an array with one object per row. Its stable
  fields are `time`, `dir`, `marker_key`, `objects`, `bytes`, and `kind` when
  one marker kind is implied. `conflict` is true for dual-marker directories;
  `protected` is true when the listing contains the directory's
  purge-protection sentinel (§5) — unlike `kind` it is authoritative, because
  sentinel presence is the whole contract;
  `slug`, `key`, and `url` appear only when inferred. These entries describe
  marker-key presence and occupancy, not validated uploads. Both human tables
  expose protection through their automatic `STATE` column when any displayed
  row is protected or a protection filter was used.
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
  `source`, while collections expose an ordered `files` array. Valid states
  also expose purge protection: `protected` is true when the directory
  listing contains the sentinel (§5), with advisory `protected_at` and
  `protect_reason` from a best-effort sentinel body read — an unreadable or
  malformed body leaves the upload protected with the listing timestamp and
  no reason. The human detail block reports the same state as a `PROTECTED`
  row, `no` or `yes (reason: ...)`. Declared object
  entries contain `key`, `url`, `exists`, `expected_bytes`, and `bytes`, with
  `bytes` omitted when missing. An invalid result
  additionally contains `error`, a stable coarse code:
  `oversized`, `malformed_json`, `unsupported_version`, `invalid_fields`, or
  `conflicting_markers`; it never exposes untrusted marker fields. Human
  output presents the same information as a labeled detail block.
- `airplan get <url|key>` fetches one object from a marker-managed upload;
  `--source` selects the declared source and `--diff` selects the declared
  adjacent revision diff. The selectors are mutually exclusive.
  Full URLs, bare keys, random directories, configured prefixes, and
  path-style endpoint URLs obey the same connection, bucket, and prefix
  rules as `delete`. Before returning bytes, `get` concurrently probes both
  exact marker keys, requires one to exist and the other to be confirmed
  absent, and validates the existing marker. This preserves object-read-only
  credentials without requiring LIST permission. A timeout, authorization
  failure, or ambiguous probe fails closed. A random-directory target selects
  the primary page, the document source under `--source`, or the adjacent diff
  under `--diff`; requesting either capability when it is not declared is an
  error. An explicit declared
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
- Before any deletion, `delete` checks the directory listing it already
  performs for the purge-protection sentinel (§5). If the sentinel is
  present, the delete fails with an actionable error naming `unprotect` and
  `--force`; the error includes the advisory reason when the sentinel body
  can be read. This delete-time guard is authoritative and catches
  protection set on another machine that local history has not seen. If
  protection cannot be determined — the listing fails with anything other
  than success — the delete fails without touching objects; only `--force`
  deletes a protected upload.
- A valid marker authorizes deletion of every object under its own
  random directory, including incomplete-upload remnants and
  unrecognized extra siblings. Deletion removes every non-marker,
  non-sentinel object first. A forced delete then removes the protection
  sentinel in its own request, so protection outlives every payload. Only after
  those deletions succeed is the marker deleted in a separate final operation.
  An interruption after sentinel removal can therefore leave only the
  ownership marker as an unprotected remnant; all payloads are already gone.
  Any payload or marker failure leaves the local upload untombstoned so retry
  can resume while the marker still establishes ownership. A successful marker
  deletion is followed by the append-only local tombstone.
- `airplan protect [--reason TEXT] <url|key>` marks one marker-managed
  upload as purge-protected by writing its sentinel (§5);
  `airplan unprotect <url|key>` removes it. Both accept the same targets as
  `delete`, resolve and validate the ownership marker first through the same
  fail-closed dual probe — so they need no LIST permission and cannot litter
  sentinels across unmanaged prefixes — and are idempotent: protecting an
  already protected upload succeeds and rewrites the sentinel, as does
  unprotecting an unprotected one. A missing, conflicting, or invalid marker
  fails without writing; pre-marker legacy uploads therefore cannot be
  protected. `--reason` is bounded to 256 Unicode characters, must not
  contain control characters, and is rejected otherwise. Each success appends the matching `protect`/`unprotect`
  manifest record best-effort and prints a one-line stderr summary
  (`protected upload (key ...)` / `unprotected upload (key ...)`); stdout
  stays empty.
- Before `show`, `get`, `delete`, `protect`, `unprotect`, `upgrade`, or `update` resolves its
  connection, it consults local
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
  `--older-than 30d`, `--slug PATTERN`, `--profile P`. `--older-than` takes
  the same values as `list --older-than`, an age with `d`/`w` units or an
  absolute date, read by the same parser with the same refusal to guess at
  ambiguous input. The flag must have a non-empty value and its resolved
  boundary must be strictly in the past; zero ages, the present, and future
  absolute dates are rejected even alongside `--all`. `--all` is the explicit
  way to ask to delete everything.
  `--profile`/`-p` behaves as on every other
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
  Manifest-sourced candidates are previewed and deleted in manifest listing
  order — record time, then ownership marker key.
  Every selected deletion still requires the marker, except for the
  local-only ensure-gone reconciliation above. Suitable for cron
  (`purge --older-than 30d --yes`).
- Purge never deletes a purge-protected upload and offers no force
  override — bulk-overridable protection is not protection; only the
  targeted `delete --force` can remove one. Matching protected uploads are
  excluded from the candidate list up front (from the manifest projection
  locally, from the LIST snapshot with `--remote`) and each prints a stderr
  note, `airplan: note: skipping protected upload <url>`; `--dry-run`
  prints the same notes and excludes them from the preview. Protection set
  after planning is still caught by the delete-time sentinel guard and is
  reported the same way. Protected skips are never failures: the summary
  becomes `purged N uploads (P protected, F failed)` and skips do not
  change the exit status. When linked revisions are skipped because
  `--include-versioned` was not supplied, purge first prints
  `airplan: note: skipped N linked revision(s); use --include-versioned to include them`,
  then one `airplan: note: skipping linked revision <url> (chain <id>, revision X of Y)`
  line per known complete projection. If the marker identifies revision X but
  validated metadata did not establish the latest revision, that suffix is
  `(chain <id>, revision X)` instead. A revision discovered only by the
  delete-time recheck prints
  `airplan: note: skipped linked revision <upload-id>; use --include-versioned to include it`.
  The summary becomes
  `purged N uploads (P protected, V versioned, F failed)`.
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
  Imported managed-marker records retain kind, exact marker identity, primary page,
  document slug/format/source where applicable, title, and repository, but do
  not duplicate collection inventories. Imported profile,
  bucket, and public URL values come from the receiving machine's resolved
  connection, never the marker.
  Sync also completes local history in place: for each active, scoped,
  marker-managed record that is missing both `objects` and `total_bytes` and
  carries a v3, v4, or v5 marker version, or a v2 marker version without a source, it
  fetches and inspects that marker and, only
  for a `complete` inspection, appends an enriched upload record carrying the
  record's original time and identity plus the declared totals. Append-only
  history holds and latest-wins reduction collapses the pair. An incomplete or
  invalid marker leaves both fields absent rather than guessing, and leaves the
  record untouched. Enrichment never resurrects a tombstoned identity: the
  record must still be active when sync locks, rereads, and reduces before
  writing. Its time and primary key must still match the inspected snapshot;
  metadata-only concurrent edits are preserved, while a replacement under the
  same marker key is left for a later sync. Ineligible v1 and v2-with-source
  records never schedule recurring marker fetches. It converges in one pass,
  shares the same `--concurrency` budget,
  writes nothing under `--dry-run`, and is reported separately from imports.
  Enrichment completes metadata for an upload already in local history, so it
  never fails the run: a marker that cannot be fetched, or that is incomplete,
  invalid, or without declared sizes, is counted as deferred and named in a
  warning, and a later sync retries it. Otherwise one unreadable marker would
  fail every later run, because the record keeps qualifying.
  A complete source-backed v4 or v5 Markdown record without a revision projection is
  also inspected because another writer may have promoted its marker from
  standalone to revision 1. A discovered complete versions index appends a
  `link` projection with chain, revision, and latest values. A still-standalone
  record remains unchanged and eligible for the same lightweight check on a
  later sync; once promoted, the link converges it permanently. New imports
  likewise record revision identity only when a complete versions index proves
  the marker's revision was announced; a marker-only rollback candidate remains
  unversioned locally so its unused integer can be reassigned. The manifest lock
  recheck prevents a concurrent delete or link from being overwritten.
  Revision tombstones are scoped by profile, bucket, and chain identifier. A
  remote marker for an already-tombstoned revision is retained as deleted and
  is not re-imported or appended again, including when its versions replica is
  missing or invalid and under `--dry-run`; the immutable marker descriptor is
  sufficient to match an existing scoped tombstone. Later syncs therefore
  converge even if stale storage restores the marker. By default,
  active scoped local records absent from LIST are considered for
  pruning, but airplan performs a targeted marker GET before appending a
  `remote_missing` tombstone. Only a definite not-found response confirms
  absence. Revision identity is retained only from a complete announced local
  projection; an incomplete candidate projection cannot permanently suppress a
  later legitimate reuse of the same integer. A returned marker is retained
  regardless of its contents; timeout,
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
  `enriched_records`, `tombstone_records`, `protection_records`, and `failures`
  arrays plus `unchanged`, `deferred`, `incomplete`, `invalid`, and `retained`
  counters. `retained` includes already-tombstoned remote revisions that sync
  deliberately leaves suppressed.
  Enriched records complete uploads already in history, so they are never
  counted as additions. `unchanged` counts scoped records already complete
  locally, including an inspected v4 or v5 Markdown record that remains standalone;
  a totals-enrichment candidate is reported as enriched or deferred and never
  also as unchanged. A partial failure exits nonzero after
  writing the result. Sync provides eventual active-inventory convergence;
  it neither uploads deletion history nor makes historical JSONL files
  identical across machines.
- Sync also reconciles purge protection from the same LIST snapshot, at no
  extra request cost, for every scoped active record present in the
  snapshot — not just newly imported ones. Remote protected with local
  unprotected appends a `protect` record; remote unprotected with local
  protected appends an `unprotect` record. Newly imported protected uploads
  carry the sentinel's advisory time and reason when its body is readable;
  already-active records reconcile on presence alone with the sync time and
  no reason. Markers absent from the snapshot are left to prune's
  confirm-absence rules. The locked reread before appending also rechecks
  protection: records are appended only for identities still active whose
  local projection still disagrees. The human summary reports
  `updated protection on N` alongside synced and tombstoned counts.
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
- Purge protection is enforced by conforming clients, not by storage.
  airplan builds older than this contract ignore it entirely: they do not
  know to look for the sentinel and will purge or delete straight through
  it. This is a documented limitation — any stronger guarantee requires
  server-side enforcement, which the S3 credential model deliberately does
  not assume. Native storage tooling is likewise unaffected by protection.
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

HTTP problem codes preserve stable public error identity across backends:
`input_too_large` and the multipart-envelope `request_too_large` map to
`ErrInputTooLarge`, `revision_history_full` maps to `ErrRevisionHistoryFull`,
and conditional mutation conflict codes map to `ErrConflict`, while the server
problem details remain available.

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
profiles. Direct local S3 `list` uses its resolved profile by default and
requires `--all-profiles` for an all-history view. `serve` requires an `s3`
profile and rejects an `airplan` profile, preventing proxy chains and loops.

### Server process

`airplan serve` runs one single-user HTTP server. An explicitly supplied flag
wins over its server-specific environment fallback. Its options are:

- `--listen`, which wins over both `AIRPLAN_SERVER_HOST` and
  `AIRPLAN_SERVER_PORT`. Without it, host and port resolve independently and
  default to `127.0.0.1` and `8080`.
- `--allow-non-loopback`, with
  `AIRPLAN_SERVER_ALLOW_NON_LOOPBACK` as its fallback and `false` as the
  default. An explicit `--allow-non-loopback=false` wins over the environment.
- `--token-file`, with `AIRPLAN_SERVER_TOKEN_FILE` as its fallback and
  `AIRPLAN_SERVER_TOKEN` as the alternative token-value source.
- repeatable `--allowed-origin` values, with comma-separated
  `AIRPLAN_SERVER_ALLOWED_ORIGINS` as the fallback.
- `--temp-dir`, with `AIRPLAN_SERVER_TEMP_DIR` as the fallback.
- `--log-level`, with `AIRPLAN_SERVER_LOG_LEVEL` as its fallback. Accepted
  values are `error`, `warn`, `info`, `debug`, and `trace`; the default is
  `info`.

Environment-derived host and port are combined without losing IPv6 address
syntax. `AIRPLAN_SERVER_HOST` must be non-empty when set.
`AIRPLAN_SERVER_PORT` accepts decimal digits only and must be from 0 through
65535; zero requests an ephemeral listener. Server boolean environment values
accept exactly `true` or `false`. The allowed-origins environment value is
split on commas, surrounding whitespace is trimmed, and empty entries are
rejected. Explicit origin flags replace rather than append to the environment
list. Invalid server environment values fail before storage initialization or
listener creation.

Exactly one non-empty server-token source is required. A token should contain
at least 32 random bytes; token files should be mode 0600. An explicit
`--token-file` replaces `AIRPLAN_SERVER_TOKEN_FILE`; the resolved file source
and `AIRPLAN_SERVER_TOKEN` conflict rather than silently choosing one. The
token is read once at startup. The server defaults to loopback. Binding to a
non-loopback address requires explicit acknowledgement, and TLS must terminate
at a trusted reverse proxy. The built-in server does not manage certificates.

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

### Official container image

The official server image is `ghcr.io/jimeh/airplan`. Each release is one OCI
image index with runnable `linux/amd64` and `linux/arm64` images. It publishes
only the release version without GitHub's leading `v` (for example `0.5.1`)
and `latest`. It does not publish `v`-prefixed, commit-SHA, major, or
major/minor tags. An exact version never intentionally changes to another
digest. `latest` identifies the repository's latest GitHub release and is
mutable; immutable deployments use the image-index digest. Published indexes
carry an SBOM and GitHub build-provenance attestation from this repository's
release workflow. Publication serializes the workflow's own GHCR mutations and
rejects an observed exact-tag conflict. The registry does not enforce tag
immutability against external writers; the image-index digest is the immutable
deployment boundary.

The image runs as numeric UID/GID `65532:65532`. Its entrypoint is `airplan`
and its exact default command is `serve`, with no shell interpolation or
baked listen address. It supplies these image-level environment defaults:

```text
XDG_CONFIG_HOME=/etc
AIRPLAN_MANIFEST=/var/lib/airplan/manifest.jsonl
AIRPLAN_SERVER_HOST=0.0.0.0
AIRPLAN_SERVER_PORT=8080
AIRPLAN_SERVER_ALLOW_NON_LOOPBACK=true
```

Consequently, `/etc/airplan/config.toml` is the optional default config file
inside the image. It is not forced through `AIRPLAN_CONFIG`: an environment-
only storage configuration works when no file is mounted. A file mounted at
the default path is discovered automatically, and `AIRPLAN_PROFILE` may select
one of its profiles. An explicitly set `AIRPLAN_CONFIG` retains the normal
requirement that its path exist.

`/var/lib/airplan` is the declared persistent volume and is writable by the
runtime user. One active server owns each mounted manifest volume. Operators
must retain and back up that volume when upload history matters; an anonymous
volume does not by itself provide a durable lifecycle association. A bind-
mounted state directory and mode-0600 config or token files must be readable
or writable as appropriate by UID/GID `65532:65532`. Configuration, bearer
tokens, and storage credentials remain external to the image.

The image exposes port 8080 as metadata but does not publish it.
`AIRPLAN_SERVER_PORT` changes the listener only; port mapping, reverse-proxy
target, and external `/healthz` probe must use the same port. The image has no
built-in healthcheck or TLS termination. It uses the same unauthenticated
`/healthz`, external TLS, static-token, trusted-content, and single-instance
boundaries as native `airplan serve`.

### REST wire contract

`api/openapi.yaml` is the authoritative OpenAPI 3.0.3 contract. The exact
checked-in schema is embedded and returned by `GET /openapi.yaml`. Compatible
changes remain under `/api/v1`; breaking changes require a new URL version.
`GET /healthz` and `GET /openapi.yaml` are unauthenticated. The following
endpoints require `Authorization: Bearer <token>`:

```text
GET    /api/v1/capabilities
POST   /api/v1/uploads/documents
POST   /api/v1/uploads/documents/update
POST   /api/v1/uploads/collections
POST   /api/v1/uploads/inspect
POST   /api/v1/uploads/get
POST   /api/v1/uploads/delete
POST   /api/v1/uploads/protect
POST   /api/v1/uploads/unprotect
POST   /api/v1/upgrades/plan
POST   /api/v1/upgrades/execute
POST   /api/v1/upgrades/preview
POST   /api/v1/upgrades
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

Inspect, get, delete, protect, and unprotect take `url_or_key` in a JSON
request body. The server
resolves it against its complete S3 configuration and permits only objects
declared by exactly one valid Airplan ownership marker. Get streams its
response with the stored object's content type. Capability URLs are not placed
in query strings. Upload, list,
inspection, and purge-preview results expose the randomized directory as an
opaque `upload_id`. `GET /api/v1/uploads` returns service-scoped manifest
records in the same order as local `list` (§9): record time, then ownership
marker key. Clients reject a manifest response whose `objects` and
`total_bytes` fields are not absent together or positive together rather than
rendering a non-conforming inventory pair. The MCP `list_uploads` tool uses the
same order.

Delete accepts an optional `force` boolean; without it, deleting a
purge-protected upload fails with problem code `upload_protected` (409),
whose problem object carries the advisory `protect_reason` when the sentinel
body could be read — so both backends surface the same delete error text.
Protect accepts an optional advisory `reason` of at most 256 characters and
returns the resulting protection state; an invalid reason fails with problem
code `invalid_protect_reason` (422). Unprotect returns the cleared state.
Inspection, manifest, and storage listings expose `protected` (with advisory
`protected_at` and `protect_reason` where known), purge previews report
protected exclusions in a separate `protected` array, purge items report a
delete-time skip as `protected: true` rather than a failure, and sync results
include `protection_records`. The capabilities operation list includes
`protect_upload` and `unprotect_upload` so clients can detect the feature.

Document update uses bounded streaming `multipart/form-data` with metadata
containing the target and optional name, title, and size limit. Refusal maps a
missing upload to `upload_not_found` (404), an unreconcilable chain to
`invalid_upload` (422), an ineligible document to `invalid_target` (422), and a
lost append serialization race to `revision_conflict` (409). A chain whose
bounded replicated index cannot fit another assigned revision maps to
`revision_history_full` (422) before candidate upload.

Purge is two-phase. `/purge/preview` applies the source and filters without
deleting and returns explicit `upload_id` candidates. The CLI displays them
and performs confirmation. `/purge` accepts only an explicit array of those
IDs, re-resolves and revalidates every current marker, attempts targets
sequentially, and reports every success or failure. It accepts no URL, key,
filter, or implicit-all execution request.

Upgrade is likewise two-phase. `/upgrades/plan` classifies one target and
returns its exact marker/page identity and ETags; `/upgrades/execute` accepts
that plan and revalidates it before mutation. `/upgrades/preview` classifies
the server-scoped active manifest; `/upgrades` accepts only the exact plan
items returned by preview. Execute-time refusal maps `missing` to
`upload_not_found` (404), `invalid` to `invalid_upload` (422), and `ineligible`
to `invalid_target` (422). Capabilities advertise all four operation names.

REST errors use RFC 9457 `application/problem+json` with stable `code` and
`request_id` fields. Problem detail is selected from stable generic text by
code; request-derived validator and parser details remain internal.
Authentication is checked before request bodies are parsed. Missing, malformed,
and incorrect bearer credentials receive the same generic 401 and
`WWW-Authenticate: Bearer`. Tokens, capability URLs, request bodies, S3
response bodies, filesystem paths, and credentials must not appear in logs,
error details, warnings, or per-item failure text. Hosted structured results
use stable generic messages where internal detail would otherwise be exposed.
Upload POSTs are not retried automatically because a timeout after server
commit is ambiguous without persistent idempotency state.

### MCP servers

`airplan mcp` is a stdio MCP server. Its upload listing tool accepts the same
selection arguments as `list` — `newer_than`, `older_than`, `limit`, `kind`,
`slug`, and boolean `protected`, with the same meanings and the same time
parser (§9). It
constructs the normal public client,
so it works with either backend. MCP frames are its only stdout content;
warnings and logs use stderr. `airplan serve` exposes the same tool
implementation at `/mcp` using MCP Streamable HTTP. Deprecated HTTP+SSE is not
supported.

Stdio list-filter errors retain their detailed local diagnostics. Hosted MCP
returns the stable `airplan: invalid list filter arguments` message instead,
without echoing the supplied value; invalid `source` values use their separate
safe enumeration error.

The minimal tool set is:

| Tool                | Stdio | HTTP | Effect                             |
| ------------------- | ----- | ---- | ---------------------------------- |
| `upload_document`   | yes   | yes  | Upload supplied text content       |
| `update_document`   | yes   | yes  | Append a linked Markdown revision  |
| `upload_files`      | yes   | no   | Upload local paths as a collection |
| `list_uploads`      | yes   | yes  | List manifest or storage records   |
| `inspect_upload`    | yes   | yes  | Validate one marker-managed upload |
| `upgrade_document`  | yes   | yes  | Preview/apply one document upgrade |
| `upgrade_documents` | yes   | yes  | Preview/apply exact bulk plans     |
| `delete_upload`     | yes   | yes  | Delete one explicit upload         |
| `protect_upload`    | yes   | yes  | Mark one upload purge-protected    |
| `unprotect_upload`  | yes   | yes  | Remove purge protection            |
| `sync_manifest`     | yes   | yes  | Preview or apply reconciliation    |
| `preview_purge`     | yes   | yes  | Return explicit purge candidates   |
| `execute_purge`     | yes   | yes  | Delete reviewed upload IDs         |

The `upload_document` tool description identifies GFM, highlighted code,
Mermaid fences, GitHub-style alerts, frontmatter, footnotes, and responsive
columns as optional Markdown affordances to use when they improve clarity.

Hosted MCP omits file collection upload because MCP has no portable
client-to-server file upload and server-local paths are unsafe. No transport
exposes template dumping, configuration inspection, credentials, server
configuration, arbitrary S3 objects, or filesystem browsing.

`sync_manifest` defaults to dry-run unless `apply: true` is explicit.
`upgrade_document` also defaults to preview and requires `apply: true` to
mutate. `upgrade_documents` defaults to manifest preview; apply requires both
`apply: true` and the exact upgradeable plan items returned by preview.
`preview_purge` never deletes, and `execute_purge` accepts only explicit
`upload_id` values. `delete_upload` accepts an optional `force` boolean and
otherwise refuses purge-protected uploads; `protect_upload` accepts an
optional `reason`. Tool results are structured and warnings remain inside the
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
