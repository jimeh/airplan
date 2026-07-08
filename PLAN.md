# airplan — Build Plan

Execution plan for building [SPEC.md](SPEC.md) v0.1.0 per
[IMPLEMENTATION.md](IMPLEMENTATION.md). Codex-first: bounded,
well-specified packets go to Codex (gpt-5.5) in isolated worktrees;
Claude owns architecture, API surface, page template/UX, integration,
and final judgement. Status: **Phase 1 complete (2026-07-08): PR
open, CI green, R2 smoke test passed end-to-end via the custom
domain. Phases 2–3 not started.**

---

## 1. Division of Labor

| Owner  | Work                                                        |
| ------ | ----------------------------------------------------------- |
| Claude | package API surface & type stubs, page template (HTML/CSS/JS),
|        | config struct shape (drives schema), integration & wiring
|        | review, README copy, skill copy, final review judgement      |
| Codex  | mechanical/spec-driven modules: config precedence, input
|        | detection + noindex splice, keygen/slug, storage, manifest,
|        | duration parser, CLI subcommand plumbing, test suites,
|        | golden fixtures, CI + GoReleaser config                      |

Rationale (model routing): the spec is unusually precise — most
modules are "clear-spec, mechanically implementable, independently
verifiable" → Codex territory. Taste-critical surfaces (template
design, Go API shape, copy) stay with Claude.

Every Codex packet: delegated via `codex-implementation` skill,
isolated worktree, explicit acceptance criteria (spec section refs +
tests that must pass). Claude inspects and reconciles each result
before merge. Review gate before any phase is called done:
`codex-review` on the diff + Claude's own pass.

## 2. Environment (verified 2026-07-08)

Go 1.26.4, golangci-lint 2.12.2, Docker 29.3.1 (MinIO integration
tests), Codex CLI 0.143.0, GoReleaser 2.17.0, mise. Module path:
`github.com/jimeh/airplan` (assumed — confirm).

## 3. Phase 1 — MVP

Goal: `airplan plan.md` prints a working URL; page readable
desktop/mobile, light/dark. Spec §1–§6, §8; IMPLEMENTATION §8/Phase 1.

### Step 0 — Skeleton first (Claude, sequential)

Fan-out only works against a stable frame. Claude lays down:

- `go.mod`, root `main.go`, `cli/root.go` shell, `airplan/` package
  with **type stubs + function signatures for every module**
  (config, input, render, keygen, storage, manifest paths) —
  compiles, does nothing.
- `Makefile` (`build`, `test`, `lint`), `.golangci.yml`,
  GitHub Actions CI (lint + test).
- Public API per IMPLEMENTATION §3 sketch: `LoadConfig`, `New`,
  `Client.Upload`, `Input`, `Result`.

This freezes the seams so parallel packets can't drift apart.

### Step 1 — Parallel Codex packets (worktree each)

| Packet | Contents | Spec refs |
| ------ | -------- | --------- |
| P1 config | TOML load, root+profile merge, env, flag overlay, profile resolution (5-step), validation errors, perms warning | §7 |
| P2 input | file/stdin read, format detection (flag > ext > sniff), HTML noindex splice rules | §2, §4 |
| P3 keygen | crypto/rand 16B → base32 lower no-pad (26 ch), slug sanitize chain, key assembly with prefix | §8 |
| P4 storage | s3 client (custom endpoint, path-style, `when_required` checksums), PutObject headers (+`x-amz-meta-title`), page+source ordering, URL assembly incl. no-`public_base_url` fallback + warning | §5 |

Each packet ships with its unit tests (config precedence matrix,
sniffing table, key/slug properties, header assertions against a
fake/recorded S3). Merge order: any; conflicts limited to stub
replacement by construction.

### Step 2 — Render (Claude, parallel with Step 1)

- Template + CSS is the product's face → Claude designs it
  directly: standalone page, `prefers-color-scheme`, ~72ch measure,
  chroma class-based highlighting with CSS custom properties,
  noindex meta, download-markdown plain anchor. **No JS in phase 1**
  (per IMPLEMENTATION §8).
- goldmark+GFM+footnotes+anchors wiring, title resolution chain,
  `go:embed` assets.
- Golden-file tests (`testdata/`, `-update` convention) — fixture
  authoring can go to Codex once the template settles.

### Step 3 — Wire-up + verification (Claude)

- `cli` root command calls core public API only; stdout/stderr/exit
  contract (§1); connection-override flags.
- MinIO integration test (docker): upload → GET → bytes+headers
  round-trip. Tagged/skippable when docker absent.
- Manual R2 smoke — **needs bucket + scoped token from user**.
- Review gate: `codex-review` on the full phase diff.

**Checkpoint: demo Phase 1 end-to-end before Phase 2.**

## 4. Phase 2 — Ergonomics

Status: **breakdown proposed 2026-07-08, awaiting discussion.**
Phase 1 already shipped several items the original phase-2 list
expected: full profile resolution, `--slug`/`--title`/`--format`,
sniffing hardening, `x-amz-meta-title`.

Same workflow as phase 1: branch `feat/phase-2-ergonomics`, Claude
lays seams first, Codex packets fan out in isolated worktrees,
Claude does the taste-critical work in parallel, merge → verify →
`codex-review` gate → PR.

### Step 0 — Seams (Claude, sequential)

- Manifest module stub (`airplan/manifest.go`): record types per
  spec §9, state-dir helper signature, append/lock seam.
- Template data contract struct (`.Title`, `.Body`, `.SourceHTML`,
  `.SourcePath`, `.Slug`, `.FileName`) and `RenderOptions` growth —
  frozen before X4 codes against it.
- CLI seams: `--json`/`--open`/`--profile` flag stubs, subcommand
  constructors (`template`, `config schema`, `completion`).

### Codex packets (parallel worktrees)

| # | Packet | Contents | Spec |
|---|--------|----------|------|
| X1 | manifest | state-dir helper (XDG_STATE_HOME → ~/.local/state; %LocalAppData% on Win), JSONL append via gofrs/flock, upload-record schema, wire into upload path, torn-line-tolerant reader (needed by tests now, phase 3 commands later) | §9 |
| X2 | cli-flags | `--json` (exact §6 schema, one line), `--open` (browser launch, warn-don't-fail), `--profile`/`-p`, short forms `-s -t -j -o`, `completion` subcommand | §6 |
| X3 | schema | invopop/jsonschema from config structs (descriptions via struct tags), `airplan config schema` cmd, committed `schema/airplan.schema.json`, CI staleness check | §7 |
| X4 | templates | load custom template from `--template`/`AIRPLAN_TEMPLATE`/profile key, md+text only (warn on HTML input), `airplan template` dump cmd | §3 |
| X5 | release | GoReleaser: 5-platform archives, checksums, Homebrew tap (jimeh/homebrew-tap), completions + schema in archives, version via ldflags; snapshot build in CI | — |

### Claude work (parallel with packets)

- C1: `.SourceHTML` embedding + template interactivity — rendered/
  source toggle, copy-markdown, per-code-block copy buttons,
  no-JS/print fallbacks, touch behavior (spec §3). Vanilla JS,
  UX-heavy, browser-verified light/dark/mobile.
- C2: agent skill `skills/airplan/SKILL.md` (trigger copy is taste).
- C3: README — R2 setup walkthrough, `#:schema` editor setup, skill
  install, security caveats, X-Robots-Tag transform rule note.
- C4: integration, `--json` schema exactness review, R2 smoke of
  every new surface, review gate.

### Sequencing constraints

- C1 (contract + SourceHTML) blocks X4; everything else is
  independent after Step 0.
- X3 wants the config structs final — they are (timeout landed).
- X5 last to merge (version stamping touches cli).

### Deferred / decisions needed (see §8b)

- `--lang` flag for stdin text highlighting → spec 0.3.0?
- Manifest writes: core library vs CLI-only.
- Schema URL for `#:schema` directive.
- First tagged release timing.

Review gate per merge batch; checkpoint at end of phase.

## 5. Phase 3 — History & cleanup

Each independently optional (per IMPLEMENTATION §8):

- `list` (manifest; `--json`) — X.
- `delete <url|key>` — directory-unit delete + tombstone — X.
- `purge` + filters + `--dry-run`/`--yes` + d/w duration parser — X.
- `--remote` on list/purge: key-shape recognition, prefix scoping,
  HeadObject titles, LastModified ages — X, C reviews the
  recognition regex/logic carefully (deletes other people's data if
  wrong).

Manifest reader (torn-line tolerance) lands here with `list`.

## 6. Testing Strategy

- Unit tests ride inside every packet (acceptance criteria include
  them): config precedence matrix, sniffing, slug/keygen properties,
  URL assembly, duration parser, manifest lock/append/torn-line.
- Golden files for rendering (`testdata/`, `-update`).
- Integration: MinIO container round-trip (bytes + Content-Type +
  Cache-Control + meta-title), auto-skip without docker.
- Manual R2 smoke at each phase checkpoint (custom domain fetch).
- CI: lint + test on linux; build matrix for the five release
  platforms (compile-only).
- Review gate: `codex-review` + Claude pass on every phase diff
  before it's called done.

## 7. Git Workflow

Conventional commits, one commit per landed packet (`feat(config): …`,
`feat(render): …`). Work directly on `main` unless user prefers PRs
per phase — confirm.

## 8. Decisions (settled 2026-07-08)

1. Module path: `github.com/jimeh/airplan`.
2. R2 smoke: `~/.config/airplan/config.toml` already exists with R2
   details + credentials — usable from day one.
3. Git workflow: **one PR per phase** (branch per phase).
4. Scope: Phase 1, then halt for user review.
5. Homebrew tap exists: `github.com/jimeh/homebrew-tap` (Phase 2).
6. Go: latest (1.26).
7. Tooling: **mise** for tool management and task running — no
   Makefile. Rebuild-skipping handled by mise task
   `sources`/`outputs` change detection.

## 9. Phase 2 open questions (2026-07-08)

1. **Manifest writes — core or CLI?** Spec §9 says every upload is
   recorded; parallel-agent safety argues for the core library doing
   it (any consumer benefits). But a library silently writing to
   XDG state would surprise embedders. Proposal: core writes it,
   with an exported opt-out on Config for embedders (spec stays
   silent on the library surface).
2. **`--lang` flag** for stdin text highlighting (deferred from
   phase 1). Small, rounds out text mode, needs a spec bump to
   0.3.0 (backward-compatible addition). Include in phase 2?
3. **Schema URL** for the `#:schema` editor directive: pin to a
   release asset URL (stable, versioned) or raw.githubusercontent
   main (always current)? Proposal: release asset of the latest
   tag, documented in README.
4. **First release timing**: X5 lands GoReleaser config + a CI
   snapshot build in the PR; the actual v0.1.0 tag + Homebrew tap
   publish happens after merge. Proposal: tag-triggered release
   workflow; user pushes the tag.
