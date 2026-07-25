# Connectors

The connector layer is Korugan's load-bearing abstraction: it turns heterogeneous provider APIs into one normalized model of **state** (snapshots), **signals** (events) and **change** (actions). Everything above it — analyzers, the AI engine, the action pipeline, the UI — is provider-agnostic. If a capability can't be expressed through this contract, it doesn't go in the core.

## Concepts

| Term | Meaning |
|---|---|
| **Provider** | An edge platform (Cloudflare, CloudFront, Fastly, …) |
| **Connector** | The in-tree Go adapter implementing this spec for one provider |
| **Capability** | A discrete feature a connector declares it supports (e.g. `waf:write`) |
| **Resource** | A managed unit within a provider — zone, distribution, service, site |
| **Snapshot** | Point-in-time normalized configuration state of a resource |
| **Event** | One normalized signal (a WAF block, an origin error, a cert nearing expiry) |
| **Action** | One normalized, provider-executable change with a diff and (usually) a rollback |

## Capabilities

Connectors declare capabilities; the platform degrades gracefully around what's missing — a provider without `waf:write` simply never receives WAF recommendations.

```
dns:read        dns:write        waf:read        waf:write
cache:read      cache:purge      cache:rules     ratelimit:read
ratelimit:write analytics:read   events:read     events:stream
ssl:read        bot:read         logs:stream     config:read
```

(Colon-separated `area:verb` strings; the enum lives in `internal/connector`. `*:write` capabilities additionally require per-action-type rollback support to be usable at L2+.)

## The interface

```go
// Package connector defines the port. Adapters live in subpackages
// (connector/cloudflare, connector/cloudfront, ...).

type Connector interface {
    // Info is static provider metadata (name, docs URL, auth spec).
    Info() ProviderInfo

    // Capabilities may depend on the credential's scopes; called after Validate.
    Capabilities(ctx context.Context) ([]Capability, error)

    // Validate checks credentials and minimal scopes without side effects.
    Validate(ctx context.Context) error

    // Resources enumerates manageable units (zones, distributions, services).
    Resources(ctx context.Context) ([]Resource, error)

    // Snapshot pulls current configuration state for one resource.
    Snapshot(ctx context.Context, res ResourceRef) (*Snapshot, error)

    // Events returns normalized events for a resource since a cursor.
    // Implementations page internally; the iterator respects ctx.
    Events(ctx context.Context, res ResourceRef, cur Cursor, f EventFilter) (EventIterator, error)

    // DryRun predicts an action's effect without applying. Connectors for
    // providers without native dry-run compute the diff locally.
    DryRun(ctx context.Context, a Action) (*Diff, error)

    // Apply executes an action. Must be idempotent under retry via
    // a.IdempotencyKey. Returns rollback material when a.Reversible.
    Apply(ctx context.Context, a Action) (*ActionResult, error)

    // Rollback reverts a previously applied action using its result.
    Rollback(ctx context.Context, prev ActionResult) error
}
```

Notes:

- **Credentials are injected at construction**, not passed per-call; the AI engine can't reach a constructed connector — only the executor and ingest scheduler hold references.
- **`Events` is pull-based** in v0.1 (poll with cursors). Providers with push (webhooks, log streaming) feed the same normalizer through the webhook receiver; the connector documents which mode it uses per event category.
- **Idempotency is mandatory.** Retries happen; `Apply` must be safe to repeat.

## Event schema

One normalized envelope; provider fidelity preserved in `raw`.

```jsonc
{
  "id": "evt_01J...",                 // ULID, Korugan-assigned
  "provider": "cloudflare",
  "provider_event_id": "...",          // dedup key within (provider, resource)
  "resource": { "kind": "zone", "external_id": "023e...", "name": "example.com" },
  "category": "waf.block",             // taxonomy below
  "severity": "info|low|medium|high|critical",
  "ts": "2026-07-25T03:12:45Z",
  "actor": {                            // who/what triggered it, when known
    "ip": "203.0.113.7", "country": "TR", "asn": 65001,
    "user_agent": "Mozilla/5.0 ..."
  },
  "target": { "host": "example.com", "path": "/wp-login.php", "method": "POST" },
  "rule": { "id": "4711", "name": "SQLi generic", "action_taken": "block" },
  "fields": { },                        // category-specific normalized extras
  "raw": { }                           // untouched provider payload (JSONB)
}
```

### Category taxonomy (initial)

```
waf.block  waf.challenge  waf.log            ratelimit.hit
bot.detected             origin.error        origin.timeout
cache.miss_spike         cache.purge         dns.changed
ssl.cert_expiring        ssl.handshake_fail  config.drift
traffic.anomaly          provider.incident
```

Rules: lowercase dot-namespaced; adding a category is a PR to this file first; connectors must not invent ad-hoc categories (unknown provider signals map to the closest category with details in `fields`, or are dropped with a counter metric — silent taxonomy sprawl is worse than lossy mapping).

## Action schema

```jsonc
{
  "id": "act_01J...",
  "type": "waf.rule.create",           // action taxonomy mirrors categories
  "resource": { "kind": "zone", "external_id": "023e..." },
  "params": { },                        // validated against per-type JSON Schema
  "diff": { "before": null, "after": { "expression": "(http.request.uri.path eq \"/wp-login.php\" and not cf.client.bot)", "action": "managed_challenge" } },
  "reversible": true,
  "idempotency_key": "sha256:...",
  "origin": { "recommendation_id": "rec_...", "approved_by": "user_...", "autonomy_level": "L2" }
}
```

Initial action types (all reversible by construction): `waf.rule.create|update|disable`, `cache.rule.create|update|disable`, `cache.purge` (reversible = no; allowed at L2 as low-risk exception, never L3), `ratelimit.rule.create|update|disable`, `dns.record.update` (no create/delete in early phases). Hard-excluded permanently: `dns.record.delete`, certificate operations, zone/account lifecycle.

## Credentials & auth

- Per-provider `AuthSpec` in `Info()` declares required credential shape and **minimum scopes**; onboarding UI renders from it.
- Secrets sealed with AES-256-GCM (see AI_ENGINE.md — same `crypto` module), stored in Postgres, decrypted only inside connector construction, never logged, masked everywhere (`cf_...last4`).
- Validation must fail loudly on over-broad tokens? No — warn: Korugan *recommends* least privilege but accepts what users paste; the UI shows which granted scopes exceed need.

## Rate limits, paging, retries

- Every connector declares a client-side budget below the provider's documented limit (Cloudflare REST: 1,200 requests / 5 minutes / user → Korugan budgets ~900) with jittered exponential backoff on 429/5xx, honoring `Retry-After`.
- Cursor-based paging everywhere; cursors persist in Postgres so restarts resume, never re-scan.
- All calls carry OpenTelemetry spans (`provider`, `endpoint`, `resource`) and a shared per-provider circuit breaker: repeated failures flip the connector to degraded, ingest pauses, a `provider.incident` event is emitted, action execution is refused.

## Cloudflare reference mapping (wave 1)

| Korugan concept | Cloudflare surface |
|---|---|
| `Resources` | `GET /zones` |
| `Snapshot` | zone settings, DNS records (`GET /zones/:id/dns_records`), rulesets (`GET /zones/:id/rulesets`), SSL/cert status |
| `Events` — `waf.*`, `ratelimit.hit`, `bot.detected` | GraphQL Analytics (`firewallEventsAdaptive` dataset), cursor = time window + pagination token |
| `Events` — `traffic.anomaly`, `cache.miss_spike`, `origin.*` | GraphQL HTTP requests datasets (adaptive sampling noted in `fields.sample_interval`) |
| `Events` — `ssl.cert_expiring` | derived by ingest from cert status in snapshots (synthetic event) |
| `waf.rule.*` actions | Rulesets API, zone-level custom phase (`http_request_firewall_custom`) |
| `cache.rule.*` | Rulesets API cache phase / Cache Rules |
| `ratelimit.rule.*` | Rulesets API rate limiting phase |
| `dns.record.update` | `PATCH /zones/:id/dns_records/:rid` (rollback = stored prior record) |
| `cache.purge` | `POST /zones/:id/purge_cache` |
| Recommended token scopes | Zone:Read, DNS:Read, Analytics:Read, Firewall Services:Read for L0/L1; corresponding `:Edit` scopes only when the user enables L2 per area |

Two-token pattern encouraged: a read token at onboarding, a separate write token added only when enabling L2 — the UI treats these as distinct credentials so L0/L1 deployments physically hold no write capability.

## Conformance suite

`connector/conformance` (ships in P3, skeleton in P1) runs any connector against: interface contract checks, event normalization golden files, cursor resume correctness, idempotent-apply under injected retry, rollback round-trip per declared action type, rate-limit budget respect (fake clock), and credential-redaction in error paths. **A connector PR without a green conformance run is not reviewable.** Write capabilities without passing rollback tests are stripped at registration.

## Adding a provider (checklist)

1. Open a design issue mapping the provider to the schemas above — mismatches surface here.
2. Implement `Connector` in `internal/connector/<name>/`, constructor `New(cfg, creds)`.
3. Golden-file tests from recorded (sanitized) API fixtures; no live-API tests in CI.
4. Pass conformance; document capability gaps in the connector README.
5. Update the provider table in README.md.
