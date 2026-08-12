package decision

import "testing"

// IC-D1: 판정 후 재수집(새 스냅샷) → 결론은 대상에 부착 유지. 관측이 사라져도 결론은 남음.
func TestDeltaReview_conclusionSurvivesRescan(t *testing.T) {
	basisA := HashBasis("libcrypto.so.3@node1")
	prior := []Judgment{{ID: "j1", Subject: "node1/openssl/libcrypto", Conclusion: "허용(예외)", BasisHash: basisA}}

	// 재수집 결과 이 subject가 관측되지 않음(currentBasis에 없음).
	got := DeltaReview(prior, map[string]string{})
	if len(got) != 1 || got[0].Conclusion != "허용(예외)" {
		t.Fatalf("결론이 유지돼야: %+v", got)
	}
	if got[0].NeedsReReview {
		t.Error("관측만 사라진 경우 재리뷰 플래그 세우면 안 됨(결론 부착 유지)")
	}
}

// IC-D2: 근거(BasisHash) 실질 변화 → 해당 판정만 재검토 플래그, 나머지 유지.
func TestDeltaReview_basisChanged(t *testing.T) {
	prior := []Judgment{
		{ID: "j1", Subject: "s1", Conclusion: "허용", BasisHash: HashBasis("v1")},
		{ID: "j2", Subject: "s2", Conclusion: "제거", BasisHash: HashBasis("x1")},
	}
	current := map[string]string{
		"s1": HashBasis("v2"), // 변화
		"s2": HashBasis("x1"), // 불변
	}
	got := DeltaReview(prior, current)
	if !got[0].NeedsReReview {
		t.Error("s1 근거 변화 → 재검토 플래그여야")
	}
	if got[1].NeedsReReview {
		t.Error("s2 근거 불변 → 재검토 안 함(나머지 유지)")
	}
	// 결론은 어느 쪽도 사라지지 않음.
	if got[0].Conclusion != "허용" || got[1].Conclusion != "제거" {
		t.Error("델타 리뷰는 결론을 지우지 않음")
	}
}

// IC-D3: 근거 불변 → 판정 유지(재리뷰 안 함).
func TestDeltaReview_basisUnchanged(t *testing.T) {
	h := HashBasis("same")
	prior := []Judgment{{ID: "j1", Subject: "s1", BasisHash: h}}
	got := DeltaReview(prior, map[string]string{"s1": h})
	if got[0].NeedsReReview {
		t.Error("근거 불변인데 재리뷰 플래그가 섰다")
	}
}

// IC-D4: stale 판정 + 만료 경과 → 신뢰도 감쇠 + 재확인 플래그.
func TestExpireStale(t *testing.T) {
	js := []Judgment{
		{ID: "old", Subject: "s1", Confidence: 1.0, DecidedAt: 0},     // 오래됨
		{ID: "fresh", Subject: "s2", Confidence: 1.0, DecidedAt: 900}, // 최근
	}
	got := ExpireStale(js, 1000, 500, 0.5) // now=1000, ttl=500s, decay=0.5
	if !got[0].Stale || !got[0].NeedsReReview {
		t.Error("만료 판정은 stale + 재확인이어야")
	}
	if got[0].Confidence != 0.5 {
		t.Errorf("만료 신뢰도 감쇠 = %.2f, want 0.5", got[0].Confidence)
	}
	if got[1].Stale || got[1].Confidence != 1.0 {
		t.Error("미만료 판정은 그대로여야")
	}
}

func TestHashBasis_orderIndependent(t *testing.T) {
	if HashBasis("a", "b", "c") != HashBasis("c", "a", "b") {
		t.Error("BasisHash는 순서 무관해야(집합 동일성)")
	}
	if HashBasis("a", "b") == HashBasis("a", "b", "c") {
		t.Error("항목이 다르면 해시가 달라야")
	}
}

// LatestPerSubject: append-only 로그에서 subject별 최신만 뽑음.
func TestLatestPerSubject(t *testing.T) {
	log := []Judgment{
		{ID: "j1", Subject: "s1", Conclusion: "허용"},
		{ID: "j2", Subject: "s2", Conclusion: "제거"},
		{ID: "j3", Subject: "s1", Conclusion: "제거"}, // s1 재판정
	}
	got := LatestPerSubject(log)
	if len(got) != 2 {
		t.Fatalf("subject 2개여야: %+v", got)
	}
	for _, j := range got {
		if j.Subject == "s1" && j.ID != "j3" {
			t.Errorf("s1 최신은 j3여야, got %s", j.ID)
		}
	}
}

// MemStore append-only: 같은 subject 재판정 시 이전 판정도 보존.
func TestMemJudgmentStore_appendOnly(t *testing.T) {
	st, err := NewMemJudgmentStore("acme")
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Save(&Judgment{ID: "j1", Subject: "s1", Conclusion: "허용"})
	_ = st.Save(&Judgment{ID: "j2", Subject: "s1", Conclusion: "제거"})
	hist, _ := st.BySubject("s1")
	if len(hist) != 2 {
		t.Fatalf("append-only: 이전 판정 보존돼 2개여야, got %d", len(hist))
	}
}
