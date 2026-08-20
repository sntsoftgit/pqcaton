package ui

import (
	"context"
	"io"
	"net/url"
	"sort"
	"strings"

	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"

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
	// Audited — 근거 없이 확정할 수 없는 변경 수. 인벤토리에서 뺀 자산은 나중에 「왜 이건
	// 안 봤나」에 답해야 하므로, 왜 뺐는지가 없으면 확정되지 않는다.
	Audited int
	// Editing — 화면에서 고칠 수 있는 계층들. 계층 파일을 주지 않았으면 비어 있고,
	// 그때 화면은 **승인만** 하는 자리가 된다(v0.11.0까지의 모습).
	Editing []LayerEdit
}

// LayerEdit — 화면에서 고치는 계층 하나.
//
// **합본이 아니라 계층을 고친다.** 합본에 쓰면 그 규칙이 조직에서 왔는지 노드군에서
// 왔는지가 사라지고, 다음 리뷰에서 누구에게 물어야 할지 알 수 없게 된다.
type LayerEdit struct {
	// Index — 폼 이름에 들어가는 번호. **순서가 곧 상속이다** — 같은 자산에 규칙이 여럿
	// 걸리면 뒤 계층의 것이 적용된다.
	Index int
	Name  string
	Path  string
	Rules []scope.Rule
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

// Editable — 계층 파일을 붙여 **고칠 수 있는 화면**으로 만든다.
//
// 이것을 나눠 둔 것은 재료를 주지 않은 자리를 만들지 않기 위해서다 — 계층 파일 없이
// 편집 표를 그리면 저장할 곳이 없는 칸을 사람이 채우게 된다.
func (v ScopeView) Editable(files []scope.LayerFile) ScopeView {
	for i, f := range files {
		e := LayerEdit{Index: i, Name: f.Layer.Name, Path: f.Path}
		for _, r := range f.Layer.Rules {
			e.Rules = append(e.Rules, toRule(r))
		}
		v.Editing = append(v.Editing, e)
	}
	return v
}

func toRule(r kscope.AssetRule) scope.Rule {
	act := "include"
	if r.Exclude {
		act = "exclude"
	}
	return scope.Rule{Action: act, Runtime: r.Runtime, Lib: r.Lib, AppKey: r.AppKey, Note: r.Note}
}

// ApplyLayers — 폼에서 온 규칙으로 계층을 **다시 만든다.**
//
// 얹지 않고 다시 만드는 이유는 선언 편집과 같다 — 줄을 지우는 방법이 「칸을 비우는 것」
// 이므로, 기존 것에 얹으면 지운 줄이 되살아난다.
//
// **세 칸이 모두 빈 줄은 규칙이 아니라 빈 줄이다.** pqcota 는 빈 칸을 `*`로 읽으므로,
// 그대로 만들면 `exclude,*,*,*` — **전부 제외**가 된다. 미리 열어 둔 빈 줄에서 action 만
// 잘못 골라도 인벤토리가 통째로 비는 규칙이 생긴다. 「전부」를 뜻하려면 `*`를 적는다.
func ApplyLayers(files []scope.LayerFile, f url.Values) []scope.LayerFile {
	out := make([]scope.LayerFile, 0, len(files))
	for li, lf := range files {
		lf.Layer.Rules = nil
		for i := 0; ; i++ {
			act, ok := f[ruleField(li, i, "action")]
			if !ok {
				break
			}
			get := func(name string) string {
				return strings.TrimSpace(f.Get(ruleField(li, i, name)))
			}
			runtime, lib, appKey := get("runtime"), get("lib"), get("app_key")
			if runtime == "" && lib == "" && appKey == "" {
				continue
			}
			lf.Layer.Rules = append(lf.Layer.Rules, kscope.AssetRule{
				Exclude: strings.TrimSpace(first(act)) == "exclude",
				Runtime: runtime, Lib: lib, AppKey: appKey, Note: get("note"),
			})
		}
		out = append(out, lf)
	}
	return out
}

// RenderScope — 자산 스코프 화면을 쓴다.
func RenderScope(w io.Writer, v ScopeView) error {
	return scopePage(v).Render(context.Background(), w)
}

// kindLabel — 변경의 종류를 그 말로. 파일에는 코드(added · removed)가 담기고, 사람이
// 읽는 말은 화면이 고른다.
func kindLabel(l Lang, kind string) string {
	switch kind {
	case scope.KindAdded:
		return tKindAdded.In(l)
	case scope.KindRemoved:
		return tKindRemoved.In(l)
	}
	return kind
}

// RenderRuleRow — 「행 추가」가 돌려주는 규칙 한 줄과, 번호가 하나 오른 버튼.
func RenderRuleRow(w io.Writer, l Lang, layer, i int) error {
	return ruleRowFragment(l, layer, i).Render(context.Background(), w)
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
