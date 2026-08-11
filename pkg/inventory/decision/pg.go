package decision

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const judgmentSchemaSQL = `
CREATE TABLE IF NOT EXISTS pqcota_judgments (
    seq         BIGSERIAL PRIMARY KEY,
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
CREATE INDEX IF NOT EXISTS idx_pqcota_judg_subject ON pqcota_judgments(subject, seq);
`

// PgJudgmentStore — Postgres append-only 판정 저장소(§3.6, §7, §0.2).
// INSERT만 한다 — 판정은 갱신/삭제하지 않고 새 레코드를 쌓는다. 파생 플래그(Stale/NeedsReReview)는
// 저장하지 않고 델타/만료 계산으로 재산출한다.
type PgJudgmentStore struct{ pool *pgxpool.Pool }

func NewPgJudgmentStore(ctx context.Context, dsn string) (*PgJudgmentStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, judgmentSchemaSQL); err != nil {
		pool.Close()
		return nil, err
	}
	return &PgJudgmentStore{pool: pool}, nil
}

func (p *PgJudgmentStore) Close() { p.pool.Close() }

func (p *PgJudgmentStore) Save(j *Judgment) error {
	_, err := p.pool.Exec(context.Background(),
		`INSERT INTO pqcota_judgments(id,subject,conclusion,reviewer,signature,basis_hash,confidence,decided_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		j.ID, j.Subject, j.Conclusion, j.Reviewer, j.Signature, j.BasisHash, j.Confidence, j.DecidedAt)
	return err
}

func (p *PgJudgmentStore) Get(id string) (*Judgment, error) {
	row := p.pool.QueryRow(context.Background(),
		`SELECT id,subject,conclusion,reviewer,signature,basis_hash,confidence,decided_at
		 FROM pqcota_judgments WHERE id=$1 ORDER BY seq DESC LIMIT 1`, id)
	return scanJudgment(row)
}

func (p *PgJudgmentStore) BySubject(subject string) ([]*Judgment, error) {
	return p.query(`SELECT id,subject,conclusion,reviewer,signature,basis_hash,confidence,decided_at
		FROM pqcota_judgments WHERE subject=$1 ORDER BY seq ASC`, subject)
}

func (p *PgJudgmentStore) All() ([]*Judgment, error) {
	return p.query(`SELECT id,subject,conclusion,reviewer,signature,basis_hash,confidence,decided_at
		FROM pqcota_judgments ORDER BY seq ASC`)
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
