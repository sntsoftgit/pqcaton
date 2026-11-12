// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"html"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-UI58 — **회차의 근거를 05에 그대로 보인다.** 근거가 운영자 진술이거나 비어 있으면(v0.27.x 이하의 과거 기록)
// 「pqcota 이력과 대조하지 않았다」를, pqcota 이력 대조면 「확인 시점에 pqcota 이력과 일치했다」를 보인다. 대조 문장은
// 지금도 일치한다거나 적재를 증명한다고 말하지 않는다. 두 정책 지문이 함께 보인다. 정책 회차로 열리지 않은 세션에는 이
// 부분이 없다.
func TestScopeRoundBasisIsShown(t *testing.T) {
	const from = "scope-policy-v1:" + "1111111111111111111111111111111111111111111111111111111111111111"
	const to = "scope-policy-v1:" + "2222222222222222222222222222222222222222222222222222222222222222"
	render := func(t *testing.T, lang ui.Lang, sc *review.ScopeChangeRef) string {
		t.Helper()
		var b strings.Builder
		if err := ui.RenderReview(&b, ui.ReviewView{Page: ui.Page{Lang: lang}, ScopeChange: sc}); err != nil {
			t.Fatal(err)
		}
		return html.UnescapeString(b.String())
	}
	operator := map[ui.Lang]string{ui.KO: "pqcota 이력과 대조하지 않았습니다", ui.EN: "It was not reconciled with the pqcota history"}
	upstream := map[ui.Lang]string{ui.KO: "확인 시점에 pqcota 이력과 일치했습니다", ui.EN: "At the time of checking, the pqcota history matched"}
	claims := map[ui.Lang][]string{ui.KO: {"증명", "보장", "지금도 일치"}, ui.EN: {"proven", "proves the", "guarantee", "still match"}}
	for _, lang := range []ui.Lang{ui.KO, ui.EN} {
		for _, basis := range []string{review.ScopeBasisOperator, ""} {
			page := render(t, lang, &review.ScopeChangeRef{Reason: review.ScopeRoundActivation, FromPolicy: from, ToPolicy: to, Basis: basis})
			if !strings.Contains(page, operator[lang]) || strings.Contains(page, upstream[lang]) {
				t.Errorf("%s basis %q: the operator statement is not marked as unproven", lang, basis)
			}
			if !strings.Contains(page, from) || !strings.Contains(page, to) {
				t.Errorf("%s basis %q: the two policy digests are not shown", lang, basis)
			}
		}
		page := render(t, lang, &review.ScopeChangeRef{Reason: review.ScopeRoundActivation, FromPolicy: from, ToPolicy: to,
			Basis: review.ScopeBasisUpstream})
		if !strings.Contains(page, upstream[lang]) || strings.Contains(page, operator[lang]) {
			t.Errorf("%s: the upstream-verified basis is not shown as such", lang)
		}
		for _, c := range claims[lang] {
			if strings.Contains(page, c) {
				t.Errorf("%s: the basis claims %q", lang, c)
			}
		}
		if page := render(t, lang, nil); strings.Contains(page, operator[lang]) || strings.Contains(page, upstream[lang]) {
			t.Errorf("%s: a session not opened by a policy round shows a basis", lang)
		}
	}
}
