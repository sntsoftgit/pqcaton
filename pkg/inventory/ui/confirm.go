// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"context"
	"io"
	"net/url"
	"sort"
)

// 확정 전 확인. **확인 요청은 아무것도 바꾸지 않는다.** 확정 버튼이 보낸 폼 값(저장하지 않은 편집
// 포함)을 숨은 필드로 그대로 다시 싣고, 무엇이 확정되는지 보여 준다. 실제 확정은 이 페이지의 버튼이
// 서명된 확인 토큰과 함께 보낸다.
const (
	ConfirmTokenField   = "confirm_token"
	ConfirmExpField     = "confirm_exp"
	ConfirmPayloadField = "confirm_payload"
	ConfirmSessionField = "confirm_session"
)

// Field: 숨은 필드 하나.
type Field struct{ Name, Value string }

// ConfirmView: 확인 페이지.
type ConfirmView struct {
	Page
	// Action: 확정 주소. SaveAction: 확정하지 않고 저장만 하는 주소.
	Action, SaveAction string
	Back               string
	// Lines: 무엇이 확정되는지(조치 수, 판정 수 같은 것).
	Lines []string
	Files []FileRef
	// Hidden: 다시 실을 폼 값. 확인 토큰과 CSRF · rev는 따로 싣는다.
	Hidden                         []Field
	Token, Exp, Payload, SessionID string
}

// confirmOwn: 확인 페이지가 직접 싣는 필드. 받은 폼에서 다시 싣지 않는다.
var confirmOwn = map[string]bool{
	CSRFField: true, RevField: true, ConfirmTokenField: true, ConfirmExpField: true,
	ConfirmPayloadField: true, ConfirmSessionField: true,
}

// CarryForm: 받은 폼 값을 숨은 필드로. 이름 순서로 늘어놓는다(같은 폼이면 같은 페이지).
func CarryForm(f url.Values) []Field {
	names := make([]string, 0, len(f))
	for k := range f {
		if !confirmOwn[k] {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	var out []Field
	for _, k := range names {
		for _, v := range f[k] {
			out = append(out, Field{Name: k, Value: v})
		}
	}
	return out
}

// RenderConfirm: 확인 페이지를 그린다.
func RenderConfirm(w io.Writer, v ConfirmView) error {
	return confirmPage(v).Render(context.Background(), w)
}

// ConfirmTitle: 확인 페이지의 제목.
func ConfirmTitle(l Lang) string { return tConfirmTitle.In(l) }

// LinePlanActions · LineJudgments · LinePolicyRules: 확인 페이지의 요약 줄.
func LinePlanActions(l Lang, n int) string { return tLinePlanActions.F(l, n) }

func LineJudgments(l Lang, n int) string { return tLineJudgments.F(l, n) }

func LinePolicyRules(l Lang, n int) string { return tLinePolicyRules.F(l, n) }
