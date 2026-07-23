# Security Policy

## Supported versions

Security fixes are provided for the latest released version of airplan. Before
the initial release, the current `main` branch is the only supported version.

## Reporting a vulnerability

Please report suspected vulnerabilities privately through
[GitHub Security Advisories](https://github.com/jimeh/airplan/security/advisories/new).
Include reproduction steps, affected versions, and the likely impact when
possible.

Do not open a public issue for an undisclosed vulnerability. You can expect an
acknowledgement within seven days, followed by updates as the report is
validated and a fix is prepared.

## Self-hosted server threat model

`airplan serve` is a single-user, single-instance service. One static bearer
token grants the complete REST and hosted MCP operation set, including upload,
capability-URL discovery, download, sync, and deletion. It is not an account,
role, tenant, audit, OAuth, or token-issuance system. Treat the token like the
S3 credentials it protects:

- generate at least 32 random bytes;
- prefer a mode-0600 token file over command-line arguments;
- never place it in a URL, repository, image, log, issue, or chat message; and
- restart the server after replacing its token file.

The server listens on loopback by default and does not terminate TLS. Put any
non-loopback deployment behind a trusted reverse proxy with HTTPS, appropriate
upload body limits, streaming-friendly buffering/timeouts, and restricted
network exposure. Plain HTTP is suitable only on loopback. Run one active
server per persistent manifest; multi-replica coordination is outside the
security and consistency model.

REST and hosted MCP share the bearer token. MCP uses a custom Authorization
header rather than the MCP OAuth authorization flow, so remote clients unable
to set that header are unsupported. Streamable HTTP rejects every present
`Origin` not in the configured allowlist. The default allowlist is empty;
requests without Origin remain valid for non-browser agent clients. CORS is
disabled by default.

`GET /healthz` and `GET /openapi.yaml` are intentionally public and contain no
credentials or storage identity. Authenticated capability responses must not
reveal S3 endpoints, bucket names, key prefixes, filesystem paths, token
metadata, or raw configuration. Request logs and RFC 9457 errors must omit
Authorization values, request bodies, capability URLs, S3 response bodies,
credentials, and internal filesystem paths.

`serve` defaults to quiet `info` logging. Debug logs may identify why bearer
authentication failed using only the fixed categories `missing`, `duplicate`,
`wrong_scheme`, `malformed`, and `token_mismatch`; the HTTP response remains
generic. Trace logs add sanitized protocol lifecycle metadata, never raw HTTP
or MCP frames. No log level records Authorization values, token length or
other token metadata, request bodies, tool arguments/results, upload content,
capability URLs or keys, S3 endpoints/buckets/response bodies, credentials, or
filesystem paths. Request IDs are server-generated; incoming `X-Request-Id`
values are ignored rather than reflected into responses or logs. Treat debug
and trace output as operational data despite these exclusions.

## Capability URLs and uploaded content

Airplan's published links are unguessable capability URLs, not authenticated
resources. Anyone who receives a URL can open and redistribute it, and chat or
proxy services may prefetch it. Bucket listing must remain private. Deletion
removes Airplan-managed storage but cannot revoke copies already fetched or
cached outside your control.

Markdown and HTML are trusted authored content, and collection members are
uploaded byte-for-byte. HTML, SVG, links, and embedded resources may execute or
load active content. Review documents, screenshots, recordings, filenames, and
other artifacts for credentials and private information before uploading.

## Disclosure scope

Please report authentication bypasses, bearer-token disclosure, Origin-policy
bypasses, arbitrary filesystem or S3 object access, upload-limit bypasses,
cross-profile manifest disclosure, ownership-marker validation bypasses,
request-log secret leakage, and temp-file retention as security issues. Normal
capability-URL forwarding by someone who already has the URL, or active content
that an authorized user intentionally uploaded, is part of the documented
trust model unless it crosses another boundary.
