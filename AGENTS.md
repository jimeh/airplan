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

## Task surface (mise)

| Task                               | Purpose                                                    |
| ---------------------------------- | ---------------------------------------------------------- |
| `mise run treeboot`                | bootstrap a linked worktree from the root checkout         |
| `mise run setup`                   | install tools + git hooks (run once)                       |
| `mise run check`                   | fast handoff gate: lint + generated files + format + tests |
| `mise run check:go`                | Go-only gate: lint + generated files + format + tests      |
| `mise run check:go-version`        | check `.go-version` matches `go.mod`                       |
| `mise run check:spec-sync`         | check contract changes update spec versions                |
| `mise run test`                    | unit tests (no Docker needed)                              |
| `mise run test:coverage`           | unit tests + text and HTML statement coverage reports      |
| `mise run test-integration`        | MinIO round-trip via testcontainers (needs Docker)         |
| `mise run test:browser`            | Chromium page smoke tests (installs browser on demand)     |
| `mise run audit:deps`              | verify modules + scan Go and npm dependencies              |
| `mise run lint`                    | all lints: `lint:go`, `lint:workflows`                     |
| `mise run format` / `format:check` | write / check formatting (`:go`, `:markdown`)              |
| `mise run generate`                | refresh committed generated files                          |
| `mise run generate:check`          | fail if generated files are stale                          |
| `mise run release:snapshot`        | build release artifacts without publishing                 |
| `mise run verify`                  | broad local check + integration + release snapshot         |
| `mise run build`                   | binary at `bin/airplan` (skipped when unchanged)           |

Run `mise run check` before handing off; `verify` for broad or risky
changes. Lefthook pre-commit hooks lint/format-check staged files.
Tool versions: major-version constraints live in `mise.toml`; exact
pins live in `mise.lock` (commit both when bumping tools).
The exact Go version lives in `.go-version`, is consumed by local mise and
Actions setup-go, and must match the `go` directive in `go.mod` and the Go
entry in `mise.lock`. When bumping Go, update `.go-version` and `go.mod`, then
run `mise lock go` and commit the refreshed lockfile.
CI additionally executes the unit tests on native Windows; that platform
coverage has no equivalent local task on non-Windows hosts.

## Conventions that bite

- **Output contract (SPEC §1)**: stdout carries the result URL (or
  one JSON object) and _nothing else_; everything else → stderr.
  Tests assert this; don't print to stdout casually in `cli/`.
- **SPEC synchronization sensor**: `mise run check:spec-sync` compares against
  `SPEC_SYNC_BASE` (default `origin/main`). It treats `main.go`, non-test Go
  files under `cli/` and `airplan/`, `airplan/assets/`, and
  `schema/airplan.schema.json` as contract-sensitive, including deletions and
  moves out of those paths. Local policy findings fail; PR CI reports them as
  warnings while signal quality matures. Git and parsing errors always fail.
- **Golden files**: rendering snapshots live in `airplan/testdata/`;
  run `GOLDEN_UPDATE=1 go test ./airplan/ -run TestRenderMarkdownGolden`
  after template/CSS/JS changes.
- **Repository text files use LF on every platform** via
  `.gitattributes`; byte-exact golden and generated-file tests depend
  on this even when Git runs on Windows.
- **Shell completions are Cobra-generated at runtime** for Bash, Zsh, Fish,
  and PowerShell. Keep the supported shell lists in README.md and SPEC.md
  aligned with the generated `airplan completion` subcommands.
- **Worktree bootstrap** (`.treeboot.toml`): copy the root checkout's
  ignored `mise.local.toml` once, then run `mise run setup`. Existing
  worktree-local copies are intentionally preserved.
- **Removed worktree lint cache**: if `golangci-lint` reports source paths from
  a deleted worktree, run `golangci-lint cache clean` before retrying. Cached
  issues otherwise keep references to files that no longer exist.
- **Config schema**: `schema/airplan.schema.json` is generated from
  the config structs and golden-tested; refresh with
  `GOLDEN_UPDATE=1 go test ./airplan/ -run TestConfigSchema`. Unknown config
  keys are a hard error — parser and schema must not drift (SPEC §7).
- **Manifest reads**: handle empty non-EOF reads as errors before continuing;
  reading a directory can otherwise spin without making progress.
- **Ownership marker resolution**: targeted get/delete operations concurrently
  probe both exact marker names and fail closed unless exactly one exists.
  LIST-backed operations reuse their snapshot and reject dual-marker conflicts.
- **Remote reconciliation**: sync may parallelize marker GETs, but it must use
  one LIST snapshot, confirm apparent absence with a targeted not-found, and
  lock/reread/reduce before deterministic manifest appends. Purge deletions
  remain sequential even when marker inspection concurrency is overridden.
- **Page assets** (`airplan/assets/`): embedded via go:embed. Mermaid is the
  only airplan-managed external load and is conditional. Update its pin with
  `mise run update:mermaid`; dependency-only updates never bump SPEC.md.
- **Live demos**: README demo links are maintained by
  `.github/workflows/update-demos.yml` from the sources and upload-mode goldens
  in `airplan/testdata/`. Published demo URLs are permanent; automation may
  replace README links but never deletes old or superseded uploads.
- **Browser smoke tests** (`tests/browser/`): Playwright generates its fixture
  through `airplan preview` with isolated configuration, then covers Chromium
  across desktop/narrow and light/dark projects. Keep selectors behavioral and
  accessible; screenshots and traces are failure evidence, not golden files.
  Resolve the active toolchain with `go env GOROOT`, then invoke its binary
  directly; a mise shim can re-inject stripped `AIRPLAN_*` variables from
  worktree-local mise environment configuration.
- **Print disclosures**: Chromium hides closed `details` content through its
  `::details-content` box. Forced child display can expose hidden, script, or
  style content; use the pseudo-element fallback plus `beforeprint`/`afterprint`
  state handling. Cover frontmatter and authored disclosures in browser tests.
- **Markdown input is trusted content**: Goldmark's unsafe renderer is
  intentionally enabled so authored HTML and URL destinations survive.
  Do not add sanitization without changing the product trust boundary
  in SPEC.md.
- **Named-input text sniffing**: the 8 KiB document/collection sniff uses up to
  `utf8.UTFMax-1` bytes of lookahead so a valid rune split at the boundary is
  not mistaken for binary data. Preserve NUL detection and reject genuinely
  malformed UTF-8.
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
- **Dependency intake is delayed by seven days** for routine Go module, npm,
  and GitHub Actions updates. Security updates bypass the Dependabot cooldown.
  `mise run audit:deps` verifies Go modules, checks reachable Go
  vulnerabilities, and audits npm development dependencies at high severity.
- **Cross-compilation target variables belong on the build step**, not the CI
  job. Job-level `GOOS`/`GOARCH` values make mise install target-platform Go
  tools that cannot run on the host runner.
- **Actions Go ownership**: setup-go installs Go from `.go-version` and owns
  module/build caching. Actions sets `MISE_DISABLE_TOOLS=go` so mise only
  installs non-Go tools, and mise shims stay off `PATH` so setup-go remains
  authoritative. Pull requests restore but do not save mise tool caches;
  production releases disable both setup-go and mise caches.
- **GoReleaser PR checks are opt-in**: apply the `ci:goreleaser`
  label when a PR changes `.goreleaser.yaml` or release packaging.
  The check remains unconditional on pushes to `main`.
- **Draft releases require an eager tag**: keep release-please's
  `force-tag-creation` enabled. It builds the next release PR immediately
  after creating a draft; without the tag it replays released commits.
- **Draft release assets require push access**: GitHub rejects a read-only
  token even when listing or downloading assets. Native macOS verification
  therefore uses `contents: write`, but exposes the token only to its download
  step and never to binary execution.
- **macOS releases fail closed**: production GoReleaser runs require all three
  Apple secrets and three identity variables, sign and notarize before
  packaging, and publish only after native Intel and Apple Silicon checks.
  Snapshots stay secretless. Raw executables cannot carry stapled notarization
  tickets, so first Gatekeeper assessment may require internet access.
- **Cask publication is the final release step**: GoReleaser OSS generates the
  Cask without uploading it. Preserve it for seven days as a same-run immutable
  artifact. A separate downstream job atomically updates the tap only after
  native checks and immutable release publication pass. A failed update must
  leave the prior Cask working; re-run failed jobs to retry only that job.
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
creates a tag and notes-bearing draft release → the reusable release
workflow builds with GoReleaser, uploads and verifies every asset, records
attestations, publishes the immutable release and locks its tag, then pushes
the generated Homebrew Cask. Retry failed draft publications with the release
workflow's manual tag/SHA inputs. Credentials come from the release bot GitHub
App; no PATs. Keep the release-please
release name and GoReleaser `name_template` equal to the full tag:
GoReleaser finds an existing draft by title, not `tag_name`.
