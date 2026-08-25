// Command checkprose — 문서와 화면 문구의 **한국어 문체**를 본다.
//
// 규칙은 하나다: **한 번 걷어낸 말이 다시 들어오지 않는다.**
//
// 왜 관문인가. 두 리포의 한국어를 전부 읽어 보니, 눈으로 지킨 규칙은 남아 있지 않고
// **기계로 막은 규칙만** 남아 있었다. 폼에 넣는 문서 다섯 개는 check-form-text.py 가
// 막고 있어 엠대시가 하나도 없는데, 그 관문이 보지 않는 문서에는 천 개가 넘게 쌓여
// 있었다. 「절차의 한 걸음」은 하루 걷어냈다가 다음 날 릴리스 노트에 되살아났다.
//
// **기준선을 둔다.** 지금 있는 것을 한꺼번에 막으면 관문이 처음부터 붉어서 아무도 켜지
// 않는다. 그래서 파일마다 지금 개수를 적어 두고 **늘면 막는다.** 줄여도 막고 새 기준선을
// 찍어 준다: 고쳐 놓고 기준선을 안 내리면 그 자리가 도로 채워져도 알 수 없다.
//
// **한국어가 없는 줄도 보지 않는다.** 지침은 한국어를 명확하게 쓰라는 것이지 외국어를
// 고치라는 것이 아니다. 화면 카탈로그는 KO 와 EN 을 나란히 적는 자리다.
//
// **코드는 보지 않는다.** 변수명·주석·커밋·로그처럼 코드에 속하는 텍스트는 프로젝트
// 관례를 따르는 자리다. 그래서 마크다운은 코드 블록과 인라인 코드를 덮고 나서 보며,
// Go 는 go/ast 로 **문자열 리터럴만** 본다(주석은 한국어다). HTML 은 code·pre·script·
// style 안을 덮는다.
//
// **규칙은 rules.tsv 에, 잘못 잡는 말은 overlap.txt 에 있다.** 코드에 적으면 tools/checktext
// 가 막는다(Go 문자열의 한글). 그리고 두 목록 모두 지적받을 때마다 늘어나므로 코드 밖에
// 있어야 한다.
//
// usage:
//
//	go run ./tools/checkprose             # 관문
//	go run ./tools/checkprose -list       # 걸린 자리를 줄 번호까지
//	go run ./tools/checkprose -baseline   # 기준선을 다시 찍는다
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// extraGo — 마크다운 밖에서 사람이 읽는 자리 가운데 Go 소스인 것. **경로로 적는다**:
// 규약으로 두면 새 파일이 슬그머니 들어오거나 빠진다(checktext 와 같은 이유).
//
// 화면 문구는 네 파일에 나뉘어 있다. text.go 가 카탈로그이고, 나머지 셋은 「영어는 그
// 사실을 가진 패키지가 갖고 한국어는 화면이 갖는다」는 규칙에 따라 갈라 둔 것들이다
// (CONTRIBUTING 「어느 말로 쓰나」). 하나라도 빠뜨리면 그 파일만 관문 밖이 된다.
var extraGo = []string{
	"pkg/inventory/ui/text.go",
	"pkg/inventory/ui/text_more.go",
	"pkg/inventory/ui/text_msg.go",
	"pkg/inventory/ui/text_refusal.go",
}

// extraHTML — 같은 이유로 적어 두는 HTML. 리포 소개 페이지다.
var extraHTML = []string{"site/index.html"}

const (
	rulesFile    = "tools/checkprose/rules.tsv"
	baselineFile = "tools/checkprose/baseline.tsv"
	overlapFile  = "tools/checkprose/overlap.txt"
)

// overlap — 규칙이 잘못 잡는 말. 재기 전에 같은 길이로 덮는다. 「헷갈리다」의 "갈리"가
// 「갈리다」 규칙에 걸리는 것이 실제로 나온 자리라, 뒤보기 없는 RE2 에서는 이 편이 낫다.
//
// 말 자체는 overlap.txt 에 있다. 여기 적으면 tools/checktext 가 막는다(Go 문자열의 한글).
var overlap []string

type rule struct {
	name string
	re   *regexp.Regexp
	fix  string
}

type hit struct {
	rule string
	file string
	line int
	text string
}

func main() {
	list := flag.Bool("list", false, "print every hit with its line")
	write := flag.Bool("baseline", false, "rewrite the baseline file")
	flag.Parse()

	rules, err := loadRules(rulesFile)
	if err != nil {
		fail(err)
	}
	overlap, err = loadWords(overlapFile)
	if err != nil {
		fail(err)
	}
	hits, err := scan(".", rules)
	if err != nil {
		fail(err)
	}
	if *list {
		printList(hits)
	}
	counts := tally(hits)

	if *write {
		if err := writeBaseline(baselineFile, counts); err != nil {
			fail(err)
		}
		fmt.Printf("✓ baseline rewritten: %d entries, %d hits\n", len(counts), len(hits))
		return
	}

	base, err := readBaseline(baselineFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ no usable baseline:", err)
		fmt.Fprintln(os.Stderr, "  run: go run ./tools/checkprose -baseline")
		os.Exit(1)
	}
	grown, shrunk := compare(base, counts)
	if len(grown) > 0 {
		fmt.Fprintln(os.Stderr, "✗ prose gate: these grew past the baseline")
		for _, l := range grown {
			fmt.Fprintln(os.Stderr, "   ", l)
		}
		fmt.Fprintln(os.Stderr, "\nSee them with: go run ./tools/checkprose -list")
		fmt.Fprintln(os.Stderr, "Each rule in", rulesFile, "says what to write instead.")
		os.Exit(1)
	}
	if len(shrunk) > 0 {
		fmt.Fprintln(os.Stderr, "✗ prose gate: the baseline is stale — these went down")
		for _, l := range shrunk {
			fmt.Fprintln(os.Stderr, "   ", l)
		}
		fmt.Fprintln(os.Stderr, "\nLock the win in: go run ./tools/checkprose -baseline")
		os.Exit(1)
	}
	fmt.Printf("✓ prose check passed (%d hits, all at the baseline)\n", len(hits))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "✗ checkprose:", err)
	os.Exit(1)
}

// ── 규칙 ───────────────────────────────────────────────────────────────────

func loadRules(path string) ([]rule, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []rule
	for i, line := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("%s:%d: want 3 tab-separated fields, got %d", path, i+1, len(f))
		}
		re, err := regexp.Compile(f[1])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, i+1, err)
		}
		out = append(out, rule{name: f[0], re: re, fix: f[2]})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no rules", path)
	}
	return out, nil
}

// loadWords — 한 줄에 하나씩 적은 말 목록. 빈 줄과 # 로 시작하는 줄은 넘긴다.
func loadWords(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// ── 훑기 ───────────────────────────────────────────────────────────────────

func scan(root string, rules []rule) ([]hit, error) {
	var out []hit
	seen := map[string]bool{}

	// 원문도 함께 넘긴다. 덮은 쪽으로 재고, **보여 줄 때는 원문 줄을 보여 준다**:
	// 덮인 줄을 그대로 찍으면 어느 문장인지 알아볼 수 없다. 덮기가 바이트 수를
	// 그대로 두므로 원문과 덮은 것의 자리가 어긋나지 않는다.
	add := func(rel string, orig, masked []byte) {
		if seen[rel] {
			return
		}
		seen[rel] = true
		out = append(out, match(rel, orig, masked, rules)...)
	}

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
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".md") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		add(rel, b, maskMarkdown(b))
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, rel := range extraGo {
		orig, masked, err := maskGo(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		add(rel, orig, masked)
	}
	for _, rel := range extraHTML {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		add(rel, b, maskHTML(b))
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].file != out[j].file {
			return out[i].file < out[j].file
		}
		if out[i].line != out[j].line {
			return out[i].line < out[j].line
		}
		return out[i].rule < out[j].rule
	})
	return out, nil
}

func match(rel string, orig, masked []byte, rules []rule) []hit {
	s := hideOverlap(string(masked))
	var out []hit
	for _, r := range rules {
		for _, loc := range r.re.FindAllStringIndex(s, -1) {
			// **한국어가 없는 줄은 보지 않는다.** 지침은 한국어를 명확하게 쓰라는 것이지
			// 외국어를 고치라는 것이 아니다(「동작 범위」 1항). 화면 카탈로그는 한 줄
			// 걸러 영어라, 이 선이 없으면 EN 문장의 엠대시까지 세게 된다.
			if !hasHangul(lineAt(s, loc[0])) {
				continue
			}
			out = append(out, hit{
				rule: r.name,
				file: rel,
				line: 1 + strings.Count(s[:loc[0]], "\n"),
				text: excerpt(string(orig), loc[0]),
			})
		}
	}
	return out
}

// lineAt — 그 자리가 든 한 줄.
func lineAt(s string, off int) string {
	start := strings.LastIndexByte(s[:off], '\n') + 1
	end := strings.IndexByte(s[off:], '\n')
	if end < 0 {
		return s[start:]
	}
	return s[start : off+end]
}

func hasHangul(s string) bool {
	for _, r := range s {
		if r >= 0xAC00 && r <= 0xD7A3 {
			return true
		}
	}
	return false
}

// hideOverlap — 잘못 잡는 말을 **같은 바이트 수로** 덮는다. 자리가 밀리면 줄 번호가
// 어긋나므로 지우지 않는다.
func hideOverlap(s string) string {
	for _, w := range overlap {
		s = strings.ReplaceAll(s, w, strings.Repeat("◌", len([]rune(w))))
	}
	return s
}

// excerpt — 걸린 자리가 어느 문장인지 알아볼 만큼만 그 줄에서 떼어 온다.
func excerpt(s string, off int) string {
	start := strings.LastIndexByte(s[:off], '\n') + 1
	end := strings.IndexByte(s[off:], '\n')
	if end < 0 {
		end = len(s)
	} else {
		end += off
	}
	line := strings.TrimSpace(s[start:end])
	r := []rune(line)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return line
}

// ── 덮기 ───────────────────────────────────────────────────────────────────
//
// 덮는 자리는 전부 **공백으로 바꾼다.** 줄바꿈은 그대로 두어야 줄 번호가 맞는다.

var (
	fence     = regexp.MustCompile("(?m)^\\s*(```|~~~)")
	inlineMD  = regexp.MustCompile("`[^`\n]*`")
	htmlBlock = regexp.MustCompile(htmlBlockPattern())
)

// htmlBlockPattern — 여는 태그와 닫는 태그를 짝지어야 하는데 RE2 에는 역참조가 없다.
// 그래서 태그마다 따로 적어 이어 붙인다.
func htmlBlockPattern() string {
	parts := make([]string, 0, 4)
	for _, t := range []string{"code", "pre", "script", "style"} {
		parts = append(parts, `<`+t+`\b[^>]*>.*?</\s*`+t+`\s*>`)
	}
	return `(?is)` + strings.Join(parts, "|")
}

func maskMarkdown(b []byte) []byte {
	lines := strings.Split(string(b), "\n")
	in := false
	for i, l := range lines {
		if fence.MatchString(l) {
			in = !in
			lines[i] = blank(l)
			continue
		}
		if in {
			lines[i] = blank(l)
			continue
		}
		lines[i] = inlineMD.ReplaceAllStringFunc(l, blank)
	}
	return []byte(strings.Join(lines, "\n"))
}

func maskHTML(b []byte) []byte {
	return []byte(htmlBlock.ReplaceAllStringFunc(string(b), keepNewlines))
}

// maskGo — 문자열 리터럴만 남기고 나머지를 덮는다. 주석은 한국어이므로 보지 않는다.
// 원문과 덮은 것을 함께 돌려준다.
func maskGo(path string) (orig, masked []byte, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, b, 0)
	if err != nil {
		return nil, nil, err
	}
	out := []byte(keepNewlines(string(b)))
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		from := fset.Position(lit.Pos()).Offset
		to := fset.Position(lit.End()).Offset
		if from < 0 || to > len(out) {
			return true
		}
		copy(out[from:to], b[from:to])
		return true
	})
	return b, out, nil
}

func blank(s string) string {
	return strings.Repeat(" ", len(s))
}

func keepNewlines(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c != '\n' {
			out[i] = ' '
		}
	}
	return string(out)
}

// ── 기준선 ─────────────────────────────────────────────────────────────────

type key struct {
	rule string
	file string
}

func tally(hits []hit) map[key]int {
	out := map[key]int{}
	for _, h := range hits {
		out[key{h.rule, h.file}]++
	}
	return out
}

func writeBaseline(path string, counts map[key]int) error {
	var b strings.Builder
	b.WriteString("# checkprose baseline. count<TAB>rule<TAB>file\n")
	b.WriteString("# Rewrite with: go run ./tools/checkprose -baseline\n")
	for _, k := range sortedKeys(counts) {
		fmt.Fprintf(&b, "%d\t%s\t%s\n", counts[k], k.rule, k.file)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func readBaseline(path string) (map[key]int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[key]int{}
	for i, line := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("%s:%d: want 3 tab-separated fields, got %d", path, i+1, len(f))
		}
		n, err := strconv.Atoi(f[0])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, i+1, err)
		}
		out[key{f[1], f[2]}] = n
	}
	return out, nil
}

// compare — 늘어난 것과 줄어든 것을 따로 돌려준다. 둘 다 관문을 막는다: 늘면 새로 들어온
// 것이고, 줄면 기준선이 낡은 것이다.
func compare(base, now map[key]int) (grown, shrunk []string) {
	all := map[key]bool{}
	for k := range base {
		all[k] = true
	}
	for k := range now {
		all[k] = true
	}
	var keys []key
	for k := range all {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].file != keys[j].file {
			return keys[i].file < keys[j].file
		}
		return keys[i].rule < keys[j].rule
	})
	for _, k := range keys {
		b, n := base[k], now[k]
		switch {
		case n > b:
			grown = append(grown, fmt.Sprintf("%s  %s  %d → %d  (+%d)", k.file, k.rule, b, n, n-b))
		case n < b:
			shrunk = append(shrunk, fmt.Sprintf("%s  %s  %d → %d  (-%d)", k.file, k.rule, b, n, b-n))
		}
	}
	return grown, shrunk
}

func sortedKeys(counts map[key]int) []key {
	var out []key
	for k := range counts {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].file != out[j].file {
			return out[i].file < out[j].file
		}
		return out[i].rule < out[j].rule
	})
	return out
}

func printList(hits []hit) {
	for _, h := range hits {
		fmt.Printf("%s:%d\t%s\t%s\n", h.file, h.line, h.rule, h.text)
	}
	fmt.Printf("%d hits\n", len(hits))
}
