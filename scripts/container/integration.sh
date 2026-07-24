#!/usr/bin/env bash

set -euo pipefail

readonly image="${AIRPLAN_CONTAINER_IMAGE:-airplan:container-test}"
readonly minio_image="minio/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
readonly mc_image="minio/mc:RELEASE.2025-08-13T08-35-41Z@sha256:a7fe349ef4bd8521fb8497f55c6042871b2ae640607cf99d9bede5e9bdf11727"
readonly suite="airplan-container-${RANDOM}-$$"
readonly network="${suite}-network"
readonly state="${suite}-state"
readonly minio="${suite}-minio"
readonly server="${suite}-server"
readonly root_user="airplan-test"
readonly root_password="airplan-container-test-password"
readonly bucket="airplan-container-test"
readonly token="01234567890123456789012345678901"
temporary="$(mktemp -d)"
readonly temporary

cleanup() {
  docker rm -f "$server" "$minio" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  docker volume rm "$state" >/dev/null 2>&1 || true
  rm -rf "$temporary"
}
trap cleanup EXIT

docker network create "$network" >/dev/null
docker volume create "$state" >/dev/null
docker run -d --name "$minio" --network "$network" \
  -e "MINIO_ROOT_USER=$root_user" \
  -e "MINIO_ROOT_PASSWORD=$root_password" \
  "$minio_image" server /data >/dev/null

for _ in {1..30}; do
  if docker run --rm --network "$network" "$mc_image" \
    alias set local "http://$minio:9000" \
    "$root_user" "$root_password" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker run --rm --network "$network" "$mc_image" \
  alias set local "http://$minio:9000" \
  "$root_user" "$root_password" >/dev/null
docker run --rm --network "$network" "$mc_image" \
  mb "local/$bucket" >/dev/null

printf '%s\n' "$token" >"$temporary/token"
chmod 0600 "$temporary/token"
cat >"$temporary/config.toml" <<EOF
endpoint = "http://$minio:9000"
bucket = "$bucket"
region = "us-east-1"
access_key_id = "$root_user"
secret_access_key = "$root_password"
public_base_url = "https://container-test.invalid"
repo = "none"
EOF
chmod 0600 "$temporary/config.toml"
printf '# Container integration\n\nPersistent state.\n' \
  >"$temporary/document.md"
docker run --rm --user 0 \
  --mount "type=bind,source=$temporary,target=/setup" \
  --entrypoint /usr/bin/chown \
  "$mc_image" 65532:65532 /setup/token /setup/config.toml

start_environment_server() {
  docker run -d --name "$server" --network "$network" \
    -p 127.0.0.1::18081 \
    --mount "type=volume,source=$state,target=/var/lib/airplan" \
    --mount "type=bind,source=$temporary/token,target=/run/secrets/airplan-token,readonly" \
    -e AIRPLAN_SERVER_PORT=18081 \
    -e AIRPLAN_SERVER_TOKEN_FILE=/run/secrets/airplan-token \
    -e "AIRPLAN_ENDPOINT=http://$minio:9000" \
    -e "AIRPLAN_BUCKET=$bucket" \
    -e AIRPLAN_REGION=us-east-1 \
    -e "AIRPLAN_ACCESS_KEY_ID=$root_user" \
    -e "AIRPLAN_SECRET_ACCESS_KEY=$root_password" \
    -e AIRPLAN_PUBLIC_BASE_URL=https://container-test.invalid \
    -e AIRPLAN_REPO=none \
    "$image" >/dev/null
}

start_file_server() {
  docker run -d --name "$server" --network "$network" \
    -p 127.0.0.1::8080 \
    --mount "type=volume,source=$state,target=/var/lib/airplan" \
    --mount "type=bind,source=$temporary/token,target=/run/secrets/airplan-token,readonly" \
    --mount "type=bind,source=$temporary/config.toml,target=/etc/airplan/config.toml,readonly" \
    -e AIRPLAN_SERVER_TOKEN_FILE=/run/secrets/airplan-token \
    "$image" >/dev/null
}

wait_for_server() {
  local container_port="$1"
  local host_port
  host_port="$(docker port "$server" "$container_port/tcp" |
    sed -n 's/.*://p')"
  for _ in {1..30}; do
    if curl --fail --silent \
      "http://127.0.0.1:$host_port/healthz" >/dev/null; then
      printf '%s' "$host_port"
      return
    fi
    if ! docker inspect --format '{{.State.Running}}' "$server" |
      grep -qx true; then
      docker logs "$server" >&2
      return 1
    fi
    sleep 1
  done
  docker logs "$server" >&2
  return 1
}

stop_server_gracefully() {
  local exit_code
  docker stop --time 15 "$server" >/dev/null
  exit_code="$(docker inspect --format '{{.State.ExitCode}}' "$server")"
  if [[ "$exit_code" != "0" ]]; then
    echo "server exited with $exit_code after SIGTERM" >&2
    docker logs "$server" >&2
    return 1
  fi
  docker rm "$server" >/dev/null
}

start_environment_server
port="$(wait_for_server 18081)"
curl --fail --silent \
  -H "Authorization: Bearer $token" \
  "http://127.0.0.1:$port/api/v1/capabilities" |
  jq -e '.api_version == "v1"' >/dev/null
curl --fail --silent \
  -H "Authorization: Bearer $token" \
  -F 'metadata={"name":"document.md","format":"md"};type=application/json' \
  -F "document=@$temporary/document.md;type=text/markdown" \
  "http://127.0.0.1:$port/api/v1/uploads/documents" |
  jq -e '.kind == "document"' >/dev/null

stop_server_gracefully

start_file_server
port="$(wait_for_server 8080)"
curl --fail --silent \
  -H "Authorization: Bearer $token" \
  "http://127.0.0.1:$port/api/v1/uploads" |
  jq -e '.uploads | length == 1' >/dev/null
stop_server_gracefully

if docker run --rm \
  -e AIRPLAN_SERVER_HOST=0.0.0.0 \
  -e AIRPLAN_SERVER_ALLOW_NON_LOOPBACK=false \
  "$image" >/dev/null 2>"$temporary/non-loopback-error"; then
  echo "server accepted non-loopback binding without acknowledgement" >&2
  exit 1
fi
if ! grep -q 'non-loopback' "$temporary/non-loopback-error"; then
  echo "server rejection did not explain the non-loopback requirement" >&2
  exit 1
fi
