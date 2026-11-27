// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"context"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	kscope "github.com/randyinthedev-hash/pqcota-common/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

// ScopeView — 자산 스코프 화면이 보는 것.
//
// **변경을 계층으로 묶어** 보여 준다 — 계층 하나에 결론 하나가 기본이다(§3.4). 규칙 한
// 줄씩 승인하는 리뷰는 수천 대에서 끝나지 않는다.
type ScopeView struct {
	Page
	// Finalized — 이 스코프 검토는 확정되었다. 화면은 읽기 전용이고 새 회차를 여는 단추를 둔다.
	Finalized bool
	Session   scope.Session
	Layers    []LayerGroup
	// Audited — 근거 없이 확정할 수 없는 변경 수. 인벤토리에서 뺀 자산은 나중에 「왜 이건
	// 안 봤나」에 답해야 하므로, 왜 뺐는지가 없으면 확정되지 않는다.
	Audited int
	// Editing — 화면에서 고칠 수 있는 계층들. 계층 파일을 주지 않았으면 비어 있고,
	// 그때 화면은 **승인만** 하는 자리가 된다(v0.11.0까지의 모습).
	Editing []LayerEdit
	// ActivationNext — 방금 자산 스코프를 확정했고(확정 알림), 그 정책이 04와 같은 판정으로 활성화를 기다린다. 확정 알림 아래에
	// 다음 단계(03에서 현재 대조 보기 · 02에서 재적재 · 활성화 보기)를 보인다. 링크는 화면이 정한 두 주소뿐이다.
	ActivationNext bool
	// RulesUnsaved — 방금 「저장만」이나 「확정」을 거절했다: 양식의 규칙이 계층 파일과 달랐다. 이때 화면의 규칙 · 결론 ·
	// 승인 칸은 파일이 아니라 사람이 적은 그대로다.
	RulesUnsaved bool
	// Refused · FieldErrors — 방금 저장을 거절했고(규칙 저장 · 저장만 · 확정), 저장을 막은 규칙 줄들. 이때 화면의 값은
	// 사람이 적은 그대로다.
	Refused     bool
	FieldErrors []DeclFieldError
}

func (v ScopeView) fieldErrs() fieldErrs {
	if len(v.FieldErrors) == 0 {
		return nil
	}
	out := fieldErrs{}
	for _, e := range v.FieldErrors {
		out[e.Field] = e
	}
	return out
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
	Rules []RuleRow
}

// RuleRow — 편집 표의 규칙 한 줄.
type RuleRow struct {
	scope.Rule
	// Was — 계층 파일에 있던 규칙이다(양식의 숨은 칸 `rule.L.I.was`). 조건을 모두 비우면 지운 것으로 읽지 않고 거절한다.
	Was bool
	// Removing — 「제거」로 표시했다(`rule.L.I.remove`). 「규칙 저장」을 누르면 파일에서 빠진다. 그때까지는 화면에 남는다.
	Removing bool
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
// 이것을 나눠 둔 것은 입력을 주지 않은 자리를 만들지 않기 위해서다 — 계층 파일 없이
// 편집 표를 그리면 저장할 곳이 없는 칸을 사람이 채우게 된다.
func (v ScopeView) Editable(files []scope.LayerFile) ScopeView {
	for i, f := range files {
		e := LayerEdit{Index: i, Name: f.Layer.Name, Path: f.Path}
		for _, r := range f.Layer.Rules {
			e.Rules = append(e.Rules, RuleRow{Rule: toRule(r), Was: true})
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

// 규칙 줄의 폼 이름 가운데 값이 아닌 것: 기존 규칙이라는 표시와 「제거」 표시.
const (
	ruleWas    = "was"
	ruleRemove = "remove"
)

// FieldRuleEmpty — 규칙의 조건(runtime · lib · app_key)이 모두 비었다.
const FieldRuleEmpty = "rule-empty"

// KindRule — 저장을 막은 칸이 자산 스코프 화면의 규칙 줄에 있다.
const KindRule = "rule"

// readRules — 양식의 규칙 줄을 읽는다. edits는 화면에 다시 그릴 줄(틀린 줄 · 「제거」로 표시한 줄 포함, 번호는 다시
// 매긴다), out은 계층 파일에 쓸 규칙, errs는 저장을 막는 줄이다.
//
// **조건을 비운 줄은 지운 줄이 아니다.** 예전에는 세 칸이 빈 줄을 건너뛰어, 조건을 고치다 만 규칙이 note가 남아 있어도
// 알림 없이 사라졌다. 지우는 뜻은 「제거」가 받는다(그 줄은 「규칙 저장」 때 빠진다). 버리는 것은 쓰지 않은 새 줄뿐이다:
// 기존 규칙이 아니고, action이 기본값(include)이고, 나머지 칸이 모두 빈 줄.
//
// **조건이 빈 줄을 규칙으로 만들지도 않는다.** pqcota는 빈 칸을 `*`로 읽으므로 그대로 쓰면 `exclude,*,*,*`, 곧 전부
// 제외가 된다. 「전부」를 뜻하려면 `*`를 적는다.
//
// 줄 번호가 끊겨도 읽는다(formRows). 끊기는 자리에서 멈추면 그 뒤의 규칙이 저장에서 통째로 빠진다.
func readRules(files []scope.LayerFile, f url.Values) (edits []LayerEdit, out []scope.LayerFile, errs []DeclFieldError) {
	for li, lf := range files {
		e := LayerEdit{Index: li, Name: lf.Layer.Name, Path: lf.Path}
		lf.Layer.Rules = nil
		for _, i := range formRows(f, "rule."+strconv.Itoa(li)+".", ".action") {
			get := func(name string) string { return strings.TrimSpace(f.Get(ruleField(li, i, name))) }
			r := RuleRow{
				Rule: scope.Rule{Action: "include", Runtime: get("runtime"), Lib: get("lib"), AppKey: get("app_key"), Note: get("note")},
				Was:  get(ruleWas) != "", Removing: get(ruleRemove) != "",
			}
			if get("action") == "exclude" {
				r.Action = "exclude"
			}
			unconditional := r.Runtime == "" && r.Lib == "" && r.AppKey == ""
			if unconditional && !r.Was && r.Action == "include" && r.Note == "" {
				continue // 쓰지 않은 새 줄
			}
			j := len(e.Rules)
			e.Rules = append(e.Rules, r)
			switch {
			case r.Removing:
				// 「제거」로 표시한 줄. 화면에는 남기고 파일에는 쓰지 않는다.
			case unconditional:
				errs = append(errs, DeclFieldError{Field: ruleField(li, j, "runtime"), Code: FieldRuleEmpty,
					kind: KindRule, node: li, row: j, col: T{KO: "runtime", EN: "runtime"}})
			default:
				lf.Layer.Rules = append(lf.Layer.Rules, kscope.AssetRule{
					Exclude: r.Action == "exclude", Runtime: r.Runtime, Lib: r.Lib, AppKey: r.AppKey, Note: r.Note})
			}
		}
		edits = append(edits, e)
		out = append(out, lf)
	}
	return edits, out, errs
}

// ApplyLayers — 폼에서 온 규칙으로 계층을 **다시 만든다.** 얹지 않는다: 「제거」로 표시한 줄이 되살아나지 않아야 한다.
//
// errs — 저장을 막는 줄(readRules). 하나라도 있으면 부르는 쪽은 아무것도 쓰지 않고 적은 값을 되돌려 보인다(EditableForm).
func ApplyLayers(files []scope.LayerFile, f url.Values) ([]scope.LayerFile, []DeclFieldError) {
	_, out, errs := readRules(files, f)
	return out, errs
}

// EditableForm — 저장을 거절한 양식의 규칙 줄을 **적은 값 그대로** 편집 표로 만든다. 파일에서 읽지 않는다.
func (v ScopeView) EditableForm(files []scope.LayerFile, f url.Values) ScopeView {
	v.Editing, _, v.FieldErrors = readRules(files, f)
	v.Refused = true
	return v
}

// RuleRowFromForm — 「제거」와 「되돌리기」가 돌려줄 줄 하나. 그 줄에 적혀 있던 값을 그대로 들고, 표시만 바꾼다.
func RuleRowFromForm(f url.Values, layer, i int, removing bool) RuleRow {
	get := func(name string) string { return strings.TrimSpace(f.Get(ruleField(layer, i, name))) }
	r := RuleRow{
		Rule: scope.Rule{Action: "include", Runtime: get("runtime"), Lib: get("lib"), AppKey: get("app_key"), Note: get("note")},
		Was:  get(ruleWas) != "", Removing: removing,
	}
	if get("action") == "exclude" {
		r.Action = "exclude"
	}
	return r
}

// RenderRuleState — 「제거」 · 「되돌리기」가 갈아 끼울 줄.
func RenderRuleState(w io.Writer, l Lang, layer, i int, r RuleRow) error {
	return ruleRow(l, layer, i, r, nil).Render(context.Background(), w)
}

// RulesShownField — 양식이 규칙 편집기를 싣고 왔다는 표시(숨은 칸). 편집기가 없는 양식(계층 파일 없이 승인만 하는
// 화면)은 규칙 칸이 없으므로, 이 표시가 있을 때만 양식의 규칙을 파일과 견준다.
const RulesShownField = "rules.shown"

// RulesEdited — 양식에 실려 온 규칙이 계층 파일의 규칙과 다른가.
//
// **「저장만」과 「확정」은 규칙을 쓰지 않는다**(세션만 쓴다). 규칙을 고친 채 그 버튼을 누르면 고친 규칙이 화면과 함께
// 사라지므로, 부르는 쪽이 이것으로 알아보고 쓰지 않은 채 적은 값을 되돌려 보인다. 규칙을 쓰는 것은 「규칙 저장」이다.
// 견주는 것은 파일에 쓸 바이트다(ApplyLayers가 만드는 것과 같다).
func RulesEdited(files []scope.LayerFile, f url.Values) (bool, error) {
	if f.Get(RulesShownField) == "" {
		return false, nil
	}
	edited, errs := ApplyLayers(files, f)
	if len(errs) > 0 {
		return true, nil // 저장할 수 없는 줄도 고친 것이다
	}
	for i := range files {
		was, err := scope.EncodeLayer(files[i])
		if err != nil {
			return false, err
		}
		now, err := scope.EncodeLayer(edited[i])
		if err != nil {
			return false, err
		}
		if string(was) != string(now) {
			return true, nil
		}
	}
	return false, nil
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

// ScopeSummary — 스코프 요약 화면(`/scope-next`)이 쓰는 숫자.
//
// **세는 일을 뷰에 둔다**(DeclSummary와 같은 선). templ 안에서 세면 같은 계산이
// 화면마다 흩어지고, 그러면 화면과 요약이 다른 답을 내는 날이 온다.
type ScopeSummary struct {
	// Layers — 화면에서 고칠 수 있는 계층. 계층 파일을 주지 않았으면 0이고, 그때 화면은
	// 승인만 하는 자리가 된다.
	Layers int
	// Rules — 그 계층들에 적힌 규칙을 합친 수.
	Rules int
	// Changes — 판정을 기다리는 변경.
	Changes int
	// Audited — 그 가운데 **왜 뺐는지를 적어야** 확정되는 것.
	Audited int
}

// Summary — 요약의 숫자 넷.
func (v ScopeView) Summary() ScopeSummary {
	s := ScopeSummary{Layers: len(v.Editing), Audited: v.Audited}
	for _, e := range v.Editing {
		s.Rules += len(e.Rules)
	}
	for _, g := range v.Layers {
		s.Changes += len(g.Changes)
	}
	return s
}

// scopeEditAnchor — 요약에서 승인 폼으로 보내는 자리.
const scopeEditAnchor = "scope-edit"

// RenderScope — 자산 스코프 화면(`/scope`).
func RenderScope(w io.Writer, v ScopeView) error {
	return scopeNextPage(v).Render(context.Background(), w)
}
