// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// enTemplates: 화면 문구 파일에서 T{…, EN: …}의 영어 문구를 모두 꺼낸다(이어 붙인 문자열도 편다).
func enTemplates(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("text*.go")
	if err != nil {
		t.Fatal(err)
	}
	var lit func(e ast.Expr) (string, bool)
	lit = func(e ast.Expr) (string, bool) {
		switch x := e.(type) {
		case *ast.BasicLit:
			s, err := strconv.Unquote(x.Value)
			return s, err == nil
		case *ast.BinaryExpr:
			a, ok1 := lit(x.X)
			b, ok2 := lit(x.Y)
			return a + b, ok1 && ok2 && x.Op == token.ADD
		}
		return "", false
	}
	var out []string
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if id, isID := cl.Type.(*ast.Ident); !isID || id.Name != "T" {
				return true
			}
			for _, el := range cl.Elts {
				if kv, ok := el.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "EN" {
						if s, ok := lit(kv.Value); ok {
							out = append(out, s)
						}
					}
				}
			}
			return true
		})
	}
	return out
}

var (
	verb       = regexp.MustCompile(`%[-+# 0]*\d*(?:\.\d+)?[dsvq]`)
	onePlural  = regexp.MustCompile(`(^|[^\d.,])1(</b>)? (?:(?:observed|required|rule|policy|auto-pass|recorded|negotiated) )?[a-z]+[^s]s\b`)
	oneAre     = regexp.MustCompile(`(^|[^\d.,])1 [^.;:·—]*?\b(are|were|need|do)\b`)
	compoundOK = regexp.MustCompile(`\band the approval signature are\b`)
)

// IC-UI53 — **영어 문구는 수가 1이면 단수다.** 수가 들어가는 영어 문구를 모두 1로 채워 그리면 「1 nodes」 같은 복수
// 명사도, 그 주어에 맞지 않는 are · were · need도 남지 않는다. 2로 그리면 복수 그대로다. 수를 채우는 길은 T.F 하나다.
func TestEnglishCountsAgree(t *testing.T) {
	n := 0
	for _, tmpl := range enTemplates(t) {
		if !strings.Contains(tmpl, "%d") {
			continue
		}
		n++
		render := func(k int) string { return T{EN: tmpl}.F(EN, argsOf(tmpl, k)...) }
		one := render(1)
		if m := onePlural.FindString(one); m != "" {
			t.Errorf("a plural after 1 in %q (%q)", one, m)
		}
		if m := oneAre.FindString(one); m != "" && !compoundOK.MatchString(one) {
			t.Errorf("a plural verb for 1 in %q (%q)", one, m)
		}
		if two := render(2); two != fmt.Sprintf(tmpl, argsOf(tmpl, 2)...) {
			t.Errorf("2 changed the sentence: %q", two)
		}
	}
	if n < 20 {
		t.Fatalf("found only %d English templates with a count; the parser missed some", n)
	}
	for in, want := range map[string]string{
		"1 nodes · 11 nodes · 0.1 rules":    "1 node · 11 nodes · 0.1 rules",
		"<b>1</b> assets observed":          "<b>1</b> asset observed",
		"Record a conclusion on 1 policies": "Record a conclusion on 1 policy",
		"1 rule changes are waiting":        "1 rule change is waiting",
		"1 of them need a reason":           "1 of them needs a reason",
	} {
		if got := enCounts(in); got != want {
			t.Errorf("enCounts(%q) = %q, want %q", in, got, want)
		}
	}
}

func argsOf(tmpl string, k int) []any {
	var args []any
	for _, v := range verb.FindAllString(tmpl, -1) {
		if strings.HasSuffix(v, "d") {
			args = append(args, k)
		} else {
			args = append(args, "x")
		}
	}
	return args
}
