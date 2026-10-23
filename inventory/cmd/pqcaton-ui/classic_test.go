// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// everyScreen: 화면 다섯이 모두 열리는 서버.
func everyScreen(t *testing.T) *server {
	t.Helper()
	s, dir := fullServer(t)
	s.results = filepath.Join(dir, "results")
	if err := os.MkdirAll(s.results, 0o700); err != nil {
		t.Fatal(err)
	}
	return s
}

// IC-U74 — **기본 주소 다섯이 새 화면이다.** 새 껍데기(`body.wb`)로 그리고, 아래쪽에 옛 판으로
// 가는 보조 링크 하나를 둔다. 옛 이동 링크(`<nav>`)는 없다.
func TestDefaultAddressesAreTheNewScreens(t *testing.T) {
	s := everyScreen(t)
	for _, sc := range ui.AllScreens() {
		w := get(t, s, sc+"?lang=ko")
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d\n%s", sc, w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `<body class="wb">`) {
			t.Errorf("%s: not the new shell", sc)
		}
		if strings.Contains(body, "<nav>") {
			t.Errorf("%s: the old navigation is on the new screen", sc)
		}
		if !strings.Contains(body, `href="`+ui.Classic(sc)+`">옛 화면(v0.23.0에서 제거)</a>`) {
			t.Errorf("%s: no secondary link to the old screen", sc)
		}
	}
}

// IC-U75 — **옛 화면은 `/classic` 아래에서 한 판 더 열린다.** 다섯 모두 200이고, 맨 위에 없어질
// 날을 알리는 안내와 같은 화면의 새 주소 링크가 있다. 안내는 고정 위치로 띄우지 않는다.
func TestClassicScreensCarryTheNotice(t *testing.T) {
	s := everyScreen(t)
	for _, sc := range ui.AllScreens() {
		for _, c := range []struct{ lang, notice string }{
			{"ko", "이 화면은 v0.23.0에서 없어집니다. 같은 작업은 새 화면에서 할 수 있습니다."},
			{"en", "This screen will be removed in v0.23.0. You can do the same work on the new screen."},
		} {
			w := get(t, s, ui.Classic(sc)+"?lang="+c.lang)
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s = %d", ui.Classic(sc), w.Code)
			}
			body := w.Body.String()
			if !strings.Contains(body, c.notice) {
				t.Errorf("%s (%s): no notice", ui.Classic(sc), c.lang)
			}
			if !strings.Contains(body, `<p class="classic-banner">`) || !strings.Contains(body, `href="`+sc+`"`) {
				t.Errorf("%s (%s): the notice does not link to %s", ui.Classic(sc), c.lang, sc)
			}
			if strings.Contains(body, `<body class="wb">`) {
				t.Errorf("%s: the old screen is drawn with the new shell", ui.Classic(sc))
			}
		}
	}
	css := get(t, s, ui.StaticPath+"app.css").Body.String()
	i := strings.Index(css, ".classic-banner")
	if i < 0 {
		t.Fatal("no style for the notice")
	}
	rule := css[i : i+strings.Index(css[i:], "}")]
	if strings.Contains(rule, "fixed") {
		t.Error("the notice is fixed in place and can cover the page when zoomed")
	}
}

// IC-U76 — **v0.21의 `-next` 주소는 301로 넘기고 질의 문자열을 그대로 붙인다.** `?lang=en`으로
// 저장한 북마크가 말을 잃지 않는다. 같은 주소를 거듭 받아도 질의가 쌓이지 않는다.
func TestNextAddressesMovePermanently(t *testing.T) {
	s := everyScreen(t)
	for _, sc := range ui.AllScreens() {
		for _, q := range []string{"", "?lang=en", "?lang=en&q=libssl"} {
			for range 2 {
				w := get(t, s, sc+"-next"+q)
				if w.Code != http.StatusMovedPermanently {
					t.Fatalf("GET %s-next%s = %d, want 301", sc, q, w.Code)
				}
				if got := w.Header().Get("Location"); got != sc+q {
					t.Errorf("GET %s-next%s → %q, want %q", sc, q, got, sc+q)
				}
			}
		}
	}
}

// IC-U77 — **확인 · 거절 · 복구 페이지도 새 껍데기다.** 복구 503은 입력을 읽지 않으므로 단계
// 카드도 그리지 않는다.
func TestConfirmRefusalAndRecoveryUseTheNewShell(t *testing.T) {
	s, _ := newServer(t)
	if body := postForm(t, s, "/finalize/confirm", signed()).Body.String(); !strings.Contains(body, `<body class="wb">`) {
		t.Error("the confirmation page is not the new shell")
	}
	stale := postForm(t, s, "/finalize/confirm", withRev(signed(), "rev/1 stale"))
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `<body class="wb">`) {
		t.Errorf("refusal: status %d, new shell = %v", stale.Code, strings.Contains(stale.Body.String(), `<body class="wb">`))
	}

	s.recovering = true
	w := get(t, s, ui.ScreenReview)
	body := w.Body.String()
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(body, `<body class="wb">`) {
		t.Fatalf("recovery: status %d\n%s", w.Code, body)
	}
	if strings.Contains(body, `class="steps"`) {
		t.Error("the recovery page draws step cards, which needs the inputs")
	}
}

// IC-U78 — **자기 화면을 다시 받는 조각은 자기 판의 주소를 쓴다.** 선언의 「다시 불러오기」와 조회의
// 검색 · 정렬은 옛 판에서는 `/classic/…`을, 새 화면에서는 기본 주소를 가리킨다. 엇갈리면 옛 화면
// 안에 새 양식이 끼어들거나 검색 한 번에 판이 바뀐다.
func TestSelfLinksStayInTheirVersion(t *testing.T) {
	s := everyScreen(t)
	for _, c := range []struct{ screen, want string }{
		{ui.ScreenDecl, `hx-get="/decl?lang=ko"`},
		{ui.Classic(ui.ScreenDecl), `hx-get="/classic/decl?lang=ko"`},
		{ui.ScreenInventory, `action="/inventory"`},
		{ui.Classic(ui.ScreenInventory), `action="/classic/inventory"`},
	} {
		if body := get(t, s, c.screen+"?lang=ko").Body.String(); !strings.Contains(body, c.want) {
			t.Errorf("%s: no %s", c.screen, c.want)
		}
	}
}
