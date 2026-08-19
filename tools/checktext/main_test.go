package main

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, root, rel, src string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// IC-T1 — **문자열의 한글을 잡되 주석은 건드리지 않는다.**
//
// 규칙은 「코드와 그 출력은 영어, 문서와 주석은 한국어」입니다. 주석까지 막으면 이
// 리포가 판단 근거를 적어 두는 방식이 통째로 막힙니다.
func TestScanFindsStringsNotComments(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/a.go", "package a\n\n// 이 주석은 한국어다 — 막으면 안 된다.\nvar X = \"저장했습니다\"\n")
	got, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("잡은 것 %d개: %+v", len(got), got)
	}
	if got[0].line != 4 {
		t.Errorf("줄 번호가 %d — 주석이 아니라 문자열을 짚어야 한다", got[0].line)
	}
}

// IC-T2 — **정규식이 놓치는 자리를 잡는다.**
//
// 이 검사를 만들기 전 즉석 스크립트로 훑었더니 URL 안의 `//` 에서 줄이 잘려
// `"화면: http://%s"` 하나를, 여러 줄 백틱 문자열에서 둘을 놓쳤습니다. **파서로 봐야
// 하는 이유가 그것입니다** — 그래서 그 두 모양을 케이스로 박아 둔다.
func TestScanCatchesWhatRegexMisses(t *testing.T) {
	root := t.TempDir()
	write(t, root, "b/b.go", "package b\n\n"+
		"var A = \"화면: http://%s\"\n"+
		"var B = `\nSELECT 1;\n-- 여기 한글이 있다\n`\n")
	got, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("URL 안의 // 와 여러 줄 백틱 중 놓친 것이 있다: %+v", got)
	}
}

// IC-T3 — **봐주는 자리는 목록에 적힌 것뿐이다.** 규약(디렉터리 이름 등)으로 두면 새
// 파일이 슬그머니 예외가 된다.
func TestScanSkipsOnlyTheListedFiles(t *testing.T) {
	root := t.TempDir()
	write(t, root, "pkg/inventory/ui/text.go", "package ui\n\nvar A = \"한국어\"\n")
	write(t, root, "pkg/inventory/ui/other.go", "package ui\n\nvar B = \"한국어\"\n")
	write(t, root, "c/c_test.go", "package c\n\nvar C = \"케이스는 한국어로 적는다\"\n")
	write(t, root, "c/c_templ.go", "package c\n\nvar D = \"생성물이라 원본이 정답지다\"\n")
	got, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].file != "pkg/inventory/ui/other.go" {
		t.Fatalf("봐주는 자리가 어긋난다: %+v", got)
	}
}
