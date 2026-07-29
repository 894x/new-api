# Relay Error Admin-Only Design

## Goal

Add one global switch that preserves the current relay error behavior when
disabled and hides every externally returned relay error behind one generic
message when enabled. Original error details remain available only to
administrators through error logs.

## Configuration contract

The persisted option is `RelayErrorAdminOnlyEnabled`.

- Default: `false`.
- `false`: do not alter the current status code, response body, stream event,
  retry behavior, channel-disable behavior, or logging behavior.
- `true`: preserve the final HTTP status code and protocol envelope, but replace
  every relay error body or error event with the generic public error.
- The switch applies only to routes tagged `relay`. Management APIs under
  `/api/*` retain their current errors.

The public message is:

> 请求处理失败，请稍后重试；如问题持续，请联系管理员。（请求 ID：&lt;request-id&gt;）

The English translation carries the same meaning. Every public format uses the
stable public code `request_failed`. No public error contains `upstream`,
provider names, channel addresses, provider response metadata, or the original
message.

## Response architecture

`middleware.RouteTag("relay")` installs the masking response writer before the
remaining relay middleware. This placement covers authentication,
distribution, rate limiting, controller validation, provider calls, task
routes, Midjourney routes, and video routes without changing management APIs.

When the option is disabled, `RouteTag` keeps its current behavior and does not
wrap the writer.

When enabled, the writer:

1. Passes successful non-stream responses through unchanged.
2. Buffers responses whose HTTP status is 400 or greater.
3. Records the original error response for administrator diagnostics.
4. Emits a protocol-compatible generic error with the original HTTP status.
5. Sanitizes SSE error events written under a successful streaming status.

Realtime WebSocket errors do not use the HTTP writer after upgrade, so the
Realtime error sender applies the same public projection explicitly.

## Protocol envelopes

- OpenAI-compatible routes: `error.message`, `error.type = "server_error"`,
  and `error.code = "request_failed"`.
- Claude Messages: outer `type = "error"` with
  `error.type = "api_error"` and the generic message.
- Task/video routes: `code = "request_failed"`, the generic `message`, and
  `data = null`.
- Midjourney routes: generic `description`, `type = "request_error"`, and a
  stable generic numeric code.
- SSE: replace only error events; successful chunks remain byte-for-byte
  compatible.
- Realtime WebSocket: send the existing Realtime error envelope with the
  generic OpenAI-compatible error.

## Administrator diagnostics

When admin-only mode is enabled, relay failures are recorded even when the
legacy `ERROR_LOG_ENABLED` environment flag is off. This makes the returned
request ID actionable.

For hidden errors:

- `logs.content` contains only the generic message.
- `other.admin_info.error_detail` contains the length-limited, masked original
  error.
- `other.admin_info.original_status_code` contains the original status.
- Existing channel-chain and multi-key diagnostics remain unchanged.

`model.formatUserLogs` already removes `admin_info` from user log responses, so
ordinary users cannot retrieve the detailed error. Administrator log responses
retain it. Detailed errors are passed through the existing sensitive
information masker and capped before persistence.

The default frontend log detail dialog shows `error_detail` only when
`isAdmin` is true.

## Settings UI

Add the switch to log settings in both frontend themes:

- Label: `Error details visible only to administrators`
- Description: enabling returns one generic error for every failed relay
  request and stores original details in administrator logs.

The default frontend uses the existing React Hook Form, Zod, and
`useUpdateOption` flow. All supported default-theme locales receive the new
strings. The classic frontend uses its existing operation-log settings form.

## Safety and compatibility

- The disabled branch is a strict compatibility path.
- HTTP status codes remain unchanged when masking is enabled.
- Original errors remain available to retry, billing refund, channel-disable,
  server logging, and administrator logging code.
- The response writer never buffers successful audio, image, video, or other
  large binary responses.
- Administrator details are masked and length-limited; “administrator-only”
  does not mean secrets may be persisted.

## Verification

Backend tests cover:

- disabled mode preserving the original body;
- enabled mode preserving status while replacing OpenAI, Claude, Task, and
  Midjourney errors;
- authentication or distribution errors being masked because the response
  writer is installed first;
- successful JSON and binary responses passing through;
- successful SSE chunks passing through and SSE error events being replaced;
- user logs removing administrator error detail;
- administrator logs retaining masked error detail;
- configuration initialization and runtime updates.

Frontend verification covers type checking, linting, formatting, i18n
synchronization, and production builds for modified themes.
