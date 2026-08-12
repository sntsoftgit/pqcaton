package decision_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// IC-D5: 판정 영속화(Postgres) 라운드트립 — append-only 보존.
// PQCOTA_TEST_DSN이 있을 때만 실행. 기본 `go test`는 스킵.
func TestPgJudgmentStore(t *testing.T) {
	dsn := os.Getenv("PQCOTA_TEST_DSN")
	if dsn == "" {
		t.Skip("PQCOTA_TEST_DSN 미설정 — Postgres 통합 테스트 스킵")
	}
	ctx := context.Background()
	st, err := decision.NewPgJudgmentStore(ctx, dsn, "acme")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// 실행마다 유니크 subject(append-only라 정리 안 함).
	subj := "asset://pgtest-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	j1 := &decision.Judgment{
		ID: subj + "#1", Subject: subj, Conclusion: "허용(예외)",
		Reviewer: "kty", Signature: "sig-abc", BasisHash: decision.HashBasis("libcrypto.so.3"),
		Confidence: 0.9, DecidedAt: 1000,
	}
	if err := st.Save(j1); err != nil {
		t.Fatal(err)
	}
	// 같은 subject 재판정 → append-only로 둘 다 남아야.
	j2 := &decision.Judgment{
		ID: subj + "#2", Subject: subj, Conclusion: "제거대상",
		Reviewer: "kty", Signature: "sig-def", BasisHash: decision.HashBasis("libcrypto.so.3", "libssl.so.3"),
		Confidence: 0.8, DecidedAt: 2000,
	}
	if err := st.Save(j2); err != nil {
		t.Fatal(err)
	}

	hist, err := st.BySubject(subj)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("append-only: 판정 2개 보존돼야, got %d", len(hist))
	}
	// 필드 라운드트립 보존 검증(첫 판정).
	got := hist[0]
	if got.Conclusion != "허용(예외)" || got.Reviewer != "kty" || got.Signature != "sig-abc" ||
		got.BasisHash != j1.BasisHash || got.Confidence != 0.9 || got.DecidedAt != 1000 {
		t.Errorf("라운드트립 필드 불일치: %+v", got)
	}

	// Get(id)로 개별 조회.
	one, err := st.Get(subj + "#2")
	if err != nil {
		t.Fatal(err)
	}
	if one == nil || one.Conclusion != "제거대상" {
		t.Errorf("Get 결과 = %+v", one)
	}

	// 최신 판정 파생(LatestPerSubject) — s별 최신 = j2.
	latest := decision.LatestPerSubject(deref(hist))
	if len(latest) != 1 || latest[0].ID != subj+"#2" {
		t.Errorf("최신 판정 = %+v, want %s#2", latest, subj)
	}
}

func deref(ps []*decision.Judgment) []decision.Judgment {
	out := make([]decision.Judgment, len(ps))
	for i, p := range ps {
		out[i] = *p
	}
	return out
}
