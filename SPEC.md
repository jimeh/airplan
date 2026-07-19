# airplan — Tool Specification

**Spec version: 0.21.1**

Semantic versioning, applied to the spec itself: while below 1.0,
**minor** covers observable behavior changes — including breaking
pre-release corrections and backward-compatible additions — and
**patch** covers clarifications and editorial fixes. Once the
contract is deliberately declared stable at 1.0.0, **major** covers
breaking changes, **minor** covers backward-compatible additions,
and **patch** covers clarifications and compatible corrections. The
first implementation release does not by itself force spec 1.0.

`airplan` uploads AI/LLM agent plan files (markdown or HTML) to
S3-compatible object storage under a randomized, unguessable URL path
and prints the resulting URL. An agent finishes writing a plan, runs
`airplan plan.md`, and drops a clickable, effectively-private link into
chat for a human to review in the browser.

This document specifies **behavior only**: what the tool does, its
interfaces, and its on-the-wire and on-disk formats. It contains no
implementation detail; a conforming implementation can be built in any
language and remain fully compatible — same CLI, same config files,
same URLs, same page features, same manifest format. How _our_
implementation is built lives in [IMPLEMENTATION.md](IMPLEMENTATION.md).

Non-goals: no server component, no accounts, not a general pastebin.

---

## 1. Processing Model & Output Contract

One process, one straight-line pipeline, no daemon:

```
input (file|stdin)
  → detect format (md | html)
  → render (md → HTML page)  [skip for html]
  → generate object key (random dir + slug)
  → PUT ownership marker
  → PUT page — and, for md/text input, the original alongside
  → append manifest entry
  → print public URL to stdout
```

Upload output contract (critical for agent use):

- **stdout**: the final public URL and nothing else. With `--json`,
  a single JSON object and nothing else.
- **stderr**: all logs, warnings, progress, errors.
- **exit code**: 0 on success; non-zero on any failure. Never print a
  URL that wasn't successfully uploaded.

---

## 2. Input Handling

`airplan [flags] [file]` — `file` omitted or `-` reads stdin.

Three input formats: markdown (rendered, §3), HTML (uploaded as-is,
§4), and plain text (rendered as a highlighted code page, §3).

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
    controls instead occupy the first row opposite the theme toggle.
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

- Every upload first creates
  `[<key_prefix>/]<random>/.airplan.json`, the ownership marker for
  that random directory. The marker is UTF-8 JSON uploaded with
  `Content-Type: application/json` and `Cache-Control: no-store`.
  Its maximum size is 64 KiB. Version 1 has this shape:

  ```json
  {
    "schema": "airplan-upload",
    "version": 1,
    "directory": "vq3nhk2p7r4wzt5c6ydjm3xhqd",
    "created_at": "2026-07-08T14:03:11Z",
    "format": "md",
    "page": "plan.html",
    "source": "plan.md",
    "title": "Refactor auth"
  }
  ```

  `schema`, `version`, `directory`, `created_at`, `format`, and
  `page` are required. `schema` is exactly `airplan-upload`; version
  1 is the only version defined here. `directory` must equal the
  containing 26-character random directory. `created_at` is RFC
  3339 UTC. `format` is `md`, `html`, or `txt`. `page` and optional
  `source` are relative basenames — never paths — and must match the
  filename rules for that format in §3 and §8. `source` is omitted
  for HTML and under `--no-source`; `title` is omitted only when
  empty. Unknown fields are ignored. Duplicate field names, invalid
  UTF-8, malformed JSON, an unsupported version, unsafe or
  inconsistent filenames, and an oversized marker make the marker
  invalid.

- The rendered page (or as-is HTML) is uploaded with:
  - `Content-Type: text/html; charset=utf-8`
  - `Cache-Control: no-store` — capability documents must remain
    revocable by deletion; neither browsers nor shared caches should
    retain a reusable response.
  - `x-amz-meta-title`: the resolved title. The marker's `title` is
    authoritative for remote management; this metadata remains a
    convenience for direct object inspection.
- Markdown input additionally uploads the original source as
  `<random>/<slug>.md` (`text/markdown; charset=utf-8`, same cache
  headers) unless `--no-source`; text input likewise uploads its
  original file as `<random>/<slug>.<ext>`
  (`text/plain; charset=utf-8`, §3). The pair shares the random
  directory, so the page can link to it relatively (`./<slug>.md`,
  or `./<slug>.<ext>` for text input) on any domain.
- Upload order is marker → source, when present → page. Failure of
  any PUT fails the command and no local manifest upload record is
  written. Because the marker is first, any partial upload remains
  recognizably owned by airplan and appears remotely as `incomplete`
  until `purge --remote` removes it. An upload becomes `complete`
  when the marker's declared page and optional source both exist.
  stdout still carries only the page URL after the page PUT succeeds.
- Bucket must **not** allow listing publicly; privacy rests on the
  key being unguessable. Documentation covers the R2 setup: public
  bucket via custom domain (listing is not exposed) or Workers
  route.
- Region defaults to `auto` (R2 convention); real AWS users set it.

---

## 6. CLI Interface

```
airplan [flags] [file]
```

`file` omitted or `-` → read stdin.

| Flag                   | Default        | Notes                               |
| ---------------------- | -------------- | ----------------------------------- |
| `--format`             | auto           | `md`\|`html`\|`txt`; overrides §2   |
| `--slug S`             | from filename  | filename portion of the URL         |
| `--title T`            | from content   | page title (see §3 fallback chain)  |
| `--template P`         | built-in       | custom page template (md and text)  |
| `--no-source`          | off            | don't upload the original .md       |
| `--indexable`          | off            | no noindex meta (md and html, §3–4) |
| `--no-external-assets` | off            | disable managed view-time loads     |
| `--mermaid-url URL`    | pinned URL     | alternate HTTPS Mermaid module      |
| `--repo VALUE`         | `auto`         | `auto`, `none`, or repository URL   |
| `--max-size N`         | 10MiB          | input size limit; 0 = no limit (§2) |
| `--timeout D`          | 30s            | operation timeout; 0 = none         |
| `--lang L`             | from filename  | highlight language, text only (§3)  |
| `--json`               | off            | JSON object on stdout               |
| `--profile P`          | config default | named profile from config file      |
| `--config PATH`        | XDG default    | alternate config file               |
| `--open`               | off            | open resulting URL in browser       |
| `--version`            |                |                                     |

Plus flag overrides for every connection setting (`--endpoint`,
`--bucket`, `--region`, `--public-base-url`, `--key-prefix`) for
one-off use.

Frequent flags get short forms: `-p` (`--profile`), `-s` (`--slug`),
`-t` (`--title`), `-j` (`--json`), `-o` (`--open`). Connection
overrides stay long-only.
`airplan completion bash|zsh|fish|powershell` emits shell completions.

If `--open` fails to launch a browser (common in headless/agent
environments), a warning goes to stderr and the exit code is
unaffected — the upload succeeded and the URL was already printed.

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

Upload, preview, list, show, and delete each receive one timeout budget. Local
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

cat plan.md | airplan --slug refactor-auth
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

Errors: human-readable single-line message to stderr prefixed
`airplan:`; with `--json`, errors still go to stderr as text (stdout
stays reserved for the success object).

### Subcommands

```
airplan config schema
airplan config profiles [--config PATH] [--json]
airplan skill
airplan template
airplan preview [flags] [file]
airplan completion bash|zsh|fish|powershell
airplan list [--remote] [--json]
airplan show [--json] <url|key>
airplan delete <url|key>
airplan purge [--remote] [--older-than 30d]
              [--all] [--dry-run] [--yes]
```

`config schema` prints the config file's JSON Schema (see §7).
`skill` prints the complete canonical airplan agent skill to stdout,
byte-for-byte, including its YAML frontmatter and trailing newline. It accepts
no arguments or command-specific flags and emits nothing to stderr on success.
It does not load configuration, inspect credentials, access storage or the
network, or write state, so it works with only the installed binary and from
any working directory. The same content is available through the public core
library API.
`template` prints the built-in page template (see §3).
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

`list`/`purge` operate on the local upload manifest by default, or
on a live bucket listing with `--remote`. `show` inspects one remote
marker directory. `delete` takes an explicit URL or key, but it only
operates on a directory carrying a valid airplan ownership marker; it
therefore works on marker-managed uploads from any machine without
becoming a general-purpose bucket deletion command. See §9.

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

All connection/behavior keys may be set at the root level of the
config file as well as inside profiles. Root-level keys are base
values every profile inherits; a profile overrides only what it
sets. The simplest config needs no profiles at all:

```toml
# ~/.config/airplan/config.toml — minimal single-bucket setup
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
```

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
AIRPLAN_ENDPOINT
AIRPLAN_BUCKET
AIRPLAN_REGION
AIRPLAN_ACCESS_KEY_ID
AIRPLAN_SECRET_ACCESS_KEY
AIRPLAN_PUBLIC_BASE_URL
AIRPLAN_KEY_PREFIX
AIRPLAN_TEMPLATE
AIRPLAN_NO_EXTERNAL_ASSETS
AIRPLAN_MERMAID_URL
AIRPLAN_REPO
AIRPLAN_TIMEOUT
AIRPLAN_CONFIG
```

Credential fallback order: `AIRPLAN_*` env → profile file values →
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
accepts only `github.com`; it never contacts the remote. For a Markdown file,
the file's repository wins. Only when the file directory is not within any Git
repository does discovery fall back to the invocation working directory. A
file inside a repository whose origin is absent, invalid, or unsupported does
not fall back. Markdown from stdin uses the invocation working directory.
Discovery failure is non-fatal. `none` performs no discovery. HTML and text
inputs perform no discovery or reference linking.

The CLI and upload client default repository context to `auto`. The direct
local-rendering API's zero-value repository option performs no discovery;
library callers opt in by passing `auto`. The lower-level renderer receives
only an already resolved canonical URL and never runs Git itself.

Unknown keys in the config file are an error naming the offending
key — typo protection, and it keeps the parser exactly in sync with
the published schema's `additionalProperties: false`.

If the config file contains credentials and is group- or
world-readable, a warning is printed to stderr.

Behavioral defaults: `no_source`, `indexable`, `no_external_assets`,
`mermaid_url`, `repo`, and `timeout` may be
set at the root or profile level; their flags override the config
values.

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

Validation at startup: missing bucket/endpoint/creds produce a clear
error naming the missing field, which profile was resolved (or that
root-level values were used), and the three ways to set it.

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
[<key_prefix>/]<random>/.airplan.json
[<key_prefix>/]<random>/<slug>.html
[<key_prefix>/]<random>/<slug>.md      (markdown input, unless
                                        --no-source)
[<key_prefix>/]<random>/<slug>.<ext>   (text input's original file,
                                        unless --no-source; <ext>
                                        per §3)
```

Each upload owns one random directory. A valid `.airplan.json`
marker establishes airplan's authority over everything under that
directory; filename shape without the marker never establishes
ownership. Management commands treat the marked directory as the
unit of deletion, so page, source, marker, and any partial-upload
remnants never get separated.

- `<random>`: 16 bytes from a cryptographically secure random source
  (never a seeded PRNG), encoded lowercase base32 (RFC 4648
  alphabet, no padding) → 26 chars, **128 bits** of entropy.
  Lowercase-only sidesteps case-folding corruption that
  base62/base64 URLs suffer; 128 bits makes brute-force enumeration
  (even with no rate limiting) computationally absurd.
- `<slug>`: human-readable filename portion so links look sane in
  chat and downloads name themselves. From `--slug`, else the source
  filename stem, else `plan`. Sanitized: lowercased, non
  `[a-z0-9-]` → `-`, collapsed, trimmed, max 64 chars; if
  sanitization leaves an empty string (e.g. an all-non-ASCII
  filename), fall back to `plan`. Contributes zero entropy by
  design — privacy never depends on it.
- `.html` extension: helps any static host / CDN infer content type
  and makes saved files open correctly.

Example keys:

```
vq3nhk2p7r4wzt5c6ydjm3xhqd/.airplan.json
vq3nhk2p7r4wzt5c6ydjm3xhqd/refactor-auth.html
vq3nhk2p7r4wzt5c6ydjm3xhqd/refactor-auth.md
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
directory on Windows) — append-only JSONL, one record per line.
Deletions append tombstone records; the file is never rewritten in
place. The manifest is best-effort convenience, never a source of
truth: it only knows about uploads made from this machine.

Record schema — exact field names are part of this spec, so two
conforming implementations can share a manifest:

```json
{"type":"upload","time":"2026-07-08T14:03:11Z",
 "key":"vq3n.../plan.html","source_key":"vq3n.../plan.md",
 "url":"https://plans.example.com/vq3n.../plan.html",
 "bucket":"plans","profile":"work","title":"Refactor auth",
 "bytes":18432,"marker_version":1}
{"type":"delete","time":"2026-07-09T09:12:44Z",
 "key":"vq3n.../plan.html"}
```

(Shown wrapped for readability; on disk each record is one line.)

- `time` is RFC 3339, UTC.
- `upload` records: `source_key` is omitted for HTML input and
  under `--no-source`; `title` is omitted when empty; `bytes`
  describes the page object; `profile` is the resolved profile
  name, omitted when root-level values were used; `marker_version`
  is the ownership-marker version written for the upload. Current
  writers always include `marker_version`; its absence identifies a
  legacy upload recorded before ownership markers were introduced.
- `delete` tombstones reference the upload by its page `key` — the
  random directory is the unit of deletion, so every sibling object
  (whatever its extension, §3) goes with it and nothing more is
  needed in the record.
- Forward compatibility: readers ignore unknown fields and skip
  records with an unknown `type`. The record itself needs no schema
  version; `marker_version` describes the remote upload format.
  Readers retain an otherwise-valid upload with no `marker_version`
  as legacy history, but it never authorizes delete or purge. An
  unsupported nonzero `marker_version` is invalid and skipped with a
  warning.

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
- `airplan list --remote`: cheaply discovers marker directories made
  from any machine. It performs only paginated bucket LIST operations
  beneath the active profile's `key_prefix`; it does not GET markers,
  HEAD pages, or trust marker content. It groups every returned object
  beneath an exact
  `[key_prefix/]<26-char lowercase base32>/` directory, then emits only
  groups containing the exact `.airplan.json` marker key. Page/source
  filename shape without that marker is never evidence of visibility.
  Unmarked directories are invisible.
- Remote list rows have `DATE`, `OBJECTS`, `SIZE`, `SLUG`, and
  `DIRECTORY` columns. `DATE` is the marker object's storage
  last-modified time. `OBJECTS` and `SIZE` count every object and byte
  recursively beneath the random directory, including the marker,
  nested keys, and unrecognized extras. `SLUG` is inferred only when
  exactly one direct-child object matches the §8 page filename shape
  (`[a-z0-9-]{1,64}.html`): it is that object's basename without
  `.html`. With zero or multiple matching objects, `SLUG` is `-`.
  `DIRECTORY` is the 26-character random directory without
  `key_prefix`. Rows sort by marker last-modified time, then marker
  key.
- `list --remote --json` prints an array with one object per row. Its
  stable fields are `time` (RFC 3339 marker last-modified time), `dir`,
  `marker_key` (the full storage key), `objects`, and `bytes`; `slug`
  is present only when inferred unambiguously. These entries describe
  marker-key presence and directory occupancy, not validated uploads.
  A malformed, oversized, or unsupported marker remains visible here
  because ordinary remote listing never reads it.
- `airplan show <url|key>` performs targeted inspection of one remote
  marker directory. The target may be its random directory, marker,
  or any direct child; full URLs and path-style endpoint URLs obey the
  same connection, bucket, and prefix checks as `delete`. `show`
  fetches and validates the marker, lists every object recursively
  beneath the directory, and reports marker fields, declared page and
  source existence and sizes, total object count and bytes, and a
  state of `complete`, `incomplete`, or `invalid`. A valid marker is
  `complete` when its declared page and optional source both exist;
  otherwise it is `incomplete`. Extra objects do not affect state. A
  present marker whose bytes cannot be validated is `invalid`; this is
  a successful inspection result but grants no deletion authority. A
  missing marker is an error. Storage, authentication, timeout,
  cancellation, and other request failures fail the command; they are
  never reported as marker states.
- `show --json` emits one object. All states contain `state`, `dir`,
  `marker_key`, `objects`, and `bytes`. Valid states additionally
  contain `time` (marker `created_at`), `format`, `page`, and `title`
  when non-empty; `source` is present when declared. `page` and
  `source` are objects containing `key`, `url`, `exists`, and `bytes`,
  with `bytes` omitted when the object is missing. An invalid result
  additionally contains `error`, a stable coarse code:
  `oversized`, `malformed_json`, `unsupported_version`, or
  `invalid_fields`; it never exposes untrusted marker fields. Human
  output presents the same information as a labeled detail block.
- `airplan delete <url|key>` only deletes a marker-managed upload.
  The target may be the random directory, its `.airplan.json` marker,
  or the page/source named by a valid marker. Any other sibling key is
  rejected. Before issuing any deletion, airplan fetches and validates
  the exact marker in the target directory. A missing, malformed,
  oversized, unsupported, or inconsistent marker is an error and no
  bucket objects are touched. A directory without a valid marker is
  not an airplan upload, regardless of its key shape; native storage
  tooling is the escape hatch.
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
- Before `delete` resolves its connection, it consults a uniquely
  matching active, marker-managed local manifest record. When neither
  `--profile` nor `AIRPLAN_PROFILE` is set and that record names a
  profile, the recorded profile overrides the general config default;
  stderr notes the selection. URL targets participate in this inference
  only when they are HTTP(S) URLs whose host matches the recorded public
  URL; URL query strings and fragments are ignored. With zero or multiple
  matching records, normal config resolution proceeds without inference.
  Explicit flag or environment selection always wins and is never silently
  changed. If marker lookup then fails and the matching record names another
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
  `incomplete` uploads, using marker `created_at` for `--older-than`
  and the marker-declared page slug for `--slug` even if the page is
  missing. It never selects an invalid marker. Such a directory cannot
  be deleted by airplan; `show` can inspect it and native storage
  tooling must clean it. Marker-last deletion keeps an interrupted
  purge discoverable and retryable.
  In a team bucket, each person sets their own `key_prefix`, which
  keeps `--remote` scoped to their own uploads.
- The local manifest still matters: it remembers titles and profile
  context, and works offline. Remote listing is the cheap storage view;
  `show`, `delete`, and `purge --remote` read marker state when they
  need validated upload details or deletion authority.

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
