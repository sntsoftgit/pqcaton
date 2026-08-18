package decision_test

import (
	"errors"
	"os"
	"path/filepath"
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
// 판정 이력이 감사 근거인데 덮어쓰면 "언제 무엇으로 바뀌었나"가 사라진다(§0.2).
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
