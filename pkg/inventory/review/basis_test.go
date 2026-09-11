package review_test

import (
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// 근거를 세는 자리가 하나인지 본다.
//
// 델타 리뷰(무엇을 다시 봐야 하나)와 서명 무효화(승인이 아직 유효한가)가 따로 세면 어긋난다.
// 실제로 어긋나 있었다: 원장은 신뢰도를 근거에 넣었는데 서명 비교는 id 와 상태만 봐서,
// 다시 보라고 큐에 올라온 항목에 옛 승인 서명이 그대로 붙어 있었다.

const rules = "pqcota-enrich/v1+pqcaton-plan/v1"

func base() review.Item {
	return review.Item{
		ID: "n1/openssl/libssl", Policy: "openssl/libssl", State: "UNDECLARED",
		Conf: 0.6, Fingerprint: "fp-1",
	}
}

// IC-D9 — 관측이 그대로면 몇 번을 다시 돌려도 근거가 흔들리지 않는다(IC-D3와 같은 규칙).
// 흔들리면 델타 큐가 매번 가득 차고, 그런 큐는 아무도 읽지 않는다.
func TestSameObservationKeepsTheSameBasis(t *testing.T) {
	if review.BasisOf(base(), rules) != review.BasisOf(base(), rules) {
		t.Error("같은 관측인데 근거 해시가 달라졌다")
	}
}

// IC-D9 — 근거를 이루는 것이 하나라도 움직이면 해시가 움직여야 한다.
func TestEveryPartOfTheBasisMovesTheHash(t *testing.T) {
	was := review.BasisOf(base(), rules)
	for _, tc := range []struct {
		what string
		it   review.Item
		ver  string
	}{
		{"규칙 판", base(), "pqcota-enrich/v2+pqcaton-plan/v1"},
		{"대조 상태", func() review.Item { i := base(); i.State = "CONFIRMED"; return i }(), rules},
		{"신뢰도", func() review.Item { i := base(); i.Conf = 0.9; return i }(), rules},
		{"정책", func() review.Item { i := base(); i.Policy = "jca/provider"; return i }(), rules},
		{"관측 내용 지문", func() review.Item { i := base(); i.Fingerprint = "fp-2"; return i }(), rules},
		{"재수집 후보 여부", func() review.Item { i := base(); i.Rescan = true; return i }(), rules},
	} {
		if review.BasisOf(tc.it, tc.ver) == was {
			t.Errorf("%s가 바뀌었는데 근거가 그대로다 — 다시 볼 일이 큐에 안 오른다", tc.what)
		}
	}
}

// IC-D10 — ★ 같은 규칙 아래 관측 근거만 바뀌는 경우.
//
// 상류의 `finding_id` 는 `sha256(노드|이름|런타임|fork)` 라 **자산이 같으면 같다.** 버전이
// 오르고 검출 방법이 바뀌고 강화가 낸 판정이 달라져도 id 는 그대로다. id 와 상태만 보던
// 동안은 그 변화가 통째로 서명을 지나쳤다 — 승인자가 본 적 없는 근거에 이름이 남는다.
func TestChangedEvidenceDropsTheSignatureEvenUnderTheSameRules(t *testing.T) {
	prev := review.Session{RulesetVersion: rules, Reviewer: "보안팀", Signature: "sig",
		Items: []review.Item{base()}}

	moved := base()
	moved.Fingerprint = "fp-2" // 같은 자산, 다른 내용
	got := review.Carry(prev, review.Session{RulesetVersion: rules,
		PolicyDecisions: map[string]string{}, Items: []review.Item{moved}})
	if got.Signature != "" {
		t.Error("관측 근거가 바뀌었는데 서명이 남았다")
	}

	same := review.Carry(prev, review.Session{RulesetVersion: rules,
		PolicyDecisions: map[string]string{}, Items: []review.Item{base()}})
	if same.Signature != "sig" {
		t.Error("관측이 그대로인데 서명이 사라졌다 — 재관측마다 다시 서명하게 된다")
	}
}

// IC-D10 — 신뢰도가 움직이면 델타도 걸리고 서명도 지워진다. 전에는 앞만 걸렸다.
func TestConfidenceMovesBothDeltaAndSignature(t *testing.T) {
	prev := review.Session{RulesetVersion: rules, Signature: "sig", Items: []review.Item{base()}}
	shaky := base()
	shaky.Conf = 0.3
	if review.BasisOf(shaky, rules) == review.BasisOf(base(), rules) {
		t.Fatal("신뢰도가 근거에 안 들어간다")
	}
	got := review.Carry(prev, review.Session{RulesetVersion: rules,
		PolicyDecisions: map[string]string{}, Items: []review.Item{shaky}})
	if got.Signature != "" {
		t.Error("델타는 걸리는데 서명은 살아남았다 — 두 계산이 어긋나 있다")
	}
}

// IC-D11 — 큐가 비면 항목별 비교가 전부 참이다. 규칙 판을 따로 한 번 더 보는 이유다.
func TestEmptyQueueStillLosesTheSignatureWhenTheRulesMove(t *testing.T) {
	prev := review.Session{RulesetVersion: rules, Signature: "sig"}
	got := review.Carry(prev, review.Session{RulesetVersion: "pqcota-enrich/v2+pqcaton-plan/v1",
		PolicyDecisions: map[string]string{}})
	if got.Signature != "" {
		t.Error("빈 큐라 규칙 변경이 서명을 지나쳤다")
	}
}
