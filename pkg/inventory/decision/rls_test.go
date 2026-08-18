package decision_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// appRole — RLS 를 재려면 **정책을 건너뛰지 않는 롤**이 있어야 한다. 슈퍼유저로는 무엇을
// 걸어도 통과하므로, 통과하지 않는 롤을 만들어 그 롤로 다시 붙는다.
const appRole = "pqcaton_app_test"

const appPassword = "rls-test-only" // 테스트 컨테이너 전용. 배포 값이 아니다.

// appDSN — 슈퍼유저 DSN 으로 앱 롤을 만들고, 그 롤로 붙는 DSN 을 돌려준다.
func appDSN(t *testing.T, superDSN string) string {
	t.Helper()
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, superDSN)
	if err != nil {
		t.Fatalf("슈퍼유저로 붙지 못했다: %v", err)
	}
	defer admin.Close()

	// 스키마를 먼저 만들어 둔다 - 앱 롤에는 테이블 생성 권한을 주지 않는다.
	if _, err := decision.NewPgJudgmentStore(ctx, superDSN, "bootstrap"); err != nil {
		t.Fatalf("스키마 준비: %v", err)
	}

	stmts := []string{
		`DO $$ BEGIN
		   IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '` + appRole + `') THEN
		     CREATE ROLE ` + appRole + ` LOGIN PASSWORD '` + appPassword + `';
		   END IF;
		 END $$`,
		`ALTER ROLE ` + appRole + ` NOSUPERUSER NOBYPASSRLS`,
		`GRANT SELECT, INSERT ON pqcota_judgments TO ` + appRole,
		`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO ` + appRole,
	}
	for _, s := range stmts {
		if _, err := admin.Exec(ctx, s); err != nil {
			t.Skipf("앱 롤을 만들 권한이 없다 — RLS 케이스 스킵: %v", err)
		}
	}

	cfg, err := pgx.ParseConfig(superDSN)
	if err != nil {
		t.Fatal(err)
	}
	dsn := superDSN
	// user:password 만 바꿔 끼운다. 나머지(host·port·db·sslmode)는 그대로 둔다.
	if i := strings.Index(dsn, "://"); i >= 0 {
		rest := dsn[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			rest = rest[at+1:]
		}
		dsn = dsn[:i+3] + appRole + ":" + appPassword + "@" + rest
	} else {
		t.Skipf("DSN 형식을 몰라 앱 롤로 바꾸지 못했다: %s", cfg.Host)
	}
	return dsn
}

func superDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("PQCOTA_TEST_DSN")
	if dsn == "" {
		t.Skip("PQCOTA_TEST_DSN 미설정 — Postgres 통합 테스트 스킵")
	}
	return dsn
}

func uniq(prefix string) string {
	return prefix + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// IC-L1 — **핸들 격리를 건너뛰어도 DB 가 막는다.**
//
// 우리 질의는 전부 org 조건을 달고 있다. 그 조건을 뺀 날것의 질의를 같은 연결로 던져 —
// 곧 핸들 격리가 뚫린 상황을 흉내 내 — 그래도 남의 행이 보이지 않는지 잰다. **이 케이스가
// 통과하지 않으면 이 버전이 더한 한 겹은 없는 것이다.**
func TestRLSBlocksRawQueryAcrossOrgs(t *testing.T) {
	dsn := appDSN(t, superDSN(t))
	ctx := context.Background()

	subj := uniq("asset://rls-")
	a, err := decision.NewPgJudgmentStore(ctx, dsn, "acme")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if !a.RLSActive() {
		t.Fatal("앱 롤인데도 RLS 가 물지 않는다 — 케이스가 아무것도 재지 못한다")
	}
	if err := a.Save(&decision.Judgment{ID: uniq("j-"), Subject: subj, Conclusion: "acme 의 판정"}); err != nil {
		t.Fatal(err)
	}

	// beta 조직으로 붙어 **org 조건 없이** 전부 세어 본다.
	pool := rawPool(t, dsn, "beta")
	defer pool.Close()
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pqcota_judgments WHERE subject=$1`, subj).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("남의 조직 행이 %d건 보인다 — RLS 가 막지 못했다", n)
	}
}

// IC-L2 — **남의 조직 이름으로 쓰지도 못한다.**
//
// 읽기만 막으면 오염을 막지 못한다. 정책의 WITH CHECK 가 없으면 org 를 남의 것으로 적은
// INSERT 가 그대로 들어가고, 그 행은 정작 우리 눈에는 안 보인다 - 가장 고약한 형태다.
func TestRLSBlocksInsertForAnotherOrg(t *testing.T) {
	dsn := appDSN(t, superDSN(t))
	ctx := context.Background()

	pool := rawPool(t, dsn, "acme")
	defer pool.Close()

	_, err := pool.Exec(ctx,
		`INSERT INTO pqcota_judgments(org,id,subject,conclusion,reviewer,signature,basis_hash,confidence,decided_at)
		 VALUES('beta',$1,$2,'','','','',0,0)`, uniq("j-"), uniq("asset://rls-"))
	if err == nil {
		t.Fatal("남의 조직 이름으로 쓴 행이 들어갔다 — WITH CHECK 가 없다")
	}
}

// IC-L3 — **자기 조직 것은 그대로 보인다.** 막는 것만 재고 통과를 안 재면, 전부 막아도
// 케이스는 통과한다.
func TestRLSLetsOwnOrgThrough(t *testing.T) {
	dsn := appDSN(t, superDSN(t))
	ctx := context.Background()

	subj := uniq("asset://rls-")
	st, err := decision.NewPgJudgmentStore(ctx, dsn, "acme")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Save(&decision.Judgment{ID: uniq("j-"), Subject: subj, Conclusion: "보여야 한다"}); err != nil {
		t.Fatal(err)
	}
	got, err := st.BySubject(subj)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("자기 조직 판정이 %d건 — RLS 가 자기 것까지 막았다", len(got))
	}
}

// IC-L4 — **무력한 연결을 조용히 넘기지 않는다.**
//
// 슈퍼유저는 정책을 통째로 건너뛴다. 걸어 놓고 아무 일도 안 하는 것이 가장 위험한 종류의
// 거짓 안심이라, 필수 모드에서는 저장소가 열리지 않는다.
func TestRLSInertIsRefusedWhenRequired(t *testing.T) {
	dsn := superDSN(t) // 슈퍼유저 - 정책을 건너뛴다
	ctx := context.Background()

	st, err := decision.NewPgJudgmentStore(ctx, dsn, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if st.RLSActive() {
		t.Skip("이 DSN 의 롤이 슈퍼유저가 아니다 — 이 케이스는 잴 것이 없다")
	}
	st.Close()

	t.Setenv(decision.RequireEnv, "1")
	if _, err := decision.NewPgJudgmentStore(ctx, dsn, "acme"); err == nil {
		t.Fatalf("%s=1 인데 무력한 연결로 저장소가 열렸다", decision.RequireEnv)
	}
}

// rawPool — 저장소를 거치지 않고 직접 붙는다. 조직은 저장소와 같은 방식으로 심는다 —
// 심는 방식이 다르면 케이스가 실제 동작을 재지 않는다.
func rawPool(t *testing.T, dsn, o string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SELECT set_config($1, $2, false)", decision.OrgSetting, o)
		return err
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}
