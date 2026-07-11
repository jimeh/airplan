# Bug Audit and Test-Coverage Review

Status: **hardening pass complete; no active correctness or panic findings**.

Audit date: 2026-07-11. Contract baseline: SPEC.md 0.10.0.

This document is the final state of the bug-hunting work. It incorporates the
earlier multi-agent findings, revalidates them against the current code, and
records the fixes and residual constraints that remain after the lifecycle,
HTML scanner, CLI-boundary, and public-API work.

## Result

No supported CLI input or exported-library call is known to panic. The upload,
inspection, deletion, and purge lifecycle now follows the marker ownership
model in SPEC.md, and every previously active correctness finding has either
been fixed or turned into an explicit documented contract.

The fresh final scan covered exported entry points, goroutines, unchecked type
assertions, slice/index operations, integer arithmetic, file replacement,
manifest locking, marker validation, and storage failure propagation. It found
one additional cross-platform mismatch: `os.Rename` does not promise atomic
replacement on Windows. That was fixed with the native Windows replacement
path before this audit was closed.

## Verification evidence

The final tree passed:

- `mise run verify`
  - formatting and generated-schema checks
  - Go and workflow linting
  - all unit tests
  - Docker-backed MinIO upload/show/delete lifecycle integration test
  - GoReleaser configuration validation
- `go test -race ./...`
- `go vet ./...`
- Windows/amd64 CLI test-binary cross-compilation
- `go test -coverprofile=/tmp/airplan-cover.out ./...`

Measured statement coverage:

- Core library: **88.7%**
- CLI: **83.9%**
- Whole repository: **86.1%**

## Findings resolved in this hardening series

### Ownership and remote lifecycle

- Upload writes `.airplan.json` first, then source/page payloads. Interrupted
  uploads remain visibly owned and inspect as incomplete.
- Remote list uses LIST only, includes exact marker directories, derives total
  occupancy and an unambiguous HTML slug, and hides markerless directories.
- `show` fetches and validates the marker and reports complete, incomplete, or
  invalid state plus the full page URL.
- Delete requires a valid marker before mutation, accepts only the directory or
  marker-declared direct children, deletes payloads first, and deletes the
  marker last.
- Remote purge inspects markers before deletion, skips invalid markers, handles
  incomplete uploads, and retains marker-last retry semantics.
- Path-style URLs must contain the configured bucket as an exact segment.
- Local purge skips records belonging to another bucket or key prefix instead
  of wedging after configuration changes.
- The narrow marker-missing reconciliation path requires an exact current local
  record and writes the correct page-key tombstone.

### Timeouts, configuration, and file output

- Manifest lock acquisition is context-aware and cannot wait indefinitely.
- SPEC.md now defines operation/phase timeout boundaries. Preview applies the
  configured timeout; local purge starts its deletion budget after prompting;
  remote purge has separate discovery and deletion budgets.
- Explicitly selected missing `--config` and `AIRPLAN_CONFIG` paths are errors;
  only an absent platform-default file is optional.
- Bare-integer timeout multiplication rejects overflow.
- Confirmation EOF is an error with `--yes` guidance instead of a successful
  silent abort.
- `config schema` rejects trailing arguments.
- Preview output is fully written beside the destination and atomically
  replaced. Unix uses same-directory rename; Windows uses `ReplaceFileW`, with
  `MoveFileEx` for a new destination. Symlink and hardlink aliases of the input
  are rejected.

### Input, URL, and manifest boundaries

- Zero-byte input is explicitly rejected while whitespace-only input remains
  valid.
- `Input.MaxSize == math.MaxInt64` no longer overflows the one-byte size probe.
- `ParseSize` rejects undocumented unit tails such as `10ib`.
- Manifest readers ignore blank lines and reject records missing required time,
  key, URL, bucket, byte-count, or marker-version fields without hiding later
  valid records.
- Upload manifest-write and delete tombstone-write failures degrade to warnings
  after successful storage work, with retry/reconciliation behavior covered.
- Fallback URLs assembled without `public_base_url` carry the same warning in
  upload, show, and remote purge paths.
- HTML noindex detection uses parser tokenization plus original-byte offsets,
  so comments, raw-text elements, templates, malformed input, and out-of-head
  metadata cannot spoof the scanner or force document reserialization.

### Panic and public API hardening

- `PublicURL` returns an error for a nil configuration.
- Nil and zero-value `Client` receivers return `ErrUninitializedClient` from
  Upload, ListRemote, InspectUpload, and DeleteUpload.
- `New`, `RenderInput`, and all Client operations reject nil contexts.
- Public input-reader documentation explains the cancellation ownership rule.
- The only `template.Must` is package initialization over committed built-in
  source. Runtime type assertions are guarded or operate on nodes registered by
  the same renderer extension. No input-driven slice/index panic was found.

## High-value regression coverage now present

- Marker, source, and page PUT failures independently prove that failed upload
  calls return no result URL and write no successful manifest record.
- DeleteObjects per-object failures and batches larger than 1,000 keys prove
  retry-safe propagation and request batching.
- Preview timeout cancellation exercises a blocked reader.
- Root CLI tests exercise the default nonzero timeout path, a real file
  argument with filename-driven format/slug inference, and nonexistent files.
- Show covers complete, incomplete, invalid, missing-marker, human, JSON, and
  fallback-URL behavior.
- Remote purge covers age/slug filtering, incomplete and invalid markers,
  dry-run, confirmation abort, inspection failure, partial deletion failure,
  fallback warnings, and more candidates than the worker pool size.
- Rendering covers empty/whitespace input and mutually exclusive template
  sources; parsing covers strict sizes, timeout overflow, and fractional ages.
- Manifest tests cover structural validation, blank lines, lock cancellation,
  append failures, and recovery.
- Public API tests cover nil configs, nil contexts, nil receivers, and
  zero-value clients.

## Accepted constraint

Cancellation can stop waiting for an arbitrary `io.Reader`, but Go cannot
forcibly interrupt that reader. If it remains blocked, the single read
goroutine remains until the reader unblocks or is closed. The caller owns a
long-lived reader and must arrange that release. Closing arbitrary readers from
inside airplan would be unsafe because ownership and concurrent use are unknown;
the contract is now documented on `Input.Reader` and in the library guide.

This is negligible for the short-lived CLI and explicit for embedders. It is
not treated as an implementation defect.

## Residual validation limits

- The full lifecycle passed against MinIO, not a live Cloudflare R2 account.
  Provider-specific behavior remains release-smoke-test territory.
- The Windows atomic-replacement implementation was cross-compiled but not
  executed on a Windows host during this pass.
- Browser-launch platform branches, cryptographic-random-source failure,
  generated command entry points, and third-party renderer internals remain
  intentionally uncovered. They do not sit on the product's risky state or
  ownership boundaries.

## Conclusion

There are no remaining audit items recommended for implementation before the
next feature or release-hardening phase. Future changes should keep the marker
ownership invariant, marker-first upload/marker-last deletion ordering, stdout
URL contract, and operation timeout boundaries under direct regression tests.
