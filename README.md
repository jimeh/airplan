<div align="center">

<img height="196px" width="196px" src="./img/airplan.svg" alt="Logo">

# airplan

**Turn a local document into a readable, shareable link.**

[![GitHub Release](https://img.shields.io/github/v/release/jimeh/airplan?logo=github&label=Release)](https://github.com/jimeh/airplan/releases/latest)
[![Go Reference](https://img.shields.io/badge/pkg.go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/jimeh/airplan/airplan)
[![GitHub Issues](https://img.shields.io/github/issues/jimeh/airplan?logo=github&label=Issues)](https://github.com/jimeh/airplan/issues)
[![GitHub Pull Requests](https://img.shields.io/github/issues-pr/jimeh/airplan?logo=github&label=PRs)](https://github.com/jimeh/airplan/pulls)
[![License](https://img.shields.io/github/license/jimeh/airplan?label=License)](https://github.com/jimeh/airplan/blob/main/LICENSE)

</div>

airplan uploads a Markdown document, HTML page, or source file to
S3-compatible storage and prints an unguessable URL:

```console
$ airplan plan.md
https://plans.example.com/vq3nhk2p7r4wzt5c6ydjm3xhqd/plan.html
```

It is useful when an agent has written a plan that a person should review, or
whenever you want to share a local document without running a server or using a
paste service.

- Markdown becomes a polished, standalone page with light and dark themes.
- Source and plain-text files become highlighted, gist-like pages.
- HTML stays HTML, with no rendering step.
- Files live in a bucket you own. There are no accounts or background services.
- The command has a predictable output contract for scripts and agents.

The exact behavior is defined in [SPEC.md](SPEC.md).

## Install

With Homebrew:

```sh
brew install --cask jimeh/tap/airplan
```

With Go:

```sh
go install github.com/jimeh/airplan@latest
```

Prebuilt binaries are available from
[GitHub Releases](https://github.com/jimeh/airplan/releases).

## Configure storage

airplan works with any S3-compatible object store. You need a bucket, API
credentials that can read and write objects, and a public base URL from which
uploaded pages can be opened.

Create `~/.config/airplan/config.toml`:

```toml
#:schema https://github.com/jimeh/airplan/releases/latest/download/airplan.schema.json
endpoint          = "https://<account-id>.r2.cloudflarestorage.com"
bucket            = "plans"
region            = "auto"
public_base_url   = "https://plans.example.com"
access_key_id     = "..." # or AIRPLAN_ACCESS_KEY_ID
secret_access_key = "..." # or AIRPLAN_SECRET_ACCESS_KEY
```

If the file contains credentials, protect it with `chmod 600`. airplan warns
when its permissions are too broad. The `#:schema` comment enables validation
and completion in editors with [Taplo](https://taplo.tamasfe.dev/) or the Even
Better TOML extension.

Run `airplan config schema` to inspect every available setting. Configuration
can also come from `AIRPLAN_*` environment variables or command-line flags.
For multiple buckets, add `[profiles.name]` tables and select one with
`--profile`/`-p`. For a shared bucket, give each person a distinct
`key_prefix`.

### Cloudflare R2 setup

[Cloudflare R2](https://developers.cloudflare.com/r2/) is a good default when
you want S3 compatibility and a custom domain for public links.

1. Create a bucket, such as `plans`, in **R2 → Create bucket**.
2. Under **Bucket → Settings → Custom Domains**, connect a domain such as
   `plans.example.com`.
3. Under **R2 → Manage API Tokens**, create an **Object Read & Write** token
   scoped to this bucket. Do not use account-level or admin credentials.
4. Put the endpoint, bucket, custom domain, and token credentials in the config
   file shown above.

The custom domain should serve uploaded objects without exposing a public
bucket listing. As a quick check, its root URL should return an error while a
known object URL loads normally.

## Share a document

Pass airplan a file and use the URL it prints:

```sh
airplan plan.md                   # Markdown → rendered page
airplan report.html               # HTML → uploaded page
airplan pkg/server/handler.go     # source → highlighted page
airplan --open plan.md            # upload and open in a browser
airplan --json plan.md            # structured result for scripts
```

Standard input works too. It defaults to Markdown when no format can be
inferred:

```sh
cat plan.md | airplan --slug my-plan -
cat main.go | airplan --format txt --lang go -
```

### Preview without uploading

`preview` uses the same renderer locally. It does not need storage credentials,
contact S3, or update the upload history.

```sh
airplan preview plan.md > plan.html
airplan preview --output plan.html plan.md
```

### Manage uploads

```sh
airplan list                     # uploads recorded on this machine
airplan list --remote            # airplan uploads currently in the bucket
airplan delete <url-or-key>      # delete one upload
airplan purge --older-than 30d   # review and delete older uploads
```

Each successful upload is recorded in
`~/.local/state/airplan/manifest.jsonl`. Local commands use that history by
default. `--remote` reads the bucket instead, so it can find uploads from other
machines. Remote discovery recognizes airplan's key shape and leaves unrelated
objects in a shared bucket alone.

## Pages airplan creates

Markdown pages include syntax highlighting, a responsive table of contents,
GitHub-style alerts, rendered/source views, copy buttons, and links to the
original Markdown. Use `--no-source` if the original should not be uploaded.

Plain-text and source files use the same standalone page shell and infer their
highlight language from the filename. Use `--lang` to override it, especially
for input piped through stdin.

Everything needed to view a rendered page is embedded in the HTML. There are no
external fonts, scripts, or other page assets.

## Automation and agents

The command-line contract is intentionally simple:

- On success, stdout contains the URL and nothing else.
- With `--json`, stdout contains one JSON object instead.
- Logs, warnings, progress, and errors go to stderr.
- A non-zero exit means no upload URL was produced.

That makes direct capture safe:

```sh
url=$(airplan plan.md)
url=$(airplan --json plan.md | jq -r .url)
```

Do not invent or reuse a URL after a failed command. For the complete CLI,
config, key, and manifest contracts, use [SPEC.md](SPEC.md).

### Agent skill

The shipped [airplan skill](skills/airplan/SKILL.md) teaches compatible agent
harnesses when and how to share a finished document. Install it globally with
the [Skills CLI](https://skills.sh/):

```sh
npx skills add jimeh/airplan@airplan --global
```

## Privacy model

airplan links are capability URLs. Every path contains 128 random bits, and
rendered pages include a `noindex` directive. The link is effectively private
while it remains unknown, but it is not access-controlled:

- Anyone with the link can open it and pass it on.
- Chat tools may scan or prefetch links shared through them.
- Objects remain in the bucket until they are deleted.
- Bucket listing must remain private.

Use `airplan purge --older-than 30d --yes` manually or from cron when uploads
should expire. For defense in depth on Cloudflare, a Transform Rule can add an
`X-Robots-Tag: noindex` response header to the custom domain.

## Go library

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

The library exposes the same behavior as the CLI. See the
[Go reference](https://pkg.go.dev/github.com/jimeh/airplan/airplan) for its API
and [IMPLEMENTATION.md](IMPLEMENTATION.md) for the repository architecture.

## Development

The project uses [mise](https://mise.jdx.dev/) for its task surface:

```sh
mise run setup              # install tools and Git hooks
mise run check              # lint, generated files, format, and unit tests
mise run test-integration   # MinIO round trip; requires Docker
mise run verify             # CI-equivalent validation
```

See [AGENTS.md](AGENTS.md) for the repository map and contribution constraints.
Releases are managed by release-please and GoReleaser from conventional commits.

## License

MIT — see [LICENSE](LICENSE).
