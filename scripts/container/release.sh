#!/usr/bin/env bash

set -euo pipefail

extract_imagetools_digest() {
  local candidate digest=""
  local digest_lines=0
  local valid_digests=0
  local line

  while IFS= read -r line; do
    if [[ "$line" != Digest:* ]]; then
      continue
    fi
    ((digest_lines += 1))
    candidate="${line#Digest:}"
    candidate="${candidate#"${candidate%%[![:space:]]*}"}"
    candidate="${candidate%"${candidate##*[![:space:]]}"}"
    if [[ "$candidate" =~ ^sha256:[0-9a-f]{64}$ ]]; then
      digest="$candidate"
      ((valid_digests += 1))
    fi
  done

  if [[ "$digest_lines" != "1" || "$valid_digests" != "1" ]]; then
    echo "expected one valid image-index digest" >&2
    return 1
  fi
  printf '%s\n' "$digest"
}

verify_release_binary() (
  local reference="${1:?image reference is required}"
  local architecture="${2:?architecture is required}"
  local expected="${3:?expected binary is required}"
  local container_id=""
  local temporary

  case "$architecture" in
  amd64 | arm64) ;;
  *)
    echo "unsupported release architecture: $architecture" >&2
    return 1
    ;;
  esac
  if [[ ! -f "$expected" ]]; then
    echo "expected release binary does not exist: $expected" >&2
    return 1
  fi

  temporary="$(mktemp -d)"
  # Invoked by the EXIT trap below.
  # shellcheck disable=SC2329
  cleanup() {
    if [[ -n "$container_id" ]]; then
      docker rm --force "$container_id" >/dev/null 2>&1 || true
    fi
    rm -rf "$temporary"
  }
  trap cleanup EXIT

  container_id="$(
    docker create --platform "linux/$architecture" "$reference"
  )"
  docker cp \
    "$container_id:/usr/local/bin/airplan" \
    "$temporary/airplan"
  if ! cmp --silent "$expected" "$temporary/airplan"; then
    echo "linux/$architecture image binary differs from release" >&2
    return 1
  fi
)

case "${1:-}" in
extract-digest)
  extract_imagetools_digest
  ;;
verify-binary)
  verify_release_binary "${2:-}" "${3:-}" "${4:-}"
  ;;
*)
  echo "usage: release.sh extract-digest" >&2
  echo "       release.sh verify-binary IMAGE ARCH EXPECTED" >&2
  exit 2
  ;;
esac
