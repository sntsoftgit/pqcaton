// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// guideDoc: 운영자 안내서 한 판에서 시험이 견주는 것.
type guideDoc struct {
	headings []string // 「## N. 이름」의 번호와 이름
	steps    []string // 실행 표시(run · setup)와 명령 블록, 차례대로
	blocks   []string // 모든 bash 블록, 차례대로
	codes    []string // 오류 표의 코드
}

var (
	guideHeading = regexp.MustCompile(`(?m)^## (\d+)\. (.+)$`)
	guideMarker  = regexp.MustCompile(`^<!-- (run(?:: [a-z]+)?|setup: .+) -->$`)
	guideCode    = regexp.MustCompile("^\\| `([a-z-]+)` \\|")
)

func readGuide(t *testing.T, name string) guideDoc {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", name))
	if err != nil {
		t.Fatal(err)
	}
	var g guideDoc
	for _, m := range guideHeading.FindAllStringSubmatch(string(b), -1) {
		g.headings = append(g.headings, m[1]+". "+m[2])
	}
	lines := strings.Split(string(b), "\n")
	marker := ""
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if m := guideMarker.FindStringSubmatch(l); m != nil {
			if strings.HasPrefix(m[1], "setup:") {
				g.steps = append(g.steps, m[1])
			} else {
				marker = m[1]
			}
			continue
		}
		if l == "```bash" {
			var code []string
			for i++; i < len(lines) && lines[i] != "```"; i++ {
				code = append(code, lines[i])
			}
			block := strings.Join(code, "\n")
			g.blocks = append(g.blocks, block)
			if marker != "" {
				g.steps = append(g.steps, marker+"\n"+block)
			}
			marker = ""
			continue
		}
		if m := guideCode.FindStringSubmatch(l); m != nil {
			g.codes = append(g.codes, m[1])
		}
		if strings.TrimSpace(l) != "" {
			marker = ""
		}
	}
	return g
}

// IC-UI52 — **운영자 안내서 두 판은 차례 · 명령 · 오류 코드가 같다.** 절의 번호와 개수, bash 블록과 실행 표시의 차례가
// 같고, 오류 표의 코드는 화면이 내는 오류 코드 전부와 같다. 화면 문구가 「운영자 안내서의 「…」 절」로 가리키는 이름은
// 그 판에 실제로 있는 절이다.
func TestOperatorGuidesAgree(t *testing.T) {
	ko, en := readGuide(t, "operator-guide.md"), readGuide(t, "operator-guide.en.md")
	if len(ko.headings) == 0 || len(ko.headings) != len(en.headings) {
		t.Fatalf("sections: ko %d, en %d", len(ko.headings), len(en.headings))
	}
	for i := range ko.headings {
		if strings.SplitN(ko.headings[i], ".", 2)[0] != strings.SplitN(en.headings[i], ".", 2)[0] {
			t.Errorf("section %d: ko %q, en %q", i, ko.headings[i], en.headings[i])
		}
	}
	if !slices.Equal(ko.blocks, en.blocks) {
		t.Errorf("the command blocks differ:\nko %q\nen %q", ko.blocks, en.blocks)
	}
	if len(ko.steps) == 0 || !slices.Equal(ko.steps, en.steps) {
		t.Errorf("the run markers differ:\nko %q\nen %q", ko.steps, en.steps)
	}
	want := RefusedKinds()
	slices.Sort(want)
	for name, g := range map[string]guideDoc{"ko": ko, "en": en} {
		got := slices.Clone(g.codes)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s: the error table has %v, want every error code %v", name, got, want)
		}
	}

	// 화면 문구가 가리키는 절.
	src := ""
	for _, f := range []string{"text.go", "text_more.go", "text_msg.go", "text_notice.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src += string(b)
	}
	titles := func(g guideDoc) []string {
		var out []string
		for _, h := range g.headings {
			out = append(out, strings.SplitN(h, ". ", 2)[1])
		}
		return out
	}
	for _, c := range []struct {
		lang string
		ref  *regexp.Regexp
		g    guideDoc
	}{
		{"ko", regexp.MustCompile(`운영자 안내서의 「([^」]+)」`), ko},
		{"en", regexp.MustCompile(`the \\"([^"\\]+)\\" section of the operator guide`), en},
	} {
		refs := c.ref.FindAllStringSubmatch(src, -1)
		if len(refs) == 0 {
			t.Errorf("%s: no references to the guide found in the screen text", c.lang)
		}
		for _, m := range refs {
			if !slices.Contains(titles(c.g), m[1]) {
				t.Errorf("%s: the screen points at section %q, which the guide does not have", c.lang, m[1])
			}
		}
	}
}
