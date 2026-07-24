#!/usr/bin/env bash

set -euo pipefail

readonly helper="scripts/container/release.sh"
readonly hex="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
readonly digest="sha256:$hex"

inspection="$(printf '%s\n' \
  'Name:      ghcr.io/jimeh/airplan:0.5.1' \
  'MediaType: application/vnd.oci.image.index.v1+json' \
  "Digest:    $digest")"
actual="$(printf '%s\n' "$inspection" | "$helper" extract-digest)"
if [[ "$actual" != "$digest" ]]; then
  echo "digest extraction returned $actual, want $digest" >&2
  exit 1
fi

expect_rejected() {
  local input="$1"
  if printf '%s\n' "$input" |
    "$helper" extract-digest >/dev/null 2>&1; then
    echo "malformed digest inspection was accepted" >&2
    exit 1
  fi
}

expect_rejected "Name: ghcr.io/jimeh/airplan:0.5.1"
expect_rejected "Digest: sha256:short"
expect_rejected "Digest: SHA256:$hex"
expect_rejected "$(printf 'Digest: %s\nDigest: %s\n' "$digest" "$digest")"

temporary="$(mktemp -d)"
cleanup() {
  rm -rf "$temporary"
}
trap cleanup EXIT
mkdir "$temporary/bin"
printf 'release-binary\n' >"$temporary/release"
printf 'different-binary\n' >"$temporary/different"
cat >"$temporary/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "$1" in
create)
  printf 'airplan-release-test\n'
  ;;
cp)
  cp "$FAKE_CONTAINER_BINARY" "$3"
  ;;
rm)
  : >"$FAKE_REMOVE_MARKER"
  ;;
*)
  echo "unexpected docker command: $1" >&2
  exit 1
  ;;
esac
EOF
chmod +x "$temporary/bin/docker"

export FAKE_CONTAINER_BINARY="$temporary/release"
export FAKE_REMOVE_MARKER="$temporary/removed"
PATH="$temporary/bin:$PATH" \
  "$helper" verify-binary \
  ghcr.io/jimeh/airplan@sha256:test \
  amd64 \
  "$temporary/release"
if [[ ! -f "$FAKE_REMOVE_MARKER" ]]; then
  echo "successful binary verification did not remove its container" >&2
  exit 1
fi

rm "$FAKE_REMOVE_MARKER"
if PATH="$temporary/bin:$PATH" \
  "$helper" verify-binary \
  ghcr.io/jimeh/airplan@sha256:test \
  arm64 \
  "$temporary/different" >/dev/null 2>&1; then
  echo "mismatched image binary was accepted" >&2
  exit 1
fi
if [[ ! -f "$FAKE_REMOVE_MARKER" ]]; then
  echo "failed binary verification did not remove its container" >&2
  exit 1
fi
