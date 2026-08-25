# Manage Airplan uploads

Airplan records successful uploads in a local append-only manifest. Management
commands use that history by default and can inspect the current bucket when
`--remote` is supplied.

This guide covers listing, fetching, synchronization, deletion, purge
protection, Markdown renderer upgrades, and linked revisions. [SPEC.md section
9](../SPEC.md#9-history--cleanup) defines the full contract.

## Manifest location

On Linux and macOS, the default manifest is
`$XDG_STATE_HOME/airplan/manifest.jsonl`, normally
`~/.local/state/airplan/manifest.jsonl`. Windows uses its per-user local app data
directory, normally `%LocalAppData%\airplan\manifest.jsonl`.

Override the path with `--manifest PATH` or `AIRPLAN_MANIFEST`. Relative paths
resolve from the current working directory. The option applies to direct S3
commands, `serve`, and stdio MCP when it uses S3.

An HTTP client rejects an explicit `--manifest` because only the server can
choose its filesystem path. It ignores `AIRPLAN_MANIFEST`.

## Common commands

```sh
airplan list                     # uploads known to the local manifest
airplan ls -r                    # Airplan uploads found in the bucket
airplan show <url-or-key>        # validate and inspect one remote upload
airplan get <url-or-key>         # fetch a document entry or collection overview
airplan delete <url-or-key>      # delete one upload
airplan protect <url-or-key>     # mark one upload purge-protected
airplan unprotect <url-or-key>   # remove purge protection
airplan purge --older-than 30d   # review and delete older uploads
airplan sync                     # reconcile remote uploads into local history
```

`ls` aliases `list`. `-r` aliases `--remote` for `list` and `purge`.

## List and filter uploads

Local `list` filters history to the active profile. `--profile NAME` selects
another recorded profile, while `--profile=` selects root-level history.
`-A` or `--all-profiles` lists every recorded profile even when configuration
cannot resolve one active profile.

An `airplan` backend reads the server manifest. It rejects `--all-profiles`
because the backend exposes only the server-scoped manifest.

Selection flags work with local or remote listing and with `--json`:

```sh
airplan list --newer-than 7d
airplan list --older-than 30d --kind document
airplan list --slug implementation-plan
airplan list --protected
airplan list --no-protected
airplan list --limit 20
```

`--newer-than` and `--older-than` accept an age such as `7d`, `2w`, or `36h`,
an absolute date such as `2026-07-01`, local date and time such as
`2026-07-01 09:30`, or a strict RFC 3339 timestamp. Bare dates mean local
midnight. Airplan refuses ambiguous slash dates such as `03/04/2026`.

`--newer-than` includes uploads recorded at the boundary. `--older-than`
excludes them. `--limit N` keeps the N most recent matches while the default
table still prints oldest first.

Purge requires its resolved `--older-than` boundary to be in the past. Use
`--all` to request deletion of everything eligible.

## Choose table columns

Table formatting is separate from row selection:

```sh
airplan list --columns date,title,url
airplan list --columns +dir,-title
airplan list --wide
airplan list --reverse
```

`--columns date,title,url` selects one set. `--columns +dir,-title` adjusts the
default. `--wide` prints every available column, and `--reverse` prints newest
first.

`PROFILE`, `STATE`, and `DIRECTORY` appear automatically when more than one
profile, legacy or protected history, an active protection filter, or a row
without a URL makes them relevant. Local `STATE` values are `managed`,
`legacy`, `protected`, or `legacy+protected`; remote values are `protected` or
`unprotected`.

Column flags are table-only and fail with `--json`. `--reverse` reorders both
formats.

## Inspect and fetch an upload

`show` validates an upload's ownership marker, kind, declared files, and
completeness:

```sh
airplan show <url-or-key>
airplan show --json <url-or-key>
```

For a document directory or marker target, `get` returns the entry by default.
Use selectors for another object:

```sh
airplan get --source <url-or-key>
airplan get --diff <url-or-key>
airplan get --page docs/design.md <url-or-key>
airplan get --page docs/design.md --source <url-or-key>
airplan get --asset images/flow.svg <url-or-key>
airplan get -o plan.html <url-or-key>
```

A direct declared child URL returns that object and cannot be combined with a
selector. Direct child targets work for inspection and lifecycle commands, but
delete, protect, unprotect, purge, sync, and upgrade always act on the owning
document.

When `show`, `get`, or `delete` matches one marker-managed local record, Airplan
selects its recorded profile unless `--profile` or `AIRPLAN_PROFILE` was
explicit. The remote ownership marker remains authoritative.

## Discover remote uploads

`list --remote` reads storage through the selected backend and can find uploads
from other machines. It recognizes document `.airplan.json` and collection
`.airplan-collection.json` ownership markers.

A directory with both marker names is a conflict. Airplan does not infer a URL
or manage it. Markerless directories are invisible.

Current Airplan releases manage marker versions 1 through 6. New uploads use
version 6, whose model covers document pages, sources, assets, diffs, and
collection files. Every declared payload has a provider-independent SHA-256
digest. Older clients fail closed when they encounter a marker version they do
not understand.

## Synchronize local history

`airplan sync` imports complete marker-managed uploads from the bucket into the
local manifest. It also tombstones local records whose markers are confirmed
absent remotely.

```sh
airplan sync
airplan sync --dry-run
airplan sync --no-prune
airplan sync --concurrency 16
```

`--dry-run` previews the changes. `--no-prune` makes synchronization
additive-only. `--concurrency` controls concurrent marker requests and accepts
1 through 64; the default is 8.

Sync uses one bucket listing, then confirms apparent absences with a targeted
request. It can enrich older records that lack object and byte totals. Marker
fetch or enrichment problems become warnings for a later sync rather than
failing an otherwise useful reconciliation.

Sync converges the current remote inventory. It does not upload the local JSONL
event stream or reconstruct deleted history.

## Protect uploads from purge

Protection stores a small `.airplan-protected.json` sentinel in the upload
directory, so it follows the upload across machines:

```sh
airplan protect --reason "linked from project README" <url-or-key>
airplan unprotect <url-or-key>
```

Purge skips protected uploads. `delete` refuses them unless `--force` is given.

Protection is enforced by compatible Airplan clients, not by storage. Older
Airplan releases and native storage tools ignore the sentinel. Older collections
that already contain the now-reserved sentinel basename must be removed with an
older client or verified storage tooling.

## Purge old uploads

Preview a purge before applying it:

```sh
airplan purge --older-than 30d
airplan purge --older-than 30d --yes
airplan purge --remote --older-than 30d --yes
airplan purge --all --include-versioned
```

Interactive purge asks for confirmation. Non-interactive execution requires
`--yes`. Linked revision history is excluded unless
`--include-versioned` is explicit.

For a large remote inventory, `--concurrency N` changes marker inspection only.
Deletions remain sequential after confirmation.

## Upgrade rendered Markdown

Airplan can re-render a source-backed Markdown upload with the current page
renderer while preserving its public URLs, source and asset bytes, creation
time, revision identity, and protection state:

```sh
airplan upgrade --check https://plans.example.com/vq3n.../plan.html
airplan upgrade https://plans.example.com/vq3n.../plan.html
airplan upgrade --force --template ./new-page.tmpl \
  https://plans.example.com/vq3n.../plan.html
airplan upgrade --all --dry-run
airplan upgrade --all --yes
airplan upgrade --all --all-profiles --yes
```

For a bundle, upgrade re-renders every managed page and writes the entry last.
Stored SHA-256 digests let it skip pages already in the target state. A failed
attempt may leave different renderer generations at different page URLs. A
retry repairs only the remaining pages.

Upgrade plans against live ownership markers and replans immediately before
writing. Only a still-upgradeable target whose marker and page ETags match the
inspection can change.

`--force` permits a custom template or theme recipe change. Pass
`--template PATH` for a custom replacement or `--template=` to return to the
built-in template. After changing theme configuration, run
`airplan upgrade --force <url-or-key>` before creating another revision. Airplan
otherwise refuses to render a revision with a different theme recipe.

`--all-profiles` inventories configured profiles even when none is selected by
default. It ignores `AIRPLAN_PROFILE` for that inventory, requires every
participating profile to use direct S3, and cannot be combined with an explicit
`--profile`.

With `--json`, dry-run returns a plan. Apply returns a result object even when
the plan is empty or already current.

## Create linked revisions

`new-revision` uploads a complete replacement while preserving each revision's
original source, URL, and numbered identity. The rendered page at a revision URL
can still change through the explicit upgrade operation described above.

```sh
airplan new-revision --json \
  https://plans.example.com/vq3n.../plan.html plan.md

airplan new-revision \
  https://plans.example.com/vq3n.../readme.html \
  README.md docs/design.md images/flow.svg
```

`update` remains a compatibility alias with the same flags and behavior.

The first real update promotes the standalone upload to revision 1 and creates
revision 2 under a new capability URL. Only then does Airplan add
`.airplan-versions.json` to both uploads. Later updates may target any surviving
member and resolve the latest revision before appending.

Deleted revisions remain as numbered tombstones and their numbers are never
reused. Each revision after 1 owns a deterministic `.airplan-changes.diff`.
Small diffs appear as a highlighted Changes view, while every diff remains
available through its raw link. Anyone with one chain URL can navigate to every
surviving linked revision URL.

The JSON result includes `previous_url`, `diff_url`, and `unchanged`. Revision
numbers are omitted only for an unchanged standalone document that has not
formed a chain.

A bundle revision is a complete replacement. Resupply every page and asset that
should remain; omission removes it from the new revision. Page metadata,
ordering, source bytes, and asset metadata or bytes participate in no-op
detection. The combined Changes view contains per-path text diffs and concise
binary asset summaries without embedding binary data.
