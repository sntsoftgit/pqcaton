// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// excludedResult — 2026-11-23 격리 데모에서 03과 04의 숫자가 갈라진 대조 결과. 선언에 없는 libcrypto는 자산 스코프 정책이
// 관리 대상에서 뺐고(UNDECLARED · EXCLUDED_BY_POLICY), 선언한 libssl은 보지 못했다(UNOBSERVED).
func excludedResult() *report.Result {
	key := func(node, runtime, component string) reconcile.AssetKey {
		return reconcile.AssetKey{NodeID: node, Runtime: runtime, Component: component}
	}
	return &report.Result{
		Org: "acme",
		Assets: []reconcile.Reconciled{
			{Key: key("pay-app", "openssl", "libcrypto"), State: reconcile.Undeclared, Managed: reconcile.ExcludedByPolicy},
			{Key: key("web-gw", "openssl", "libssl"), State: reconcile.Unobserved, Managed: reconcile.NotEvaluated, RescanCandidate: true},
			{Key: key("pay-db", "openssl", "libssl"), State: reconcile.Confirmed, Managed: reconcile.Managed, Confidence: 0.9},
			{Key: key("pay-db", "openssl", "libcrypto"), State: reconcile.Confirmed, Managed: reconcile.ExcludedByPolicy},
		},
	}
}

// IC-UI74 — **03의 「판정할 항목」과 04의 「필수 판정」은 같은 항목을 센다.**
//
// 03이 선언과 어긋난 자산을 모두 세면 정책이 뺀 자산까지 들어가, 판정 큐에 없는 항목을 판정하라고 알린다
// (03은 2개, 04는 1개였다). 03은 큐에 필수로 오르는 것만 세고, 뺀 것은 따로 밝힌다.
func TestSurveyCountsWhatTheReviewQueueHolds(t *testing.T) {
	r := excludedResult()
	_, queue := reconcile.BuildReviewQueue(r.Assets)
	sf := review.Session{PolicyDecisions: map[string]string{}}
	for _, it := range queue {
		item := review.ItemOf(it.Rec, it.Mandatory)
		sf.Items = append(sf.Items, item)
		sf.PolicyDecisions[item.Policy] = ""
	}
	for _, l := range []ui.Lang{ui.KO, ui.EN} {
		page := ui.Page{Title: "x", Lang: l}
		survey := ui.NewSurveyView(r, page)
		got := survey.Summary()
		want := ui.NewReviewView(sf, page).Summary().Mandatory
		if want != 1 {
			t.Fatalf("the review queue holds %d required items, want 1", want)
		}
		if got.ToJudge != want {
			t.Errorf("03 counts %d items to judge, 04 counts %d required judgments", got.ToJudge, want)
		}
		if got.Undeclared != 1 || got.Unobserved != 1 || got.Excluded != 1 {
			t.Errorf("the states and the excluded count are %+v", got)
		}
		var b strings.Builder
		if err := ui.RenderSurvey(&b, survey); err != nil {
			t.Fatal(err)
		}
		body := b.String()
		head, note, mark := "판정할 항목이 1개입니다", "스코프 정책이 관리 대상에서 뺀", "정책 제외"
		if l == ui.EN {
			head, note, mark = "1 item is waiting for a judgment", "scope policy excludes: 1.", "excluded by policy"
		}
		for _, s := range []string{head, note, mark, `data-survey-excluded="1"`} {
			if !strings.Contains(body, s) {
				t.Errorf("%s: 03 does not show %q", l, s)
			}
		}
	}

	// 어긋난 것이 정책이 뺀 자산뿐이면 판정할 것이 없다. 「다 맞았다」고 하지 않고 뺀 것이 있다고 알린다.
	only := &report.Result{Org: "acme", Assets: r.Assets[:1]}
	v := ui.NewSurveyView(only, ui.Page{Title: "x", Lang: ui.KO})
	if s := v.Summary(); s.ToJudge != 0 || s.Excluded != 1 {
		t.Fatalf("only an excluded asset: %+v", s)
	}
	var b strings.Builder
	if err := ui.RenderSurvey(&b, v); err != nil {
		t.Fatal(err)
	}
	if body := b.String(); strings.Contains(body, "대조가 다 맞았습니다") || !strings.Contains(body, "판정할 항목이 없습니다") {
		t.Error("03 says everything reconciled although an undeclared asset is excluded by policy")
	}
}
