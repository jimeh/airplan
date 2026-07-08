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

Order below ≈ dependency order; C=Claude, X=Codex.

1. Profiles + `default_profile` + `--profile`/`AIRPLAN_PROFILE` (X —
   mostly already in P1 config; finish + tests).
2. `--json` output, `--slug`, `--title`, `--format`, `--open` (X;
   Claude reviews `--json` schema exactness vs §6).
3. Manifest recording on upload: JSONL append, flock, state-dir
   helper (X — spec §9 write path only).
4. Template interactivity: rendered/source toggle, copy-markdown,
   per-block copy, no-JS/print fallbacks (C — vanilla JS, UX-heavy).
5. Custom templates: `--template`/env/profile, data contract struct,
   `airplan template` dump (X after C fixes the contract struct).
6. Config JSON Schema: invopop struct tags, `config schema` cmd,
   committed `schema/airplan.schema.json`, CI staleness check (X).
7. `completion` subcommand (X — cobra built-in).
8. Agent skill `skills/airplan/SKILL.md` (C — trigger copy is taste).
9. GoReleaser config + Homebrew tap wiring (X; tap repo needs user).
10. README: R2 walkthrough, schema editor setup, skill install,
    caveats (C drafts, user reviews — public-facing copy).

Review gate per batch; checkpoint at end of phase.

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
