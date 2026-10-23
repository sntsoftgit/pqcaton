// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// observedServer: 화면 다섯이 모두 열리고 대조 · 조회 표에 줄이 있는 서버.
func observedServer(t *testing.T) *server {
	t.Helper()
	s := everyScreen(t)
	observed(t, s.results, "web", "libssl.so.3", "/usr/sbin/nginx")
	observed(t, s.results, "odd-node", "libcrypto.so.3", "/usr/sbin/sshd")
	return s
}

var (
	anyTable     = regexp.MustCompile(`<table\b`)
	wrappedTable = regexp.MustCompile(`<div class="table-wrap">\s*<table\b`)
	editTable    = regexp.MustCompile(`(?s)<table class="[^"]*\beditable\b[^"]*"[^>]*>.*?</table>`)
	cell         = regexp.MustCompile(`<td\b[^>]*>`)
	row          = regexp.MustCompile(`<tr\b[^>]*>`)
)

// IC-UI43 — **표는 모두 가로 스크롤 상자 안에 있다.** 다섯 화면 · 두 말에서 `<table>`마다 바로 바깥에
// `.table-wrap`이 있다. 좁은 화면에서 표만 옆으로 밀리고 페이지 전체는 넘치지 않게 하는 자리다.
func TestEveryTableScrollsInItsBox(t *testing.T) {
	s := observedServer(t)
	for _, sc := range ui.AllScreens() {
		for _, lang := range []string{"ko", "en"} {
			body := get(t, s, sc+"?lang="+lang).Body.String()
			n, wrapped := len(anyTable.FindAllString(body, -1)), len(wrappedTable.FindAllString(body, -1))
			if n == 0 {
				t.Errorf("%s (%s): no table to check", sc, lang)
			}
			if wrapped != n {
				t.Errorf("%s (%s): %d of %d tables are outside a scroll box", sc, lang, n-wrapped, n)
			}
		}
	}
	css := get(t, s, ui.StaticPath+"app.css").Body.String()
	for _, want := range []string{"body.wb .table-wrap { overflow-x: auto;", "body.wb fieldset { min-width: 0; }", "box-sizing: border-box", "@media (max-width: 650px)"} {
		if !strings.Contains(css, want) {
			t.Errorf("the stylesheet has no %q", want)
		}
	}
}

// IC-UI44 — **편집 표의 모든 칸에 칸 이름(data-label)이 있고, 표의 역할이 마크업에 적혀 있다.** 좁은
// 화면에서 행을 카드로 접으면 머리글이 보이지 않으므로 칸마다 이름을 단다. 접기 위해 display를
// 바꾸면 브라우저가 표의 의미를 지우므로 역할(row · cell)을 직접 적는다. 「행 추가」가 돌려주는
// 조각도 같다.
func TestEditTablesLabelEveryCell(t *testing.T) {
	s := observedServer(t)
	pages := map[string]string{}
	for _, sc := range []string{ui.ScreenDecl, ui.ScreenScope} {
		for _, lang := range []string{"ko", "en"} {
			pages[sc+" "+lang] = get(t, s, sc+"?lang="+lang).Body.String()
		}
	}
	for name, path := range map[string]string{
		"asset row": ui.RowPath + "?kind=asset&node=0&i=5&lang=en",
		"edge row":  ui.RowPath + "?kind=edge&node=0&i=5&lang=en",
		"rule row":  ui.ScopeRowPath + "?layer=0&i=5&lang=en",
	} {
		pages[name] = "<table class=\"editable\">" + get(t, s, path).Body.String() + "</table>"
	}
	for name, body := range pages {
		tables := editTable.FindAllString(body, -1)
		if len(tables) == 0 {
			t.Errorf("%s: no edit table", name)
		}
		for _, tb := range tables {
			for _, td := range cell.FindAllString(tb, -1) {
				if !strings.Contains(td, `data-label=`) || !strings.Contains(td, `role="cell"`) {
					t.Errorf("%s: a cell has no label or role: %s", name, td)
				}
			}
			for _, tr := range row.FindAllString(tb, -1) {
				if !strings.Contains(tr, `role="row"`) {
					t.Errorf("%s: a row has no role: %s", name, tr)
				}
			}
		}
	}
}
