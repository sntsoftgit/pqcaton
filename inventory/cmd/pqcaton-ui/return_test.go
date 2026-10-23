// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// landed: 303이 보내는 주소의 경로.
func landed(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	location(t, w) // 303인지 본다
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return u.Path
}

// IC-U103 — **저장 · 확정 뒤에는 그 화면으로 돌아온다.** 선언 · 스코프 · 판정의 양식은 CSRF와 rev만 싣고
// 돌아갈 주소(return_to)는 싣지 않는다. 손으로 실어 보낸 return_to(바깥 주소 · 옛 주소 · 다른 화면)는 무시하고,
// 확인 페이지와 거절 페이지의 「돌아가기」도 그 화면을 가리킨다.
func TestSavingReturnsToTheScreen(t *testing.T) {
	s := everyScreen(t)
	for _, c := range []struct{ save, screen string }{
		{"/decl/save", ui.ScreenDecl},
		{"/scope/save", ui.ScreenScope},
		{"/save", ui.ScreenReview},
	} {
		form := hiddenFields(get(t, s, c.screen).Body.String())
		if form.Get(ui.CSRFField) == "" || form.Get(ui.RevField) == "" {
			t.Errorf("%s: the form does not carry the CSRF token and rev: %v", c.screen, form)
		}
		if _, ok := form["return_to"]; ok {
			t.Errorf("%s: the form still carries return_to", c.screen)
		}
		form.Del(ui.CSRFField) // 화면이 받은 쿠키가 아니라 테스트 쿠키의 토큰을 싣는다
		if got := landed(t, postForm(t, s, c.save, form)); got != c.screen {
			t.Errorf("save from %s landed on %s", c.screen, got)
		}
	}

	r, _ := newServer(t)
	for _, bad := range []string{"https://evil.example/review", "//evil.example/review", "/classic/review", "/decl"} {
		w := postForm(t, r, "/save", url.Values{"reviewer": {"보안팀"}, "return_to": {bad}})
		if got := landed(t, w); got != ui.ScreenReview {
			t.Errorf("return_to %q landed on %s", bad, got)
		}
	}
	form := signed()
	form.Set("return_to", "/classic/review")
	body := postForm(t, r, "/finalize/confirm", form).Body.String()
	if !strings.Contains(body, `href="/review"`) || strings.Contains(body, `href="/classic`) {
		t.Error("the confirmation page's back link is not the review screen")
	}
	if got := landed(t, postForm(t, r, "/finalize", hiddenFields(body))); got != ui.ScreenReview {
		t.Errorf("finalize landed on %s", got)
	}
	if w := postForm(t, r, "/finalize/confirm", withRev(form, "rev/1 stale")); !strings.Contains(w.Body.String(), `href="/review"`) {
		t.Errorf("the refusal's back link is not the review screen:\n%s", w.Body.String())
	}
}
