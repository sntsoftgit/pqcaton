// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/zip"
)

// fixture 는 rels 를 빈 파일로 만들고 zip.File 목록으로 돌려준다.
func fixture(t *testing.T, rels ...string) []zip.File {
	t.Helper()
	root := t.TempDir()
	var files []zip.File
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, file{root: root, rel: rel})
	}
	return files
}

// v0.26.2 에서 실제로 막힌 경로를 잡는다.
func TestRejectsWindowsReservedDirectory(t *testing.T) {
	got := check(fixture(t, "go.mod", "inventory/cmd/pqcaton-ui/testdata/nul/plan-ok.json"))
	if len(got) != 1 || !strings.Contains(got[0], "testdata/nul/plan-ok.json") {
		t.Fatalf("want one problem naming testdata/nul, got %q", got)
	}
}

// 대소문자만 다른 두 경로는 대소문자를 가리지 않는 파일 시스템에서 겹친다.
func TestRejectsCaseCollision(t *testing.T) {
	if got := check(fixture(t, "go.mod", "docs/A.md", "docs/a.md")); len(got) == 0 {
		t.Fatal("want a case-collision problem, got none")
	}
}

// 고친 이름과 하위 모듈은 통과한다. 하위 모듈 안의 예약어는 본 모듈 zip 에 실리지 않는다.
func TestAcceptsRenamedAndSubmodule(t *testing.T) {
	got := check(fixture(t, "go.mod", "inventory/cmd/pqcaton-ui/testdata/plantext/plan-ok.json",
		"tools/sub/go.mod", "tools/sub/nul/x.txt"))
	if len(got) != 0 {
		t.Fatalf("want no problems, got %q", got)
	}
}
