// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// 상류 규칙 판이 바뀌면 자산 ID와 스냅샷 지문이 다른 규칙으로 셈해진다. 옛 세션의 근거는 지금 상류 이력에서
// 찾히지 않으므로, 두 규칙 판이 한 판정 · 한 계획 안에서 섞이지 않게 한다.

// oldUpstream: 지금 도구와 상류 쪽만 다른 결합 판. 계획 규칙 판은 같다.
func oldUpstream() string {
	return "pqcota-enrich/v0" + strings.TrimPrefix(review.RulesetVersion, review.UpstreamRuleset(review.RulesetVersion))
}

// IC-P19 — **상류 규칙 판이 다른 세션은 확정하지 않는다.** 결론과 서명이 다 있어도 계획을 내지 않고,
// 거절은 두 판을 값으로 적는다. 계획 규칙 판만 낮은 세션(IC-P18)은 이 거절에 걸리지 않는다.
func TestFinalizeRefusesAnotherUpstreamRuleset(t *testing.T) {
	sf := judgedSession()
	sf.RulesetVersion = oldUpstream()
	res, err := review.Finalize(sf)
	if !errors.Is(err, review.ErrRulesetChanged) {
		t.Fatalf("err = %v, want ErrRulesetChanged", err)
	}
	if res != nil {
		t.Error("a plan came out of a session judged under another upstream ruleset")
	}
	if msg := err.Error(); !strings.Contains(msg, sf.RulesetVersion) || !strings.Contains(msg, review.RulesetVersion) {
		t.Errorf("the refusal does not name both rulesets: %q", msg)
	}
	ok := judgedSession()
	if _, err := review.Finalize(ok); err != nil {
		t.Fatalf("the same session under the current ruleset was refused: %v", err)
	}
}

// IC-P20 — **상류 규칙 판이 다르면 Carry는 옮기지 않고 저장된 세션을 그대로 돌려준다.** 옛 결론이 새 근거에
// 실리면 그 항목은 판정된 것이 되어 서명만으로 확정을 지나간다. 계획 규칙 판만 다르면 지금처럼 옮기고 서명만 지운다.
func TestCarryDoesNotCrossAnUpstreamRuleset(t *testing.T) {
	prev := judgedSession()
	prev.RulesetVersion = oldUpstream()
	next := judgedSession()
	next.Signature, next.Reviewer = "", ""
	next.PolicyDecisions = map[string]string{"openssl/libssl": ""}
	next.Items[0].Conclusion, next.Items[0].Plan = "", false

	got := review.Carry(prev, next)
	if got.RulesetVersion != prev.RulesetVersion || got.SessionID != prev.SessionID || got.Signature != prev.Signature {
		t.Errorf("Carry did not hand back the stored session: ruleset %q session %q", got.RulesetVersion, got.SessionID)
	}

	prev.RulesetVersion = review.UpstreamRuleset(review.RulesetVersion) + "+pqcaton-plan/v1"
	moved := review.Carry(prev, next)
	if moved.RulesetVersion != next.RulesetVersion || moved.PolicyDecisions["openssl/libssl"] != "교체" || moved.Signature != "" {
		t.Errorf("a plan-ruleset change must still carry the conclusions and drop the signature: %+v", moved)
	}
}

// UpstreamRuleset는 결합 판에서 상류 쪽만 꺼낸다. 계획 규칙 판이 없으면 통째로 상류 판이다.
func TestUpstreamRulesetSplitsTheCombinedVersion(t *testing.T) {
	for in, want := range map[string]string{
		"pqcota-enrich/v3+pqcaton-plan/v3": "pqcota-enrich/v3",
		"pqcota-enrich/v2":                 "pqcota-enrich/v2",
		"":                                 "",
	} {
		if got := review.UpstreamRuleset(in); got != want {
			t.Errorf("UpstreamRuleset(%q) = %q, want %q", in, got, want)
		}
	}
}
