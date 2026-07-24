#!/usr/bin/env bash

set -euo pipefail

image="${AIRPLAN_CONTAINER_IMAGE:-airplan:container-test}"
version="$(jq -r '.version' dist/metadata.json)"
revision="$(git rev-parse HEAD)"

docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --pull \
  --output type=cacheonly \
  --build-arg "RELEASE_VERSION=$version" \
  --build-arg "RELEASE_REVISION=$revision" \
  dist/container

scripts/container/build.sh

inspect="$(docker image inspect "$image")"
jq -e '
  .[0].Config.User == "65532:65532"
  and .[0].Config.Entrypoint == ["/usr/local/bin/airplan"]
  and .[0].Config.Cmd == ["serve"]
  and .[0].Config.Volumes["/var/lib/airplan"] == {}
  and (.[] | .Config.Env | index("XDG_CONFIG_HOME=/etc"))
  and (.[] | .Config.Env
    | index("AIRPLAN_MANIFEST=/var/lib/airplan/manifest.jsonl"))
  and (.[] | .Config.Env | index("AIRPLAN_SERVER_HOST=0.0.0.0"))
  and (.[] | .Config.Env | index("AIRPLAN_SERVER_PORT=8080"))
  and (.[] | .Config.Env
    | index("AIRPLAN_SERVER_ALLOW_NON_LOOPBACK=true"))
  and (.[] | .Config.Env
    | all(startswith("AIRPLAN_CONFIG=") | not))
  and .[0].Config.Labels["org.opencontainers.image.revision"] == $revision
  and .[0].Config.Labels["org.opencontainers.image.version"] == $version
' --arg revision "$revision" --arg version "$version" <<<"$inspect" >/dev/null

version_output="$(docker run --rm "$image" --version)"
if [[ "$version_output" != "airplan version $version" ]]; then
  echo "unexpected image version output: $version_output" >&2
  exit 1
fi

printf '# Writable temporary directory\n' |
  docker run --rm -i "$image" \
    preview --output /tmp/airplan-container-check.html - >/dev/null

for executable in /bin/sh /bin/bash /usr/bin/apt /usr/bin/apt-get; do
  if docker run --rm --entrypoint "$executable" "$image" \
    >/dev/null 2>&1; then
    echo "runtime unexpectedly contains $executable" >&2
    exit 1
  fi
done
