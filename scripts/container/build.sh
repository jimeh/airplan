#!/usr/bin/env bash

set -euo pipefail

image="${AIRPLAN_CONTAINER_IMAGE:-airplan:container-test}"
platform="${AIRPLAN_CONTAINER_PLATFORM:-linux/$(go env GOARCH)}"
version="$(jq -r '.version' dist/metadata.json)"
revision="$(git rev-parse HEAD)"

docker buildx build \
  --platform "$platform" \
  --pull \
  --load \
  --tag "$image" \
  --build-arg "RELEASE_VERSION=$version" \
  --build-arg "RELEASE_REVISION=$revision" \
  dist/container
