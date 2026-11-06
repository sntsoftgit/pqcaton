// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-UI57 — **정책 쪽 거절은 지금 계획(plan.json)의 문제로 적힌다.** 사유마다 두 말의 문장이 있고, 서명된 계획
// (plan.signed.json)의 문제로 적지 않는다(그렇게 읽히면 서명을 보관하면 풀린다고 여긴다). 서명이 있든 없든 서명 ·
// 생성 양식이 없고, 서명된 계획이 있으면 보관 양식이 있다. 사유가 없으면 아무 문장도 없다.
func TestPlanScopeProblemIsAboutTheCurrentPlan(t *testing.T) {
	reasons := []string{ui.PlanScopeBadID, ui.PlanScopeIdentity, ui.PlanScopeNoReceipt, ui.PlanScopePlanHash,
		ui.PlanScopeUnverifiable, ui.PlanScopeMismatch}
	for _, lang := range []ui.Lang{ui.KO, ui.EN} {
		for _, reason := range reasons {
			for _, signed := range []ui.SignState{ui.SignNone, ui.SignValid} {
				v := ui.ApproveView{Page: ui.Page{Lang: lang}, Writable: true, ScopeProblem: reason,
					Plan: &ui.PlanSummary{ID: "pqcaton:org://acme:s-1", Status: "PLAN_STATUS_IN_REVIEW"},
					Key:  ui.KeyCheck{State: ui.KeyOK}, Signed: ui.SignedState{State: signed}}
				text := v.ScopeText()
				if text == "" || !strings.Contains(text, "plan.json") || strings.Contains(text, "plan.signed.json") {
					t.Errorf("%s %s: the reason is not written about the current plan: %q", lang, reason, text)
				}
				if v.CanSign() || v.CanGenerate() {
					t.Errorf("%s %s (signed %s): a sign or generate form would be offered", lang, reason, signed)
				}
				if v.CanArchive() != (signed != ui.SignNone) {
					t.Errorf("%s %s (signed %s): archive offered = %v", lang, reason, signed, v.CanArchive())
				}
				var b strings.Builder
				if err := ui.RenderApprove(&b, v); err != nil {
					t.Fatal(err)
				}
				page := b.String()
				if !strings.Contains(page, text) {
					t.Errorf("%s %s: the screen does not show the reason", lang, reason)
				}
				for _, form := range []string{ui.SignConfirmPath, ui.GeneratePath} {
					if strings.Contains(page, `action="`+form+`"`) {
						t.Errorf("%s %s: the screen has the %s form", lang, reason, form)
					}
				}
			}
		}
	}
	if (ui.ApproveView{}).ScopeText() != "" {
		t.Error("no reason, yet a sentence")
	}
}
