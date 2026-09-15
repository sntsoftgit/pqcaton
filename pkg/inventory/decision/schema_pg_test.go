package decision

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// IC-D27 — **준비 상태 검사는 이 연결이 보는 표의 열만 센다.** 같은 데이터베이스에 같은 이름의 표가
// 두 스키마에 있고(하나는 이 판의 모양, 하나는 옛 모양) 둘 다 행 수준 보안이 켜져 있을 때, 옛 표를
// 보는 연결은 준비되지 않았다고, 새 표를 보는 연결은 준비됐다고 답해야 한다. 표 이름으로만 세면
// 앞은 다른 스키마의 열을 보고 준비됐다고 하고, 뒤는 열이 넷이라 준비되지 않았다고 한다.
func TestSchemaReadyLooksOnlyAtTheResolvedTable(t *testing.T) {
	dsn := os.Getenv("PQCOTA_TEST_DSN")
	if dsn == "" {
		t.Skip("PQCOTA_TEST_DSN 미설정 — Postgres 통합 테스트 스킵")
	}
	ctx := context.Background()
	stamp := strconv.FormatInt(time.Now().UnixNano(), 36)
	fresh, stale := "pqcaton_ready_"+stamp, "pqcaton_stale_"+stamp
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	for _, sc := range []string{fresh, stale} {
		if _, err := admin.Exec(ctx, "CREATE SCHEMA "+sc); err != nil {
			t.Fatal(err)
		}
		defer admin.Exec(ctx, "DROP SCHEMA "+sc+" CASCADE") //nolint:errcheck
	}
	base := func(sc string) string {
		return `CREATE TABLE ` + sc + `.pqcota_judgments (
		    seq BIGSERIAL PRIMARY KEY, org TEXT NOT NULL, id TEXT NOT NULL, subject TEXT NOT NULL,
		    conclusion TEXT NOT NULL, reviewer TEXT NOT NULL, signature TEXT NOT NULL, basis_hash TEXT NOT NULL,
		    confidence DOUBLE PRECISION NOT NULL, decided_at BIGINT NOT NULL, session_id TEXT NOT NULL DEFAULT '');
		ALTER TABLE ` + sc + `.pqcota_judgments ENABLE ROW LEVEL SECURITY;`
	}
	if _, err := admin.Exec(ctx, base(stale)); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, base(fresh)+`
		ALTER TABLE `+fresh+`.pqcota_judgments ADD COLUMN record_kind TEXT NOT NULL DEFAULT '';
		ALTER TABLE `+fresh+`.pqcota_judgments ADD COLUMN confidence_evaluated BOOLEAN NOT NULL DEFAULT TRUE;`); err != nil {
		t.Fatal(err)
	}

	look := func(sc string) bool {
		pool, err := pgxpool.New(ctx, dsn+"&options=-csearch_path%3D"+sc)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		ready, err := schemaReady(ctx, pool)
		if err != nil {
			t.Fatal(err)
		}
		return ready
	}
	if look(stale) {
		t.Error("옛 모양의 표를 보는 연결이 준비됐다고 했다 - 다른 스키마의 열을 센 것이다")
	}
	if !look(fresh) {
		t.Error("이 판의 표를 보는 연결이 준비되지 않았다고 했다 - 두 스키마의 열을 다 센 것이다")
	}
}
