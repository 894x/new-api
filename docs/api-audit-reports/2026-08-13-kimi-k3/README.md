# kimi-k3 Chat API audit

- Date: 2026-08-13 (Asia/Shanghai)
- Endpoint: `https://oneapi.xunlitec.com/v1/chat/completions`
- Model requested: `kimi-k3`
- Authentication: Bearer credential supplied through `API_AUDIT_API_KEY`; the credential is omitted from all artifacts
- Suite: OpenAI Chat default case `T001`

## Result

The endpoint returned an OpenAI-compatible HTTP 200 response in 10.507 seconds and the assistant content matched the case expectation (`5`). The final response used `finish_reason=stop`. Usage was 91 prompt tokens, 35 completion tokens, and 126 total tokens; this response did not expose a separate reasoning-token count.

The initial run used `max_tokens=16`. It returned HTTP 200, but the reasoning model consumed the available completion budget before emitting visible content, producing `finish_reason=length` and an empty assistant message. A direct diagnostic retry with `max_tokens=128` succeeded. A subsequent run also showed that this model only accepts its default temperature, so the built-in T001 case now omits `temperature` and allows 256 completion tokens before generating the committed report.

See:

- `report.html` for the standalone rendered report
- `report.json` for the machine-readable result
- `raw/T001/exchange-01.json` for the redacted HTTP exchange
