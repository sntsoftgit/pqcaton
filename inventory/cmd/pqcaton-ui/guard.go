// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 요청 보호. **이 화면에는 로그인이 없다.** 그래서 「누가 보냈나」 대신 「어디서 왔나」를 본다.
//
// 화면은 루프백에서만 열린다(checkBind). 그래도 브라우저는 다른 출처의 페이지가 시키는 대로
// 이 주소에 요청을 보낸다. 막는 길은 셋이고 서로 다른 것을 막는다.
//
//   - Host 검사(hostGuard): DNS rebinding을 막는다. 공격자의 이름이 127.0.0.1을 가리키게 되면 그
//     페이지는 같은 출처로 이 화면을 **읽는다.** 그래서 POST만이 아니라 모든 요청에 건다.
//   - Origin 검사(postGuard): 다른 출처의 폼 POST를 막는다. 같은 기계의 다른 포트도 여기서 걸린다.
//     쿠키와 SameSite는 포트를 가르지 않으므로 그 몫을 Origin이 한다.
//   - CSRF 토큰(csrfCookie · postGuard): 쿠키와 숨은 필드가 같아야 한다. Origin을 보내지
//     않거나 잘못 보내는 브라우저가 있어도 남는 둘째 울타리다.
//
// **거부는 처리기에 닿기 전에 끝난다.** 403을 냈는데 처리기가 먼저 돌아 파일이 바뀌었다면
// 막은 것이 아니다(IC-U53).

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// csrfCookie 이름. 쿠키는 포트를 가리지 않아 같은 기계의 다른 pqcaton-ui와 값을 나눠 쓰지만,
// 값이 같아도 Origin이 포트까지 보므로 다른 화면의 POST가 여기 닿지 않는다.
const csrfCookieName = "pqcaton_csrf"

// csrfBytes: 토큰의 난수 길이. base64url로 43자가 된다.
const csrfBytes = 32

type csrfKey struct{}

// checkBind: **루프백이 아니면 열지 않는다.**
//
// 예전에는 경고만 하고 열었다. 그런데 이 화면에는 인증이 없고 리뷰 큐는 그 조직의 공격면이다.
// 경고는 지나쳐 읽히고, 한 번 열린 화면은 누가 보는지 알 수 없다. 다른 기계에서 써야 하면
// SSH로 포트를 넘겨 이 기계의 루프백으로 들어온다. 그러면 인증은 SSH가 맡는다.
func checkBind(addr string) error {
	if loopback(addr) {
		return nil
	}
	return fmt.Errorf("refusing to listen on %s: the screen has no authentication, so it listens only on loopback (127.0.0.1, ::1, localhost)\n"+
		"   To use it from another machine, forward a port over SSH and open http://127.0.0.1:8765 there:\n"+
		"     ssh -L 8765:127.0.0.1:8765 <this-host>", addr)
}

// securityHeaders: 모든 응답에 붙는 머리글. **맨 바깥에 둔다.** 403과 panic 응답에도 붙어야
// 하기 때문이다.
//
// 화면·조각·오류는 저장하지 않게 한다(no-store). 리뷰 큐와 토큰이 담겨 있다. 정적 파일(CSS·JS)은
// 담긴 것이 없으므로 두지 않는다.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		// 주소에 파일 이름과 거절 사유가 실린다. 다른 곳으로 따라 나가지 않게 한다.
		//
		// **no-referrer로 두면 저장이 막힌다.** 그 정책의 페이지가 보내는 양식 POST에 브라우저는
		// `Origin: null`을 싣고(Fetch 명세의 요청 출처 직렬화), postGuard는 그것을 거절한다. v0.20.0부터
		// v0.22.0까지 실제 브라우저에서 저장 · 확정이 모두 403이었다(v0.23.0 브라우저 검증에서 찾음).
		// same-origin은 이 화면 자신에게만 출처를 싣고 다른 곳에는 아무것도 보내지 않는다.
		h.Set("Referrer-Policy", "same-origin")
		if !strings.HasPrefix(r.URL.Path, ui.StaticPath) {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// hostGuard: 루프백 이름으로 온 요청만 받는다. **포트는 보지 않는다.** SSH로 다른 포트에 넘겨
// 들어오면 Host의 포트가 바뀐다. DNS rebinding은 이름에서 걸린다.
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.Host) {
			reject(w, r, "the Host header is not a loopback name")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allowedHost: Host 머리글의 이름이 루프백인가. IPv6는 대괄호를 벗겨 본다.
func allowedHost(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	name = strings.TrimSuffix(strings.TrimPrefix(name, "["), "]")
	if strings.EqualFold(name, "localhost") {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

// csrfCookie: 브라우저마다 토큰 하나를 둔다. **서버가 아니라 브라우저가 들고 있다.** 서버를
// 다시 띄워도 열어 둔 양식을 그대로 저장할 수 있고, 비밀을 디스크에 두지 않는다.
//
// 읽기 요청에서만 새로 낸다. 바꾸는 요청에서 내 주면, 토큰 없이 온 요청에 토큰을 쥐여 주는 셈이다.
func csrfCookie(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := cookieToken(r)
		if tok == "" && safeMethod(r.Method) {
			tok = newToken()
			http.SetCookie(w, &http.Cookie{
				Name: csrfCookieName, Value: tok, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode,
			})
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), csrfKey{}, tok)))
	})
}

// postGuard: 바꾸는 요청은 같은 출처에서, 같은 토큰을 들고 와야 한다.
func postGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		if why := originProblem(r); why != "" {
			reject(w, r, why)
			return
		}
		if why := tokenProblem(r); why != "" {
			reject(w, r, why)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originProblem: Origin이 이 화면 자신인가. 문자열을 이어 붙여 견주지 않고 URL로 읽는다.
func originProblem(r *http.Request) string {
	vals := r.Header.Values("Origin")
	switch {
	case len(vals) == 0:
		return "the request has no Origin header"
	case len(vals) > 1:
		return "the request has more than one Origin header"
	}
	o := vals[0]
	if o == "" || o == "null" {
		return fmt.Sprintf("the request has no usable Origin (%q)", o)
	}
	u, err := url.Parse(o)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Opaque != "" ||
		u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Host == "" {
		return fmt.Sprintf("the Origin is not a plain http origin (%q)", o)
	}
	// 호스트 이름은 대소문자를 가리지 않는다. 브라우저는 Origin을 소문자로 보내므로
	// LOCALHOST로 연 화면의 저장이 막히지 않게 한다. 포트는 글자 그대로 같아야 한다.
	if !strings.EqualFold(u.Host, r.Host) {
		return fmt.Sprintf("the Origin %q is not this screen", o)
	}
	return ""
}

// tokenProblem: 쿠키와 숨은 필드가 둘 다 있고 같은가. 비교는 상수 시간이다.
func tokenProblem(r *http.Request) string {
	c := cookieToken(r)
	if c == "" {
		return "the request has no CSRF cookie"
	}
	if err := r.ParseForm(); err != nil {
		return "the form could not be read"
	}
	vals := r.PostForm[ui.CSRFField]
	switch {
	case len(vals) == 0 || vals[0] == "":
		return "the form has no CSRF token"
	case len(vals) > 1:
		return "the form has more than one CSRF token"
	}
	if subtle.ConstantTimeCompare([]byte(c), []byte(vals[0])) != 1 {
		return "the CSRF token does not match"
	}
	return ""
}

// cookieToken: 쿠키에 든 토큰. 모양이 맞지 않으면 없는 것으로 본다.
func cookieToken(r *http.Request) string {
	c, err := r.Cookie(csrfCookieName)
	if err != nil || !wellFormed(c.Value) {
		return ""
	}
	return c.Value
}

func wellFormed(tok string) bool {
	b, err := base64.RawURLEncoding.DecodeString(tok)
	return err == nil && len(b) == csrfBytes
}

func newToken() string {
	b := make([]byte, csrfBytes)
	if _, err := rand.Read(b); err != nil {
		// 난수를 못 얻으면 토큰을 낼 수 없다. 약한 값으로 대신하지 않는다.
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// csrfToken: 이 요청에 그릴 토큰. 화면은 이 값을 숨은 필드에 넣는다.
func csrfToken(r *http.Request) string {
	tok, _ := r.Context().Value(csrfKey{}).(string)
	return tok
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// reject: 403을 낸다. **본문은 영어 한 줄이다**(명령의 출력과 같은 규칙). 같은 줄을 표준오류에도 남겨
// 왜 막혔는지 화면 밖에서도 보이게 한다.
func reject(w http.ResponseWriter, r *http.Request, why string) {
	fmt.Fprintf(os.Stderr, "rejected %s %s: %s\n", r.Method, r.URL.Path, why)
	errorPage(w, r, http.StatusForbidden, ui.ErrForbidden, nil)
}
