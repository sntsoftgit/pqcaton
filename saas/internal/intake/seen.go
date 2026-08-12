package intake

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pqcota/pqcota/pkg/org"
)

// SeenStore — 이미 받은 결과인지 아는 곳.
//
// **조직이 키에 든다.** 두 조직이 같은 결과를 올릴 일은 없지만, 없다고 전제하지 않는다 —
// 여기서 섞이면 한 조직의 결과가 다른 조직에서 "이미 받았다"로 사라진다.
type SeenStore interface {
	Seen(o org.ID, fingerprint string) (bool, error)
	Mark(o org.ID, fingerprint string) error
}

// SeenSchema — 멱등 표. [MigrateSeen]이 올린다.
//
// 지문만 남기고 결과 본문은 두지 않는다. 본문은 히스토리에 있고, 여기 필요한 것은
// "이미 봤나" 하나다.
const SeenSchema = `
CREATE TABLE IF NOT EXISTS pqcaton_result_seen (
    org         TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org, fingerprint)
);
`

// MemSeen — 인메모리. Pg판과 같은 규칙을 지킨다.
type MemSeen struct {
	mu sync.Mutex
	m  map[string]bool
}

// NewMemSeen — 빈 저장소.
func NewMemSeen() *MemSeen { return &MemSeen{m: map[string]bool{}} }

func key(o org.ID, fp string) string { return string(o) + "\x00" + fp }

func (s *MemSeen) Seen(o org.ID, fp string) (bool, error) {
	if o == "" {
		return false, org.ErrEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[key(o, fp)], nil
}

func (s *MemSeen) Mark(o org.ID, fp string) error {
	if o == "" {
		return org.ErrEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key(o, fp)] = true
	return nil
}

// PgSeen — Postgres 구현.
type PgSeen struct{ pool *pgxpool.Pool }

// ErrSeenSchemaMissing — 표가 없다.
var ErrSeenSchemaMissing = errors.New("멱등 표가 없다 — MigrateSeen을 먼저 돌려야 한다")

// MigrateSeen — 표를 올린다. 저장소를 여는 것과 나눠 둔다(access.Migrate와 같은 이유).
func MigrateSeen(ctx context.Context, dsn string) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, SeenSchema)
	return err
}

// NewPgSeen — 연다. 표가 없으면 만들지 않고 끊는다.
func NewPgSeen(ctx context.Context, dsn string) (*PgSeen, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	var exists *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('pqcaton_result_seen')::text`).Scan(&exists); err != nil {
		pool.Close()
		return nil, err
	}
	if exists == nil {
		pool.Close()
		return nil, ErrSeenSchemaMissing
	}
	return &PgSeen{pool: pool}, nil
}

// Close — 연결을 닫는다.
func (s *PgSeen) Close() { s.pool.Close() }

func (s *PgSeen) Seen(o org.ID, fp string) (bool, error) {
	if o == "" {
		return false, org.ErrEmpty
	}
	var n int
	err := s.pool.QueryRow(context.Background(),
		`SELECT 1 FROM pqcaton_result_seen WHERE org=$1 AND fingerprint=$2`,
		string(o), fp).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		// 조회 실패를 "이미 봤다"로 읽지 않는다 — 중복 적재는 이력이 감당하지만
		// 유실은 되돌릴 수 없다. 오류는 그대로 올린다.
		return false, err
	}
	return true, nil
}

func (s *PgSeen) Mark(o org.ID, fp string) error {
	if o == "" {
		return org.ErrEmpty
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO pqcaton_result_seen(org,fingerprint) VALUES($1,$2)
		 ON CONFLICT(org,fingerprint) DO NOTHING`, string(o), fp)
	return err
}
