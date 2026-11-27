// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

func histTopology() upstream.Topology {
	at := time.Date(2026, 11, 24, 10, 18, 31, 0, time.UTC)
	return upstream.Topology{
		DOT:       "digraph pqcota_topology {\n}\n",
		Summary:   inventory.TopologySummary{Nodes: 3, Edges: 4, Addresses: 1},
		Endpoints: 2,
		Nodes: []upstream.TopoNode{
			{ID: "pay-app", Snapshot: "ingest-20261124-aaaa", Digest: strings.Repeat("a", 64), Ruleset: "pqcota-enrich/v4", At: at, Loaded: true,
				Records: []upstream.TopoRecord{{Role: upstream.RoleServer, Peer: "web-gw", PeerKind: upstream.PeerNode, PeerHasSnapshot: true, Proto: "tls", Port: 8443, Group: "X25519MLKEM768", Count: 6}}},
			{ID: "pay-db", Name: "Payments DB", Snapshot: "ingest-20261124-bbbb", Digest: strings.Repeat("b", 64), Ruleset: "pqcota-enrich/v4", At: at,
				Apps: []upstream.TopoApp{{Key: `/opt/apps/<script>alert(1)</script>`, Kinds: []string{"exe-path"}, Libraries: []string{"libssl.so.1.1 1.1.1f"}}},
				Records: []upstream.TopoRecord{
					{App: `/opt/apps/<script>alert(1)</script>`, Role: upstream.RoleServer, Peer: "203.0.113.9", PeerKind: upstream.PeerUnregistered, Proto: "tls", Port: 4433, Group: "x25519", Count: 1},
					{Role: upstream.RoleUnconfirmed, Peer: "10.0.0.50", PeerKind: upstream.PeerShared, SharedBy: 2, Proto: "ssh"},
				}},
			{ID: "web-gw", Snapshot: "ingest-20261124-cccc", Digest: strings.Repeat("c", 64), Ruleset: "pqcota-enrich/v4", At: at, Loaded: true,
				Records: []upstream.TopoRecord{{App: "/usr/bin/openssl", Role: upstream.RoleClient, Peer: "pay-app", PeerKind: upstream.PeerNode, PeerHasSnapshot: true, Proto: "tls", Port: 8443, Group: "X25519MLKEM768", Count: 6}}},
		},
	}
}

func renderHist(t *testing.T, v ui.HistTopoView) string {
	t.Helper()
	var b strings.Builder
	if err := ui.RenderHistTopo(&b, v); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// IC-UI76 — **이력 화면은 무엇을 읽었는지부터 밝히고, 표는 그림 없이도 읽힌다.**
//
// 조회한 조직과 노드마다의 이력 스냅샷을 적고, 작업 폴더의 것과 같은지는 지문으로만 가른다(일치 · 다름 · 비교 불가). 요약
// 표는 고른 노드 하나로 좁혀지고, 기록 수를 03의 통신 건수라고 하지 않는다. 관측에서 온 문자열은 글자로만 나간다.
func TestHistoryTopologyStatesItsSourceAndReadsWithoutThePicture(t *testing.T) {
	top := histTopology()
	local := map[string]ui.LocalSnapshot{
		"pay-app": {Digest: strings.Repeat("a", 64), Ruleset: "pqcota-enrich/v4"}, // 일치
		"pay-db":  {Digest: strings.Repeat("9", 64), Ruleset: "pqcota-enrich/v4"}, // 다름
		"batch":   {Digest: strings.Repeat("d", 64), Ruleset: "pqcota-enrich/v4"}, // 작업 폴더에만 있다
	}
	for _, l := range []ui.Lang{ui.KO, ui.EN} {
		v := ui.NewHistTopoView(top, "demo-corp", local, "", ui.Page{Title: "x", Lang: l})
		want := map[string]string{"pay-app": ui.CompareSame, "pay-db": ui.CompareDiffer, "web-gw": ui.CompareUnknown}
		for _, r := range v.Rows {
			if r.Compare != want[r.ID] {
				t.Errorf("%s: %s is %q, want %q", l, r.ID, r.Compare, want[r.ID])
			}
		}
		if len(v.OnlyLocal) != 1 || v.OnlyLocal[0] != "batch" {
			t.Errorf("%s: only in the workspace: %v", l, v.OnlyLocal)
		}
		body := renderHist(t, v)
		words := []string{"일치", "다름", "비교 불가", "작업 폴더에 이 노드의 결과가 없습니다", "노드 3대 · 저장된 연결 기록 4건 · 노드로 잇지 못한 상대 2곳",
			"03의 「협상된 통신」 건수와 다릅니다", "등록 노드 2대가 함께 쓰는 IP", "등록되지 않은 주소", "← 받았음", "→ 열었음", "↔ 방향 미확인", "DOT 내려받기"}
		if l == ui.EN {
			words = []string{"same", "different", "cannot compare", "the workspace has no results for this node",
				"Nodes: 3 · stored connection records: 4 · peers not tied to a node: 2", "is not the “negotiated connections” count of 03",
				"IP shared by registered nodes: 2", "unregistered address", "← accepted", "→ opened", "↔ not confirmed", "Download DOT"}
		}
		for _, s := range append(words, `data-hist-org>demo-corp<`, `data-compare="same"`, `data-compare="differ"`, `data-compare="none"`,
			`data-hist-only-local="1"`, `data-hist-endpoints="2"`, `href="`+ui.HistTopoDotPath+`"`, `class="hint picture-progress" data-picture="pending"`, `data-picture="place"`,
			`hx-get="`+ui.HistTopoPicturePath+`?lang=`+string(l)+`"`, `hx-trigger="load"`,
			`title="`+strings.Repeat("b", 64)+`"`, "/opt/apps/&lt;script&gt;alert(1)&lt;/script&gt;") {
			if !strings.Contains(body, s) {
				t.Errorf("%s: the screen does not show %q", l, s)
			}
		}
		if strings.Contains(body, "<script>alert(1)") {
			t.Errorf("%s: an observed app key went out as markup", l)
		}
		if n := strings.Count(body, "data-hist-summary="); n != 3 {
			t.Errorf("%s: %d node summaries with no node chosen, want 3", l, n)
		}
	}

	// 노드 하나를 고르면 요약 표만 좁혀진다. 출처 표는 그대로다.
	one := renderHist(t, ui.NewHistTopoView(top, "demo-corp", local, "pay-db", ui.Page{Title: "x", Lang: ui.KO}))
	if strings.Count(one, "data-hist-summary=") != 1 || !strings.Contains(one, `data-hist-summary="pay-db"`) || strings.Count(one, "data-hist-node=") != 3 {
		t.Error("choosing a node did not narrow the summary to that node only")
	}
	if got := ui.NewHistTopoView(top, "demo-corp", local, "no-such-node", ui.Page{Lang: ui.KO}); got.Node != "" {
		t.Errorf("an unknown node was kept as the choice: %q", got.Node)
	}

	// 작업 폴더를 읽지 못했으면 모든 줄이 비교 불가이고 그 까닭을 적는다.
	unread := renderHist(t, ui.NewHistTopoView(top, "demo-corp", nil, "", ui.Page{Title: "x", Lang: ui.KO}))
	if strings.Count(unread, `data-compare="none"`) != 3 || !strings.Contains(unread, "작업 폴더의 결과를 읽지 못했습니다") {
		t.Error("an unreadable workspace is not shown as cannot-compare with its reason")
	}

	// 등록된 엔드포인트가 없으면 그 사실과 등록 방법을 적는다.
	top.Endpoints = 0
	if b := renderHist(t, ui.NewHistTopoView(top, "demo-corp", local, "", ui.Page{Title: "x", Lang: ui.KO})); !strings.Contains(b, `data-hist-endpoints="0"`) || !strings.Contains(b, "pqcota-hosts") {
		t.Error("no registered endpoint: the screen does not say so")
	}

	// 이력이 비었으면 어느 조직을 읽었는지와 다음 할 일을 적고, 표와 그림은 그리지 않는다.
	empty := renderHist(t, ui.NewHistTopoView(upstream.Topology{}, "demo-corp", local, "", ui.Page{Title: "x", Lang: ui.KO}))
	for _, s := range []string{"data-hist-empty", "data-hist-org>demo-corp<", "pqcota-ingest", "PQCOTA_ORG"} {
		if !strings.Contains(empty, s) {
			t.Errorf("an empty history: the screen does not show %q", s)
		}
	}
	if strings.Contains(empty, "data-hist-counts") || strings.Contains(empty, "data-hist-dot") {
		t.Error("an empty history still draws the summary or the picture")
	}

	// 읽지 못했으면 까닭을 적는다.
	v := ui.NewHistTopoView(upstream.Topology{}, "demo-corp", local, "", ui.Page{Title: "x", Lang: ui.KO})
	v.Err = "pqcota history: unreachable: timed out"
	if b := renderHist(t, v); !strings.Contains(b, "data-hist-error") || !strings.Contains(b, "timed out") || strings.Contains(b, "data-hist-empty") {
		t.Error("an unreadable history is not shown as such")
	}

	// 화면은 그림을 싣지 않고 나간다. 그림은 따로 받는 조각이고, 그리지 못한 까닭을 가려 적는다.
	if strings.Contains(renderHist(t, ui.NewHistTopoView(histTopology(), "demo-corp", local, "", ui.Page{Title: "x", Lang: ui.KO})), "<svg") {
		t.Error("the screen carries a drawing: it must come as a separate fragment")
	}
	for state, word := range map[string]string{
		ui.PictureNoDot: "이 기계에 없어", ui.PictureFailed: "그리지 못했습니다", ui.PictureRefused: "화면이 싣지 않는 요소",
		ui.PictureTimeout: "정한 시간을 넘겨", ui.PictureTooLarge: "크기 상한을 넘어",
	} {
		var b strings.Builder
		if err := ui.RenderHistPicture(&b, ui.KO, "", state, false); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), `data-picture="`+state+`"`) || !strings.Contains(b.String(), word) {
			t.Errorf("the picture fragment for %q: %s", state, b.String())
		}
	}
	var b strings.Builder
	if err := ui.RenderHistPicture(&b, ui.EN, `<svg xmlns="http://www.w3.org/2000/svg"></svg>`, "", false); err != nil || !strings.Contains(b.String(), "data-hist-picture") || !strings.Contains(b.String(), "<svg") {
		t.Errorf("a drawn picture fragment: %v %s", err, b.String())
	}
	b.Reset()
	if err := ui.RenderHistPicture(&b, ui.KO, "", "", true); err != nil || !strings.Contains(b.String(), `data-picture="unreadable"`) {
		t.Errorf("the fragment of an unreadable history: %v %s", err, b.String())
	}
}
