# Run a single-user Airplan server

The default `s3` backend gives the CLI direct access to S3 credentials and
storage. The `airplan` backend sends the same operations to `airplan serve`, so
client machines only need a server URL and bearer token.

Airplan v1 is a single-user service. It has one static bearer token and no
accounts, roles, OAuth flow, or token issuance. Run only one server process for
each manifest.

## Start the native server

Create a token file with at least 32 random bytes and restrict its permissions:

```sh
openssl rand -base64 32 > /run/secrets/airplan-token
chmod 600 /run/secrets/airplan-token
```

Start the server with a direct S3 profile and a persistent manifest path:

```sh
airplan --profile storage \
  --manifest /var/lib/airplan/manifest.jsonl \
  serve --token-file /run/secrets/airplan-token
```

`serve` defaults to `127.0.0.1:8080`. It checks storage before listening and
exposes:

- the authenticated REST API under `/api/v1`;
- authenticated MCP Streamable HTTP at `/mcp`;
- unauthenticated liveness at `/healthz`; and
- the authoritative OpenAPI 3.0.3 schema at `/openapi.yaml`.

Terminate TLS at a trusted reverse proxy. HTTPS is required for non-loopback
client URLs, and a non-loopback listen address requires
`--allow-non-loopback`. Configure the proxy's body limits, buffering, and
timeouts for large streaming uploads and downloads.

## Configure clients

A client profile needs only the server URL and bearer token:

```toml
[profiles.shared]
backend   = "airplan"
api_url   = "https://airplan.example.com"
api_token = "..." # or AIRPLAN_API_TOKEN
```

Normal commands then use the server without local S3 credentials:

```sh
airplan --profile shared plan.md
airplan --profile shared list
airplan --profile shared list --remote
airplan --profile shared sync
airplan --profile shared purge --older-than 30d
```

The server renders uploads with its own theme and template configuration. A
client cannot override the server's storage connection, source policy,
indexability, Mermaid policy, or theme catalog.

Clients negotiate document-bundle and revision-route support through
`/api/v1/capabilities` before streaming an upload. They do not replay a
partially sent body after probing an unsupported route.

## Connect MCP clients

Remote MCP clients connect to `https://airplan.example.com/mcp` and send
`Authorization: Bearer <token>`. Clients that cannot attach a custom
Authorization header are not supported.

The MCP server exposes upload, revision, list, inspect, delete, sync, two-phase
purge, and preview-first document upgrade tools. Hosted MCP accepts inline UTF-8
pages and base64 assets, but never server-local paths. Inline assets have a 32
MiB decoded aggregate limit in addition to the hosted request-body limit.

Local agents can run `airplan mcp` as a stdio server. Stdio follows the selected
`s3` or `airplan` profile and adds path-based upload tools when the operation is
local. Template dumping, config inspection, arbitrary object access, and
filesystem browsing are not exposed through MCP.

## Configure logs

The default `info` level keeps stderr quiet apart from the listening line and
server failures. `warn` and `error` suppress the listening line. Use
`--log-level debug` for request completion, safe authentication rejection
reasons, Origin and size-limit failures, and MCP tool outcomes:

```sh
airplan --profile storage serve \
  --token-file /run/secrets/airplan-token \
  --log-level debug
```

`--log-level trace` also reports sanitized request starts, MCP protocol methods,
and SDK lifecycle events. `AIRPLAN_SERVER_LOG_LEVEL` is the environment
fallback; an explicit flag wins.

Logs never include Authorization values, request or MCP bodies, tool arguments
or results, uploaded content, capability URLs, storage identity, credentials,
or filesystem paths.

## Share the manifest safely

The manifest must live on persistent storage. A same-user local CLI and native
server share the platform manifest by default, so local `airplan list` includes
uploads received through the API when both processes select the same S3 profile.

Containers and services often use a different state directory. Mount a
persistent directory and pass the same `--manifest` path when shared local
history is wanted. The server scopes manifest results to its configured profile,
bucket, and key prefix. It does not expose unrelated records from a shared file.

`airplan sync` can reconstruct remote uploads that still have ownership markers
after manifest loss. It may not recover every historical or local field, and the
server does not sync automatically at startup. Back up the manifest when upload
history matters.

## Run the container

The official image is available for `linux/amd64` and `linux/arm64`:

```sh
docker pull ghcr.io/jimeh/airplan:latest
```

Use an exact unprefixed version or image index digest for reproducible
deployments. [Verify a release](release-verification.md) explains the image
attestation and digest checks.

The container defaults to `airplan serve`, listens on `0.0.0.0:8080`, runs as
UID/GID `65532:65532`, and stores its manifest at
`/var/lib/airplan/manifest.jsonl`. Give that directory a named volume:

```sh
docker volume create airplan-data

docker run --detach --name airplan \
  --publish 127.0.0.1:8080:8080 \
  --env-file ./airplan.env \
  --mount type=volume,source=airplan-data,target=/var/lib/airplan \
  --mount type=bind,source="$PWD/airplan-token",\
target=/run/secrets/airplan-token,readonly \
  --env AIRPLAN_SERVER_TOKEN_FILE=/run/secrets/airplan-token \
  ghcr.io/jimeh/airplan:<version>
```

`airplan.env` may contain the environment-only storage configuration:

```dotenv
AIRPLAN_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
AIRPLAN_BUCKET=plans
AIRPLAN_REGION=auto
AIRPLAN_ACCESS_KEY_ID=...
AIRPLAN_SECRET_ACCESS_KEY=...
AIRPLAN_PUBLIC_BASE_URL=https://plans.example.com
AIRPLAN_REPO=none
```

Protect both files from other users. The mode-0600 token file must be owned by
UID/GID `65532:65532` so the container can read it. Prefer a runtime secret
manager over a plaintext environment file in production.

### Mount a config file

For file-based configuration, omit the storage variables and mount the file at
the optional default path:

```sh
docker run --detach --name airplan \
  --publish 127.0.0.1:8080:8080 \
  --mount type=volume,source=airplan-data,target=/var/lib/airplan \
  --mount type=bind,source="$PWD/config.toml",\
target=/etc/airplan/config.toml,readonly \
  --mount type=bind,source="$PWD/airplan-token",\
target=/run/secrets/airplan-token,readonly \
  --env AIRPLAN_PROFILE=storage \
  --env AIRPLAN_SERVER_TOKEN_FILE=/run/secrets/airplan-token \
  ghcr.io/jimeh/airplan:<version>
```

The container discovers `/etc/airplan/config.toml` without
`AIRPLAN_CONFIG`. If `AIRPLAN_CONFIG` is set explicitly, the selected file must
exist.

### Use Docker Compose

A minimal Compose service is:

```yaml
services:
  airplan:
    image: ghcr.io/jimeh/airplan:<version>
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      AIRPLAN_PROFILE: storage
      AIRPLAN_SERVER_TOKEN_FILE: /run/secrets/airplan-token
    volumes:
      - airplan-data:/var/lib/airplan
      - ./config.toml:/etc/airplan/config.toml:ro
      - ./airplan-token:/run/secrets/airplan-token:ro

volumes:
  airplan-data:
```

Run one container against the volume. Anonymous volumes can become orphaned
when a container is replaced. For a state bind mount, make the host directory
writable by UID/GID `65532:65532`. Mode-0600 config and token bind mounts must
be owned by the same IDs.

The image declares `EXPOSE 8080`, which does not publish the port. If
`AIRPLAN_SERVER_PORT` changes, update the Docker port mapping, reverse-proxy
target, and external `/healthz` probe. The image contains no shell, probe
utility, credentials, config, token, or in-image healthcheck.

## Server option precedence

An explicit flag wins over its environment fallback, followed by the native
default:

| Concern                | Flag                   | Environment fallback                         |
| ---------------------- | ---------------------- | -------------------------------------------- |
| Listen address         | `--listen`             | `AIRPLAN_SERVER_HOST`, `AIRPLAN_SERVER_PORT` |
| Non-loopback consent   | `--allow-non-loopback` | `AIRPLAN_SERVER_ALLOW_NON_LOOPBACK`          |
| Token file             | `--token-file`         | `AIRPLAN_SERVER_TOKEN_FILE`                  |
| Hosted MCP origins     | `--allowed-origin`     | `AIRPLAN_SERVER_ALLOWED_ORIGINS`             |
| Upload spool directory | `--temp-dir`           | `AIRPLAN_SERVER_TEMP_DIR`                    |
| Log level              | `--log-level`          | `AIRPLAN_SERVER_LOG_LEVEL`                   |

`AIRPLAN_SERVER_TOKEN` is the alternative token-value source. Do not set it
together with a resolved token file. The origins environment value is a
comma-separated list. Server ports are decimal values from 0 through 65535, and
server booleans accept exactly `true` or `false`.

The image sets host `0.0.0.0`, port `8080`, and non-loopback consent `true`.
Native execution retains the safer `127.0.0.1:8080` and `false` defaults.

For the full wire contract and operation ownership rules, see
[SPEC.md section 11](../SPEC.md#11-backends-http-server-rest-api-and-mcp) and
the authoritative [OpenAPI schema](../api/openapi.yaml).
