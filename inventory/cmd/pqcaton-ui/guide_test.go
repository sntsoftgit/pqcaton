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
