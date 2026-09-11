package decision_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/randyinthedev-hash/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// 원장 행이 세션 id 를 들고 가고, 그것으로 찾을 수 있어야 「계획에서 원장으로 되짚는다」가
// 기능이 된다. 저장만 하고 찾는 길이 없으면 가능성에 그친다.

func sessionRows(prefix string) []*decision.Judgment {
	return []*decision.Judgment{
		{ID: prefix + "#a", Subject: prefix + "/a", Conclusion: "교체", Reviewer: "r", Signature: "s",
			BasisHash: decision.HashBasis("x"), Confidence: 0.9, DecidedAt: 1, SessionID: "sess-A"},
		{ID: prefix + "#b", Subject: prefix + "/b", Conclusion: "교체", Reviewer: "r", Signature: "s",
			BasisHash: decision.HashBasis("y"), Confidence: 0.9, DecidedAt: 2, SessionID: "sess-A"},
		{ID: prefix + "#c", Subject: prefix + "/c", Conclusion: "허용", Reviewer: "r", Signature: "s",
			BasisHash: decision.HashBasis("z"), Confidence: 0.9, DecidedAt: 3, SessionID: "sess-B"},
		// 옛 빌드가 남긴 행. 세션을 모른다.
		{ID: prefix + "#old", Subject: prefix + "/old", Conclusion: "허용", Reviewer: "r", Signature: "s",
			BasisHash: decision.HashBasis("w"), Confidence: 0.9, DecidedAt: 0},
	}
}

// 세 저장소가 같은 답을 내야 한다. 파일과 Postgres 가 갈리면 화면과 명령이 다른 원장을 본다.
func checkBySession(t *testing.T, st decision.JudgmentStore, prefix string) {
	t.Helper()
	for _, j := range sessionRows(prefix) {
		if err := st.Save(j); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.BySessionID("sess-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("sess-A 의 판정은 둘이어야 한다: %d", len(got))
	}
	for _, j := range got {
		if j.SessionID != "sess-A" {
			t.Errorf("다른 세션의 판정이 섞였다: %+v", j)
		}
	}
	// ★ 빈 id 로 찾으면 세션이 아니라 「세션을 모르는 판정 전부」가 나온다. 세션이 아닌 것을
	// 세션이라고 돌려주지 않는다.
	if _, err := st.BySessionID(""); !errors.Is(err, decision.ErrNoSessionID) {
		t.Errorf("빈 세션 id 를 거절하지 않았다: %v", err)
	}
	// 왕복. 저장한 세션 id 가 읽어도 그대로여야 한다.
	all, _ := st.All()
	for _, j := range all {
		if j.ID == prefix+"#c" && j.SessionID != "sess-B" {
			t.Errorf("세션 id 가 왕복에서 사라졌다: %+v", j)
		}
	}
}

// IC-D22 — 메모리 원장.
func TestMemLedgerFindsBySession(t *testing.T) {
	st, err := decision.NewMemJudgmentStore(org.ID("acme"))
	if err != nil {
		t.Fatal(err)
	}
	checkBySession(t, st, "mem")
}

// IC-D22 — 파일 원장. JSON 줄에 세션 id 가 실려 나가고 다시 읽힌다.
func TestFileLedgerFindsBySession(t *testing.T) {
	st, err := decision.NewFileJudgmentStore(org.ID("acme"), filepath.Join(t.TempDir(), "j.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	checkBySession(t, st, "file")
}

// IC-D22 — Postgres 원장. 열과 (org, session_id, seq) 인덱스. PQCOTA_TEST_DSN 이 있을 때만.
func TestPgLedgerFindsBySession(t *testing.T) {
	dsn := os.Getenv("PQCOTA_TEST_DSN")
	if dsn == "" {
		t.Skip("PQCOTA_TEST_DSN 미설정 — Postgres 통합 테스트 스킵")
	}
	st, err := decision.NewPgJudgmentStore(context.Background(), dsn, "acme")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// append-only 라 정리하지 않는다. 실행마다 유일한 접두어로 다른 실행의 행과 섞이지 않게 한다.
	// 세션 id 도 실행마다 달라야 하므로 접두어를 붙인다.
	prefix := "pgsess-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	rows := sessionRows(prefix)
	for _, j := range rows {
		if j.SessionID != "" {
			j.SessionID = prefix + "/" + j.SessionID
		}
		if err := st.Save(j); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.BySessionID(prefix + "/sess-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("sess-A 의 판정은 둘이어야 한다: %d", len(got))
	}
	if _, err := st.BySessionID(""); !errors.Is(err, decision.ErrNoSessionID) {
		t.Errorf("빈 세션 id 를 거절하지 않았다: %v", err)
	}
}
