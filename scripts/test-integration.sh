#!/usr/bin/env bash
# Runs the integration tests against a throwaway MinIO container.
set -euo pipefail

PORT="${AIRPLAN_TEST_MINIO_PORT:-19100}"
NAME="airplan-minio-test"

cleanup() {
	docker stop "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --rm --name "$NAME" \
	-p "127.0.0.1:${PORT}:9000" \
	minio/minio server /data >/dev/null

echo "waiting for MinIO on port ${PORT}..." >&2
for _ in $(seq 1 50); do
	if curl -fsS "http://127.0.0.1:${PORT}/minio/health/live" \
		>/dev/null 2>&1; then
		break
	fi
	sleep 0.2
done

AIRPLAN_TEST_ENDPOINT="http://127.0.0.1:${PORT}" \
	go test ./airplan/ -run TestIntegration -v -count=1 "$@"
