# Token log query rate limits

`GET /api/log/token` uses a dedicated per-user query limit after read-only token authentication. All API keys belonging to the same user share the allowance. This route no longer consumes the Critical IP bucket used by login and other sensitive operations. Other log routes and `/api/usage/token/` retain their existing limits.

Administrators can edit **Log query limit** in the user edit drawer. The server's current global default and window duration appear beside the input.

- `0` or an unset database value inherits the global default.
- An integer from `1` to `60000` overrides the request count for that user, using the global window duration.
- Omitting `log_query_rate_limit` (or sending `null`) in an existing `PUT /api/user/` request preserves the saved value. Send `0` to restore inheritance.
- Only administrator edits can change the field. Self-profile updates cannot change it. Existing target-role restrictions still apply.
- Changes take effect on the next log query on every instance. They do not clear already counted requests, revoke login sessions, or require restarting the application.

Global environment settings:

```env
LOG_QUERY_RATE_LIMIT_ENABLE=true
LOG_QUERY_RATE_LIMIT=60
LOG_QUERY_RATE_LIMIT_DURATION=60
```

The default count must be between 1 and 60000. The duration is in seconds and must be between 1 and 1200, within the existing in-memory limiter retention period. Changing environment settings requires restarting the application. Disabling the global switch bypasses this dedicated limiter, including user overrides; saved overrides remain available when it is re-enabled.

The users table gains a nullable integer `log_query_rate_limit` column through the existing GORM migration. Existing rows inherit the global setting. There is no database-specific SQL or change to relaykit.

The query path reads only the user's limit column from the primary database to avoid stale authorization-cache snapshots after an admin edit. Redis deployments use the existing atomic fixed-window limiter and share counters across instances. Without Redis, counters are process-local and use the existing in-memory sliding window. Use Redis for consistent multi-instance enforcement. A rejected query returns HTTP 429 and `Retry-After`; a limit lookup or Redis failure returns 500. Clients should respect `Retry-After` before retrying.

The outer global IP limiter still applies. Its default is 360 requests per 180 seconds, shared with other `/api/*` requests from the same client IP. A user override of 300 per minute does not bypass this ceiling. Check real client IP forwarding and adjust `GLOBAL_API_RATE_LIMIT` for legitimate shared-egress traffic when needed.

The response and data scope remain unchanged: each API key can read only its own token logs. The endpoint still returns up to `MaxRecentItems` recent rows; it is not a paginated log export API.
