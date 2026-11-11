// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 06 이 아직 다루지 않는 상류 계획 필드(상류 v0.11.0: 조치의 source_evidence 와 guide).
//
// 승인 서명은 이 두 필드까지 덮는다. 그런데 06 은 조치마다 노드 · 종류 · 제공자 · 대상 알고리즘 · 위임 수준만 보이고, 소스 근거나
// 가이드 본문을 보이지 않으며, 가이드를 카탈로그와 견주지도(provisioning.CheckGuides) 않는다. 생성도
// GenerateProvisioningPlaybook 이라 소스 위치와 가이드를 플레이북에 싣지 않는다. 그 필드가 든 계획에 06 이 서명하면 승인자가
// 보지 못한 내용까지 승인하게 된다. 그래서 06 이 그 내용을 검토할 수 있게 될 때까지 **서명도 생성도 하지 않는다.** 계획이
// 손상된 것이 아니라 pqcaton 의 한계다. 그런 계획은 상류 pqcota-approve --review · pqcota-approve · pqcota-provision 으로 다룬다.
//
// 이 판정 하나를 화면 상태 · 서명 확인 · 실제 서명(잠금 안) · 생성(잠금 안)이 함께 쓴다. 서명한 계획에 있으면 보관만 허용한다.
// 지금도 06 은 이 작업 폴더의 확정 영수증과 해시가 맞는 계획만 서명하므로(checkPlanScope) 이 거부는 그 위의 이중 방어다.
// 설계: pqcaton-aai plans/2026-11-11-pqcota-v0110-uptake.md

import (
	"errors"
	"fmt"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"
)

// errPlanFields: 계획에 pqcaton 이 아직 검토 · 생성하지 않는 필드(소스 근거 · 가이드)가 있다.
var errPlanFields = errors.New("the plan carries source evidence or a guide, which pqcaton does not review or generate yet")

// checkPlanFields: 조치 가운데 소스 근거나 가이드를 가진 것이 있으면 errPlanFields 다.
func checkPlanFields(p *provisioningv1.FinalizedPlan) error {
	for _, a := range p.GetActions() {
		if len(a.GetSourceEvidence()) > 0 || a.GetGuide() != nil {
			return fmt.Errorf("%w (action %q)", errPlanFields, a.GetId())
		}
	}
	return nil
}
