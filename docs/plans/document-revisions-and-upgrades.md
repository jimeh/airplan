# Linked Document Revisions and Upload Upgrades Implementation Plan

Status: proposed
Scope: linked Markdown revisions, adjacent diffs, renderer provenance, and
individual or manifest-wide document upgrades
Repository baseline: Airplan v0.7.0, spec 0.33.0

## 1. Goal

Make iterative agent-authored plans easy to follow without turning an Airplan
upload directory into a mutable document database.

Each Markdown revision remains a complete ordinary Airplan upload with its own
unguessable capability URL. Airplan links those uploads into a linear revision
chain, generates a unified diff from each revision to its predecessor, and
adds a version picker to the built-in page. Opening an older URL continues to
show the exact document revision originally shared, while clearly identifying
it as old and linking to the latest revision.

Airplan also gains an explicit document-upgrade operation. It can re-render an
existing source-backed Markdown upload with the current renderer while keeping
the page URL and source bytes unchanged. Upgrades can target one upload or all
eligible active records known to the selected local manifest.

The design preserves these core properties:

- every document revision is independently readable and manageable;
- ordinary uploads use the ordinary upload pipeline;
- document history is application-managed and portable across supported
  S3-compatible services;
- document revision numbers are positive whole integers starting at `1`;
- diffs are generated once by the operation service, never in the browser;
- the browser performs only same-origin metadata discovery and navigation;
- ownership markers remain the authority for managed storage operations; and
- marker schema generations describe wire compatibility, not optional feature
  use.

## 2. Settled decisions

1. A new document revision is a full new upload with a new random directory
   and public URL. The previous page URL remains an immutable view of the
   previous document rather than becoming an alias for latest.
2. The built-in document page always contains a dormant revision-discovery
   script. It fetches `.airplan-versions.json` from its own upload directory and
   shows no revision UI when the metadata describes fewer than two revisions.
3. A regular standalone upload does not create `.airplan-versions.json`. The
   first update creates the file in both the promoted revision `1` upload and
   the new revision `2` upload. Discovery requests bypass browser and edge
   negative caches so a previously observed 404 cannot hide a newly linked
   chain.
4. Every chain member receives a complete replicated chain index. A page needs
   one metadata request rather than recursively following adjacent pointers.
5. Revision `N` stores `.airplan-changes.diff`, meaning the unified diff from
   revision `N-1` to revision `N`. Revision `1` has no diff.
6. `.airplan-changes.diff`, `.airplan-versions.json`, and future
   `.airplan-*` names are reserved Airplan control names. User-supplied
   collection members may not use that prefix.
7. Diff HTML is generated and syntax-highlighted server-side. No client diff
   library and no arbitrary pairwise diff selector are added.
8. Ownership marker version 4 is emitted for every new upload after the feature
   lands: Markdown, text, HTML, collections, linked revisions, and standalone
   uploads. Optional v4 fields express capabilities; marker versions do not
   fork according to feature use.
9. Marker v4 records the producing Airplan release. Generated pages also record
   an independent renderer generation so automatic migrations occur only when
   page structure or capabilities require them.
10. An upgrade changes rendered output and marker metadata without creating a
    document revision. The source bytes, public page URL, original creation
    time, purge-protection state, and chain identity remain unchanged.
11. An identical proposed Markdown source is a successful no-op. It returns the
    existing latest result and does not consume a revision number.
12. The local manifest selects bulk-upgrade candidates but never authorizes
    remote mutation. Every candidate is revalidated from its remote marker and
    payload before any write.
13. Linked revisions are linear. Concurrent writers may not silently fork a
    chain; a stale writer receives a conflict and must retry from the newly
    resolved latest revision.
14. Purge skips chain members by default. Explicit targeted deletion remains
    available and leaves a revision tombstone in surviving chain metadata;
    revision numbers are never reused or renumbered.

## 3. Non-goals

- One stable page URL that always displays the latest revision.
- Branching, merging, named branches, tags, or revision aliases.
- Arbitrary diffs between two user-selected revisions.
- Client-side Markdown rendering or diff calculation.
- Binary, HTML, text, or collection revision chains in the first release.
- Reconstructing a byte-identical historical HTML page after a renderer
  upgrade. The exact Markdown source is the document-history authority.
- Automatic bucket-wide discovery during `upgrade --all`; `sync` remains the
  explicit discovery operation.
- Automatically uploading or preserving custom-template source.
- A background migration service or automatic mutation merely because a new
  Airplan binary is installed.
- Native S3 bucket versioning. Provider-generated version IDs are not the
  product's document revision model and Cloudflare R2 does not expose the
  required S3 versioning operations.

## 4. Terminology

Keep four independent concepts explicit in code, JSON, CLI output, and docs:

| Term                | Meaning                                                    |
| ------------------- | ---------------------------------------------------------- |
| Marker version      | Ownership-marker wire schema; v4 after this feature        |
| Airplan version     | Release that last generated the rendered page              |
| Renderer generation | Integer page-capability generation used for upgrade checks |
| Document revision   | User-visible integer within one linked Markdown chain      |

The append-only local operation manifest has no global schema-version field.
It evolves through forward-compatible record fields and event types.

## 5. User experience

### 5.1 Creating and updating documents

Ordinary upload remains unchanged:

```sh
airplan --json plan.md
# new independent upload; no visible revision picker
```

Create a new revision from any member of an existing chain, or from a
standalone eligible upload:

```sh
airplan update <url|key> plan.md
cat plan.md | airplan update <url|key>
airplan update --json <url|key> plan.md
```

The target may be the upload directory, marker, page, source, or any chain
member's public page URL. Airplan resolves it to the chain's latest valid
revision before comparing or diffing. Supplying an older link therefore does
not accidentally branch history.

Plain stdout remains one URL: the new revision's page URL. JSON adds revision
metadata without changing the existing result field meanings:

```json
{
  "url": "https://plans.example.com/new-dir/plan.html",
  "key": "new-dir/plan.html",
  "source_url": "https://plans.example.com/new-dir/plan.md",
  "revision": 3,
  "latest_revision": 3,
  "previous_url": "https://plans.example.com/old-dir/plan.html",
  "diff_url": "https://plans.example.com/new-dir/.airplan-changes.diff",
  "unchanged": false
}
```

For an identical source, `url` identifies the existing latest page,
`unchanged` is true, and no storage or manifest write occurs.

### 5.2 Page controls

The built-in Markdown toolbar gains a revision control group when valid
metadata describes at least two live revisions:

- `Revision N of M` selector with one option per non-deleted revision;
- a prominent `Latest: revision M` action when viewing an older revision;
- previous and next navigation where those live neighbours exist;
- a distinct stale state in the toolbar and document heading area;
- a `Changes` view for revisions greater than `1`; and
- a raw diff link targeting `.airplan-changes.diff`.

The existing Rendered and Source views remain. A revision with a diff uses the
view order Rendered, Source, Changes. Changes is labelled `Changes from
revision N-1` so its direction is unambiguous.

The page embeds its creation-time revision context as a no-JavaScript
fallback. JavaScript fetches the replicated metadata to discover future
revisions. If the request fails or the metadata is invalid, the document,
source controls, theme, table of contents, printing, and Mermaid behavior
continue working; only revision navigation is unavailable.

The discovery fetch uses `cache: "no-store"` and a per-page-load query nonce.
This is required because the same metadata path legitimately changes from 404
to present when a standalone upload becomes revision `1`; neither browser nor
custom-domain negative caching may make that transition invisible.

Metadata-derived text is inserted with DOM text APIs, never as raw HTML.
Metadata URLs must pass the same public-URL and upload-key validation used by
the operation service before the page exposes them.

### 5.3 Upgrading rendered pages

One upload:

```sh
airplan upgrade --check <url|key>
airplan upgrade <url|key>
airplan upgrade --json <url|key>
```

All eligible active records known to the selected operation manifest:

```sh
airplan upgrade --all --dry-run
airplan upgrade --all
airplan upgrade --all --yes
airplan upgrade --all --all-profiles --yes
```

`--all` prompts in an interactive terminal. Non-interactive application
requires `--yes`. `--dry-run` performs the complete read-only classification
without rendering or writing remote objects.

The table result classifies every considered record:

```text
STATE        CURRENT                 TARGET                  URL
upgradeable  marker 3 / renderer ?   marker 4 / renderer 1   https://…
current      marker 4 / renderer 1   -                       https://…
ineligible   markdown source missing -                       https://…
invalid      malformed marker        -                       https://…
missing      ownership marker absent -                       https://…
```

`--json` returns the same per-record classifications and aggregate counts.
Any candidate that required an upgrade but failed makes the command non-zero.
Already-current and genuinely ineligible records do not fail the command;
invalid or missing records are warnings unless they were selected for
mutation after a successful preview.

### 5.4 Inspection and retrieval

Extend `show` with marker, producer, renderer, and document-revision fields:

```text
MARKER VERSION     4
AIRPLAN VERSION    0.8.0
RENDERER VERSION   1
DOCUMENT REVISION  2 of 3
LATEST URL         https://…
UPGRADE             available: Airplan 0.9.0 / renderer 2
```

`show --json` returns the validated chain metadata and any advisory metadata
error separately from ownership-marker validity.

Add `get --diff` for the marker-declared `.airplan-changes.diff`. It is valid
only for a linked revision greater than `1`; `get --source` keeps selecting the
exact current upload's source.

## 6. Storage layout

### 6.1 Standalone v4 Markdown upload

```text
[prefix/]<random>/.airplan.json
[prefix/]<random>/<slug>.md
[prefix/]<random>/<slug>.html
```

The page contains dormant discovery code, but no versions control object exists
until this upload becomes revision `1` of a linked chain. A missing metadata
object means standalone, not invalid.

### 6.2 Linked revisions

```text
[prefix/]<revision-1-random>/.airplan.json
[prefix/]<revision-1-random>/.airplan-versions.json
[prefix/]<revision-1-random>/<slug>.md
[prefix/]<revision-1-random>/<slug>.html

[prefix/]<revision-2-random>/.airplan.json
[prefix/]<revision-2-random>/.airplan-versions.json
[prefix/]<revision-2-random>/.airplan-changes.diff
[prefix/]<revision-2-random>/<slug>.md
[prefix/]<revision-2-random>/<slug>.html
```

Each ownership marker continues to own one random directory and deletion unit.
The v4 marker declares the page, source, and optional diff with exact sizes and
content types. `.airplan-versions.json` is a reserved mutable control object,
analogous to the protection sentinel: its presence and validity affect
revision navigation but never make the page/source payload incomplete.

All payload and control writes retain `Cache-Control: no-store`. The diff uses
`Content-Type: text/plain; charset=utf-8`; the page provides syntax-highlighted
presentation.

Remote listings include control objects in observed object and byte totals.
Marker-declared manifest totals continue to describe the immutable payload
inventory and may differ from observed totals, just as protection sentinels
already cause them to differ.

### 6.3 Reserved namespace

Marker v4 reserves every direct basename beginning `.airplan-`. The known set
starts with:

```text
.airplan.json
.airplan-collection.json
.airplan-protected.json
.airplan-versions.json
.airplan-changes.diff
```

Document slugs already cannot produce these names. Collection validation must
reject the whole prefix so later control files cannot be forged by an authored
member. Existing v1-v3 collection markers retain their original validation
rules; readers must not reinterpret an old legitimate member as a v4 control
object.

## 7. Ownership marker v4

### 7.1 Rollout rule

Set `MarkerVersion = 4` and make every writer emit v4 immediately. Continue
strictly decoding and managing versions 1 through 3. Older clients see v4 as
unsupported and fail closed rather than partially managing new directories.

Marker v4 is one capability superset. Standalone documents and collections
simply omit revision-specific fields.

### 7.2 Producer and render provenance

Every v4 marker contains producer identity:

```json
{
  "producer": {
    "name": "airplan",
    "version": "0.8.0"
  }
}
```

`version` uses the same resolved release string as `airplan --version`.
Development builds record `dev`; an unknown or non-comparable version remains
inspectable but never drives a downgrade decision.

Pages generated by Airplan additionally contain a rendering recipe:

```json
{
  "render": {
    "generation": 1,
    "template": {
      "kind": "builtin"
    },
    "indexable": false,
    "no_external_assets": false,
    "mermaid_url": "https://cdn.jsdelivr.net/npm/mermaid@…"
  }
}
```

HTML uploaded as authored has producer metadata but no render recipe. The
collection renderer uses the same generation field but records template kind
`builtin_collection` or `custom_collection`.

For a custom template, store no path or source:

```json
{
  "template": {
    "kind": "custom",
    "sha256": "<lowercase SHA-256>"
  }
}
```

The digest identifies the bytes needed for a reproducible upgrade without
exposing a local path. An automatic upgrade proceeds only when the selected
custom template has the same digest. An explicit user may supply a replacement
template, accepting a presentation change that the new marker records.

Increment render generation only when generated-page structure, embedded
assets, or capability bootstrap changes in a way that requires re-rendering.
Do not increment it for CLI-only behavior, dependency updates with identical
output, marker-only schema additions, or documentation changes.

### 7.3 Optional immutable revision descriptor

A linked document marker records immutable chain identity:

```json
{
  "revision": {
    "chain_id": "<128-bit lowercase base32>",
    "number": 2,
    "previous_url": "https://plans.example.com/revision-1/plan.html"
  }
}
```

Revision `1` omits `previous_url`. The descriptor never stores `latest`; that
is mutable navigation state in `.airplan-versions.json`. It gives inspection,
sync, and interrupted-link repair a stable fact even when replicated metadata
is stale.

The first update promotes its standalone predecessor to revision `1` as part
of the link transaction. That promotion adds the immutable descriptor to the
existing marker and re-renders the existing page with revision `1` fallback
context, but it does not change the source, URL, creation time, or protection
state. It is distinct from a renderer upgrade even though it reuses the same
safe re-rendering machinery.

Revision numbers are strictly positive. New chains begin at `1`; each
successful append consumes the next never-before-assigned integer. Deletion
creates a tombstone and never permits reuse.

### 7.4 Object roles

Extend v4 document objects with role `diff`:

- zero or one diff object;
- exact name `.airplan-changes.diff`;
- required only when revision number is greater than `1`;
- forbidden for standalone documents, revision `1`, HTML, text, and
  collections; and
- positive size with normalized `text/plain; charset=utf-8`.

Page and source rules remain unchanged. Marker v4 still declares every
immutable payload object with exact bytes. Unknown roles remain invalid.

## 8. Versions metadata contract

### 8.1 Shape

Use a separately versioned JSON control schema:

```json
{
  "schema": "airplan-versions",
  "version": 1,
  "chain_id": "vx6g5g4m5bk2vngj7msw4qun7u",
  "current_revision": 2,
  "latest_revision": 3,
  "last_assigned_revision": 3,
  "revisions": [
    {
      "number": 1,
      "url": "https://plans.example.com/first/plan.html",
      "created_at": "2026-08-15T10:00:00Z"
    },
    {
      "number": 2,
      "url": "https://plans.example.com/second/plan.html",
      "created_at": "2026-08-15T10:15:00Z",
      "diff_url": "https://plans.example.com/second/.airplan-changes.diff"
    },
    {
      "number": 3,
      "url": "https://plans.example.com/third/plan.html",
      "created_at": "2026-08-15T10:30:00Z",
      "diff_url": "https://plans.example.com/third/.airplan-changes.diff"
    }
  ]
}
```

Every live member receives the full revision array. Bodies differ only in
`current_revision`. Unlinked uploads have no versions metadata object; an empty
revision array is not a valid persisted chain.

A deleted revision remains as a tombstone so numbers are not reused:

```json
{
  "number": 2,
  "deleted": true,
  "deleted_at": "2026-09-01T12:00:00Z"
}
```

Deleted entries omit URLs. `latest_revision` is the greatest live revision;
`last_assigned_revision` is the greatest number ever assigned. A new revision
uses `last_assigned_revision + 1`.

### 8.2 Validation

Accept at most 64 KiB and require:

- exact schema and supported metadata version;
- valid UTF-8, unique JSON names, and no trailing values;
- a valid chain ID and at least two assigned revisions;
- strictly increasing unique positive revision numbers;
- one entry matching `current_revision` and the containing upload;
- `latest_revision` naming the greatest live entry;
- `last_assigned_revision` greater than or equal to every entry;
- revision `1` without a diff and each later live revision with one;
- canonical absolute HTTPS URLs under the same resolved public base URL,
  bucket, and key-prefix scope;
- page and diff URLs that resolve to valid Airplan key shapes; and
- timestamps in RFC 3339 UTC.

The operation service validates more strongly than the browser because it can
fetch markers and compare chain descriptors. The browser treats invalid data
as absent and logs one safe console warning without capability URLs.

### 8.3 Replication and reconciliation

One complete index per member is deliberately O(n) metadata writes per new
revision. Expected plan histories are short; one-request navigation and
deletion tolerance are more valuable than optimizing for very large chains.

Bound chain length by the metadata-size ceiling. Before any storage mutation,
encode every candidate metadata body and fail clearly if one exceeds the
limit. Do not truncate history or silently omit tombstones.

Metadata reconciliation is idempotent. A retry derives the canonical chain
from validated marker revision descriptors plus the winning latest-link
transition, then rewrites only stale member bodies.

## 9. Adjacent diff generation

Generate a standard unified line diff with three context lines:

```diff
--- revision-1/plan.md
+++ revision-2/plan.md
@@ -10,3 +10,4 @@
 existing context
-old statement
+new statement
+additional detail
```

Requirements:

- compare the exact previous and proposed UTF-8 source bytes;
- use stable revision-based headers rather than local filesystem paths;
- preserve meaningful final-newline behavior;
- produce deterministic LF output on every platform;
- generate before any storage mutation;
- fail the update rather than publish a revision without its required diff;
- cap generated diff size explicitly and report the limit; and
- benchmark worst-case behavior at the existing 10 MiB document input limit.

Promote the already-resolved pure-Go unified-diff implementation to a direct
dependency rather than invoking a platform `diff` command. Keep the algorithm
behind a small internal interface so focused tests can assert Airplan's format
without coupling callers to that dependency.

Highlight `.airplan-changes.diff` through Chroma's diff lexer and add these
fields to `TemplateData`:

```go
Revision               int
RevisionCount          int
PreviousRevision       int
VersionsPath           string
DiffPath               string
DiffText               string
HighlightedDiffHTML    template.HTML
```

Custom templates may ignore the additive fields. They retain full ownership of
presentation and do not automatically gain a picker or Changes view until the
template author adopts them.

## 10. Creating a new revision

### 10.1 Eligibility and preflight

The first release accepts only a complete marker-managed Markdown document
with a source object. Reject:

- HTML, text, and collections;
- `--no-source` uploads;
- invalid, unsupported, conflicting, or incomplete markers;
- a target outside the active backend's bucket and key-prefix scope;
- a chain whose metadata and marker descriptors cannot be reconciled; and
- a custom-template predecessor that cannot be reproduced for a required
  renderer upgrade.

Resolve the target's metadata to its latest live member, fetch that member's
marker and exact source, and run any required predecessor upgrade before
reading the proposed input. Re-read its marker and metadata afterward so an
upgrade cannot leave stale ETags in the revision transaction.

For a standalone predecessor, preflight also plans its promotion to revision
`1`, including the chain ID, promoted marker, revision-aware page, and the
initial two-member metadata bodies. Existing chains retain their chain ID and
allocate only the next revision number.

### 10.2 Mutation protocol

S3-compatible storage has atomic operations per key but no multi-object
transaction. Use conditional requests and a recoverable order:

1. Read the latest marker, source, and their ETags. Read and validate versions
   metadata plus its ETag for an existing chain; confirm metadata absence for a
   standalone target.
2. Render the proposed document and generate its adjacent diff locally.
3. Return the existing latest result immediately when source bytes are equal.
4. Reserve `last_assigned_revision + 1` in memory and encode all future
   metadata bodies. For a standalone upload, assign the existing document
   revision `1`, assign the candidate revision `2`, and generate a new chain
   ID.
5. Create the complete new ordinary upload under a fresh random directory. Its
   marker contains the immutable chain descriptor and declares the diff.
6. Conditionally publish the old latest's versions metadata. The first link
   creates it with `If-None-Match: *`; later appends replace it with `If-Match`
   against the validated ETag. This transition to the new URL is the
   serialization point. A precondition failure means another writer won.
7. On a lost race, delete the unannounced candidate upload as scoped rollback.
   Report any cleanup failure and leave the managed orphan visible to normal
   cleanup tooling.
8. On the first link, conditionally replace the predecessor's standalone
   marker with its revision `1` marker and then replace its page with the
   revision-aware rendering. This is a repairable post-commit promotion; it
   preserves the predecessor's URL, source, creation time, and protection.
9. Write the full metadata body to the new revision and propagate the full
   index to every surviving member with ETag conditions.
10. Verify every live member's marker descriptor and metadata report the same
    chain, latest revision, and assignment high-water mark, and verify the
    promoted page and marker when this was the first link.
11. Append local manifest upload/link events and return the new URL only after
    verification.

After the serialization point, a propagation failure returns an error but does
not roll back the winning chain append. Retrying from any member detects the
committed next link, repairs replication, and returns the already-created new
revision rather than allocating another number. This repair includes finishing
a first predecessor's marker/page promotion when the winning metadata
transition committed before those writes completed.

Use `PutObject If-Match` and `If-None-Match` through the existing AWS SDK.
Treat HTTP 409/412 conditional failures as typed conflicts, distinct from
ordinary transport failures. R2 and AWS provide strong read-after-write and
list consistency through their storage APIs; public-domain cache behavior is
kept out of the mutation protocol.

## 11. Document upgrade operation

### 11.1 Eligibility

An automatic document upgrade initially supports Markdown uploads whose
validated marker declares an existing source. Decode v1 through v4 into the
current normalized model before planning.

An upgrade is required when:

- the marker schema is older than v4;
- the recorded renderer generation is older than the binary's generation;
- revision linking requires picker bootstrap that the page lacks; or
- an interrupted prior upgrade left marker/page sizes inconsistent and the
  source is sufficient for repair.

A user-requested upgrade may also re-render a page produced by an older
Airplan release even when its renderer generation matches. `--force` permits
an explicit same-version re-render or repair. Refuse an implicit downgrade
when a marker records a newer renderer generation.

HTML is never re-rendered. Source-less documents are ineligible. Text and
collection regeneration are deferred; marker-only migration for those kinds
may be added once it has a concrete management benefit.

### 11.2 Rendering recipe

For v4, reproduce the stored render recipe. Preserve marker title, slug,
repository context, source basename, indexability, external-asset policy, and
Mermaid URL. Require the matching custom-template digest unless the user
explicitly supplies a replacement.

Versions 1 through 3 do not contain a complete recipe. `--check` reports that
the recipe will be inferred from the currently selected operation profile.
The actual upgrade uses marker-derived title, slug, repository metadata, and
source identity together with current profile rendering policy, then records
that complete v4 recipe for future deterministic upgrades.

### 11.3 Mutation protocol

1. Resolve and validate the remote marker and source; local history is never
   sufficient.
2. Fetch the current page and marker ETags.
3. Render and encode the complete v4 marker before mutation.
4. If page bytes and all v4 provenance/recipe fields already match, return a
   no-op unless `--force` was given.
5. Conditionally write the upgraded marker using the old marker ETag.
6. Write the page last using the old page ETag.
7. Verify the page, source, marker, protection sentinel, and versions metadata
   when present. A standalone upgrade must preserve versions metadata absence.
8. Append a manifest `upgrade` event containing the refreshed projection.

The marker-first/page-last order follows the current creation recovery model:
an interruption remains discoverable as incomplete and a retry can reproduce
the page from its unchanged source. Never report success before final
verification.

Upgrading a chain member does not alter its document revision, diff, chain
high-water mark, or versions metadata. It may refresh the built-in picker and
Changes presentation from the already-declared source and diff.

## 12. Bulk upgrade from manifest history

### 12.1 Selection

`upgrade --all` reads the reduced active records from the selected operation
service's manifest. By default it scopes them exactly like `list`:

- selected profile;
- resolved bucket; and
- resolved key prefix.

Deduplicate candidates by `(bucket, marker_key)`. Tombstones, unknown manifest
event types, pre-marker legacy records, and inactive duplicates never become
mutation candidates.

`--all-profiles` is local-S3-only. It reloads every referenced named profile
from current configuration and verifies its bucket and prefix against the
record before connecting. A missing profile or configuration drift fails that
record closed. The manifest never stores or supplies endpoints, public bases,
or credentials.

For an `airplan` backend, bulk selection and execution occur on the server's
manifest and one configured S3 profile. A client cannot request arbitrary
server filesystem paths or profiles.

### 12.2 Planning and execution

Model bulk upgrade like safe purge:

- planning is read-only and returns exact candidate identities, current marker
  ETags, reason, and target renderer generation;
- interactive CLI confirmation occurs between planning and execution;
- execution accepts only the planned candidate identities and rechecks their
  ETags before mutation; and
- storage drift produces a per-item conflict rather than widening selection.

Support bounded concurrency for independent directories, defaulting to a
conservative value and preserving input/result order. One failure does not
cancel unrelated upgrades, but context timeout prevents starting new jobs and
is reported distinctly from completed failures.

`sync` remains the way to import remotely present uploads before bulk upgrade:

```sh
airplan sync
airplan upgrade --all --dry-run
airplan upgrade --all --yes
```

### 12.3 Manifest events

Add an `upgrade` event carrying:

- event time;
- identity `(bucket, marker_key)` and profile;
- stable page/source keys and URL;
- refreshed marker, producer, and renderer versions;
- page and declared payload sizes; and
- preserved original creation time and revision descriptor.

Manifest reduction applies the event to the active upload projection without
clearing protection or treating the document as newly created. Older readers
ignore the unknown event type and remain safe; their later remote operation
still fails closed if they cannot decode marker v4.

Add a `link` projection event only when revision fields need to appear in local
listing without a remote read. Keep replicated versions metadata authoritative
for navigation; local link events remain reconstructable by `sync`.

## 13. Lifecycle behavior

### 13.1 Protection

Protection remains per upload directory. Upgrading or linking never deletes,
rewrites, or implicitly creates `.airplan-protected.json`. A chain may contain
both protected and unprotected revisions.

### 13.2 Purge

Manifest and remote purge skip live version-chain members by default even when
their individual creation times match the age filter. Add an explicit
`--include-versioned` acknowledgement for callers that intend to prune chain
history. Existing protection rules still apply independently and win unless
their established force mechanism is used.

Dry-run output identifies skipped chain members and their chain/revision
numbers. A current latest revision is never inferred safe to purge merely
because an older replicated metadata copy fails to mention it.

### 13.3 Targeted deletion

Deleting one linked upload remains an explicit single-directory operation:

1. Validate its marker, protection state, and chain metadata.
2. Plan metadata bodies that tombstone its revision in every surviving member.
3. Conditionally propagate those tombstones.
4. Delete the target payloads, control objects, protection sentinel when
   forced, and ownership marker in the existing safe order.
5. Verify surviving metadata and append the normal delete tombstone plus link
   projection events.

If metadata propagation fails, abort before deleting the target. If deletion
fails after successful propagation, a retry observes the tombstone and either
completes deletion or restores the live entry if the target remains complete.
Specify and test this recovery decision rather than leaving a permanent hidden
live upload.

Deleting the latest makes the greatest remaining live revision the displayed
latest, but `last_assigned_revision` does not decrease. The next append uses a
new number. No command renumbers surviving revisions.

Whole-chain deletion and chain compaction are deferred until there is a
concrete use case.

## 14. Go library design

Add public operation types without putting business logic in `cli/` or protocol
adapters:

```go
type UpdateDocumentInput struct {
    Target string
    Input  Input
}

type UpdateDocumentResult struct {
    Result
    Revision       int
    LatestRevision int
    PreviousURL    string
    DiffURL        string
    Unchanged      bool
}

type UpgradeDocumentOptions struct {
    Force    bool
    Template *template.Template
}

type UpgradeDocumentPlan struct { /* identity and expected ETags */ }
type UpgradeDocumentResult struct { /* refreshed result and status */ }
type BulkUpgradePlan struct { /* ordered candidate classifications */ }
type BulkUpgradeRequest struct { /* exact selected candidates */ }
type BulkUpgradeResult struct { /* per-item outcomes and totals */ }
```

Expose facade methods backed by the selected operation transport:

```go
func (c *Client) UpdateDocument(
    context.Context, UpdateDocumentInput,
) (*UpdateDocumentResult, error)

func (c *Client) PlanUpgradeDocument(
    context.Context, string, UpgradeDocumentOptions,
) (*UpgradeDocumentPlan, error)

func (c *Client) UpgradeDocument(
    context.Context, UpgradeDocumentPlan,
) (*UpgradeDocumentResult, error)

func (c *Client) PlanBulkUpgrade(
    context.Context, BulkUpgradeOptions,
) (*BulkUpgradePlan, error)

func (c *Client) ExecuteBulkUpgrade(
    context.Context, BulkUpgradeRequest,
) (*BulkUpgradeResult, error)
```

Keep these internal concerns separate:

- marker v1-v4 normalization and encoding;
- versions-metadata validation and replication;
- diff generation;
- revision transaction/reconciliation;
- render upgrade planning;
- conditional storage primitives and typed conflicts; and
- manifest projection/event writing.

Extend storage with ETag-bearing `GET`/`HEAD`, conditional byte `PUT`, and
conditional control-object updates. Do not leak AWS SDK types through the
public API.

## 15. CLI, REST, MCP, and capability parity

### 15.1 CLI

Add `update` and `upgrade` Cobra commands as thin adapters. Reuse document
input loading, config resolution, timeout, repository resolution, JSON output,
warning handling, and stdout purity.

Reject upload-only identity flags that cannot change during update, especially
`--slug` and `--no-source`. Allow presentation inputs such as title only when
their effect and persistence are defined by the revision marker recipe.

Bulk table output, warnings, and progress go to stderr; stdout remains reserved
for a result URL or one JSON object.
Never print capability URLs in debug/progress logs.

### 15.2 OpenAPI

Add schema-first operations under `/api/v1`:

- multipart `updateDocument` with target in the metadata part;
- `planDocumentUpgrade` for one target;
- `executeDocumentUpgrade` bound to the planned identity/ETags;
- `planBulkUpgrade`; and
- `executeBulkUpgrade` with an exact candidate set.

The target stays in a request body rather than an access-log path. Generate the
strict server, client, and models from `api/openapi.yaml`. Keep the API major at
v1 because the additions are backward-compatible; advertise operation support
through capabilities so newer clients fail clearly against older servers.

Hosted requests use server rendering policy. They never accept local template
paths. A replacement custom template is therefore a local-S3-only explicit
operation until a safe server-owned template-selection contract exists.

### 15.3 MCP

Add:

```text
update_document
upgrade_document
upgrade_documents
```

`update_document` accepts content and a capability target, never a hosted local
path. `upgrade_document` plans or applies one target. `upgrade_documents`
defaults to preview and requires `apply: true` plus the preview's exact
candidate identities to mutate. Return structured per-item outcomes and only
safe human summaries; do not echo capability URLs in errors or server logs.

Local stdio MCP may use the ordinary local file path workflow where the
existing upload tool permits it. Hosted MCP remains content-based.

### 15.4 Shipped agent skill

Update `skills/airplan/SKILL.md` and embedded MCP descriptions so agents:

- use `update_document` when the user asks to revise an existing Airplan plan;
- may supply any known chain URL because Airplan resolves latest;
- return the new revision URL;
- do not create revisions for byte-identical content;
- do not bulk-upgrade opportunistically; and
- run upgrade operations only when the user requests maintenance or revision
  creation requires a compatible predecessor.

Any observable tool description, CLI flag, response field, or page behavior
must update `SPEC.md` in the same change.

## 16. Local manifest, list, show, and sync

Extend manifest projections with optional:

```text
producer_version
renderer_version
created_at
revision_chain_id
revision
latest_revision
```

Keep `time` as event time. Preserve original upload creation separately so an
upgrade does not make an old upload look newly created. A genuinely new
revision is a new upload and naturally has its own creation time.

Add optional wide list columns for `REVISION`, `LATEST`, `AIRPLAN`, and
`RENDERER`; do not expand the compact default table until real usage shows the
extra density is worthwhile. JSON exposes the fields when known.

Remote inspection validates `.airplan-versions.json` after the ownership
marker. Metadata failure produces an advisory revision error, not an invalid
ownership marker or incomplete document payload.

`sync` remains marker-led. For v4 Markdown documents it may fetch the small
versions control after marker validation to reconstruct revision projections.
Bound and parallelize those reads with the existing inspection concurrency;
do not add one unbounded request goroutine per upload.

## 17. Specification and documentation changes

This is an observable backward-compatible feature and should receive a spec
minor bump from the implementation branch's actual baseline. Against the
current baseline, that is expected to be 0.34.0. If other contract changes land
first, calculate the bump from the then-current spec rather than hard-coding
0.34.0.

Update at least:

- SPEC processing, rendering, upload, marker, key, history, CLI, lifecycle,
  backend, REST, MCP, and security sections;
- IMPLEMENTATION marker/transport/render/storage descriptions;
- README document upload, update, upgrade, cleanup, and examples;
- `skills/airplan/SKILL.md` and its embedded-byte test;
- OpenAPI source and generated code;
- config/schema docs only if durable settings are added; and
- completion expectations for the new commands and flags.

Document that anyone who can read one chain URL learns every linked capability
URL in that chain. This is intentional sharing behavior: chain membership
widens the read capability from one revision to the complete revision history.

## 18. Testing strategy

### 18.1 Marker and metadata unit tests

Cover:

- every upload kind emitting marker v4;
- v1-v3 decode compatibility;
- producer and render-recipe validation;
- standalone versus linked revision descriptors;
- diff role and reserved `.airplan-*` names;
- metadata live, deleted, malformed, oversized, duplicate, gapped, empty, and
  inconsistent cases, with empty metadata rejected;
- URL scope and percent-encoding validation;
- high-water revision behavior after deletion; and
- deterministic JSON encoding where bytes participate in tests or ETags.

### 18.2 Rendering and browser tests

Add focused goldens for:

- ordinary Markdown with dormant picker bootstrap;
- revision `1` without Changes;
- current and stale later revisions;
- highlighted unified diff; and
- additive custom-template fields.

Extend Chromium smoke coverage across desktop/narrow and light/dark projects:

- no picker when the versions metadata request returns 404;
- a cache-busted discovery request finding metadata after an earlier 404;
- picker population from mocked/fixture metadata;
- stale highlight and latest navigation;
- previous/next selection;
- Rendered, Source, and Changes view switching;
- accessible labels, focus, and keyboard operation;
- invalid/failed metadata preserving the document;
- print hiding navigation while preserving intended content; and
- raw diff/source links resolving to the correct revision.

Use behavioral selectors. Keep screenshots and traces as failure evidence, not
goldens.

### 18.3 Update transaction tests

Exercise:

- standalone upload becoming revisions 1 and 2;
- a third revision created from the latest URL;
- a third revision requested through revision 1 resolving revision 2 first;
- byte-identical no-op;
- deterministic diff headers and final-newline cases;
- diff size failure before mutation;
- conditional race with exactly one winner;
- candidate rollback after a lost race;
- partial metadata propagation followed by idempotent repair;
- no stdout URL before full verification; and
- protection preservation.

Confirm each new test runs by name or count. For the core concurrent-append
regression, observe the intended failing assertion before the implementation
passes.

### 18.4 Upgrade tests

Use marker v1, v2, v3, and v4 fixtures to cover:

- current built-in no-op;
- old schema migration;
- old renderer migration under current schema;
- old Airplan producer with unchanged and changed output;
- source bytes and URL preserved;
- marker-first/page-last request order;
- interrupted upgrade repaired on retry;
- stale ETag conflict;
- source-less and HTML ineligibility;
- unknown legacy recipe using current policy with an explicit warning;
- matching and mismatching custom-template digests;
- revision metadata and diff preserved; and
- manifest upgrade reduction preserving protection and original creation.

### 18.5 Bulk upgrade tests

Cover:

- active-record reduction and identity deduplication;
- selected-profile scoping;
- all-profile reconstruction and configuration drift;
- stale, missing, invalid, ineligible, current, and upgradeable records;
- dry-run causing no writes;
- non-interactive apply requiring `--yes`;
- exact candidate/ETag execution binding;
- bounded concurrency and stable result order;
- partial failures with continued independent work;
- context timeout preventing new jobs; and
- REST/MCP preview-by-default behavior.

CLI tests must use the existing environment-isolation helpers so a worktree's
`AIRPLAN_PROFILE` cannot select the wrong manifest.

### 18.6 Integration evidence

Run the MinIO integration suite for real conditional PUT/GET/HEAD behavior and
a complete multi-revision round trip. Add server transport coverage proving the
same result through the `airplan` backend.

Before handoff run:

```sh
mise run check
mise run test-integration
mise run test:browser
mise run audit:deps
mise run release:snapshot
```

Use `mise run verify` for the final broad gate when Docker is available. A real
R2 smoke test is optional, requires explicit authorization, and must use the
developer's existing configuration without printing credentials. It should
verify conditional conflict behavior, old-page stale indication, new-page
diff, and upgrade visibility through the public custom domain.

## 19. Delivery sequence

Deliver this in two independently reviewable feature PRs rather than one
contract-wide change.

### PR 1: Marker v4 and document upgrades

1. Update SPEC for marker v4, provenance, render recipes, control namespace,
   and upgrade behavior.
2. Implement v4 read/write compatibility and make every upload writer use it.
3. Add producer/render provenance and reserved control-name validation.
4. Bake dormant picker discovery into the built-in Markdown renderer and
   keep standalone Markdown uploads free of versions metadata.
5. Implement individual upgrade planning/execution with conditional storage.
6. Add manifest upgrade events and individual CLI/REST/MCP surfaces.
7. Implement safe manifest-wide preview/execution and all-profile local mode.
8. Update docs and the shipped agent skill.
9. Run focused, integration, browser, generated-file, and broad validation.

PR 1 is useful alone: future uploads are upgrade-aware, old source-backed
Markdown pages can be refreshed, and maintainers can migrate known history.

### PR 2: Linked revisions and adjacent diffs

1. Update SPEC for revision identity, versions metadata, update semantics,
   diff objects, and lifecycle behavior.
2. Add diff generation/highlighting and template data.
3. Implement versions metadata parsing, replication, and reconciliation.
4. Implement `UpdateDocument` with conditional linear-chain append and
   idempotent recovery.
5. Add picker/stale/Changes browser behavior.
6. Add CLI, REST, MCP, capability, and agent-skill update surfaces.
7. Integrate list/show/get/sync/delete/purge behavior.
8. Validate concurrent updates, partial propagation, lifecycle recovery, and
   both backends end-to-end.

Do not expose linked revisions before deletion and purge behavior are safe;
revision history cannot ship as an unmanaged exception to Airplan's normal
ownership lifecycle.

## 20. Completion criteria

The feature is complete when all of the following are true:

- every new upload kind writes and round-trips marker v4;
- current clients continue to manage valid v1-v3 uploads;
- new ordinary Markdown pages have no versions metadata and show no revision UI
  but are ready to join a future chain;
- updating a standalone Markdown upload creates revisions 1 and 2 as complete
  independent uploads and creates versions metadata in both;
- any chain URL resolves to latest before a new revision is created;
- stale pages clearly identify and navigate to latest;
- each revision greater than 1 exposes its server-generated adjacent diff;
- identical Markdown creates no revision;
- concurrent updates have one winner and no silent fork;
- interrupted metadata propagation and upgrades are safely retryable;
- individual and manifest-wide upgrades preserve URLs, source, protection,
  chain identity, and original creation time;
- bulk upgrade is previewable, profile-scoped, and remotely revalidated;
- custom-template limitations are explicit and fail closed;
- linked deletion and purge cannot silently corrupt navigation history;
- S3 and Airplan backends provide equivalent intent and results;
- SPEC, README, IMPLEMENTATION, OpenAPI, generated files, completions, and the
  shipped agent skill agree; and
- focused tests, `mise run check`, integration, browser, dependency audit, and
  release snapshot validation pass on the final revision.
