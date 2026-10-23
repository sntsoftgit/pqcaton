// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// changePaths: 상태를 바꾸는 여섯 주소. 보호 범위의 기준은 이 목록이 아니라 라우터에
// 등록된 POST 전부다. 이 목록은 실제 파일이 바뀌지 않는지 보는 데 쓴다.
var changePaths = []string{"/decl/save", "/scope/rules", "/scope/save", "/scope/finalize", "/save", "/finalize"}

// fullServer: 여섯 주소가 모두 처리기까지 갈 수 있게 입력을 다 갖춘 서버.
func fullServer(t *testing.T) (*server, string) {
	t.Helper()
	s, dir := withLayers(t)
	s.decl = filepath.Join(dir, "declaration.json")
	if err := os.WriteFile(s.decl, []byte(declJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	s.judgments = filepath.Join(dir, "judgments.jsonl")
	return s, dir
}

// guarded: 라우터와 같은 순서의 울타리 뒤에 가짜 처리기를 둔다. 처리기가 몇 번 불렸는지 센다.
func guarded(calls *int) http.Handler {
	r := chi.NewRouter()
	r.Use(securityHeaders, middleware.Recoverer, hostGuard, csrfCookie, postGuard, rememberLang)
	h := func(w http.ResponseWriter, _ *http.Request) { *calls++; w.WriteHeader(http.StatusNoContent) }
	r.Get("/x", h)
	r.Post("/x", h)
	return r
}

// goodPost: 화면이 보내는 그대로의 POST. 고칠 자리만 바꿔 쓴다.
func goodPost(target string) *http.Request {
	form := url.Values{ui.CSRFField: {testToken}}
	req := newReq(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://"+testHost)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: testToken})
	return req
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// IC-U47 — **루프백 이름으로 온 읽기 요청은 지나간다.**
//
// 포트는 보지 않는다. SSH로 다른 포트에 넘겨 들어오면 Host의 포트가 바뀌는데, 그때 막히면
// 원격에서 쓰라고 알려 준 방법이 통하지 않는다.
func TestLoopbackHostsPass(t *testing.T) {
	s, _ := withDecl(t)
	h := s.handler()
	for _, host := range []string{"127.0.0.1:8765", "localhost:9000", "LOCALHOST:8765", "[::1]:8765", "127.0.0.2:8765", "127.0.0.1"} {
		req := newReq(http.MethodGet, "/review", nil)
		req.Host = host
		if w := serve(h, req); w.Code != http.StatusOK {
			t.Errorf("Host %q: GET /review = %d\n%s", host, w.Code, w.Body.String())
		}
	}
}

// IC-U48 — **다른 이름으로 온 요청은 읽기든 쓰기든 막는다.**
//
// DNS rebinding이 여기서 걸린다. 공격자의 이름이 127.0.0.1을 가리키면 그 페이지는 같은 출처로
// 이 화면을 읽을 수 있다. 그래서 POST만 막으면 리뷰 큐가 그대로 읽힌다. 막힌 요청에는 쿠키도
// 내주지 않는다.
func TestForeignHostsAreRejected(t *testing.T) {
	s, _ := fullServer(t)
	h := s.handler()
	for _, host := range []string{"evil.example:8765", "evil.example", "192.168.1.5:8765", "", "127.0.0.1.evil.example:8765", "localhost.evil.example"} {
		get := newReq(http.MethodGet, "/review?lang=ko", nil)
		get.Host = host
		w := serve(h, get)
		if w.Code != http.StatusForbidden {
			t.Errorf("Host %q: GET = %d, want 403", host, w.Code)
		}
		if c := w.Result().Cookies(); len(c) != 0 {
			t.Errorf("Host %q: 막힌 요청에 쿠키를 냈다: %v", host, c)
		}
		post := goodPost("/save")
		post.Host = host
		post.Header.Set("Origin", "http://"+host)
		if w := serve(h, post); w.Code != http.StatusForbidden {
			t.Errorf("Host %q: POST = %d, want 403", host, w.Code)
		}
	}
}

// IC-U49 — **같은 출처에서 같은 토큰을 들고 오면 여섯 주소 모두 처리기에 닿는다.**
//
// 막는 것만 재면 전부 막아도 통과한다. 화면이 실제로 보내는 요청이 지나가는지 함께 본다.
func TestSameOriginWithTokenReachesHandlers(t *testing.T) {
	for _, p := range changePaths {
		s, _ := fullServer(t)
		w := serve(s.handler(), goodPost(p))
		// 가드의 거부는 본문이 「forbidden」으로 시작한다. 확정 주소는 처리기 자신이 확인 토큰이 없다고
		// 403을 낼 수 있다(confirm.go). 그것은 처리기에 닿았다는 뜻이다.
		if strings.HasPrefix(w.Body.String(), "forbidden") {
			t.Errorf("%s: 바른 요청을 막았다 (%d)\n%s", p, w.Code, w.Body.String())
		}
	}
}

// IC-U50 — **다른 출처, 출처 없음, 모양이 틀린 출처는 막는다.**
//
// 같은 기계의 다른 포트도 다른 출처다. 쿠키와 SameSite는 포트를 가르지 않으므로 그 몫을
// Origin이 한다.
func TestForeignOriginsAreRejected(t *testing.T) {
	for name, set := range map[string]func(*http.Request){
		"다른 이름":  func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") },
		"다른 포트":  func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:9999") },
		"https":  func(r *http.Request) { r.Header.Set("Origin", "https://"+testHost) },
		"없음":     func(r *http.Request) { r.Header.Del("Origin") },
		"null":   func(r *http.Request) { r.Header.Set("Origin", "null") },
		"빈 값":    func(r *http.Request) { r.Header.Set("Origin", "") },
		"둘":      func(r *http.Request) { r.Header.Add("Origin", "http://"+testHost) },
		"경로가 붙음": func(r *http.Request) { r.Header.Set("Origin", "http://"+testHost+"/x") },
		"사용자 정보": func(r *http.Request) { r.Header.Set("Origin", "http://u@"+testHost) },
		"질의가 붙음": func(r *http.Request) { r.Header.Set("Origin", "http://"+testHost+"?a=1") },
	} {
		calls := 0
		req := goodPost("/x")
		set(req)
		w := serve(guarded(&calls), req)
		if w.Code != http.StatusForbidden || calls != 0 {
			t.Errorf("%s: 코드 %d, 처리기 %d번 — 막고 처리기에 닿지 않아야 한다", name, w.Code, calls)
		}
	}

	// 대소문자만 다른 것은 같은 출처다. 호스트 이름은 대소문자를 가리지 않고, 브라우저는
	// Origin을 소문자로 보낸다. 막으면 LOCALHOST로 연 화면에서 저장이 늘 막힌다.
	for _, c := range []struct{ host, origin string }{
		{"LOCALHOST:8765", "http://localhost:8765"},
		{"localhost:8765", "http://LocalHost:8765"},
	} {
		calls := 0
		req := goodPost("/x")
		req.Host = c.host
		req.Header.Set("Origin", c.origin)
		if w := serve(guarded(&calls), req); w.Code == http.StatusForbidden || calls != 1 {
			t.Errorf("Host %q, Origin %q: 코드 %d, 처리기 %d번 — 같은 출처인데 막았다", c.host, c.origin, w.Code, calls)
		}
	}
}

// IC-U51 — **토큰이 없거나 다르면 막는다.**
//
// 쿠키와 숨은 필드가 둘 다 있고 같아야 한다. 비어 있는 값도 없는 것으로 본다. 빈 값끼리
// 같다고 지나가면 토큰을 채우지 않은 실수가 그대로 통과한다.
func TestMissingOrWrongTokensAreRejected(t *testing.T) {
	other := strings.Repeat("A", 43)
	for name, build := range map[string]func() *http.Request{
		"쿠키 없음": func() *http.Request {
			r := goodPost("/x")
			r.Header.Del("Cookie")
			return r
		},
		"쿠키 모양이 틀림": func() *http.Request {
			r := goodPost("/x")
			r.Header.Del("Cookie")
			r.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "short"})
			return withField(r, "short")
		},
		"필드 없음":   func() *http.Request { return withForm(goodPost("/x"), url.Values{}) },
		"필드가 빔":   func() *http.Request { return withField(goodPost("/x"), "") },
		"필드가 다름":  func() *http.Request { return withField(goodPost("/x"), other) },
		"한 글자 바뀜": func() *http.Request { return withField(goodPost("/x"), testToken[:42]+"B") },
		"필드가 둘": func() *http.Request {
			return withForm(goodPost("/x"), url.Values{ui.CSRFField: {testToken, testToken}})
		},
	} {
		calls := 0
		w := serve(guarded(&calls), build())
		if w.Code != http.StatusForbidden || calls != 0 {
			t.Errorf("%s: 코드 %d, 처리기 %d번 — 막고 처리기에 닿지 않아야 한다", name, w.Code, calls)
		}
	}
}

func withField(r *http.Request, v string) *http.Request {
	return withForm(r, url.Values{ui.CSRFField: {v}})
}

func withForm(r *http.Request, form url.Values) *http.Request {
	nr := newReq(r.Method, r.URL.String(), strings.NewReader(form.Encode()))
	nr.Header = r.Header.Clone()
	return nr
}

// IC-U52 — **라우터에 등록된 POST는 모두 보호를 받는다.**
//
// 보호 목록을 사람이 따로 적으면, 새 주소를 더한 날 그 목록에 넣는 것을 잊는다. 그래서
// 라우터를 훑어 POST 주소를 모두 모으고, 토큰 없이 보내 모두 막히는지 본다. 경로에 매개변수가
// 생기면 시험값으로 바꿔 보낸다.
func TestEveryPostRouteIsGuarded(t *testing.T) {
	s, _ := fullServer(t)
	h := s.handler()
	routes, ok := h.(chi.Routes)
	if !ok {
		t.Fatalf("handler가 chi 라우터가 아니다: %T", h)
	}
	param := regexp.MustCompile(`\{[^}]*\}`)
	var posts []string
	if err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodPost {
			posts = append(posts, param.ReplaceAllString(route, "x"))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(posts) < len(changePaths) {
		t.Fatalf("POST 주소를 %d개만 찾았다: %v", len(posts), posts)
	}
	for _, p := range posts {
		req := withForm(goodPost(p), url.Values{})
		if w := serve(h, req); w.Code != http.StatusForbidden {
			t.Errorf("%s: 토큰 없는 POST = %d, want 403", p, w.Code)
		}
	}
}

// IC-U53 — **막힌 요청은 아무것도 바꾸지 않는다.**
//
// 403을 냈는데 처리기가 먼저 돌았다면 막은 것이 아니다. 두 층으로 본다. 가짜 처리기는 어떤
// 이유로 막혀도 한 번도 불리지 않아야 하고, 실제 여섯 주소는 막힌 뒤 파일이 한 바이트도
// 바뀌지 않아야 한다.
func TestRejectedRequestsChangeNothing(t *testing.T) {
	for name, build := range map[string]func() *http.Request{
		"Host": func() *http.Request { r := goodPost("/x"); r.Host = "evil.example"; return r },
		"Origin": func() *http.Request {
			r := goodPost("/x")
			r.Header.Set("Origin", "http://evil.example")
			return r
		},
		"쿠키": func() *http.Request { r := goodPost("/x"); r.Header.Del("Cookie"); return r },
		"토큰": func() *http.Request { return withField(goodPost("/x"), strings.Repeat("A", 43)) },
	} {
		calls := 0
		w := serve(guarded(&calls), build())
		if w.Code != http.StatusForbidden || calls != 0 {
			t.Errorf("%s: 코드 %d, 처리기 %d번", name, w.Code, calls)
		}
	}

	for _, p := range changePaths {
		s, dir := fullServer(t)
		h := s.handler()
		// 막히기 전에 화면을 한 번 열어 세션 파일까지 갖춰 둔다.
		serve(h, newReq(http.MethodGet, "/scope", nil))
		before := snapshot(t, dir)
		form := url.Values{"org": {""}, "reviewer": {"x"}, "signature": {"x"}}
		w := serve(h, withForm(goodPost(p), form))
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: 토큰 없는 POST = %d", p, w.Code)
		}
		if after := snapshot(t, dir); !equalMaps(before, after) {
			t.Errorf("%s: 막혔는데 파일이 바뀌었다\n전 %v\n후 %v", p, before, after)
		}
	}
}

// snapshot: 디렉토리 아래 모든 파일의 해시.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		out[p] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// IC-U54 — **루프백이 아니면 열지 않는다.**
//
// 이 화면에는 인증이 없다. 예전에는 경고만 하고 열었는데, 경고는 지나쳐 읽힌다. 거부하면서
// 다른 기계에서 쓰는 길(SSH 포트 포워딩)을 함께 알린다.
func TestNonLoopbackBindIsRefused(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8765", "[::1]:8765", "localhost:8765", "127.0.0.2:1"} {
		if err := checkBind(addr); err != nil {
			t.Errorf("%s: 루프백인데 막았다: %v", addr, err)
		}
	}
	for _, addr := range []string{":8765", "0.0.0.0:8765", "[::]:8765", "192.168.1.5:8765", "example.com:8765", "8765"} {
		err := checkBind(addr)
		if err == nil {
			t.Errorf("%s: 루프백이 아닌데 열었다", addr)
			continue
		}
		if !strings.Contains(err.Error(), "ssh -L") {
			t.Errorf("%s: 다른 기계에서 쓰는 길을 알리지 않는다: %v", addr, err)
		}
	}
}

// IC-U55 — **보안 머리글은 모든 응답에 붙고, 저장 금지는 정적 파일에만 빠진다.**
//
// 막힌 응답에도 붙어야 한다. 그래서 머리글은 울타리보다 바깥에 둔다. 화면·조각·오류에는
// 리뷰 큐와 토큰이 담기므로 저장하지 않게 하고, CSS·JS는 담긴 것이 없으므로 그대로 둔다.
func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	s, _ := fullServer(t)
	h := s.handler()
	foreign := newReq(http.MethodGet, "/review", nil)
	foreign.Host = "evil.example"
	for _, c := range []struct {
		name    string
		req     *http.Request
		noStore bool
	}{
		{"화면", newReq(http.MethodGet, "/review", nil), true},
		{"옛 주소", newReq(http.MethodGet, "/classic/review", nil), true},
		{"조각", newReq(http.MethodGet, "/decl/row?kind=node&i=1", nil), true},
		{"막힌 요청", foreign, true},
		{"막힌 POST", withForm(goodPost("/save"), url.Values{}), true},
		{"정적 CSS", newReq(http.MethodGet, ui.StaticPath+"app.css", nil), false},
		{"정적 JS", newReq(http.MethodGet, ui.StaticPath+"htmx.min.js", nil), false},
	} {
		w := serve(h, c.req)
		hd := w.Header()
		for k, want := range map[string]string{
			"Content-Security-Policy": "frame-ancestors 'none'",
			"X-Frame-Options":         "DENY",
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "no-referrer",
		} {
			if got := hd.Get(k); got != want {
				t.Errorf("%s (%d): %s = %q, want %q", c.name, w.Code, k, got, want)
			}
		}
		if got := hd.Get("Cache-Control") == "no-store"; got != c.noStore {
			t.Errorf("%s (%d): Cache-Control = %q, no-store 기대 %v", c.name, w.Code, hd.Get("Cache-Control"), c.noStore)
		}
	}
}
