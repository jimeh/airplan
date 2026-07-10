# airplan — Tool Specification

**Spec version: 0.6.0**

Changes in 0.6.0: uploaded objects use `no-store` so deletion is not
defeated by long-lived caches; invalid UTF-8 and
partial explicit credentials are rejected; configured URLs and key
prefixes are validated and object keys are URL-encoded in public
links; wrong-profile ensure-gone deletion cannot hide a still-live
manifest upload, and incomplete manifest checks produce warnings;
installed Go binaries report their module version;
and the default invocation timeout is 30 seconds (§2, §5–§9).

Changes in 0.5.2: layouts without the sticky table-of-contents rail
keep it reachable after the inline table of contents scrolls away (§3).

Changes in 0.5.1: built-in document controls use a quieter,
borderless treatment with a segmented view toggle and visible keyboard
focus (§3).

Changes in 0.5.0: markdown rendering supports GitHub-style alerts;
the built-in page gains a clearer typographic hierarchy, refined code
surfaces, and labelled rendered/source controls; and uploaded source
files can be opened raw as well as downloaded (§3).

Changes in 0.4.0: rendered markdown pages gain a responsive table of
contents and a wider document shell; the custom-template data contract
exposes rendered, highlighted, and raw source forms plus structured
headings and syntax CSS; `airplan template` emits an exact reusable
built-in template; and `airplan preview` renders locally without S3
access (§3, §6).

Changes in 0.3.2: `airplan list` text/table output renders sizes as
human-readable binary units; `--json` keeps exact byte counts (§9).

Changes in 0.3.1: purge's `--profile` gets the standard `-p` short
form and unified semantics — connection profile selection plus
record filter (§9). §9 also clarified for text input — remote recognition
keys off the 26-char base32 directory containing a `.html` page (the
source sibling may carry any extension, §3), and tombstones reference
the page key because the directory is the unit of deletion.

Changes in 0.3.0: `--lang` overrides the highlight language for text
input (§3, §6); unknown config file keys are rejected (§7).

Changes in 0.2.0: input size limit and `--max-size` (§2, §6);
configurable invocation timeout, default 20 s (§6, §7);
plain-text input rendered as a highlighted code page (§2, §3, §5,
§6, §8); binary input rejection (§2); profile resolution counts env
vars and flag overrides toward a complete non-profile configuration
(§7).

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
  → render (md → standalone HTML)  [skip for html]
  → generate object key (random dir + slug)
  → PUT page — and, for md input, the original .md alongside
  → append manifest entry
  → print public URL to stdout
```

Output contract (critical for agent use):

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

Size limit: input larger than the configured maximum — default
**10 MiB** — is rejected with an error before any upload. The whole
document is loaded into memory for rendering (md/text) or the noindex
splice (html), and a plan document over the default is invariably a
mistake — the wrong file, like a database dump. Implementations must
detect the overflow without buffering meaningfully past the limit.
`--max-size` sets the limit per invocation: a plain byte count, or an
integer with a `k`/`m`/`g` suffix (binary multiples; optional
trailing `b`/`ib`; case-insensitive — `10MB`, `512k`, `1gib`). `0`
removes the limit. There is deliberately no config key, so raising or
removing the guard stays a per-invocation decision.

---

## 3. Markdown Rendering

Markdown input is rendered to a fully standalone HTML page: embedded
CSS, no external fonts/scripts/assets, system font stack.

- Markdown dialect: CommonMark plus GitHub Flavored Markdown
  extensions — tables, strikethrough, task lists, autolinks — plus
  footnotes, heading anchors, and GitHub-style alerts. Alerts use the
  standard blockquote markers `NOTE`, `TIP`, `IMPORTANT`, `WARNING`,
  and `CAUTION`; they are converted to static HTML during
  rendering and may contain normal block Markdown. Unrecognized alert
  markers remain ordinary blockquotes.
- Trust boundary: raw inline/block HTML and link/image destinations are
  rendered as authored. Markdown and HTML input are trusted content and
  may execute active content when someone opens the resulting page.
  The original Markdown remains exact in source view and in the
  uploaded sibling.
- Fenced code blocks are syntax-highlighted at render time. The
  highlighting must follow `prefers-color-scheme` (light and dark
  palettes).
- Page styling: dark/light aware via `prefers-color-scheme`, a centered
  document shell around 54rem wide, prose constrained to a readable
  measure around 78ch, comfortable line height, distinct heading/body/
  muted color roles, and section hierarchy carried primarily by type and
  spacing rather than repeated divider rules. Code blocks and tables may
  use the full shell width so an 80-column source line fits without
  horizontal scrolling at the default font size. Inline and block code
  use separate subtle surfaces; block code has a quiet border and thin
  horizontal scrollbar.
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
  - Scroll position highlighting is a progressive enhancement and
    respects `prefers-reduced-motion`. The table of contents is hidden
    in source view and omitted when fewer than two entries remain.
- `<title>` from `--title`, else first `<h1>`, else source filename,
  else the resolved slug (covers stdin input with no `<h1>`).
- `<meta name="robots" content="noindex, nofollow">` — belt and
  braces on top of URL unguessability; works regardless of what
  headers the CDN/domain serves. Omitted under `--indexable`.
- Interactive niceties via a small amount of embedded vanilla JS —
  no frameworks, no external scripts, page stays fully standalone:
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

| Field                    | Type      | Meaning                              |
| ------------------------ | --------- | ------------------------------------ |
| `.Title`                 | string    | resolved title                       |
| `.RenderedHTML`          | raw HTML  | rendered markdown or text page body  |
| `.SourceText`            | string    | original unmodified source           |
| `.HighlightedSourceHTML` | raw HTML  | syntax-highlighted original source   |
| `.SyntaxCSS`             | raw CSS   | styles required by highlighted HTML  |
| `.Headings`              | heading[] | all markdown headings                |
| `.TOC`                   | heading[] | built-in H1-H3 ToC entries           |
| `.Format`                | string    | `md` or `txt`                        |
| `.Language`              | string    | resolved source-highlight language   |
| `.SourceName`            | string    | original basename; empty for stdin   |
| `.SourcePath`            | string    | relative path to the uploaded source |
| `.Slug`                  | string    | resolved slug                        |
| `.Indexable`             | boolean   | whether indexing is allowed          |

Each heading has `.Level` (1–6), `.ID`, `.Text`, and `.IsTitle`.
`.IsTitle` is true only for a leading H1 that the built-in table of
contents omits. `.TOC` is structured data, not pre-rendered navigation
HTML, so custom templates retain control of markup and presentation.

For compatibility, `.Body` remains an alias for `.RenderedHTML`,
`.SourceHTML` remains the markdown-only alias for
`.HighlightedSourceHTML` (and therefore stays empty for text input), and
`.FileName` remains the legacy text-input-only filename. New templates
should use the canonical fields.

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

- The tag is spliced in immediately after the first `<head …>` tag,
  found by a case-insensitive scan. This is a byte-level splice —
  the document is never parsed or re-serialized, and every other
  byte is served exactly as uploaded.
- If the document already contains a robots `<meta>` tag, nothing
  is injected — author intent wins.
- If no `<head>` tag is found, a warning is printed to stderr and
  the file is uploaded unmodified.
- `--indexable` disables injection entirely.

No other parsing or modification, ever. HTML input never uploads a
sibling source object: the uploaded object already is the original
file.

---

## 5. Upload Behavior

- The rendered page (or as-is HTML) is uploaded with:
  - `Content-Type: text/html; charset=utf-8`
  - `Cache-Control: no-store` — capability documents must remain
    revocable by deletion; neither browsers nor shared caches should
    retain a reusable response.
  - `x-amz-meta-title`: the resolved title, so `list --remote` can
    show titles via `HeadObject`.
- Markdown input additionally uploads the original source as
  `<random>/<slug>.md` (`text/markdown; charset=utf-8`, same cache
  headers) unless `--no-source`; text input likewise uploads its
  original file as `<random>/<slug>.<ext>`
  (`text/plain; charset=utf-8`, §3). The pair shares the random
  directory, so the page can link to it relatively (`./<slug>.md`,
  or `./<slug>.<ext>` for text input) on any domain. The source uploads first; failure of either upload
  fails the command (an orphaned first object is harmless; it never
  reaches the manifest, so cleaning it up takes `purge --remote`).
  stdout still carries only the page URL.
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

| Flag            | Default        | Notes                               |
| --------------- | -------------- | ----------------------------------- |
| `--format`      | auto           | `md`\|`html`\|`txt`; overrides §2   |
| `--slug S`      | from filename  | filename portion of the URL         |
| `--title T`     | from content   | page title (see §3 fallback chain)  |
| `--template P`  | built-in       | custom page template (md and text)  |
| `--no-source`   | off            | don't upload the original .md       |
| `--indexable`   | off            | no noindex meta (md and html, §3–4) |
| `--max-size N`  | 10MiB          | input size limit; 0 = no limit (§2) |
| `--timeout D`   | 30s            | invocation timeout; 0 = none        |
| `--lang L`      | from filename  | highlight language, text only (§3)  |
| `--json`        | off            | JSON object on stdout               |
| `--profile P`   | config default | named profile from config file      |
| `--config PATH` | XDG default    | alternate config file               |
| `--open`        | off            | open resulting URL in browser       |
| `--version`     |                |                                     |

Plus flag overrides for every connection setting (`--endpoint`,
`--bucket`, `--region`, `--public-base-url`, `--key-prefix`) for
one-off use.

Frequent flags get short forms: `-p` (`--profile`), `-s` (`--slug`),
`-t` (`--title`), `-j` (`--json`), `-o` (`--open`). Connection
overrides stay long-only. `airplan completion bash|zsh|fish` emits
shell completions.

If `--open` fails to launch a browser (common in headless/agent
environments), a warning goes to stderr and the exit code is
unaffected — the upload succeeded and the URL was already printed.

Released binaries report their release version under `--version`.
GoReleaser builds may stamp it directly; binaries installed through
the Go module path derive it from embedded Go build information.
Module pseudo-versions are reported without their leading `v`.
Unversioned local development builds, including dirty builds, report
`dev`.

The whole invocation is bounded by a timeout — default **30
seconds** — so a stalled endpoint fails with a clear error instead
of hanging the caller (often an agent harness) indefinitely.
Configurable via `--timeout` / `AIRPLAN_TIMEOUT` / the `timeout`
config key (root or profile level), with the usual precedence (§7).
Values are Go-style duration strings (`30s`, `1m30s`) or a bare
integer meaning seconds; `0` disables the timeout.

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
airplan template
airplan preview [flags] [file]
airplan completion bash|zsh|fish
airplan list [--remote] [--json]
airplan delete <url|key>
airplan purge [--remote] [--older-than 30d]
              [--all] [--dry-run] [--yes]
```

`config schema` prints the config file's JSON Schema (see §7).
`template` prints the built-in page template (see §3).
`preview` runs input detection and page rendering locally, writing the
resulting HTML to stdout or to `--output PATH`. It supports the rendering
flags `--format`, `--lang`, `--slug`, `--title`, `--template`,
`--indexable`, and `--max-size`, plus `--config` and `--profile` for
resolving template settings. It does not validate S3 connection fields,
access the network, upload source, or write the manifest. Consequently
`.SourcePath` is empty in a preview, while markdown's embedded source
view remains available. HTML input receives the same conservative
noindex injection as an upload. `file` omitted or `-` reads stdin;
`--output -` is equivalent to the stdout default. An output path that
resolves to the input file is rejected without modifying the input.
`list`/`purge` operate on the local upload manifest by default, or
on a live bucket listing with `--remote`. `delete` takes an explicit
URL or key, so it works on any upload regardless of which machine
made it. See §9.

---

## 7. Configuration

Resolution precedence: **flags > env vars > selected profile >
root-level values > built-in defaults**. Config file location:
`$XDG_CONFIG_HOME/airplan/config.toml`
(`~/.config/airplan/config.toml`; platform-appropriate config
directory on Windows), overridable with `--config` /
`AIRPLAN_CONFIG`.

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
# no_source = true    # behavior defaults; flags override
# timeout = "30s"     # invocation timeout; 0 = none
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

Unknown keys in the config file are an error naming the offending
key — typo protection, and it keeps the parser exactly in sync with
the published schema's `additionalProperties: false`.

If the config file contains credentials and is group- or
world-readable, a warning is printed to stderr.

Behavioral defaults: `no_source`, `indexable`, and `timeout` may be
set at the root or profile level; their flags override the config
values.

`public_base_url` is strongly recommended whenever the endpoint URL
isn't itself publicly readable (always the case for R2). If unset,
the URL is assembled as `<endpoint>/<bucket>/<key>` (path-style) and
a warning is printed to stderr noting it may not be publicly
reachable.

Validation at startup: missing bucket/endpoint/creds produce a clear
error naming the missing field, which profile was resolved (or that
root-level values were used), and the three ways to set it.

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
[<key_prefix>/]<random>/<slug>.html
[<key_prefix>/]<random>/<slug>.md      (markdown input, unless
                                        --no-source)
[<key_prefix>/]<random>/<slug>.<ext>   (text input's original file,
                                        unless --no-source; <ext>
                                        per §3)
```

Each upload owns one random directory; everything under it belongs
to that upload. Management commands treat the directory as the unit
of deletion, so page and source never get separated.

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
object-scoped token covers `DeleteObject` and `ListObjectsV2`;
public listing stays blocked either way).

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
 "bytes":18432}
{"type":"delete","time":"2026-07-09T09:12:44Z",
 "key":"vq3n.../plan.html"}
```

(Shown wrapped for readability; on disk each record is one line.)

- `time` is RFC 3339, UTC.
- `upload` records: `source_key` is omitted for HTML input and
  under `--no-source`; `title` is omitted when empty; `bytes`
  describes the page object; `profile` is the resolved profile
  name, omitted when root-level values were used.
- `delete` tombstones reference the upload by its page `key` — the
  random directory is the unit of deletion, so every sibling object
  (whatever its extension, §3) goes with it and nothing more is
  needed in the record.
- Forward compatibility: readers ignore unknown fields and skip
  records with an unknown `type`. No version field needed.

Concurrent invocations are expected (parallel agents on one
machine) and must be safe:

- Each record is written as a single append — one write of the full
  line, trailing newline included — to a file opened in append
  mode.
- Appends are wrapped in an advisory file lock (`flock` /
  `LockFileEx` style). All writers are airplan, so advisory
  suffices; the lock removes reliance on append atomicity, which
  doesn't hold on network filesystems.
- Readers tolerate a torn, malformed, or oversized line by skipping
  it with a warning on stderr — never by failing, never losing the
  rest of the file. Implementations may bound retained bytes per line,
  but must discard through the next newline before resuming.
- Never rewriting in place (tombstones, not deletion) means there
  is no read-modify-write cycle to race on.

### Commands

- `airplan list`: past uploads from the manifest (date, title,
  human-readable binary size, URL); `--json` for scripting with exact
  byte counts.
- `airplan delete <url|key>`: delete an upload — every object under
  its random directory, so page and markdown source go together —
  and tombstone its manifest entry if one exists (append a deletion
  record; the file stays append-only). Takes an explicit URL/key,
  so it also works on uploads made from other machines. Full URLs must
  use HTTP(S) and match the configured public base URL or endpoint by
  host and base path; HTTP and HTTPS variants of the same host are
  equivalent because the URL is parsed, not fetched. Bucket-only URL
  parsing is allowed only when neither connection URL is configured.
  Ensure-gone tombstoning checks a matching active manifest record: if its
  recorded bucket or profile differs from the active connection,
  deletion fails with an actionable error instead of hiding an upload
  that may still be live under another profile. If the manifest cannot
  be read, or malformed/oversized records were skipped, ensure-gone
  proceeds from the explicit target but warns that the bucket/profile
  check was skipped or may be incomplete.
- `airplan purge`: bulk delete driven by the manifest with filters —
  `--older-than 30d`, `--slug PATTERN`, `--profile P`. Durations
  accept `d`/`w` units. `--profile`/`-p` behaves as on every other
  command — it selects the connection profile — and on `purge` it
  additionally filters to uploads recorded with that profile, so
  purging a profile's uploads uses that profile's credentials. Requires at least one filter or an explicit
  `--all`. `--dry-run` previews; confirmation prompt unless `--yes`.
  Failed deletes are reported to stderr and left un-tombstoned so a
  re-run retries them. Purge only considers manifest records for the
  connected bucket (records from other buckets are skipped with a
  note). Deletion is ensure-gone: an upload whose directory no
  longer contains any objects is tombstoned as already deleted with
  a warning, not treated as a failure — so a manifest referencing
  externally-deleted objects converges instead of jamming. Suitable
  for cron (`purge --older-than 30d --yes`).
- `--remote` (on `list` and `purge`): operate on a bucket listing
  instead of the manifest, discovering uploads made from any
  machine. Airplan uploads are recognized by key shape: a
  `[key_prefix/]<26-char lowercase base32>/` directory under the
  profile's `key_prefix` that contains a `<slug>.html` page object
  (source siblings may carry any extension, §3). Unrelated objects
  in a shared bucket are never touched; deletion is per random
  directory, keeping page/source pairs together. `LastModified` from the listing
  drives `--older-than`.
  In a team bucket, each person sets their own `key_prefix`, which
  keeps `--remote` scoped to their own uploads.
- The local manifest still matters: it remembers titles and profile
  context, and works offline. `--remote` is the source of truth for
  what actually exists; `x-amz-meta-title` (set at upload) lets
  `list --remote` show titles via `HeadObject`.

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
