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

It is especially useful with coding agents: an agent can turn a local plan into
a link you can open from any device. That makes reviewing plans from a mobile
app practical even when the agent is running elsewhere and its local files are
hard to reach.

It also works whenever you want to share a local document without running a
server or using a paste service.

- Markdown becomes a polished page with light and dark themes.
  Authored HTML and link destinations are preserved, so treat it as trusted
  content.
- Source and plain-text files become highlighted, gist-like pages.
- HTML stays HTML, with no rendering step. Treat HTML input as trusted code: it
  may execute scripts when someone opens the link.
- Files live in a bucket you own. There are no accounts or background services.
- The command has a predictable output contract for scripts and agents.

## Live examples

- [Zero-downtime token migration](https://demo.airplan.dev/xyeknypg6lgzwpawpg4vshygeq/implementation-plan.html)
  is a realistic Markdown implementation plan with a diagram, responsive
  columns, alerts, tables, task lists, highlighted code, and automatic GitHub
  issue and pull-request links.
- [How airplan works](https://demo.airplan.dev/lsbpxbvfucyu6zkp5scelp6ni4/how-airplan-works.html)
  is a concise architecture overview showing the CLI and library workflows.
- [Upload with airplan's Go API](https://demo.airplan.dev/vdsuzbk6qkstspjbod4242rfge/upload-example.html)
  is a runnable Go example presented as a highlighted, gist-like page.

The exact behavior is defined in [SPEC.md](SPEC.md).

## Install

With Homebrew:

```sh
brew install --cask jimeh/tap/airplan
```

With mise:

```sh
mise use -g github:jimeh/airplan
```

With Go:

```sh
go install github.com/jimeh/airplan@latest
```

Prebuilt binaries are available from
[GitHub Releases](https://github.com/jimeh/airplan/releases).
The macOS release archives contain Developer ID-signed, hardened-runtime,
Apple-notarized binaries. Because a raw executable cannot carry a stapled
notarization ticket, its first Gatekeeper assessment may require internet
access. Binaries built locally with `go install` are not project-signed.

### Install the agent skill

This repository includes an [airplan agent skill](skills/airplan/SKILL.md) for
compatible coding agents. It teaches the agent to upload a document when you
ask for a shareable link, then return that link in chat.

Install it globally with the [Skills CLI](https://skills.sh/):

```sh
npx skills add jimeh/airplan --skill airplan --global
```

The `airplan` CLI must also be installed and configured on the machine where
the agent runs. Once it is, ask the agent to share a plan as a link and open the
result from your phone, tablet, or any other browser.

Release assets include separate SPDX JSON SBOMs, and the archives are covered by
GitHub artifact attestations. After downloading the release assets, verify them:

<!-- x-release-please-start-version -->

```sh
# Linux
sha256sum --ignore-missing --check checksums.txt
# macOS
shasum --ignore-missing --algorithm 256 --check checksums.txt

gh release verify v0.1.0 --repo jimeh/airplan

gh attestation verify airplan_0.1.0_darwin_arm64.tar.gz \
  --repo jimeh/airplan
```

<!-- x-release-please-end -->

Use the matching `.zip` name on Windows. Release verification checks GitHub's
immutable release attestation; artifact verification confirms that the archive
was produced by this repository's release workflow.

## Configure storage

airplan works with any S3-compatible object store. You need a bucket, a public
base URL, and API credentials with the permissions required by the commands you
use. Uploads need object-write access. Remote listing and all delete or purge
operations need bucket-list access; deletion also needs object-delete access.
Object-read access lets remote listings retrieve titles.

Create `~/.config/airplan/config.toml`:

```toml
#:schema https://github.com/jimeh/airplan/releases/latest/download/airplan.schema.json
endpoint          = "https://<account-id>.r2.cloudflarestorage.com"
bucket            = "plans"
region            = "auto"
public_base_url   = "https://plans.example.com"
access_key_id     = "..." # or AIRPLAN_ACCESS_KEY_ID
secret_access_key = "..." # or AIRPLAN_SECRET_ACCESS_KEY
# repo = "auto"           # infer GitHub origin for Markdown links
```

Explicit access and secret keys must be configured as a pair. Omit both to use
the standard AWS credential chain. Endpoint and public base URLs must be
absolute HTTP(S) URLs.

If the file contains credentials, protect it with `chmod 600`. airplan warns
when its permissions are too broad. The `#:schema` comment enables validation
and completion in editors with [Taplo](https://taplo.tamasfe.dev/) or the Even
Better TOML extension.

Run `airplan config schema` to inspect every available setting. Configuration
can also come from `AIRPLAN_*` environment variables or command-line flags.
For multiple buckets, add `[profiles.name]` tables and select one with
`--profile`/`-p`. For a shared bucket, give each person a distinct
`key_prefix`.

Use `airplan config profiles` to list configured profile names and identify an
explicit `default_profile`. Add `--json` / `-j` for scriptable output. This
inventory does not require profiles to contain complete or valid storage
settings and does not inspect AWS credential profiles.

Use `airplan config show` to inspect the active profile, resolved values, and
the winning source for each field. `airplan config show --json` provides the
same information for scripts. Access and secret key values are always
redacted; the command does not contact storage or resolve the standard AWS
credential chain.

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
airplan show <url-or-key>         # validate and inspect one remote upload
airplan delete <url-or-key>      # delete one upload
airplan purge --older-than 30d   # review and delete older uploads
```

Each successful upload is recorded in
`~/.local/state/airplan/manifest.jsonl`. Local commands use that history by
default. `--remote` reads the bucket instead, so it can find uploads from other
machines. Remote discovery recognizes exact `.airplan.json` ownership markers
with one bucket listing; it does not fetch each marker. Markerless directories
are invisible to airplan and cannot be deleted or purged through it. Use
`airplan show` when you need validated marker details and completeness state.

## Pages airplan creates

Markdown pages include syntax highlighting, Mermaid diagrams from exact
`mermaid` fences, a responsive table of contents, GitHub-style alerts,
definition lists, YAML/TOML frontmatter, responsive Pandoc columns,
rendered/source views, copy buttons, and links to the original Markdown.
Frontmatter is shown collapsed at the top, and its string `title` sets the page
title unless `--title` is given. Use `--no-source` if the original should not
be uploaded.

By default, Markdown references such as `#123`, `owner/repo#456`, and full
commit IDs link against a locally discovered GitHub `origin`. File repository
context wins; a file outside Git falls back to the current working directory,
which supports plans written to temporary directories. Use `--repo none` to
disable this or `--repo https://github.example/owner/repo` to supply explicit
GitHub Enterprise-compatible context. Discovery is local and never contacts
the remote.

Pandoc columns use an outer `{.columns}` fenced div containing two or more
`{.column}` children; optional validated `width="40%"` attributes weight them.
Columns stack on narrow screens and when printed.

Plain-text and source files use the same standalone page shell and infer their
highlight language from the filename. Use `--lang` to override it, especially
for input piped through stdin.

Everything except the conditionally loaded Mermaid runtime is embedded in the
HTML. Use `--no-external-assets` to keep airplan-managed features offline, or
`--mermaid-url` to select another HTTPS CDN or self-hosted module. This policy
does not block external content authored in trusted Markdown, HTML, or custom
templates. The original Markdown remains exact in source view and the optional
source object.

## Automation and agents

For upload invocations (`airplan <file>`), the command-line contract is
intentionally simple:

- On a successful upload, stdout contains the URL and nothing else.
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

## Privacy model

airplan links are capability URLs. Every path contains 128 random bits, and
rendered pages include a `noindex` directive. The link is effectively private
while it remains unknown, but it is not access-controlled:

- Anyone with the link can open it and pass it on.
- Chat tools may scan or prefetch links shared through them.
- Objects remain in the bucket until they are deleted. airplan serves them with
  `Cache-Control: no-store` so browsers and shared caches should not retain a
  reusable response after deletion.
- Bucket listing must remain private.

Use `airplan purge --older-than 30d --yes` manually or from cron when uploads
should expire. For defense in depth on Cloudflare, a Transform Rule can add an
`X-Robots-Tag: noindex` response header to the custom domain.

## Go library

The CLI is a thin shell over an importable Go package:

```go
import (
    "context"
    "fmt"
    "io"

    "github.com/jimeh/airplan/airplan"
)

func upload(ctx context.Context, f io.Reader) error {
    cfg, err := airplan.LoadConfig(airplan.ConfigOptions{})
    if err != nil {
        return err
    }
    client, err := airplan.New(ctx, cfg)
    if err != nil {
        return err
    }
    res, err := client.Upload(ctx, airplan.Input{
        Reader: f,
        Name:   "plan.md",
    })
    if err != nil {
        return err
    }
    fmt.Println(res.URL)
    return nil
}
```

The library exposes the same behavior as the CLI. See the
[Go reference](https://pkg.go.dev/github.com/jimeh/airplan/airplan) for its API
and [IMPLEMENTATION.md](IMPLEMENTATION.md) for the repository architecture.
Construct clients with `airplan.New`; nil contexts, nil configuration, and
zero-value clients return errors. Canceling a context stops waiting for a
blocked input reader, but callers that retain one must still unblock or close
it because Go cannot interrupt an arbitrary `io.Reader`.

## Development

The project uses [mise](https://mise.jdx.dev/) for its task surface:

```sh
mise run treeboot           # bootstrap a new linked worktree
mise run setup              # install tools and Git hooks
mise run check              # lint, generated files, format, and unit tests
mise run test:coverage      # statement summary + coverage.html report
mise run test-integration   # MinIO round trip; requires Docker
mise run verify             # CI-equivalent validation
mise run update:mermaid     # update an eligible, 72-hour-old Mermaid pin
```

See [AGENTS.md](AGENTS.md) for the repository map and contribution constraints.
Releases are managed by release-please and GoReleaser from conventional commits.

## License

MIT — see [LICENSE](LICENSE).
