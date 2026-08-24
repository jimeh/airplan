<div align="center">

<img height="196px" width="196px" src="./img/airplan.svg" alt="Logo">

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

It is especially useful with coding agents: an agent can turn a local plan into
a link you can open from any device. That makes reviewing plans from a mobile
app practical even when the agent is running elsewhere and its local files are
hard to reach.

It also works whenever you want to share a local document without running a
server or using a paste service.

Document bundles keep a primary Markdown narrative together with supporting
Markdown or source-code pages and byte-exact assets. Airplan renders each page,
rewrites declared page links, and adds ordinary browser navigation among them.
Collections remain the right fit for peer screenshots, browser recordings,
PDFs, archives, and other artifacts with no primary narrative. Both shapes are
one cleanup unit.

- Markdown becomes a polished page with eleven built-in themes, independent
  light/dark mode slots, and validated custom themes.
  Authored HTML and link destinations are preserved, so treat it as trusted
  content.
- Source and plain-text files become highlighted, gist-like pages.
- Multiple files containing Markdown or HTML become one document automatically;
  the entry URL stays the command result.
- HTML stays HTML, with no rendering step. Treat HTML input as trusted code: it
  may execute scripts when someone opens the link.
- Images, video, and audio render on a responsive collection overview. Generic
  files remain available through direct open and download links.
- Files live in a bucket you own. Use S3 directly or run one single-user
  Airplan server that keeps storage credentials off client machines.
- The command has a predictable output contract for scripts and agents.

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
  is a document bundle with linked Markdown and Go pages, ordinary page
  navigation, and an SVG request-flow asset.
- [Release verification evidence][airplan-demo-collection]
  is a file collection with an image preview and direct links to every
  original artifact.

[airplan-demo-implementation-plan]: https://demo.airplan.dev/drhsb6uca5q5wflmhh3lnkvjty/implementation-plan.html
[airplan-demo-how-it-works]: https://demo.airplan.dev/k47u2jqntlardyxoycfxur5fcm/how-airplan-works.html
[airplan-demo-go-api]: https://demo.airplan.dev/zdmnzvdlgsy5si7vmogewrnoki/upload-example.html
[airplan-demo-document-bundle]: https://demo.airplan.dev/uhjsfkgu7lmj32lzrvyas7rnoq/implementation-plan.html
[airplan-demo-collection]: https://demo.airplan.dev/a63h4b7hohllev7auqxg6rhcee/index.html

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

The official server container is available for `linux/amd64` and
`linux/arm64`:

```sh
docker pull ghcr.io/jimeh/airplan:latest
```

Use an exact unprefixed version or digest for reproducible deployments.

### Install the agent skill

This repository includes an [airplan agent skill](skills/airplan/SKILL.md) for
compatible coding agents. It teaches an agent to upload a requested document or
file set, including visual evidence captured during authorized pull-request or
issue work, then return direct and overview links.

Install it globally with the [Skills CLI](https://skills.sh/):

```sh
npx skills add jimeh/airplan --skill airplan --global
```

The installed binary also contains the canonical skill. An agent that only has
the CLI can read it directly with:

```sh
airplan skill
```

Or install that exact embedded copy without checking out the repository:

```sh
mkdir -p ~/.agents/skills/airplan
airplan skill > ~/.agents/skills/airplan/SKILL.md
```

The `airplan` CLI must also be installed and configured on the machine where
the agent runs. Once it is, ask the agent to share a plan, screenshot, or
recording as a link and open the result from any browser.

Release assets include separate SPDX JSON SBOMs, and the archives are covered by
GitHub artifact attestations. After downloading the release assets, verify them:

<!-- x-release-please-start-version -->

```sh
# Linux
sha256sum --ignore-missing --check checksums.txt
# macOS
shasum --ignore-missing --algorithm 256 --check checksums.txt

gh release verify v0.11.1 --repo jimeh/airplan

gh attestation verify airplan_0.11.1_darwin_arm64.tar.gz \
  --repo jimeh/airplan

docker pull ghcr.io/jimeh/airplan:0.11.1
gh attestation verify oci://ghcr.io/jimeh/airplan:0.11.1 \
  --repo jimeh/airplan \
  --signer-workflow jimeh/airplan/.github/workflows/release.yml
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
Object-read access powers `show`, `get`, remote purge inspection, and `sync`.

Create `~/.config/airplan/config.toml`:

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
# repo = "auto"           # infer GitHub origin for Markdown links
# collection_template = "~/.config/airplan/collection.html"
```

Explicit access and secret keys must be configured as a pair. Omit both to use
the standard AWS credential chain. Endpoint and public base URLs must be
absolute HTTP(S) URLs.

If the file contains credentials, protect it with `chmod 600`. airplan warns
when its permissions are too broad. The `#:schema` comment enables validation
and completion in editors with [Taplo](https://taplo.tamasfe.dev/) or the Even
Better TOML extension.

Run `airplan config schema` to inspect every available file setting. See the
[configuration reference](#configuration-reference) for profiles,
environment-only setup, precedence, and diagnostic commands.

### Run a single-user Airplan server

The default `s3` backend keeps today's self-contained behavior: the CLI owns
S3 credentials and calls storage directly. To configure S3 once for clients on
other machines, run the built-in REST and MCP server:

```sh
openssl rand -base64 32 > /run/secrets/airplan-token
chmod 600 /run/secrets/airplan-token

airplan --profile storage \
  --manifest /var/lib/airplan/manifest.jsonl \
  serve --token-file /run/secrets/airplan-token
```

`serve` defaults to `127.0.0.1:8080` and requires an `s3` profile. It checks
storage before listening and exposes:

- the authenticated REST API under `/api/v1`;
- authenticated MCP Streamable HTTP at `/mcp`;
- unauthenticated liveness at `/healthz`; and
- the authoritative OpenAPI 3.0.3 schema at `/openapi.yaml`.

REST document uploads stream one metadata part, one entry part, then repeated
page and asset parts. `POST /api/v1/uploads/documents/revisions` is the canonical
revision route; `/api/v1/uploads/documents/update` remains a deprecated
compatibility route to the same operation. Clients negotiate document-bundle
and canonical-route support through `/api/v1/capabilities` before streaming, so
they never replay a partially sent body after probing an unsupported route.

The default `info` log level keeps stderr quiet apart from the listening line
and server failures. `warn` and `error` suppress the listening line. Use
`--log-level debug` to diagnose request completion,
safe authentication rejection reasons, Origin and size-limit failures, and MCP
tool outcomes. `--log-level trace` additionally shows sanitized request, MCP
method, and SDK lifecycle events:

```sh
airplan --profile storage serve \
  --token-file /run/secrets/airplan-token \
  --log-level debug
```

`AIRPLAN_SERVER_LOG_LEVEL` is the environment fallback; an explicit flag wins.
Logs never include Authorization values, request or MCP bodies, tool arguments
or results, uploaded content, capability URLs, storage identity, credentials,
or filesystem paths.

Configure a client profile with only the server URL and bearer token:

```toml
[profiles.shared]
backend   = "airplan"
api_url   = "https://airplan.example.com"
api_token = "..." # or AIRPLAN_API_TOKEN
```

Normal commands then use the server without local S3 credentials:

```sh
airplan --profile shared plan.md
airplan --profile shared list
airplan --profile shared list --remote
airplan --profile shared sync
airplan --profile shared purge --older-than 30d
```

Terminate TLS at a trusted reverse proxy. HTTPS is required for non-loopback
client URLs, and a non-loopback listen address requires
`--allow-non-loopback`. Configure proxy body limits, buffering, and timeouts
for large streaming uploads and downloads. Airplan v1 server authentication is one
static bearer token: there are no accounts, roles, OAuth flow, or token
issuance.

Run only one server process for a manifest. The file must be on persistent
storage. A same-user local CLI and `serve` share the normal platform manifest
by default, so `airplan --profile storage list` on the server machine includes
uploads received through the API. Containers and services usually run with a
different state directory; mount a persistent directory and pass the same
explicit `--manifest` path when shared local history is desired. The server
API scopes manifest results to its configured profile, bucket, and key prefix
and does not expose unrelated records from a shared local file.

#### Run the server container

The container defaults to `airplan serve`, listens on `0.0.0.0:8080`, runs as
UID/GID `65532:65532`, and stores its manifest at
`/var/lib/airplan/manifest.jsonl`. Always give that directory a named volume:

```sh
docker volume create airplan-data

docker run --detach --name airplan \
  --publish 127.0.0.1:8080:8080 \
  --env-file ./airplan.env \
  --mount type=volume,source=airplan-data,target=/var/lib/airplan \
  --mount type=bind,source="$PWD/airplan-token",\
target=/run/secrets/airplan-token,readonly \
  --env AIRPLAN_SERVER_TOKEN_FILE=/run/secrets/airplan-token \
  ghcr.io/jimeh/airplan:<version>
```

`airplan.env` may contain the environment-only storage configuration:

```dotenv
AIRPLAN_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
AIRPLAN_BUCKET=plans
AIRPLAN_REGION=auto
AIRPLAN_ACCESS_KEY_ID=...
AIRPLAN_SECRET_ACCESS_KEY=...
AIRPLAN_PUBLIC_BASE_URL=https://plans.example.com
AIRPLAN_REPO=none
```

Protect both files from other users. The mode-0600 token file must be owned by
UID/GID `65532:65532` so the container process can read it. Prefer a runtime
secret manager over a plaintext environment file in production.

For file-based configuration, omit the storage variables and mount the file at
the optional default path:

```sh
docker run --detach --name airplan \
  --publish 127.0.0.1:8080:8080 \
  --mount type=volume,source=airplan-data,target=/var/lib/airplan \
  --mount type=bind,source="$PWD/config.toml",\
target=/etc/airplan/config.toml,readonly \
  --mount type=bind,source="$PWD/airplan-token",\
target=/run/secrets/airplan-token,readonly \
  --env AIRPLAN_PROFILE=storage \
  --env AIRPLAN_SERVER_TOKEN_FILE=/run/secrets/airplan-token \
  ghcr.io/jimeh/airplan:<version>
```

The mounted config is discovered without setting `AIRPLAN_CONFIG`; setting
that variable explicitly still requires the selected file to exist. A minimal
Compose deployment is:

```yaml
services:
  airplan:
    image: ghcr.io/jimeh/airplan:<version>
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      AIRPLAN_PROFILE: storage
      AIRPLAN_SERVER_TOKEN_FILE: /run/secrets/airplan-token
    volumes:
      - airplan-data:/var/lib/airplan
      - ./config.toml:/etc/airplan/config.toml:ro
      - ./airplan-token:/run/secrets/airplan-token:ro

volumes:
  airplan-data:
```

Run one container against the volume. Back it up if upload history matters.
`airplan sync` can reconstruct currently discoverable remote uploads after
manifest loss, but may not recover all historical or local metadata; the image
does not sync automatically at startup. Anonymous volumes can become orphaned
when a container is replaced. For state bind mounts, make the host directory
writable by UID/GID `65532:65532`; mode-0600 config and token bind mounts must
be owned by the same IDs.

The image declares `EXPOSE 8080`, which is metadata rather than port
publication. If `AIRPLAN_SERVER_PORT` changes, update the Docker port mapping,
reverse-proxy target, and external `/healthz` probe. The image contains no
shell, probe utility, credentials, config, or token, and has no in-image
healthcheck. Terminate TLS at a trusted reverse proxy.

`latest` is mutable. The publication workflow serializes its own GHCR
mutations and refuses an exact release tag whose observed digest conflicts.
GHCR does not enforce tag immutability against external writers, so a digest is
the registry-level immutable reference:

```sh
docker pull ghcr.io/jimeh/airplan@sha256:<image-index-digest>
gh attestation verify \
  oci://ghcr.io/jimeh/airplan@sha256:<image-index-digest> \
  --repo jimeh/airplan \
  --signer-workflow jimeh/airplan/.github/workflows/release.yml
```

Container server precedence is explicit flag, then environment, then the
native default:

| Concern                | Flag                   | Environment fallback                         |
| ---------------------- | ---------------------- | -------------------------------------------- |
| Listen address         | `--listen`             | `AIRPLAN_SERVER_HOST`, `AIRPLAN_SERVER_PORT` |
| Non-loopback consent   | `--allow-non-loopback` | `AIRPLAN_SERVER_ALLOW_NON_LOOPBACK`          |
| Token file             | `--token-file`         | `AIRPLAN_SERVER_TOKEN_FILE`                  |
| Hosted MCP origins     | `--allowed-origin`     | `AIRPLAN_SERVER_ALLOWED_ORIGINS`             |
| Upload spool directory | `--temp-dir`           | `AIRPLAN_SERVER_TEMP_DIR`                    |
| Log level              | `--log-level`          | `AIRPLAN_SERVER_LOG_LEVEL`                   |

`AIRPLAN_SERVER_TOKEN` remains the alternative token-value source. Do not set
it together with a resolved token file. The image sets host `0.0.0.0`, port
`8080`, and non-loopback consent `true`; native executions retain the safe
`127.0.0.1:8080` and `false` defaults. The origins environment value is a
comma-separated list. Server ports are decimal values from 0 through 65535,
and server booleans accept exactly `true` or `false`.

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

## Share documents and files

Pass airplan a file and use the URL it prints:

```sh
airplan plan.md                   # Markdown → rendered page
airplan report.html               # HTML → uploaded page
airplan pkg/server/handler.go     # source → highlighted page
airplan --open plan.md            # upload and open in a browser
airplan --json plan.md            # structured result for scripts
```

Pass a primary Markdown document with its related files to create a document
bundle:

```sh
airplan README.md docs/design.md examples/server.go \
  images/flow.svg recordings/demo.mp4
```

Airplan prefers a root `README.md`, then `index.md`, then the first Markdown
file as the entry. `--entrypoint PATH` overrides that choice. Other Markdown
and UTF-8 source files become managed pages; HTML, media, and opaque resources
become byte-exact assets. Use `--page` or `--asset` to override an ambiguous
file's inferred role. The entry file's directory is the bundle root. Every path
must remain beneath it, including after resolving symlinks. Airplan preserves safe nested
paths, renders Markdown pages as sibling `.html` files, appends `.html` to
source-code page names, and rewrites links only when they exactly name a
declared managed source. Assets are uploaded unchanged and do not appear in the
Pages navigation. The plain result remains the entry URL. JSON adds ordered
`.pages` and `.assets`; existing top-level fields still describe the entry.

Bundles accept at most 100 total items. Defaults are 10 MiB per managed source,
100 MiB across managed sources, 100 MiB of generated HTML, 1 GiB per asset, and
2 GiB across assets. Adjust the client-side limits with `--max-size`,
`--max-total-page-size`, `--max-asset-size`, and `--max-total-size`. A server may
advertise lower limits.

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
`--collection` when one text-like input should be uploaded unchanged, or when a
Markdown or HTML file set should remain a collection. `--files` remains a
compatibility alias.

```sh
airplan screenshot.png
airplan --collection README.md
airplan --json screenshot.png recording.webm
```

Collection JSON retains `.url` for the overview and puts the ordered direct
links in `.files[].url`. The original file bytes are unchanged. Duplicate
basenames, directories, and reserved names are rejected before upload.

Standard input works too. It defaults to Markdown when no format can be
inferred:

```sh
cat plan.md | airplan --slug my-plan -
cat main.go | airplan --format txt --lang go -
```

### Upgrade existing Markdown pages

Airplan can re-render an existing source-backed Markdown upload with the
current page capabilities while keeping its original URLs, source bytes, and
assets:

```sh
airplan upgrade --check https://plans.example.com/vq3n.../plan.html
airplan upgrade https://plans.example.com/vq3n.../plan.html
airplan upgrade --force --template ./new-page.tmpl https://plans.example.com/vq3n.../plan.html
airplan upgrade --all --dry-run
airplan upgrade --all --yes
airplan upgrade --all --all-profiles --yes
```

For a bundle, upgrade re-renders every managed page. It writes the entry last
and uses stored SHA-256 digests to skip pages already in the desired state. A
failed attempt may temporarily leave old and new renderer generations at
different page URLs; inspection reports the mismatch and a retry repairs only
the remaining pages.

Single-target and bulk upgrades are planned against live ownership markers and
replanned immediately before apply; only a still-upgradeable target whose
inspected marker/page ETags match can be written. Bulk apply
prompts interactively and requires `--yes` in non-interactive use. A regular
standalone upload has no `.airplan-versions.json`; upgrading it does not create
one. Built-in Markdown pages merely carry dormant, cache-busted discovery so a
future revision 2 can add history without replacing the original page URL.
Template mismatches are refused unless `--force` explicitly replaces the
stored recipe; pass `--template PATH` for a custom replacement or
`--template=` to return to the built-in template. Version 5 and 6 pages also
record their theme catalog and defaults. After changing theme configuration, run
`airplan upgrade --force <url|key>` before creating that revision;
otherwise Airplan fails closed rather than re-rendering it under a different
theme recipe implicitly.

`--all-profiles` inventories configured profiles even when none is selected by
default and ignores `AIRPLAN_PROFILE` for that inventory; every participating
profile must use direct S3. It cannot be combined with an explicit `--profile`.
With `--json`, dry-run returns a plan while apply always returns a result object,
including for empty or current-only plans.

### Revise an existing Markdown document

Upload a complete new revision while keeping every previously shared page as
an immutable view of its original source:

```sh
airplan new-revision --json https://plans.example.com/vq3n.../plan.html plan.md

airplan new-revision https://plans.example.com/vq3n.../readme.html \
  README.md docs/design.md images/flow.svg
```

`new-revision` is the canonical name. `update` remains a warning-free
compatibility alias with the same flags and behavior.

The first real update promotes the standalone upload to revision 1 and creates
revision 2 under a new capability URL. Only then is
`.airplan-versions.json` added to both uploads; ordinary standalone uploads
still have no versions object. Later updates may target any member and resolve
the latest revision before appending. Only surviving revisions are navigation
and update targets; deleted entries remain as numbered tombstones and are never
reused. Each revision after 1 owns a deterministic `.airplan-changes.diff`,
rendered as a highlighted Changes view when small enough and always available
through its raw diff link. Anyone with one chain URL can navigate to every
surviving linked revision URL.
The JSON result includes `previous_url`, `diff_url`, and `unchanged`; revision
numbers are omitted only for an unchanged standalone document that has not yet
formed a chain.

A bundle revision is complete replacement. Resupply every page and asset that
should remain; omission removes it from the new revision. Page metadata,
ordering, source bytes, and asset metadata or bytes participate in no-op
detection. The combined Changes view contains per-path text diffs and concise
binary asset summaries without embedding binary data.

### Preview without uploading

`preview` uses the same renderer locally. It does not need storage credentials,
contact S3, or update the upload history.

```sh
airplan preview plan.md > plan.html
airplan preview -o plan.html plan.md
airplan preview README.md docs/design.md images/flow.svg \
  --output-dir ./preview
airplan preview --collection screenshot.png demo.webm -o index.html
```

A preview with pages or assets requires `--output-dir`. Airplan stages the
complete rendered-page, included-source, and asset tree beside the destination,
then renames it into place. It refuses an existing directory so a failed
preview cannot look complete. The output contains no ownership marker or other
remote-management metadata.

Use `--collection-template custom.html` to replace the collection overview
without affecting document templates. `airplan template collection` prints the
built-in starting point; `airplan template` continues to print the document
template. Collection preview does not copy large members, so save the overview
beside staged input files when its local media links need to work.

### Manage uploads

```sh
airplan list                     # uploads known to the local manifest
airplan ls -r                    # airplan uploads currently in the bucket
airplan show <url-or-key>         # validate and inspect one remote upload
airplan get [--source|--diff|--page PATH|--asset PATH] <url-or-key>
airplan delete <url-or-key>      # delete one upload
airplan list --newer-than 7d     # only recent uploads (also --kind, --slug)
airplan list --protected         # only purge-protected uploads
airplan list --all-profiles      # uploads recorded under every profile
airplan protect <url-or-key>     # mark one upload purge-protected
airplan unprotect <url-or-key>   # remove purge protection
airplan purge --older-than 30d   # review and delete older uploads
airplan purge --all --include-versioned  # explicitly include revision history
airplan sync                     # reconcile remote uploads into local history
```

Each successful upload is recorded in
`~/.local/state/airplan/manifest.jsonl`. Local commands use that history by
default. Override it globally with `--manifest PATH` or `AIRPLAN_MANIFEST`;
relative paths resolve from the current working directory. The option applies
to direct S3 commands, `serve`, and stdio MCP using S3. An HTTP client rejects
an explicit `--manifest` because only the server may choose its filesystem
path, and ignores `AIRPLAN_MANIFEST`.

`ls` aliases `list`, and `-r` aliases `--remote` for `list` and
`purge`. Local `list` filters history to the resolved active profile by default.
An explicit `--profile NAME` selects another recorded profile, while
`--profile=` selects root-level history. `-A`/`--all-profiles` lists every
recorded profile and works even when local configuration cannot resolve one
active profile. A config can also select an `airplan` profile, in which case
list reads the server manifest and `--config` is useful without `--remote`;
`--all-profiles` is rejected because that backend exposes only its
server-scoped manifest.

`list` selects rows with `--newer-than`, `--older-than`, `--limit`, `--kind`,
`--slug`, `--protected`, and `--no-protected`, in both listing modes and in
`--json`, because filters are selection rather than presentation. The two
protection flags are mutually exclusive and select protected or unprotected
uploads respectively. The two time flags take either an age such
as `7d`, `2w`, or `36h`, or an absolute date such as `2026-07-01`,
`2026-07-01 09:30`, or a strict RFC 3339 timestamp; bare dates mean local midnight
while the manifest keeps recording UTC. A slash date that does not lead with a
four-digit year, such as `03/04/2026`, is refused rather than guessed, because
`purge --older-than` reads the same values. `--newer-than` includes uploads
recorded exactly at the boundary and `--older-than` excludes them, and
`--limit N` keeps the N most recent matches while still printing them oldest
first. Explicit empty filter values are errors. Purge also requires its
resolved `--older-than` boundary to lie strictly in the past; use `--all` to
request deletion of everything.

Table shape is controlled separately: `--columns date,title,url` selects an
exact set, `--columns +dir,-title` adjusts the default one, `--wide` prints
every column a mode offers, and `--reverse` prints newest first. `PROFILE`,
`STATE`, and `DIRECTORY` appear automatically only when relevant — more than
one profile, legacy or protected history (or an active protection filter), or
a row with no URL. Local `STATE` values are `managed`, `legacy`, `protected`,
or `legacy+protected`; remote values are `protected` or `unprotected`. Local
wide output includes `AIRPLAN` and `RENDERER` provenance when known. Column
flags are table-only and are rejected with `--json`, while `--reverse` reorders
both.

`--remote` reads storage through the selected backend instead, so it can find
uploads from other machines.
Remote discovery recognizes document `.airplan.json` and collection
`.airplan-collection.json` ownership markers with one bucket listing; it does
not fetch each marker. The marker name supplies an untrusted kind hint, letting
collection rows select exact `index.html` even beside other HTML files. A
directory containing both names is a conflict with no inferred URL and cannot
be managed through Airplan. Markerless directories are invisible. Use
`airplan show` for validated kind, declared files, and completeness.

`preview` and `get` accept `-o` as shorthand for `--output`. When `show`, `get`,
or `delete` uniquely matches marker-managed local history, its recorded profile
is selected automatically unless `--profile` or `AIRPLAN_PROFILE` is explicit.
The remote ownership marker is still authoritative.

For a document directory or marker target, `get` returns the entry by default.
Use `--page PATH` or `--asset PATH` to select a logical child;
`--page PATH --source` returns that page's source. A direct declared child URL
still returns that exact object and cannot be combined with a selector. Direct
child targets work for inspection and lifecycle commands, but delete, protect,
unprotect, purge, sync, and upgrade always act on the complete owning document.

`airplan sync` imports complete marker-managed uploads made from other machines
into the receiving machine's manifest. It also tombstones local records whose
markers are confirmed absent remotely; `--no-prune` makes the operation
additive-only and `--dry-run` previews it. Sync verifies apparent absences with
a targeted request instead of trusting a bucket listing alone. It also completes
older local records that predate the recorded `objects` and `total_bytes`
totals, appending an enriched copy of each one with its original time and
identity; those are reported separately from imports and never resurrect a
deleted upload. Marker v3 through v6, plus v2-without-source records, can supply
exact totals; v1 and v2-with-source records stay absent without recurring marker fetches.
Enrichment fetch or marker problems are warnings deferred to a later sync, not
sync failures. `--concurrency`
controls concurrent marker requests (default 8, range 1-64). It converges the
active remote inventory, not the historical JSONL event stream; deletion
history is not uploaded.

Every new upload uses ownership marker version 6 with one declared-object model
for document pages, sources, assets, diffs, and collection files. Every declared
payload has a provider-independent SHA-256 digest, and object paths may preserve
validated nested document structure. Documents require a slug and entrypoint;
collections have no slug and always use `index.html`. Current Airplan releases
manage marker versions 1 through 6. Version 4 introduced producer and renderer
provenance, revisions, and page SHA-256; version 5 added theme identity; version
6 adds document page descriptors, assets, nested paths, and digests for every
payload. Older clients fail closed and must be upgraded before they can manage
v6 uploads.
Repository metadata is stored remotely for every input mode when `--repo`
supplies or discovers a repository.

## Pages airplan creates

Markdown pages include per-theme syntax highlighting, theme-derived Mermaid
diagrams from exact
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

Multi-page documents add an ordered Pages rail, an active-page indicator,
previous and next links, and a separate On this page rail. Narrow layouts keep
a normal no-JavaScript page list and add an accessible dialog after the page
runtime initializes. Every link remains an ordinary link to a standalone HTML
document. In same-origin browsers that support cross-document View Transitions,
the built-in template uses a short native transition unless the reader requests
reduced motion. Other browsers perform the same normal navigation. Each page
loads fresh and initializes its own scripts; Airplan does not implement a
client-side router, page cache, or custom history.

Collection overview pages render images inline, video and audio with controls,
and arbitrary files as linked cards. Every member has Open, Download, and Copy
URL actions, and the page can copy its own overview URL. Media never autoplays;
images lazy-load; links remain usable without JavaScript. The page is
self-contained, responsive, uses the same configurable theme catalog, and is
noindexed by default.

Collections accept at most 100 files. Defaults are 1 GiB per member and 2 GiB
total; use `--max-size` and `--max-total-size` to adjust them per invocation,
with `0` meaning unlimited. Increase `--timeout` explicitly for a substantial
recording. Members stream from disk and downloads stream to their destination
rather than buffering whole recordings in memory.

## Automation and agents

For upload invocations, the command-line contract is intentionally simple:

- A successful document prints its page URL and nothing else.
- A successful collection prints ordered direct URLs followed by its overview.
- With `--json`, stdout contains one JSON object instead.
- Logs, warnings, progress, and errors go to stderr.
- A non-zero exit means no upload URL was produced.

That makes direct capture safe:

```sh
url=$(airplan plan.md)
url=$(airplan --json plan.md | jq -r .url)
overview=$(airplan --json screenshot.png demo.webm | jq -r .url)
image=$(airplan --json screenshot.png | jq -r '.files[0].url')
```

Do not invent or reuse a URL after a failed command. For the complete CLI,
config, key, and manifest contracts, use [SPEC.md](SPEC.md).

The same operation set is available to MCP clients. For a local agent, add
`airplan mcp` as a stdio server; it follows the selected `s3` or `airplan`
profile exactly like the CLI. For remote agents, connect to the server's
Streamable HTTP endpoint at `https://airplan.example.com/mcp` and configure
`Authorization: Bearer <token>`. Clients that cannot attach a custom
Authorization header are not supported by the initial single-user server.

The MCP server exposes upload, revision, list, inspect, delete, sync, two-phase
purge, and preview-by-default document upgrade tools. `upload_document` and the
canonical `new_document_revision` accept inline UTF-8 pages and base64 assets;
`update_document` remains an identical compatibility tool. Inline assets have a
32 MiB decoded aggregate limit in addition to the hosted request-body limit.
Local stdio adds `upload_paths` for the same automatic classification,
`upload_document_files` and `new_document_revision_files` for document paths,
and explicit collection `upload_files`. Hosted MCP never accepts server-local
paths and omits all four local-file tools. `upgrade_documents` applies only exact
items returned by its preview. Template dumping, config inspection, arbitrary
object access, and filesystem browsing are not exposed.

## Configuration reference

airplan resolves settings in this order, from highest to lowest priority:

1. Command-line flags
2. `AIRPLAN_*` environment variables
3. The selected named profile
4. Root-level config file values
5. Built-in defaults

The default config path is `$XDG_CONFIG_HOME/airplan/config.toml`, normally
`~/.config/airplan/config.toml` on Linux and macOS, with the corresponding
platform config directory used on Windows. Select another file with
`--config PATH` or `AIRPLAN_CONFIG`. An explicitly selected file must exist;
only the platform-default file is optional.

### Config files and profiles

Root-level values are shared defaults. Named profiles inherit them and
override only what differs:

```toml
# Root-level keys must appear before the first [profiles.*] table.
backend = "s3"
endpoint = "https://<account-id>.r2.cloudflarestorage.com"
region = "auto"
key_prefix = "jimeh"
default_profile = "work"
light_theme = "github-light"
dark_theme = "github-dark"

[profiles.work]
bucket = "work-plans"
public_base_url = "https://plans.work.example.com"
light_theme = "solarized-light"
dark_theme = "tokyo-night"

[profiles.personal]
bucket = "personal-plans"
public_base_url = "https://plans.example.com"

[profiles.shared]
backend = "airplan"
api_url = "https://airplan.example.com"
api_token = "..."
```

`backend` defaults to `s3`, so existing configuration is unchanged. An
`airplan` profile needs only `api_url` and `api_token`; it does not load the
AWS credential chain. S3 values inherited from a root configuration are
inactive for that profile. Conversely, API settings are inactive for an `s3`
profile. Hosted rendering uses the server's theme configuration: explicit
client `light_theme`, `dark_theme`, `theme`, `available_themes`, and custom
`themes` definitions are not transmitted. Custom definitions are still
validated strictly at client config load; valid definitions and explicit theme
selectors produce inactive-field warnings.

Select a profile for one command with `--profile` / `-p`, or for profile-aware
commands in a project-specific shell environment with `AIRPLAN_PROFILE`:

```sh
# For example, in a project shell setup, task runner, or .envrc:
export AIRPLAN_PROFILE="work"
```

airplan does not load `.env` files itself; the variable must be exported by
your shell or another environment manager. An explicitly selected profile must
exist in the config file. Without an explicit selection, airplan uses
`default_profile`, the only named profile, or complete root-level values, in
that order.

For a shared bucket, give each person a distinct `key_prefix`. The prefix also
scopes remote list and purge operations.

### Environment variables

A config file is not required. You can provide a complete backend setup
through environment variables instead. For direct S3:

```sh
export AIRPLAN_ENDPOINT="https://<account-id>.r2.cloudflarestorage.com"
export AIRPLAN_BUCKET="plans"
export AIRPLAN_REGION="auto"
export AIRPLAN_PUBLIC_BASE_URL="https://plans.example.com"
export AIRPLAN_ACCESS_KEY_ID="..."
export AIRPLAN_SECRET_ACCESS_KEY="..."

airplan plan.md
```

For a server-backed client:

```sh
export AIRPLAN_BACKEND="airplan"
export AIRPLAN_API_URL="https://airplan.example.com"
export AIRPLAN_API_TOKEN="..."

airplan plan.md
```

Avoid committing credential values in project environment files. Explicit
access and secret keys must be set as a pair. If neither is set through
`AIRPLAN_*` variables or the config file, airplan uses the standard AWS
credential chain, including `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and
the shared credentials file.

| Variable                            | Purpose                                       |
| ----------------------------------- | --------------------------------------------- |
| `AIRPLAN_CONFIG`                    | Select an alternate config file               |
| `AIRPLAN_PROFILE`                   | Select a named `[profiles.*]` profile         |
| `AIRPLAN_BACKEND`                   | Select `s3` or `airplan`                      |
| `AIRPLAN_API_URL`                   | Set an Airplan server base URL                |
| `AIRPLAN_API_TOKEN`                 | Set its bearer token                          |
| `AIRPLAN_SERVER_HOST`               | Set the `serve` bind host                     |
| `AIRPLAN_SERVER_PORT`               | Set the decimal `serve` port                  |
| `AIRPLAN_SERVER_ALLOW_NON_LOOPBACK` | Acknowledge non-loopback serving              |
| `AIRPLAN_SERVER_TOKEN`              | Provide the static server token               |
| `AIRPLAN_SERVER_TOKEN_FILE`         | Read the static token from a file             |
| `AIRPLAN_SERVER_ALLOWED_ORIGINS`    | Set comma-separated hosted MCP origins        |
| `AIRPLAN_SERVER_TEMP_DIR`           | Set the upload spool directory                |
| `AIRPLAN_SERVER_LOG_LEVEL`          | Set `serve` logging from `error` to `trace`   |
| `AIRPLAN_MANIFEST`                  | Select a local S3/service manifest            |
| `AIRPLAN_ENDPOINT`                  | Set the S3-compatible API endpoint            |
| `AIRPLAN_BUCKET`                    | Set the destination bucket                    |
| `AIRPLAN_REGION`                    | Set the S3 signing region                     |
| `AIRPLAN_ACCESS_KEY_ID`             | Set the explicit access key ID                |
| `AIRPLAN_SECRET_ACCESS_KEY`         | Set the matching secret access key            |
| `AIRPLAN_PUBLIC_BASE_URL`           | Set the base URL used for public links        |
| `AIRPLAN_KEY_PREFIX`                | Prefix and scope uploaded object keys         |
| `AIRPLAN_TEMPLATE`                  | Select a custom HTML page template            |
| `AIRPLAN_COLLECTION_TEMPLATE`       | Select a collection overview template         |
| `AIRPLAN_LIGHT_THEME`               | Select the default light-mode slot theme      |
| `AIRPLAN_DARK_THEME`                | Select the default dark-mode slot theme       |
| `AIRPLAN_THEME`                     | Force one theme and omit theme controls       |
| `AIRPLAN_NO_EXTERNAL_ASSETS`        | Disable airplan-managed external loads        |
| `AIRPLAN_MERMAID_URL`               | Set an alternate HTTPS Mermaid module URL     |
| `AIRPLAN_REPO`                      | Set `auto`, `none`, or a repository URL       |
| `AIRPLAN_TIMEOUT`                   | Set a duration such as `30s`; `0` disables it |

`no_source` and `indexable` do not have environment variables. Configure them
in TOML or override them with `--no-source` and `--indexable`.

### Page themes

The appearance button keeps System/Light/Dark mode separate from the theme
assigned to each mode slot. Both theme selects show the full catalog in Light
themes and Dark themes groups, so either variant can be assigned to either
slot. Preferences persist per origin; JavaScript-disabled pages follow the
uploader defaults and the reader's system mode. Printing always uses GitHub
Light for the page, syntax highlighting, and Mermaid diagrams.

Limit the selectable catalog, preserving the listed order within each variant
group, with a root- or profile-level list:

```toml
available_themes = ["github-light", "tokyo-night-day", "tokyo-night"]
light_theme = "tokyo-night-day"
dark_theme = "tokyo-night"
```

Airplan adds a configured slot default missing from that list and warns on
stderr. An explicit profile list replaces the root list. To publish a fixed
theme with no appearance button, use `theme = "tokyo-night"` (or
`AIRPLAN_THEME`); it takes priority over the list and slot defaults, warning
when they are also explicitly configured.

Built-ins are GitHub Light/Dark, Catppuccin Latte/Mocha, Rose Pine Dawn/Main,
Solarized Light/Dark, Tokyo Night Day/Night, and One Dark. Custom themes are
global config entries available to every profile:

```toml
[themes.docs]
name = "Docs"
variant = "dark"
background = "#10131a"
foreground = "#e7eaf0"
muted = "#9aa3b2"
accent = "#7aa2f7"
accent_foreground = "#10131a"
border = "#3b4261"
surface = "#181c27"
surface_emphasis = "#242a3a"
info = "#7dcfff"
success = "#9ece6a"
important = "#bb9af7"
warning = "#e0af68"
danger = "#f7768e"
syntax = "derived" # or chroma:<registered-style-name>
```

Every token is required and colors use `#rrggbb` or `#rrggbbaa`. Built-in IDs
cannot be replaced, and unknown keys or selected IDs fail before upload unless
the selector is one of the slot/list values intentionally ignored by an active
forced `theme`.
Omitting `syntax` or setting it to an empty string is equivalent to `derived`.
See [built-in theme sources](THIRD_PARTY_THEMES.md) for pinned upstream palettes
and licenses. Custom page templates receive safe theme CSS/catalog fields,
conditional Mermaid palette data, and `.AppearanceEnabled`, but must opt into
all theme styling, controls, and runtime behavior themselves.

### Inspect configuration

```sh
airplan config profiles        # list named profiles and the explicit default
airplan config show            # show resolved values and their winning source
airplan config show --json     # return the same diagnostics for scripts
airplan config schema          # print the complete config file JSON Schema
```

`config show` always redacts access keys, secret keys, and API tokens. It
resolves the active configuration without contacting storage or resolving the
standard AWS credential chain. `config profiles --json` provides a scriptable
profile inventory without requiring each profile to be complete.

## Shell completion

airplan generates completion scripts at runtime for Bash, Zsh, Fish, and
PowerShell:

```sh
# Bash
source <(airplan completion bash)

# Zsh
source <(airplan completion zsh)

# Fish
airplan completion fish | source
```

```powershell
# PowerShell
airplan completion powershell | Out-String | Invoke-Expression
```

These commands enable completion for the current session. Run
`airplan completion <shell> --help` for persistent installation instructions
specific to that shell and platform.

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

Collections broaden what can leak through a capability URL. Review screenshots
and recordings for tokens, usernames, private messages, browser chrome, and
unrelated desktop content before uploading. Filenames are public in direct URLs
and on the overview. HTML and SVG members may execute active content when
opened; Airplan uploads every member byte-for-byte and does not sanitize it.
Document assets have the same byte-exact trust boundary. HTML, SVG, and
JavaScript assets may execute when opened or embedded. Every nested page and
asset URL shares the entry's capability boundary and persists until the owning
document is deleted.

Use `airplan purge --older-than 30d --yes` manually or from cron when uploads
should expire. Mark uploads that must survive bulk cleanup — a demo linked
from a README, say — with `airplan protect --reason "why" <url-or-key>`.
Purge skips protected uploads with a stderr note and no failure, and
`delete` refuses them unless given `--force`; `airplan unprotect` lifts the
mark again. Protection lives in the bucket as a small
`.airplan-protected.json` sentinel object, so it follows the upload across
machines; note that older airplan builds do not know about the sentinel and
will purge straight through it. Protection-aware builds also reject older
collections that already used that now-reserved basename; remove those with the
older client or verified storage tooling. For large remote inventories,
`purge --remote --concurrency N`
changes only parallel marker inspection (default 8, range 1-64); destructive
deletions stay sequential after confirmation. For defense in depth on
Cloudflare, a Transform Rule can add an
`X-Robots-Tag: noindex` response header to the custom domain.

## Go library

The CLI is a thin shell over an importable Go package:

```go
import (
    "context"
    "fmt"
    "io"
    "os"

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

Use `UploadDocument` when one entry owns managed pages and assets. Logical
paths are explicit; filesystem-root and symlink checks belong to the CLI or
another local adapter:

```go
func uploadBundle(
    ctx context.Context,
    client *airplan.Client,
    entry io.Reader,
    design io.Reader,
    diagram io.ReadSeeker,
    diagramSize int64,
) error {
    res, err := client.UploadDocument(ctx, airplan.DocumentInput{
        Entry: airplan.PageInput{Reader: entry, Path: "README.md"},
        Pages: []airplan.PageInput{{
            Reader: design,
            Path:   "docs/design.md",
        }},
        Assets: []airplan.AssetInput{{
            Reader:      diagram,
            Path:        "images/flow.svg",
            Size:        diagramSize,
            ContentType: "image/svg+xml",
        }},
    })
    if err != nil {
        return err
    }
    fmt.Println(res.URL, res.Pages[1].URL, res.Assets[0].URL)
    return nil
}
```

`CreateDocumentRevision` creates a complete replacement revision. The older
`UpdateDocument` API remains as a deprecated, source-compatible one-entry
wrapper. `RenderDocument` renders the complete bundle without storage;
`Upload` and `RenderInput` remain the single-page wrappers.

Collections use seekable readers and declared sizes so large members can
stream without whole-file buffering:

```go
func uploadFiles(ctx context.Context, client *airplan.Client) error {
    image, err := os.Open("screenshot.png")
    if err != nil {
        return err
    }
    defer image.Close()

    info, err := image.Stat()
    if err != nil {
        return err
    }
    res, err := client.UploadFiles(ctx, airplan.FilesInput{
        Files: []airplan.FileInput{{
            Name:   "screenshot.png",
            Reader: image,
            Size:   info.Size(),
        }},
    })
    if err != nil {
        return err
    }
    fmt.Println(res.Files[0].URL, res.URL)
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
mise run check:spec-sync    # check contract changes update spec versions
mise run test:coverage      # statement summary + coverage.html report
mise run test-integration   # MinIO round trip; requires Docker
mise run container:context  # prepare image inputs from snapshot binaries
mise run container:build    # build and load the native image; requires Docker
mise run container:check    # validate amd64 + arm64 images; requires Docker
mise run test:container-integration # container server smoke; requires Docker
mise run test:browser       # Chromium page smoke tests; installs browser
mise run test:web           # pure TypeScript unit tests
mise run typecheck:web      # strict browser + Playwright TypeScript checks
mise run generate:web:check # verify committed browser assets
mise run audit:deps         # audit Go modules and Bun dependencies
mise run release:snapshot   # build release artifacts without publishing
mise run verify             # broad local validation
mise run update:mermaid     # update an eligible, 72-hour-old Mermaid pin
```

`mise run setup` installs Bun, performs a frozen `bun ci`, and installs the Git
hooks. Bun is the repository package manager and builds the maintained
TypeScript and CSS into readable and minified committed browser assets.
Playwright runs under Bun, so the development toolchain does not install Node.
`bunfig.toml` rejects every direct and transitive package release newer than
three days with no standing exclusions. A confirmed security update may add a
narrowly scoped temporary exception in the same update and must remove it once
the release reaches the normal age.

The browser suite requires Chromium. The shared task installs the matching
build on demand, and CI also installs Chromium's Linux system dependencies.
Failed CI runs retain the Playwright HTML report, traces, screenshots, and
other test results for seven days. On a Linux development host missing those
system libraries, run `bun ci` followed by
`bunx playwright install-deps chromium` once; the latter may require elevated
privileges.

`mise run check:spec-sync` compares the branch with `origin/main` by default.
Set `SPEC_SYNC_BASE` to check against another revision. It requires a SPEC.md
version bump when `main.go`, non-test Go files in `cli/` or `airplan/`, page
assets, or the generated configuration schema are added, changed, moved, or
removed. It also keeps the target version in IMPLEMENTATION.md aligned with a
bumped spec. The PR job currently reports policy findings as warning
annotations while the signal matures; operational failures still fail CI.
Local policy findings fail the task.

See [AGENTS.md](AGENTS.md) for the repository map and contribution constraints.
Releases are managed by release-please and GoReleaser from conventional commits.

The first container release creates a private GHCR package. For that one-time
bootstrap, confirm the package is linked to this repository, change its
visibility to public in GitHub package settings, and then verify anonymous
exact-tag and digest pulls. Also verify the GitHub attestation, run the image
on real amd64 and arm64 hosts, inspect the package metadata, and repeat the
documented commands from a clean machine.

## License

MIT — see [LICENSE](LICENSE).
