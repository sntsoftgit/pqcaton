// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"html"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-UI59 — **04의 활성화 안내는 두 말로, 두 지문과 05로 가는 링크를 보인다.** 이 대조가 활성 정책으로 한 것이라는 문장이
// 있고 활성화 양식은 없다. 활성화할 것이 없으면(nil) 안내도 없다.
func TestSurveyActivationNote(t *testing.T) {
	const from = "scope-policy-v1:" + "1111111111111111111111111111111111111111111111111111111111111111"
	const to = "scope-policy-v1:" + "2222222222222222222222222222222222222222222222222222222222222222"
	body := map[ui.Lang]string{ui.KO: "02에서 확정한 정책(asset-scope.csv)은 아직 대조에 반영되지 않았습니다", ui.EN: "has not yet been applied to reconciliation"}
	for _, lang := range []ui.Lang{ui.KO, ui.EN} {
		v := ui.NewSurveyView(&report.Result{}, ui.Page{Lang: lang})
		v.Activation = &ui.ActivationOffer{From: from, To: to}
		var b strings.Builder
		if err := ui.RenderSurvey(&b, v); err != nil {
			t.Fatal(err)
		}
		page := html.UnescapeString(b.String())
		for _, want := range []string{body[lang], from, to, `href="` + ui.ScreenReview + `#scope-activate"`} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: the notice does not show %q", lang, want)
			}
		}
		for _, form := range []string{ui.ReviewScopeActivateConfirmPath, ui.ReviewScopeActivatePath} {
			if strings.Contains(page, `action="`+form+`"`) {
				t.Errorf("%s: the reconciliation screen has the %s form", lang, form)
			}
		}
		v.Activation = nil
		b.Reset()
		if err := ui.RenderSurvey(&b, v); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(html.UnescapeString(b.String()), body[lang]) {
			t.Errorf("%s: a notice without anything to activate", lang)
		}
	}
}
