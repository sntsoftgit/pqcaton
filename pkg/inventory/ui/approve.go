// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

// 06 승인 · 생성 화면.
//
// **서명과 생성은 상류 함수가 한다.** 계획을 FINALIZED로 올리고 서명하는 일(`PrepareApproval` ·
// `SignApproval`), 서명을 확인하고 플레이북을 만드는 일(`VerifyApprovals` · `Generate…Playbook`)은
// pqcota-provisioning과 pqcota-common의 것을 그대로 부른다. 화면은 무엇에 서명하는지 보이고, 키를 점검하고,
// 결과를 파일로 남긴다. 운영 기계에 적용하는 일은 하지 않는다.

import (
	"context"
	"io"
	"strings"
	"time"
)

// ApproveView — 06 승인 · 생성 화면.
type ApproveView struct {
	Page
	// Plan — 05에서 확정한 계획. nil이면 아직 없다.
	Plan *PlanSummary
	// Key — 승인 키 점검.
	Key KeyCheck
	// Signed — 서명한 계획(plan.signed.json)의 상태.
	Signed SignedState
	// Playbooks — 만든 플레이북의 상태.
	Playbooks PlaybookState
	// Writable — 이 서버가 저장하는가.
	Writable bool
	// Approver — 서명 양식에 채울 승인자(마지막으로 서명한 사람).
	Approver string
}

// PlanSummary — 계획에서 승인자가 볼 것.
type PlanSummary struct {
	ID, Status string
	Actions    []PlanAction
}

// PlanAction — 조치 하나. 무엇을 어디에 어디까지 하는지.
type PlanAction struct {
	ID, Node, Kind, Provider, Target, Level string
}

// KeyState — 승인 키의 상태.
type KeyState string

const (
	KeyOK         KeyState = "ok"
	KeyNotGiven   KeyState = "not-given"
	KeyUnreadable KeyState = "unreadable"
	KeyOpen       KeyState = "open"
	KeyInside     KeyState = "inside"
	KeyBad        KeyState = "bad"
	KeyLink       KeyState = "link"
	KeyOwner      KeyState = "owner"
	KeyLinks      KeyState = "links"
)

// KeyCheck — 승인 키 점검. Public은 확인 키(공개 키)다. pqcota-provision에 넘길 때 쓴다.
type KeyCheck struct {
	State  KeyState
	Path   string
	Public string
	// Fingerprint — 공개 키의 지문. 확인 페이지가 보이고 확인 토큰이 덮는다.
	Fingerprint string
}

// Text — 점검 결과 한 줄과, 빠졌으면 할 일.
func (k KeyCheck) Text(l Lang) string {
	if t, ok := keyText[k.State]; ok {
		return t.In(l)
	}
	return string(k.State)
}

// SignState — 서명한 계획의 상태.
type SignState string

const (
	SignNone    SignState = "none"
	SignValid   SignState = "valid"
	SignStale   SignState = "stale"
	SignInvalid SignState = "invalid"
)

// SignedState — 서명한 계획. Approvers는 확인된 승인자다.
type SignedState struct {
	State     SignState
	Approvers []string
	At        time.Time
}

// PlaybookState — 만든 플레이북. Warnings는 상류 생성기가 짚은 빈 곳이다(pqcota-provision의 종료 상태 3).
type PlaybookState struct {
	Made     bool
	Stale    bool
	Level    string
	At       time.Time
	Warnings []string
	Files    []string
}

// ApproveSummary — 06 카드에 적을 것.
type ApproveSummary struct {
	HasPlan  bool
	Signed   SignState
	Made     bool
	Stale    bool
	Warnings int
}

// Summary — 카드의 한 줄을 고를 값.
func (v ApproveView) Summary() ApproveSummary {
	return ApproveSummary{HasPlan: v.Plan != nil, Signed: v.Signed.State, Made: v.Playbooks.Made,
		Stale: v.Playbooks.Stale, Warnings: len(v.Playbooks.Warnings)}
}

// CanSign — 서명 양식을 낼 수 있는가. 계획이 있고, 키가 쓸 만하고, **서명한 계획 파일이 아직 없어야 한다.** 있는
// 파일은 어떤 경우에도 덮지 않는다(낡았으면 운영자가 먼저 보관한다).
func (v ApproveView) CanSign() bool {
	return v.Writable && v.Plan != nil && v.Key.State == KeyOK && v.Signed.State == SignNone
}

// CanArchive — 지난 서명을 보관할 수 있는가. 지금 계획과 맞지 않거나 이 키로 확인되지 않는 서명이 있을 때다.
func (v ApproveView) CanArchive() bool {
	return v.Writable && (v.Signed.State == SignStale || v.Signed.State == SignInvalid)
}

// CanGenerate — 생성 양식을 낼 수 있는가. 서명이 지금 계획과 맞고 이 키로 확인돼야 한다.
func (v ApproveView) CanGenerate() bool {
	return v.Writable && v.Signed.State == SignValid && v.Key.State == KeyOK
}

// RenderApprove — 06 승인 · 생성 화면.
func RenderApprove(w io.Writer, v ApproveView) error {
	return approvePage(v).Render(context.Background(), w)
}

// approveState — 06 카드의 한 줄.
func approveState(l Lang, s ApproveSummary) (string, string) {
	switch {
	case !s.HasPlan:
		return tStepApproveNoPlan.In(l), ""
	case s.Signed == SignStale || s.Signed == SignInvalid:
		return tStepApproveStale.In(l), "danger"
	case s.Signed != SignValid:
		return tStepApproveSign.In(l), "warn"
	case !s.Made || s.Stale:
		return tStepApproveGenerate.In(l), "warn"
	case s.Warnings > 0:
		return tStepApproveIncomplete.F(l, s.Warnings), "warn"
	}
	return tStepApproveDone.In(l), "ok"
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

// StepApproveIncompleteText — 빈 곳이 남은 06 카드의 한 줄(시험이 견준다).
func StepApproveIncompleteText(l Lang, n int) string { return tStepApproveIncomplete.F(l, n) }

// shortLevel — 위임 수준을 L1 · L2 · L3로. 고르지 않았으면 빈 값이다(생성기가 기본 수준을 쓴다).
func shortLevel(level string) string {
	rest := strings.TrimPrefix(level, "DEPLOY_AUTOMATION_LEVEL_")
	if len(rest) >= 2 && rest[0] == 'L' {
		return rest[:2]
	}
	return ""
}

// SignConfirmLines — 서명 확인 페이지의 줄: 무엇에(계획 · 조치 수 · 해시) 누가(승인자) 어느 키로(지문).
func SignConfirmLines(l Lang, planID string, actions int, planHash, approver, fingerprint string) []string {
	return []string{tSignLinePlan.F(l, planID, actions), tSignLineHash.F(l, planHash), tSignLineApprover.F(l, approver),
		tSignLineKey.F(l, fingerprint)}
}
