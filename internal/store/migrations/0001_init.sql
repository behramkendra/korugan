-- Korugan initial schema.
-- Events land in a plain table with BRIN(ts); native partitioning is a
-- planned migration once retention/volume demand it (documented in DATABASE.md).

CREATE TABLE resources (
    id          TEXT PRIMARY KEY,              -- ULID
    provider    TEXT NOT NULL,
    kind        TEXT NOT NULL,
    external_id TEXT NOT NULL,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, kind, external_id)
);

CREATE TABLE events (
    id                TEXT PRIMARY KEY,        -- ULID (time-ordered)
    provider          TEXT NOT NULL,
    provider_event_id TEXT NOT NULL,
    resource_id       TEXT NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    category          TEXT NOT NULL,
    severity          TEXT NOT NULL,
    ts                TIMESTAMPTZ NOT NULL,
    actor             JSONB,
    target            JSONB,
    rule              JSONB,
    fields            JSONB,
    raw               JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_event_id)
);
CREATE INDEX events_resource_ts_idx ON events (resource_id, ts DESC);
CREATE INDEX events_category_ts_idx ON events (category, ts DESC);
CREATE INDEX events_ts_brin ON events USING BRIN (ts);

CREATE TABLE findings (
    id          TEXT PRIMARY KEY,
    resource_id TEXT NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    severity    TEXT NOT NULL,
    state       TEXT NOT NULL DEFAULT 'open',
    title       TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    evidence    TEXT[] NOT NULL DEFAULT '{}',
    source      TEXT NOT NULL,                 -- 'rule' | 'ai'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- one open finding per (resource, kind): analyzers refresh instead of spamming
CREATE UNIQUE INDEX findings_open_dedup ON findings (resource_id, kind) WHERE state = 'open';

CREATE TABLE recommendations (
    id            TEXT PRIMARY KEY,
    finding_id    TEXT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
    action_type   TEXT NOT NULL,
    params        JSONB NOT NULL DEFAULT '{}',
    diff_before   JSONB,
    diff_after    JSONB,
    rationale     TEXT NOT NULL,
    rollback_plan TEXT NOT NULL,
    confidence    DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE actions (
    id                TEXT PRIMARY KEY,
    type              TEXT NOT NULL,
    resource_id       TEXT NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    params            JSONB NOT NULL DEFAULT '{}',
    state             TEXT NOT NULL DEFAULT 'pending',
    recommendation_id TEXT REFERENCES recommendations(id) ON DELETE SET NULL,
    approved_by       TEXT,
    idempotency_key   TEXT NOT NULL UNIQUE,
    autonomy_level    INT NOT NULL DEFAULT 0,
    result            JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
    id         BIGSERIAL PRIMARY KEY,
    ts         TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor      TEXT NOT NULL,                  -- user id | 'system' | 'ai'
    kind       TEXT NOT NULL,                  -- e.g. action.approved, key.saved
    subject_id TEXT,
    detail     JSONB
);
CREATE INDEX audit_log_ts_idx ON audit_log (ts DESC);

CREATE TABLE llm_usage (
    id         BIGSERIAL PRIMARY KEY,
    ts         TIMESTAMPTZ NOT NULL DEFAULT now(),
    provider   TEXT NOT NULL,
    model      TEXT NOT NULL,
    task_class TEXT NOT NULL,
    tokens_in  BIGINT NOT NULL DEFAULT 0,
    tokens_out BIGINT NOT NULL DEFAULT 0,
    est_cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0
);
CREATE INDEX llm_usage_ts_idx ON llm_usage (ts DESC);

CREATE TABLE secrets (
    name       TEXT PRIMARY KEY,
    ciphertext BYTEA NOT NULL,
    nonce      BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sync_cursors (
    provider             TEXT NOT NULL,
    resource_external_id TEXT NOT NULL,
    stream               TEXT NOT NULL,        -- e.g. 'firewall_events'
    cursor               TEXT NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, resource_external_id, stream)
);
