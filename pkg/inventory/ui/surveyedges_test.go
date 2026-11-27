// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-UI75 — **02는 받은 노드에서만 본 엣지와 방향을 확인하지 못한 연결을 가려 보인다.**
//
// 받은 쪽 기록(상류 v0.18.0부터)으로만 본 엣지는 보내는 쪽 노드를 관측하지 못했어도 대조 표에 오른다. 그 사정을 적지 않으면
// 「관측 안 된 노드」에서 나간 연결을 어떻게 보았는지 읽는 사람이 알 수 없다. 방향을 모르는 기록은 표에 넣지 않고 따로 적는다.
func TestSurveyShowsServerSideAndUnconfirmedEdges(t *testing.T) {
	ekey := func(src, dst string, port uint32) reconcile.EdgeKey {
		return reconcile.EdgeKey{Org: "acme", Src: src, Dst: dst, Port: port, Proto: "TLS"}
	}
	r := &report.Result{
		Org: "acme",
		Edges: []reconcile.ReconciledEdge{
			{Key: ekey("web-gw", "pay-app", 8443), State: reconcile.Confirmed, SeenByClient: true, SeenByServer: true},
			{Key: ekey("pay-app", "pay-db", 5432), State: reconcile.Confirmed, SeenByServer: true},
			{Key: ekey("203.0.113.9", "pay-db", 5432), State: reconcile.Undeclared, SeenByServer: true, OffScopeSrc: true, NeedsReview: true},
		},
		UnconfirmedEdges: []reconcile.UnconfirmedEdge{
			{Node: "web-gw", Peer: "pay-db", Proto: "SSH"},
			{Node: "pay-db", Peer: "198.51.100.7", Proto: "SSH", OffScopePeer: true},
		},
		Uncovered:     map[string]bool{"pay-app": true},
		ObservedEdges: 3,
	}
	for _, l := range []ui.Lang{ui.KO, ui.EN} {
		v := ui.NewSurveyView(r, ui.Page{Title: "x", Lang: l})
		if len(v.Edges) != 3 || len(v.Unconfirmed) != 2 {
			t.Fatalf("%s: %d rows and %d unconfirmed, want 3 and 2", l, len(v.Edges), len(v.Unconfirmed))
		}
		var b strings.Builder
		if err := ui.RenderSurvey(&b, v); err != nil {
			t.Fatal(err)
		}
		body := b.String()
		only, title, off := "받은 노드에서만 관측", "방향을 확인하지 못한 연결: 2건", "등재 판정 요청"
		if l == ui.EN {
			only, title, off = "observed on the accepting node only", "Connections whose direction was not confirmed: 2", "needs enrollment decision"
		}
		if n := strings.Count(body, "data-edge-server-only"); n != 2 {
			t.Errorf("%s: %d rows are marked as seen by the accepting node only, want 2 (the edge seen on both nodes is not)", l, n)
		}
		// 등재 판정 요청: 받은 연결의 등재되지 않은 상대(보내는 쪽 칸)와 방향 미확인 기록의 등재되지 않은 상대.
		if n := strings.Count(body, off); n != 2 {
			t.Errorf("%s: %q appears %d times, want 2", l, off, n)
		}
		if !strings.Contains(v.DOT, "role not confirmed") {
			t.Errorf("%s: the topology DOT does not draw the unconfirmed records", l)
		}
		for _, s := range []string{only, title, `data-edges-unconfirmed="2"`, "198.51.100.7"} {
			if !strings.Contains(body, s) {
				t.Errorf("%s: 02 does not show %q", l, s)
			}
		}
	}

	// 방향을 확인하지 못한 기록이 없으면 그 절을 그리지 않는다.
	r.UnconfirmedEdges = nil
	var b strings.Builder
	if err := ui.RenderSurvey(&b, ui.NewSurveyView(r, ui.Page{Title: "x", Lang: ui.KO})); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "data-edges-unconfirmed") {
		t.Error("02 draws the unconfirmed section although there is no such record")
	}
}
