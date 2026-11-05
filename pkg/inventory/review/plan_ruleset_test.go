// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review_test

import (
	"errors"
	"testing"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/normalize"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

func evidence(ruleset string) *provisioningv1.ActionEvidenceSource {
	return &provisioningv1.ActionEvidenceSource{FindingId: "f", Snapshot: &provisioningv1.SnapshotReference{SourceNodeId: "n1",
		Reference: &provisioningv1.SnapshotReference_Content{Content: &provisioningv1.SnapshotContentReference{
			FormatVersion: "pqcota-snapshot-content/v1", Digest: "d", RulesetVersion: ruleset}}}}
}

// IC-P30 — **계획의 규칙 판은 최상위와 근거마다 둘 다 본다.** 최상위가 옛 판이면, 최상위는 지금 판인데 근거 하나만 옛
// 판이면(섞인 계획), 관측 근거가 있는데 스냅샷 참조나 규칙 판이 비어 되짚을 수 없으면 거절하고 사유를 가른다. 처음부터
// 관측 근거가 없는 조치(finding도 근거도 없다)는 지나간다.
func TestPlanRulesetIsCheckedAtBothLevels(t *testing.T) {
	now := normalize.RulesetVersion
	plan := func(top string, actions ...*provisioningv1.RemediationAction) *provisioningv1.FinalizedPlan {
		return &provisioningv1.FinalizedPlan{Id: "p", RulesetVersion: top, Actions: actions}
	}
	act := func(id, finding string, ev ...*provisioningv1.ActionEvidenceSource) *provisioningv1.RemediationAction {
		return &provisioningv1.RemediationAction{Id: id, FindingId: finding, EvidenceSources: ev}
	}
	noSnapshot := &provisioningv1.ActionEvidenceSource{FindingId: "f"}
	for name, c := range map[string]struct {
		p    *provisioningv1.FinalizedPlan
		want string
	}{
		"current":              {plan(review.RulesetVersion, act("a", "f", evidence(now))), ""},
		"no observed evidence": {plan(review.RulesetVersion, act("a", "")), ""},
		"old top":              {plan(oldUpstream(), act("a", "f", evidence(now))), review.PlanRulesetTop},
		"mixed evidence":       {plan(review.RulesetVersion, act("a", "f", evidence(now), evidence("pqcota-enrich/v0"))), review.PlanRulesetEvidence},
		"no snapshot":          {plan(review.RulesetVersion, act("a", "f", noSnapshot)), review.PlanRulesetUnverifiable},
		"empty ruleset":        {plan(review.RulesetVersion, act("a", "f", evidence(""))), review.PlanRulesetUnverifiable},
		"finding, no evidence": {plan(review.RulesetVersion, act("a", "f")), review.PlanRulesetUnverifiable},
	} {
		t.Run(name, func(t *testing.T) {
			err := review.CheckPlanRuleset(c.p)
			if c.want == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			var pe *review.PlanRulesetError
			if !errors.As(err, &pe) || pe.Reason != c.want || !errors.Is(err, review.ErrPlanRuleset) {
				t.Fatalf("err = %v, want reason %s", err, c.want)
			}
		})
	}
}
