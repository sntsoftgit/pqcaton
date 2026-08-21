package review_test

import (
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// queued — 판정이 채워진 세션 하나. 정책 하나에 항목 둘.
func queued() review.Session {
	sf := review.Session{
		Reviewer: "보안팀", Signature: "sig-1",
		PolicyDecisions: map[string]string{"openssl/1.1": "PQC 라이브러리로 교체한다"},
		Items: []review.Item{
			{ID: "a", Policy: "openssl/1.1", State: "UNDECLARED", Conclusion: "판정 a", Plan: true},
			{ID: "b", Policy: "openssl/1.1", State: "UNDECLARED", Conclusion: "판정 b"},
		},
	}
	return sf
}

// IC-Q5 — **관측이 갱신돼도 적어 둔 판정은 남는다.**
//
// 큐는 관측에서 파생된 것이라 결과가 늘면 다시 세워야 합니다. 그때마다 판정을 처음부터
// 다시 적게 하면 아무도 화면을 쓰지 않습니다.
func TestCarryKeepsJudgments(t *testing.T) {
	prev := queued()
	next := review.Carry(prev, review.Session{
		PolicyDecisions: map[string]string{"openssl/1.1": ""},
		Items: []review.Item{
			{ID: "a", Policy: "openssl/1.1", State: "UNDECLARED"},
			{ID: "b", Policy: "openssl/1.1", State: "UNDECLARED"},
		},
	})
	if next.Items[0].Conclusion != "판정 a" || !next.Items[0].Plan {
		t.Errorf("판정과 계획 표시가 날아갔다: %+v", next.Items[0])
	}
	if next.PolicyDecisions["openssl/1.1"] == "" {
		t.Error("정책 일괄 결론이 날아갔다 — 못 보던 항목이 생긴 것이 아니다")
	}
	if next.Signature != "sig-1" || next.Reviewer != "보안팀" {
		t.Error("큐가 같은데 승인이 지워졌다")
	}
}

// IC-Q6 — **정책에 못 보던 항목이 생기면 그 정책의 일괄 결론을 지운다.**
//
// 일괄 판정은 「이 정책의 항목들을 보고 내린 결론」입니다. 새로 관측된 UNDECLARED 는 사람이
// 본 적이 없는데, 그대로 두면 **누가 승인한 적 없는 근거를 달고** 확정을 통과합니다.
func TestCarryClearsPolicyDecisionOnNewItem(t *testing.T) {
	prev := queued()
	next := review.Carry(prev, review.Session{
		PolicyDecisions: map[string]string{"openssl/1.1": ""},
		Items: []review.Item{
			{ID: "a", Policy: "openssl/1.1", State: "UNDECLARED"},
			{ID: "b", Policy: "openssl/1.1", State: "UNDECLARED"},
			{ID: "c", Policy: "openssl/1.1", State: "UNDECLARED", Mandatory: true},
		},
	})
	if next.PolicyDecisions["openssl/1.1"] != "" {
		t.Fatalf("못 보던 항목이 생겼는데 일괄 결론이 남았다: %q", next.PolicyDecisions["openssl/1.1"])
	}
	if next.Signature != "" {
		t.Error("큐가 달라졌는데 서명이 남았다")
	}
	if next.Reviewer != "보안팀" {
		t.Error("승인자 이름까지 지웠다 — 그건 사람이지 확인이 아니다")
	}
	if _, err := review.Finalize(next); err == nil {
		t.Error("아무도 본 적 없는 필수 항목이 확정을 통과했다")
	}
}

// IC-Q7 — 사라진 항목의 판정은 따라오지 않는다. 더는 올라온 것이 아니다.
func TestCarryDropsVanishedItems(t *testing.T) {
	next := review.Carry(queued(), review.Session{
		PolicyDecisions: map[string]string{"openssl/1.1": ""},
		Items:           []review.Item{{ID: "a", Policy: "openssl/1.1", State: "UNDECLARED"}},
	})
	if len(next.Items) != 1 || next.Items[0].ID != "a" {
		t.Fatalf("항목이 되살아났다: %+v", next.Items)
	}
}
