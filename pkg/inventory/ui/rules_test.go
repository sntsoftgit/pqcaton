// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"net/url"
	"strings"
	"testing"

	kscope "github.com/randyinthedev-hash/pqcota-common/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func threeRules() []scope.LayerFile {
	return []scope.LayerFile{{Path: "/x/corp.csv", Layer: scope.Layer{Name: "corp", Rules: []kscope.AssetRule{
		{Exclude: true, Runtime: "openssl", Lib: "liba", AppKey: "/a", Note: "first"},
		{Exclude: false, Runtime: "jca", Lib: "libb", AppKey: "/b", Note: "second"},
		{Exclude: true, Runtime: "cng", Lib: "libc", AppKey: "/c", Note: "third"},
	}}}}
}

// rule: 양식의 규칙 한 줄. extra는 was · remove 같은 표시다.
func rule(f url.Values, i int, action, runtime, lib, appKey, note string, extra ...string) {
	at := "rule.0." + string(rune('0'+i)) + "."
	f.Set(at+"action", action)
	f.Set(at+"runtime", runtime)
	f.Set(at+"lib", lib)
	f.Set(at+"app_key", appKey)
	f.Set(at+"note", note)
	for _, e := range extra {
		f.Set(at+e, "1")
	}
}

func keys(rules []kscope.AssetRule) string {
	var out []string
	for _, r := range rules {
		out = append(out, r.Note)
	}
	return strings.Join(out, ",")
}

// IC-UI66 — **조건을 비운 규칙은 지운 규칙이 아니다.** 기존 규칙의 runtime · lib · app_key를 모두 비우면 note가 남아
// 있든 없든 그 줄을 짚는다. 새 줄은 기본값(include)이고 나머지가 모두 비었을 때만 쓰지 않은 줄로 버리고, exclude를
// 골랐거나 note를 적었는데 조건이 없으면 짚는다. 「제거」로 표시한 줄만 파일에서 빠진다. 세 규칙 가운데 하나를 표시하면
// 앞뒤 규칙이 그 순서로 남고, 줄 번호가 끊긴 양식도 끊긴 뒤까지 읽는다.
func TestRulesAreRemovedOnlyByTheRemoveMark(t *testing.T) {
	base := func() url.Values {
		f := url.Values{}
		rule(f, 0, "exclude", "openssl", "liba", "/a", "first", "was")
		rule(f, 1, "include", "jca", "libb", "/b", "second", "was")
		rule(f, 2, "exclude", "cng", "libc", "/c", "third", "was")
		return f
	}
	for _, c := range []struct {
		name string
		edit func(url.Values)
		bad  string
		kept string
	}{
		{"untouched", func(url.Values) {}, "", "first,second,third"},
		{"an existing rule with its three conditions emptied, note kept", func(f url.Values) { rule(f, 1, "include", "", " ", "", "second", "was") }, "rule.0.1.runtime=rule-empty", "first,third"},
		{"an existing rule with everything emptied", func(f url.Values) { rule(f, 1, "include", "", "", "", "", "was") }, "rule.0.1.runtime=rule-empty", "first,third"},
		{"a new row left at its defaults", func(f url.Values) { rule(f, 3, "include", "", "", "", "") }, "", "first,second,third"},
		{"a new row with exclude picked and no conditions", func(f url.Values) { rule(f, 3, "exclude", "", "", "", "") }, "rule.0.3.runtime=rule-empty", "first,second,third"},
		{"a new row with only a note", func(f url.Values) { rule(f, 3, "include", "", "", "", "why") }, "rule.0.3.runtime=rule-empty", "first,second,third"},
		{"a new row meaning everything", func(f url.Values) { rule(f, 3, "exclude", "*", "", "", "all") }, "", "first,second,third,all"},
		{"the middle rule marked for removal", func(f url.Values) { rule(f, 1, "include", "jca", "libb", "/b", "second", "was", "remove") }, "", "first,third"},
		{"a marked rule with its conditions emptied", func(f url.Values) { rule(f, 1, "include", "", "", "", "second", "was", "remove") }, "", "first,third"},
		{"a gap in the row numbers", func(f url.Values) {
			for _, k := range []string{"action", "runtime", "lib", "app_key", "note", "was"} {
				f.Del("rule.0.1." + k)
			}
		}, "", "first,third"},
	} {
		f := base()
		c.edit(f)
		got, bad := ui.ApplyLayers(threeRules(), f)
		if codes(bad) != c.bad {
			t.Errorf("%s: refused %q, want %q", c.name, codes(bad), c.bad)
		}
		if kept := keys(got[0].Layer.Rules); kept != c.kept {
			t.Errorf("%s: rules %q, want %q", c.name, kept, c.kept)
		}
	}
	// 가운데를 뺀 뒤에도 앞뒤 규칙의 칸이 그대로다.
	f := base()
	rule(f, 1, "include", "jca", "libb", "/b", "second", "was", "remove")
	got, _ := ui.ApplyLayers(threeRules(), f)
	want := threeRules()[0].Layer.Rules
	if len(got[0].Layer.Rules) != 2 || got[0].Layer.Rules[0] != want[0] || got[0].Layer.Rules[1] != want[2] {
		t.Errorf("after removing the middle rule: %+v", got[0].Layer.Rules)
	}
}

// IC-UI67 — **거절한 자산 스코프 화면은 규칙 줄을 적은 그대로 돌려주고, 「삭제 예정」 줄을 줄로 남긴다.** 조건을 비운 줄은
// aria-invalid와 그 줄로 가는 링크가 있고, note와 기존 규칙 표시가 그대로다. 「제거」로 표시한 줄은 값이 읽기 전용으로
// 남고 「삭제 예정」과 「되돌리기」가 있으며 표시(숨은 칸)가 양식에 실린다. 파일에서 그린 줄에는 「제거」가 있고 묻는다.
// 두 말 모두.
func TestRefusedScopeViewKeepsRuleRows(t *testing.T) {
	f := url.Values{ui.RulesShownField: {"1"}}
	rule(f, 0, "exclude", "openssl", "liba", "/a", "first", "was", "remove")
	rule(f, 1, "include", "", "", "", "typed-note", "was")
	rule(f, 2, "exclude", "cng", "libc", "/c", "third", "was")
	for _, c := range []struct {
		lang                       ui.Lang
		head, removing, undo, star string
	}{
		{ui.KO, "저장하지 않았습니다", "삭제 예정", "되돌리기", "별표(*)를 적으십시오"},
		{ui.EN, "Not saved", "To be removed", "Undo", "write *"},
	} {
		sf := scope.NewSession(scope.Layers(threeRules()), nil, "acme")
		v := ui.NewScopeView(sf, ui.Page{Lang: c.lang}).EditableForm(threeRules(), f)
		if !v.Refused || codes(v.FieldErrors) != "rule.0.1.runtime=rule-empty" {
			t.Fatalf("%s: refused %v with %q", c.lang, v.Refused, codes(v.FieldErrors))
		}
		var b strings.Builder
		if err := ui.RenderScope(&b, v); err != nil {
			t.Fatal(err)
		}
		body := b.String()
		for _, want := range []string{
			`id="save-refused"`, c.head, c.star, `href="#f-rule.0.1.runtime"`, `id="f-rule.0.1.runtime"`, `aria-describedby="e-rule.0.1.runtime"`,
			`name="rule.0.1.note" value="typed-note"`, `name="rule.0.1.was" value="1"`,
			`name="rule.0.0.remove" value="1"`, `name="rule.0.0.lib" value="liba"`, c.removing, c.undo, `undo=1`,
			`name="rule.0.2.lib" value="libc"`, `undo=0`, `hx-confirm=`, `hx-include="closest tr"`,
			`data-guard="dirty"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the refused scope screen has no %s", c.lang, want)
			}
		}
		if n := strings.Count(body, `aria-invalid="true"`); n != 1 {
			t.Errorf("%s: %d fields are marked invalid, want 1", c.lang, n)
		}
		if n := strings.Count(body, ` readonly`); n != 4 {
			t.Errorf("%s: %d read-only fields, want the four of the marked row", c.lang, n)
		}
	}
	// 파일에서 그린 화면: 모든 줄이 기존 규칙이고 표시한 줄이 없다.
	var b strings.Builder
	v := ui.NewScopeView(scope.NewSession(scope.Layers(threeRules()), nil, "acme"), ui.Page{Lang: ui.EN}).Editable(threeRules())
	if err := ui.RenderScope(&b, v); err != nil {
		t.Fatal(err)
	}
	body := b.String()
	if strings.Count(body, `.was" value="1"`) != 3 || strings.Contains(body, `.remove" value`) || strings.Contains(body, `class="removing`) || strings.Contains(body, `id="save-refused"`) {
		t.Error("a scope screen drawn from the files does not mark its three rules as existing, or shows a removal or a refusal")
	}
	var row strings.Builder
	if err := ui.RenderRuleRow(&row, ui.EN, 0, 7); err != nil {
		t.Fatal(err)
	}
	if got := row.String(); strings.Contains(got, ".was") || !strings.Contains(got, "undo=0") {
		t.Errorf("a new rule row is marked as an existing one, or has no Remove:\n%s", got)
	}
}
