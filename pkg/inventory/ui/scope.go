package ui

import (
	"context"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

// ScopeView — 자산 스코프 화면이 보는 것.
//
// **변경을 계층으로 묶어** 보여 준다 — 계층 하나에 결론 하나가 기본이다(§3.4). 규칙 한
// 줄씩 승인하는 리뷰는 수천 대에서 끝나지 않는다.
type ScopeView struct {
	Page
	Session scope.Session
	Layers  []LayerGroup
	// Audited — 근거 없이 확정할 수 없는 변경 수. 「안 본다」는 사고 뒤에 근거를 대야 한다.
	Audited int
}

// LayerGroup — 한 계층과 그에 묶인 변경들.
type LayerGroup struct {
	Name       string
	Conclusion string
	Changes    []scope.ChangeItem
	Audited    int
}

// NewScopeView — 세션을 화면이 보는 모양으로 옮긴다.
func NewScopeView(sf scope.Session, page Page) ScopeView {
	byLayer := map[string]*LayerGroup{}
	var order []string
	for _, c := range sf.Changes {
		g, ok := byLayer[c.Layer]
		if !ok {
			g = &LayerGroup{Name: c.Layer, Conclusion: sf.LayerDecisions[c.Layer]}
			byLayer[c.Layer] = g
			order = append(order, c.Layer)
		}
		g.Changes = append(g.Changes, c)
		if c.Audited {
			g.Audited++
		}
	}
	sort.Strings(order)
	v := ScopeView{Page: page, Session: sf, Audited: sf.AuditedCount()}
	for _, n := range order {
		v.Layers = append(v.Layers, *byLayer[n])
	}
	return v
}

// RenderScope — 자산 스코프 화면을 쓴다.
func RenderScope(w io.Writer, v ScopeView) error {
	return scopePage(v).Render(context.Background(), w)
}

// ApplyScope — 폼에서 온 값을 세션에 얹는다. **받은 세션을 고쳐서 돌려준다** — 부르는 쪽이
// 방금 읽어 온 것에 얹으므로 그 사이 파일을 고친 사람의 편집이 사라지지 않는다.
func ApplyScope(sf scope.Session, f url.Values) scope.Session {
	sf.Reviewer = strings.TrimSpace(f.Get("reviewer"))
	sf.Signature = strings.TrimSpace(f.Get("signature"))
	if sf.LayerDecisions == nil {
		sf.LayerDecisions = map[string]string{}
	}
	for layer := range sf.LayerDecisions {
		sf.LayerDecisions[layer] = strings.TrimSpace(f.Get("layer:" + layer))
	}
	for i, c := range sf.Changes {
		sf.Changes[i].Conclusion = strings.TrimSpace(f.Get("change:" + c.ID))
	}
	return sf
}
