package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func req(target string, header, cookie string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if header != "" {
		r.Header.Set("Accept-Language", header)
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: ui.LangCookie, Value: cookie})
	}
	return r
}

// IC-UI21 — **고른 것이 브라우저 설정을 이긴다.**
//
// 한국어 브라우저를 쓰면서 영어 화면을 보려는 사람이 있고, 그 반대도 있습니다. 브라우저가
// 이기면 토글을 눌러도 다음 화면에서 되돌아갑니다 — 그러면 토글이 있으나 마나입니다.
func TestPickLangOrder(t *testing.T) {
	for _, tc := range []struct {
		name              string
		target, hdr, cook string
		want              ui.Lang
	}{
		{"주소가 가장 세다", "/?lang=en", "ko-KR", "ko", ui.EN},
		{"쿠키가 브라우저를 이긴다", "/", "ko-KR", "en", ui.EN},
		{"아무것도 안 골랐으면 브라우저", "/", "ko-KR,ko;q=0.9", "", ui.KO},
		{"브라우저도 말이 없으면 영어", "/", "", "", ui.EN},
		{"모르는 말은 받지 않는다", "/?lang=jp", "", "", ui.EN},
		{"쿠키가 깨졌어도 브라우저로 떨어진다", "/", "ko", "zz", ui.KO},
		{"지역이 붙어 와도 받는다", "/", "en-GB", "", ui.EN},
	} {
		if got := ui.PickLang(req(tc.target, tc.hdr, tc.cook)); got != tc.want {
			t.Errorf("%s: %q (want %q)", tc.name, got, tc.want)
		}
	}
}

// IC-UI22 — **말을 바꿔도 보던 자리에 남는다.**
//
// 토글이 첫 화면으로 되돌리면, 표를 한참 내려온 사람은 말을 바꾸느니 그냥 참습니다.
func TestSwitchHrefKeepsThePlace(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/scope?msg=saved&lang=ko", nil)
	got := ui.SwitchHref(r, ui.EN)
	for _, want := range []string{"/scope", "lang=en", "msg=saved"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q 에 %q 가 없다", got, want)
		}
	}
	if strings.Contains(got, "lang=ko") {
		t.Errorf("옛 말이 남았다: %q", got)
	}
}

// IC-UI23 — 토글에 적히는 이름은 **가려는 쪽의 말로** 적는다. 지금 말을 못 읽는 사람도
// 자기 말은 알아본다.
func TestToggleLabelIsInItsOwnScript(t *testing.T) {
	if ui.EN.Other() != ui.KO || ui.KO.Other() != ui.EN {
		t.Fatal("토글이 반대쪽을 가리키지 않는다")
	}
	if ui.KO.Label() != "한국어" || ui.EN.Label() != "English" {
		t.Errorf("이름표: %q · %q", ui.KO.Label(), ui.EN.Label())
	}
}
