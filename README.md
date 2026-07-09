<div align="center">

<img height="196px" width="196px" src="./img/airplan.svg" alt="Logo">

# airplan

**Upload plan documents to S3-compatible storage — get back an
unguessable, shareable link.**

[![GitHub Release](https://img.shields.io/github/v/release/jimeh/airplan?logo=github&label=Release)](https://github.com/jimeh/airplan/releases/latest)
[![Go Reference](https://img.shields.io/badge/pkg.go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/jimeh/airplan/airplan)
[![GitHub Issues](https://img.shields.io/github/issues/jimeh/airplan?logo=github&label=Issues)](https://github.com/jimeh/airplan/issues)
[![GitHub Pull Requests](https://img.shields.io/github/issues-pr/jimeh/airplan?logo=github&label=PRs)](https://github.com/jimeh/airplan/pulls)
[![License](https://img.shields.io/github/license/jimeh/airplan?label=License)](https://github.com/jimeh/airplan/blob/main/LICENSE)

</div>

Upload a plan, doc, or source file to S3-compatible storage under an
unguessable URL — and get back a link anyone can open in a browser.

```sh
$ airplan plan.md
https://plans.example.com/vq3nhk2p7r4wzt5c6ydjm3xhqd/plan.html
```

Built for AI/LLM agents that finish writing a plan and need to drop a
clickable, effectively-private link into chat for a human to review —
but just as handy by hand. Markdown renders to a readable standalone
page (dark/light aware, syntax highlighting, source toggle, copy
buttons); HTML uploads as-is; plain-text and source files become
highlighted, gist-like code pages.

No server, no accounts, no daemon: one binary, one bucket you own.
Behavior is fully specified in [SPEC.md](SPEC.md).

## Install

```sh
brew install --cask jimeh/tap/airplan   # Homebrew
go install github.com/jimeh/airplan@latest
```

Or grab a binary from the
[releases](https://github.com/jimeh/airplan/releases).

## Setup (Cloudflare R2)

Any S3-compatible storage works; R2 is the sweet spot (free egress,
custom domains). Once:

1. **Create a bucket** — e.g. `plans` — in the Cloudflare dashboard
   (R2 → Create bucket).
2. **Connect a custom domain** (bucket → Settings → Custom Domains),
   e.g. `plans.example.com`. This serves objects publicly _without_
   exposing bucket listing — exactly the privacy model airplan needs.
   Verify: `https://plans.example.com/` should return an error, while
   uploaded objects load fine.
3. **Create an API token** (R2 → Manage API Tokens): **Object Read &
   Write**, scoped to that one bucket only. Never use account-level
   or admin credentials.
4. **Write the config**:

```toml
#:schema https://github.com/jimeh/airplan/releases/latest/download/airplan.schema.json
# ~/.config/airplan/config.toml
endpoint          = "https://<account-id>.r2.cloudflarestorage.com"
bucket            = "plans"
region            = "auto"
public_base_url   = "https://plans.example.com"
access_key_id     = "..."   # or AIRPLAN_ACCESS_KEY_ID env var
secret_access_key = "..."   # or AIRPLAN_SECRET_ACCESS_KEY env var
```

`chmod 600` the file if it holds credentials — airplan warns
otherwise. The `#:schema` line gives you validation and completion in
editors with [Taplo](https://taplo.tamasfe.dev/) or the Even Better
TOML extension.

Multiple buckets? Use `[profiles.name]` tables and `--profile`/`-p`
(see `airplan config schema` for every key). Shared team bucket? Give
each person their own `key_prefix`.

## Usage

```sh
airplan plan.md                     # markdown → rendered page
airplan report.html                 # HTML → uploaded as-is
airplan pkg/server/handler.go       # source file → highlighted page
cat plan.md | airplan -s my-plan -  # stdin (defaults to markdown)
airplan --json plan.md              # one-line JSON for scripts
airplan -o plan.md                  # open in browser too
```

Output contract, built for scripting and agents: stdout is the URL
and nothing else (or a single JSON object with `--json`); everything
else goes to stderr; non-zero exit means nothing was uploaded.

Rendered markdown pages are fully standalone — embedded styles, no
external assets — with a rendered/source toggle, copy-markdown and
per-code-block copy buttons, and a download link to the original
`.md` uploaded alongside (`--no-source` to skip). Text input picks
its highlight language from the filename; use `--lang` when piping
(`cat main.go | airplan --format txt --lang go -`).

Every upload is recorded in a local manifest
(`~/.local/state/airplan/manifest.jsonl`) — history and cleanup
commands build on it.

## Agent skill

`skills/airplan/SKILL.md` teaches Claude Code (and compatible
harnesses) to share plans via airplan when asked. Install by copying
into your project or user skills directory:

```sh
mkdir -p ~/.claude/skills/airplan
curl -fsSL https://raw.githubusercontent.com/jimeh/airplan/main/skills/airplan/SKILL.md \
  -o ~/.claude/skills/airplan/SKILL.md
```

(`.agents/skills/` works too for harnesses that read it.)

## Privacy model, honestly stated

Links are **capability URLs**: 128 bits of `crypto/rand` in every
path makes guessing computationally absurd, and rendered pages carry
`noindex` meta tags. But unguessable ≠ private-forever:

- Anyone you give a link to can pass it on.
- Chat tools may scan or prefetch URLs shared through them.
- Objects stay in the bucket until deleted — periodic cleanup is
  coming in the `purge` command; until then, delete via your storage
  provider's tools.
- Keep bucket listing non-public (the R2 custom-domain setup above
  gets this right by default).

Belt and braces: R2/S3 can't emit custom response headers themselves,
but on Cloudflare you can add a Transform Rule on the custom domain
setting `X-Robots-Tag: noindex` for defense in depth.

## Library

The CLI is a thin shell over an importable Go package:

```go
import "github.com/jimeh/airplan/airplan"

cfg, _ := airplan.LoadConfig(airplan.ConfigOptions{})
client, _ := airplan.New(ctx, cfg)
res, _ := client.Upload(ctx, airplan.Input{
    Reader: f,
    Name:   "plan.md",
})
fmt.Println(res.URL)
```

Anything the CLI does, the package does — same spec-defined behavior.

## Development

Tooling runs through [mise](https://mise.jdx.dev): `mise run build`,
`mise run test`, `mise run lint`, `mise run test-integration` (spins
a MinIO container via testcontainers). Releases are cut by
release-please + GoReleaser on merged conventional commits.

## License

MIT — see [LICENSE](LICENSE).
