package intake

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pqcota/pqcota/pkg/org"
)

// SeenStore — 이미 받은 결과인지 아는 곳.
//
// **조직이 키에 든다.** 두 조직이 같은 결과를 올릴 일은 없지만, 없다고 전제하지 않는다 —
// 여기서 섞이면 한 조직의 결과가 다른 조직에서 "이미 받았다"로 사라진다.
//
// **「봤나 물어보고 나중에 표시하기」를 두지 않는다.** 그 사이가 벌어지면 동시에 온 같은
// 결과가 둘 다 통과한다 — 러너가 응답을 못 받고 재시도하는 것이 정상 동작이라 실제로
// 겹친다. 그래서 [Claim]이 확인과 표시를 한 번에 한다.
type SeenStore interface {
	// Claim — 이 지문을 **처음 보는 것이면 표시하고 true**를 돌려준다. 이미 있으면 false다.
	// 원자적이어야 한다.
	Claim(o org.ID, fingerprint string) (bool, error)
	// Release — 적재하지 못한 것의 표시를 거둔다.
	//
	// 없으면 거절이 굳는다 — 키를 뒤늦게 등록하거나 노드를 등재한 뒤 같은 결과를 다시
	// 올려도 "이미 봤다"로 접힌다. 멱등은 재전송을 접는 장치이지 실패를 굳히는 장치가 아니다.
	Release(o org.ID, fingerprint string) error
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

func (s *MemSeen) Claim(o org.ID, fp string) (bool, error) {
	if o == "" {
		return false, org.ErrEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(o, fp)
	if s.m[k] {
		return false, nil
	}
	s.m[k] = true
	return true, nil
}

func (s *MemSeen) Release(o org.ID, fp string) error {
	if o == "" {
		return org.ErrEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key(o, fp))
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

// Claim — 한 문장으로 확인과 표시를 함께 한다.
//
// `ON CONFLICT DO NOTHING`이 이미 있는 행에는 아무것도 하지 않으므로, 영향 행 수가
// 그대로 "처음 보는 것인가"의 답이 된다. 두 요청이 동시에 와도 하나만 1을 받는다.
func (s *PgSeen) Claim(o org.ID, fp string) (bool, error) {
	if o == "" {
		return false, org.ErrEmpty
	}
	tag, err := s.pool.Exec(context.Background(),
		`INSERT INTO pqcaton_result_seen(org,fingerprint) VALUES($1,$2)
		 ON CONFLICT(org,fingerprint) DO NOTHING`, string(o), fp)
	if err != nil {
		// 실패를 "이미 봤다"로 읽지 않는다 — 중복 적재는 이력이 감당하지만 유실은
		// 되돌릴 수 없다. 오류는 그대로 올린다.
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PgSeen) Release(o org.ID, fp string) error {
	if o == "" {
		return org.ErrEmpty
	}
	_, err := s.pool.Exec(context.Background(),
		`DELETE FROM pqcaton_result_seen WHERE org=$1 AND fingerprint=$2`, string(o), fp)
	return err
}
