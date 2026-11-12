// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review_test

import (
	"errors"
	"testing"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// v0.28.0까지 이 리포가 쓰던 규칙 판. 상류 pqcota v0.12.0이 규칙 묶음을 v4로 올렸다(JCA 배포본 식별).
// 다른 시험은 옛 판을 지어낸 값(pqcota-enrich/v0)으로 쓰지만, 여기서는 실제로 지나가는 v3 → v4를 그대로 적는다.
const (
	upstreamV3 = "pqcota-enrich/v3"
	sessionV3  = upstreamV3 + "+pqcaton-plan/v3"
)

// IC-P51 — **이 판은 상류 규칙 묶음 v4에 묶인다. v3 세션 · 계획은 기존 규칙 판 안전장치가 거절한다.** 결합 판은
// `pqcota-enrich/v4+pqcaton-plan/v3`이다(계획 규칙 판은 그대로). 결론과 서명이 다 찬 v3 세션도 확정하지 않는다
// (ErrRulesetChanged). 최상위가 v3인 계획, 최상위는 v4인데 근거 하나가 v3인 계획은 06이 사유를 갈라 거절한다.
func TestV3SessionsAndPlansAreRefusedUnderV4(t *testing.T) {
	if got := review.UpstreamRuleset(review.RulesetVersion); got != "pqcota-enrich/v4" {
		t.Fatalf("upstream ruleset = %q, want pqcota-enrich/v4", got)
	}
	if review.RulesetVersion != "pqcota-enrich/v4+pqcaton-plan/v3" {
		t.Fatalf("combined ruleset = %q", review.RulesetVersion)
	}

	sf := judgedSession()
	sf.RulesetVersion = sessionV3
	if _, err := review.Finalize(sf); !errors.Is(err, review.ErrRulesetChanged) {
		t.Errorf("a judged and signed v3 session: err = %v, want ErrRulesetChanged", err)
	}

	act := func(ev ...*provisioningv1.ActionEvidenceSource) *provisioningv1.RemediationAction {
		return &provisioningv1.RemediationAction{Id: "a", FindingId: "f", EvidenceSources: ev}
	}
	now := review.UpstreamRuleset(review.RulesetVersion)
	for name, c := range map[string]struct {
		p    *provisioningv1.FinalizedPlan
		want string
	}{
		"v3 plan":        {&provisioningv1.FinalizedPlan{Id: "p", RulesetVersion: sessionV3, Actions: []*provisioningv1.RemediationAction{act(evidence(upstreamV3))}}, review.PlanRulesetTop},
		"v3 evidence":    {&provisioningv1.FinalizedPlan{Id: "p", RulesetVersion: review.RulesetVersion, Actions: []*provisioningv1.RemediationAction{act(evidence(now), evidence(upstreamV3))}}, review.PlanRulesetEvidence},
		"v4 plan passes": {&provisioningv1.FinalizedPlan{Id: "p", RulesetVersion: review.RulesetVersion, Actions: []*provisioningv1.RemediationAction{act(evidence(now))}}, ""},
	} {
		err := review.CheckPlanRuleset(c.p)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: refused: %v", name, err)
			}
			continue
		}
		var pe *review.PlanRulesetError
		if !errors.As(err, &pe) || pe.Reason != c.want {
			t.Errorf("%s: err = %v, want reason %s", name, err, c.want)
		}
	}
}

// judgedV3 — v0.28.0(v3)에서 판정을 마친 세션. JCA 항목 하나(옛 수집기의 finding 하나)와 v4에서 사라지는 OpenSSL 항목.
func judgedV3() review.Session {
	sf := judgedSession()
	sf.RulesetVersion = sessionV3
	jca := sf.Items[0]
	jca.ID, jca.Runtime, jca.Policy = "n1/jca/jca-provider-chain", "jca", "jca/jca-provider-chain"
	jca.Conclusion = "교체한다"
	jca.Sources = []review.EvidenceSource{{FindingID: "f-jca-v3", Fingerprint: "fp-jca-v3", SourceNodeID: "n1", SnapshotRuleset: upstreamV3}}
	gone := sf.Items[0]
	gone.ID, gone.Conclusion = "n1/openssl/libgone", "제거한다"
	sf.Items = []review.Item{jca, gone}
	return sf
}

// rebuiltV4 — 같은 노드를 v0.13.0 수집기로 다시 관측해 지금 규칙(v4)으로 세운 세션. JCA 자산은 배포본 둘(새 finding id ·
// 새 지문)이 근거이고, 새 OpenSSL 항목이 하나 생겼다.
func rebuiltV4() review.Session {
	sf := judgedV3()
	sf.RulesetVersion, sf.Signature, sf.Reviewer = review.RulesetVersion, "", ""
	sf.PolicyDecisions = map[string]string{}
	jca := sf.Items[0]
	jca.Conclusion, jca.Plan, jca.Kind, jca.TargetAlgorithm = "", false, "", ""
	jca.Sources = []review.EvidenceSource{
		{FindingID: "f-jca-dep-a", Fingerprint: "fp-dep-a", SourceNodeID: "n1", SnapshotRuleset: "pqcota-enrich/v4"},
		{FindingID: "f-jca-dep-b", Fingerprint: "fp-dep-b", SourceNodeID: "n1", SnapshotRuleset: "pqcota-enrich/v4"},
	}
	fresh := jca
	fresh.ID, fresh.Runtime, fresh.Policy = "n1/openssl/libnew", "openssl", "openssl/libssl"
	fresh.Sources = []review.EvidenceSource{{FindingID: "f-new", Path: "/usr/lib/libnew.so", SourceNodeID: "n1", SnapshotRuleset: "pqcota-enrich/v4"}}
	sf.Items = []review.Item{jca, fresh}
	return sf
}

// IC-P52 — **JCA 식별이 바뀐 항목의 옛 결론은 현재 결론으로 넘어가지 않는다.** v3 세션을 v4로 다시 세운 세션에 그대로
// 옮기면(Carry) 아무것도 옮기지 않고 저장된 v3 세션을 돌려준다. 규칙 전환 회차를 열면 JCA 항목의 현재 결론은 비고 옛
// 결론은 참고값(previous)에만 있다. 보고서는 그 항목을 changed(근거 변경, 옛 finding과 새 finding, 옛 결론)로, 사라진
// 항목을 only-old로, 새 항목을 only-new로 가른다. 서명과 검토자만 넣어서는 확정되지 않는다. 옛 id와 새 id를 잇는 호환
// 규칙은 없다.
func TestV3ToV4RoundDoesNotCarryJCAConclusions(t *testing.T) {
	old := judgedV3()
	if carried := review.Carry(old, rebuiltV4()); carried.RulesetVersion != sessionV3 || carried.Items[0].Sources[0].FindingID != "f-jca-v3" {
		t.Fatalf("Carry moved a v3 session onto v4 evidence: %+v", carried)
	}
	next, rep := review.RulesetRound(old, rebuiltV4(), "s-v4", "ruleset-change-v4.json", "session.ruleset-v3.json")
	if next.RulesetVersion != review.RulesetVersion || next.Signature != "" || next.Reviewer != "" {
		t.Fatalf("the new round: ruleset %q signature %q reviewer %q", next.RulesetVersion, next.Signature, next.Reviewer)
	}
	jca := next.Items[0]
	if jca.Conclusion != "" || jca.Plan {
		t.Errorf("the JCA conclusion was carried as current: %+v", jca)
	}
	if jca.Previous == nil || jca.Previous.Conclusion != "교체한다" || jca.Previous.RulesetVersion != sessionV3 {
		t.Errorf("the JCA reference values: %+v", jca.Previous)
	}
	if rep.FromRuleset != sessionV3 || rep.ToRuleset != review.RulesetVersion {
		t.Errorf("the report head: from %q to %q", rep.FromRuleset, rep.ToRuleset)
	}
	got := map[string]review.ItemChange{}
	for _, c := range rep.Items {
		got[c.ID] = c
	}
	if c := got["n1/jca/jca-provider-chain"]; c.Change != review.ChangeChanged || !c.EvidenceChanged ||
		len(c.OldFindings) != 1 || len(c.NewFindings) != 2 || c.PreviousConclusion != "교체한다" {
		t.Errorf("the JCA item in the report: %+v", c)
	}
	if got["n1/openssl/libgone"].Change != review.ChangeOnlyOld || got["n1/openssl/libgone"].PreviousConclusion != "제거한다" {
		t.Errorf("the removed item: %+v", got["n1/openssl/libgone"])
	}
	if c := got["n1/openssl/libnew"]; c.Change != review.ChangeOnlyNew || c.PreviousConclusion != "" {
		t.Errorf("the new item: %+v", c)
	}
	next.Signature, next.Reviewer = "kty", "보안팀"
	if _, err := review.Finalize(next); !errors.Is(err, decision.ErrMandatoryPending) {
		t.Errorf("a signature alone finalized the v4 round: %v", err)
	}
}
