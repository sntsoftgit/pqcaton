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

// IC-U79 — **바꾸는 양식은 두 판 모두 CSRF · rev · return_to를 싣는다.** return_to는 그 양식을 그린
// 화면의 주소다. 옛 판이면 `/classic/…`이다.
func TestFormsCarryReturnTo(t *testing.T) {
	s := everyScreen(t)
	for _, sc := range []string{ui.ScreenDecl, ui.ScreenScope, ui.ScreenReview} {
		for _, here := range []string{sc, ui.Classic(sc)} {
			f := hiddenFields(get(t, s, here).Body.String())
			if f.Get(ui.CSRFField) == "" || f.Get(ui.RevField) == "" {
				t.Errorf("%s: the form does not carry the CSRF token and rev: %v", here, f)
			}
			if got := f[ui.ReturnField]; len(got) == 0 || got[0] != here {
				t.Errorf("%s: return_to = %v, want %s", here, got, here)
			}
		}
	}
}

// IC-U80 — **저장하면 저장한 판으로 돌아온다.** 선언 · 스코프 · 판정 모두 같다. 확정은 확인 페이지가
// return_to를 다시 실어 확정 뒤에도 같은 판으로 오고, 확인 페이지와 거절 페이지의 「돌아가기」도
// 그 판을 가리킨다.
func TestSavingReturnsToTheSameVersion(t *testing.T) {
	s := everyScreen(t)
	for _, c := range []struct{ save, screen string }{
		{"/decl/save", ui.ScreenDecl},
		{"/scope/save", ui.ScreenScope},
		{"/save", ui.ScreenReview},
	} {
		for _, here := range []string{c.screen, ui.Classic(c.screen)} {
			form := hiddenFields(get(t, s, here).Body.String())
			form.Del(ui.CSRFField) // 화면이 받은 쿠키가 아니라 테스트 쿠키의 토큰을 싣는다
			if got := landed(t, postForm(t, s, c.save, form)); got != here {
				t.Errorf("save from %s landed on %s", here, got)
			}
		}
	}

	r, _ := newServer(t)
	form := signed()
	form.Set(ui.ReturnField, ui.Classic(ui.ScreenReview))
	page := postForm(t, r, "/finalize/confirm", form)
	body := page.Body.String()
	if hiddenFields(body).Get(ui.ReturnField) != ui.Classic(ui.ScreenReview) {
		t.Fatal("the confirmation page does not carry return_to")
	}
	if !strings.Contains(body, `href="/classic/review"`) {
		t.Error("the confirmation page's back link leaves the old screen")
	}
	if got := landed(t, postForm(t, r, "/finalize", hiddenFields(body))); got != ui.Classic(ui.ScreenReview) {
		t.Errorf("finalize from the old screen landed on %s", got)
	}

	stale := withRev(form, "rev/1 stale")
	if w := postForm(t, r, "/finalize/confirm", stale); !strings.Contains(w.Body.String(), `href="/classic/review"`) {
		t.Errorf("the refusal's back link leaves the old screen:\n%s", w.Body.String())
	}
}

// IC-U81 — **return_to는 그 화면의 두 판 주소만 받는다.** 바깥 주소 · 스킴 없는 바깥 주소 · 질의나
// fragment가 붙은 주소 · 다른 화면의 주소는 버리고 그 화면의 새 판으로 보낸다. 숨은 필드는 사람이
// 고칠 수 있어서, 받은 값을 그대로 따르면 열린 리다이렉트가 된다.
func TestReturnToIsAllowListed(t *testing.T) {
	s, _ := newServer(t)
	for _, bad := range []string{
		"https://evil.example/review", "//evil.example/review", "/classic/review?x=1",
		"/classic/review#top", "/classic/decl", "/decl", "/classic/review/../decl", "/CLASSIC/review", "",
	} {
		form := url.Values{"reviewer": {"보안팀"}, ui.ReturnField: {bad}}
		w := postForm(t, s, "/save", form)
		if got := landed(t, w); got != ui.ScreenReview {
			t.Errorf("return_to %q landed on %s", bad, got)
		}
		if loc := w.Header().Get("Location"); strings.Contains(loc, "evil") || strings.Contains(loc, "#") {
			t.Errorf("return_to %q leaked into Location %q", bad, loc)
		}
	}
}
