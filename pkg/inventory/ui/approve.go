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
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"io"
	"strings"
	"time"
)

// ApproveView — 06 승인 · 생성 화면.
type ApproveView struct {
	Page
	// Plan — 05에서 확정한 계획. nil이면 아직 없다.
	Plan *PlanSummary
	// Key — 승인 키(서명용 개인 키) 점검. Verify — 확인용 공개 키 파일 점검. 서명된 계획의 확인은 Key 가 쓸 만하면 그
	// 공개 키로, 아니면 Verify 로 한다.
	Key    KeyCheck
	Verify KeyCheck
	// Signed — 서명한 계획(plan.signed.json)의 상태.
	Signed SignedState
	// Playbooks — 만든 플레이북의 상태.
	Playbooks PlaybookState
	// Writable — 이 서버가 저장하는가.
	Writable bool
	// Approver — 서명 양식에 채울 승인자(마지막으로 서명한 사람).
	Approver string
	// RulesetProblem — 계획(또는 서명된 계획)의 규칙 판이 지금 도구와 맞지 않는 사유(review.PlanRuleset*). 비면 맞다.
	// 맞지 않으면 서명 · 생성 양식을 내지 않는다. 보관은 그대로 할 수 있고, 있는 플레이북은 그대로 둔다.
	RulesetProblem string
	// RulesetSigned — 맞지 않는 것이 서명된 계획(plan.signed.json) 쪽이다. 지금 계획(plan.json)은 맞을 수 있다.
	RulesetSigned bool
	// ScopeProblem — 지금 계획(plan.json)을 이 작업 폴더의 확정 기록에 잇지 못했거나, 그 계획을 만든 자산 스코프 정책이
	// 지금 활성 정책과 다른 사유(PlanScope*). 비면 맞다. 맞지 않으면 서명 · 생성 양식을 내지 않는다. 보관은 그대로 할
	// 수 있고, 있는 계획 · 서명 · 플레이북은 그대로 둔다.
	ScopeProblem string
	// TextProblem — 지금 계획(또는 서명한 계획)의 문자열에 NUL 문자가 있다. 승인 정규형이 모호해 서명 · 생성하지 않는다.
	// TextSigned — 그것이 서명한 계획(plan.signed.json) 쪽이다.
	TextProblem, TextSigned bool
}

// 06이 계획을 서명 · 생성하지 않는 정책 쪽 사유. 검사 순서(계획 ID 형식 → 동일성 → 계획 파일 해시 → 정책 지문)와 같다.
const (
	PlanScopeBadID        = "bad-id"       // 계획 ID가 05가 쓰는 모양이 아니다
	PlanScopeIdentity     = "identity"     // 계획 ID의 조직이 이 작업 폴더의 조직이 아니다
	PlanScopeNoReceipt    = "no-receipt"   // 그 세션의 확정 영수증이 이 상태 디렉토리에 없다
	PlanScopePlanHash     = "plan-hash"    // 계획 파일이 그 영수증이 적은 내용이 아니다
	PlanScopeUnverifiable = "unverifiable" // 영수증에 정책 지문이 없다(이 판보다 먼저 확정)
	PlanScopeMismatch     = "mismatch"     // 계획을 만든 정책이 지금 활성 정책과 다르다
)

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
	// KeyMissing — 기본 위치에 키가 없다(정상 시작, 서명만 비활성). KeyDisabled — -no-default-approval-key 로 기본 검색을 껐다.
	KeyMissing  KeyState = "missing"
	KeyDisabled KeyState = "disabled"
	// KeyMismatch — 개인 키의 공개 키와 확인용 공개 키가 다르다. 서명도 확인도 하지 않는다.
	KeyMismatch KeyState = "mismatch"
	// KeyHasPrivate — 확인용 공개 키 파일에 개인 키가 들어 있다. 공개 키 파일은 읽기를 넓게 열 수 있으므로 받지 않는다.
	KeyHasPrivate KeyState = "has-private"
	// KeyConflict — 쓸 만한 개인 키와 쓸 수 없는 확인용 공개 키를 함께 골랐다. 구성 오류라 서명도 생성도 하지 않는다.
	KeyConflict KeyState = "conflict"
	// KeyUnwritable — 확인용 공개 키를 다른 사용자가 고칠 수 있다.
	KeyUnwritable KeyState = "writable"
)

// KeySource — 키를 어디서 정했나.
type KeySource string

const (
	KeyExplicit      KeySource = "explicit" // -approval-key · -approval-verify-key
	KeyDefault       KeySource = "default"  // os.UserConfigDir()/pqcaton/
	KeySearchOff     KeySource = "off"      // -no-default-approval-key
	KeyOtherGiven    KeySource = "other"    // 다른 하나(개인 키나 공개 키)를 명시해 기본 위치를 읽지 않음
	KeyDefaultFolder           = "pqcaton"  // 사용자 설정 폴더 안의 폴더
)

// KeyCheck — 승인 키 점검. Public은 확인 키(공개 키)다. pqcota-provision에 넘길 때 쓴다.
type KeyCheck struct {
	State  KeyState
	Source KeySource
	// Path — 해석한 전체 경로(로그와 확인 토큰). Where — 화면에 보일 자리: 기본 위치면 사용자 설정 폴더 기준의 상대 위치다.
	Path   string
	Where  string
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
	return v.Writable && v.Plan != nil && v.Key.State == KeyOK && v.Signed.State == SignNone && v.RulesetProblem == "" &&
		v.ScopeProblem == "" && !v.TextProblem
}

// CanArchive — 지난 서명을 보관할 수 있는가. 지금 계획과 맞지 않거나 이 키로 확인되지 않는 서명이 있을 때, 그리고 규칙 판이
// 맞지 않는 서명된 계획이 있을 때다(그 계획으로는 다시 서명도 생성도 하지 않으므로, 옮겨 두어야 새 회차의 계획을 서명한다).
func (v ApproveView) CanArchive() bool {
	return v.Writable && (v.Signed.State == SignStale || v.Signed.State == SignInvalid ||
		((v.RulesetProblem != "" || v.ScopeProblem != "" || v.TextProblem) && v.Signed.State != SignNone))
}

// CanGenerate — 생성 양식을 낼 수 있는가. 서명이 지금 계획과 맞고 확인돼야 한다. **개인 키는 요구하지 않는다** — 개인 키는 새
// 승인 서명에만 필요하고, 서명의 확인은 확인용 공개 키로도 한다.
func (v ApproveView) CanGenerate() bool {
	return v.Writable && v.Signed.State == SignValid && v.RulesetProblem == "" && v.ScopeProblem == "" && !v.TextProblem
}

// PublicKey — 서명된 계획을 확인하는 공개 키. 개인 키가 쓸 만하면 그 공개 키, 아니면 확인용 공개 키다. 둘 다 없으면 빈 값이다.
func (v ApproveView) PublicKey() string {
	if v.Key.State == KeyOK {
		return v.Key.Public
	}
	if v.Verify.State == KeyOK {
		return v.Verify.Public
	}
	return ""
}

// KeyLines — 06 의 키 점검 문구. canVerify 는 개인 키 없이도 서명된 계획을 확인할 수 있는가(확인용 공개 키가 쓸 만한가)다.
func (k KeyCheck) Lines(l Lang, canVerify bool) []string {
	after := tKeyNoVerify.In(l)
	if canVerify {
		after = tKeyCanVerify.In(l)
	}
	switch {
	case k.State == KeyOK && k.Source == KeyDefault:
		return []string{tKeyDefaultOK.In(l), tKeyWhere.F(l, k.Where), tKeyFingerprint.F(l, k.Fingerprint)}
	case k.State == KeyOK:
		return []string{tKeyExplicitOK.In(l), tKeyFingerprint.F(l, k.Fingerprint)}
	case k.State == KeyDisabled:
		return []string{tKeyDisabledText.In(l)}
	case k.State == KeyNotGiven && k.Source == KeyOtherGiven:
		return []string{tKeyVerifyOnlyText.In(l), after}
	case k.State == KeyMissing || k.State == KeyNotGiven:
		return []string{tKeyMissingText.In(l), after}
	case k.State == KeyMismatch:
		return []string{tKeyMismatchText.In(l)}
	case k.State == KeyConflict:
		return []string{tKeyConflictText.In(l)}
	case k.Source == KeyDefault:
		return []string{tKeyDefaultBad.F(l, k.Text(l)), after}
	}
	return []string{k.Text(l), after}
}

// VerifyLine — 확인용 공개 키 한 줄. 기본 위치에 없으면 빈 값이다(보이지 않는다).
func (k KeyCheck) VerifyLine(l Lang) string {
	switch {
	case k.State == KeyOK:
		return tVerifyKeyOK.F(l, k.Where, k.Fingerprint)
	case k.State == KeyMissing || k.State == KeyNotGiven || k.State == KeyDisabled || k.State == "":
		return ""
	}
	return tVerifyKeyBad.F(l, k.Where, k.Text(l))
}

// ScopeText — 정책 쪽 거절의 화면 말. **지금 계획(plan.json)의 문제로 적는다.** 서명한 계획의 문제로 읽히면 운영자가
// 서명을 보관하면 풀린다고 여긴다.
func (v ApproveView) ScopeText() string {
	switch v.ScopeProblem {
	case "":
		return ""
	case PlanScopeBadID, PlanScopeIdentity, PlanScopeNoReceipt:
		return tPlanScopeNotHere.In(v.Page.Lang)
	case PlanScopePlanHash:
		return tPlanScopeChanged.In(v.Page.Lang)
	case PlanScopeMismatch:
		return tPlanScopeMismatch.In(v.Page.Lang)
	default:
		return tPlanScopeUnverifiable.In(v.Page.Lang)
	}
}

// TextText — 문자열 속 NUL로 서명 · 생성하지 않는 까닭의 화면 말. 서명한 계획 쪽이면 그 계획을 가리킨다.
func (v ApproveView) TextText() string {
	switch {
	case !v.TextProblem:
		return ""
	case v.TextSigned:
		return tSignedPlanText.In(v.Page.Lang)
	default:
		return tRefusedPlanText.In(v.Page.Lang)
	}
}

// RulesetText — 규칙 판 거절의 화면 말.
func (v ApproveView) RulesetText() string {
	if v.RulesetSigned && v.RulesetProblem != "" {
		return tSignedRulesetProblem.In(v.Page.Lang)
	}
	switch v.RulesetProblem {
	case "":
		return ""
	case review.PlanRulesetTop:
		return tPlanRulesetTop.In(v.Page.Lang)
	case review.PlanRulesetEvidence:
		return tPlanRulesetEvidence.In(v.Page.Lang)
	default:
		return tPlanRulesetUnverifiable.In(v.Page.Lang)
	}
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
