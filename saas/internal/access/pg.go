package access

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pqcota/pqcota/pkg/org"
)

// Schema — 이 패키지가 쓰는 테이블. [Migrate]가 올린다.
//
// 조직 컬럼이 셋 다에 있다. 토큰이 조직을 유도하는 자리라 여기가 어긋나면 그 뒤가
// 전부 어긋난다.
const Schema = `
CREATE TABLE IF NOT EXISTS pqcaton_runner_token (
    lookup        TEXT PRIMARY KEY,
    org           TEXT NOT NULL,
    secret_sha256 BYTEA NOT NULL,
    label         TEXT NOT NULL DEFAULT '',
    issued_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at  TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_pqcaton_token_org ON pqcaton_runner_token(org);

-- 기본키에 public_key가 든다 — 같은 (조직, collector)에 유효한 키가 둘 이상인 구간이
-- 있어야 키를 바꿀 수 있다. 그 구간이 없으면 교체하는 순간 관측이 전부 거절된다.
CREATE TABLE IF NOT EXISTS pqcaton_collector_key (
    org           TEXT NOT NULL,
    collector_id  TEXT NOT NULL,
    public_key    TEXT NOT NULL,
    label         TEXT NOT NULL DEFAULT '',
    registered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at    TIMESTAMPTZ,
    PRIMARY KEY (org, collector_id, public_key)
);

CREATE TABLE IF NOT EXISTS pqcaton_runner (
    id            TEXT PRIMARY KEY,
    org           TEXT NOT NULL,
    token_lookup  TEXT NOT NULL,
    label         TEXT NOT NULL DEFAULT '',
    version       TEXT NOT NULL DEFAULT '',
    registered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_pqcaton_runner_org ON pqcaton_runner(org, id);
`

// ErrSchemaMissing — 테이블이 없다.
var ErrSchemaMissing = errors.New("스키마가 없다 — Migrate를 먼저 돌려야 한다")

// PgStore — Postgres 구현.
type PgStore struct{ pool *pgxpool.Pool }

// Migrate — 스키마를 올린다.
//
// **저장소를 여는 것과 나눠 둔다.** 생성자가 말없이 DDL을 돌면, 가리키는 곳이 어긋났을 때
// 빈 테이블이 새로 생기고 거기에 쓴다 — 데이터가 사라진 것처럼 보인다. 스키마 배포는
// 의도적 행위여야 한다(pqcaton-saas infra.md §5).
func Migrate(ctx context.Context, dsn string) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, Schema)
	return err
}

// NewPgStore — 연다. 스키마가 없으면 만들지 않고 [ErrSchemaMissing]으로 끊는다.
func NewPgStore(ctx context.Context, dsn string) (*PgStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	var exists *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('pqcaton_runner_token')::text`).Scan(&exists); err != nil {
		pool.Close()
		return nil, err
	}
	if exists == nil {
		pool.Close()
		return nil, ErrSchemaMissing
	}
	return &PgStore{pool: pool}, nil
}

// Close — 연결을 닫는다.
func (p *PgStore) Close() { p.pool.Close() }

func nullable(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func fromNull(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func (p *PgStore) PutToken(r TokenRecord) error {
	if r.Org == "" {
		return org.ErrEmpty
	}
	_, err := p.pool.Exec(context.Background(),
		`INSERT INTO pqcaton_runner_token(lookup,org,secret_sha256,label,issued_at,last_used_at,revoked_at)
		 VALUES($1,$2,$3,$4,COALESCE($5,now()),$6,$7)`,
		r.Lookup, string(r.Org), r.Digest, r.Label,
		nullable(r.IssuedAt), nullable(r.LastUsed), nullable(r.Revoked))
	return err
}

func (p *PgStore) TokenByLookup(lookup string) (TokenRecord, error) {
	var (
		rec           TokenRecord
		o             string
		last, revoked *time.Time
	)
	err := p.pool.QueryRow(context.Background(),
		`SELECT lookup,org,secret_sha256,label,issued_at,last_used_at,revoked_at
		   FROM pqcaton_runner_token WHERE lookup=$1`, lookup).
		Scan(&rec.Lookup, &o, &rec.Digest, &rec.Label, &rec.IssuedAt, &last, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return TokenRecord{}, ErrUnknownToken
	}
	if err != nil {
		return TokenRecord{}, err
	}
	rec.Org, rec.LastUsed, rec.Revoked = org.ID(o), fromNull(last), fromNull(revoked)
	return rec, nil
}

func (p *PgStore) RevokeToken(lookup string, at time.Time) error {
	tag, err := p.pool.Exec(context.Background(),
		`UPDATE pqcaton_runner_token SET revoked_at=$2 WHERE lookup=$1`, lookup, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnknownToken
	}
	return nil
}

func (p *PgStore) TouchToken(lookup string, at time.Time) error {
	tag, err := p.pool.Exec(context.Background(),
		`UPDATE pqcaton_runner_token SET last_used_at=$2 WHERE lookup=$1`, lookup, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnknownToken
	}
	return nil
}

func (p *PgStore) RegisterKey(k CollectorKey) error {
	if k.Org == "" {
		return org.ErrEmpty
	}
	_, err := p.pool.Exec(context.Background(),
		`INSERT INTO pqcaton_collector_key(org,collector_id,public_key,label,registered_at,revoked_at)
		 VALUES($1,$2,$3,$4,COALESCE($5,now()),$6)
		 ON CONFLICT(org,collector_id,public_key)
		 DO UPDATE SET label=$4, revoked_at=$6`,
		string(k.Org), k.CollectorID, k.PublicKey, k.Label,
		nullable(k.Registered), nullable(k.Revoked))
	return err
}

func (p *PgStore) ActiveKeys(o org.ID, collectorID string) ([]string, error) {
	if o == "" {
		return nil, org.ErrEmpty
	}
	rows, err := p.pool.Query(context.Background(),
		`SELECT public_key FROM pqcaton_collector_key
		  WHERE org=$1 AND collector_id=$2 AND revoked_at IS NULL
		  ORDER BY registered_at`, string(o), collectorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (p *PgStore) RevokeKey(o org.ID, collectorID, publicKey string, at time.Time) error {
	tag, err := p.pool.Exec(context.Background(),
		`UPDATE pqcaton_collector_key SET revoked_at=$4
		  WHERE org=$1 AND collector_id=$2 AND public_key=$3`,
		string(o), collectorID, publicKey, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PgStore) PutRunner(r Runner) error {
	if r.Org == "" {
		return org.ErrEmpty
	}
	_, err := p.pool.Exec(context.Background(),
		`INSERT INTO pqcaton_runner(id,org,token_lookup,label,version,registered_at,last_seen_at)
		 VALUES($1,$2,$3,$4,$5,COALESCE($6,now()),$7)
		 ON CONFLICT(id) DO UPDATE SET version=$5, last_seen_at=$7`,
		r.ID, string(r.Org), r.TokenLookup, r.Label, r.Version,
		nullable(r.Registered), nullable(r.LastSeen))
	return err
}

func (p *PgStore) Runner(id string) (Runner, error) {
	var (
		r    Runner
		o    string
		seen *time.Time
	)
	err := p.pool.QueryRow(context.Background(),
		`SELECT id,org,token_lookup,label,version,registered_at,last_seen_at
		   FROM pqcaton_runner WHERE id=$1`, id).
		Scan(&r.ID, &o, &r.TokenLookup, &r.Label, &r.Version, &r.Registered, &seen)
	if errors.Is(err, pgx.ErrNoRows) {
		return Runner{}, ErrNotFound
	}
	if err != nil {
		return Runner{}, fmt.Errorf("러너 조회: %w", err)
	}
	r.Org, r.LastSeen = org.ID(o), fromNull(seen)
	return r, nil
}

func (p *PgStore) TouchRunner(id, version string, at time.Time) error {
	tag, err := p.pool.Exec(context.Background(),
		`UPDATE pqcaton_runner
		    SET last_seen_at=$2, version=COALESCE(NULLIF($3,''), version)
		  WHERE id=$1`, id, at, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
