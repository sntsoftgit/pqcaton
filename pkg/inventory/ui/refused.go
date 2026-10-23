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
)

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
}

// maxShown: 칸 하나에 보일 글자 수.
const maxShown = 200

// NewRefused: 거절 종류로 문구를 고른다. back은 이 화면 안의 주소여야 한다(밖이면 첫 화면으로).
func NewRefused(p Page, kind, back string, files []FileRef) RefusedView {
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		back = "/"
	}
	body := map[string]T{
		RefusedStale: tRefusedStale, RefusedFinalized: tRefusedFinalized, RefusedBusy: tRefusedBusy,
		RefusedRecovery: tRefusedRecovery, RefusedViewOnly: tRefusedViewOnly,
	}[kind]
	v := RefusedView{Page: p, Head: tNotSaved.In(p.Lang), Body: body.In(p.Lang), Back: back,
		BackText: tBackToScreen.In(p.Lang)}
	v.Page.Title = tNotSaved.In(p.Lang)
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
