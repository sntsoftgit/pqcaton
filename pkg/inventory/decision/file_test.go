package decision_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/randyinthedev-hash/pqcota/pkg/org"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

func fileStore(t *testing.T, o string) (*decision.FileJudgmentStore, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "judgments.jsonl")
	s, err := decision.NewFileJudgmentStore(org.ID(o), p)
	if err != nil {
		t.Fatalf("열기: %v", err)
	}
	return s, p
}

// IC-D6 — 파일 저장소는 **쌓기만 한다.** 같은 대상을 다시 판정해도 앞 줄이 사라지지 않는다.
//
// 판정 이력이 감사 근거인데 덮어쓰면 "언제 무엇으로 바뀌었나"가 사라진다(§1.2).
func TestFileStoreAppendsNeverOverwrites(t *testing.T) {
	s, path := fileStore(t, "acme")
	for i, c := range []string{"허용(예외)", "제거대상"} {
		if err := s.Save(&decision.Judgment{
			ID: "j" + string(rune('1'+i)), Subject: "node/openssl/libssl",
			Conclusion: c, Reviewer: "김", DecidedAt: int64(100 + i),
		}); err != nil {
			t.Fatalf("저장: %v", err)
		}
	}
	all, err := s.All()
	if err != nil {
		t.Fatalf("읽기: %v", err)
	}
	if len(all) != 2 || all[0].Conclusion != "허용(예외)" || all[1].Conclusion != "제거대상" {
		t.Fatalf("앞 판정이 사라졌다: %+v", all)
	}
	// 최신은 파생이다 — 저장소가 고르지 않는다.
	latest := decision.LatestPerSubject([]decision.Judgment{*all[0], *all[1]})
	if len(latest) != 1 || latest[0].Conclusion != "제거대상" {
		t.Fatalf("최신 파생이 틀렸다: %+v", latest)
	}
	raw, _ := os.ReadFile(path)
	if n := countLines(raw); n != 2 {
		t.Fatalf("파일에 %d줄 — 한 판정에 한 줄이어야 한다", n)
	}
}

// IC-D7 — **다른 조직의 판정이 섞인 파일은 읽지 않는다.**
//
// 파일은 누구나 이어 쓸 수 있다. 읽는 쪽에서 거르지 않으면 격리가 파일 권한에만 기댄다.
func TestFileStoreRefusesAnotherOrgsRecords(t *testing.T) {
	a, path := fileStore(t, "acme")
	if err := a.Save(&decision.Judgment{ID: "j1", Subject: "s", Conclusion: "허용"}); err != nil {
		t.Fatal(err)
	}
	b, err := decision.NewFileJudgmentStore(org.ID("beta"), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Save(&decision.Judgment{ID: "j2", Subject: "s", Conclusion: "제거"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.All(); !errors.Is(err, decision.ErrOrgMismatch) {
		t.Fatalf("남의 판정을 그대로 읽었다: %v", err)
	}
}

// IC-D8 — 조직 없이 열리지 않는다. 아직 아무것도 없는 파일은 오류가 아니다.
func TestFileStoreNeedsOrgAndToleratesMissingFile(t *testing.T) {
	if _, err := decision.NewFileJudgmentStore("", "/tmp/x.jsonl"); err == nil {
		t.Fatal("조직 없이 열렸다")
	}
	s, err := decision.NewFileJudgmentStore(org.ID("acme"), filepath.Join(t.TempDir(), "none.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	all, err := s.All()
	if err != nil || len(all) != 0 {
		t.Fatalf("아직 없는 파일을 오류로 봤다: %v %v", all, err)
	}
}

func countLines(b []byte) int {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

// IC-D25 — **옛 원장 줄(칸 없음)은 평가된 값으로 읽히고, 새 미평가 줄은 명시적 false 그대로다.**
//
// Judgment에는 json 태그가 없어 bool로 두면 옛 줄의 부재가 false로 읽힌다. 그러면 옛 판정 전부가
// 미평가로 둔갑해 만료 시 신뢰도가 감쇠되지 않는다. wire 형식이 포인터로 받아 부재를 참으로 읽는다.
func TestFileStoreReadsMissingEvaluatedAsTrue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "j.jsonl")
	old := `{"org":"acme","judgment":{"ID":"s@1","Subject":"s","Conclusion":"실존","Confidence":0.9,"DecidedAt":1}}` + "\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := decision.NewFileJudgmentStore(org.ID("acme"), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&decision.Judgment{ID: "u@2", Subject: "u", Confidence: 0.3, ConfidenceEvaluated: false, DecidedAt: 2, RecordKind: decision.RecordJudgment}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&decision.Judgment{ID: "s@3#plan", Subject: "s", Confidence: 0.9, ConfidenceEvaluated: true, DecidedAt: 3, RecordKind: decision.RecordPlanSelection}); err != nil {
		t.Fatal(err)
	}
	all, err := st.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("행 %d, want 3", len(all))
	}
	if !all[0].ConfidenceEvaluated || all[0].Kind() != decision.RecordJudgment {
		t.Errorf("옛 줄이 평가된 판정으로 읽히지 않았다: %+v", all[0])
	}
	if all[1].ConfidenceEvaluated {
		t.Errorf("명시적 false 가 보존되지 않았다: %+v", all[1])
	}
	if all[2].Kind() != decision.RecordPlanSelection || !all[2].ConfidenceEvaluated {
		t.Errorf("계획 선택 행이 그대로 읽히지 않았다: %+v", all[2])
	}
	// 새로 쓴 줄에는 부재가 없다 - 두 번째 읽기도 같은 답이다.
	raw, _ := os.ReadFile(path)
	if n := strings.Count(string(raw), `"ConfidenceEvaluated"`); n != 2 {
		t.Errorf("새 줄 둘에 명시적 값이 있어야 한다: %d", n)
	}
}
