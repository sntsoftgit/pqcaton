// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
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

// IC-U107 — **운영자 안내서가 적은 플래그는 실제로 있다.** 두 판의 명령 블록에서 `pqcaton-ui`에 준 플래그와, 본문의
// 인라인 코드로 적은 플래그가 모두 `-h`가 찍는 목록에 있다. 플래그 이름을 바꾸고 안내서를 두면 여기서 막힌다.
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
	for _, guide := range []string{"operator-guide.md", "operator-guide.en.md"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", guide))
		if err != nil {
			t.Fatal(err)
		}
		var cmds []string
		inBlock := false
		for _, l := range strings.Split(string(b), "\n") {
			switch {
			case l == "```bash":
				inBlock = true
			case l == "```":
				inBlock = false
			case inBlock:
				for _, seg := range regexp.MustCompile(`&&|\|\||;|\|`).Split(l, -1) {
					if seg = strings.TrimSpace(seg); strings.HasPrefix(seg, "pqcaton-ui") {
						cmds = append(cmds, seg)
					}
				}
			default:
				for _, m := range inlineCode.FindAllStringSubmatch(l, -1) {
					if c := m[1]; strings.HasPrefix(c, "pqcaton-ui") || (strings.HasPrefix(c, "-") && c != "-next") {
						cmds = append(cmds, c)
					}
				}
			}
		}
		if len(cmds) == 0 {
			t.Fatalf("%s: no pqcaton-ui commands found", guide)
		}
		for _, c := range cmds {
			for _, m := range flagToken.FindAllStringSubmatch(c, -1) {
				if !known[m[1]] {
					t.Errorf("%s: %q uses -%s, which pqcaton-ui does not have", guide, c, m[1])
				}
			}
		}
	}
}

var (
	latestNote   = regexp.MustCompile(`(?m)^## (v\d+\.\d+\.\d+): `)
	guideBase    = regexp.MustCompile(`(v\d+\.\d+\.\d+) 기준입니다|It covers (v\d+\.\d+\.\d+)\.`)
	guideInstall = regexp.MustCompile(`go install github\.com/sntsoftgit/pqcaton/[^@\s]+@(v[^\s]+)`)
	guideRaw     = regexp.MustCompile(`raw\.githubusercontent\.com/sntsoftgit/pqcaton/(v[^/\s]+)/`)
)

// IC-U112 — **운영자 안내서의 판은 최신 릴리스와 같다.** 두 판에서 안내서가 밝힌 기준판, `go install`의 판,
// `raw.githubusercontent.com` 다운로드 주소의 판이 모두 릴리스 노트 맨 위 절의 판과 같다. 새 판을 내며 안내서를
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
	for _, guide := range []string{"operator-guide.md", "operator-guide.en.md"} {
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
}

var (
	readmeUp  = regexp.MustCompile(`pqcota \*\*(v\d+\.\d+\.\d+)\*\*`)
	demoUp    = regexp.MustCompile(`pqcota는 (v\d+\.\d+\.\d+)으로 받습니다|git clone --branch (v\d+\.\d+\.\d+) `)
	noteUp    = regexp.MustCompile("(?s)^## v[^\n]*\n.*?\n### [^\n]*\n\\s*- [^\n]*pqcota:\\s*[`*]*(v\\d+\\.\\d+\\.\\d+)")
	compatTop = regexp.MustCompile(`(?m)^\| (v\d+\.\d+\.\d+)(?: ~ (v\d+\.\d+\.\d+))? \| (v\d+\.\d+\.\d+) \|`)
	goModUp   = regexp.MustCompile(`(?m)^\s*github\.com/randyinthedev-hash/(pqcota[a-z-]*) (v\d+\.\d+\.\d+)`)
)

// IC-U144 — **판별 표(docs/compatibility.md)의 맨 위 줄은 지금 판이다.** 그 줄의 pqcaton 판(범위면 끝 판)은 릴리스 노트 맨 위
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
	top := compatTop.FindSubmatch(read(filepath.Join("docs", "compatibility.md")))
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
	readme := readmeUp.FindAllSubmatch(read("README.md"), -1)
	if len(readme) < 3 {
		t.Errorf("README names the pqcota version in %d places, want the intro, the prerequisites and the matching-versions note", len(readme))
	}
	for _, m := range readme {
		if string(m[1]) != pinned {
			t.Errorf("README names pqcota %s, but go.mod requires %s", m[1], pinned)
		}
	}
	demo := demoUp.FindAllSubmatch(read(filepath.Join("demo", "README.md")), -1)
	if len(demo) < 2 {
		t.Errorf("the demo guide does not name the pqcota version and the clone tag")
	}
	for _, m := range demo {
		if v := string(m[1]) + string(m[2]); v != pinned {
			t.Errorf("the demo guide names pqcota %s, but go.mod requires %s", v, pinned)
		}
	}
	// v0.25.0 뒤의 절은 첫 소제목(「받는 쪽이 달라지는 것」)의 첫 항목에 함께 쓰는 pqcota를 적는다(그 전 절은 소급하지 않는다).
	if string(note[1]) != "v0.25.0" {
		notes := read("RELEASE_NOTES.md")
		top := notes[latestNote.FindIndex(notes)[0]:]
		if m := noteUp.FindSubmatch(top); m == nil || string(m[1]) != pinned {
			t.Errorf("the first item of the latest release section does not name `pqcota: %s`", pinned)
		}
	}
}
