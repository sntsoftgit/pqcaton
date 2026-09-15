package decision_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// IC-D26 — **v0.17 모양의 원장(record_kind · confidence_evaluated 열 없음)을 이 판이 열면**, 옛 행은
// 평가된 판정으로 읽히고, 새 미평가 행과 계획 선택 행은 명시적으로 저장된다. 열의 기본값이
// TRUE 라 기존 행이 그 자리에서 이행된다 - 그때는 미평가라는 개념이 없었다.
//
// 전용 스키마에서 돌아 공유 표의 모양을 흔들지 않는다. PQCOTA_TEST_DSN 이 있을 때만.
func TestPgUpgradeFromV017(t *testing.T) {
	dsn := os.Getenv("PQCOTA_TEST_DSN")
	if dsn == "" {
		t.Skip("PQCOTA_TEST_DSN 미설정 — Postgres 통합 테스트 스킵")
	}
	ctx := context.Background()
	schema := "pqcaton_mig_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") //nolint:errcheck

	// v0.17 의 표. 이 판이 더한 두 열이 없다.
	old := `CREATE TABLE ` + schema + `.pqcota_judgments (
	    seq BIGSERIAL PRIMARY KEY, org TEXT NOT NULL, id TEXT NOT NULL, subject TEXT NOT NULL,
	    conclusion TEXT NOT NULL, reviewer TEXT NOT NULL, signature TEXT NOT NULL, basis_hash TEXT NOT NULL,
	    confidence DOUBLE PRECISION NOT NULL, decided_at BIGINT NOT NULL,
	    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), session_id TEXT NOT NULL DEFAULT '');
	INSERT INTO ` + schema + `.pqcota_judgments(org,id,subject,conclusion,reviewer,signature,basis_hash,confidence,decided_at)
	    VALUES ('acme','old@1','old-subject','실존','kty','sig','h1',0.9,1);`
	if _, err := admin.Exec(ctx, old); err != nil {
		t.Fatal(err)
	}

	// 이 판으로 연다. search_path 로 전용 스키마를 보게 한다.
	st, err := decision.NewPgJudgmentStore(ctx, dsn+"&options=-csearch_path%3D"+schema, "acme")
	if err != nil {
		t.Fatalf("옛 표 위에 열리지 않는다: %v", err)
	}
	defer st.Close()

	var cols int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema=$1 AND table_name='pqcota_judgments' AND column_name IN ('record_kind','confidence_evaluated')`, schema).Scan(&cols); err != nil {
		t.Fatal(err)
	}
	if cols != 2 {
		t.Fatalf("열이 더해지지 않았다: %d", cols)
	}

	if err := st.Save(&decision.Judgment{ID: "u@2", Subject: "u", Confidence: 0.3, ConfidenceEvaluated: false, DecidedAt: 2, RecordKind: decision.RecordJudgment}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&decision.Judgment{ID: "old@3#plan", Subject: "old-subject", Confidence: 0.9, ConfidenceEvaluated: true, DecidedAt: 3, RecordKind: decision.RecordPlanSelection}); err != nil {
		t.Fatal(err)
	}
	all, err := st.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("행 %d, want 3", len(all))
	}
	if !all[0].ConfidenceEvaluated || all[0].Kind() != decision.RecordJudgment || all[0].Conclusion != "실존" {
		t.Errorf("옛 행이 평가된 판정으로 읽히지 않았다: %+v", all[0])
	}
	if all[1].ConfidenceEvaluated || all[1].Kind() != decision.RecordJudgment {
		t.Errorf("새 미평가 판정이 그대로 읽히지 않았다: %+v", all[1])
	}
	if all[2].Kind() != decision.RecordPlanSelection || all[2].Conclusion != "" {
		t.Errorf("계획 선택 행이 그대로 읽히지 않았다: %+v", all[2])
	}
	latest := decision.LatestPerSubject(deref(all))
	if len(latest) != 2 {
		t.Fatalf("최신 판정 %d, want 2 (계획 선택 행은 세지 않는다)", len(latest))
	}
}
