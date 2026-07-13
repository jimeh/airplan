# airplan — agent guide

Go CLI + importable library that uploads plan documents (markdown,
HTML, plain text) to S3-compatible storage under an unguessable URL
and prints that URL.

## The one rule

**[SPEC.md](SPEC.md) is the authoritative behavior contract.** Any
change to observable behavior — CLI flags, output, config keys, key
scheme, manifest format, page features — updates SPEC.md **in the
same PR**, including its version per the semver rules at the top of
the file. Code comments reference spec sections (`SPEC.md §7`); keep
them accurate. [IMPLEMENTATION.md](IMPLEMENTATION.md) describes how
this implementation is built and must not contradict the spec.
[RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) tracks the v0.1.0
release-hardening work and evidence gates.

## Task surface (mise)

| Task                               | Purpose                                                     |
| ---------------------------------- | ----------------------------------------------------------- |
| `mise run treeboot`                | bootstrap a linked worktree from the root checkout          |
| `mise run setup`                   | install tools + git hooks (run once)                        |
| `mise run check`                   | fast handoff gate: lint + generated files + format + tests  |
| `mise run test`                    | unit tests (no Docker needed)                               |
| `mise run test:coverage`           | unit tests + text and HTML statement coverage reports       |
| `mise run test-integration`        | MinIO round-trip via testcontainers (needs Docker)          |
| `mise run lint`                    | all lints: `lint:go`, `lint:workflows`                      |
| `mise run format` / `format:check` | write / check formatting (`:go`, `:markdown`)               |
| `mise run generate`                | refresh committed generated files                           |
| `mise run generate:check`          | fail if generated files are stale                           |
| `mise run verify`                  | CI-equivalent: check + workflows + integration + goreleaser |
| `mise run build`                   | binary at `bin/airplan` (skipped when unchanged)            |

Run `mise run check` before handing off; `verify` for broad or risky
changes. Lefthook pre-commit hooks lint/format-check staged files.
Tool versions: major-version constraints live in `mise.toml`; exact
pins live in `mise.lock` (commit both when bumping tools).

## Conventions that bite

- **Output contract (SPEC §1)**: stdout carries the result URL (or
  one JSON object) and _nothing else_; everything else → stderr.
  Tests assert this; don't print to stdout casually in `cli/`.
- **Golden files**: rendering snapshots live in `airplan/testdata/`;
  refresh with `go test ./airplan/ -run TestRenderMarkdownGolden
-update` after template/CSS/JS changes.
- **Repository text files use LF on every platform** via
  `.gitattributes`; byte-exact golden and generated-file tests depend
  on this even when Git runs on Windows.
- **Worktree bootstrap** (`.treeboot.toml`): copy the root checkout's
  ignored `mise.local.toml` once, then run `mise run setup`. Existing
  worktree-local copies are intentionally preserved.
- **Removed worktree lint cache**: if `golangci-lint` reports source paths from
  a deleted worktree, run `golangci-lint cache clean` before retrying. Cached
  issues otherwise keep references to files that no longer exist.
- **Config schema**: `schema/airplan.schema.json` is generated from
  the config structs and golden-tested; refresh with
  `go test ./airplan/ -run TestConfigSchema -update`. Unknown config
  keys are a hard error — parser and schema must not drift (SPEC §7).
- **Manifest reads**: handle empty non-EOF reads as errors before continuing;
  reading a directory can otherwise spin without making progress.
- **Page assets** (`airplan/assets/`): embedded via go:embed. Mermaid is the
  only airplan-managed external load and is conditional. Update its pin with
  `mise run update:mermaid`; dependency-only updates never bump SPEC.md.
- **Markdown input is trusted content**: Goldmark's unsafe renderer is
  intentionally enabled so authored HTML and URL destinations survive.
  Do not add sanitization without changing the product trust boundary
  in SPEC.md.
- **Markdown alerts** (`airplan/alert.go`): Goldmark splits markers
  such as `[!NOTE]` across multiple text nodes. Reconstruct the first
  blockquote line when matching alerts; do not assume one marker node.
- **Markdown columns** (`airplan/columns.go`): pre-validate complete fenced
  containers before the Goldmark block parser creates custom nodes. Invalid
  or incomplete column syntax must remain ordinary literal Markdown.
- **Conventional commits are load-bearing**: PR titles are validated
  (semantic-pr) and release-please derives versions and changelogs
  from squash-merged titles.
- **Actions are SHA-pinned** (pinact); `mise run lint:workflows`
  fails on tag-pinned actions. `pinact run` re-pins after bumping.
- **GoReleaser PR checks are opt-in**: apply the `ci:goreleaser`
  label when a PR changes `.goreleaser.yaml` or release packaging.
  The check remains unconditional on pushes to `main`.
- **MinIO is immutable-pinned** in `airplan/integration_test.go`:
  update the release tag and multi-platform digest together, inspect
  the image labels, then run `mise run test-integration`.
- **Real R2 release smoke tests may use `AIRPLAN_TIMEOUT=60s`** when
  local firewall approval could interrupt the sequence. The product
  default remains 30 seconds.

## Layout

`airplan/` — core library, one cohesive package, public Go API
(`LoadConfig` / `New` / `Client.Upload` / `RenderInput`). `cli/` —
cobra commands, no business logic; anything the CLI does must be
possible through the library. `main.go` — shim. `skills/airplan/` —
the _product's_ agent skill shipped to users (not guidance for
working on this repo).

## Verification beyond tests

A real end-to-end check needs an S3 target: integration tests spin
MinIO; a genuine R2 smoke test needs the developer's own
`~/.config/airplan/config.toml` (never commit credentials, never
print secret values). Rendered-page changes deserve a real-browser
look — light + dark (`prefers-color-scheme`) and narrow viewports.
For an isolated module-version `go install` smoke test, invoke
`$(mise which go)` directly; the mise Go shim can reset a temporary
`GOBIN` to mise's managed binary directory.

## Releases

Merge to main → release-please maintains the release PR → merging it
creates a notes-bearing draft release → the reusable release workflow
builds with GoReleaser, uploads and verifies every asset, records
attestations, pushes the Homebrew cask, then publishes the immutable
release and tag. Retry failed publications with the release workflow's
manual tag/SHA inputs while the release remains a draft. Credentials
come from the release bot GitHub App; no PATs. Keep the release-please
release name and GoReleaser `name_template` equal to the full tag:
GoReleaser finds an existing draft by title, not `tag_name`.
