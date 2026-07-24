# Multi-Platform Container Publishing Plan

Status: implemented
Scope: official `linux/amd64` and `linux/arm64` server image on GHCR
Repository baseline: Airplan v0.5.0, spec 0.28.1

## 1. Goal

Publish an official container image for `airplan serve` at:

```text
ghcr.io/jimeh/airplan
```

The image must:

- contain the exact Linux binaries already built by GoReleaser;
- publish `linux/amd64` and `linux/arm64` as one OCI image index;
- avoid compiling or running target-platform code inside Docker;
- require no QEMU setup;
- run as a non-root user with a minimal runtime filesystem;
- preserve Airplan's single-instance and persistent-manifest constraints;
- include an SBOM and GitHub build-provenance attestation;
- publish only after the GitHub release is immutable and verified; and
- be independently retryable without silently replacing an existing version.

The implementation should fit the existing release design rather than create a
second release system. GoReleaser remains the source of truth for binaries,
archives, version stamping, checksums, and Homebrew generation. Buildx only
packages the already-built Linux executables into an image index.

## 2. Settled design

### 2.1 Use a downstream Buildx publication job

Add a `publish-container` job to the reusable release workflow. It depends on:

```text
build-release ─┐
               ├─> publish ─> publish-container
verify-macos ──┘
```

`build-release` prepares and preserves a small container build context.
`publish-container` downloads that context after `publish` has made and
verified the immutable GitHub release.

This mirrors the existing downstream Homebrew Cask publication model:

- the immutable GitHub release remains the primary release boundary;
- GHCR publication is independently rerunnable;
- a container failure cannot modify the immutable release;
- no release-version container tag appears before release verification; and
- a durable issue reports a failure that occurs after publication.

The accepted tradeoff is brief eventual consistency: the GitHub release can be
public while the downstream container job is still running. This is preferable
to exposing an unverified version tag before the macOS and immutable-release
gates finish.

### 2.2 Reuse GoReleaser binaries

Do not compile Go code in the Dockerfile. The existing GoReleaser build already
produces:

```text
dist/airplan_linux_amd64_v1/airplan
dist/airplan_linux_arm64_v8.0/airplan
```

Those directory names are implementation details and must not be embedded in
the Dockerfile. A preparation command reads `dist/artifacts.json`, selects
exactly one `Binary` artifact for each supported Linux architecture, validates
it, and creates a stable Buildx context:

```text
dist/container/
├── Dockerfile
├── linux/
│   ├── amd64/
│   │   └── airplan
│   └── arm64/
│       └── airplan
└── state/
    └── .keep
```

GoReleaser's artifact metadata, rather than path-name reconstruction, remains
the source of truth for locating binaries. This avoids coupling the container
workflow to `goamd64`, `goarm64`, or future GoReleaser naming changes.

### 2.3 Build one multi-platform image index without emulation

The release Dockerfile contains no target-platform `RUN` instruction:

```dockerfile
FROM gcr.io/distroless/static-debian13:nonroot@sha256:<pinned-index-digest>

ARG TARGETPLATFORM

COPY --chmod=0555 \
  $TARGETPLATFORM/airplan \
  /usr/local/bin/airplan
COPY --chown=65532:65532 --chmod=0700 \
  state/ \
  /var/lib/airplan/

ENV XDG_CONFIG_HOME=/etc \
    AIRPLAN_MANIFEST=/var/lib/airplan/manifest.jsonl \
    AIRPLAN_SERVER_HOST=0.0.0.0 \
    AIRPLAN_SERVER_PORT=8080 \
    AIRPLAN_SERVER_ALLOW_NON_LOOPBACK=true

VOLUME ["/var/lib/airplan"]
EXPOSE 8080
STOPSIGNAL SIGTERM

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/airplan"]
CMD ["serve"]
```

The exact base digest is selected and verified during implementation. Keep the
human-readable distroless tag beside its digest so dependency automation can
identify the source release. Use the current Debian 13 static non-root variant;
there is no need to prefer Debian 12 solely for its longer history.

Buildx expands `TARGETPLATFORM` separately for `linux/amd64` and
`linux/arm64`. It selects the corresponding distroless base manifest and copies
the matching static binary. It does not execute either binary, a target shell,
or a target package manager. Therefore the build needs Buildx but not QEMU.

The final push stores one image index with one runnable manifest for each
platform. Docker and other OCI clients select the appropriate manifest during
pull.

### 2.4 Make server configuration environment-aware

Do not rely on shell expansion in `CMD`. Exec-form Docker commands intentionally
do not interpolate environment variables, and the distroless image has no
shell. Add server-specific environment fallbacks in the Airplan process so the
image can keep the simple default:

```text
airplan serve
```

Add this configuration contract:

| Concern                | CLI flag                  | Environment fallback                         | Native default      | Image default     |
| ---------------------- | ------------------------- | -------------------------------------------- | ------------------- | ----------------- |
| Bind host and port     | `--listen ADDR`           | `AIRPLAN_SERVER_HOST`, `AIRPLAN_SERVER_PORT` | `127.0.0.1`, `8080` | `0.0.0.0`, `8080` |
| Non-loopback consent   | `--allow-non-loopback`    | `AIRPLAN_SERVER_ALLOW_NON_LOOPBACK`          | `false`             | `true`            |
| Bearer token file      | `--token-file PATH`       | `AIRPLAN_SERVER_TOKEN_FILE`                  | unset               | unset             |
| Hosted MCP origins     | `--allowed-origin ORIGIN` | `AIRPLAN_SERVER_ALLOWED_ORIGINS`             | unset               | unset             |
| Upload spool directory | `--temp-dir PATH`         | `AIRPLAN_SERVER_TEMP_DIR`                    | system default      | system default    |
| Server log level       | `--log-level LEVEL`       | `AIRPLAN_SERVER_LOG_LEVEL`                   | `info`              | `info`            |

Keep `AIRPLAN_SERVER_TOKEN` as the token-value source. Resolve
`--token-file` before `AIRPLAN_SERVER_TOKEN_FILE`, then require exactly one of
the resolved token-file source and `AIRPLAN_SERVER_TOKEN`. Do not add a default
token-file path because it would conflict with environment-only secrets and
turn an absent mount into an unexpected startup failure.

The resolution rules are:

1. An explicitly supplied flag wins over its environment fallback.
2. An explicit `--listen` wins over both host and port environment variables.
3. Otherwise, resolve host and port independently and combine them with
   `net.JoinHostPort`, including IPv6 support.
4. Explicit `--allow-non-loopback=false` wins over an image environment value
   of `true`.
5. Parse server booleans and ports strictly and fail with a configuration error
   before storage initialization or listener creation. Accept decimal ports
   from `0` through `65535`; port zero retains the CLI's useful ephemeral-port
   behavior.
6. Parse `AIRPLAN_SERVER_ALLOWED_ORIGINS` as a comma-separated list, trim
   surrounding whitespace, and reject empty entries. An explicit
   `--allowed-origin` list replaces rather than appends to the environment
   list.

Do not add generic `HOST` or `PORT` fallbacks. Namespaced variables avoid
collisions with orchestrators and unrelated processes. A platform-specific
`PORT` alias can be added later if a concrete hosting environment requires it.

This makes `docker run ghcr.io/jimeh/airplan:<tag>` a server image while
retaining the normal Airplan entrypoint. Operators can replace the command to
run `--version`, configuration inspection, stdio MCP, or another CLI command.

### 2.5 Treat configuration and state as first-class interfaces

Support two equally documented configuration modes.

#### Environment-only configuration

The existing Airplan configuration variables provide S3 or remote API
configuration without a file. The image must not set `AIRPLAN_CONFIG`: an
explicit `AIRPLAN_CONFIG` path is required to exist, which would make an
environment-only deployment fail when no file is mounted.

Set:

```text
XDG_CONFIG_HOME=/etc
```

This changes the optional default config path to
`/etc/airplan/config.toml`. When the file is mounted it is discovered
automatically; when it is absent, environment-only configuration continues to
work.

Document the existing configuration variables relevant to container use,
including:

```text
AIRPLAN_BACKEND
AIRPLAN_API_URL
AIRPLAN_API_TOKEN
AIRPLAN_ENDPOINT
AIRPLAN_BUCKET
AIRPLAN_REGION
AIRPLAN_ACCESS_KEY_ID
AIRPLAN_SECRET_ACCESS_KEY
AIRPLAN_PUBLIC_BASE_URL
AIRPLAN_KEY_PREFIX
AIRPLAN_TIMEOUT
```

The standard AWS credential chain remains supported. Examples should favor
runtime secret injection or secret files rather than literal secrets in a
Compose file or shell history.

`AIRPLAN_PROFILE` selects a record from a config file and therefore is not an
environment-only storage substitute. Document it with the mounted-file mode.

#### File-based configuration

Mount a config file read-only at:

```text
/etc/airplan/config.toml
```

Select a named profile with `AIRPLAN_PROFILE` when needed. Advanced deployments
may mount elsewhere and set `AIRPLAN_CONFIG` explicitly, with the existing
rule that an explicitly selected file must exist.

Do not declare `/etc/airplan` as a Docker volume. Configuration is immutable
operator input, not application-managed state.

#### Persistent manifest state

Set:

```text
AIRPLAN_MANIFEST=/var/lib/airplan/manifest.jsonl
```

and declare:

```dockerfile
VOLUME ["/var/lib/airplan"]
```

The image must create `/var/lib/airplan` with numeric ownership
`65532:65532` before the volume declaration. The explicit manifest variable
makes the state location visible and independent of the runtime's home
directory.

The Dockerfile declaration documents the state boundary but does not provide a
durable lifecycle policy. Documentation must recommend an explicitly named
volume:

```text
--mount type=volume,source=airplan-data,target=/var/lib/airplan
```

An anonymous volume can become orphaned or lose its association when a
container is replaced. Bind mounts are also supported, but their host
directory must be writable by UID/GID `65532:65532`.

State guidance must also say:

- run only one Airplan server against a manifest volume;
- back up the volume if upload history matters;
- `airplan sync` can reconstruct currently discoverable remote uploads after
  manifest loss but may not recover all historical or local metadata; and
- do not automatically run `sync` during container startup because it can hide
  a missing or incorrectly mounted state volume.

Deployments must additionally provide a bearer token through
`AIRPLAN_SERVER_TOKEN`, `AIRPLAN_SERVER_TOKEN_FILE`, or `--token-file`, and a
trusted reverse proxy for non-loopback TLS termination.

Do not bake credentials, tokens, configuration, or a default token-file path
into the image.

Do not declare an in-image `HEALTHCHECK`. The distroless runtime intentionally
has no `curl`, shell, or similar probe tool. Docker Compose, Kubernetes, and
other orchestrators should perform an HTTP probe against `/healthz`.

`EXPOSE 8080` is static image metadata: it neither publishes the port nor
tracks `AIRPLAN_SERVER_PORT`. When an operator changes the server port, the
published port mapping, reverse-proxy target, and external health probe must
change with it.

## 3. Image and tag contract

### 3.1 Image identity

Use:

```text
ghcr.io/jimeh/airplan
```

Set OCI metadata on both platform images and, where supported, the index:

```text
org.opencontainers.image.title=airplan
org.opencontainers.image.description=<short server-oriented description>
org.opencontainers.image.url=https://github.com/jimeh/airplan
org.opencontainers.image.source=https://github.com/jimeh/airplan
org.opencontainers.image.documentation=https://github.com/jimeh/airplan
org.opencontainers.image.licenses=MIT
org.opencontainers.image.revision=<full release SHA>
org.opencontainers.image.version=<version without leading v>
```

The `source` label links the GHCR package to the repository and helps the
repository-scoped `GITHUB_TOKEN` retain package access.

### 3.2 Tags

Publish only these user-facing container tags:

```text
<major>.<minor>.<patch>
latest
```

Rules:

1. Derive the exact container version from the validated GitHub release tag by
   removing its single leading `v`; GitHub release `v0.5.1` becomes container
   tag `0.5.1`.
2. Do not publish a `v`-prefixed container tag, a commit-SHA tag, or any other
   staging tag.
3. Do not publish floating `<major>` or `<major>.<minor>` tags while Airplan is
   pre-1.0. A minor release may still be intentionally breaking.
4. Update `latest` only if the GitHub API reports that this tag is still the
   repository's latest release. Rerunning an old release must not move
   `latest` backwards.
5. Consumers requiring immutability should pin the image digest. GHCR version
   tags are operationally protected by the workflow but are not registry-level
   immutable.

### 3.3 Push by digest, verify, then tag

When the exact version tag does not already exist, configure Buildx to push the
index canonically by digest without assigning a tag:

```text
ghcr.io/jimeh/airplan@sha256:<index-digest>
```

Use BuildKit's `push-by-digest=true` and `name-canonical=true` image exporter
options. This keeps the GHCR tag set limited to the two agreed forms while
making the pushed index available for registry, runtime, SBOM, and attestation
verification.

`docker/build-push-action` returns the image-index digest. The job verifies and
attests that digest before assigning the exact container version:

```text
ghcr.io/jimeh/airplan@sha256:<index-digest>
  ├─> :<version>
  └─> :latest, when this is still the latest GitHub release
```

Use `docker buildx imagetools create` or the equivalent digest-preserving
registry operation for tagging.

Before assigning the exact-version tag:

- resolve the existing tag, if any;
- accept it only if it already points to the verified digest; and
- fail closed if it points anywhere else.

Assign `latest` only after the exact-version tag re-resolves to the verified
digest. A failed attempt before exact-version tagging may leave an untagged
manifest in GHCR, but it cannot expose a partial or conflicting release tag.

The per-tag release concurrency group serializes whole attempts for one
release. A second, package-scoped `publish-container` concurrency group with
`queue: max` serializes all workflow-owned GHCR mutations across versions
without dropping pending publications. The explicit checks also protect manual
retries and unexpected pre-existing tags.

These protections govern workflow-owned mutations. GHCR does not provide
registry-enforced tag immutability or a documented conditional tag
compare-and-swap operation, so external writers retain a tag race. Consumers
requiring immutability must pin the image-index digest.

## 4. Build-context preparation

### 4.1 Add a small preparation command

Add an internal Go command:

```text
internal/cmd/preparecontainer
```

Proposed interface:

```text
go run ./internal/cmd/preparecontainer \
  --artifacts dist/artifacts.json \
  --dockerfile Dockerfile.release \
  --output dist/container
```

Responsibilities:

1. Parse GoReleaser's `artifacts.json`.
2. Select `Binary` artifacts for build ID `airplan`.
3. Require exactly one `linux/amd64` and one `linux/arm64` artifact.
4. Reject missing, duplicate, empty, non-regular, or unexpected artifacts.
5. Read each ELF header and require:
   - `EM_X86_64` for `linux/amd64`; and
   - `EM_AARCH64` for `linux/arm64`.
6. Copy binaries to `$output/linux/<arch>/airplan`.
7. Copy `Dockerfile.release` to `$output/Dockerfile`.
8. Create `$output/state/.keep` for the owned persistent-state directory.
9. Ensure the generated context contains no other files.
10. Write into a fresh temporary directory and rename it into place only after
    every validation succeeds.

Keep the command narrow. It prepares a deterministic context; it does not
invoke Docker, inspect credentials, push images, or infer a release version.

### 4.2 Test the preparation command

Unit tests cover:

- successful `amd64` and `arm64` selection;
- artifact ordering independence;
- missing architecture;
- duplicate architecture;
- wrong build ID;
- wrong OS;
- empty or missing binary;
- symlink or non-regular input;
- mismatched ELF machine type;
- stale output replacement without partial results; and
- final context paths and file modes.

Tests should construct minimal ELF fixtures rather than cross-compile full
Airplan binaries.

### 4.3 Prove archive and container bytes match

Before uploading the context artifact, compare each staged binary byte-for-byte
with the `airplan` executable inside its matching GoReleaser Linux archive.

This proves that the container input is not merely another build from the same
commit: it is the exact executable shipped in the corresponding release
archive.

Fail if:

- the matching archive cannot be identified uniquely from `artifacts.json`;
- the archive does not contain exactly one root `airplan` executable; or
- the extracted and staged binary SHA-256 digests differ.

## 5. Release workflow changes

### 5.1 Preserve the prepared context in `build-release`

After GoReleaser and existing draft-asset verification:

1. Run `preparecontainer`.
2. Compare staged binaries with their release archives.
3. Name the workflow artifact:

   ```text
   container-context-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}
   ```

4. Upload `dist/container/` with:
   - missing files treated as an error;
   - a seven-day retention period; and
   - hidden-file upload enabled because the exact, prevalidated context contains
     the intentional `state/.keep`.
5. Expose the artifact name as a `build-release` job output.

This artifact is an internal handoff, not a GitHub release asset. It contains
only the Dockerfile, two public release binaries, and an empty state-directory
placeholder.

### 5.2 Add `publish-container`

Add a job with:

```text
needs:
  - build-release
  - publish
```

Grant only:

```yaml
permissions:
  contents: read
  packages: write
  id-token: write
  attestations: write
  artifact-metadata: write
  issues: write
```

Do not grant `contents: write`. The job reads the immutable release and writes
only the repository's GHCR package, attestations, linked-artifact metadata, and
failure issue.

The `publish` reusable-workflow call in
`.github/workflows/release-please.yml` must also grant `packages: write`.
Reusable workflows can narrow caller permissions but cannot elevate them. Keep
the caller's existing `contents`, `id-token`, `attestations`,
`artifact-metadata`, and `issues` permissions aligned with the called jobs.

Steps:

1. Download the exact context artifact named by `build-release`.
2. Validate the published release still matches `RELEASE_TAG` and
   `RELEASE_SHA`.
3. Set up Buildx.
4. Log in to `ghcr.io` with the job's `GITHUB_TOKEN`.
5. Derive the unprefixed container version from the validated release tag.
6. Inspect the exact-version tag.
7. Reuse its digest only if it already passes release identity, platform,
   exact release-binary, runtime, and signer-constrained attestation checks;
   fail closed if it conflicts.
8. If it does not exist, build and push the index canonically by digest without
   assigning a tag.
9. For a newly built digest, generate and push the GitHub provenance
   attestation. Reuse an existing digest only after verifying its current
   attestation; do not create a duplicate.
10. Verify platforms, configuration, labels, version, server startup, and
    attestation.
11. Assign the exact-version tag to the verified digest and re-resolve it.
12. Update `latest` only when the GitHub latest-release check allows it.
13. Re-resolve every assigned tag and require the expected digest.

Pin every third-party action by full commit SHA. Run `pinact` after choosing
current action versions.

### 5.3 Buildx configuration

Configure `docker/build-push-action` with:

```text
context: <downloaded generated context>
file: <context>/Dockerfile
platforms: linux/amd64,linux/arm64
outputs: type=image,name=ghcr.io/jimeh/airplan,push-by-digest=true,name-canonical=true,push=true
sbom: true
provenance: false
no-cache: true
pull: true
```

Do not provide a `tags` input for a new build. Confirm during implementation
that the pinned Buildx/BuildKit versions return the top-level image-index
digest for this multi-platform exporter configuration. The workflow must fail
if it receives a child-manifest digest or if a tag appears implicitly.

Set `provenance: false` on Buildx deliberately. GitHub's explicit
`actions/attest` step creates the one authoritative provenance attestation,
using the digest returned by `build-push-action`. This avoids two independent
provenance records with different policies. Buildx still attaches the image
SBOM.

Do not add `docker/setup-qemu-action`. A future Dockerfile change that needs
target-platform execution must be an explicit architectural decision, not
something CI silently enables.

Do not restore or save a shared BuildKit cache in production release jobs.
Release inputs are small, the binaries are already built, and avoiding shared
cache input matches the existing release workflow's cache-poisoning posture.

### 5.4 Retry behavior

A failed-job rerun must be safe at every boundary:

- If no exact-version tag exists, build and push canonically by digest without
  assigning a tag.
- A retry may create a new untagged digest if the earlier attempt stopped before
  version tagging; it verifies the new digest normally.
- If the exact-version tag exists and fully validates, reuse its digest without
  creating a duplicate attestation.
- If the exact-version tag has the wrong revision, version, platforms, runtime
  configuration, or attestation, fail without overwriting it.
- `latest` may move only to the verified digest of the current latest GitHub
  release.
- Re-running an old release never changes `latest`.
- Attestation verification is repeatable for the same digest. Generate a new
  attestation only for a newly built digest.

Keep the current `release-${{ inputs.tag }}` concurrency group for complete
attempts. Also give `publish-container` a constant repository/package-scoped
concurrency group with `queue: max`. This serializes all workflow-owned GHCR
mutations across release versions without dropping pending jobs.

### 5.5 Durable post-publication failure signal

If `publish-container` fails, create an issue containing:

- the release tag;
- the expected release SHA;
- the workflow run URL;
- whether an untagged digest or exact-version tag was created;
- which build, verification, tagging, or attestation phase failed; and
- instructions to rerun failed jobs after fixing the workflow or external
  package setting.

Never include credentials, tokens, registry responses containing auth data, or
the Airplan server configuration.

Avoid duplicate issues by searching for the exact title before creation:

```text
Container publication failed for <tag>
```

## 6. Image verification

### 6.1 Verify the index structure

Inspect the pushed digest with:

```text
docker buildx imagetools inspect \
  ghcr.io/jimeh/airplan@sha256:<digest>
```

Require exactly these runnable platforms:

```text
linux/amd64
linux/arm64
```

SBOM and provenance attachments can appear in raw OCI index output as
`unknown/unknown` attestation manifests. Filter those by their attestation
annotations before comparing runnable platforms. Do not assert that the index
contains exactly two descriptors.

Reject:

- a single-platform manifest instead of an index;
- a missing required platform;
- any additional runnable platform;
- duplicate runnable descriptors for one platform; or
- a malformed or empty top-level digest.

### 6.2 Verify the exact release binaries

For each `linux/amd64` and `linux/arm64` child image:

1. create, but do not run, a container with
   `docker create --platform linux/<architecture>`;
2. copy `/usr/local/bin/airplan` out with `docker cp`; and
3. compare it byte-for-byte with
   `dist/container/linux/<architecture>/airplan`.

Perform this check before accepting either a newly built digest or an existing
exact-version tag. Container creation and file extraction do not require QEMU.
Always remove temporary containers and copied files on success or failure.

### 6.3 Verify the native image

On the `linux/amd64` release runner, pull by digest and verify:

1. `airplan --version` reports the release version.
2. Image user is numeric non-root `65532:65532`.
3. Entrypoint is `/usr/local/bin/airplan`.
4. Default command is exactly `serve`, without a baked address or port.
5. OCI revision and version labels match the release SHA and version.
6. `/tmp` exists and is writable by the runtime user.
7. `/var/lib/airplan` exists and is writable by the runtime user.
8. `/var/lib/airplan` is declared as a volume and
   `AIRPLAN_MANIFEST` points inside it.
9. `XDG_CONFIG_HOME=/etc` is set and `AIRPLAN_CONFIG` is not set.
10. The documented server host, port, and non-loopback environment defaults
    are present.
11. The image contains no shell or package manager.

Pulling by top-level digest lets Docker select the native `amd64` child while
keeping every check tied to the exact verified index.

### 6.4 Verify server operation

Run a release smoke test with:

- the repository's immutable-pinned MinIO image;
- a private Docker network;
- an isolated test bucket;
- a generated 32-byte-or-longer bearer token;
- environment-only S3 configuration for one run;
- a generated S3 config mounted read-only for a second run;
- a temporary persistent-state volume; and
- the Airplan image selected by digest.

Verify:

1. storage readiness succeeds before the listener appears;
2. `/healthz` returns success without authentication;
3. an authenticated capability request succeeds;
4. one small upload succeeds and is present in the manifest;
5. SIGTERM produces a bounded graceful shutdown; and
6. replacing the container while retaining the same named volume preserves the
   manifest record;
7. the optional `/etc/airplan/config.toml` path works without
   `AIRPLAN_CONFIG`;
8. an environment-only deployment starts with no config file mounted;
9. `AIRPLAN_SERVER_PORT` changes the listener and the matching external probe
   succeeds; and
10. the server still rejects a non-loopback listener when the environment
    acknowledgement is explicitly disabled.

Do not print the token or S3 credentials. Use a test-only public base URL and
delete the throwaway containers, network, and volume after the test.

### 6.5 Verify provenance

For a newly built digest, generate the attestation with:

```yaml
with:
  subject-name: ghcr.io/jimeh/airplan
  subject-digest: ${{ steps.build.outputs.digest }}
  push-to-registry: true
```

Then verify:

```text
gh attestation verify \
  oci://ghcr.io/jimeh/airplan@sha256:<digest> \
  --repo jimeh/airplan \
  --signer-workflow jimeh/airplan/.github/workflows/release.yml
```

Apply the signer-workflow constraint before reusing an existing exact-version
tag. For a newly built digest, apply it after creating the release attestation.

Documentation should show digest-based verification and pulling, while still
presenting the exact unprefixed container version tag as the convenient
default.

## 7. Pull-request and local checks

### 7.1 Add a container CI job

Add a Linux job to the existing CI workflow:

1. Install the locked Go and GoReleaser versions.
2. Run `goreleaser build --snapshot --clean`.
3. Run `preparecontainer`.
4. Build both target manifests without pushing.
5. Build and load the native `linux/amd64` image.
6. Run structural and non-root checks.
7. Run the MinIO server smoke test.

The multi-platform no-push build proves both context branches and both
distroless base manifests resolve. The native loaded image supplies runtime
coverage without QEMU.

Do not install QEMU in CI. This makes accidental target-platform `RUN`
instructions fail rather than become slow, hidden emulation.

### 7.2 Add server configuration tests

Add focused CLI tests for the new observable server contract:

- built-in native host, port, and non-loopback defaults;
- independent host and port environment resolution;
- explicit `--listen` overriding both environment values;
- explicit `--allow-non-loopback=false` overriding an image value of `true`;
- IPv4, hostname, and IPv6 host assembly;
- empty, malformed, above-65535, negative, and non-numeric port rejection,
  while retaining port-zero support;
- strict boolean parsing;
- `--token-file` overriding `AIRPLAN_SERVER_TOKEN_FILE`;
- conflict detection between the resolved token-file source and
  `AIRPLAN_SERVER_TOKEN`;
- comma-separated origin parsing, trimming, and empty-item rejection;
- explicit origin flags replacing the environment list;
- `AIRPLAN_SERVER_TEMP_DIR` and `AIRPLAN_SERVER_LOG_LEVEL` fallback and flag
  precedence; and
- configuration errors occurring before storage access or listener creation.

Keep these tests independent of Docker. The container smoke tests separately
prove that the Dockerfile defaults activate the same contract.

### 7.3 Add mise tasks

Add tasks along these lines:

| Task                         | Purpose                                     |
| ---------------------------- | ------------------------------------------- |
| `container:context`          | Build snapshot binaries and prepare context |
| `container:build`            | Build and load the native image             |
| `container:check`            | Build both platforms and run image checks   |
| `test:container-integration` | Run the MinIO-backed server container smoke |

Keep `mise run check` reasonably fast. It should cover deterministic
preparation tests and workflow linting. Put Docker-dependent image and server
checks in `mise run verify`, alongside the existing integration and release
snapshot checks.

Document that Docker is required for the container-specific tasks.

### 7.4 Update dependency automation

Add a Docker ecosystem entry to `.github/dependabot.yml` so the pinned
distroless base can receive delayed routine updates and immediate security
updates under the repository's existing dependency policy.

Pin Docker GitHub Actions by full SHA; the existing GitHub Actions Dependabot
entry and `pinact` cover action updates.

## 8. Documentation and specification changes

### 8.1 SPEC.md

Publishing an official image adds observable distribution behavior. Bump the
pre-1.0 spec minor version and define:

- the official GHCR image name;
- supported platforms;
- image version behavior;
- the unprefixed `X.Y.Z` and `latest` tag set, with no `v`-prefixed, SHA, major,
  or major/minor tags;
- default entrypoint and the exact `serve` command;
- server flag/environment/default precedence;
- host, port, non-loopback, token-file, allowed-origin, temporary-directory,
  and log-level environment variables;
- strict parsing and error behavior for new environment inputs;
- environment-only and mounted-file configuration;
- optional `/etc/airplan/config.toml` discovery without a forced
  `AIRPLAN_CONFIG`;
- the manifest path and `/var/lib/airplan` persistent-state boundary;
- non-root runtime;
- persistent single-instance manifest requirement;
- external TLS termination;
- unauthenticated `/healthz`;
- digest-pinning and attestation support; and
- exact-version versus mutable `latest` behavior.

Keep the specification implementation-neutral where possible. Dockerfile,
Buildx, distroless, action, and workflow details belong in IMPLEMENTATION.md.

Update IMPLEMENTATION.md's target spec version in the same change.

### 8.2 README.md

Add:

- a GHCR installation/run example;
- an environment-only `docker run` example;
- a file-configured `docker run` example;
- a minimal Compose example with a named state volume, read-only config mount,
  and secret file;
- a container configuration and precedence table;
- config, profile, token-file, state-volume, bind-host, port, health-probe, and
  reverse-proxy notes;
- the one-active-instance warning;
- bind-mount UID/GID requirements and state backup guidance;
- exact-tag and digest-pinned pull examples;
- image attestation verification; and
- supported platforms.

Do not place a bearer token directly on a command line in examples. Prefer a
mounted mode-0600 token file, with `AIRPLAN_SERVER_TOKEN` documented as the
alternative.

Do not manually change release-please's current-version marker block. Extend
the template inside that block only if container verification needs the current
release version substituted automatically.

The examples must distinguish `EXPOSE` from port publication. A custom
`AIRPLAN_SERVER_PORT` requires a matching Docker port mapping, reverse-proxy
target, and external health check.

### 8.3 IMPLEMENTATION.md and SECURITY.md

IMPLEMENTATION.md should explain:

- GoReleaser binary reuse;
- the generated `$TARGETPLATFORM` context;
- why the build needs no QEMU;
- why the executable, rather than a shell entrypoint, resolves environment
  defaults;
- optional config discovery through `XDG_CONFIG_HOME`;
- explicit manifest state under the declared volume;
- context artifact handoff;
- build/verify/tag ordering;
- tagless canonical digest publication before release tagging;
- GHCR tag retry protections;
- SBOM and provenance attachment; and
- downstream failure reporting.

SECURITY.md should clarify:

- the image contains no credentials or token;
- it runs as non-root;
- state, config, and secret mount expectations;
- the risk of losing an anonymous volume association;
- numeric ownership requirements for bind-mounted state;
- digest pinning for immutable deployments;
- reverse-proxy TLS and network exposure; and
- the same static-token and active-content trust boundaries as native
  `airplan serve`.

### 8.4 AGENTS.md

Add only durable operational rules:

- container binaries come from GoReleaser, never an in-Docker Go build;
- no target-platform `RUN` instructions or QEMU;
- `CMD` remains shell-free and contains no baked listen address;
- `/etc/airplan/config.toml` is an optional default, not a forced explicit
  config path;
- the runtime state directory, declared volume, and single-replica constraint;
- exact container version tags must never be overwritten with another digest;
  and
- container publication remains downstream of immutable release verification.

Add new mise tasks to the task-surface table.

## 9. First-publication rollout

GHCR creates a package as private by default. The first release needs an
explicit operator checklist. The one-time manual visibility change is an
accepted rollout step:

1. Confirm the workflow-created package is linked to `jimeh/airplan`.
2. Change package visibility to public in GitHub package settings.
3. Confirm anonymous pull of the exact unprefixed container version tag.
4. Confirm anonymous digest pull.
5. Verify the GitHub attestation without repository write credentials.
6. Run the image on both an amd64 host and a real arm64 host.
7. Confirm the package page shows source, description, license, and version
   metadata.
8. Confirm the README commands work from a clean machine.

If GitHub adds a stable, appropriately scoped API for package visibility,
automation can be considered later. Do not introduce a broad PAT solely to
automate this one-time setting.

## 10. Implementation sequence

### Phase 1: Deterministic build input

1. Add `Dockerfile.release`.
2. Add `internal/cmd/preparecontainer` and focused tests.
3. Add local mise tasks.
4. Build native and multi-platform no-push images.
5. Prove there are no target-platform `RUN` instructions or QEMU dependency.

Exit criteria:

- both platform contexts are selected from `artifacts.json`;
- both ELF architectures validate;
- the native image runs non-root; and
- the multi-platform build succeeds without QEMU installed.

### Phase 2: Container behavior

1. Add server environment fallbacks and focused precedence and validation
   tests.
2. Add the MinIO-backed container smoke test.
3. Verify environment-only and mounted-file configuration, token sources,
   custom bind settings, state, health, upload, replacement, and SIGTERM
   behavior.
4. Validate the distroless CA bundle against the MinIO TLS mode used by the
   integration environment, or separately against an HTTPS S3 endpoint.
5. Add Docker dependency automation.

Exit criteria:

- the image can run the complete minimal server lifecycle;
- server flags override environment values deterministically;
- environment-only configuration needs no placeholder config file;
- a config mounted at `/etc/airplan/config.toml` is auto-discovered;
- a custom port is reflected in the actual listener and external probe;
- persistent state survives container replacement through a named volume; and
- credentials and token never enter the image or logs.

### Phase 3: Release handoff and publication

1. Prepare and upload the context artifact from `build-release`.
2. Grant package publication permission at the reusable-workflow caller.
3. Add `publish-container`.
4. Push the multi-platform index canonically by digest without a tag.
5. Add index, runtime, server, SBOM, and provenance verification.
6. Add digest-based exact-version and conditional `latest` tagging with retry
   guards.
7. Add durable failure issue reporting.

Exit criteria:

- no exact version tag exists before all image checks pass;
- the only container tags are unprefixed `X.Y.Z` versions and `latest`;
- a rerun reuses a valid exact-version digest or safely verifies a new untagged
  digest;
- a conflicting tag fails closed;
- an old release rerun cannot move `latest`; and
- assigned tags resolve to the attested digest.

### Phase 4: Contract and operator documentation

1. Update SPEC.md and its version.
2. Align IMPLEMENTATION.md.
3. Update README.md, SECURITY.md, and AGENTS.md.
4. Add the first-publication operator checklist.
5. Run the complete verification set.

## 11. Verification strategy

Required automated commands:

```text
mise run check
mise run check:spec-sync
mise run container:check
mise run test:container-integration
mise run audit:deps
mise run release:snapshot
mise run verify
```

Required release-workflow validation:

```text
actionlint
zizmor --offline .github/workflows
pinact run --check
```

Required manual or real-registry validation before declaring the feature done:

```text
docker buildx imagetools inspect ghcr.io/jimeh/airplan@sha256:<digest>
docker pull ghcr.io/jimeh/airplan:<version>
docker pull ghcr.io/jimeh/airplan@sha256:<digest>
gh attestation verify oci://ghcr.io/jimeh/airplan@sha256:<digest> \
  --repo jimeh/airplan
```

Also test:

- anonymous pulls after the package becomes public;
- native execution on real amd64 and arm64 hosts;
- reverse-proxy deployment with HTTPS;
- environment-only storage configuration with no config file;
- automatic discovery of a read-only `/etc/airplan/config.toml`;
- a mounted mode-0600 token file;
- a custom server port with matching publication and health probe;
- a named persistent state volume across container replacement;
- a bind-mounted state directory owned by UID/GID `65532:65532`; and
- failure/retry behavior after deliberately stopping before version tagging.

## 12. Acceptance criteria

The work is complete when:

1. `ghcr.io/jimeh/airplan:<version>` resolves to one multi-platform index.
2. The runnable platform set is exactly `linux/amd64` and `linux/arm64`.
3. Each platform contains the exact corresponding GoReleaser release binary.
4. No Docker build step compiles Go or executes target-platform code.
5. CI and release workflows do not install QEMU.
6. The image runs as numeric non-root and has writable temp and state paths.
7. The exact default command is `serve`, with no baked listen address or shell
   interpolation.
8. The image supplies documented host, port, and non-loopback environment
   defaults while native CLI defaults remain loopback-safe.
9. Explicit server flags override environment fallbacks, and malformed
   environment inputs fail before the server listens.
10. Environment-only configuration works without a config file.
11. A read-only `/etc/airplan/config.toml` is auto-discovered without forcing
    `AIRPLAN_CONFIG`.
12. Config, token, and persistent manifest state remain external to the image.
13. `/var/lib/airplan` is a declared, non-root-writable volume and
    `AIRPLAN_MANIFEST` points inside it.
14. The native image passes MinIO-backed environment-only and file-configured
    upload tests.
15. Manifest state survives container replacement through a named volume.
16. A custom server port works when the external mapping and probe match it.
17. The image index has an SBOM and a verifiable GitHub provenance attestation.
18. Exact version tags are never overwritten with a conflicting digest.
19. No `v`-prefixed, SHA, major, or major/minor container tags are published.
20. `latest` cannot move backwards during an old release rerun.
21. Container publication starts only after immutable release verification.
22. A post-publication failure creates one durable repository issue.
23. README, SPEC, IMPLEMENTATION, SECURITY, and AGENTS guidance agree.
24. The first public package supports anonymous tag and digest pulls.
25. The full repository verification gate passes.

## 13. Sanity-check findings

### The no-QEMU design is sound

Buildx does not require emulation merely to produce a multi-platform index.
Emulation is needed when a build executes target-platform programs. This
Dockerfile only resolves a platform-specific base and copies a
platform-specific static binary. Omitting QEMU from CI makes the constraint
enforceable.

### The generated context removes the awkward path conditional

GoReleaser currently emits architecture-feature suffixes such as `_v1` and
`_v8.0`. Directly interpolating `TARGETARCH` into those paths is insufficient.
Normalizing through `artifacts.json` makes the Dockerfile's
`$TARGETPLATFORM/airplan` copy stable and avoids an Alpine `case`/`mv` stage.

### Job filesystems do not leak across the release graph

The existing `build-release` and downstream jobs run on separate runners.
Passing a named, short-retention workflow artifact is necessary. This is the
same proven mechanism already used for the generated Homebrew Cask.

### GoReleaser `dockers_v2` is capable but has the wrong publication boundary

GoReleaser 2.17 can construct the same context and multi-platform index
directly. Adding it to the current production `goreleaser release` invocation
would push the image before native macOS checks and immutable publication.
Separating build input from GHCR publication preserves Airplan's release
ordering without recompiling binaries.

### Attestations affect index inspection

SBOM and provenance artifacts may appear as `unknown/unknown` descriptors.
Platform verification must distinguish runnable image manifests from
attestation manifests. A naive assertion that the index has exactly two total
descriptors would reject a correctly attested image.

### GHCR tags need workflow-level protection

GitHub's immutable-release protection covers Git tags and release assets, not
GHCR tags. Pushing canonically by digest, verifying before tagging, refusing
conflicting exact tags, and recommending digest pins close that gap without
requiring destructive package cleanup or extra staging tags.

BuildKit's image exporter explicitly defines `push-by-digest=true` as an
unnamed image push and `name-canonical=true` as a canonical
`name@sha256:<digest>` reference. This supports verification before the first
user-facing tag without inventing a SHA tag.

### Retries need a latest-release check

The reusable workflow accepts manual tag/SHA inputs. A rerun of an older
release after a newer one exists could otherwise move `latest` backwards.
Querying GitHub's current latest release before tagging prevents that subtle
regression.

### The runtime matches the server's actual constraints

The image design preserves:

- one server per persistent manifest;
- startup S3 readiness;
- non-loopback acknowledgement;
- external TLS termination;
- static bearer authentication;
- graceful SIGTERM shutdown;
- writable bounded-upload temporary storage; and
- an external persistent manifest.

The plan does not imply horizontal replicas, embedded TLS, account management,
or secret injection that the server does not provide.

### Exec-form commands do not expand environment variables

Baking `--listen=0.0.0.0:8080` into exec-form `CMD` would make the default
address impossible to customize without replacing the command. Adding a shell
entrypoint solely for interpolation would expand the runtime and complicate
signal handling. Resolving namespaced environment defaults inside Airplan keeps
the distroless, shell-free image and lets `CMD` remain exactly `serve`.

### A forced config path breaks environment-only deployments

`AIRPLAN_CONFIG` is an explicit selection and must point to an existing file.
Setting it in the image would make a missing config mount fatal even when all
storage settings are supplied through environment variables. Setting
`XDG_CONFIG_HOME=/etc` instead gives the image an optional conventional path
without changing the existing explicit-path contract.

### A Dockerfile volume is documentation, not lifecycle management

`VOLUME ["/var/lib/airplan"]` identifies mutable state and protects it from
being written into the disposable container layer, but Docker can satisfy it
with an anonymous volume. Operator examples must name the volume explicitly so
container replacement retains an intentional reference to the manifest state.
Automatic startup reconciliation is not a substitute for correct persistence.

## 14. Risks and mitigations

### A valid GitHub release exists while GHCR publication is failing

Mitigation: keep publication independently rerunnable, avoid partial version
tags, and create a durable issue after failure. The existing Cask flow accepts
the same downstream-publication model.

### A workflow retry changes an exact release image

Mitigation: push new builds without a tag, reuse an existing exact-version
digest only after complete validation, and refuse to overwrite exact tags that
resolve elsewhere. Serialize workflow-owned package mutations across versions.
GHCR does not enforce tag immutability or document an atomic create-only tag
operation, so an external writer remains outside this guarantee; digest pinning
is the immutable consumer boundary.

### The base image moves or changes between retries

Mitigation: pin the multi-platform distroless index by digest and update it
through reviewed dependency PRs.

### A future Dockerfile silently reintroduces emulation

Mitigation: do not install QEMU and reject target-platform `RUN` instructions
in review. CI must build both platforms on an amd64 runner.

### Workflow artifacts lose executable mode

Mitigation: `COPY --chmod=0555` sets the executable mode in the final image;
do not rely on artifact transport preserving Unix permissions.

### The non-root process cannot write state or temporary files

Mitigation: create `/var/lib/airplan` with numeric ownership in the image,
verify `/tmp` and state writes in CI, and document bind-mount ownership.

### Environment and CLI configuration drift

Mitigation: define one precedence table in SPEC.md, keep flag resolution in
focused helpers, test every new fallback and override path without Docker, and
exercise the image defaults separately in container smoke tests.

### A changed server port is not reachable

Mitigation: document that `EXPOSE` is static metadata and require a matching
published port, reverse-proxy target, and health probe whenever
`AIRPLAN_SERVER_PORT` changes.

### An anonymous state volume is orphaned

Mitigation: declare the state boundary in the Dockerfile, use named volumes in
all primary examples, document backup and bind-mount ownership, and verify
manifest persistence across container replacement.

### A first release remains private in GHCR

Mitigation: include the one-time visibility operation and anonymous pull tests
in the rollout checklist. Do not claim availability until they pass.

### Registry attestations duplicate or disagree

Mitigation: disable Buildx provenance, retain Buildx SBOM generation, and use
the repository's existing GitHub attestation mechanism as the one provenance
authority.

## 15. References

- Docker multi-platform builds:
  <https://docs.docker.com/build/building/multi-platform/>
- Docker Buildx GitHub Actions:
  <https://docs.docker.com/build/ci/github-actions/multi-platform/>
- Buildx image inspection:
  <https://docs.docker.com/reference/cli/docker/buildx/imagetools/inspect/>
- Buildx image creation and digest-based tag assignment:
  <https://docs.docker.com/reference/cli/docker/buildx/imagetools/create/>
- BuildKit image exporter and tagless digest options:
  <https://github.com/moby/buildkit#imageregistry>
- GitHub container registry:
  <https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry>
- GitHub container attestations:
  <https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations>
- GoReleaser Docker v2:
  <https://www.goreleaser.com/customization/package/dockers_v2/>
- Prior no-recompile release-image pattern:
  <https://github.com/jimeh/go-mcp-time/blob/main/Dockerfile.release>

## 16. Resolved review decisions

1. Publish only unprefixed exact versions such as `0.5.1` and `latest`.
   Do not publish `v0.5.1`, SHA, major, or major/minor tags.
2. Keep the one-time manual GHCR package visibility change in the first-release
   operator checklist.
3. Use the current distroless Debian 13 static non-root base, pinned by its
   multi-platform index digest.

Unresolved questions: none.
