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
[PLAN.md](PLAN.md) tracks phased execution status.

## Task surface (mise)

| Task                        | Purpose                                                     |
| --------------------------- | ----------------------------------------------------------- |
| `mise run setup`            | install tools + git hooks (run once)                        |
| `mise run check`            | fast handoff gate: lint + tests                             |
| `mise run test`             | unit tests (no Docker needed)                               |
| `mise run test-integration` | MinIO round-trip via testcontainers (needs Docker)          |
| `mise run lint` / `fmt`     | golangci-lint check / write-mode format                     |
| `mise run verify`           | CI-equivalent: check + workflows + integration + goreleaser |
| `mise run build`            | binary at `bin/airplan` (skipped when unchanged)            |

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
- **Config schema**: `schema/airplan.schema.json` is generated from
  the config structs and golden-tested; refresh with
  `go test ./airplan/ -run TestConfigSchema -update`. Unknown config
  keys are a hard error — parser and schema must not drift (SPEC §7).
- **Page assets** (`airplan/assets/`): embedded via go:embed; pages
  must stay fully standalone (no external fonts/scripts/requests).
- **Conventional commits are load-bearing**: PR titles are validated
  (semantic-pr) and release-please derives versions and changelogs
  from squash-merged titles.
- **Actions are SHA-pinned** (pinact); `mise run lint:workflows`
  fails on tag-pinned actions. `pinact run` re-pins after bumping.

## Layout

`airplan/` — core library, one cohesive package, public Go API
(`LoadConfig` / `New` / `Client.Upload`). `cli/` — cobra commands,
no business logic; anything the CLI does must be possible through
the library. `main.go` — shim. `skills/airplan/` — the _product's_
agent skill shipped to users (not guidance for working on this
repo).

## Verification beyond tests

A real end-to-end check needs an S3 target: integration tests spin
MinIO; a genuine R2 smoke test needs the developer's own
`~/.config/airplan/config.toml` (never commit credentials, never
print secret values). Rendered-page changes deserve a real-browser
look — light + dark (`prefers-color-scheme`) and narrow viewports.

## Releases

Merge to main → release-please maintains the release PR → merging it
tags `vX.Y.Z` → the release workflow builds with GoReleaser and
pushes the Homebrew cask. Credentials come from the release bot
GitHub App; no PATs.
