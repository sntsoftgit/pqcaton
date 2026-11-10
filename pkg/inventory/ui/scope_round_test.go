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

// IC-UI58 — **운영자 진술로 연 정책 회차는 05에 「상류 이력으로 증명하지 않았다」가 남는다.** 회차의 근거가 운영자
// 진술이거나 비어 있으면(옛 기록) 그 문장을, 상류 확인이면 상류 확인 문장을 보인다. 두 정책 지문이 함께 보인다. 정책
// 회차로 열리지 않은 세션에는 이 부분이 없다.
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
	operator := map[ui.Lang]string{ui.KO: "상류 이력으로 증명하지 않았습니다", ui.EN: "it was not proven against the upstream history"}
	upstream := map[ui.Lang]string{ui.KO: "노드별 최신 스냅샷에서 모두 확인했습니다", ui.EN: "were found as the latest snapshot of every node"}
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
		if page := render(t, lang, nil); strings.Contains(page, operator[lang]) || strings.Contains(page, upstream[lang]) {
			t.Errorf("%s: a session not opened by a policy round shows a basis", lang)
		}
	}
}
