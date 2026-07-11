# airplan — Implementation Notes

How _our_ implementation of [SPEC.md](SPEC.md) is built: language,
dependencies, code structure, repo deliverables, phasing, and
testing. Behavior is defined exclusively by the spec; nothing here
may contradict it. Targets spec version 0.6.0.

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

| Dependency                  | Purpose                              |
| --------------------------- | ------------------------------------ |
| `yuin/goldmark` (+ GFM ext) | markdown → HTML body                 |
| `alecthomas/chroma/v2`      | code block syntax highlighting       |
| `aws/aws-sdk-go-v2` (s3)    | uploads (SigV4, retries, checksums)  |
| `BurntSushi/toml`           | config file parsing                  |
| `spf13/cobra`               | CLI: subcommands, flags, completion  |
| `invopop/jsonschema`        | config JSON Schema from Go structs   |
| `gofrs/flock`               | cross-platform manifest file locking |

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

Two public surfaces: the CLI (contract defined by SPEC.md) and an
importable Go package. No `internal/` directory — the core library
is meant to be pulled into other projects and tooling. The `main`
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
                        rendering, key/slug generation, S3 upload,
                        URL assembly; embeds template/CSS assets
                        via go:embed — no external assets at
                        runtime, ever
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
// res.URL, res.Key, res.SourceURL, res.Bytes, res.ContentType
```

## 4. Spec Requirements → Mechanisms

- Rendering: goldmark with GFM extensions (tables,
  strikethrough, task lists, autolinks), footnotes, heading anchors,
  and a small local AST transformer/renderer for GitHub-style alerts.
  Alert parsing and HTML generation happen before template execution;
  the uploaded page needs CSS for presentation but no alert JavaScript.
  Unsafe rendering remains enabled so Markdown preserves authored raw
  HTML and link destinations; Markdown and explicit HTML input share the
  same trusted-content boundary.
- Highlighting: chroma emitting class-based markup with CSS custom
  properties for the palette — required so highlighting can follow
  `prefers-color-scheme` (inline styles can't switch light/dark).
  The spec's source view is chroma's markdown lexer run at render
  time.
- Templates: Go `html/template`. Canonical template data exposes the
  raw source string, rendered and highlighted `template.HTML`, Chroma's
  `template.CSS`, structured headings/ToC entries, format metadata,
  title, slug, indexing intent, and source names/paths. Legacy
  `Body`/`SourceHTML`/`FileName` aliases remain. The built-in page CSS
  and JS are expanded into the embedded template source before parsing,
  so `airplan template` prints an exact reusable template containing
  only public data fields.
- Local rendering: `RenderInput` owns read limits, binary and invalid
  UTF-8 rejection,
  format detection, title/slug resolution, template execution, and
  noindex handling. `Client.Upload` adds source/page storage, URLs, and
  manifest recording; `airplan preview` stops after `RenderInput`.
- Key randomness: `crypto/rand` — never `math/rand` (spec requires a
  CSPRNG).
- Public URL assembly percent-encodes each object-key path segment;
  delete parsing uses `net/url` to recover the original UTF-8 key.
- `--older-than` durations: small custom parser for `d`/`w` units —
  Go's stdlib `time.ParseDuration` has no days.
- Manifest appends: `O_APPEND` open, whole line in one `Write` call,
  wrapped in `gofrs/flock` (flock on Unix, LockFileEx on Windows)
  per spec §9's concurrency rules. Readers discard malformed or
  oversized lines completely and resume at the following newline.
- Config/state paths: `os.UserConfigDir` for config; a small helper
  for the state dir (`XDG_STATE_HOME` → `~/.local/state`,
  `%LocalAppData%` on Windows — Go stdlib has no state-dir
  function).

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
  (Claude Code and compatible) to use airplan when the user asks for
  a plan or document they can open in a browser or share as a link —
  write it to a file (markdown and HTML on equal footing; whichever
  the agent already produced), run `airplan <file>`, capture the URL
  from stdout (`--json` when scripting), and present it as a
  clickable link; note that stdout carries only the URL. Frontmatter
  description tuned to trigger on "share this plan", "upload the
  plan", "give me a link to the plan" and similar.
- README: R2 setup walkthrough (bucket, custom domain, token scoped
  to Object Read & Write on the one bucket), `#:schema` editor
  setup, installing the agent skill (copy into `.agents/skills/`
  or `.claude/skills/`, or reference from a plugin marketplace), and
  an optional belt-and-braces note: serving `X-Robots-Tag: noindex`
  via a Cloudflare Transform Rule on the custom domain (S3/R2 can't
  emit custom response headers themselves).

## 7. Distribution

GoReleaser: cross-platform archives, checksums, SPDX JSON SBOMs from
Syft, Homebrew tap (cask);
`airplan.schema.json` bundled into archives and published as a
standalone release asset (the `#:schema` URL). Shell completions are
generated at runtime by `airplan completion` rather than shipped.
Releases are cut by release-please from conventional commits; the
tag triggers the GoReleaser publish workflow, which records GitHub
SLSA provenance attestations for every checksum-listed artifact.
`go install` works as a fallback and derives its version from Go build
information.

## 8. Phased Plan

### Phase 1 — MVP (usable end-to-end)

1. Repo scaffolding: module, root `main.go` + `cli/` + `airplan/`,
   CI (lint + test), Makefile with `build`, `test`, `lint` targets.
2. Core config: TOML file + env + flags, single profile,
   precedence, validation errors.
3. Core input: file/stdin reading, format detection, noindex
   splice for HTML input (spec §4).
4. Core render: goldmark + GFM + chroma, embedded template/CSS,
   dark/light, title resolution, noindex meta, download-markdown
   anchor (plain link — no JS in phase 1).
5. Core keygen: 128-bit base32 keys, slug sanitization.
6. Core storage: aws-sdk-go-v2 client (custom endpoint,
   path-style toggle), PutObject with content-type/cache headers —
   page plus sibling `.md` (and `--no-source` opt-out), URL
   assembly.
7. Wire-up: `cli` calling the core's public API only (dogfoods the
   Go surface), stdout/stderr contract, exit codes.
8. Verified manually against R2 with a custom domain, and against
   MinIO locally.

Exit criteria: `airplan plan.md` prints a working R2 URL; rendered
page is readable on desktop + mobile, light + dark.

### Phase 2 — Agent & daily-driver ergonomics

- Named profiles + `default_profile` + `--profile`.
- `--json` output mode.
- `--slug`, `--title`, `--open`.
- `--format` override; stdin sniffing hardening (BOM, whitespace).
- Record every upload in the local manifest (spec §9) so history
  already exists when the phase-3 commands ship; set
  `x-amz-meta-title` on upload.
- Default template interactivity: rendered/source toggle, "copy
  markdown", raw/download source links, per-code-block copy buttons —
  embedded vanilla JS with no-JS and print fallbacks (spec §3). Phase 1
  ships the template static, without JS.
- Custom template support (`--template` / `AIRPLAN_TEMPLATE` /
  profile `template`) with the documented data contract, plus
  `airplan template` to dump the built-in as a starting point.
- Config JSON Schema: `airplan config schema` subcommand, committed
  `schema/airplan.schema.json` with CI staleness check (see §5).
- Agent skill shipped in-repo (see §6).
- GoReleaser distribution (see §7).
- README (see §6).

### Phase 3 — History & cleanup (each independently optional)

- `airplan list` (manifest-driven; `--json`).
- `airplan delete <url|key>` (directory-unit deletion, manifest
  tombstones).
- `airplan purge` with filters, `--dry-run`, `--yes`, custom
  duration parser.
- `--remote` on `list`/`purge`: bucket listing, key-shape
  recognition, `key_prefix` scoping, `HeadObject` titles.

All behavior per spec §9.

## 9. Testing Strategy

- Unit: config precedence matrix, slug sanitization, format
  sniffing, key entropy/encoding properties, URL assembly.
- Golden files: markdown fixtures → rendered HTML snapshots
  (`testdata/`, `-update` flag convention).
- Integration: MinIO in a container (CI service / testcontainers);
  round-trip upload, then GET and compare bytes + headers. The image
  release tag and multi-platform digest are immutable-pinned together
  in `airplan/integration_test.go`.
- Smoke (manual or tagged, needs creds): real R2 upload via a
  scoped token, fetched through the custom domain.
