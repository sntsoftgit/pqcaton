// Command checktext — **코드 안의 문자열에 한글이 남아 있는지** 막는다.
//
// 규칙은 하나다: 코드와 그 출력은 영어, 화면만 두 말. 문서와 주석은 한국어다.
//
// 왜 관문인가: 문자열 하나를 옮기지 않으면 그 자리만 한국어로 뜨고, 눈으로는 못
// 찾는다. 실제로 이 검사를 만들기 전에 즉석 스크립트로 훑었더니 URL 안의 `//` 에서
// 줄이 잘려 `"화면: http://%s"` 하나를 놓쳤다 — **정규식이 아니라 파서로 봐야 한다.**
//
// 봐주는 자리는 셋뿐이고, 셋 다 그래야 할 이유가 있다:
//
//   - `_test.go` — 케이스가 무엇을 재는지는 이 리포를 만드는 사람이 읽는다
//   - 화면 문구 카탈로그 — 두 말이 나란히 있는 자리다
//   - 말 바꾸기 토글 — 지금 말을 못 읽는 사람도 자기 말은 알아봐야 한다
//
// usage: go run ./tools/checktext
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// allowed — 한글이 있어도 되는 파일. **경로로 적는다** — 규약이 아니라 목록이라야
// 새 파일이 슬그머니 예외가 되지 않는다.
var allowed = map[string]bool{
	"pkg/inventory/ui/text.go":         true,
	"pkg/inventory/ui/text_decl.go":    true,
	"pkg/inventory/ui/text_msg.go":     true,
	"pkg/inventory/ui/text_more.go":    true,
	"pkg/inventory/ui/text_refusal.go": true,
	"pkg/inventory/ui/i18n.go":         true,
}

type hit struct {
	file string
	line int
	text string
}

func main() {
	hits, err := scan(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ could not scan the sources:", err)
		os.Exit(1)
	}
	if len(hits) > 0 {
		fmt.Fprintln(os.Stderr, "✗ Korean in a code string — code and its output are English:")
		for _, h := range hits {
			fmt.Fprintf(os.Stderr, "    %s:%d  %s\n", h.file, h.line, h.text)
		}
		fmt.Fprintln(os.Stderr, "\nIf it shows on a screen, put the Korean in pkg/inventory/ui/text*.go")
		fmt.Fprintln(os.Stderr, "and keep the English where the package that owns the fact keeps it.")
		os.Exit(1)
	}
	fmt.Println("✓ text check passed (no Korean in code strings)")
}

// scan — Go 소스의 **문자열 리터럴만** 본다. 주석은 한국어이므로 건드리지 않는다.
func scan(root string) ([]hit, error) {
	var out []hit
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == ".git" || n == "testdata" || n == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		// 봐주는 목록은 리포 상대 경로로 적혀 있다 — 어디서 돌리든 같게 본다.
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		// 생성물은 원본(.templ)이 정답지다. 원본을 고치면 여기도 같이 바뀐다.
		if strings.HasSuffix(rel, "_templ.go") || allowed[rel] {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || !hasHangul(lit.Value) {
				return true
			}
			out = append(out, hit{rel, fset.Position(lit.Pos()).Line, trim(lit.Value)})
			return true
		})
		return nil
	})
	return out, err
}

func hasHangul(s string) bool {
	for _, r := range s {
		if r >= 0xAC00 && r <= 0xD7A3 {
			return true
		}
	}
	return false
}

func trim(s string) string {
	if len(s) > 70 {
		return s[:70] + "…"
	}
	return s
}
