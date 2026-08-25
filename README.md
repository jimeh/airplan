<div align="center">

<img height="196px" width="196px" src="./img/airplan.svg" alt="Airplan logo">

# airplan

**Turn local documents and artifacts into shareable links.**

[![GitHub Release](https://img.shields.io/github/v/release/jimeh/airplan?logo=github&label=Release)](https://github.com/jimeh/airplan/releases/latest)
[![Go Reference](https://img.shields.io/badge/pkg.go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/jimeh/airplan/airplan)
[![GitHub Issues](https://img.shields.io/github/issues/jimeh/airplan?logo=github&label=Issues)](https://github.com/jimeh/airplan/issues)
[![GitHub Pull Requests](https://img.shields.io/github/issues-pr/jimeh/airplan?logo=github&label=PRs)](https://github.com/jimeh/airplan/pulls)
[![License](https://img.shields.io/github/license/jimeh/airplan?label=License)](https://github.com/jimeh/airplan/blob/main/LICENSE)

</div>

airplan uploads documents and file collections to S3-compatible storage and
prints unguessable URLs:

```console
$ airplan plan.md
https://plans.example.com/vq3nhk2p7r4wzt5c6ydjm3xhqd/plan.html
```

It is especially useful with coding agents. An agent can turn a local plan into
a link you can open from any device, even when the agent runs elsewhere and its
local files are hard to reach. It also works whenever you want to share a local
document without running a server or using a paste service.

Airplan handles several kinds of input:

- Markdown becomes a polished page with eleven built-in themes, independent
  light and dark mode themes, and validated custom themes. Authored HTML and
  link destinations are preserved, so treat the input as trusted content.
- Source and plain-text files become highlighted, gist-like pages.
- A primary Markdown file and its supporting pages and assets become one linked
  document bundle.
- HTML stays HTML, with no rendering step. It may execute scripts when someone
  opens the link, so treat it as trusted code.
- Images, video, audio, PDFs, archives, and other peer artifacts become a file
  collection with an overview page.
- Files live in a bucket you own. The CLI can call S3 directly, or clients can
  use one single-user Airplan server that holds the storage credentials.
- stdout has a stable contract for scripts and agents. URLs or one JSON object
  go to stdout; logs and errors go to stderr.

[SPEC.md](SPEC.md) defines the exact behavior.

## Live examples

- [Zero-downtime token migration][airplan-demo-implementation-plan]
  is a realistic Markdown implementation plan with a diagram, responsive
  columns, alerts, tables, task lists, highlighted code, and automatic GitHub
  issue and pull-request links.
- [How airplan works][airplan-demo-how-it-works]
  is a concise architecture overview showing the CLI and library workflows.
- [Upload with airplan's Go API][airplan-demo-go-api]
  is a runnable Go example presented as a highlighted, gist-like page.
- [API rollout handoff][airplan-demo-document-bundle]
  is a document bundle with linked Markdown and Go pages, page navigation, and
  an SVG request-flow asset.
- [Release verification evidence][airplan-demo-collection]
  is a file collection with an image preview and direct links to every original
  artifact.

[airplan-demo-implementation-plan]: https://demo.airplan.dev/evul6bxtrsh3mokdgwrj22frqq/implementation-plan.html
[airplan-demo-how-it-works]: https://demo.airplan.dev/rbarjn4cdxs5oxf6pgnpzvw6ja/how-airplan-works.html
[airplan-demo-go-api]: https://demo.airplan.dev/755ezjbjo3jfkgcdwwmti2atuq/upload-example.html
[airplan-demo-document-bundle]: https://demo.airplan.dev/vhfu6263aupd42gd7puzobvw2i/implementation-plan.html
[airplan-demo-collection]: https://demo.airplan.dev/sxxrmahx2drwrfexkvm6gb3dua/index.html

## Install

On macOS, install the signed and notarized release with Homebrew:

```sh
brew install --cask jimeh/tap/airplan
```

On Linux, macOS, or Windows, mise can install the matching release binary:

```sh
mise use -g github:jimeh/airplan
```

If you have Go, build the latest release from source:

```sh
go install github.com/jimeh/airplan@latest
```

Building requires the Go version declared in `go.mod`. Locally built binaries
are not signed by the project. You can also download prebuilt Linux, macOS, and
Windows binaries from [GitHub Releases](https://github.com/jimeh/airplan/releases).

See [Verify a release](docs/release-verification.md) for checksums, signatures,
attestations, SBOMs, and reproducible container references.

## Quick start

The default `s3` backend calls storage directly from the CLI. You need an
S3-compatible bucket, a public base URL, and credentials scoped to that bucket.
Uploads require object-write access. Commands that inspect or delete remote
uploads also need object-read, bucket-list, or object-delete access as relevant.

If several machines should share one storage configuration, follow
[Run a single-user Airplan server](docs/server.md) instead.

### Create a Cloudflare R2 bucket

[Cloudflare R2](https://developers.cloudflare.com/r2/) provides an S3-compatible
API and custom domains for public links. To use it:

1. Create a bucket, such as `plans`, in **R2 > Create bucket**.
2. Under **Bucket > Settings > Custom Domains**, connect a domain such as
   `plans.example.com`.
3. Under **R2 > Manage API Tokens**, create an **Object Read & Write** token
   scoped to this bucket. Do not use account-level or admin credentials.
4. Record the S3 endpoint, bucket name, custom domain, access key ID, and secret
   access key for the next step.

The custom domain should serve uploaded objects without exposing a public bucket
listing. As a quick check, its root URL should return an error while a known
object URL loads normally.

Another S3-compatible service works too. Substitute its endpoint, region,
bucket, credentials, and public URL in the same configuration.

### Configure airplan

Create `~/.config/airplan/config.toml` on Linux or macOS. Windows uses its normal
per-user configuration directory.

```toml
#:schema https://github.com/jimeh/airplan/releases/latest/download/airplan.schema.json
backend           = "s3"
endpoint          = "https://<account-id>.r2.cloudflarestorage.com"
bucket            = "plans"
region            = "auto"
public_base_url   = "https://plans.example.com"
light_theme       = "github-light"
dark_theme        = "github-dark"
access_key_id     = "..." # or AIRPLAN_ACCESS_KEY_ID
secret_access_key = "..." # or AIRPLAN_SECRET_ACCESS_KEY
```

Replace the placeholders with your storage values. Explicit access and secret
keys must be configured as a pair. Omit both to use the standard AWS credential
chain.

If the file contains credentials, protect it with `chmod 600`. Airplan warns
when its permissions are too broad. The `#:schema` comment enables validation
and completion in editors with [Taplo](https://taplo.tamasfe.dev/) or the Even
Better TOML extension.

Run `airplan config show` to inspect the resolved values and their sources.
Credentials are redacted. The [configuration guide](docs/configuration.md)
covers profiles, environment variables, themes, precedence, and diagnostics.

### Upload a document

Pass Airplan a file and use the URL it prints:

```console
$ airplan plan.md
https://plans.example.com/vq3nhk2p7r4wzt5c6ydjm3xhqd/plan.html
```

Add `--open` to open the result in a browser, or `--json` for structured output:

```sh
airplan --open plan.md
airplan --json plan.md
```

## Share documents and files

### Upload one document

Airplan infers the document format from its name and content:

```sh
airplan plan.md                # Markdown page
airplan report.html            # authored HTML
airplan pkg/server/handler.go  # highlighted source page
```

Standard input defaults to Markdown when no format can be inferred:

```sh
cat plan.md | airplan --slug my-plan -
cat main.go | airplan --format txt --lang go -
```

### Upload a document bundle

Pass a primary Markdown document with its supporting pages and assets:

```sh
airplan README.md docs/design.md examples/server.go \
  images/flow.svg recordings/demo.mp4
```

Airplan prefers a root `README.md`, then `index.md`, then the first Markdown
file as the entry. `--entrypoint PATH` overrides that choice. Other Markdown and
UTF-8 source files become linked pages. HTML, media, and opaque resources become
byte-exact assets. Use `--page` or `--asset` when an inferred role is wrong.

The entry file's directory is the bundle root, and every path must remain
beneath it after Airplan resolves symlinks. Assets keep their nested paths and
bytes. The plain result remains the entry URL. JSON adds ordered `.pages` and
`.assets` arrays.

### Upload a file collection

Multiple files without Markdown or HTML create a collection. Airplan prints
each direct file URL in argument order, then the overview URL:

```console
$ airplan login.png settings.png demo.webm
https://plans.example.com/vq3n.../login.png
https://plans.example.com/vq3n.../settings.png
https://plans.example.com/vq3n.../demo.webm
https://plans.example.com/vq3n.../index.html
```

A single recognized media or binary file also becomes a collection. Use
`--collection` when text-like input should be uploaded unchanged:

```sh
airplan screenshot.png
airplan --collection README.md
airplan --json screenshot.png recording.webm
```

Collection JSON retains `.url` for the overview and puts ordered direct links
in `.files[].url`. Airplan does not change the original file bytes.

### Preview without uploading

`preview` uses the same renderer locally. It does not need storage credentials,
contact S3, or update upload history.

```sh
airplan preview plan.md > plan.html
airplan preview -o plan.html plan.md
airplan preview README.md docs/design.md images/flow.svg \
  --output-dir ./preview
airplan preview --collection screenshot.png demo.webm -o index.html
```

A preview with supporting pages or assets requires `--output-dir`. Airplan
refuses an existing directory so an interrupted preview cannot look complete.

## Manage uploads

Airplan records successful uploads in the platform state directory. On Linux
and macOS the default is `~/.local/state/airplan/manifest.jsonl`; Windows uses
`%LocalAppData%\airplan\manifest.jsonl`. Override it with `--manifest PATH` or
`AIRPLAN_MANIFEST`.

Common lifecycle commands are:

```sh
airplan list                         # local upload history
airplan list --remote                # uploads found in the bucket
airplan show <url-or-key>            # inspect one remote upload
airplan get <url-or-key>             # fetch its entry page
airplan protect <url-or-key>         # exclude it from purge
airplan delete <url-or-key>          # delete one upload
airplan purge --older-than 30d       # review and delete older uploads
airplan sync                         # reconcile remote and local history
airplan upgrade --check <url-or-key> # inspect a Markdown upgrade
airplan new-revision <url-or-key> plan.md
```

`new-revision` preserves every revision's source, URL, and numbered identity.
The renderer may still be upgraded in place without changing those values.

See [Manage uploads](docs/upload-management.md) for filtering, output columns,
remote discovery, synchronization, protection, Markdown upgrades, and linked
revisions.

## Pages Airplan creates

Markdown pages include per-theme syntax highlighting, Mermaid diagrams from
`mermaid` fences, a responsive table of contents, GitHub-style alerts,
definition lists, YAML and TOML frontmatter, responsive Pandoc columns,
rendered and source views, copy buttons, and links to the original Markdown.
Frontmatter appears collapsed at the top. Its string `title` sets the page title
unless `--title` is given. Use `--no-source` if the original should not be
uploaded.

By default, Markdown references such as `#123`, `owner/repo#456`, and full
commit IDs link against a locally discovered GitHub `origin`. Use `--repo none`
to disable this or `--repo https://github.example/owner/repo` to supply explicit
GitHub Enterprise-compatible context. Discovery is local and does not contact
the remote.

Everything except the optional Mermaid runtime is embedded in the HTML. Use
`--no-external-assets` to keep Airplan-managed features offline, or
`--mermaid-url` to select another HTTPS CDN or self-hosted module. This setting
does not block external content authored in trusted Markdown, HTML, or custom
templates.

Multi-page documents add a Pages rail, active-page indicator, previous and next
links, and a separate On this page rail. Each page remains a standalone HTML
document with ordinary browser navigation.

Collection overview pages render images inline, video and audio with controls,
and arbitrary files as linked cards. Members have Open, Download, and Copy URL
actions. Media never autoplays, images lazy-load, and links work without
JavaScript.

## Automation and agents

For upload commands, stdout is safe to capture:

- A successful document prints its page URL and nothing else.
- A successful collection prints ordered direct URLs followed by its overview.
- With `--json`, stdout contains one JSON object instead.
- Logs, warnings, progress, and errors go to stderr.
- A non-zero exit means no upload URL was printed.

```sh
url=$(airplan plan.md)
url=$(airplan --json plan.md | jq -r .url)
overview=$(airplan --json screenshot.png demo.webm | jq -r .url)
image=$(airplan --json screenshot.png | jq -r '.files[0].url')
```

Do not invent or reuse a URL after a failed command.

### Install the agent skill

Install and configure the Airplan CLI on the machine where the agent runs, then
install the [Airplan agent skill](skills/airplan/SKILL.md). The skill teaches
compatible coding agents to upload requested documents and file sets, including
visual evidence from authorized pull-request or issue work.

Install it globally with the [Skills CLI](https://skills.sh/):

```sh
npx skills add jimeh/airplan --skill airplan --global
```

The binary also contains the matching skill. An agent with the CLI can inspect
or install that copy directly:

```sh
airplan skill

mkdir -p ~/.agents/skills/airplan
airplan skill > ~/.agents/skills/airplan/SKILL.md
```

Once configured, ask the agent to share a plan, screenshot, or recording as a
link.

### Connect through MCP

For a local agent, add `airplan mcp` as a stdio server. It follows the selected
`s3` or `airplan` profile exactly like the CLI. Remote agents can connect to a
running Airplan server at `https://airplan.example.com/mcp` with an
`Authorization: Bearer <token>` header.

MCP exposes document and collection uploads, revisions, listing, inspection,
deletion, synchronization, two-phase purge, and preview-first upgrades. Hosted
MCP does not accept server-local paths. See the [server guide](docs/server.md)
for the transport and trust boundaries.

## Configuration

Airplan resolves settings from command-line flags, `AIRPLAN_*` environment
variables, a named profile, root-level config values, then built-in defaults.
Use profiles when several buckets or an Airplan server share one config file:

```toml
backend = "s3"
endpoint = "https://<account-id>.r2.cloudflarestorage.com"
region = "auto"
default_profile = "work"

[profiles.work]
bucket = "work-plans"
public_base_url = "https://plans.work.example.com"

[profiles.shared]
backend = "airplan"
api_url = "https://airplan.example.com"
api_token = "..."
```

Select a profile with `--profile NAME`, `-p NAME`, or `AIRPLAN_PROFILE`.
`airplan config profiles` lists configured profiles, `airplan config show`
explains the resolved values, and `airplan config schema` prints the complete
JSON Schema.

The built-in appearance control keeps System, Light, and Dark mode separate
from the theme assigned to each mode. Airplan includes GitHub, Catppuccin, Rose
Pine, Solarized, Tokyo Night, and One Dark themes. You can limit that catalog or
define custom themes in the config file.

The [configuration guide](docs/configuration.md) documents every configuration
source, backend rule, theme setting, environment variable, and shell completion
command.

## Privacy model

Airplan links are capability URLs. Every path contains 128 random bits. By
default, generated pages include a `noindex` directive. `--indexable` or
`indexable = true` removes that directive. The link is effectively private
while it remains unknown, but it is not access-controlled:

- Anyone with the link can open it and pass it on.
- Chat tools may scan or prefetch links shared through them.
- Objects remain in the bucket until they are deleted. Airplan serves them with
  `Cache-Control: no-store` so browsers and shared caches should not retain a
  reusable response after deletion.
- Bucket listing must remain private.

Review screenshots and recordings for tokens, usernames, private messages,
browser chrome, and unrelated desktop content before uploading. Filenames are
public in direct URLs and collection overviews.

Airplan does not sanitize authored HTML or byte-exact assets. HTML, SVG, and
JavaScript may execute when opened or embedded. Every nested page and asset URL
shares the entry's capability boundary and persists until the owning document
is deleted.

Use `airplan purge --older-than 30d --yes` manually or from cron when uploads
should expire. Protect an upload that must survive bulk cleanup:

```sh
airplan protect --reason "linked from project README" <url-or-key>
```

Protection is enforced by compatible Airplan clients, not by storage. Older
Airplan builds and native storage tools can delete protected uploads.

## Go library

The CLI is a thin shell over the importable
[`github.com/jimeh/airplan/airplan`](https://pkg.go.dev/github.com/jimeh/airplan/airplan)
package. With `ctx` and an open `io.Reader` named `file`:

```go
cfg, err := airplan.LoadConfig(airplan.ConfigOptions{})
if err != nil {
    return err
}

client, err := airplan.New(ctx, cfg)
if err != nil {
    return err
}

result, err := client.Upload(ctx, airplan.Input{
    Reader: file,
    Name:   "plan.md",
})
if err != nil {
    return err
}

fmt.Println(result.URL)
```

Use `UploadDocument` for linked pages and assets, `UploadFiles` for
collections, and `CreateDocumentRevision` for replacement revisions.
`RenderDocument` renders a bundle without storage. See [Use Airplan as a Go
library](docs/go-library.md) for examples and API boundaries.

## Development

The project uses [mise](https://mise.jdx.dev/) for its task commands:

```sh
mise run treeboot           # bootstrap a new linked worktree
mise run setup              # install tools and Git hooks
mise run check              # lint, generated files, format, and unit tests
mise run test-integration   # MinIO round trip; requires Docker
mise run test:browser       # Chromium page smoke tests
mise run audit:deps         # audit Go modules and Bun dependencies
mise run release:snapshot   # build release artifacts without publishing
mise run verify             # broad local validation
```

Run `mise run check` before handing work off. Use `mise run verify` for broad or
risky changes. [AGENTS.md](AGENTS.md) contains the repository map, full task
list, testing guidance, and contribution constraints. [IMPLEMENTATION.md](IMPLEMENTATION.md)
describes the architecture.

Releases use release-please and GoReleaser. Pull-request titles follow
Conventional Commits because they determine release versions and changelog
entries.

## License

MIT. See [LICENSE](LICENSE).
