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

// everyScreen: 화면 여섯이 모두 열리는 서버.
func everyScreen(t *testing.T) *server {
	t.Helper()
	s, dir := fullServer(t)
	s.results = filepath.Join(dir, "results")
	if err := os.MkdirAll(s.results, 0o700); err != nil {
		t.Fatal(err)
	}
	return s
}

// IC-U74 — **기본 주소 여섯이 화면이다.** 모두 화면 껍데기(`body.wb`)로 그리고, 브라우저 제목은 그 화면의
// 이름이다. 옛 이동 링크(`<nav>`)도 옛 화면으로 가는 링크도 없다.
func TestDefaultAddressesAreTheScreens(t *testing.T) {
	s := everyScreen(t)
	for _, sc := range ui.AllScreens() {
		w := get(t, s, sc+"?lang=ko")
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d\n%s", sc, w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `<body class="wb">`) {
			t.Errorf("%s: not the screen shell", sc)
		}
		if strings.Contains(body, "<nav>") || strings.Contains(body, "/classic") {
			t.Errorf("%s: the old navigation or a link to the old screens is still there", sc)
		}
		if !strings.Contains(body, "<title>"+titles[sc]+" — pqcaton</title>") {
			t.Errorf("%s: the title is not %q", sc, titles[sc])
		}
	}
}

// titles: 화면마다 브라우저 제목. 단계 카드의 이름과 같다.
var titles = map[string]string{
	ui.ScreenDecl: "선언", ui.ScreenScope: "암호 자산 스코프", ui.ScreenObserve: "관측", ui.ScreenSurvey: "대조",
	ui.ScreenReview: "판정(리뷰 큐)", ui.ScreenInventory: "인벤토리·판정 이력",
}

// IC-U76 — **옛 주소 열 개(v0.21의 `-next`, v0.22의 `/classic/…`)는 301로 지금 화면에 넘기고 질의 문자열을
// 그대로 붙인다.** `?lang=en`으로 저장한 북마크가 말을 잃지 않는다. 같은 주소를 거듭 받아도 질의가 쌓이지 않는다.
// 옛 화면 자체는 없다.
func TestOldAddressesMovePermanently(t *testing.T) {
	s := everyScreen(t)
	for _, sc := range ui.AllScreens() {
		if sc == ui.ScreenObserve {
			continue // v0.24.0에 생긴 화면이라 옛 주소가 없다
		}
		for _, from := range []string{sc + "-next", "/classic" + sc} {
			for _, q := range []string{"", "?lang=en", "?lang=en&q=libssl"} {
				for range 2 {
					w := get(t, s, from+q)
					if w.Code != http.StatusMovedPermanently {
						t.Fatalf("GET %s%s = %d, want 301", from, q, w.Code)
					}
					if got := w.Header().Get("Location"); got != sc+q {
						t.Errorf("GET %s%s → %q, want %q", from, q, got, sc+q)
					}
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

// IC-U78 — **자기 화면을 다시 받는 조각은 그 화면의 주소를 쓴다.** 선언의 「다시 불러오기」와 조회의 검색 ·
// 정렬이 기본 주소를 가리킨다.
func TestSelfLinksStayOnTheScreen(t *testing.T) {
	s := everyScreen(t)
	for _, c := range []struct{ screen, want string }{
		{ui.ScreenDecl, `hx-get="/decl?lang=ko"`},
		{ui.ScreenInventory, `action="/inventory"`},
	} {
		if body := get(t, s, c.screen+"?lang=ko").Body.String(); !strings.Contains(body, c.want) {
			t.Errorf("%s: no %s", c.screen, c.want)
		}
	}
}
