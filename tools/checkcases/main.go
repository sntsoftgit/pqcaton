// Command checkcases — **케이스 번호와 실제 테스트를 맞댄다.**
//
// docs/testcases.md 가 첫머리에서 「케이스 번호가 곧 테스트 파일 링크입니다」라고 약속하는데,
// 실제로는 174개 가운데 링크가 하나도 없었다. 약속과 사실이 어긋난 것을 사람이 알아채기까지
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
// 손으로 붙이면 파일을 옮기는 날 백일흔넷이 한꺼번에 썩는다.
//
// **테스트가 번호를 축약해 적는다.** `// IC-R1·R2·R3` 처럼 앞머리를 한 번만 쓰는 자리가 있어
// 그것을 펴서 읽는다. 처음에 이 축약을 놓쳐 「✅ 인데 테스트가 없는 케이스 여섯」을 잘못
// 세었다. 그 실수가 이 도구를 만든 이유이기도 하다.
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

const docPath = "docs/testcases.md"

// marker — 테스트 주석에 적힌 번호. 「IC-R1·R2·R3」 처럼 앞머리를 한 번만 쓰는 축약을 편다.
var marker = regexp.MustCompile(`IC-([A-Z]+)(\d+(?:\s*[·,]\s*[A-Z]*\d+)*)`)

var part = regexp.MustCompile(`^([A-Z]*)(\d+)$`)

// row — 케이스 표의 첫 칸. 두 모양뿐이다: `| IC-R1 ✅ |` 과 `| **IC-R1 ✅** |`.
// 이미 링크가 붙은 것도 같은 자리에서 읽는다.
var row = regexp.MustCompile(`^\|\s*(\*\*)?(?:\[(IC-[A-Z]+\d+)\]\(([^)]*)\)|(IC-[A-Z]+\d+))\s*(✅|🔜|⏳)\s*(\*\*)?\s*\|`)

type docCase struct {
	id     string
	status string
	link   string
	line   int
	bold   bool
}

func main() {
	write := flag.Bool("write", false, "rewrite the case IDs as links to their tests")
	flag.Parse()

	tests, err := scanTests(".")
	if err != nil {
		fail(err)
	}
	cases, lines, err := scanDoc(docPath)
	if err != nil {
		fail(err)
	}

	var problems []string
	seen := map[string]bool{}
	for _, c := range cases {
		seen[c.id] = true
		files := tests[c.id]
		switch {
		case c.status == "✅" && len(files) == 0:
			problems = append(problems, fmt.Sprintf("%s:%d  %s is ✅ but no test carries that id", docPath, c.line, c.id))
		case c.status != "✅" && len(files) > 0:
			problems = append(problems, fmt.Sprintf("%s:%d  %s is %s but %s carries that id", docPath, c.line, c.id, c.status, files[0]))
		case len(files) > 0 && c.link != "" && c.link != linkTo(files[0]):
			problems = append(problems, fmt.Sprintf("%s:%d  %s links to %s, but the test is %s", docPath, c.line, c.id, c.link, files[0]))
		}
	}
	for _, id := range sortedKeys(tests) {
		if !seen[id] {
			problems = append(problems, fmt.Sprintf("%s  is in %s but not in %s", id, tests[id][0], docPath))
		}
	}

	if *write {
		n := rewrite(lines, cases, tests)
		if err := os.WriteFile(docPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("✓ %d case ids now link to their tests\n", n)
		return
	}

	if len(problems) > 0 {
		fmt.Fprintln(os.Stderr, "✗ case gate: the doc and the tests disagree")
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "   ", p)
		}
		fmt.Fprintln(os.Stderr, "\nFix the ids, or run: go run ./tools/checkcases -write")
		os.Exit(1)
	}
	missing := 0
	for _, c := range cases {
		if c.status == "✅" && c.link == "" {
			missing++
		}
	}
	if missing > 0 {
		fmt.Fprintf(os.Stderr, "✗ case gate: %d case ids are not links yet\n", missing)
		fmt.Fprintln(os.Stderr, "  the doc promises that the id is the link. run: go run ./tools/checkcases -write")
		os.Exit(1)
	}
	fmt.Printf("✓ case check passed (%d cases, all tied to a test)\n", len(cases))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "✗ checkcases:", err)
	os.Exit(1)
}

// scanTests — 테스트 파일의 주석에 적힌 번호를 모은다. 한 번호가 두 파일에서 재어질 수
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
		// **주석만 본다.** 이 도구의 테스트가 픽스처로 `| IC-C3 ⏳ |` 같은 줄을 문자열에
		// 담고 있어, 파일 전체를 정규식으로 훑으면 그것이 표식으로 잡힌다. 실제로 IC-C3 이
		// 이 도구의 테스트 파일로 링크됐다. checktext 가 반대 방향으로 겪은 것과 같은 일이라
		// 같은 답을 쓴다: 정규식이 아니라 파서로 본다.
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

// ids — 「IC-R1·R2·R3」 를 IC-R1 · IC-R2 · IC-R3 으로 편다.
func ids(s string) []string {
	var out []string
	for _, m := range marker.FindAllStringSubmatch(s, -1) {
		head := m[1]
		for _, seg := range regexp.MustCompile(`\s*[·,]\s*`).Split(m[2], -1) {
			p := part.FindStringSubmatch(seg)
			if p == nil {
				continue
			}
			pre := p[1]
			if pre == "" {
				pre = head
			}
			out = append(out, "IC-"+pre+p[2])
		}
	}
	return out
}

func scanDoc(path string) ([]docCase, []string, error) {
	b, err := os.ReadFile(path)
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
		id, link := m[4], ""
		if m[2] != "" {
			id, link = m[2], m[3]
		}
		out = append(out, docCase{id: id, status: m[5], link: link, line: i + 1, bold: m[1] == "**"})
	}
	return out, lines, nil
}

// rewrite — 번호 칸을 링크로 바꾼다. 굵게와 상태 표시는 그대로 둔다.
func rewrite(lines []string, cases []docCase, tests map[string][]string) int {
	n := 0
	for _, c := range cases {
		files := tests[c.id]
		if len(files) == 0 {
			continue
		}
		want := fmt.Sprintf("[%s](%s)", c.id, linkTo(files[0]))
		i := c.line - 1
		m := row.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		old := m[0]
		bold := ""
		if c.bold {
			bold = "**"
		}
		neu := fmt.Sprintf("| %s%s %s%s |", bold, want, c.status, bold)
		if old == neu {
			continue
		}
		lines[i] = neu + lines[i][len(old):]
		n++
	}
	return n
}

// linkTo — 문서가 docs/ 에 있으므로 리포 루트 기준 경로를 한 단계 올려 적는다.
func linkTo(rel string) string {
	return "../" + rel
}

func contains(xs []string, s string) bool {
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
