// Command checkcases — **케이스 번호와 실제 테스트를 맞댄다.**
//
// docs/testcases.md가 첫머리에서 「케이스 번호가 곧 테스트 파일 링크입니다」라고 약속하는데,
// 실제로는 백일흔넷 가운데 링크가 하나도 없었다. 약속과 사실이 어긋난 것을 사람이 알아채기까지
// 두 주가 걸렸다. 그래서 **약속을 지키는 일을 기계에 맡긴다.**
//
// 재는 것은 셋이다.
//
//  1. 문서가 ✅라고 적은 케이스는 **테스트에 그 번호가 적혀 있어야 한다.** 없으면 통과했다는
//     말의 근거가 없다.
//  2. 테스트에 적힌 번호는 **문서에 있어야 한다.** 없으면 무엇을 재는 케이스인지 아무도 모른다.
//  3. 문서가 ⏳·🔜라고 적은 것에 테스트가 있으면 **표시가 낡은 것이다.**
//
// 그리고 링크는 **손으로 붙이지 않는다.** `-write`가 번호에서 파일로 가는 링크를 찍는다.
// 손으로 붙이면 파일을 옮기는 날 링크 전부가 한꺼번에 썩는다.
//
// **문서가 하나가 아니다.** 인벤토리 케이스는 docs/testcases.md에, 러너 케이스는 러너 옆에
// 있다. 코드가 거기 있으니 케이스도 거기 있어야 한다. 문서마다 맡는 접두어를 적어 두고, 그
// 접두어의 번호만 그 문서에서 찾는다.
//
// **그 목록은 코드가 아니라 설정 파일(-dir 의 docs.tsv)에 있다.** 엔진은 리포마다 같고 다른
// 것은 이 목록뿐이라, 목록을 코드에 두면 엔진째로 복사하게 된다. 문서마다 접두어를 둘로 나눠
// 적는다. owned 는 이 리포의 테스트가 재는 것, external 은 다른 리포가 재서 여기서는 테스트를
// 요구하지 않는 것이다. 어느 쪽에도 없는 접두어의 행은 막는다. 목록에서 빼기만 하면 그 행이
// 아무 표시 없이 관문 밖으로 나가고, 왜 재지 않는지가 어디에도 남지 않는다.
//
// 경로는 리포 루트 기준이고, 관문은 리포 루트에서 돈다. docs.tsv 가 있는 자리가 기준이 아니다.
//
// **테스트가 번호를 축약해 적는다.** 앞머리를 한 번만 쓰고 뒤 번호를 가운뎃점으로 이어 붙이는
// 자리가 있어 그것을 펴서 읽는다. 처음에 이 축약을 놓쳐 「✅ 인데 테스트가 없는 케이스 여섯」을
// 잘못 세었다. 그 실수가 이 도구를 만든 이유이기도 하다.
//
// usage:
//
//	go run ./tools/checkcases          # 관문
//	go run ./tools/checkcases -write   # 번호에 링크를 찍는다
//	go run ./tools/checkcases -dir d   # 설정을 d/docs.tsv 에서 읽는다(기본 tools/checkcases)
package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// docSpec — docs.tsv 의 한 행. 케이스 표가 있는 문서와 **그 문서가 접두어를 어떻게 다루는가.**
type docSpec struct {
	path     string   // 리포 루트 기준
	owned    []string // 이 리포의 테스트가 잰다
	external []string // 다른 리포가 잰다. 여기서는 테스트를 요구하지 않는다
}

// kinds — 엔진이 읽을 줄 아는 접두어. 아래 shapes 와 짝이다. 새 접두어는 설정만으로는 안 되고
// 번호 모양을 엔진에 더해야 한다. 모르는 접두어를 설정에 적으면 막는다.
var kinds = []string{"IC", "CP", "RUN"}

// 번호 모양 셋. IC-R1은 글자와 숫자가 붙고, CP-TOKEN-1은 낱말이 하나 더 있고,
// RUN-2는 숫자만이다. 뒤의 괄호가 「·」로 이어 붙인 축약을 받는다.
var shapes = []*regexp.Regexp{
	regexp.MustCompile(`(IC)-([A-Z]+)(\d+(?:\s*[·,]\s*[A-Z]*\d+)*)`),
	regexp.MustCompile(`(CP)-([A-Z]+)-(\d+(?:\s*[·,]\s*\d+)*)`),
	regexp.MustCompile(`(RUN)()-(\d+(?:\s*[·,]\s*\d+)*)`),
}

var part = regexp.MustCompile(`^([A-Z]*)(\d+)$`)
var sep = regexp.MustCompile(`\s*[·,]\s*`)

// idAlt — 표의 번호 칸. **문서도 테스트처럼 축약해 적는다**(`CP-TOKEN-4·5·6`). 축약을 못
// 읽으면 그 행이 통째로 안 보이고, 그러면 셋 다 「표에 없다」로 막힌다.
const idAlt = `IC-[A-Z]+\d+(?:\s*[·,]\s*[A-Z]*\d+)*|CP-[A-Z]+-\d+(?:\s*[·,]\s*\d+)*|RUN-\d+(?:\s*[·,]\s*\d+)*`

// row — 케이스 표의 첫 칸. 굵게가 대괄호 밖일 수도 안일 수도 있고, 상태 표시는 없을 수도
// 있다(컨트롤 플레인 명세가 그렇다). 이미 링크가 붙은 것도 같은 자리에서 읽는다.
var row = regexp.MustCompile(
	`^\|[ \t]*(\*\*)?(?:\[(\*\*)?(` + idAlt + `)(?:\*\*)?\]\(([^)]*)\)|(` + idAlt + `))[ \t]*(✅|🔜|⏳)?[ \t]*(\*\*)?[ \t]*\|`)

type docCase struct {
	id      string
	covers  []string // 축약으로 한 행이 여러 번호를 맡는다. 링크는 첫 번호로 건다.
	status  string
	link    string
	doc     string
	line    int
	boldOut bool
	boldIn  bool
}

func main() {
	dir := flag.String("dir", "tools/checkcases", "directory holding docs.tsv")
	write := flag.Bool("write", false, "rewrite the case IDs as links to their tests")
	flag.Parse()

	docs, err := loadDocs(filepath.Join(*dir, "docs.tsv"))
	if err != nil {
		fail(err)
	}
	tests, err := scanTests(".")
	if err != nil {
		fail(err)
	}

	owned := map[string]bool{}
	for _, d := range docs {
		for _, k := range d.owned {
			owned[k] = true
		}
	}

	var all []docCase
	var strays []string
	lines := map[string][]string{}
	for _, d := range docs {
		cs, ls, st, err := scanDoc(".", d)
		if err != nil {
			fail(err)
		}
		all = append(all, cs...)
		strays = append(strays, st...)
		lines[d.path] = ls
	}

	var problems []string
	seen := map[string]bool{}
	for _, c := range all {
		for _, id := range c.covers {
			seen[id] = true
		}
		files := tests[c.covers[0]]
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
	problems = append(problems, strays...)

	if *write {
		n := 0
		for _, d := range docs {
			n += rewrite(d, lines[d.path], all, tests)
			if err := os.WriteFile(filepath.FromSlash(d.path), []byte(strings.Join(lines[d.path], "\n")), 0o644); err != nil {
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
		fmt.Fprintln(os.Stderr, "\nFix the ids, or rerun the case gate with -write")
		os.Exit(1)
	}
	missing := 0
	for _, c := range all {
		if c.link == "" && len(tests[c.covers[0]]) > 0 {
			missing++
		}
	}
	if missing > 0 {
		fmt.Fprintf(os.Stderr, "✗ case gate: %d case ids are not links yet\n", missing)
		fmt.Fprintln(os.Stderr, "  the id is meant to be the link. rerun the case gate with -write")
		os.Exit(1)
	}
	fmt.Printf("✓ case check passed (%d cases in %d tables, all tied to a test)\n", len(all), len(docs))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "✗ checkcases:", err)
	os.Exit(1)
}

// ── 설정 ───────────────────────────────────────────────────────────────────

// loadDocs — docs.tsv 를 읽는다. 한 줄에 하나, 탭으로 나눈 세 칸(경로·owned·external)이다.
// 칸 안의 접두어는 쉼표로 잇고, 없으면 `-` 를 적는다. 정렬을 위해 탭을 여럿 써도 된다.
//
// **잘못된 행은 고쳐 읽지 않고 막는다.** 칸 수가 다른 것, 리포 밖이나 정규형이 아닌 경로, 같은
// 경로가 두 번 나오는 것, 엔진이 모르는 접두어, 빈 항목·중복 항목, 아무 접두어도 다루지 않는
// 행, 한 접두어를 owned 와 external 에 함께 적은 것. 설정이 틀렸는데 관문이 통과하면 그 통과가
// 아무것도 재지 않은 결과일 수 있다.
func loadDocs(file string) ([]docSpec, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var out []docSpec
	paths := map[string]bool{}
	role := map[string]string{}
	for i, l := range strings.Split(string(b), "\n") {
		if t := strings.TrimSpace(l); t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		at := fmt.Sprintf("%s:%d", file, i+1)
		var f []string
		for _, s := range strings.Split(l, "\t") {
			if s = strings.TrimSpace(s); s != "" {
				f = append(f, s)
			}
		}
		if len(f) != 3 {
			return nil, fmt.Errorf("%s: want 3 tab-separated fields (path, owned, external), got %d", at, len(f))
		}
		p := f[0]
		if !filepath.IsLocal(filepath.FromSlash(p)) || path.Clean(p) != p {
			return nil, fmt.Errorf("%s: %q must be a clean path relative to the repo root", at, p)
		}
		if paths[p] {
			return nil, fmt.Errorf("%s: %s is listed twice", at, p)
		}
		paths[p] = true
		d := docSpec{path: p}
		if d.owned, err = prefixes(f[1]); err != nil {
			return nil, fmt.Errorf("%s: owned: %w", at, err)
		}
		if d.external, err = prefixes(f[2]); err != nil {
			return nil, fmt.Errorf("%s: external: %w", at, err)
		}
		if len(d.owned)+len(d.external) == 0 {
			return nil, fmt.Errorf("%s: %s classifies no prefix", at, p)
		}
		for _, r := range []struct {
			name string
			ks   []string
		}{{"owned", d.owned}, {"external", d.external}} {
			for _, k := range r.ks {
				if prev, ok := role[k]; ok && prev != r.name {
					return nil, fmt.Errorf("%s: %s is both owned and external", at, k)
				}
				role[k] = r.name
			}
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no case tables listed", file)
	}
	return out, nil
}

// prefixes — 한 칸의 접두어 목록. `-` 는 없음이다.
func prefixes(s string) ([]string, error) {
	if s == "-" {
		return nil, nil
	}
	var out []string
	for _, k := range strings.Split(s, ",") {
		switch {
		case k == "":
			return nil, fmt.Errorf("empty prefix in %q", s)
		case !containsStr(kinds, k):
			return nil, fmt.Errorf("unknown prefix %q (the engine reads %s)", k, strings.Join(kinds, ", "))
		case containsStr(out, k):
			return nil, fmt.Errorf("prefix %s repeated", k)
		}
		out = append(out, k)
	}
	return out, nil
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
		// 하나가 이 도구의 테스트 파일로 링크됐다. checktext가 반대 방향으로 겪은 것과
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

// scanDoc — 문서 하나의 케이스 행을 읽는다. 경로는 root 에 이어 붙여 읽되, 케이스에는 리포
// 루트 기준 경로를 그대로 적는다. 링크가 그 경로에서 계산되기 때문이다.
//
// owned 접두어의 행만 케이스로 돌려준다. external 접두어의 행은 건너뛰고, 어느 쪽에도 없는
// 접두어의 행은 세 번째 반환값으로 짚는다.
func scanDoc(root string, d docSpec) ([]docCase, []string, []string, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(d.path)))
	if err != nil {
		return nil, nil, nil, err
	}
	lines := strings.Split(string(b), "\n")
	var out []docCase
	var strays []string
	for i, l := range lines {
		m := row.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		raw, link := m[5], ""
		if m[3] != "" {
			raw, link = m[3], m[4]
		}
		covers := ids(raw)
		if len(covers) == 0 {
			continue
		}
		if k := kindOf(covers[0]); !containsStr(d.owned, k) {
			if !containsStr(d.external, k) {
				strays = append(strays, fmt.Sprintf("%s:%d  %s has prefix %s, which this table neither owns nor marks external", d.path, i+1, raw, k))
			}
			continue
		}
		out = append(out, docCase{
			id: raw, covers: covers, status: m[6], link: link, doc: d.path, line: i + 1,
			boldOut: m[1] == "**", boldIn: m[2] == "**",
		})
	}
	return out, lines, strays, nil
}

// rewrite — 번호 칸을 링크로 바꾼다. 굵게가 대괄호 밖이었는지 안이었는지, 상태 표시가
// 있었는지를 그대로 지킨다. 모양이 달라지면 사람이 diff를 못 읽는다.
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

// linkTo — 문서마다 자리가 다르므로 그 문서에서 본 상대 경로로 적는다. docs/에 있는 문서는
// `../pkg/…`, 테스트 옆에 있는 문서는 `runner_test.go`가 된다.
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
