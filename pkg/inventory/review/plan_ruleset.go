// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review

// 계획의 규칙 판 검사. 06 화면의 서명 확인 · 서명 · 생성과 화면의 가능 여부가 이 함수 하나를 쓴다.
//
// 두 층을 본다. 계획 최상위 ruleset_version의 상류 쪽이 지금 도구와 같은가. 조치마다 근거의 스냅샷 규칙 판이 지금
// 상류 판인가. 최상위는 지금 판인데 근거 하나만 옛 판인 계획(손상 · 혼합)도 거절한다. 관측 근거가 있는데(finding이
// 있거나 근거가 적혀 있는데) 스냅샷 참조나 그 규칙 판이 비어 지금 이력에서 되짚을 수 없는 것도 거절한다. 처음부터
// 관측 근거가 없는 조치(finding도 근거도 없다. UNOBSERVED에서 세운 것)는 따로 지나간다.

import (
	"errors"
	"fmt"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"
)

// ErrPlanRuleset — 계획의 규칙 판이 지금 도구와 맞지 않거나 근거의 규칙 판을 확인할 수 없다.
var ErrPlanRuleset = errors.New("plan ruleset")

// 계획 규칙 판 거절의 사유.
const (
	PlanRulesetTop          = "plan"         // 계획 최상위의 상류 판이 다르다
	PlanRulesetEvidence     = "evidence"     // 근거 하나의 스냅샷 규칙 판이 다르다
	PlanRulesetUnverifiable = "unverifiable" // 관측 근거가 있는데 스냅샷 참조나 그 규칙 판이 비었다
)

// PlanRulesetError — 무엇이 어긋났나. 명령이 읽는 영어 문장이다.
type PlanRulesetError struct {
	Reason, Action, Have, Want string
}

func (e *PlanRulesetError) Error() string {
	switch e.Reason {
	case PlanRulesetTop:
		return fmt.Sprintf("the plan was judged under %q but this tool runs %q: the upstream ruleset differs, so it is not "+
			"signed or generated here. Finalize it again from a new round under the current rules", e.Have, e.Want)
	case PlanRulesetEvidence:
		return fmt.Sprintf("action %s cites a snapshot under %q but this tool runs %q: a plan that mixes rulesets is not "+
			"signed or generated", e.Action, e.Have, e.Want)
	default:
		return fmt.Sprintf("action %s has observed evidence whose snapshot or ruleset is not recorded, so it cannot be traced "+
			"in the current history", e.Action)
	}
}

func (e *PlanRulesetError) Unwrap() error { return ErrPlanRuleset }

// CheckPlanRuleset — 계획을 지금 도구로 서명하고 생성해도 되는가. 되면 nil.
func CheckPlanRuleset(p *provisioningv1.FinalizedPlan) error {
	want := UpstreamRuleset(RulesetVersion)
	if have := UpstreamRuleset(p.GetRulesetVersion()); have != want {
		return &PlanRulesetError{Reason: PlanRulesetTop, Have: p.GetRulesetVersion(), Want: RulesetVersion}
	}
	for _, a := range p.GetActions() {
		src := a.GetEvidenceSources()
		if len(src) == 0 {
			if a.GetFindingId() != "" {
				return &PlanRulesetError{Reason: PlanRulesetUnverifiable, Action: a.GetId()}
			}
			continue // 처음부터 관측 근거가 없는 조치
		}
		for _, e := range src {
			c := e.GetSnapshot().GetContent()
			if c == nil || c.GetRulesetVersion() == "" {
				return &PlanRulesetError{Reason: PlanRulesetUnverifiable, Action: a.GetId()}
			}
			if c.GetRulesetVersion() != want {
				return &PlanRulesetError{Reason: PlanRulesetEvidence, Action: a.GetId(), Have: c.GetRulesetVersion(), Want: want}
			}
		}
	}
	return nil
}
