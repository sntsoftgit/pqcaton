// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 이력에서 앱별 통신 보기(03에서 여는 조회 화면). 판정 조직의 pqcota 이력을 읽기만 한다. 그림은 상류 함수가 그리고
// (pqcota-inventory -topology와 같다), 화면은 같은 입력에서 요약 표를 만든다. 엔드포인트는 **그 조직에 등록된 것만** 쓴다.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// openEndpoints — 조직의 등록 엔드포인트(조회용). 표를 만들지 않고 읽기 권한만 있는 계정으로 열린다. 시험이 메모리 저장소로 바꾼다.
var openEndpoints = func(dsn, org string) (inventory.EndpointLister, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), upstream.Timeout)
	defer cancel()
	m, err := inventory.OpenPgMetaStoreIn(ctx, dsn, org)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", upstream.ErrUnreachable, upstream.Classify(err))
	}
	return m, m.Close, nil
}

// readHistTopology — 판정 조직의 이력에서 토폴로지를 읽는다. 연결 정보는 읽을 때마다 다시 가져오고 어디에도 남기지 않는다.
func (s *server) readHistTopology() (upstream.Topology, error) {
	if s.upstream == nil {
		return upstream.Topology{}, fmt.Errorf("%w: not connected", upstream.ErrUnreachable)
	}
	dsn, _, err := s.loadDSN()
	if err != nil {
		return upstream.Topology{}, err
	}
	eps, closeEps, err := openEndpoints(dsn, s.upstreamOrgName)
	if err != nil {
		return upstream.Topology{}, err
	}
	defer closeEps()
	return upstream.ReadTopology(s.upstream, eps)
}

// localSnapshots — 작업 폴더의 관측 결과에서 화면이 만든 스냅샷의 지문(원천 노드별). 결과를 읽지 못했으면 nil이다.
func localSnapshots(wd *world) map[string]ui.LocalSnapshot {
	if wd == nil || wd.resErr != nil || wd.res == nil {
		return nil
	}
	out := map[string]ui.LocalSnapshot{}
	for _, sn := range wd.res.Snapshots {
		out[sn.SourceNodeID] = ui.LocalSnapshot{Digest: sn.Digest, Ruleset: sn.RulesetVersion}
	}
	return out
}

// histTopology — 조회 화면. 이력을 읽지 못해도 화면은 뜨고(503), 어느 조직을 읽으려 했는지와 까닭을 보인다.
// **여기서는 `dot`을 돌리지 않는다.** 그림은 화면이 뜬 뒤 histTopologyPicture가 따로 그린다.
func (s *server) histTopology(w http.ResponseWriter, r *http.Request) {
	if s.needsDecl(w, r) {
		return
	}
	wd := s.load()
	page := s.nextPage(r, ui.ScreenSurvey, s.results, s.steps(wd))
	page.Title = ui.ScreenTitle(ui.ScreenHistTopo, page.Lang)
	page.Self = ui.ScreenHistTopo

	t, err := s.readHistTopology()
	v := ui.NewHistTopoView(t, s.upstreamOrgName, localSnapshots(wd), r.URL.Query().Get("node"), page)
	status := http.StatusOK
	if err != nil {
		fmt.Fprintln(os.Stderr, "history topology:", err)
		v.Err, status = err.Error(), http.StatusServiceUnavailable
	}
	htmlStatus(w, r, status, func(out io.Writer) error { return ui.RenderHistTopo(out, v) })
}

// histTopologyPicture — 그림 조각. 조직 전체의 그림이라 노드가 많으면 오래 걸리고 커진다. 그래서 표와 따로 받고, 그리는
// 데 시간 · 크기 상한을 둔다(renderDOT). 같은 DOT은 다시 그리지 않는다. 그리지 못해도 200으로 까닭을 적은 조각을 준다:
// 끼워 넣는 쪽이 오류 응답을 화면에 싣지 않기 때문이다.
func (s *server) histTopologyPicture(w http.ResponseWriter, r *http.Request) {
	l := ui.PickLang(r)
	t, err := s.readHistTopology()
	svg, state := "", ""
	if err != nil {
		fmt.Fprintln(os.Stderr, "history topology:", err)
	} else {
		svg, state = renderDOT(t.DOT)
	}
	html(w, r, func(out io.Writer) error { return ui.RenderHistPicture(out, l, svg, state, err != nil) })
}

// histTopologyDOT — 그 그림의 DOT. 상류가 낸 그대로이고 `dot`이 없어도 받을 수 있다. 브라우저가 열지 않고 받게 한다.
func (s *server) histTopologyDOT(w http.ResponseWriter, r *http.Request) {
	t, err := s.readHistTopology()
	if err != nil {
		errorPage(w, r, http.StatusServiceUnavailable, ui.ErrInputBroken, fmt.Errorf("history topology: %w", err))
		return
	}
	w.Header().Set("Content-Type", "text/vnd.graphviz; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pqcota-topology.dot"`)
	_, _ = io.WriteString(w, t.DOT)
}
