package review_test

import (
	"strings"
	"testing"

	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// ★ 규칙 판은 **상류의 강화 규칙과 이 리포의 규칙을 합친 것**이다.
//
// 상류만 적으면 대조·계획 변환이 바뀌었을 때 값이 그대로여서, 옛 판정과 새 판정이 같은 규칙
// 아래 나온 것처럼 보인다. 이 리포만 적으면 반대로 강화 규칙이 바뀐 것을 놓친다.
func TestRulesetVersionJoinsBothSides(t *testing.T) {
	if !strings.HasPrefix(review.RulesetVersion, normalize.RulesetVersion+"+") {
		t.Errorf("상류 판이 앞에 오지 않는다: %q (상류 %q)", review.RulesetVersion, normalize.RulesetVersion)
	}
	if !strings.Contains(review.RulesetVersion, "pqcaton-plan/") {
		t.Errorf("이 리포의 판이 빠졌다: %q", review.RulesetVersion)
	}
}

// ★ 규칙 판이 바뀌면 **서명이 남지 않는다.**
//
// 서명은 「이 근거를 이 규칙으로 보고 승인했다」는 뜻이다. 항목과 상태가 그대로여도 규칙이
// 달라졌으면 승인자가 본 것과 다른 근거가 된다. 전에는 ID와 상태만 비교해서 살아남았다.
func TestChangedRulesetDropsTheSignature(t *testing.T) {
	items := []review.Item{{ID: "n1/openssl/libssl", Policy: "openssl/libssl", State: "UNDECLARED"}}
	prev := review.Session{RulesetVersion: "rules/v1", Reviewer: "보안팀", Signature: "sig", Items: items}

	same := review.Carry(prev, review.Session{RulesetVersion: "rules/v1", Items: items,
		PolicyDecisions: map[string]string{}})
	if same.Signature != "sig" {
		t.Error("규칙이 그대로인데 서명이 사라졌다 — 매번 다시 서명하게 된다")
	}

	moved := review.Carry(prev, review.Session{RulesetVersion: "rules/v2", Items: items,
		PolicyDecisions: map[string]string{}})
	if moved.Signature != "" {
		t.Error("규칙이 바뀌었는데 서명이 남았다 — 승인자가 보지 않은 근거에 서명이 붙는다")
	}
}

// ★ 다시 열어도 사람이 고른 것이 사라지지 않는다.
//
// 하나라도 빠뜨리면 검토자가 같은 선택을 되풀이하게 되고, 그러다 놓친 칸이 확정에서 막힌다.
func TestCarryKeepsEveryChoice(t *testing.T) {
	old := review.Item{
		ID: "n1/openssl/libssl", Policy: "openssl/libssl", State: "UNDECLARED",
		Conclusion: "교체한다", Plan: true, Level: "L3", Kind: "REMEDIATION_KIND_CONFIG_ONLY",
		TargetAlgorithm: "ML-KEM (FIPS 203)", FIPS: true, Config: "cfg",
		Pre: "p", Activate: "a", Deactivate: "d", Restart: "r",
	}
	next := review.Carry(
		review.Session{RulesetVersion: "rules/v1", Items: []review.Item{old}},
		review.Session{RulesetVersion: "rules/v1", PolicyDecisions: map[string]string{},
			Items: []review.Item{{ID: old.ID, Policy: old.Policy, State: old.State}}},
	)
	got := next.Items[0]
	for _, c := range []struct{ name, want, have string }{
		{"결론", old.Conclusion, got.Conclusion},
		{"위임 수준", old.Level, got.Level},
		{"조치 종류", old.Kind, got.Kind},
		{"목표 알고리즘", old.TargetAlgorithm, got.TargetAlgorithm},
		{"config 조각", old.Config, got.Config},
		{"pre", old.Pre, got.Pre}, {"activate", old.Activate, got.Activate},
		{"deactivate", old.Deactivate, got.Deactivate}, {"restart", old.Restart, got.Restart},
	} {
		if c.want != c.have {
			t.Errorf("%s이(가) 사라졌다: %q → %q", c.name, c.want, c.have)
		}
	}
	if !got.Plan || !got.FIPS {
		t.Error("계획 포함·FIPS 표시가 사라졌다")
	}
}
