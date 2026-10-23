// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func hasHangul(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// plainPage: 오류 페이지가 지켜야 할 것. 새 껍데기 · 알림 역할 · 경로와 플래그 이름 없음 · 영어에 한글 없음.
// visible: 화면에 보이는 글. 링크 주소(언어 전환은 지금 질의를 그대로 싣는다)와 언어 전환 단추의 이름은 뺀다.
func visible(body string) string {
	body = regexp.MustCompile(`href="[^"]*"`).ReplaceAllString(body, "")
	return strings.ReplaceAll(body, ">한국어<", "><")
}

func plainPage(t *testing.T, name, lang, dir string, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	body := visible(w.Body.String())
	if !strings.Contains(body, `<body class="wb">`) || !strings.Contains(body, `role="alert"`) {
		t.Errorf("%s (%s): not an error page in the new shell:\n%s", name, lang, body)
	}
	for _, leak := range []string{dir, "pass -", "-decl", "-results", "-layers", "no such file"} {
		if leak != "" && (strings.Contains(body, leak) || strings.Contains(w.Header().Get("Location"), leak)) {
			t.Errorf("%s (%s): the page shows %q", name, lang, leak)
		}
	}
	if lang == "en" && hasHangul(body) {
		t.Errorf("%s (en): Korean on the English page", name)
	}
	if lang == "ko" && !strings.Contains(body, want) {
		t.Errorf("%s (ko): no %q", name, want)
	}
}

// IC-U88 — **브라우저로 가는 오류는 모두 새 껍데기의 오류 페이지다.** 없는 주소 · 받지 않는 방식 · 잘못된 요청 ·
// 이 화면에 필요한 파일 없음 · 입력을 읽지 못함 · 요청 보호 · 처리 중 panic을 두 말로 잰다. 오류 원문과 경로,
// 플래그 이름은 화면에 없고, 원인을 화면 말로 옮길 수 없는 것은 참조 번호를 보인다.
func TestEveryErrorIsAPage(t *testing.T) {
	for _, lang := range []string{"ko", "en"} {
		s := everyScreen(t)
		dir := filepath.Dir(s.path)
		bare, bareDir := newServer(t) // 대조 · 선언 · 스코프가 없다
		broken, brokenDir := newServer(t)
		if err := os.WriteFile(broken.path, []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		foreign := newReq(http.MethodGet, "/review?lang="+lang, nil)
		foreign.Host = "evil.example"
		cases := []struct {
			name   string
			srv    *server
			dir    string
			req    *http.Request
			status int
			want   string
			ref    bool
		}{
			{"not found", s, dir, newReq(http.MethodGet, "/nope?lang="+lang, nil), 404, "이 주소에는 화면이 없습니다", false},
			{"method", s, dir, newReq(http.MethodGet, "/save?lang="+lang, nil), 405, "이 방식의 요청을 받지 않습니다", false},
			{"bad request", s, dir, newReq(http.MethodGet, ui.RowPath+"?kind=bogus&lang="+lang, nil), 400, "요청을 처리할 수 없습니다", false},
			{"input missing", bare, bareDir, newReq(http.MethodGet, "/survey?lang="+lang, nil), 404, "관측 결과", false},
			{"input broken", broken, brokenDir, newReq(http.MethodGet, "/review?lang="+lang, nil), 500, "파일을 읽지 못했습니다", true},
			{"forbidden", s, dir, foreign, 403, "이 요청을 받지 않았습니다", false},
		}
		for _, c := range cases {
			w := serve(c.srv.handler(), c.req)
			if w.Code != c.status {
				t.Errorf("%s (%s): status %d, want %d", c.name, lang, w.Code, c.status)
			}
			plainPage(t, c.name, lang, c.dir, w, c.want)
			if c.ref != (strings.Contains(w.Body.String(), "참조 번호") || strings.Contains(w.Body.String(), "Reference")) {
				t.Errorf("%s (%s): reference shown = %v, want %v", c.name, lang, !c.ref, c.ref)
			}
		}

		// 처리기가 터져도 서버는 그 요청에 내부 오류 페이지를 낸다.
		boom := recoverPanic(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom " + dir) }))
		w := httptest.NewRecorder()
		boom.ServeHTTP(w, newReq(http.MethodGet, "/x?lang="+lang, nil))
		if w.Code != 500 {
			t.Errorf("panic (%s): status %d", lang, w.Code)
		}
		plainPage(t, "panic", lang, dir, w, "처리하지 못했습니다")
	}
}

// IC-U89 — **주소의 알림은 코드만 받는다.** 질의에 실린 임의의 글 · 경로 · 스크립트는 화면에 보이지 않고 「알 수
// 없는 알림」이 된다. 확정 관문이 막은 까닭은 코드로 실려 가서 그 말의 문장으로 그려진다.
func TestNoticesCarryCodesOnly(t *testing.T) {
	s, _ := newServer(t)
	for _, q := range []string{
		"msg=" + url.QueryEscape("<script>alert(1)</script>"),
		"problem=" + url.QueryEscape("/etc/passwd"),
		"problem=" + url.QueryEscape("internal~/etc/passwd%0Aspoof"),
		"msg=" + url.QueryEscape("decl-saved~1~2~x"),
	} {
		body := visible(get(t, s, "/review?lang=ko&"+q).Body.String())
		for _, bad := range []string{"<script>alert", "/etc/passwd", "spoof"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s: the page shows %q", q, bad)
			}
		}
		if !strings.Contains(body, "알 수 없는 알림입니다") {
			t.Errorf("%s: not drawn as an unknown notice", q)
		}
	}

	for _, lang := range []string{"ko", "en"} {
		form := signed()
		form.Del("signature")
		w := postFormLang(t, s, "/finalize/confirm", lang, form)
		if p := location(t, w).Get("problem"); !strings.HasPrefix(p, ui.ProblemMandatory) && !strings.HasPrefix(p, ui.ProblemNoSignature) {
			t.Fatalf("%s: problem = %q, want a gate code", lang, p)
		}
		body := followed(t, s, w, lang)
		switch lang {
		case "ko":
			if !strings.Contains(body, "서명을 채우십시오") {
				t.Errorf("ko: the gate refusal is not drawn in Korean:\n%s", body)
			}
		case "en":
			if hasHangul(visible(body)) || !strings.Contains(body, "fill in the signature") {
				t.Errorf("en: the gate refusal is not drawn in English")
			}
		}
	}
}

// IC-U90 — **화면 서버는 평문 오류를 쓰지 않는다.** 시험이 아닌 소스에 `http.Error`가 없다.
func TestNoPlainErrors(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "http.Error(") {
			t.Errorf("%s uses http.Error; draw an error page instead (errorPage)", f)
		}
	}
}

// IC-U91 — **저장소가 막은 까닭은 문장이 아니라 값으로 가린다.** 센티널마다 거절 종류와 상태가 정해져 있고,
// 거절 페이지는 두 말 모두 경로를 싣지 않는다. 모르는 오류는 참조 번호를 붙인 내부 오류다.
func TestStoreErrorsMapByValue(t *testing.T) {
	dir := "/tmp/somewhere/secret"
	for _, c := range []struct {
		err    error
		kind   string
		status int
	}{
		{store.ErrUnsafeFile, ui.RefusedUnsafe, 503},
		{store.ErrStateMismatch, ui.RefusedMismatch, 503},
		{store.ErrDamaged, ui.RefusedDamaged, 503},
		{store.ErrInProgress, ui.RefusedBusy, 409},
		{store.ErrBusy, ui.RefusedBusy, 409},
		{store.ErrTargetBusy, ui.RefusedBusy, 409},
		{store.ErrStale, ui.RefusedStale, 409},
		{store.ErrNeedsRecovery, ui.RefusedRecovery, 503},
	} {
		err := fmt.Errorf("%w: %s/.pqcaton/x", c.err, dir)
		kind, status := refusedKind(err)
		if kind != c.kind || status != c.status {
			t.Errorf("%v: %s %d, want %s %d", c.err, kind, status, c.kind, c.status)
		}
		for _, lang := range []string{"ko", "en"} {
			s, _ := newServer(t)
			w := httptest.NewRecorder()
			s.fail(w, withForm(goodPost("/save?lang="+lang), url.Values{}), "/review", err)
			if w.Code != c.status {
				t.Errorf("%v (%s): status %d", c.err, lang, w.Code)
			}
			plainPage(t, c.kind, lang, dir, w, "")
		}
	}
	if ps := problemsFor(newReq(http.MethodGet, "/", nil), errors.New("open "+dir+": boom")); len(ps) != 1 || ps[0].Code != ui.ProblemInternal {
		t.Errorf("an unknown error became %v, want one internal problem with a reference", ps)
	}
}
