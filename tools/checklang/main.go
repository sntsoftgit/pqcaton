// Command checklang — **공개 사이트의 한국어와 영어가 짝을 이루는지 막는다.**
//
// site/ 의 두 장은 문구를 두 벌 적어 두고 CSS 로 한쪽을 숨긴다. 규칙은 두 줄뿐이다.
//
//	html[data-lang="ko"] [lang="en"] { display: none; }
//	html[data-lang="en"] [lang="ko"] { display: none; }
//
// **이 방식은 한쪽 말로 볼 때 멀쩡해 보이는 고장을 만든다.** 2026-08-27 에
// `<span lang="ko">` 를 닫지 않은 자리가 아홉 곳 있었다. 닫는 태그가 없으니 파서가
// 뒤따르는 `<span lang="en">` 을 **그 안으로** 넣었고, 영어로 열면 바깥 한국어가
// 숨겨질 때 안쪽 영어까지 함께 사라져 표 머리글 여섯과 표 셀 셋이 통째로 빈칸이
// 됐다. 한국어로 보면 아홉 자리가 다 멀쩡해서 아무도 몰랐다.
//
// 관문 다섯이 그것을 하나도 잡지 못했다. 사람이 사이트를 영어로 열어 본 날에야
// 드러났다. 그래서 **여는 것을 기계에 맡긴다.**
//
// 재는 것은 셋이다.
//
//  1. **한 말 안에 다른 말이 들어간 자리.** 이번 고장의 모양이다. 바깥이 숨겨질 때
//     안쪽도 함께 사라지므로 그 자리는 어느 말로도 읽을 수 없다.
//  2. **짝이 없는 자리.** 두 말 가운데 하나만 적힌 자리는 다른 말로 열었을 때 빈다.
//  3. **모르는 말.** `ko`·`en` 이 아닌 값은 CSS 두 줄 가운데 어느 것도 걸지 못해서,
//     양쪽 말에서 모두 보인다.
//
// **정규식이 아니라 파서로 본다.** 이번 고장이 정확히 그 자리였다 — 닫는 태그가
// 빠져도 글자는 그대로 남아 있어서, 문자열을 세는 검사는 아홉 자리를 모두 통과시킨다.
// 고장은 브라우저가 그 HTML 을 **어떻게 읽는가**에서 생겼으므로 같은 규칙으로 읽는
// 파서라야 보인다.
//
// usage: go run ./tools/checklang
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// langs — 이 사이트가 쓰는 말. 여기 없는 값은 3번 규칙이 막는다.
var langs = map[string]string{"ko": "en", "en": "ko"}

// kind — 무엇이 어긋났나. 출력에서 자리마다 이것을 앞세운다.
type kind string

const (
	nested   kind = "nested"   // 한 말 안에 다른 말이 들어갔다
	unpaired kind = "unpaired" // 짝이 없다
	unknown  kind = "unknown"  // 모르는 말이다
)

type finding struct {
	file string
	line int
	kind kind
	lang string
	text string
}

func main() {
	found, files, err := check(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ could not read the site pages:", err)
		os.Exit(1)
	}
	if len(found) > 0 {
		fmt.Fprintln(os.Stderr, "✗ the two languages do not pair up:")
		for _, f := range found {
			where := f.file
			if f.line > 0 {
				where = fmt.Sprintf("%s:%d", f.file, f.line)
			}
			fmt.Fprintf(os.Stderr, "    %-8s %s  [%s] %s\n", f.kind, where, f.lang, f.text)
		}
		fmt.Fprintln(os.Stderr, "\nEvery phrase is written twice, side by side under the same parent:")
		fmt.Fprintln(os.Stderr, `    <th><span lang="ko">…</span><span lang="en">…</span></th>`)
		fmt.Fprintln(os.Stderr, "A missing </span> nests one inside the other, and then the cell is")
		fmt.Fprintln(os.Stderr, "empty in English while it still looks right in Korean.")
		os.Exit(1)
	}
	fmt.Printf("✓ lang check passed (%d pages, every phrase paired)\n", files)
}

// check — site/ 의 .html 을 모두 본다. **목록이 아니라 디렉터리로 잡는다** — 새로
// 넣는 장이 목록에 안 적혀 슬그머니 빠지는 편이, 여기 없는 파일이 끼어드는 것보다
// 위험하다.
func check(root string) ([]finding, int, error) {
	paths, err := filepath.Glob(filepath.Join(root, "site", "*.html"))
	if err != nil {
		return nil, 0, err
	}
	if len(paths) == 0 {
		return nil, 0, fmt.Errorf("no pages under %s", filepath.Join(root, "site"))
	}
	sort.Strings(paths)

	var out []finding
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, 0, err
		}
		doc, err := html.Parse(strings.NewReader(string(src)))
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", p, err)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = p
		}
		out = append(out, inspect(doc, filepath.ToSlash(rel), string(src))...)
	}
	return out, len(paths), nil
}

// inspect — `lang` 이 붙은 요소를 문서 순서대로 모아 규칙 셋을 댄다.
//
// **중첩된 자리는 바깥만 짚는다.** 안쪽까지 세면 원인이 하나인 고장이 두 줄로 나와,
// 무엇을 고쳐야 하는지 도리어 흐려진다. 바깥의 닫는 태그를 넣으면 둘 다 풀린다.
func inspect(doc *html.Node, rel, src string) []finding {
	nodes := collect(doc)
	lines := lineIndex(src)
	// 줄을 못 맞추겠으면 아예 쓰지 않는다. 어긋난 줄은 없는 줄보다 나쁘다.
	if len(lines) != len(nodes) {
		lines = nil
	}

	inner := map[*html.Node]bool{}
	for _, n := range nodes {
		if firstLang(n) != nil {
			markInner(n, inner)
		}
	}

	var out []finding
	for i, n := range nodes {
		if inner[n] {
			continue
		}
		line := 0
		if lines != nil {
			line = lines[i]
		}
		lang, _ := attr(n, "lang")
		text := trim(textOf(n))
		other, known := langs[lang]
		if !known {
			out = append(out, finding{rel, line, unknown, lang, text})
			continue
		}
		// 1. 안쪽에 다른 말이 들어갔나. 바깥이 숨겨지면 안쪽도 함께 사라진다.
		if firstLang(n) != nil {
			out = append(out, finding{rel, line, nested, lang, text})
			continue
		}
		// 2. 같은 부모 아래에 짝이 있나. 두 벌은 나란히 적는 것이 이 사이트의 관례다.
		//
		// 옆칸이 모르는 말이면 여기는 짚지 않는다. `kr` 같은 오타 하나가 제 줄과 옆칸의
		// 「짝이 없다」 줄을 함께 내는데, 그 오타를 고치면 둘 다 풀린다. 중첩을 바깥만
		// 짚는 것과 같은 이유다.
		if !hasSibling(n, other) && !siblingUnknown(n) {
			out = append(out, finding{rel, line, unpaired, lang, text})
		}
	}
	return out
}

// collect — `lang` 이 붙은 요소를 문서 순서대로. `<html lang="ko">` 는 문서 전체의
// 초깃값이라 세지 않는다(스크립트가 막혀도 한국어가 보이게 하는 자리다).
func collect(doc *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data != "html" {
				if _, ok := attr(c, "lang"); ok {
					out = append(out, c)
				}
			}
			walk(c)
		}
	}
	walk(doc)
	return out
}

func markInner(n *html.Node, set map[*html.Node]bool) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			if _, ok := attr(c, "lang"); ok {
				set[c] = true
			}
		}
		markInner(c, set)
	}
}

// siblingUnknown — 같은 부모 아래에 모르는 말이 있나. 그 자리는 이미 제 줄로 잡혀 있다.
func siblingUnknown(n *html.Node) bool {
	if n.Parent == nil {
		return false
	}
	for c := n.Parent.FirstChild; c != nil; c = c.NextSibling {
		if c == n || c.Type != html.ElementNode {
			continue
		}
		if v, ok := attr(c, "lang"); ok {
			if _, known := langs[v]; !known {
				return true
			}
		}
	}
	return false
}

func hasSibling(n *html.Node, want string) bool {
	if n.Parent == nil {
		return false
	}
	for c := n.Parent.FirstChild; c != nil; c = c.NextSibling {
		if c == n || c.Type != html.ElementNode {
			continue
		}
		if v, ok := attr(c, "lang"); ok && v == want {
			return true
		}
	}
	return false
}

// firstLang — 하위에서 `lang` 이 붙은 첫 요소. 있으면 그 자리가 중첩이다.
func firstLang(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			if _, ok := attr(c, "lang"); ok {
				return c
			}
		}
		if got := firstLang(c); got != nil {
			return got
		}
	}
	return nil
}

func attr(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

// textOf — 요소가 품은 글자. 어느 자리인지 사람이 알아보게 하는 용도다.
func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// lineIndex — 원본에서 `lang` 이 붙은 여는 태그의 줄을 **나온 순서대로** 모은다.
//
// 파서는 줄을 세어 주지 않는다. 그렇다고 품은 글자를 원본에서 되찾는 방식으로는 안
// 된다 — 「정의」·「등급」처럼 짧고 흔한 낱말이 문서 앞쪽의 다른 자리에 먼저 걸려서
// **엉뚱한 줄을 짚는다.** 그래서 같은 원본을 토크나이저로 한 번 더 훑어 태그가 실제로
// 나온 줄을 세고, 트리에서 만난 순서와 자리를 맞춘다. 둘 다 문서 순서라 짝이 맞는다.
//
// 개수가 어긋나면 부르는 쪽이 줄을 통째로 버린다. 파서가 태그를 옮겨 놓는 자리가
// 있으면 순서가 흔들리는데, 그때는 **줄을 안 내는 편이 낫다.**
func lineIndex(src string) []int {
	var out []int
	z := html.NewTokenizer(strings.NewReader(src))
	line := 1
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return out
		}
		raw := string(z.Raw())
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			name, hasAttr := z.TagName()
			isRoot := string(name) == "html"
			found := false
			for hasAttr {
				var k []byte
				k, _, hasAttr = z.TagAttr()
				if string(k) == "lang" {
					found = true
				}
			}
			if found && !isRoot {
				out = append(out, line)
			}
		}
		line += strings.Count(raw, "\n")
	}
}

func trim(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > 50 {
		return string([]rune(s)[:50]) + "…"
	}
	return s
}
