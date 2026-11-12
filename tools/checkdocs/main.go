// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// Command checkdocs — 문서 관문(`make check-docs`, `make` 전체에 든다). 문서가 두 언어로 갈리고 자리를 옮겨도 서로 어긋나거나
// 끊어지지 않게 한다.
//
// 재는 것은 넷이다.
//
//  1. **링크.** 리포가 추적하는 모든 md의 상대 링크가 있는 파일을 가리키고, `#앵커`는 그 문서의 실제 제목이다(GitHub의 앵커
//     규칙: 소문자, 글자 · 숫자 · `-` · `_`만 남기고 공백 하나를 `-` 하나로, 같은 제목은 `-1` · `-2`).
//
//  2. **짝.** 영어 문서 `x.md`와 한국어 문서 `x.ko.md`는 짝으로 있어야 한다. 짝이 없어도 되는 문서는 single.txt에 그 문서의
//     말(en · ko)과 까닭을 함께 적는다. 짝의 첫 줄은 서로를 가리키는 언어 전환 줄이다(langs.tsv의 이름으로 `English · [한국어](x.ko.md)` /
//     `[English](x.md) · 한국어`).
//
//  3. **짝의 내용.** 두 판이 말하는 사실이 같아야 한다: 판 번호(`v1.2.3`)의 집합, 케이스 ID(`IC-…` · `RUN-…` · `CP-…`)의 집합,
//     셸 코드 블록의 명령 줄(주석 줄을 빼고 `<…>` 자리 표시를 하나로 본다)의 차례, 링크가 가리키는 파일의 집합(한국어 판 이름은
//     영어 판 이름으로 보고 앵커는 뺀다), 수준별 제목의 수. md 안의 `<a id="…">`는 앵커로 센다(옮긴 제목의 호환 앵커).
//
//  4. **언어 일관.** 한국어 문서는 짝이 있는 문서를 한국어 판으로, 영어 문서는 영어 판으로 가리킨다. 다른 말의 판을 일부러
//     가리키는 링크(「영문본은 [CLA.md](CLA.md)」)는 링크 글자가 언어 이름이거나 그 파일 이름일 때 받는다. 언어 전환 줄은 빼고 본다.
//
//  5. **사이트.** Pages는 `docs/`를 그대로 올린다. `docs/`의 .html 페이지에서 `href` · `src`의 상대 경로는 `docs/` 안의 있는 파일을
//     가리키고(밖을 가리키면 Pages 산출물에서 끊긴다), `#앵커`는 그 페이지의 `id`이며, 이 리포의 `blob|tree/main/…` 절대 URL은 있는
//     경로(md면 앵커까지)를 가리킨다. md 안의 같은 절대 URL도 잰다.
//
//  6. **케이스 표.** tables.txt에 적은 짝(영어판 경로)은 케이스 행을 ID로 짝지어 견준다: 첫 칸(ID · 코드 링크 · 상태 표시)이 바이트까지
//     같고, 열 개수와 행 안의 링크 대상(한국어 판 이름은 영어 판 이름으로 보고 앵커는 뺀다)이 같으며, 케이스의 차례가 같아야 한다.
//     한쪽에만 있는 케이스와 한 판 안의 중복 ID는 막는다. 제목의 수준별 개수도 같아야 한다. 케이스는 케이스 관문(checkcases)이
//     영어판에서 한 번만 세고, 여기서는 두 판이 같은지만 본다. `-sync-cases`는 케이스 관문이 영어판의 링크를 다시 찍은 뒤 한국어판의
//     첫 칸을 영어판대로 맞춘다.
//
// 영어판이 한국어 원문의 **요약**인 짝은 summary.txt에 적는다. 그 짝은 내용(판 번호 · 케이스 ID · 셸 명령)을 견주지 않는 대신, 영어판
// 앞쪽(제목 뒤 열 줄 안)에 요약임을 밝히는 말(summary)이 있어야 한다.
//
// 문자열의 한국어는 코드에 두지 않는다(check-text). 언어 이름은 langs.tsv에서 읽는다.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

func main() {
	dir := flag.String("dir", "tools/checkdocs", "folder holding langs.tsv, single.txt, summary.txt and tables.txt")
	sync := flag.Bool("sync-cases", false, "copy the first cell of every case row from the English edition to the Korean one (after checkcases -write), then check")
	flag.Parse()
	files, err := tracked()
	if err != nil {
		fmt.Fprintln(os.Stderr, "checkdocs:", err)
		os.Exit(2)
	}
	cfg, err := loadConfig(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "checkdocs:", err)
		os.Exit(2)
	}
	if *sync {
		for en := range cfg.tables {
			n, err := syncCases(en, koOf(en))
			if err != nil {
				fmt.Fprintln(os.Stderr, "checkdocs:", err)
				os.Exit(2)
			}
			fmt.Printf("synced %d case rows of %s from %s\n", n, koOf(en), en)
		}
	}
	problems := checkIn(dirFS{"."}, files, cfg)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "  "+p)
		}
		fmt.Fprintf(os.Stderr, "✗ docs check: %d problem(s)\n", len(problems))
		os.Exit(1)
	}
	fmt.Printf("✓ docs check passed (%d documents, %d pairs, links and anchors resolve)\n", countMD(files), countPairs(files))
}

// tracked — 리포가 추적하는 파일(리포 루트 기준, / 구분).
func tracked() ([]string, error) {
	out, err := exec.Command("git", "ls-files", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []string
	for _, f := range bytes.Split(out, []byte{0}) {
		if len(f) > 0 {
			files = append(files, string(f))
		}
	}
	return files, nil
}

// config — langs.tsv(언어 이름)와 single.txt(짝이 없어도 되는 문서).
type config struct {
	en, ko string
	single map[string]bool
	// koSingle — single.txt에 ko로 적은 문서(짝 없이 한국어로 있는 것).
	koSingle map[string]bool
	// alias — 언어 이름 말고도 다른 말의 판을 일부러 가리킨다고 보는 링크 글자(langs.tsv의 alias).
	alias map[string]bool
	// summary — 영어판이 요약인 짝(summary.txt, 영어판 경로).
	summary map[string]bool
	// tables — 케이스 표를 행 단위로 견주는 짝(tables.txt, 영어판 경로).
	tables map[string]bool
}

func loadConfig(dir string) (config, error) {
	c := config{single: map[string]bool{}, koSingle: map[string]bool{}, alias: map[string]bool{}, summary: map[string]bool{}, tables: map[string]bool{}}
	b, err := os.ReadFile(filepath.Join(dir, "langs.tsv"))
	if err != nil {
		return c, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "\t")
		switch {
		case !ok || strings.HasPrefix(line, "#"):
		case k == "en":
			c.en = v
		case k == "ko":
			c.ko = v
		case k == "alias":
			c.alias[v] = true
		}
	}
	if c.en == "" || c.ko == "" {
		return c, fmt.Errorf("langs.tsv needs an en and a ko line")
	}
	s, err := os.ReadFile(filepath.Join(dir, "single.txt"))
	if err != nil {
		return c, err
	}
	for _, line := range strings.Split(string(s), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if len(f) < 2 || (f[1] != "en" && f[1] != "ko") {
			return c, fmt.Errorf("single.txt: %q needs its language (en or ko) after the path", f[0])
		}
		c.single[f[0]] = true
		c.koSingle[f[0]] = f[1] == "ko"
	}
	for name, into := range map[string]map[string]bool{"summary.txt": c.summary, "tables.txt": c.tables} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return c, err
		}
		for _, line := range strings.Split(string(b), "\n") {
			if f := strings.Fields(line); len(f) > 0 && !strings.HasPrefix(f[0], "#") {
				into[f[0]] = true
			}
		}
	}
	return c, nil
}

func isMD(f string) bool { return strings.HasSuffix(f, ".md") }

func countMD(files []string) int {
	n := 0
	for _, f := range files {
		if isMD(f) {
			n++
		}
	}
	return n
}

func countPairs(files []string) int {
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f, ".ko.md") {
			n++
		}
	}
	return n
}

// koOf · enOf — 짝의 경로.
func koOf(en string) string { return strings.TrimSuffix(en, ".md") + ".ko.md" }
func enOf(ko string) string { return strings.TrimSuffix(ko, ".ko.md") + ".md" }

var (
	linkRe    = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	headingRe = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	versionRe = regexp.MustCompile(`\bv\d+\.\d+\.\d+\b`)
	caseRe    = regexp.MustCompile(`\b(?:IC|RUN|CP)-[A-Z]*\d+\b`)
	holderRe  = regexp.MustCompile(`<[^<>]*>`)
	fenceRe   = regexp.MustCompile("^\\s*```")
	inlineRe  = regexp.MustCompile("`[^`]*`")
	mdLinkRe  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// doc — 읽은 md 하나.
type doc struct {
	lines    []string
	anchors  map[string]bool
	versions map[string]bool
	cases    map[string]bool
	commands []string
}

func readDoc(fsys readFS, name string) (*doc, error) {
	b, err := fsys.ReadFile(name)
	if err != nil {
		return nil, err
	}
	d := &doc{anchors: map[string]bool{}, versions: map[string]bool{}, cases: map[string]bool{}}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	seen := map[string]int{}
	inFence, shell := false, false
	for sc.Scan() {
		line := sc.Text()
		d.lines = append(d.lines, line)
		if fenceRe.MatchString(line) {
			if !inFence {
				lang := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "`"))
				shell = lang == "bash" || lang == "sh" || lang == "shell" || lang == "console"
			}
			inFence = !inFence
			continue
		}
		for _, v := range versionRe.FindAllString(line, -1) {
			d.versions[v] = true
		}
		for _, c := range caseRe.FindAllString(line, -1) {
			d.cases[c] = true
		}
		if inFence {
			if shell {
				if c := commandLine(line); c != "" {
					d.commands = append(d.commands, c)
				}
			}
			continue
		}
		for _, m := range idRe.FindAllStringSubmatch(line, -1) {
			d.anchors[m[1]] = true
		}
		if m := headingRe.FindStringSubmatch(line); m != nil {
			a := slug(m[2])
			if n := seen[a]; n > 0 {
				d.anchors[fmt.Sprintf("%s-%d", a, n)] = true
			} else {
				d.anchors[a] = true
			}
			seen[a]++
		}
	}
	return d, sc.Err()
}

// commandLine — 셸 블록의 한 줄을 견줄 꼴로. 빈 줄과 주석 줄은 빼고, 줄 끝 주석(` # …`)을 떼고, `<…>` 자리 표시를 `<>`로 본다.
func commandLine(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return ""
	}
	if i := strings.Index(t, " # "); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	t = holderRe.ReplaceAllString(t, "<>")
	return strings.Join(strings.Fields(t), " ")
}

// slug — GitHub의 제목 앵커.
func slug(text string) string {
	t := mdLinkRe.ReplaceAllString(text, "$1")
	t = strings.NewReplacer("`", "", "**", "", "__", "").Replace(t)
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(t)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// readFS — 시험이 바꿔 끼우는 파일 자리.
type readFS interface {
	ReadFile(name string) ([]byte, error)
}

type dirFS struct{ root string }

func (d dirFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(filepath.Join(d.root, name)) }

// checkIn — 네 가지를 재고 문제를 돌려준다(정렬).
func checkIn(fsys readFS, files []string, cfg config) []string {
	have := map[string]bool{}
	dirs := map[string]bool{}
	for _, f := range files {
		have[f] = true
		for d := path.Dir(f); d != "." && d != "/"; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	docs := map[string]*doc{}
	var problems []string
	add := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }
	for _, f := range files {
		if !isMD(f) {
			continue
		}
		d, err := readDoc(fsys, f)
		if err != nil {
			add("%s: cannot be read: %v", f, err)
			continue
		}
		docs[f] = d
	}
	for f, d := range docs {
		isKoFile := strings.HasSuffix(f, ".ko.md")
		isKo := isKoFile || cfg.koSingle[f]
		// 2. 짝과 언어 전환 줄
		var pair string
		if isKoFile {
			pair = enOf(f)
		} else {
			pair = koOf(f)
		}
		_, hasPair := docs[pair]
		switch {
		case hasPair:
			want := cfg.en + " · [" + cfg.ko + "](" + path.Base(pair) + ")"
			if isKoFile {
				want = "[" + cfg.en + "](" + path.Base(pair) + ") · " + cfg.ko
			}
			if len(d.lines) == 0 || strings.TrimSpace(d.lines[0]) != want {
				add("%s: the first line must be the language line %q", f, want)
			}
		case isKoFile:
			add("%s: the English page %s is missing", f, pair)
		case !cfg.single[f]:
			add("%s: has no Korean page %s and is not listed in single.txt", f, pair)
		}
		// 1 · 4. 링크(여러 줄에 걸친 링크는 이어 읽는다)
		for _, ll := range logicalLines(d.lines) {
			i, plain := ll.n, inlineRe.ReplaceAllString(ll.text, "")
			for _, m := range linkRe.FindAllStringSubmatch(plain, -1) {
				target := m[2]
				if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
					continue
				}
				file, anchor, _ := strings.Cut(target, "#")
				resolved := f
				if file != "" {
					resolved = path.Clean(path.Join(path.Dir(f), file))
				}
				if file != "" && !have[resolved] && !dirs[strings.TrimSuffix(resolved, "/")] {
					add("%s:%d: the link %q points to nothing (%s)", f, i+1, target, resolved)
					continue
				}
				if anchor != "" && isMD(resolved) {
					if td, ok := docs[resolved]; ok && !td.anchors[anchor] {
						add("%s:%d: the anchor #%s is not a heading of %s", f, i+1, anchor, resolved)
					}
				}
				// 다른 말의 판을 일부러 가리키는 링크: 글자가 언어 이름이거나 그 파일 이름이다.
				text := m[1]
				if i == 0 || file == "" || !isMD(resolved) || text == cfg.en || text == cfg.ko || cfg.alias[text] || text == path.Base(resolved) || text == file {
					continue
				}
				switch {
				case isKo && !strings.HasSuffix(resolved, ".ko.md") && docs[koOf(resolved)] != nil:
					add("%s:%d: link the Korean page %s", f, i+1, koOf(resolved))
				case !isKo && strings.HasSuffix(resolved, ".ko.md") && docs[enOf(resolved)] != nil:
					add("%s:%d: link the English page %s", f, i+1, enOf(resolved))
				}
			}
		}
		// 3. 짝의 내용(영어 쪽에서 한 번만). 요약판은 견주지 않고, 요약임을 밝혔는지만 본다.
		if hasPair && !isKoFile && cfg.summary[f] {
			if !saysSummary(d.lines) {
				add("%s: listed in summary.txt but does not say near the top that it is a summary", f)
			}
		} else if hasPair && !isKoFile {
			ko := docs[pair]
			if a, b := diffSet(d.versions, ko.versions); a != "" || b != "" {
				add("%s ↔ %s: versions differ (only in English: %s; only in Korean: %s)", f, pair, a, b)
			}
			if a, b := diffSet(d.cases, ko.cases); a != "" || b != "" {
				add("%s ↔ %s: case ids differ (only in English: %s; only in Korean: %s)", f, pair, a, b)
			}
			if i := firstDiff(d.commands, ko.commands); i >= 0 {
				add("%s ↔ %s: shell commands differ at command %d (%q vs %q)", f, pair, i+1, at(d.commands, i), at(ko.commands, i))
			}
			if a, b := diffSet(fileLinks(d.lines), fileLinks(ko.lines)); a != "" || b != "" {
				add("%s ↔ %s: linked files differ (only in English: %s; only in Korean: %s)", f, pair, a, b)
			}
			eh, kh := headingLevels(d.lines), headingLevels(ko.lines)
			for lv := 1; lv <= 6; lv++ {
				if eh[lv] != kh[lv] {
					add("%s ↔ %s: %d level-%d headings vs %d", f, pair, eh[lv], lv, kh[lv])
				}
			}
		}
	}
	for en := range cfg.tables {
		if docs[en] == nil || docs[koOf(en)] == nil {
			problems = append(problems, fmt.Sprintf("%s: listed in tables.txt but the pair %s · %s is not there", en, en, koOf(en)))
			continue
		}
		problems = append(problems, compareTables(en, docs[en].lines, koOf(en), docs[koOf(en)].lines)...)
	}
	problems = append(problems, checkPages(fsys, files, have, dirs)...)
	problems = append(problems, checkRepoURLs(fsys, files, have, dirs, docs)...)
	sort.Strings(problems)
	return problems
}

// saysSummary — 제목 뒤 열 줄 안에 「summary」가 있나.
func saysSummary(lines []string) bool {
	seen := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "# ") {
			seen = 1
			continue
		}
		if seen > 0 {
			if strings.Contains(strings.ToLower(l), "summary") {
				return true
			}
			if seen++; seen > 10 {
				return false
			}
		}
	}
	return false
}

// repoURL — 이 리포의 main을 가리키는 절대 URL.
var repoURL = regexp.MustCompile(`https://github\.com/sntsoftgit/pqcaton/(?:blob|tree)/main/([^\s)"'#<>]+)(?:#([^\s)"'<>]*))?`)

// checkRepoURLs — md · html 안의 이 리포 main 절대 URL이 있는 경로(md면 앵커까지)를 가리키나.
func checkRepoURLs(fsys readFS, files []string, have, dirs map[string]bool, docs map[string]*doc) []string {
	var out []string
	for _, f := range files {
		if !isMD(f) && !isPage(f) {
			continue
		}
		b, err := fsys.ReadFile(f)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, m := range repoURL.FindAllStringSubmatch(line, -1) {
				p := strings.TrimSuffix(m[1], "/")
				if !have[p] && !dirs[p] {
					out = append(out, fmt.Sprintf("%s:%d: %s points to %s, which is not in the repository", f, i+1, m[0], p))
					continue
				}
				if m[2] != "" && isMD(p) {
					if td, ok := docs[p]; ok && !td.anchors[m[2]] {
						out = append(out, fmt.Sprintf("%s:%d: the anchor #%s is not a heading of %s", f, i+1, m[2], p))
					}
				}
			}
		}
	}
	return out
}

func isPage(f string) bool { return strings.HasPrefix(f, "docs/") && strings.HasSuffix(f, ".html") }

var (
	attrRe = regexp.MustCompile(`\b(?:href|src)="([^"]*)"`)
	idRe   = regexp.MustCompile(`\b(?:id|name)="([^"]*)"`)
)

// checkPages — Pages 산출물(docs/)의 페이지. 상대 경로는 docs/ 안의 있는 파일, #앵커는 그 페이지의 id다.
func checkPages(fsys readFS, files []string, have, dirs map[string]bool) []string {
	var out []string
	for _, f := range files {
		if !isPage(f) {
			continue
		}
		b, err := fsys.ReadFile(f)
		if err != nil {
			out = append(out, fmt.Sprintf("%s: cannot be read: %v", f, err))
			continue
		}
		ids := map[string]bool{}
		for _, m := range idRe.FindAllStringSubmatch(string(b), -1) {
			ids[m[1]] = true
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, m := range attrRe.FindAllStringSubmatch(line, -1) {
				v := m[1]
				switch {
				case v == "" || strings.Contains(v, "://") || strings.HasPrefix(v, "//") || strings.HasPrefix(v, "mailto:") ||
					strings.HasPrefix(v, "data:") || strings.HasPrefix(v, "javascript:"):
					continue
				case strings.HasPrefix(v, "#"):
					if len(v) > 1 && !ids[v[1:]] {
						out = append(out, fmt.Sprintf("%s:%d: the anchor %s is not an id on this page", f, i+1, v))
					}
					continue
				}
				file, _, _ := strings.Cut(v, "#")
				file, _, _ = strings.Cut(file, "?")
				resolved := path.Clean(path.Join(path.Dir(f), file))
				if file == "" || file == "./" || file == "." {
					resolved = path.Dir(f)
				}
				switch {
				case resolved != "docs" && !strings.HasPrefix(resolved, "docs/"):
					out = append(out, fmt.Sprintf("%s:%d: %q leaves docs/, so it breaks on the Pages site", f, i+1, v))
				case !have[resolved] && !dirs[resolved] && !have[path.Join(resolved, "index.html")]:
					out = append(out, fmt.Sprintf("%s:%d: %q points to nothing (%s)", f, i+1, v, resolved))
				}
			}
		}
	}
	return out
}

func diffSet(a, b map[string]bool) (onlyA, onlyB string) {
	var x, y []string
	for k := range a {
		if !b[k] {
			x = append(x, k)
		}
	}
	for k := range b {
		if !a[k] {
			y = append(y, k)
		}
	}
	sort.Strings(x)
	sort.Strings(y)
	return strings.Join(x, " "), strings.Join(y, " ")
}

func firstDiff(a, b []string) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		if at(a, i) != at(b, i) {
			return i
		}
	}
	return -1
}

func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "(none)"
}

// caseRow — 케이스 표의 한 행.
type caseRow struct {
	id, first string
	cells     int
	links     string
	line      int
}

// caseCell — 케이스 행의 첫 칸에서 ID를 읽는다(`**[IC-R1](…) ✅**` · `[IC-R1](…) ✅` · `IC-R1 ⏳`).
var caseCell = regexp.MustCompile(`^\s*(?:\*\*)?\[?((?:IC|RUN|CP)-[A-Z]*\d+)\b`)

// splitRow — 표의 한 줄을 칸으로 나눈다(인라인 코드 안의 `|`는 나누지 않는다).
func splitRow(line string) []string {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "|") {
		return nil
	}
	t = strings.TrimSuffix(strings.TrimPrefix(t, "|"), "|")
	var cells []string
	var b strings.Builder
	code := false
	for i := 0; i < len(t); i++ {
		switch {
		case t[i] == '`':
			code = !code
			b.WriteByte(t[i])
		case t[i] == '\\' && i+1 < len(t) && t[i+1] == '|':
			b.WriteString("\\|")
			i++
		case t[i] == '|' && !code:
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(t[i])
		}
	}
	return append(cells, strings.TrimSpace(b.String()))
}

// rowLinks — 행 안의 링크 대상. 한국어 판 이름은 영어 판 이름으로, 앵커는 뺀다. 정렬해 잇는다.
func rowLinks(line string) string {
	var out []string
	for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
		t, _, _ := strings.Cut(m[2], "#")
		out = append(out, strings.Replace(t, ".ko.md", ".md", 1))
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

func caseRows(lines []string) ([]caseRow, map[string]int) {
	var rows []caseRow
	count := map[string]int{}
	for i, l := range lines {
		cells := splitRow(l)
		if len(cells) == 0 {
			continue
		}
		m := caseCell.FindStringSubmatch(cells[0])
		if m == nil {
			continue
		}
		rows = append(rows, caseRow{id: m[1], first: cells[0], cells: len(cells), links: rowLinks(l), line: i + 1})
		count[m[1]]++
	}
	return rows, count
}

func headingLevels(lines []string) map[int]int {
	out := map[int]int{}
	inFence := false
	for _, l := range lines {
		if fenceRe.MatchString(l) {
			inFence = !inFence
			continue
		}
		if m := headingRe.FindStringSubmatch(l); m != nil && !inFence {
			out[len(m[1])]++
		}
	}
	return out
}

// compareTables — 두 판의 케이스 표를 행 단위로 견준다.
func compareTables(en string, enLines []string, ko string, koLines []string) []string {
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }
	er, ec := caseRows(enLines)
	kr, kc := caseRows(koLines)
	for id, n := range ec {
		if n > 1 {
			add("%s: the case %s appears %d times", en, id, n)
		}
		if kc[id] == 0 {
			add("%s: the case %s is missing from %s", en, id, ko)
		}
	}
	for id, n := range kc {
		if n > 1 {
			add("%s: the case %s appears %d times", ko, id, n)
		}
		if ec[id] == 0 {
			add("%s: the case %s is missing from %s", ko, id, en)
		}
	}
	koByID := map[string]caseRow{}
	for _, r := range kr {
		koByID[r.id] = r
	}
	for _, r := range er {
		k, ok := koByID[r.id]
		if !ok {
			continue
		}
		switch {
		case r.first != k.first:
			add("%s:%d ↔ %s:%d: the first cell of %s differs (%q vs %q)", en, r.line, ko, k.line, r.id, r.first, k.first)
		case r.cells != k.cells:
			add("%s:%d ↔ %s:%d: %s has %d cells vs %d", en, r.line, ko, k.line, r.id, r.cells, k.cells)
		case r.links != k.links:
			add("%s:%d ↔ %s:%d: the links in the row of %s differ (%s vs %s)", en, r.line, ko, k.line, r.id, r.links, k.links)
		}
	}
	if len(out) == 0 {
		for i := range er {
			if i < len(kr) && er[i].id != kr[i].id {
				add("%s ↔ %s: the case order differs at %s (English) vs %s (Korean)", en, ko, er[i].id, kr[i].id)
				break
			}
		}
	}
	return out
}

// syncCases — 영어판 케이스 행의 첫 칸을 한국어판의 같은 ID 행에 옮겨 쓴다. 나머지 칸은 그대로다.
func syncCases(en, ko string) (int, error) {
	eb, err := os.ReadFile(en)
	if err != nil {
		return 0, err
	}
	kb, err := os.ReadFile(ko)
	if err != nil {
		return 0, err
	}
	first := map[string]string{}
	for _, l := range strings.Split(string(eb), "\n") {
		if cells := splitRow(l); len(cells) > 0 {
			if m := caseCell.FindStringSubmatch(cells[0]); m != nil {
				first[m[1]] = cells[0]
			}
		}
	}
	lines := strings.Split(string(kb), "\n")
	n := 0
	for i, l := range lines {
		cells := splitRow(l)
		if len(cells) == 0 {
			continue
		}
		m := caseCell.FindStringSubmatch(cells[0])
		if m == nil || first[m[1]] == "" || first[m[1]] == cells[0] {
			continue
		}
		j := strings.Index(l, cells[0])
		lines[i] = l[:j] + first[m[1]] + l[j+len(cells[0]):]
		n++
	}
	return n, os.WriteFile(ko, []byte(strings.Join(lines, "\n")), 0o644)
}

// fileLinks — 코드 밖 링크가 가리키는 리포 안 파일의 집합(바깥 URL은 뺀다). 한국어 판 이름은 영어 판 이름으로 보고 앵커는 뺀다. 첫 줄(언어 전환 줄)과 같은
// 문서 안 앵커(`#…`)는 뺀다.
func fileLinks(lines []string) map[string]bool {
	out := map[string]bool{}
	for _, ll := range logicalLines(lines) {
		if ll.n == 0 {
			continue
		}
		for _, m := range linkRe.FindAllStringSubmatch(inlineRe.ReplaceAllString(ll.text, ""), -1) {
			t, _, _ := strings.Cut(m[2], "#")
			if t == "" || strings.Contains(t, "://") || strings.HasPrefix(t, "mailto:") {
				continue
			}
			out[strings.Replace(t, ".ko.md", ".md", 1)] = true
		}
	}
	return out
}

// logicalLine — 코드 블록 밖의 줄. 링크 글자가 줄을 넘기면(「[」가 닫히지 않음) 다음 줄과 이어 하나로 본다. n은 첫 줄의 번호(0부터)다.
type logicalLine struct {
	n    int
	text string
}

func logicalLines(lines []string) []logicalLine {
	var out []logicalLine
	inFence := false
	for i := 0; i < len(lines); i++ {
		if fenceRe.MatchString(lines[i]) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		text, n := lines[i], i
		for j := 0; j < 3 && i+1 < len(lines) && strings.Count(inlineRe.ReplaceAllString(text, ""), "[") > strings.Count(inlineRe.ReplaceAllString(text, ""), "]") &&
			strings.TrimSpace(lines[i+1]) != "" && !fenceRe.MatchString(lines[i+1]); j++ {
			i++
			text += " " + strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), ">"))
		}
		out = append(out, logicalLine{n: n, text: text})
	}
	return out
}
