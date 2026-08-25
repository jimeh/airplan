# Verify an Airplan release

Airplan publishes release archives for Linux, macOS, and Windows, plus a server
container for `linux/amd64` and `linux/arm64`. This guide verifies that an
archive or image came from Airplan's release workflow.

## Verify an archive

Release assets include a checksum file and a separate SPDX JSON SBOM for each
archive. Download the archive, its `.spdx.json` file, and `checksums.txt` from
the same [GitHub release](https://github.com/jimeh/airplan/releases).

Check the archive checksum:

```sh
# Linux
sha256sum --ignore-missing --check checksums.txt

# macOS
shasum --ignore-missing --algorithm 256 --check checksums.txt
```

On Windows, use the matching `.zip` name with a SHA-256 tool that can read
`checksums.txt`.

GitHub marks published releases as immutable. Verify that state and the release
workflow attestation with the GitHub CLI. Replace the version and archive name
with the release you downloaded.

<!-- x-release-please-start-version -->

```sh
gh release verify v0.12.0 --repo jimeh/airplan

gh attestation verify airplan_0.12.0_darwin_arm64.tar.gz \
  --repo jimeh/airplan
```

The first command verifies GitHub's immutable release state. The second confirms
that this repository's release workflow produced the archive.

## Verify a macOS binary

Official macOS archives contain native `amd64` or `arm64` executables. The
release workflow signs each executable with a Developer ID Application identity,
enables the hardened runtime and a secure timestamp, and waits for Apple to
accept it for notarization before publishing the release.

A raw executable cannot carry a stapled notarization ticket. Its first
Gatekeeper assessment may need internet access to retrieve the ticket from
Apple. This guarantee does not cover `go install`; locally built executables are
not signed or notarized by the project.

## Verify the server image

The official image is `ghcr.io/jimeh/airplan`. Releases publish an exact
unprefixed version and the mutable `latest` tag. Use an exact version or image
index digest for a reproducible deployment.

Verify a versioned image:

```sh
docker pull ghcr.io/jimeh/airplan:0.12.0

gh attestation verify oci://ghcr.io/jimeh/airplan:0.12.0 \
  --repo jimeh/airplan \
  --signer-workflow jimeh/airplan/.github/workflows/release.yml
```

<!-- x-release-please-end -->

For the registry-level immutable reference, resolve and use the image index
digest:

```sh
docker pull ghcr.io/jimeh/airplan@sha256:<image-index-digest>

gh attestation verify \
  oci://ghcr.io/jimeh/airplan@sha256:<image-index-digest> \
  --repo jimeh/airplan \
  --signer-workflow jimeh/airplan/.github/workflows/release.yml
```

The publication workflow serializes its own GHCR changes and rejects an exact
release tag whose observed digest conflicts. GHCR does not prevent an external
writer from moving a tag, so consumers that require registry-level immutability
must pin the image index digest.

## Release maintainer checks

The first container release creates a private GHCR package. For that one-time
setup, link the package to this repository, change its visibility to public,
then verify anonymous exact-tag and digest pulls.

Before publishing a release, verify the GitHub attestation, run the image on
real `amd64` and `arm64` hosts, inspect the package metadata, and repeat the
documented commands from a clean machine. [AGENTS.md](../AGENTS.md) contains the
full release constraints and repository tasks.
