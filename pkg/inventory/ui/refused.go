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
	ErrNotFound:     {tErrNotFoundHead, tErrNotFound},
	ErrMethod:       {tErrMethodHead, tErrMethod},
	ErrBadRequest:   {tErrBadRequestHead, tErrBadRequest},
	ErrInputMissing: {tErrInputMissingHead, tErrInputMissing},
	ErrInputBroken:  {tErrInputBrokenHead, tErrInputBroken},
	ErrForbidden:    {tErrForbiddenHead, tErrForbidden},
	ErrInternal:     {tErrInternalHead, tErrInternal},
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
		txt = refusedText[ErrInternal]
	}
	head := tNotSaved
	if txt.head.EN != "" {
		head = txt.head
	}
	v := RefusedView{Page: p, Head: head.In(p.Lang), Body: txt.body.In(p.Lang), Back: back,
		BackText: tBackToScreen.In(p.Lang), RefText: tErrRef.In(p.Lang)}
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
