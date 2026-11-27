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

// IC-UI60 — **02 · 04 · 05의 안내와 03의 두 단계.** 02는 두 말로 「아직 대조에 반영되지 않았다」와 두 링크(먼저 04의 현재 대조,
// 그다음 03의 재적재 · 활성화)를 보이고, 다음 단계가 없으면 아무것도 보이지 않는다. 05의 상자는 두 지문과 03으로 가는 링크만
// 보이고 활성화 양식이 없다. 03의 「활성화할 확정 정책」은 두 지문을 나란히 두고, 활성화할 확정 정책으로 재적재 · 정책 활성화를 의미 있는
// <ol> 안에 그 순서로 둔다. 화면 적재가 설정되지 않았으면 첫 단계에 작업 폴더 기준 상대 경로의 두 명령(서명 확인이 먼저)이
// 있다. 재적재가 확인되기 전에는 둘째 단계에 단추가 없고, 확인되면 활성화 확인으로 가는 단추가 그 단계 안에 있다(운영자 확인
// 칸은 없다).
func TestActivationStepsAndLinks(t *testing.T) {
	survey, steps := `href="`+ui.ScreenSurvey+`"`, `href="`+ui.ScreenObserve+`#scope-activate"`
	type texts struct{ next, s1, s2, hint, go03, wait string }
	want := map[ui.Lang]texts{
		ui.KO: {"이 정책은 아직 대조에 반영되지 않았습니다", "활성화할 확정 정책으로 재적재", "정책 활성화", "작업 폴더 기준의 상대 경로입니다", "03에서 재적재 · 활성화 보기", "그 전에는 현재 활성 정책이 유지됩니다"},
		ui.EN: {"This policy has not yet been applied to reconciliation", "Re-ingest under the finalized policy to activate", "Activate the policy", "the paths are relative to it", "View re-ingest and activation in 03", "Until then the active policy stays as it is"},
	}
	render := func(t *testing.T, f func(*strings.Builder) error) string {
		t.Helper()
		var b strings.Builder
		if err := f(&b); err != nil {
			t.Fatal(err)
		}
		return html.UnescapeString(b.String())
	}
	offer := ui.ActivationOffer{From: "scope-policy-v1:aaaa", To: "scope-policy-v1:bbbb", Candidate: "asset-scope.csv", Results: "results"}
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

		rv := ui.ReviewView{Page: ui.Page{Lang: lang}, Activation: &offer}
		page = render(t, func(b *strings.Builder) error { return ui.RenderReview(b, rv) })
		if !strings.Contains(page, steps) || !strings.Contains(page, offer.From) || !strings.Contains(page, offer.To) ||
			strings.Contains(page, `action="`+ui.ReviewScopeActivateConfirmPath+`"`) || strings.Contains(page, "activation-steps") {
			t.Errorf("%s: 05 should only point to 03 with the two fingerprints, with no activation form", lang)
		}

		// 03: 화면 적재가 설정되지 않았고 아직 재적재되지 않았다.
		ov := ui.NewObserveView(nil, ui.Page{Lang: lang})
		ov.HasResults = true
		ov.Policy = &ui.ObservePolicy{Offer: offer, ActiveFile: "scope-assets.csv", CandidateFile: "asset-scope.csv", ActivateRev: "r1",
			Ingest: ui.IngestView{Files: 1, Observed: 2, Managed: 1, Nodes: []ui.IngestNode{{Node: "web", Want: "abc"}}}}
		page = render(t, func(b *strings.Builder) error { return ui.RenderObserve(b, ov) })
		if a, c := strings.Index(page, offer.From), strings.Index(page, offer.To); a < 0 || c < a || !strings.Contains(page, `id="scope-activate"`) {
			t.Fatalf("%s: 03 does not show the two fingerprints side by side", lang)
		}
		ol := strings.Index(page, `<ol class="activation-steps">`)
		end := strings.Index(page, "</ol>")
		if ol < 0 || end < ol {
			t.Fatalf("%s: the steps are not an ordered list", lang)
		}
		list := page[ol:end]
		items := strings.Split(list, "<li ")[1:]
		if len(items) != 2 || !strings.Contains(items[0], w.s1) || !strings.Contains(items[1], w.s2) {
			t.Fatalf("%s: the steps are not 1 re-ingest, 2 activate: %q", lang, list)
		}
		if !strings.Contains(items[0], "pqcota-ingest -scope-assets asset-scope.csv results") || !strings.Contains(items[0], w.hint) {
			t.Errorf("%s: the first step has no command relative to the workspace folder", lang)
		}
		// 적재 전에 수집기 서명을 확인하는 명령이 먼저 온다(pqcota v0.15.0, 저장하지 않는 -verify-keys).
		if v, i := strings.Index(items[0], "pqcota-ingest -verify-keys results"), strings.Index(items[0], "pqcota-ingest -scope-assets"); v < 0 || v > i || !strings.Contains(items[0], "PQCOTA_VERIFY_KEY") {
			t.Errorf("%s: the first step does not check the signatures with -verify-keys before ingesting", lang)
		}
		if strings.Contains(list, `type="checkbox"`) || strings.Contains(items[1], "<button") || !strings.Contains(items[1], w.wait) {
			t.Errorf("%s: before the re-ingest is confirmed the second step must have no button and say the active policy stays", lang)
		}
		// 재적재가 확인되면 둘째 단계에 활성화 확인으로 가는 단추가 있고, 03에서 냈다는 표시와 활성화의 rev를 싣는다.
		ov.Policy.Ingest.AllCurrent = true
		page = render(t, func(b *strings.Builder) error { return ui.RenderObserve(b, ov) })
		second := strings.Split(page[strings.Index(page, `<ol class="activation-steps">`):], "<li ")[2]
		for _, has := range []string{"<button", `action="` + ui.ReviewScopeActivateConfirmPath + `"`, `name="` + ui.ActivateFromField + `" value="` + ui.ActivateFromObserve + `"`,
			`name="` + ui.RevField + `" value="r1"`} {
			if !strings.Contains(second, has) {
				t.Errorf("%s: with the re-ingest confirmed the second step lacks %s", lang, has)
			}
		}

		sv := ui.NewSurveyView(&report.Result{}, ui.Page{Lang: lang})
		sv.Activation = &ui.ActivationOffer{From: "a", To: "b"}
		if page := render(t, func(b *strings.Builder) error { return ui.RenderSurvey(b, sv) }); !strings.Contains(page, w.go03) {
			t.Errorf("%s: the reconciliation button is not %q", lang, w.go03)
		}
	}
}
