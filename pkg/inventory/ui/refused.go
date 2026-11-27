// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// 저장하지 않은 까닭. 상태 코드는 서버가 정한다(409 · 503).
const (
	RefusedStale     = "stale"
	RefusedFinalized = "finalized"
	RefusedBusy      = "busy"
	RefusedRecovery  = "recovery"
	RefusedViewOnly  = "view-only"
	RefusedExpired   = "expired"
	RefusedConfirm   = "confirm"
	// 저장소가 막은 까닭 가운데 운영자가 손봐야 하는 것(503).
	RefusedUnsafe   = "unsafe-file"
	RefusedMismatch = "state-mismatch"
	RefusedDamaged  = "damaged"
	// RefusedDiagnostic — 작업 폴더의 입력을 읽지 못해 진단 전용으로 열었다(503).
	RefusedDiagnostic = "diagnostic"
	// RefusedExists — 만들려는 파일이 이미 있다(409). 덮어쓰지 않는다.
	RefusedExists = "exists"
	// RefusedRunNotReady · RefusedRunBusy — 관측을 시작하지 않았다(409). 빠진 것이 있거나 이미 돌고 있다.
	RefusedRunNotReady = "run-not-ready"
	// RefusedRunHostsUndeclared — 접근 정보(hosts.csv)에 선언에 없는 노드 ID가 있어(또는 선언을 읽지 못해) 관측을 시작하지
	// 않았다(409). 노드에 접속하기 전이고 파일은 그대로다. 시작 요청을 처리할 때 현재 선언으로 다시 견준다.
	RefusedRunHostsUndeclared = "run-hosts-undeclared"
	RefusedRunBusy            = "run-busy"
	// RefusedProvNotReady · RefusedProvBusy — 04의 미리 보기를 시작하지 않았다(409).
	RefusedProvNotReady = "prov-not-ready"
	RefusedProvBusy     = "prov-busy"
	// RefusedProvGrant — 적용을 시작하지 않았다(409). 유효한 미리 보기가 없다(없음 · 만료 · 입력이 바뀜 · 이미 쓰임).
	RefusedProvGrant = "prov-grant"
	// RefusedIngestNotReady · RefusedIngestBusy · RefusedIngestReplay — 01의 이력 적재를 시작하지 않았다(409). 조건이 맞지
	// 않거나(설정 · 서명 · 이미 적재됨 · 이력을 읽지 못함), 이미 돌고 있거나, 이 확인으로 이미 한 번 시작했다.
	RefusedIngestNotReady = "ingest-not-ready"
	RefusedIngestBusy     = "ingest-busy"
	RefusedIngestReplay   = "ingest-replayed"
	// RefusedIngestChanged… — 확인한 뒤 적재할 입력이 바뀌어 시작하지 않았다(409). 무엇이 바뀌었는지로 가른다: 결과 파일,
	// 활성 정책, 검증 키. 둘 이상이거나 그 밖의 것(조직 · 규칙 판)이면 뭉뚱그린 종류다.
	RefusedIngestChangedResults = "ingest-results-changed"
	RefusedIngestChangedPolicy  = "ingest-policy-changed"
	RefusedIngestChangedKeys    = "ingest-keys-changed"
	RefusedIngestChanged        = "ingest-inputs-changed"
	// RefusedSignNotReady · RefusedPlanNotApprovable · RefusedGenNotReady — 04 화면이 서명 · 생성을 하지 않았다(409).
	RefusedSignNotReady      = "sign-not-ready"
	RefusedPlanNotApprovable = "plan-not-approvable"
	RefusedGenNotReady       = "generate-not-ready"
	// RefusedSignedExists — 서명한 계획 파일이 이미 있어 덮지 않았다(409).
	RefusedSignedExists = "signed-exists"
	// RefusedRulesetChanged — 세션이 지금과 다른 상류 규칙 판으로 열려 저장 · 확정하지 않았다(409).
	RefusedRulesetChanged = "ruleset-changed"
	// RefusedPlanRuleset — 계획의 규칙 판이 지금 도구와 맞지 않아 서명 · 생성하지 않았다(409).
	RefusedPlanRuleset = "plan-ruleset"
	// RefusedPlanScope — 계획이 이 작업 폴더의 확정 기록과 이어지지 않거나, 그 계획을 만든 자산 스코프 정책이 지금 활성
	// 정책과 달라 서명 · 생성하지 않았다(409).
	RefusedPlanScope = "plan-scope"
	// RefusedPlanText — 계획(또는 서명한 계획)의 문자열에 NUL 문자가 있어 서명 · 생성하지 않았다(409). 그런 계획은 승인
	// 정규형이 모호해 한 계획의 승인이 다른 계획에서 통과할 수 있다(상류 sign.CheckPlanText). 확정된 계획이라 새 회차로 고친다.
	RefusedPlanText = "plan-text"
	// RefusedPlanFields — 계획(또는 서명한 계획)에 소스 근거나 가이드(상류 v0.11.0)가 있어 서명 · 생성하지 않았다(409).
	// 계획이 손상된 것이 아니라 04 이 아직 그 내용을 검토 · 생성하지 않는다. 상류 pqcota-approve · pqcota-provision 으로 다룬다.
	RefusedPlanFields = "plan-fields"
	// RefusedSessionText — 확정하면 만들어질 계획의 문자열에 NUL 문자가 있어 확정하지 않았다(409). 세션 · 계획 · 영수증은
	// 그대로이고, 아직 확정 전이라 그 자리에서 값을 고친다.
	RefusedSessionText = "session-text"
	// RefusedScopePolicy — 판정 세션이 기록한 자산 스코프 정책이 지금 정책과 다르거나 세션에 정책 기록이 없어 확정하지
	// 않았다(409).
	RefusedScopePolicy = "scope-policy"
	// RefusedScopeBaseline — 세션에 정책 기록이 없어 저장 · 확정하지 않았다. 기준선 회차로 넘어간다(409).
	RefusedScopeBaseline = "scope-baseline"
	// RefusedScopeUnactivated — 활성화 기록 없이 자산 스코프 정책이 바뀌어 저장 · 확정하지 않았다(409).
	RefusedScopeUnactivated = "scope-unactivated"
	// RefusedUpstreamUnreachable — pqcota 이력에 닿지 못했거나 조회가 제한 시간(10초) 안에 끝나지 않아 정책 회차를 열지
	// 않았다(503). 운영자 진술로 우회하지 않는다.
	RefusedUpstreamUnreachable = "upstream-unreachable"
	// RefusedUpstreamOrg — 연 pqcota 이력 · 판정 조직 · 판정 세션의 조직이 같지 않아 조회하지 않았다(409). 다른 조직의 이력으로
	// 정책 회차를 열지 않는다.
	RefusedUpstreamOrg = "upstream-org"
	// RefusedUpstreamMissing — pqcota 이력에서 일부 노드의 최신 적재 결과가 없거나 이 정책으로 만든 결과와 달라 열지 않았다(409).
	RefusedUpstreamMissing = "upstream-missing"
	// RefusedUpstreamEmpty — 결과 폴더에 대조할 근거가 없어 열지 않았다(409).
	RefusedUpstreamEmpty = "upstream-empty"
	// RefusedUpstreamRevert — 이 정책으로 만든 결과가 pqcota 이력에 있지만 노드의 최신이 아니다(이전 정책 복귀가 이렇게 보인다).
	// 이 정책으로 결과를 다시 적재하면 최신이 되어 풀린다(409).
	RefusedUpstreamRevert = "upstream-revert"
	// RefusedActivateHistoryChanged — 활성화 확인 뒤 pqcota 이력의 최신 참조가 달라져 활성화하지 않았다(409). 확인 페이지는
	// 대조가 맞을 때만 나오므로, 실행 때의 불일치는 그 사이에 이력이 바뀌었다는 뜻이다. 현재 활성 정책은 그대로다.
	RefusedActivateHistoryChanged = "activate-history-changed"
	// RefusedScopeSame — 자산 스코프 화면에서 확정한 정책이 이미 활성 정책과 같아 활성화할 것이 없다. 아무것도 쓰지 않았다(409).
	RefusedScopeSame = "scope-same"
	// RefusedScopeNotFinalized — 자산 스코프 화면에서 확정한 정책이 없거나, 자산 스코프 화면에서 확정한 뒤 바뀌었거나, 읽을 수 없어 활성화하지 않았다(409).
	RefusedScopeNotFinalized = "scope-not-finalized"

	// 저장과 무관한 오류. 같은 틀(새 껍데기 · 역할과 파일 이름만 · 참조 번호)로 그린다.
	ErrNotFound     = "not-found"     // 404
	ErrMethod       = "method"        // 405
	ErrBadRequest   = "bad-request"   // 400
	ErrInputMissing = "input-missing" // 404: 이 화면에 필요한 파일을 주지 않았다
	ErrInputBroken  = "input-broken"  // 500: 입력 파일을 읽지 못했다
	ErrForbidden    = "forbidden"     // 403: 요청 보호
	ErrInternal     = "internal"      // 500
)

// refusedText — 종류마다 머리와 본문. 머리가 비면 「저장하지 않았습니다」다.
var refusedText = map[string]struct{ head, body T }{
	RefusedStale: {body: tRefusedStale}, RefusedFinalized: {body: tRefusedFinalized}, RefusedBusy: {body: tRefusedBusy},
	RefusedRecovery: {body: tRefusedRecovery}, RefusedViewOnly: {body: tRefusedViewOnly},
	RefusedExpired: {body: tRefusedExpired}, RefusedConfirm: {body: tRefusedConfirmBad},
	RefusedUnsafe: {body: tRefusedUnsafe}, RefusedMismatch: {body: tRefusedMismatch}, RefusedDamaged: {body: tRefusedDamaged},
	RefusedDiagnostic: {tErrInputBrokenHead, tRefusedDiagnostic}, RefusedExists: {body: tRefusedExists},
	RefusedRunHostsUndeclared: {tRunRefusedHead, tRunRefusedHostsUndeclared},
	RefusedRunNotReady:        {tRunRefusedHead, tRunRefusedNotReady}, RefusedRunBusy: {tRunRefusedHead, tRunRefusedBusy},
	RefusedProvNotReady: {tProvRefusedHead, tProvRefusedNotReady}, RefusedProvBusy: {tProvRefusedHead, tProvRefusedBusy},
	RefusedProvGrant:      {tApplyRefusedHead, tApplyRefusedGrant},
	RefusedIngestNotReady: {tIngestRefusedHead, tIngestRefusedNotReady}, RefusedIngestBusy: {tIngestRefusedHead, tIngestRefusedBusy},
	RefusedIngestReplay:         {tIngestRefusedHead, tIngestRefusedReplay},
	RefusedIngestChangedResults: {tIngestRefusedHead, tIngestChangedResults}, RefusedIngestChangedPolicy: {tIngestRefusedHead, tIngestChangedPolicy},
	RefusedIngestChangedKeys: {tIngestRefusedHead, tIngestChangedKeys}, RefusedIngestChanged: {tIngestRefusedHead, tIngestChangedInputs},
	RefusedSignNotReady: {tSignRefusedHead, tSignNotReady}, RefusedPlanNotApprovable: {tSignRefusedHead, tPlanNotApprovable},
	RefusedGenNotReady: {tGenRefusedHead, tGenNotReady}, RefusedSignedExists: {tSignRefusedHead, tSignedExists},
	RefusedRulesetChanged:         {body: tRefusedRulesetChanged},
	RefusedPlanRuleset:            {tSignRefusedHead, tRefusedPlanRuleset},
	RefusedPlanScope:              {tSignRefusedHead, tRefusedPlanScope},
	RefusedPlanText:               {tSignRefusedHead, tRefusedPlanText},
	RefusedPlanFields:             {tSignRefusedHead, tRefusedPlanFields},
	RefusedSessionText:            {body: tRefusedSessionText},
	RefusedScopePolicy:            {body: tRefusedScopePolicy},
	RefusedScopeBaseline:          {body: tRefusedScopeBaseline},
	RefusedScopeUnactivated:       {body: tRefusedScopeUnactivated},
	RefusedUpstreamUnreachable:    {body: tRefusedUpstreamUnreachable},
	RefusedUpstreamOrg:            {body: tRefusedUpstreamOrg},
	RefusedUpstreamMissing:        {body: tRefusedUpstreamMissing},
	RefusedUpstreamEmpty:          {body: tRefusedUpstreamEmpty},
	RefusedUpstreamRevert:         {body: tRefusedUpstreamRevert},
	RefusedActivateHistoryChanged: {tActivateRefusedHead, tActivateHistoryChanged},
	RefusedScopeSame:              {body: tRefusedScopeSame},
	RefusedScopeNotFinalized:      {body: tRefusedScopeNotFinalized},
	ErrNotFound:                   {tErrNotFoundHead, tErrNotFound},
	ErrMethod:                     {tErrMethodHead, tErrMethod},
	ErrBadRequest:                 {tErrBadRequestHead, tErrBadRequest},
	ErrInputMissing:               {tErrInputMissingHead, tErrInputMissing},
	ErrInputBroken:                {tErrInputBrokenHead, tErrInputBroken},
	ErrForbidden:                  {tErrForbiddenHead, tErrForbidden},
	ErrInternal:                   {tErrInternalHead, tErrInternal},
}

// RefusedKinds — 알려진 종류 전부(시험이 문구 목록과 견준다).
func RefusedKinds() []string {
	out := make([]string, 0, len(refusedText))
	for k := range refusedText {
		out = append(out, k)
	}
	return out
}

// FileRef: 거절 페이지에 보일 파일. **역할과 파일 이름뿐이다.** 같은 이름이 여럿일 수 있어 역할을
// 함께 적고, 전체 경로는 싣지 않는다(서버 표준오류와 -recover-status에만 있다).
type FileRef struct {
	Role, Name string
}

// RefusedView: 거절 페이지. **보이는 칸은 여기 있는 것뿐이다.** 오류 문자열을 그대로 싣지 않는다.
type RefusedView struct {
	Page
	Head, Body string
	Files      []FileRef
	Back       string
	BackText   string
	// Ref — 서버 표준오류에 원문과 함께 남긴 참조 번호. 원인을 화면 말로 옮길 수 없을 때만 있다.
	Ref     string
	RefText string
	// Code — 오류 코드(종류). 운영자 안내서의 오류 표가 이 코드로 할 일을 적는다.
	Code, CodeText string
}

// maxShown: 칸 하나에 보일 글자 수.
const maxShown = 200

// NewRefused: 거절 종류로 문구를 고른다. back은 이 화면 안의 주소여야 한다(밖이면 첫 화면으로).
func NewRefused(p Page, kind, back string, files []FileRef) RefusedView {
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		back = "/"
	}
	txt, ok := refusedText[kind]
	if !ok {
		kind, txt = ErrInternal, refusedText[ErrInternal]
	}
	head := tNotSaved
	if txt.head.EN != "" {
		head = txt.head
	}
	v := RefusedView{Page: p, Head: head.In(p.Lang), Body: txt.body.In(p.Lang), Back: back,
		BackText: tBackToScreen.In(p.Lang), RefText: tErrRef.In(p.Lang), Code: kind, CodeText: tErrCode.In(p.Lang)}
	v.Page.Title = head.In(p.Lang)
	v.Page.Message, v.Page.Problem, v.Page.Warnings = "", "", nil
	for _, f := range files {
		v.Files = append(v.Files, FileRef{Role: clip(f.Role), Name: clip(filepath.Base(f.Name))})
	}
	return v
}

func clip(s string) string {
	if utf8.RuneCountInString(s) <= maxShown {
		return s
	}
	r := []rune(s)
	return string(r[:maxShown]) + "…"
}

// NewActivationRefused: 정책 활성화(확인 · 실행)의 거절. 문구는 종류가 고르고, 머리는 「정책을 활성화하지 않았습니다」다.
// 같은 종류가 저장 · 기준선 채택에서도 쓰여 머리를 종류에 묶지 않는다.
func NewActivationRefused(p Page, kind, back string, files []FileRef) RefusedView {
	v := NewRefused(p, kind, back, files)
	v.Head = tActivateRefusedHead.In(p.Lang)
	v.Page.Title = v.Head
	return v
}

// RenderRefused: 거절 페이지를 그린다.
func RenderRefused(w io.Writer, v RefusedView) error {
	return refusedPage(v).Render(context.Background(), w)
}

// RoleLabel: 파일의 역할 이름.
func RoleLabel(l Lang, role string) string {
	if t, ok := roleText[role]; ok {
		return t.In(l)
	}
	return role
}
