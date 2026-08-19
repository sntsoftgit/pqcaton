package decision

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
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

// rlsSQL — 행 수준 보안. **핸들 격리가 뚫려도 DB가 막는 한 겹**이다.
//
// 질의에 org 를 다는 것은 우리가 안 틀린다는 전제 위에 서 있다. 여러 조직이 한 데이터베이스를
// 쓰는 배포에서는 그 전제 하나에 전부를 걸 수 없다 — 조건 하나를 빠뜨린 질의가 언젠가 들어온다.
//
// **FORCE 가 없으면 테이블 소유자는 예외가 된다.** 대개 앱이 소유자로 붙으므로, 그 한 줄이
// 없으면 정책을 걸어 놓고도 아무 일도 일어나지 않는다.
const rlsSQL = `
ALTER TABLE pqcota_judgments ENABLE ROW LEVEL SECURITY;
ALTER TABLE pqcota_judgments FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS pqcaton_org_isolation ON pqcota_judgments;
CREATE POLICY pqcaton_org_isolation ON pqcota_judgments
    USING       (org = current_setting('pqcaton.org', true))
    WITH CHECK  (org = current_setting('pqcaton.org', true));
`

// OrgSetting — 정책이 읽는 세션 변수 이름. 연결마다 이 값이 조직으로 채워진다.
const OrgSetting = "pqcaton.org"

// RequireEnv — "1"이면 RLS 가 실제로 물지 않는 연결로는 **저장소를 열지 않는다.**
//
// pqcota의 `PQCOTA_REQUIRE_SIGNATURE` 와 같은 모양이다 — 조용히 통과하는 경로를 닫아야 하는
// 배포용이고, 두 리포를 오가는 사람이 같은 것을 같은 자리에서 찾게 한다.
const RequireEnv = "PQCATON_REQUIRE_RLS"

// RequireRLS — 필수 모드인가.
func RequireRLS() bool { return os.Getenv(RequireEnv) == "1" }

// ErrRLSInert — 정책은 걸렸지만 이 연결에서는 물지 않는다.
//
// 슈퍼유저와 BYPASSRLS 롤은 정책을 통째로 건너뛴다. **그런 롤로 붙으면 RLS 는 걸어 두어도
// 아무 일도 하지 않는다** — 가장 위험한 종류의 거짓 안심이라, 조용히 넘기지 않는다.
var ErrRLSInert = errors.New("이 롤에서는 RLS 가 물지 않는다(슈퍼유저 또는 BYPASSRLS)")

// PgJudgmentStore — Postgres append-only 판정 저장소(§3.6, §7, §0.2).
// INSERT만 한다 — 판정은 갱신/삭제하지 않고 새 레코드를 쌓는다. 파생 플래그(Stale/NeedsReReview)는
// 저장하지 않고 델타/만료 계산으로 재산출한다.
//
// ★ 핸들이 조직에 묶인다. 아래 질의 전부가 org를 조건에 달고 있고, 그것을 빼는 방법이 없다 —
// 다른 조직의 판정은 이 핸들로 읽을 수도 쓸 수도 없다. 격리를 질의마다 기억하지 않게 하려는 것이다.
type PgJudgmentStore struct {
	pool *pgxpool.Pool
	org  org.ID
	// rls — 이 연결에서 정책이 실제로 무는가. 열 때 한 번 재고 들고 다닌다.
	rls bool
}

// NewPgJudgmentStore — 조직을 지정해 연다. 조직 없이는 열리지 않는다(org.ErrEmpty).
func NewPgJudgmentStore(ctx context.Context, dsn string, o org.ID) (*PgJudgmentStore, error) {
	if o == "" {
		return nil, org.ErrEmpty
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	// **연결마다 조직을 심는다.** 핸들 하나가 조직 하나이므로 세션 단위로 두면 되고,
	// 질의마다 기억할 것이 없다. 값은 파라미터로 넘긴다 - 문자열을 이어 붙이면 조직
	// 이름이 SQL 이 되는 길이 열린다.
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SELECT set_config($1, $2, false)", OrgSetting, string(o))
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := ensureSchema(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	active, err := rlsBites(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if !active && RequireRLS() {
		pool.Close()
		return nil, fmt.Errorf("%w — %s=1 이므로 열지 않는다", ErrRLSInert, RequireEnv)
	}
	return &PgJudgmentStore{pool: pool, org: o, rls: active}, nil
}

// ensureSchema — 테이블과 정책을 갖춘다. **DDL 권한이 없어도 열린다.**
//
// RLS 가 실제로 물게 하려면 앱이 테이블 소유자로 붙지 않아야 한다 - 그러면 이 연결에는
// DDL 권한이 없는 것이 정상이다. 그때는 이미 갖춰져 있는지 확인하고 넘어간다. 갖춰지지도
// 않았다면 **무엇을 소유자로 돌려야 하는지 말하고 멈춘다** - 조용히 빈 테이블을 만들거나
// 정책 없이 여는 쪽이 훨씬 위험하다.
func ensureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, ddlErr := pool.Exec(ctx, judgmentSchemaSQL)
	if ddlErr == nil {
		if _, err := pool.Exec(ctx, rlsSQL); err != nil {
			return fmt.Errorf("행 수준 보안: %w", err)
		}
		return nil
	}
	ready, err := schemaReady(ctx, pool)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("스키마를 만들 수 없고 갖춰져 있지도 않다: %w\n"+
			"   소유자 권한으로 아래를 먼저 돌리십시오:\n%s%s", ddlErr, judgmentSchemaSQL, rlsSQL)
	}
	return nil
}

// schemaReady — 테이블이 있고 그 위에 행 수준 보안이 켜져 있는가.
//
// 테이블만 보고 넘어가면 **정책 없는 테이블에 조용히 붙는다** - 이 버전이 더하려던 한 겹이
// 없는 채로 있다는 사실을 아무도 모르게 된다.
func schemaReady(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var enabled bool
	err := pool.QueryRow(ctx,
		`SELECT relrowsecurity FROM pg_class WHERE oid = to_regclass('pqcota_judgments')`).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("스키마 확인: %w", err)
	}
	return enabled, nil
}

// RLSActive — 이 연결에서 행 수준 보안이 실제로 무는가.
//
// **false 면 격리는 질의의 org 조건 하나에만 기대고 있다.** 그것도 격리이긴 하지만, 이
// 버전이 더하려던 한 겹은 없는 것이다 - 부르는 쪽이 그 사실을 말할 수 있어야 한다.
func (p *PgJudgmentStore) RLSActive() bool { return p.rls }

// rlsBites — 지금 롤이 정책을 건너뛰는가. 슈퍼유저와 BYPASSRLS 가 그렇다.
func rlsBites(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var bypass bool
	err := pool.QueryRow(ctx,
		`SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&bypass)
	if err != nil {
		return false, fmt.Errorf("롤 확인: %w", err)
	}
	return !bypass, nil
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
