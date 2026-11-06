// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 06의 정책 지문 검사. **06은 이 작업 폴더에서 확정한 계획만 서명 · 생성한다.**
//
// 상류 계획 계약에는 정책 지문을 실을 칸이 없다. 그래서 05 확정이 계획과 한 트랜잭션으로 쓴 영수증(형식 2)이 그 계획을
// 만든 정책의 지문을 들고, 06은 계획 파일을 그 영수증에 이어 지금 활성 정책과 견준다. 상류 생성기는 상류 DB에서 근거를
// 찾다가 정책이 다르면 거기서 멈추지만, 06은 DB 없이 생성하므로 이 검사가 그 자리를 대신한다.
//
// 검사 순서는 계획 ID 형식 → 동일성 → 계획 파일 해시 → 정책 지문이다. 앞에서 어긋나면 뒤는 보지 않는다. 어느 것이든
// 어긋나면 열람과 보관만 하고, 이미 있는 계획 · 서명 · 플레이북은 고치거나 지우지 않는다.

import (
	"errors"
	"fmt"
	"strings"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// errPlanScope: 06이 이 계획을 서명 · 생성하지 않는다. 사유는 planScopeError가 든다.
var errPlanScope = errors.New("plan-scope")

// planScopeError — 어긋난 자리(ui.PlanScope*)와 기록용 설명.
type planScopeError struct{ Reason, Detail string }

func (e *planScopeError) Error() string { return "plan-scope (" + e.Reason + "): " + e.Detail }
func (e *planScopeError) Unwrap() error { return errPlanScope }

// planIDPrefix: 05가 계획 ID에 쓰는 머리. 뒤는 "<조직>:<세션 ID>"다(review.go의 "pqcaton:" + 범위 + ":" + 세션).
const planIDPrefix = "pqcaton:org://"

// planIdentity: 계획 ID에서 그 계획을 쓴 판정 세션의 동일성을 꺼낸다. 조직과 세션 ID에는 콜론이 들어갈 수 없으므로
// 마지막 콜론에서 나누면 하나로 정해진다.
func planIdentity(planID string) (store.Identity, bool) {
	rest, ok := strings.CutPrefix(planID, planIDPrefix)
	if !ok {
		return store.Identity{}, false
	}
	i := strings.LastIndexByte(rest, ':')
	if i <= 0 || i == len(rest)-1 {
		return store.Identity{}, false
	}
	id := store.Identity{Kind: kindReview, Org: rest[:i], SessionID: rest[i+1:]}
	if id.Validate() != nil {
		return store.Identity{}, false
	}
	return id, true
}

// checkPlanScope: 06이 이 계획(plan.json)을 서명 · 생성해도 되는가. 되면 nil, 아니면 *planScopeError다.
func (s *server) checkPlanScope(plan *provisioningv1.FinalizedPlan) error {
	if s.st == nil {
		return &planScopeError{ui.PlanScopeNoReceipt, "no state directory to look the receipt up in"}
	}
	id, ok := planIdentity(plan.GetId())
	if !ok {
		return &planScopeError{ui.PlanScopeBadID, fmt.Sprintf("plan id %q is not one 05 writes", plan.GetId())}
	}
	if id.Org != s.orgName() {
		return &planScopeError{ui.PlanScopeIdentity, fmt.Sprintf("plan id names organization %q, this workspace judges for %q", id.Org, s.orgName())}
	}
	rc, err := s.st.OutputReceipt(id, s.planOut)
	switch {
	case errors.Is(err, store.ErrNoReceipt):
		return &planScopeError{ui.PlanScopeNoReceipt, err.Error()}
	case errors.Is(err, store.ErrOutputMismatch):
		return &planScopeError{ui.PlanScopePlanHash, err.Error()}
	case err != nil:
		return err
	}
	got, ok := rc.PolicyDigest()
	if !ok {
		// 지문이 없는 것을 규칙 0개로 읽지 않는다(Receipt.PolicyDigest).
		return &planScopeError{ui.PlanScopeUnverifiable, fmt.Sprintf("receipt %s (format %d) records no scope policy", id, rc.Format)}
	}
	want, err := scope.PolicyFileDigest(s.scopeAssets)
	if err != nil {
		return &planScopeError{ui.PlanScopeUnverifiable, err.Error()}
	}
	if got != want {
		return &planScopeError{ui.PlanScopeMismatch, fmt.Sprintf("the plan was made under %s, the active policy is %s", got, want)}
	}
	return nil
}

// planScopeReason: 거절의 사유 코드.
func planScopeReason(err error) string {
	var pe *planScopeError
	if errors.As(err, &pe) {
		return pe.Reason
	}
	return ui.PlanScopeUnverifiable
}
