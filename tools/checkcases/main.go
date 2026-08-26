// Command checkcases — **케이스 번호와 실제 테스트를 맞댄다.**
//
// docs/testcases.md 가 첫머리에서 「케이스 번호가 곧 테스트 파일 링크입니다」라고 약속하는데,
// 실제로는 백일흔넷 가운데 링크가 하나도 없었다. 약속과 사실이 어긋난 것을 사람이 알아채기까지
// 두 주가 걸렸다. 그래서 **약속을 지키는 일을 기계에 맡긴다.**
//
// 재는 것은 셋이다.
//
//  1. 문서가 ✅ 라고 적은 케이스는 **테스트에 그 번호가 적혀 있어야 한다.** 없으면 통과했다는
//     말의 근거가 없다.
//  2. 테스트에 적힌 번호는 **문서에 있어야 한다.** 없으면 무엇을 재는 케이스인지 아무도 모른다.
//  3. 문서가 ⏳·🔜 라고 적은 것에 테스트가 있으면 **표시가 낡은 것이다.**
//
// 그리고 링크는 **손으로 붙이지 않는다.** `-write` 가 번호에서 파일로 가는 링크를 찍는다.
// 손으로 붙이면 파일을 옮기는 날 백일흔아홉이 한꺼번에 썩는다.
//
// **문서가 하나가 아니다.** 인벤토리 케이스는 docs/testcases.md 에, 러너 케이스는 러너 옆에
// 있다. 코드가 거기 있으니 케이스도 거기 있어야 한다. 문서마다 맡는 접두어를 적어 두고, 그
// 접두어의 번호만 그 문서에서 찾는다.
//
// **테스트가 번호를 축약해 적는다.** 앞머리를 한 번만 쓰고 뒤 번호를 가운뎃점으로 이어 붙이는
// 자리가 있어 그것을 펴서 읽는다. 처음에 이 축약을 놓쳐 「✅ 인데 테스트가 없는 케이스 여섯」을
// 잘못 세었다. 그 실수가 이 도구를 만든 이유이기도 하다.
//
// usage:
//
//	go run ./tools/checkcases          # 관문
//	go run ./tools/checkcases -write   # 번호에 링크를 찍는다
package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// docs — 케이스 표가 있는 문서와 **그 문서가 맡는 접두어.** 목록에 없는 접두어는 그 문서에서
// 읽지 않는다. 이 리포는 인벤토리(IC)와 러너(RUN) 둘을 갖는다.
var docs = []docSpec{
	{path: "docs/testcases.md", kinds: []string{"IC"}},
	{path: "saas/runner/README.md", kinds: []string{"RUN"}},
}

type docSpec struct {
	path  string
	kinds []string
}

// 번호 모양 셋. IC-R1 은 글자와 숫자가 붙고, CP-TOKEN-1 은 낱말이 하나 더 있고,
// RUN-2 는 숫자만이다. 뒤의 괄호가 「·」로 이어 붙인 축약을 받는다.
var shapes = []*regexp.Regexp{
	regexp.MustCompile(`(IC)-([A-Z]+)(\d+(?:\s*[·,]\s*[A-Z]*\d+)*)`),
	regexp.MustCompile(`(CP)-([A-Z]+)-(\d+(?:\s*[·,]\s*\d+)*)`),
	regexp.MustCompile(`(RUN)()-(\d+(?:\s*[·,]\s*\d+)*)`),
}

var part = regexp.MustCompile(`^([A-Z]*)(\d+)$`)
var sep = regexp.MustCompile(`\s*[·,]\s*`)

const idAlt = `IC-[A-Z]+\d+|CP-[A-Z]+-\d+|RUN-\d+`

// row — 케이스 표의 첫 칸. 굵게가 대괄호 밖일 수도 안일 수도 있고, 상태 표시는 없을 수도
// 있다(컨트롤 플레인 명세가 그렇다). 이미 링크가 붙은 것도 같은 자리에서 읽는다.
var row = regexp.MustCompile(
	`^\|[ \t]*(\*\*)?(?:\[(\*\*)?(` + idAlt + `)(?:\*\*)?\]\(([^)]*)\)|(` + idAlt + `))[ \t]*(✅|🔜|⏳)?[ \t]*(\*\*)?[ \t]*\|`)

type docCase struct {
	id      string
	status  string
	link    string
	doc     string
	line    int
	boldOut bool
	boldIn  bool
}

func main() {
	write := flag.Bool("write", false, "rewrite the case IDs as links to their tests")
	flag.Parse()

	tests, err := scanTests(".")
	if err != nil {
		fail(err)
	}

	owned := map[string]bool{}
	for _, d := range docs {
		for _, k := range d.kinds {
			owned[k] = true
		}
	}

	var all []docCase
	lines := map[string][]string{}
	for _, d := range docs {
		cs, ls, err := scanDoc(d)
		if err != nil {
			fail(err)
		}
		all = append(all, cs...)
		lines[d.path] = ls
	}

	var problems []string
	seen := map[string]bool{}
	for _, c := range all {
		seen[c.id] = true
		files := tests[c.id]
		want := ""
		if len(files) > 0 {
			want = linkTo(c.doc, files[0])
		}
		switch {
		case c.status != "🔜" && c.status != "⏳" && len(files) == 0:
			problems = append(problems, fmt.Sprintf("%s:%d  %s claims a test but none carries that id", c.doc, c.line, c.id))
		case (c.status == "🔜" || c.status == "⏳") && len(files) > 0:
			problems = append(problems, fmt.Sprintf("%s:%d  %s is %s but %s carries that id", c.doc, c.line, c.id, c.status, files[0]))
		case want != "" && c.link != "" && c.link != want:
			problems = append(problems, fmt.Sprintf("%s:%d  %s links to %s, but the test is %s", c.doc, c.line, c.id, c.link, files[0]))
		}
	}
	for _, id := range sortedKeys(tests) {
		if !owned[kindOf(id)] || seen[id] {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s  is in %s but in no case table", id, tests[id][0]))
	}

	if *write {
		n := 0
		for _, d := range docs {
			n += rewrite(d, lines[d.path], all, tests)
			if err := os.WriteFile(d.path, []byte(strings.Join(lines[d.path], "\n")), 0o644); err != nil {
				fail(err)
			}
		}
		fmt.Printf("✓ %d case ids now link to their tests\n", n)
		return
	}

	if len(problems) > 0 {
		fmt.Fprintln(os.Stderr, "✗ case gate: the docs and the tests disagree")
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "   ", p)
		}
		fmt.Fprintln(os.Stderr, "\nFix the ids, or run: go run ./tools/checkcases -write")
		os.Exit(1)
	}
	missing := 0
	for _, c := range all {
		if c.link == "" && len(tests[c.id]) > 0 {
			missing++
		}
	}
	if missing > 0 {
		fmt.Fprintf(os.Stderr, "✗ case gate: %d case ids are not links yet\n", missing)
		fmt.Fprintln(os.Stderr, "  the id is meant to be the link. run: go run ./tools/checkcases -write")
		os.Exit(1)
	}
	fmt.Printf("✓ case check passed (%d cases in %d tables, all tied to a test)\n", len(all), len(docs))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "✗ checkcases:", err)
	os.Exit(1)
}

// ── 테스트 ─────────────────────────────────────────────────────────────────

// scanTests — 테스트 파일의 **주석에** 적힌 번호를 모은다. 한 번호가 두 파일에서 재어질 수
// 있으므로(엣지판·본판) 목록으로 들고, 링크는 정렬해서 첫 파일로 건다.
func scanTests(root string) (map[string][]string, error) {
	out := map[string][]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == ".git" || n == "node_modules" || n == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		// **주석만 본다.** 이 도구의 테스트가 픽스처로 케이스 표의 한 줄을 문자열에 담고
		// 있어, 파일 전체를 정규식으로 훑으면 그것이 표식으로 잡힌다. 실제로 미구현 케이스
		// 하나가 이 도구의 테스트 파일로 링크됐다. checktext 가 반대 방향으로 겪은 것과
		// 같은 일이라 같은 답을 쓴다: 정규식이 아니라 파서로 본다.
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		rel := strings.TrimPrefix(filepath.ToSlash(p), "./")
		for _, g := range f.Comments {
			for _, id := range ids(g.Text()) {
				if !contains(out[id], rel) {
					out[id] = append(out[id], rel)
				}
			}
		}
		return nil
	})
	for id := range out {
		sort.Strings(out[id])
	}
	return out, err
}

// ids — 축약한 번호를 편다. 앞머리를 한 번만 적고 뒤를 가운뎃점으로 이어 붙인 자리를 푼다.
func ids(s string) []string {
	var out []string
	for _, re := range shapes {
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			kind, head := m[1], m[2]
			for _, seg := range sep.Split(m[3], -1) {
				p := part.FindStringSubmatch(seg)
				if p == nil {
					continue
				}
				w := p[1]
				if w == "" {
					w = head
				}
				out = append(out, join(kind, w, p[2]))
			}
		}
	}
	return out
}

func join(kind, word, num string) string {
	switch kind {
	case "IC":
		return "IC-" + word + num
	case "CP":
		return "CP-" + word + "-" + num
	default:
		return kind + "-" + num
	}
}

func kindOf(id string) string {
	if i := strings.Index(id, "-"); i > 0 {
		return id[:i]
	}
	return id
}

// ── 문서 ───────────────────────────────────────────────────────────────────

func scanDoc(d docSpec) ([]docCase, []string, error) {
	b, err := os.ReadFile(d.path)
	if err != nil {
		return nil, nil, err
	}
	lines := strings.Split(string(b), "\n")
	var out []docCase
	for i, l := range lines {
		m := row.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		id, link := m[5], ""
		if m[3] != "" {
			id, link = m[3], m[4]
		}
		if !containsStr(d.kinds, kindOf(id)) {
			continue
		}
		out = append(out, docCase{
			id: id, status: m[6], link: link, doc: d.path, line: i + 1,
			boldOut: m[1] == "**", boldIn: m[2] == "**",
		})
	}
	return out, lines, nil
}

// rewrite — 번호 칸을 링크로 바꾼다. 굵게가 대괄호 밖이었는지 안이었는지, 상태 표시가
// 있었는지를 그대로 지킨다. 모양이 달라지면 사람이 diff 를 못 읽는다.
func rewrite(d docSpec, lines []string, all []docCase, tests map[string][]string) int {
	n := 0
	for _, c := range all {
		if c.doc != d.path {
			continue
		}
		files := tests[c.id]
		if len(files) == 0 {
			continue
		}
		i := c.line - 1
		m := row.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		neu := cell(c, linkTo(c.doc, files[0]))
		if m[0] == neu {
			continue
		}
		lines[i] = neu + lines[i][len(m[0]):]
		n++
	}
	return n
}

func cell(c docCase, link string) string {
	st := ""
	if c.status != "" {
		st = " " + c.status
	}
	switch {
	case c.boldIn:
		return "| [**" + c.id + "**](" + link + ")" + st + " |"
	case c.boldOut:
		return "| **[" + c.id + "](" + link + ")" + st + "** |"
	default:
		return "| [" + c.id + "](" + link + ")" + st + " |"
	}
}

// linkTo — 문서마다 자리가 다르므로 그 문서에서 본 상대 경로로 적는다. docs/ 에 있는 문서는
// `../pkg/…`, 테스트 옆에 있는 문서는 `runner_test.go` 가 된다.
func linkTo(doc, test string) string {
	rel, err := filepath.Rel(filepath.Dir(doc), test)
	if err != nil {
		return test
	}
	return filepath.ToSlash(rel)
}

func contains(xs []string, s string) bool { return containsStr(xs, s) }

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
