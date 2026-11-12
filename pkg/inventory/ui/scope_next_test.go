// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"html"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-UI60 — **02의 다음 단계와 05의 세 단계.** 02는 두 말로 「아직 대조에 반영되지 않았다」와 두 링크(먼저 04의 현재 대조,
// 그다음 05의 활성화 절차)를 보이고, 다음 단계가 없으면 아무것도 보이지 않는다. 05의 활성화 상자는 pqcota 재적재 · pqcota
// 재적재 결과 대조 · 정책 활성화를 의미 있는 <ol> 안에 그 순서로 두고, 첫 단계에 작업 폴더 기준 상대 경로의 명령과 그 안내,
// 둘째에 대조 설명(운영자 확인 칸은 없다), 셋째에 단추가 있다. 04의 단추는 「활성화 절차 보기」다.
func TestActivationStepsAndLinks(t *testing.T) {
	survey, steps := `href="`+ui.ScreenSurvey+`"`, `href="`+ui.ScreenReview+`#scope-activate"`
	type texts struct{ next, s1, s2, s3, hint, go04 string }
	want := map[ui.Lang]texts{
		ui.KO: {"이 정책은 아직 대조에 반영되지 않았습니다", "pqcota 재적재", "pqcota 재적재 결과 대조", "정책 활성화", "작업 폴더 기준의 상대 경로입니다", "05에서 활성화 절차 보기"},
		ui.EN: {"This policy has not yet been applied to reconciliation", "Re-ingest pqcota", "Reconcile the pqcota re-ingest", "Activate the policy", "the paths are relative to it", "View the activation steps in 05"},
	}
	render := func(t *testing.T, f func(*strings.Builder) error) string {
		t.Helper()
		var b strings.Builder
		if err := f(&b); err != nil {
			t.Fatal(err)
		}
		return html.UnescapeString(b.String())
	}
	for lang, w := range want {
		v := ui.NewScopeView(scope.Session{}, ui.Page{Lang: lang})
		v.ActivationNext = true
		page := render(t, func(b *strings.Builder) error { return ui.RenderScope(b, v) })
		if !strings.Contains(page, w.next) || !strings.Contains(page, survey) || !strings.Contains(page, steps) ||
			strings.Index(page, survey) > strings.Index(page, steps) {
			t.Errorf("%s: the next step after finalizing 02 is missing or out of order", lang)
		}
		v.ActivationNext = false
		if page := render(t, func(b *strings.Builder) error { return ui.RenderScope(b, v) }); strings.Contains(page, steps) {
			t.Errorf("%s: a next step without anything waiting for activation", lang)
		}

		rv := ui.ReviewView{Page: ui.Page{Lang: lang}, Activation: &ui.ActivationOffer{From: "a", To: "b", Candidate: "asset-scope.csv", Results: "results"}}
		page = render(t, func(b *strings.Builder) error { return ui.RenderReview(b, rv) })
		ol := strings.Index(page, `<ol class="activation-steps">`)
		end := strings.Index(page, "</ol>")
		if ol < 0 || end < ol {
			t.Fatalf("%s: the activation steps are not an ordered list", lang)
		}
		list := page[ol:end]
		items := strings.Split(list, "<li>")[1:]
		if len(items) != 3 || !strings.Contains(items[0], w.s1) || !strings.Contains(items[1], w.s2) || !strings.Contains(items[2], w.s3) {
			t.Fatalf("%s: the steps are not 1 re-ingest, 2 confirm, 3 activate: %q", lang, list)
		}
		if !strings.Contains(items[0], "pqcota-ingest -scope-assets asset-scope.csv results") || !strings.Contains(items[0], w.hint) {
			t.Errorf("%s: the first step has no command relative to the workspace folder", lang)
		}
		if strings.Contains(list, `type="checkbox"`) || !strings.Contains(items[2], "<button") {
			t.Errorf("%s: the steps take an operator statement, or the button is not in its step", lang)
		}

		sv := ui.NewSurveyView(&report.Result{}, ui.Page{Lang: lang})
		sv.Activation = &ui.ActivationOffer{From: "a", To: "b"}
		if page := render(t, func(b *strings.Builder) error { return ui.RenderSurvey(b, sv) }); !strings.Contains(page, w.go04) {
			t.Errorf("%s: the reconciliation button is not %q", lang, w.go04)
		}
	}
}
