// Package store owns PostgreSQL access: pool, migrations, repositories.
// Repositories speak domain types; SQL stays inside this package.
package store

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/behramkendra/korugan/internal/domain"
)

type Store struct {
	Pool *pgxpool.Pool
}

// Open connects, migrates, and returns a ready Store.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	mConn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect for migrate: %w", err)
	}
	if err := Migrate(ctx, mConn); err != nil {
		_ = mConn.Close(ctx)
		return nil, err
	}
	_ = mConn.Close(ctx)

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() { s.Pool.Close() }

// NewID returns a ULID: sortable by creation time, safe in URLs.
func NewID() string { return ulid.MustNew(ulid.Now(), rand.Reader).String() }

// --- resources ---

// UpsertResource inserts or refreshes a resource, returning its internal ID.
func (s *Store) UpsertResource(ctx context.Context, r domain.ResourceRef) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	id := NewID()
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO resources (id, provider, kind, external_id, name)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (provider, kind, external_id)
		DO UPDATE SET name = EXCLUDED.name
		RETURNING id`,
		id, r.Provider, r.Kind, r.ExternalID, r.Name).Scan(&id)
	return id, err
}

func (s *Store) ListResources(ctx context.Context) ([]ResourceRow, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, provider, kind, external_id, name, created_at
		FROM resources ORDER BY provider, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ResourceRow
	for rows.Next() {
		var r ResourceRow
		if err := rows.Scan(&r.ID, &r.Ref.Provider, &r.Ref.Kind, &r.Ref.ExternalID, &r.Ref.Name, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type ResourceRow struct {
	ID        string             `json:"id"`
	Ref       domain.ResourceRef `json:"ref"`
	CreatedAt time.Time          `json:"created_at"`
}

// --- events ---

// InsertEvents appends normalized events, silently skipping duplicates
// (same provider + provider_event_id). Returns the number inserted.
func (s *Store) InsertEvents(ctx context.Context, resourceID string, evs []domain.Event) (int, error) {
	if len(evs) == 0 {
		return 0, nil
	}
	batch := &pgx.Batch{}
	for _, e := range evs {
		if err := e.Validate(); err != nil {
			return 0, fmt.Errorf("event %s: %w", e.ProviderEventID, err)
		}
		actor, _ := json.Marshal(e.Actor)
		target, _ := json.Marshal(e.Target)
		rule, _ := json.Marshal(e.Rule)
		fields, _ := json.Marshal(e.Fields)
		id := e.ID
		if id == "" {
			id = NewID()
		}
		batch.Queue(`
			INSERT INTO events (id, provider, provider_event_id, resource_id,
				category, severity, ts, actor, target, rule, fields, raw)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (provider, provider_event_id) DO NOTHING`,
			id, e.Resource.Provider, e.ProviderEventID, resourceID,
			e.Category, e.Severity, e.TS, actor, target, rule, fields, []byte(e.Raw))
	}
	br := s.Pool.SendBatch(ctx, batch)
	defer br.Close()
	inserted := 0
	for range evs {
		ct, err := br.Exec()
		if err != nil {
			return inserted, err
		}
		inserted += int(ct.RowsAffected())
	}
	return inserted, nil
}

type EventFilter struct {
	ResourceID string
	Category   string
	Since      time.Time
	Until      time.Time
	Limit      int
}

func (s *Store) ListEvents(ctx context.Context, f EventFilter) ([]domain.Event, error) {
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 200
	}
	q := `SELECT id, provider, provider_event_id, category, severity, ts,
			actor, target, rule, fields,
			r.provider, r.kind, r.external_id, r.name
		FROM events e JOIN resources r ON r.id = e.resource_id WHERE 1=1`
	args := []any{}
	n := 0
	add := func(cond string, v any) {
		n++
		q += fmt.Sprintf(" AND "+cond, n)
		args = append(args, v)
	}
	if f.ResourceID != "" {
		add("e.resource_id = $%d", f.ResourceID)
	}
	if f.Category != "" {
		add("e.category = $%d", f.Category)
	}
	if !f.Since.IsZero() {
		add("e.ts >= $%d", f.Since)
	}
	if !f.Until.IsZero() {
		add("e.ts < $%d", f.Until)
	}
	n++
	q += fmt.Sprintf(" ORDER BY e.ts DESC LIMIT $%d", n)
	args = append(args, f.Limit)

	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		var e domain.Event
		var actor, target, rule, fields []byte
		if err := rows.Scan(&e.ID, (*string)(&e.Resource.Provider), &e.ProviderEventID,
			(*string)(&e.Category), (*string)(&e.Severity), &e.TS,
			&actor, &target, &rule, &fields,
			(*string)(&e.Resource.Provider), &e.Resource.Kind, &e.Resource.ExternalID, &e.Resource.Name); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(actor, &e.Actor)
		_ = json.Unmarshal(target, &e.Target)
		_ = json.Unmarshal(rule, &e.Rule)
		_ = json.Unmarshal(fields, &e.Fields)
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountEventsByCategory aggregates a time window, feeding analyzers cheaply.
func (s *Store) CountEventsByCategory(ctx context.Context, resourceID string, since, until time.Time) (map[domain.Category]int64, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT category, count(*) FROM events
		WHERE resource_id=$1 AND ts >= $2 AND ts < $3
		GROUP BY category`, resourceID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.Category]int64{}
	for rows.Next() {
		var c string
		var n int64
		if err := rows.Scan(&c, &n); err != nil {
			return nil, err
		}
		out[domain.Category(c)] = n
	}
	return out, rows.Err()
}

// --- findings ---

// UpsertOpenFinding refreshes the single open finding per (resource, kind):
// severity, detail and evidence update in place instead of duplicating.
func (s *Store) UpsertOpenFinding(ctx context.Context, resourceID string, f domain.Finding) (string, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	id := NewID()
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO findings (id, resource_id, kind, severity, state, title, detail, evidence, source)
		VALUES ($1,$2,$3,$4,'open',$5,$6,$7,$8)
		ON CONFLICT (resource_id, kind) WHERE state='open'
		DO UPDATE SET severity=EXCLUDED.severity, title=EXCLUDED.title,
			detail=EXCLUDED.detail, evidence=EXCLUDED.evidence, updated_at=now()
		RETURNING id`,
		id, resourceID, f.Kind, f.Severity, f.Title, f.Detail, f.Evidence, f.Source).Scan(&id)
	return id, err
}

func (s *Store) ListFindings(ctx context.Context, state string, limit int) ([]FindingRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if state == "" {
		state = "open"
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT f.id, f.kind, f.severity, f.state, f.title, f.detail, f.evidence, f.source,
			f.created_at, f.updated_at, r.provider, r.kind, r.external_id, r.name
		FROM findings f JOIN resources r ON r.id = f.resource_id
		WHERE f.state = $1 ORDER BY f.updated_at DESC LIMIT $2`, state, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FindingRow
	for rows.Next() {
		var fr FindingRow
		if err := rows.Scan(&fr.ID, &fr.Kind, (*string)(&fr.Severity), (*string)(&fr.State),
			&fr.Title, &fr.Detail, &fr.Evidence, &fr.Source, &fr.CreatedAt, &fr.UpdatedAt,
			(*string)(&fr.Resource.Provider), &fr.Resource.Kind, &fr.Resource.ExternalID, &fr.Resource.Name); err != nil {
			return nil, err
		}
		out = append(out, fr)
	}
	return out, rows.Err()
}

type FindingRow struct {
	ID        string              `json:"id"`
	Resource  domain.ResourceRef  `json:"resource"`
	Kind      string              `json:"kind"`
	Severity  domain.Severity     `json:"severity"`
	State     domain.FindingState `json:"state"`
	Title     string              `json:"title"`
	Detail    string              `json:"detail"`
	Evidence  []string            `json:"evidence"`
	Source    string              `json:"source"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// --- cursors ---

func (s *Store) GetCursor(ctx context.Context, provider domain.Provider, resourceExternalID, stream string) (string, error) {
	var cur string
	err := s.Pool.QueryRow(ctx, `
		SELECT cursor FROM sync_cursors
		WHERE provider=$1 AND resource_external_id=$2 AND stream=$3`,
		provider, resourceExternalID, stream).Scan(&cur)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return cur, err
}

func (s *Store) SetCursor(ctx context.Context, provider domain.Provider, resourceExternalID, stream, cursor string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO sync_cursors (provider, resource_external_id, stream, cursor)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (provider, resource_external_id, stream)
		DO UPDATE SET cursor=EXCLUDED.cursor, updated_at=now()`,
		provider, resourceExternalID, stream, cursor)
	return err
}

// --- secrets (sealed by internal/crypto before they reach here) ---

func (s *Store) PutSecret(ctx context.Context, name string, ciphertext, nonce []byte) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO secrets (name, ciphertext, nonce) VALUES ($1,$2,$3)
		ON CONFLICT (name) DO UPDATE SET ciphertext=EXCLUDED.ciphertext,
			nonce=EXCLUDED.nonce, updated_at=now()`, name, ciphertext, nonce)
	return err
}

func (s *Store) GetSecret(ctx context.Context, name string) (ciphertext, nonce []byte, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT ciphertext, nonce FROM secrets WHERE name=$1`, name).
		Scan(&ciphertext, &nonce)
	if err == pgx.ErrNoRows {
		return nil, nil, nil
	}
	return
}

// --- llm usage ---

func (s *Store) RecordLLMUsage(ctx context.Context, provider, model, taskClass string, tokensIn, tokensOut int64, estCostUSD float64) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO llm_usage (provider, model, task_class, tokens_in, tokens_out, est_cost_usd)
		VALUES ($1,$2,$3,$4,$5,$6)`, provider, model, taskClass, tokensIn, tokensOut, estCostUSD)
	return err
}

// --- audit ---

func (s *Store) Audit(ctx context.Context, actor, kind, subjectID string, detail any) error {
	d, _ := json.Marshal(detail)
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO audit_log (actor, kind, subject_id, detail) VALUES ($1,$2,$3,$4)`,
		actor, kind, subjectID, d)
	return err
}
