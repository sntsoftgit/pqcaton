// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 선언의 노드와 접근 정보(hosts.csv)의 행을 잇는다.
//
// **노드의 정체는 선언이 정한다.** 접근 정보의 노드 ID는 선언의 노드 이름과 글자 그대로(대소문자까지) 같아야 그 노드의 접속
// 정보다. `observed_as`나 IP가 같다는 이유로 잇지 않는다: 접속 자격 증명이 다른 노드에 붙으면 안 된다. `observed_as`는 화면
// 밖에서 수집한 결과를 선언 노드에 붙이는 데만 쓴다(report.ResolveAssetNode).
//
//   - 선언에는 있고 접근 정보에 없는 노드: 「아직 관측하지 않는 노드」. 이번 관측에서 빠질 뿐이고 대조에는 남는다.
//   - 접근 정보에는 있고 선언에 없는 행: 관측과 04의 원격 실행을 시작하지 않는다. 화면은 파일을 고치지 않는다.
//
// 이 판정은 화면을 그릴 때와, **시작 요청을 처리할 때 현재 선언과 현재 파일을 다시 읽어** 한다. 화면을 연 뒤 선언에서 노드
// 이름을 바꿨으면, 예전 화면에서 보낸 시작 요청은 노드에 접속하기 전에 거절된다.

import (
	"errors"
	"os"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// errHostsUndeclared: 접근 정보에 선언에 없는 노드 ID가 있거나, 견줄 선언을 읽지 못했다.
var errHostsUndeclared = errors.New("the access details hold a node ID that is not in the declaration, or the declaration cannot be read")

// hostTargets: 지금 선언과 주어진 접근 정보로 줄을 만든다. 관측 상태(관측됨 · 실패)는 채우지 않는다(observeTargets가 채운다).
func (s *server) hostTargets(hosts []inventory.Host) ui.ObserveTargets {
	var t ui.ObserveTargets
	names, ok := s.declaredNodes()
	lines := s.hostLines(len(hosts))
	line := func(i int) int {
		if i < len(lines) {
			return lines[i]
		}
		return i + 2 // 줄 번호를 읽지 못했으면 머리글 한 줄 뒤의 차례로 어림한다
	}
	if !ok {
		t.DeclUnknown = true
		for i, h := range hosts {
			t.Orphans = append(t.Orphans, ui.OrphanHost{Row: line(i), ID: h.NodeID})
		}
		return t
	}
	declared, access := map[string]bool{}, map[string]bool{}
	for _, n := range names {
		declared[n] = true
	}
	for i, h := range hosts {
		if declared[h.NodeID] {
			access[h.NodeID] = true
		} else {
			t.Orphans = append(t.Orphans, ui.OrphanHost{Row: line(i), ID: h.NodeID})
		}
	}
	for _, n := range names {
		row := ui.TargetRow{Node: n, HasAccess: access[n], State: ui.TargetUnready}
		if row.HasAccess {
			row.State = ui.TargetReady
		}
		t.Rows = append(t.Rows, row)
	}
	return t
}

// hostLines: 접근 정보 파일의 행마다 그 행이 시작하는 파일의 줄 번호. 읽지 못하거나 행의 수가 맞지 않으면 nil이다.
func (s *server) hostLines(want int) []int {
	raw, err := os.ReadFile(s.hosts)
	if err != nil {
		return nil
	}
	return ui.HostLines(raw, want)
}

// declaredNodes: 지금 선언의 노드 이름(적힌 순서, 겹친 이름은 한 번). 선언이 없거나 읽지 못하면 ok가 거짓이다.
func (s *server) declaredNodes() ([]string, bool) {
	if s.decl == "" {
		return nil, false
	}
	if _, err := os.Stat(s.decl); err != nil {
		return nil, false
	}
	d, err := decl.Load(s.decl)
	if err != nil {
		return nil, false
	}
	seen := map[string]bool{}
	var out []string
	for _, n := range d.Nodes {
		if n.Name != "" && !seen[n.Name] {
			seen[n.Name] = true
			out = append(out, n.Name)
		}
	}
	return out, true
}

// hostsDeclared: 접근 정보의 노드 ID가 모두 지금 선언에 있는가. 아니면 errHostsUndeclared와 그 행들이다.
func (s *server) hostsDeclared(hosts []inventory.Host) ([]ui.OrphanHost, error) {
	t := s.hostTargets(hosts)
	if t.DeclUnknown || len(t.Orphans) > 0 {
		return t.Orphans, errHostsUndeclared
	}
	return nil, nil
}

// observeTargets: 01 관측의 「대상 노드의 접속 준비」. 줄마다 접속 준비에 더해, 지금 결과에 그 노드가 있는가와 가장 최근
// 실행이 그 노드에서 실패했는가를 채운다.
//
// 「관측됨」의 시각은 지금 결과를 낸 실행의 기록(results/의 실행 표지)에서만 온다. 표지가 없는 결과(화면 밖에서 받은 것)는
// 시각을 지어내지 않는다. 「실패」는 가장 최근 실행이 결과를 바꾸지 못하고 끝났고, 그 실행의 로그에 이 노드의 실패 근거가 있을
// 때만 적는다.
func (s *server) observeTargets(hosts []inventory.Host, wd *world) ui.ObserveTargets {
	t := s.hostTargets(hosts)
	cur := s.currentRun()
	inRun := map[string]bool{}
	if cur != nil {
		for _, n := range cur.Nodes {
			inRun[n.Node] = true
		}
	}
	inResults := map[string]bool{}
	if wd != nil && wd.res != nil {
		for n := range wd.res.SeenBy {
			inResults[n] = true
		}
	}
	failed := map[string]bool{}
	last := s.run.status()
	lastFailed := last != nil && last.Over() && last.State != ui.RunDone && (cur == nil || cur.ID != last.ID)
	if lastFailed {
		for _, n := range last.Progress().Nodes {
			if n.State == ui.NodeFailed {
				failed[n.Node] = true
			}
		}
	}
	for i := range t.Rows {
		r := &t.Rows[i]
		switch {
		case inRun[r.Node]:
			r.InResults, r.ObservedAt = true, cur.Ended
		case inResults[r.Node]:
			r.InResults = true
		}
		switch {
		case !r.HasAccess:
		case failed[r.Node]:
			r.State, r.FailedAt = ui.TargetFailed, last.Ended
		case r.InResults:
			r.State = ui.TargetObserved
		}
	}
	return t
}

// surveySource: 02 대조가 어느 결과로 대조했는가. 대조에 쓴 결과의 시각(실행 표지가 있을 때만), 가장 최근 실행이 결과를 바꾸지
// 못하고 끝났으면 그 시각과 상태, 그리고 선언에는 있고 결과에는 없는 노드다.
func (s *server) surveySource(wd *world) ui.SurveySource {
	src := ui.SurveySource{Empty: s.resultsEmpty()}
	cur := s.currentRun()
	seen := map[string]bool{}
	if wd != nil && wd.res != nil {
		for n := range wd.res.SeenBy {
			seen[n] = true
		}
	}
	if !src.Empty {
		if cur != nil {
			// 실행 표지는 결과가 모두 온 노드를 적는다. 자산이 하나도 없던 노드도 관측한 노드다.
			for _, n := range cur.Nodes {
				seen[n.Node] = true
			}
			src.At, src.Nodes = cur.Ended, len(cur.Nodes)
		} else {
			src.Outside, src.Nodes = true, len(seen)
		}
	}
	if last := s.run.status(); last != nil && last.Over() && last.State != ui.RunDone && (cur == nil || cur.ID != last.ID) {
		src.LastFailedAt, src.LastFailedState = last.Ended, last.State
	}
	if names, ok := s.declaredNodes(); ok {
		for _, n := range names {
			if !seen[n] {
				src.Absent = append(src.Absent, n)
			}
		}
	}
	return src
}
