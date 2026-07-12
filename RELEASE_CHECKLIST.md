# airplan v0.1.0 Release Checklist

Status: **in progress** on `codex/pre-release-hardening`.

This is the working contract for the initial release. It replaces the obsolete
build-phase plan and covers every finding from the pre-release project review.
Do not merge the release-please v0.1.0 PR until all **must** items and final
evidence gates are complete.

## Working rules

- [SPEC.md](SPEC.md) remains authoritative. Update it before implementation for
  every observable behavior change, including its version and change summary.
- Keep the hardening work in one cohesive PR, split into reviewable commits.
- Add or update tests with each behavior change; do not leave verification to
  the final pass.
- Check off an item only when its implementation, tests, documentation, and
  required evidence are complete.
- Record deliberate deferrals in the decision log with a reason and follow-up
  trigger.

## Decisions

- [x] **One hardening PR:** complete the initial-release work together so the
      spec, implementation, tests, and release automation cannot drift across PRs.
- [x] **Trusted Markdown and HTML:** both input modes preserve authored active
      content. Airplan uploads the user's own documents, so the trust boundary is
      documented rather than enforced by sanitizing Markdown.
- [x] **Revocable cache behavior:** pages and uploaded sources default to
      `Cache-Control: no-store`; deletion and expiry matter more than caching for
      capability URLs.
- [x] **[GO-2026-5856](https://vuln.go.dev/ID/GO-2026-5856.json) accepted
      temporarily:** airplan reaches `crypto/tls` through the AWS SDK, but does not
      configure
      `tls.Config.EncryptedClientHelloConfigList`. The advisory only affects
      handshakes that actively use ECH. Keep Go 1.26.4 until 1.26.5 satisfies the
      minimum-age policy. Revisit if airplan adds custom TLS/ECH configuration or
      the advisory scope changes.
- [x] **Remove the obsolete build plan:** `PLAN.md` is deleted rather than kept
      as misleading historical documentation.
- [x] **Spec 0.6.0 for pre-release corrections:** while the spec is below 1.0,
      breaking corrections and compatible additions bump the minor version. Stable
      major-version semantics begin when the contract is deliberately declared 1.0.
- [x] **Use a 30-second default timeout:** the first real-R2 attempt was delayed
      by local Little Snitch approval rather than storage latency. Thirty seconds
      adds modest headroom without turning ordinary endpoint failures into
      minute-long waits; release smoke tests may still override it to 60 seconds.

## Must fix before v0.1.0

### Document safety and privacy

- [x] **SEC-01 — Define and verify the Markdown trust boundary.**
  - Keep Goldmark unsafe rendering enabled so Markdown preserves authored raw
    HTML and link destinations.
  - Preserve the exact original Markdown in source view and source downloads.
  - State clearly in the spec, README, and shipped skill that Markdown and
    explicit `--format html` input are trusted and may execute active content.
  - Add regression tests covering raw HTML, authored link destinations, source
    preservation, and explicit HTML input.
  - Browser-test trusted Markdown and verify its authored content is preserved.

- [x] **SEC-02 — Make deletion meaningful despite caches.**
  - Change page and source object headers from one-year public immutable caching
    to `Cache-Control: no-store`.
  - Update the spec, integration assertions, README privacy language, and any
    implementation comments that describe the old header.
  - Verify the header for HTML pages, rendered Markdown/text pages, and source
    siblings against MinIO and the real R2 custom domain.
  - Confirm a deleted R2 object is no longer retrievable through the public URL;
    note any provider cache behavior that remains outside airplan's control.

- [x] **SEC-03 — Preserve the Go vulnerability exception as evidence.**
  - Keep the official GO-2026-5856 advisory URL and ECH-only rationale in this
    file while Go 1.26.4 remains pinned.
  - Run `govulncheck` at the final gate and confirm this is the only reachable
    report; investigate any additional result independently.
  - Do not add a broad vulnerability-scanner suppression that could hide future
    findings.

### Destructive command safety

- [x] **DEL-01 — Do not tombstone an upload through the wrong profile.**
  - Before ensure-gone tombstoning, compare a matching manifest record's bucket
    and profile with the active connection.
  - Refuse with an actionable error, or safely resolve the recorded profile,
    when they differ. Do not hide an object that may still be live elsewhere.
  - Preserve ensure-gone convergence for genuinely externally deleted uploads.
  - Warn when an unreadable or incomplete manifest prevents a complete
    bucket/profile check before ensure-gone tombstoning.
  - Cover correct profile, wrong profile, different bucket, shared bucket,
    missing record, bare directory, page URL, source key, and disabled-manifest
    cases.

### Configuration and input correctness

- [x] **CFG-01 — Reject partial explicit credentials.**
  - If exactly one of access key ID and secret access key is configured, return
    a clear validation error instead of silently falling back to the ambient AWS
    credential chain.
  - Preserve the standard AWS chain only when neither explicit value is set.
  - Test file, profile, environment, and mixed-precedence combinations.

- [x] **CFG-02 — Validate URL-bearing configuration and key prefixes.**
  - Validate endpoint and public base URLs with actionable scheme/host errors.
  - Define whether key prefixes are restricted to URL-safe path segments or are
    percent-encoded when public URLs are assembled; specify and test the choice.
  - Cover public base URLs with path prefixes and trailing slashes.
  - Ensure delete URL parsing remains the inverse of public URL construction.
  - Add schema descriptions or constraints where they can match runtime
    validation exactly.

- [x] **INP-01 — Enforce the UTF-8 input contract.**
  - Reject invalid UTF-8 even when the first 8 KiB contains no NUL byte.
  - Keep the existing binary-input and size-limit error behavior.
  - Test Markdown, text, HTML, forced formats, invalid boundary sequences, and
    valid non-ASCII input.

### Installation and version identity

- [x] **REL-01 — Report the correct version outside GoReleaser builds.**
  - When the ldflag remains `dev`, derive the module version from
    `debug.ReadBuildInfo` so `go install github.com/jimeh/airplan@latest`
    reports the released version.
  - Keep the GoReleaser ldflag authoritative for packaged artifacts.
  - Define sensible output for local builds and dirty VCS builds.
  - Preserve module pseudo-versions without their leading `v`.
  - Test ldflag, module-version, devel, and unavailable-build-info cases.

## Should fix before v0.1.0

### CI and reproducibility

- [x] **CI-01 — Enforce repository formatting in CI.**
  - Add `mise run format:check` or an equivalent dedicated job to the normal PR
    workflow so Go and Markdown formatting cannot bypass hooks.

- [x] **CI-02 — Exercise platform behavior, not only cross-compilation.**
  - Run unit tests on Windows in CI, particularly config/state paths and browser
    launching behavior. Add macOS only if it provides distinct behavioral
    coverage worth its cost.
  - Keep the existing six-target cross-compile matrix.

- [x] **CI-03 — Pin the MinIO integration image.**
  - Replace `minio/minio:latest` with a reviewed immutable version or digest.
  - Document the refresh mechanism so the pin does not become forgotten.

### Manifest resilience

- [x] **MAN-01 — Keep malformed-line tolerance true for oversized lines.**
  - A manifest line over the scanner limit must produce a warning and allow
    later records to be read, or the documented behavior must explicitly state
    and justify a hard maximum.
  - Add a regression test with valid records after an oversized line.

### API and documentation polish

- [x] **DOC-01 — Remove stale `PLAN.md` and update repository guidance.**
- [x] **DOC-02 — Make the README library example handle errors.**
  - Show a compact, idiomatic example that cannot dereference a nil client or
    result after ignored errors.
- [x] **DOC-03 — Correct the public `Input.Format` documentation.**
  - List `md`, `html`, and `txt`, matching `ParseFormat` and the spec.
- [x] **DOC-04 — Add a concise security policy.**
  - Create `SECURITY.md` with supported-release expectations and a private
    reporting route, preferably GitHub Security Advisories.
- [x] **DOC-05 — Reconcile all release-facing claims.**
  - Recheck README, shipped skill, implementation guide, schema descriptions,
    CLI help, code comments, and spec references after behavior changes.
  - Ensure the cache, executable-HTML trust boundary, cleanup semantics, Go
    installation version, and supported platforms are described consistently.

### Supply-chain evidence

- [x] **SUPPLY-01 — Publish artifact attestations and SPDX SBOMs.**
  - Generate SPDX JSON SBOMs for release archives with GoReleaser and Syft.
  - Use the current `actions/attest` action to attest every checksum-listed
    release artifact with signed SLSA build provenance.
  - Pin the action and Syft version using the repository's existing tooling.
  - Document `gh attestation verify` alongside checksum verification.

## Final evidence gates

### Repository and automated checks

- [x] `mise run check` passes from a clean worktree.
- [x] `mise run verify` passes, including the MinIO round trip and GoReleaser
      configuration validation.
- [x] `go test -race ./...` passes.
- [x] `go mod verify` passes.
- [x] Coverage is reviewed for meaningful regressions from the pre-hardening
      baseline: library 88.1%, CLI 74.9%, overall 82.7%.
- [x] `govulncheck ./...` has no unexplained reachable findings; the expected
      GO-2026-5856 ECH-only exception is revalidated if Go remains at 1.26.4.
- [x] `goreleaser release --snapshot --clean --skip=publish` succeeds.
- [x] Snapshot archives contain the binary, license, README, spec, and generated
      schema for every intended platform.
- [ ] Version output is checked from a snapshot artifact and a module-versioned
      Go installation path.

### Browser and real-storage checks

- [x] Rendered Markdown is checked in light and dark modes, desktop and narrow
      viewports, including source toggle, copy controls, code blocks, alerts,
      tables, and responsive table of contents.
- [x] Browser console and page errors are empty during the rendered-page checks.
- [x] Trusted Markdown preserves authored raw HTML and link destinations.
- [x] Explicit HTML input remains byte-preserving apart from documented noindex
      injection.
- [x] Real R2 smoke test covers upload, public GET, metadata title, list remote,
      delete, ensure-gone behavior, and the final cache headers.
- [x] Real R2 smoke-test objects are removed after verification.

### Evidence recorded 2026-07-10

- `mise run check` and `mise run verify` passed; the latter used the immutable
  MinIO image and validated GoReleaser configuration.
- Race tests and module verification passed. Coverage increased to 88.4% for
  the library, 75.2% for the CLI, and 83.2% overall.
- `govulncheck` reported only GO-2026-5856 in Go 1.26.4. airplan does not
  configure ECH, so the accepted exception remains narrowly applicable.
- The snapshot produced six platform archives, six SPDX 2.3 JSON SBOMs, and a
  checksum file covering both. The inspected archive contained the binary,
  license, README, spec, and schema; its binary reported the snapshot version.
- Real-browser QA covered light/dark desktop and narrow layouts, reduced motion,
  source/rendered toggling, copy controls, alerts, tables, code, responsive ToC,
  trusted Markdown, and trusted HTML. A focused follow-up confirmed Markdown's
  authored script executed and its raw HTML marker and `javascript:` link were
  preserved. Product console/page errors were empty.
- Real R2 checks returned `200` and `Cache-Control: no-store` for Markdown,
  HTML, text, and both source-object types. Page metadata appeared in remote
  listing; page/source URLs returned `404` after deletion; ensure-gone
  converged. Remote listing confirmed zero remaining smoke objects, including
  cleanup after the initial attempt was delayed by local firewall approval.

### Release automation and publication

- [ ] Hardening PR CI is fully green; apply `ci:goreleaser` if release packaging
      changes require the opt-in PR check.
- [ ] The release-please v0.1.0 PR regenerates after the hardening PR merges.
- [ ] Generated changelog accurately reflects all user-visible fixes without
      internal checklist noise.
- [ ] Release workflow credentials and Homebrew tap access are confirmed without
      exposing secret values.
- [ ] GitHub release immutability is enabled before merging the release PR; it
      only applies to releases published after the setting is enabled.
- [ ] Merging the release PR creates a draft, uploads and verifies checksums,
      platform archives, SBOMs, and the standalone schema asset, records
      attestations, updates the Homebrew cask, then publishes immutable release
      `v0.1.0` and its tag.
- [ ] Fresh-install smoke tests pass through Homebrew, mise, Go, and a downloaded
      release archive.
- [ ] Published `airplan --version`, schema URL, release badge, and Go package
      documentation resolve to v0.1.0.

## Post-release cleanup

- [ ] Remove this temporary checklist after v0.1.0 is verified, carrying any
      deliberate deferrals into durable issues or repository documentation first.
