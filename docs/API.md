# HTTP API (v0.1)

Base: `/api/v1`. Auth: `Authorization: Bearer $KORUGAN_API_TOKEN` when the
env var is set (constant-time compare); open otherwise — localhost dev only.
All responses JSON. The future web UI consumes exactly these endpoints.

| Method | Path | Purpose |
|---|---|---|
| GET | `/healthz` | liveness + `ai_enabled` flag (no auth) |
| GET | `/api/v1/resources` | synced provider resources |
| GET | `/api/v1/events?resource_id&category&since&limit` | normalized events, newest first (limit ≤ 1000) |
| GET | `/api/v1/findings?state=open` | findings feed |
| POST | `/api/v1/chat` `{message, resource_id?}` | grounded AI answer; `503` with explanation in zero-key mode |

Chat grounding: last-24h events (≤100) + open findings (≤25), serialized
into untrusted-data blocks (see AI_ENGINE.md injection defense). Errors:
`400` bad input, `401` bad token, `503` zero-key, `500` opaque (details in
server logs only — never in responses).

Planned next: SSE streaming for chat, action approval endpoints (P2),
session auth for the UI.
