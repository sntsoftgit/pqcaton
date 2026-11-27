// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"context"
	"io"
	"sort"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// 이력의 스냅샷을 작업 폴더의 스냅샷과 견준 결과. **지문으로만 견준다.** 시각이 가깝다고 같다고 하지 않는다.
const (
	CompareSame    = "same"   // 같은 노드의 지문과 규칙 판이 같다
	CompareDiffer  = "differ" // 같은 노드의 지문이나 규칙 판이 다르다
	CompareUnknown = "none"   // 견줄 것이 없다(작업 폴더에 그 노드의 결과가 없거나 결과를 읽지 못했다)
)

// 그림의 상태. 처리기가 `dot`을 돌린 결과를 그대로 넣는다.
const (
	PictureDrawn   = ""
	PictureNoDot   = "no-dot"
	PictureFailed  = "failed"
	PictureRefused = "refused"
	// PictureTimeout · PictureTooLarge — 그리는 데 둔 상한(시간 · 크기)을 넘었다.
	PictureTimeout  = "timeout"
	PictureTooLarge = "too-large"
)

// LocalSnapshot — 작업 폴더의 관측 결과에서 화면이 만든 스냅샷 하나의 지문.
type LocalSnapshot struct{ Digest, Ruleset string }

// HistTopoView — 「이력에서 앱별 통신 보기」. 02의 대조 그림과 다른 것을 본다: 판정 조직의 pqcota 이력에 **적재된** 관측에서
// 노드마다 어느 앱이 무엇과 통신했는지다. 선언과 대조하지 않는다.
type HistTopoView struct {
	Page
	// Org — 조회한 조직. 판정 조직이다.
	Org string
	T   upstream.Topology
	// Rows — 출처 표. 이력의 노드마다 한 줄.
	Rows []HistNodeRow
	// LocalRead — 작업 폴더의 결과를 읽었는가. 못 읽었으면 모든 줄이 비교 불가다.
	LocalRead bool
	// OnlyLocal — 작업 폴더에는 결과가 있는데 이력에 스냅샷이 없는 노드.
	OnlyLocal []string
	// Node — 요약 표에서 고른 노드. 비어 있으면 모든 노드다. 그림은 고른 것과 관계없이 조직 전체다.
	Node string
	// Err — 이력을 읽지 못했다. 까닭(연결 정보는 싣지 않은 분류)이다.
	Err string
}

// HistNodeRow — 출처 표의 한 줄.
type HistNodeRow struct {
	upstream.TopoNode
	Compare string
}

// NewHistTopoView — 이력에서 읽은 것과 작업 폴더의 지문으로 화면을 세운다. local이 nil이면 작업 폴더를 읽지 못한 것이다.
func NewHistTopoView(t upstream.Topology, org string, local map[string]LocalSnapshot, node string, page Page) HistTopoView {
	v := HistTopoView{Page: page, Org: org, T: t, LocalRead: local != nil}
	inHistory := map[string]bool{}
	for _, n := range t.Nodes {
		inHistory[n.ID] = true
		row := HistNodeRow{TopoNode: n, Compare: CompareUnknown}
		if l, ok := local[n.ID]; ok {
			row.Compare = CompareDiffer
			if l.Digest == n.Digest && l.Ruleset == n.Ruleset {
				row.Compare = CompareSame
			}
		}
		v.Rows = append(v.Rows, row)
		if n.ID == node {
			v.Node = node
		}
	}
	for id := range local {
		if !inHistory[id] {
			v.OnlyLocal = append(v.OnlyLocal, id)
		}
	}
	sort.Strings(v.OnlyLocal)
	return v
}

// Shown — 요약 표에 보일 노드. 고른 노드가 있으면 그것 하나다.
func (v HistTopoView) Shown() []upstream.TopoNode {
	if v.Node == "" {
		return v.T.Nodes
	}
	for _, n := range v.T.Nodes {
		if n.ID == v.Node {
			return []upstream.TopoNode{n}
		}
	}
	return nil
}

// histTime — 이력의 시각. UTC로 적는다(상류 그림과 같다).
func histTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

// short12 — 긴 식별자의 앞 열두 글자. 전체는 title로 든다.
func short12(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// compareLabel — 비교 결과의 이름.
func compareLabel(l Lang, c string) string {
	switch c {
	case CompareSame:
		return tCmpSame.In(l)
	case CompareDiffer:
		return tCmpDiffer.In(l)
	}
	return tCmpNone.In(l)
}

// dirLabel — 기록의 방향. 관측한 노드가 열었는지 받았는지다.
func dirLabel(l Lang, role string) string {
	switch role {
	case upstream.RoleClient:
		return tDirOpened.In(l)
	case upstream.RoleServer:
		return tDirAccepted.In(l)
	}
	return tDirUnknown.In(l)
}

// RenderHistPicture — 그림 조각. 화면이 뜬 뒤에 따로 받아 끼운다. svg는 검사를 지난 것이고, 비었으면 state가 까닭이다.
// unreadable이면 이력을 읽지 못한 것이다.
func RenderHistPicture(w io.Writer, l Lang, svg, state string, unreadable bool) error {
	return histPictureBody(l, svg, state, unreadable).Render(context.Background(), w)
}

// RenderHistTopo — 화면을 그린다.
func RenderHistTopo(w io.Writer, v HistTopoView) error {
	return histTopoPage(v).Render(context.Background(), w)
}
