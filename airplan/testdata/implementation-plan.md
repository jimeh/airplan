---
title: Zero-downtime token migration
owner: Identity Platform
status: Proposed
target: 2026-Q3
---

# Zero-downtime token migration

This fictional plan shows how a team can replace session cookies with signed
access tokens without interrupting active users. The rollout is deliberately
phased: compatibility first, migration second, cleanup last.

> [!NOTE]
> This is a demonstration document. The systems, metrics, and contacts below
> are illustrative rather than production guidance.

## Outcome

:::: {.columns}
::: {.column width="55%"}
### Goals

- Keep existing sessions valid throughout the migration.
- Make rollback a configuration change, not a deployment.
- Give support a clear explanation for every rejected request.
:::
::: {.column width="45%"}
### Non-goals

- Replacing the identity provider.
- Redesigning account recovery.
- Supporting ~~indefinite~~ permanent legacy sessions.
:::
::::

Success
: At least 99.95% of authenticated requests complete without a token-related
  retry.

Rollback
: Operators can restore cookie-first validation within five minutes.

## Migration flow

```mermaid
flowchart LR
    A[Cookie session] --> B[Dual validation]
    B --> C[Token preferred]
    C --> D[Token only]
    B -. rollback .-> A
    C -. rollback .-> B
```

Every phase has an explicit observation window. Promotion is manual until the
error budget and support signals agree.

## Compatibility contract

| Client state        | Read cookie | Read token | Issue token | Result       |
| ------------------- | ----------- | ---------- | ----------- | ------------ |
| Existing session    | Yes         | No         | Yes         | Transparent  |
| Migrated session    | Yes         | Yes        | Refresh     | Token-first  |
| New authentication  | No          | Yes        | Yes         | Token-only   |
| Invalid credentials | No          | No         | No          | Reauthenticate |

> [!IMPORTANT]
> Token and cookie validation must share the same revocation source. Running
> two independent authorization models would make rollback unsafe.

## Delivery tracking

The rollout is coordinated in #184. Dual-validation middleware lands in PR
#231, followed by token-first authentication in PR #248. The mobile client
dependency is tracked separately in octo-mobile/auth-client#77.

These references use the fictional `octo-org/identity-platform` repository
configured for this demonstration.

## Implementation

### 1. Add dual validation

The middleware accepts either credential while recording which path succeeded.
The public handler contract remains unchanged.

```go
func authenticate(r *http.Request) (*Identity, error) {
	if token := bearerToken(r); token != "" {
		return tokens.Validate(r.Context(), token)
	}
	return sessions.Validate(r.Context(), r)
}
```

- [x] Define token claims and clock-skew policy.
- [x] Add structured outcomes for both validators.
- [ ] Shadow-validate tokens without changing request decisions.
- [ ] Enable dual validation for internal users.

### 2. Migrate active sessions

On a successful cookie request, issue a short-lived token and let the client
adopt it on its next request. Limit each migration cohort to 10% of eligible
sessions per hour.

> [!WARNING]
> Do not infer migration success from token issuance alone. Count the first
> request authenticated by that token.

### 3. Prefer tokens

After two stable observation windows, check tokens before cookies. Keep cookie
fallback enabled until the oldest supported client has crossed one complete
release cycle.[^window]

### 4. Remove compatibility code

- [ ] Disable new cookie creation.
- [ ] Wait through the maximum session lifetime.
- [ ] Remove cookie validation and migration metrics.
- [ ] Archive the operational dashboard and decision log.

## Observability

| Signal                    | Promotion gate | Rollback threshold |
| ------------------------- | -------------- | ------------------ |
| Authentication failures   | Below 0.05%    | Above 0.20%        |
| Token validation latency  | p95 under 20ms | p95 above 50ms     |
| Cookie fallback rate      | Below 2%       | Above 10%          |
| Related support contacts  | No increase    | Five in 30 minutes |

The dashboard separates invalid credentials, expired credentials, verifier
errors, and dependency timeouts. Aggregating them as “authentication failed”
would hide the failure mode we need to act on.

## Rollback

1. Set `auth_preference=cookie` in the runtime configuration.
2. Confirm cookie-first traffic exceeds 95% within five minutes.
3. Preserve token issuance logs for analysis.
4. Open an incident only if user-visible failures remain elevated.

> [!CAUTION]
> Never revoke all issued tokens as the first rollback step. Existing clients
> may already have discarded their cookies.

## Open questions

- [ ] Should service-to-service tokens use the same signing keys?
- [ ] Who owns the final legacy-session deletion approval?
- [ ] Do mobile clients need an additional release cycle?

Questions and rollout decisions go to `identity-platform@example.com`.

[^window]: For this example, one release cycle means fourteen days with no
    supported client version older than the token-capable release.
