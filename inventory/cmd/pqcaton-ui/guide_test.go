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
