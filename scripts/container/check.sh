#!/usr/bin/env bash

set -euo pipefail

image="${AIRPLAN_CONTAINER_IMAGE:-airplan:container-test}"
version="$(jq -r '.version' dist/metadata.json)"
revision="$(git rev-parse HEAD)"
config_file="$(mktemp)"

cleanup() {
  rm -f "$config_file"
}
trap cleanup EXIT

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

docker run --rm "$image" config profiles --json |
  jq -e 'length == 0' >/dev/null

printf '[profiles.container-check]\nregion = "auto"\n' >"$config_file"
chmod 0444 "$config_file"
docker run --rm \
  --mount "type=bind,source=$config_file,target=/etc/airplan/config.toml,readonly" \
  "$image" config profiles --json |
  jq -e '
    length == 1
    and .[0].name == "container-check"
  ' >/dev/null

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
