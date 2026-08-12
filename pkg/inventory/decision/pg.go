package decision

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pqcota/pqcota/pkg/org"
)

const judgmentSchemaSQL = `
CREATE TABLE IF NOT EXISTS pqcota_judgments (
    seq         BIGSERIAL PRIMARY KEY,
    org         TEXT NOT NULL,
    id          TEXT NOT NULL,
    subject     TEXT NOT NULL,
    conclusion  TEXT NOT NULL,
    reviewer    TEXT NOT NULL,
    signature   TEXT NOT NULL,
    basis_hash  TEXT NOT NULL,
    confidence  DOUBLE PRECISION NOT NULL,
    decided_at  BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE pqcota_judgments ADD COLUMN IF NOT EXISTS org TEXT NOT NULL DEFAULT '';
-- 인덱스 선두가 org다 — 모든 질의가 조직으로 먼저 걸린다.
CREATE INDEX IF NOT EXISTS idx_pqcota_judg_org_subject ON pqcota_judgments(org, subject, seq);
`

// PgJudgmentStore — Postgres append-only 판정 저장소(§3.6, §7, §0.2).
// INSERT만 한다 — 판정은 갱신/삭제하지 않고 새 레코드를 쌓는다. 파생 플래그(Stale/NeedsReReview)는
// 저장하지 않고 델타/만료 계산으로 재산출한다.
//
// ★ 핸들이 조직에 묶인다. 아래 질의 전부가 org를 조건에 달고 있고, 그것을 빼는 방법이 없다 —
// 다른 조직의 판정은 이 핸들로 읽을 수도 쓸 수도 없다. 격리를 질의마다 기억하지 않게 하려는 것이다.
type PgJudgmentStore struct {
	pool *pgxpool.Pool
	org  org.ID
}

// NewPgJudgmentStore — 조직을 지정해 연다. 조직 없이는 열리지 않는다(org.ErrEmpty).
func NewPgJudgmentStore(ctx context.Context, dsn string, o org.ID) (*PgJudgmentStore, error) {
	if o == "" {
		return nil, org.ErrEmpty
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, judgmentSchemaSQL); err != nil {
		pool.Close()
		return nil, err
	}
	return &PgJudgmentStore{pool: pool, org: o}, nil
}

func (p *PgJudgmentStore) Close() { p.pool.Close() }

// Org — 이 핸들이 묶인 조직.
func (p *PgJudgmentStore) Org() org.ID { return p.org }

func (p *PgJudgmentStore) Save(j *Judgment) error {
	_, err := p.pool.Exec(context.Background(),
		`INSERT INTO pqcota_judgments(org,id,subject,conclusion,reviewer,signature,basis_hash,confidence,decided_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		p.org, j.ID, j.Subject, j.Conclusion, j.Reviewer, j.Signature, j.BasisHash, j.Confidence, j.DecidedAt)
	return err
}

func (p *PgJudgmentStore) Get(id string) (*Judgment, error) {
	row := p.pool.QueryRow(context.Background(),
		`SELECT id,subject,conclusion,reviewer,signature,basis_hash,confidence,decided_at
		 FROM pqcota_judgments WHERE org=$1 AND id=$2 ORDER BY seq DESC LIMIT 1`, p.org, id)
	return scanJudgment(row)
}

func (p *PgJudgmentStore) BySubject(subject string) ([]*Judgment, error) {
	return p.query(`SELECT id,subject,conclusion,reviewer,signature,basis_hash,confidence,decided_at
		FROM pqcota_judgments WHERE org=$1 AND subject=$2 ORDER BY seq ASC`, p.org, subject)
}

func (p *PgJudgmentStore) All() ([]*Judgment, error) {
	return p.query(`SELECT id,subject,conclusion,reviewer,signature,basis_hash,confidence,decided_at
		FROM pqcota_judgments WHERE org=$1 ORDER BY seq ASC`, p.org)
}

func (p *PgJudgmentStore) query(sql string, args ...any) ([]*Judgment, error) {
	rows, err := p.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Judgment
	for rows.Next() {
		j, err := scanJudgment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

type scannable interface{ Scan(dest ...any) error }

func scanJudgment(r scannable) (*Judgment, error) {
	var j Judgment
	if err := r.Scan(&j.ID, &j.Subject, &j.Conclusion, &j.Reviewer,
		&j.Signature, &j.BasisHash, &j.Confidence, &j.DecidedAt); err != nil {
		return nil, err
	}
	return &j, nil
}
