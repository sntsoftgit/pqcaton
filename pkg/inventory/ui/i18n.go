// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Lang — 화면을 그릴 말.
//
// **화면만 두 말을 쓴다.** 명령의 출력과 로그는 영어 하나다 — 그쪽은 사람이 읽는
// 자리이기 전에 기록이고, 붙여 넣어 검색하고 이슈에 올리는 것이라 말이 다르면
// 같은 문제가 두 문장으로 남는다.
type Lang string

const (
	KO Lang = "ko"
	EN Lang = "en"
)

// LangParam · LangCookie — 사람이 고른 말이 오가고 남는 자리.
const (
	LangParam  = "lang"
	LangCookie = "pqcaton_lang"
)

// PickLang — 이 요청을 어느 말로 그릴까.
//
//	?lang= → 쿠키 → Accept-Language → 영어
//
// **고른 것이 브라우저 설정보다 우선한다.** 한국어 브라우저를 쓰면서 영어 화면을 보고
// 싶은 사람이 있고, 그 반대도 있다. 아무것도 고르지 않았을 때만 브라우저에 묻는다.
func PickLang(r *http.Request) Lang {
	if l, ok := parseLang(r.URL.Query().Get(LangParam)); ok {
		return l
	}
	if c, err := r.Cookie(LangCookie); err == nil {
		if l, ok := parseLang(c.Value); ok {
			return l
		}
	}
	if l, ok := acceptLang(r.Header.Get("Accept-Language")); ok {
		return l
	}
	return EN
}

// parseLang — 모르는 값은 받지 않는다. 주소와 쿠키는 밖에서 오는 값이다.
func parseLang(v string) (Lang, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case string(KO):
		return KO, true
	case string(EN):
		return EN, true
	}
	return "", false
}

// acceptLang — 브라우저가 말한 것 중 **먼저 오는 것**만 본다.
//
// q 값까지 재지 않는 것은, 여기서 고르는 것이 둘뿐이라 순서로 충분하기 때문이다.
// `ko-KR`처럼 지역이 붙어 와도 앞의 두 글자로 받는다.
func acceptLang(header string) (Lang, bool) {
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(part)
		if i := strings.Index(tag, ";"); i >= 0 {
			tag = tag[:i]
		}
		switch {
		case strings.HasPrefix(strings.ToLower(tag), "ko"):
			return KO, true
		case strings.HasPrefix(strings.ToLower(tag), "en"):
			return EN, true
		}
	}
	return "", false
}

// SetLang — 고른 말을 기억한다. 화면을 옮길 때마다 다시 고르게 하지 않는다.
//
// 쿠키에 담는 것은 언어 하나뿐이다 — 누가 무엇을 보는지가 아니다.
func SetLang(w http.ResponseWriter, l Lang) {
	http.SetCookie(w, &http.Cookie{
		Name: LangCookie, Value: string(l), Path: "/",
		MaxAge: 365 * 24 * 60 * 60, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// Other — 토글이 가리킬 반대쪽 말.
func (l Lang) Other() Lang {
	if l == KO {
		return EN
	}
	return KO
}

// Label — 토글에 적을 이름. **가려는 쪽의 말로 적는다** — 지금 말을 모르는 사람도
// 자기 말을 알아본다.
func (l Lang) Label() string {
	if l == KO {
		return "한국어"
	}
	return "English"
}

// SwitchHref — 지금 보고 있는 자리 그대로 **말만 바꾸는** 주소.
//
// 토글이 첫 화면으로 되돌리면, 표를 한참 내려온 사람은 말을 바꾸느니 그냥 참는다.
func SwitchHref(r *http.Request, to Lang) string {
	q := r.URL.Query()
	q.Set(LangParam, string(to))
	u := *r.URL
	u.RawQuery = q.Encode()
	// **호스트는 떼고 경로만 남긴다.** 밖에서 온 값으로 링크를 만들지 않는다.
	return u.RequestURI()
}

// T — 한 문구를 두 말로.
//
// **두 말을 한 자리에 나란히 둔다.** 파일을 갈라 두면 한쪽만 고쳐지는 날이 오고,
// 그날 어느 쪽이 최신인지 아무도 모른다. 이름 있는 변수라 빠뜨리면 컴파일이 막는다.
type T struct{ KO, EN string }

// In — 그 말로. 비어 있으면 영어로 떨어진다 — 빈 화면보다는 낫다.
func (t T) In(l Lang) string {
	if l == KO && t.KO != "" {
		return t.KO
	}
	return t.EN
}

// F — 그 말로, 값을 채워서. **수가 들어가는 문구는 이것으로 만든다.** 영어는 수가 1이면 뒤의 명사와 동사를
// 단수로 맞춘다(enCounts). 문구마다 단수형을 따로 두면 하나쯤은 빠뜨린다(「1 nodes」, v0.23.0에서 고침).
func (t T) F(l Lang, args ...any) string {
	s := fmt.Sprintf(t.In(l), args...)
	if l == EN || t.KO == "" {
		s = enCounts(s)
	}
	return s
}

// enCounts: 영어 문장에서 「1」 바로 뒤의 복수 명사를 단수로, 그 바로 뒤의 are · were · need를 is · was · needs로
// 맞춘다. 바로 뒤 낱말이 꾸밈말(observed · required 같은)이면 그다음 낱말을 명사로 본다. 「1 of them need」도
// 맞춘다. 숫자의 일부인 1(11 · 0.1 · 1,000)은 건드리지 않는다.
func enCounts(s string) string {
	s = oneOfThem.ReplaceAllString(s, "${1}1 of them needs")
	return oneWords.ReplaceAllStringFunc(s, func(m string) string {
		g := oneWords.FindStringSubmatch(m)
		w1, w2, rest := g[3], g[4], g[5]
		switch {
		case plural(w1):
			next := ""
			if w2 != "" {
				next = agree(" " + w2)
			}
			return g[1] + "1" + g[2] + " " + singular(w1) + next + rest
		case enModifiers[w1] && plural(w2):
			return g[1] + "1" + g[2] + " " + w1 + " " + singular(w2) + agree(rest)
		}
		return m
	})
}

func plural(w string) bool {
	return strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && len(w) > 2
}

func singular(w string) string {
	if strings.HasSuffix(w, "ies") {
		return strings.TrimSuffix(w, "ies") + "y"
	}
	return strings.TrimSuffix(w, "s")
}

// agree: 바로 뒤의 동사를 단수 주어에 맞춘다(앞에 공백 하나가 붙은 낱말).
func agree(w string) string {
	switch w {
	case " are":
		return " is"
	case " were":
		return " was"
	case " need":
		return " needs"
	}
	return w
}

// enModifiers: 수와 명사 사이에 오는 꾸밈말. 이 낱말 뒤의 낱말이 명사다.
var enModifiers = map[string]bool{"observed": true, "required": true, "rule": true, "policy": true,
	"auto-pass": true, "recorded": true, "negotiated": true}

var (
	oneWords  = regexp.MustCompile(`(^|[^\d.,])1(</b>)? ([a-z-]+)(?: ([a-z-]+))?( are| were| need)?\b`)
	oneOfThem = regexp.MustCompile(`(^|[^\d.,])1 of them need\b`)
)
