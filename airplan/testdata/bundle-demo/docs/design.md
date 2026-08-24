# Gateway design notes

[Back to the rollout handoff](../implementation-plan.md)

The account service trusts identity headers only when the request came through
the gateway. The gateway verifies the bearer token, strips any client-supplied
identity headers, then writes the verified account ID.

## Failure behavior

| Condition | Response | Retry |
| --- | --- | --- |
| Missing token | `401` | No |
| Invalid token | `401` | No |
| Gateway timeout | `503` | Yes |

The service keeps its existing direct validator behind a rollback flag. That
path expires after the regional rollout has stayed healthy for 24 hours.
