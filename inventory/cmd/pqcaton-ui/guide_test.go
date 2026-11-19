// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	usageFlag  = regexp.MustCompile(`(?m)^  -([a-z][a-z-]*)`)
	inlineCode = regexp.MustCompile("`([^`]+)`")
	flagToken  = regexp.MustCompile(`(?:^|\s)-([a-z][a-z-]*)`)
)

// collectorFlags — 준비 도구 pqcaton-collectors 가 정의한 플래그 이름. 다른 main 패키지라 불러 쓸 수 없어 원본의
// `fs.String("이름", …)` 호출을 파서로 읽는다.
func collectorFlags(t *testing.T) map[string]bool {
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "pqcaton-collectors", "main.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || len(c.Args) == 0 {
			return true
		}
		sel, ok := c.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "fs" {
			return true
		}
		if lit, ok := c.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			known[strings.Trim(lit.Value, "\"")] = true
		}
		return true
	})
	if !known["arch"] || !known["out"] {
		t.Fatalf("could not read the pqcaton-collectors flags: %v", known)
	}
	return known
}

// IC-U107 — **운영자 안내서가 적은 플래그는 실제로 있다.** 두 판의 명령 블록에서 `pqcaton-ui`에 준 플래그는 `-h`가
// 찍는 목록에, `pqcaton-collectors`에 준 플래그는 그 도구가 정의한 목록에 있다. 본문의 인라인 코드로 적은 플래그는
// 둘 가운데 한쪽에 있다. 플래그 이름을 바꾸고 안내서를 두면 여기서 막힌다.
func TestOperatorGuideFlagsExist(t *testing.T) {
	var usage bytes.Buffer
	if _, err := parseStart([]string{"-h"}, &usage); err == nil {
		t.Fatal("-h did not stop")
	}
	known := map[string]bool{}
	for _, m := range usageFlag.FindAllStringSubmatch(usage.String(), -1) {
		known[m[1]] = true
	}
	if !known["workspace"] || !known["no-browser"] {
		t.Fatalf("could not read the flag list:\n%s", usage.String())
	}
	coll := collectorFlags(t)
	// 표준 flag는 -h · -help를 목록에 찍지 않고 받는다(도움말을 찍고 끝낸다). 위에서 -h가 멈추는 것을 확인했고 두 도구 모두
	// flag 패키지를 쓰므로, 둘을 있는 플래그로 본다. 빼 두면 안내서의 `pqcaton-ui -h` 같은 쓰기 좋은 확인 명령을 없는 플래그로
	// 거절해, 시험의 한계 때문에 문서를 바꾸게 된다(2026-11-19).
	for _, h := range []string{"h", "help"} {
		known[h], coll[h] = true, true
	}
	either := map[string]bool{}
	for k := range known {
		either[k] = true
	}
	for k := range coll {
		either[k] = true
	}
	type use struct{ cmd, tool string }
	for _, guide := range []string{"operator-guide.md", "operator-guide.ko.md"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", guide))
		if err != nil {
			t.Fatal(err)
		}
		var cmds []use
		inBlock := false
		for _, l := range strings.Split(string(b), "\n") {
			switch {
			case l == "```bash":
				inBlock = true
			case l == "```":
				inBlock = false
			case inBlock:
				for _, seg := range regexp.MustCompile(`&&|\|\||;|\|`).Split(l, -1) {
					switch seg = strings.TrimSpace(seg); {
					case strings.HasPrefix(seg, "pqcaton-ui"):
						cmds = append(cmds, use{seg, "pqcaton-ui"})
					case strings.HasPrefix(seg, "pqcaton-collectors"):
						cmds = append(cmds, use{seg, "pqcaton-collectors"})
					}
				}
			default:
				for _, m := range inlineCode.FindAllStringSubmatch(l, -1) {
					switch c := m[1]; {
					case strings.HasPrefix(c, "pqcaton-ui"):
						cmds = append(cmds, use{c, "pqcaton-ui"})
					case strings.HasPrefix(c, "pqcaton-collectors"):
						cmds = append(cmds, use{c, "pqcaton-collectors"})
					case strings.HasPrefix(c, "-") && c != "-next":
						cmds = append(cmds, use{c, "either"})
					}
				}
			}
		}
		seen := map[string]bool{}
		for _, c := range cmds {
			seen[c.tool] = true
			set, name := known, "pqcaton-ui"
			switch c.tool {
			case "pqcaton-collectors":
				set, name = coll, c.tool
			case "either":
				set, name = either, "neither pqcaton-ui nor pqcaton-collectors"
			}
			for _, m := range flagToken.FindAllStringSubmatch(c.cmd, -1) {
				if !set[m[1]] {
					t.Errorf("%s: %q uses -%s, which %s does not have", guide, c.cmd, m[1], name)
				}
			}
		}
		if !seen["pqcaton-ui"] || !seen["pqcaton-collectors"] {
			t.Fatalf("%s: commands of both tools expected, found %v", guide, seen)
		}
	}
}

var (
	latestNote   = regexp.MustCompile(`(?m)^## (v\d+\.\d+\.\d+): `)
	guideBase    = regexp.MustCompile(`(v\d+\.\d+\.\d+) 기준입니다|It covers (v\d+\.\d+\.\d+)\.`)
	guideInstall = regexp.MustCompile(`go install github\.com/sntsoftgit/pqcaton/[^@\s]+@(v[^\s]+)`)
	guideRaw     = regexp.MustCompile(`raw\.githubusercontent\.com/sntsoftgit/pqcaton/(v[^/\s]+)/`)
	readmeClone  = regexp.MustCompile(`git clone --branch (v[^\s]+) https://github\.com/sntsoftgit/pqcaton`)
)

// IC-U112 — **운영자 안내서의 판은 최신 릴리스와 같다.** 두 판에서 안내서가 밝힌 기준판, `go install`의 판,
// `raw.githubusercontent.com` 다운로드 주소의 판, 두 판 README의 빠른 시작이 받으라는 판(`git clone --branch`)이 모두 릴리스 노트
// 맨 위 절의 판과 같다. 새 판을 내며 안내서를
// 두면 낡은 판을 설치하게 하는 것을 여기서 막는다(v0.23.1에서 v0.23.0을 설치하게 하던 것).
func TestOperatorGuideInstallsTheLatestRelease(t *testing.T) {
	notes, err := os.ReadFile(filepath.Join("..", "..", "..", "RELEASE_NOTES.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := latestNote.FindSubmatch(notes)
	if m == nil {
		t.Fatal("no release section in RELEASE_NOTES.md")
	}
	latest := string(m[1])
	for _, guide := range []string{"operator-guide.md", "operator-guide.ko.md"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", guide))
		if err != nil {
			t.Fatal(err)
		}
		for name, re := range map[string]*regexp.Regexp{"base version": guideBase, "go install": guideInstall, "download URL": guideRaw} {
			found := re.FindAllSubmatch(b, -1)
			if len(found) == 0 {
				t.Errorf("%s: no %s found", guide, name)
			}
			for _, f := range found {
				v := string(f[1])
				if len(f) > 2 && v == "" {
					v = string(f[2])
				}
				if v != latest {
					t.Errorf("%s: the %s is %s, want the latest release %s", guide, name, v, latest)
				}
			}
		}
	}
	for _, readme := range []string{"README.md", "README.ko.md"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", readme))
		if err != nil {
			t.Fatal(err)
		}
		found := readmeClone.FindAllSubmatch(b, -1)
		if len(found) == 0 {
			t.Errorf("%s: no git clone --branch of this repository found", readme)
		}
		for _, f := range found {
			if v := string(f[1]); v != latest {
				t.Errorf("%s: git clone takes %s, want the latest release %s", readme, v, latest)
			}
		}
	}
}

var (
	readmeUp  = regexp.MustCompile(`pqcota \*\*(v\d+\.\d+\.\d+)\*\*`)
	demoUp    = regexp.MustCompile(`pqcota는 (v\d+\.\d+\.\d+)(?:으로|로) 받습니다|git clone --branch (v\d+\.\d+\.\d+) `)
	noteUp    = regexp.MustCompile("(?s)^## v[^\n]*\n.*?\n### [^\n]*\n\\s*- [^\n]*pqcota:\\s*[`*]*(v\\d+\\.\\d+\\.\\d+)")
	compatTop = regexp.MustCompile(`(?m)^\| (v\d+\.\d+\.\d+)(?: ~ (v\d+\.\d+\.\d+))? \| (v\d+\.\d+\.\d+) \|`)
	goModUp   = regexp.MustCompile(`(?m)^\s*github\.com/randyinthedev-hash/(pqcota[a-z-]*) (v\d+\.\d+\.\d+)`)
)

// IC-U144 — **판별 표(docs/compatibility.ko.md, 영어 판과의 판 번호 일치는 checkdocs가 잰다)의 맨 위 줄은 지금 판이다.** 그 줄의 pqcaton 판(범위면 끝 판)은 릴리스 노트 맨 위
// 절의 판과 같다. `go.mod`의 pqcota 계열 모듈은 모두 같은 판이고, 표의 고정한 pqcota 판은 그 판과 같다. 새 판을 내며 표를
// 두면 사용자가 옛 조합을 따르고, 모듈 하나만 다른 판이면 상류가 검증하지 않은 조합이 된다.
func TestCompatibilityTableIsCurrent(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	read := func(p string) []byte {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	note := latestNote.FindSubmatch(read("RELEASE_NOTES.md"))
	mods := goModUp.FindAllSubmatch(read("go.mod"), -1)
	top := compatTop.FindSubmatch(read(filepath.Join("docs", "compatibility.ko.md")))
	if note == nil || len(mods) == 0 || top == nil {
		t.Fatalf("could not read the latest note (%v), go.mod (%d pqcota modules) or the table's top row (%v)", note != nil, len(mods), top != nil)
	}
	pinned := string(mods[0][2])
	for _, m := range mods[1:] {
		if string(m[2]) != pinned {
			t.Errorf("go.mod pins %s %s but %s %s: the pqcota modules must share one tag", mods[0][1], pinned, m[1], m[2])
		}
	}
	pqcaton := string(top[1])
	if len(top[2]) > 0 {
		pqcaton = string(top[2])
	}
	if pqcaton != string(note[1]) {
		t.Errorf("the table's top row is pqcaton %s, but the latest release is %s", pqcaton, note[1])
	}
	if string(top[3]) != pinned {
		t.Errorf("the table's top row pins pqcota %s, but go.mod requires %s", top[3], pinned)
	}
	// 판을 정하는 자리들도 같은 판을 적는다.
	// README는 맨 위 소개 · 사전 준비 · 맞는 조합 세 곳에서 판을 밝힌다.
	readme := readmeUp.FindAllSubmatch(read("README.ko.md"), -1)
	if len(readme) < 3 {
		t.Errorf("README names the pqcota version in %d places, want the intro, the prerequisites and the matching-versions note", len(readme))
	}
	for _, m := range readme {
		if string(m[1]) != pinned {
			t.Errorf("README names pqcota %s, but go.mod requires %s", m[1], pinned)
		}
	}
	demo := demoUp.FindAllSubmatch(read(filepath.Join("demo", "README.ko.md")), -1)
	if len(demo) < 2 {
		t.Errorf("the demo guide does not name the pqcota version and the clone tag")
	}
	for _, m := range demo {
		if v := string(m[1]) + string(m[2]); v != pinned {
			t.Errorf("the demo guide names pqcota %s, but go.mod requires %s", v, pinned)
		}
	}
	// v0.25.0 뒤의 절은 첫 소제목(「받는 쪽이 달라지는 것」)의 첫 항목에 함께 쓰는 pqcota를 적는다(그 전 절은 소급하지 않는다).
	// 영어 노트와 한국어 노트 둘 다 본다(v0.28.0부터 두 언어).
	if string(note[1]) != "v0.25.0" {
		for _, f := range []string{"RELEASE_NOTES.md", "RELEASE_NOTES.ko.md"} {
			notes := read(f)
			top := notes[latestNote.FindIndex(notes)[0]:]
			if m := noteUp.FindSubmatch(top); m == nil || string(m[1]) != pinned {
				t.Errorf("%s: the first item of the latest release section does not name `pqcota: %s`", f, pinned)
			}
		}
	}
}
