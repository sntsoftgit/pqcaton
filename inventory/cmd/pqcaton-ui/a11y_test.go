// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"regexp"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func attr(n *xhtml.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func walk(n *xhtml.Node, fn func(*xhtml.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

// nameless: 접근 가능한 이름이 없는 입력 · 선택 · 여러 줄 입력. aria-label, aria-labelledby, 감싼
// <label>, <label for>가 이름이다. 숨은 필드와 버튼(글자가 이름이다)은 세지 않는다.
func nameless(t *testing.T, body string) []string {
	t.Helper()
	doc, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	labelled := map[string]bool{}
	walk(doc, func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "label" {
			if f, ok := attr(n, "for"); ok {
				labelled[f] = true
			}
		}
	})
	var out []string
	var visit func(n *xhtml.Node, inLabel bool)
	visit = func(n *xhtml.Node, inLabel bool) {
		if n.Type == xhtml.ElementNode {
			switch n.Data {
			case "label":
				inLabel = true
			case "input", "select", "textarea":
				typ, _ := attr(n, "type")
				if typ == "hidden" || typ == "submit" || typ == "button" {
					break
				}
				label, _ := attr(n, "aria-label")
				_, by := attr(n, "aria-labelledby")
				id, _ := attr(n, "id")
				if strings.TrimSpace(label) == "" && !by && !inLabel && !(id != "" && labelled[id]) {
					name, _ := attr(n, "name")
					out = append(out, n.Data+" "+name)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c, inLabel)
		}
	}
	visit(doc, false)
	return out
}

// IC-UI45 — **일곱 화면 · 두 말에서 이름 없는 입력이 하나도 없다.** 표 칸의 입력은 「노드 2 IP」처럼
// 몇 번째 무엇의 어느 칸인지를 이름으로 단다. 「행 추가」가 돌려주는 조각도 같다.
func TestEveryControlHasAName(t *testing.T) {
	s := observedServer(t)
	for _, sc := range ui.AllScreens() {
		for _, lang := range []string{"ko", "en"} {
			if left := nameless(t, get(t, s, sc+"?lang="+lang).Body.String()); len(left) > 0 {
				t.Errorf("%s (%s): controls without a name: %v", sc, lang, left)
			}
		}
	}
	for _, path := range []string{
		ui.RowPath + "?kind=node&node=0&i=5&lang=ko", ui.RowPath + "?kind=asset&node=0&i=5&lang=ko",
		ui.RowPath + "?kind=edge&node=0&i=5&lang=ko", ui.ScopeRowPath + "?layer=0&i=5&lang=ko",
	} {
		body := "<table>" + get(t, s, path).Body.String() + "</table>"
		if left := nameless(t, body); len(left) > 0 {
			t.Errorf("%s: controls without a name: %v", path, left)
		}
	}
	body := get(t, s, ui.ScreenDecl+"?lang=ko").Body.String()
	if !strings.Contains(body, `aria-label="노드 1 IP"`) {
		t.Error(`the first node's IP cell is not named "노드 1 IP"`)
	}
}

var firstLink = regexp.MustCompile(`<body class="wb">\s*<a class="skip" href="#main">`)

// IC-UI46 — **지금 자리를 기계도 안다.** 첫 Tab이 닿는 자리는 「본문으로 건너뛰기」이고, 본문은
// `<main id="main">` 하나다. 지금 단계 카드에 `aria-current="step"`, 조회 탭에 `aria-current="page"`가
// 붙고 다른 카드에는 붙지 않는다.
func TestCurrentPlaceIsMarked(t *testing.T) {
	s := observedServer(t)
	for _, sc := range ui.AllScreens() {
		body := get(t, s, sc+"?lang=ko").Body.String()
		if !firstLink.MatchString(body) || !strings.Contains(body, "본문으로 건너뛰기") {
			t.Errorf("%s: the skip link is not the first thing in the page", sc)
		}
		if strings.Count(body, `<main id="main">`) != 1 {
			t.Errorf("%s: want exactly one <main id=\"main\">", sc)
		}
		steps := strings.Count(body, `aria-current="step"`)
		pages := strings.Count(body, `aria-current="page"`)
		if sc == ui.ScreenInventory {
			if steps != 0 || pages != 1 {
				t.Errorf("%s: aria-current step %d, page %d; want 0 and 1", sc, steps, pages)
			}
			continue
		}
		if steps != 1 || pages != 0 {
			t.Errorf("%s: aria-current step %d, page %d; want 1 and 0", sc, steps, pages)
		}
		if !regexp.MustCompile(`href="` + sc + `" aria-current="step"`).MatchString(body) {
			t.Errorf("%s: the current card is not the one marked", sc)
		}
	}
}

// IC-UI47 — **알림은 역할을 갖는다.** 저장 성공은 `role="status"`로 조용히 읽히고, 막힌 이유와 거절
// 페이지는 `role="alert"`로 곧바로 읽힌다.
func TestNoticesHaveRoles(t *testing.T) {
	s := observedServer(t)
	if body := get(t, s, ui.ScreenReview+"?msg=saved").Body.String(); !strings.Contains(body, `<p class="msg ok" role="status">`) {
		t.Error("the success notice has no status role")
	}
	if body := get(t, s, ui.ScreenReview+"?problem=no").Body.String(); !strings.Contains(body, `<div class="msg bad" role="alert">`) {
		t.Error("the problem box has no alert role")
	}
	if body := postForm(t, s, "/save", withRev(nil, "rev/1 stale")).Body.String(); !strings.Contains(body, `<div class="msg bad" role="alert">`) {
		t.Error("the refusal page has no alert role")
	}
}

// IC-UI48 — **포커스 스크립트는 바이너리에 담겨 나가고, 포커스를 가리지 않는다.** `app.js`가 정적
// 파일로 나가고 새 껍데기가 그것을 싣는다. 행 추가 · 제거 버튼이 data-focus로 고른다. 어느 화면에도
// 양수 tabindex가 없고, 스타일이 포커스 표시를 지우지 않는다.
func TestFocusScriptShipsAndKeepsFocusVisible(t *testing.T) {
	s := observedServer(t)
	js := get(t, s, ui.StaticPath+"app.js")
	if js.Code != 200 || !strings.Contains(js.Body.String(), "htmx:afterSettle") || !strings.Contains(js.Body.String(), "SPDX-"+"License-Identifier: BUSL-1.1") {
		t.Fatalf("app.js: status %d", js.Code)
	}
	positive := regexp.MustCompile(`tabindex="[1-9]`)
	for _, sc := range ui.AllScreens() {
		body := get(t, s, sc).Body.String()
		if !strings.Contains(body, `src="/static/app.js"`) {
			t.Errorf("%s: the new shell does not load app.js", sc)
		}
		if positive.MatchString(body) {
			t.Errorf("%s: a positive tabindex", sc)
		}
	}
	decl := get(t, s, ui.ScreenDecl).Body.String()
	for _, want := range []string{`data-focus="new"`, `data-focus="next"`} {
		if !strings.Contains(decl, want) {
			t.Errorf("the declaration screen has no %s button", want)
		}
	}
	css := get(t, s, ui.StaticPath+"app.css").Body.String()
	if regexp.MustCompile(`outline:\s*(none|0)\b`).MatchString(css) || strings.Contains(css, ":focus-visible { outline: none") {
		t.Error("the stylesheet hides the focus ring")
	}
}
