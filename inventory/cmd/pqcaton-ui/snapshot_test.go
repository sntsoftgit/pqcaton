// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-U72 — **화면에 그린 내용과 양식의 rev는 같은 시점이다.** 화면 모델을 만든 직후에 다른 탭이나 손
// 편집이 세션을 바꾸면, 옛 모델에 새 rev를 짝지어 내보내지 않고 다시 만든다. 확인 페이지도 같아서, 미리
// 보는 사이에 바뀌면 폼의 rev와 맞지 않아 409다(토큰을 내주지 않는다).
func TestScreenAndRevAreTheSameMoment(t *testing.T) {
	s, _ := newServer(t)
	changed := false
	s.afterModel = func() {
		if changed {
			return
		}
		changed = true
		sf, err := review.Load(s.path)
		if err != nil {
			t.Fatal(err)
		}
		sf.Reviewer = "다른 탭의 판정자"
		if err := review.Save(s.path, sf); err != nil {
			t.Fatal(err)
		}
	}
	w := get(t, s, ui.ScreenReview)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /review = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "다른 탭의 판정자") {
		t.Error("the screen shows the model from before the change")
	}
	want := `name="` + ui.RevField + `" value="` + s.revFor(ui.ScreenReview) + `"`
	if !strings.Contains(body, want) {
		t.Error("the form does not carry the rev of what the screen shows")
	}

	// 확인 페이지: 폼은 바뀌기 전 rev를 들고 왔는데, 미리 보는 사이에 바뀐다.
	old := s.revFor("/finalize/confirm")
	changed = false
	s.afterModel = func() {
		if changed {
			return
		}
		changed = true
		sf, _ := review.Load(s.path)
		sf.Reviewer = "또 다른 탭"
		review.Save(s.path, sf)
	}
	c := postForm(t, s, "/finalize/confirm", withRev(signed(), old))
	if c.Code != http.StatusConflict || strings.Contains(c.Body.String(), ui.ConfirmTokenField) {
		t.Fatalf("confirm while the session changed: status %d", c.Code)
	}
}
