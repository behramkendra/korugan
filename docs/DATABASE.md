# Database

PostgreSQL 16+. Schema lives in `internal/store/migrations/*.sql`, applied
by the embedded migrator (`internal/store/migrate.go`) at boot; versions
tracked in `schema_migrations`. No down migrations by design.

## Tables (0001)

| Table | Purpose | Notes |
|---|---|---|
| `resources` | provider units (zones, …) | `UNIQUE(provider, kind, external_id)`; internal IDs are ULIDs |
| `events` | normalized signals | dedup `UNIQUE(provider, provider_event_id)`; `BRIN(ts)` + btree `(resource_id, ts DESC)`, `(category, ts DESC)`; raw payload preserved as JSONB |
| `findings` | detected issues | partial unique `(resource_id, kind) WHERE state='open'` → analyzers refresh instead of duplicating |
| `recommendations` | provider-ready change proposals | requires rationale + rollback plan |
| `actions` | executable changes | `idempotency_key UNIQUE`; state machine pending→…→verified/rolled_back |
| `audit_log` | every consequential transition | append-only |
| `llm_usage` | BYOK token/cost accounting | per provider/model/task_class |
| `secrets` | AES-256-GCM sealed credentials | ciphertext+nonce only; master key never stored |
| `sync_cursors` | per-(provider,resource,stream) resume position | restart-safe ingestion |

## Scaling plan

Plain `events` table + BRIN carries early volumes. Triggers for change
(documented in ARCHITECTURE.md): monthly native partitions first, ClickHouse
only after sustained multi-million events/day. Collation note: server
databases were REFRESH COLLATION VERSION'd after the glibc 2.43 upgrade.
