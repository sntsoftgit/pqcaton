// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// memFS — 시험용 파일 자리.
type memFS map[string]string

func (m memFS) ReadFile(name string) ([]byte, error) {
	s, ok := m[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(s), nil
}

func (m memFS) files() []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var testCfg = config{en: "English", ko: "KO", single: map[string]bool{"NOTES.md": true}, koSingle: map[string]bool{}, alias: map[string]bool{"EN-ED": true}}

func run(m memFS) []string { return checkIn(m, m.files(), testCfg) }

func expect(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(want) == 0 && len(got) > 0 {
		t.Fatalf("problems:\n%s", strings.Join(got, "\n"))
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if strings.Contains(g, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("no problem mentions %q; got:\n%s", w, strings.Join(got, "\n"))
		}
	}
}

// good — 짝 하나와 짝이 없어도 되는 문서 하나가 바르게 선 리포.
func good() memFS {
	return memFS{
		"README.md":        "English · [KO](README.ko.md)\n\n# pqcaton\n\nSee [the guide](docs/guide.md#2-install) and [notes](NOTES.md). v1.2.3, IC-U1.\n\n```bash\nmake          # gates\ngo install x@v1.2.3\n```\n",
		"README.ko.md":     "[English](README.md) · KO\n\n# pqcaton\n\n[안내](docs/guide.ko.md#2-설치)와 [노트](NOTES.md). v1.2.3, IC-U1.\n\n```bash\n# 관문\nmake          # 관문\ngo install x@v1.2.3\n```\n",
		"docs/guide.md":    "English · [KO](guide.ko.md)\n\n## 2. Install\n\n```bash\ncp <workspace>/a <workspace>/b\n```\n",
		"docs/guide.ko.md": "[English](guide.md) · KO\n\n## 2. 설치\n\n```bash\ncp <작업 폴더>/a <작업 폴더>/b\n```\n",
		"NOTES.md":         "# notes\n\n[back](README.md)\n",
	}
}

// IC-DC1 — **바르게 선 리포는 통과한다.** 짝의 첫 줄이 서로를 가리키고, 링크와 앵커(한국어 제목 포함)가 있으며, 두 판의 판 번호 ·
// 케이스 ID · 셸 명령(주석 줄과 줄 끝 주석은 빼고, `<…>` 자리 표시는 하나로 본다)이 같다. single.txt의 문서는 짝이 없어도 된다.
func TestGoodRepositoryPasses(t *testing.T) {
	expect(t, run(good()))
}

// IC-DC2 — **끊어진 링크와 없는 앵커를 막는다.** 없는 파일을 가리키는 링크, 그 문서에 없는 제목의 앵커를 줄 번호와 함께 알린다.
// 코드 블록과 인라인 코드 안의 링크 꼴, 바깥 URL은 보지 않는다.
func TestBrokenLinksAndAnchors(t *testing.T) {
	m := good()
	m["README.md"] = strings.Replace(m["README.md"], "docs/guide.md#2-install", "docs/gone.md", 1) +
		"\n[x](docs/guide.md#no-such)\n`[y](missing.md)`\n[z](https://example.com/missing.md)\n"
	expect(t, run(m), `README.md:5: the link "docs/gone.md" points to nothing`, "the anchor #no-such is not a heading")
	for _, p := range run(m) {
		if strings.Contains(p, "missing.md") {
			t.Errorf("a link in inline code or an external URL was checked: %s", p)
		}
	}
}

// IC-DC3 — **짝과 언어 전환 줄을 잰다.** 한국어 판만 있으면 영어 판이 없다고, 영어 문서에 짝도 single.txt 항목도 없으면 그렇다고
// 알린다. 짝의 첫 줄이 정해진 언어 전환 줄이 아니면 막는다.
func TestPairsAndLanguageLines(t *testing.T) {
	m := good()
	m["docs/only.ko.md"] = "[English](only.md) · KO\n"
	m["docs/lonely.md"] = "# lonely\n"
	m["docs/guide.ko.md"] = strings.Replace(m["docs/guide.ko.md"], "[English](guide.md) · KO", "English", 1)
	expect(t, run(m), "docs/only.ko.md: the English page docs/only.md is missing",
		"docs/lonely.md: has no Korean page docs/lonely.ko.md", "docs/guide.ko.md: the first line must be the language line")
}

// IC-DC4 — **두 판이 말하는 사실이 같아야 한다.** 한쪽에만 있는 판 번호, 한쪽에만 있는 케이스 ID, 차례나 내용이 다른 셸 명령을
// 막는다.
func TestPairContentsMustAgree(t *testing.T) {
	m := good()
	m["README.ko.md"] = strings.Replace(m["README.ko.md"], "v1.2.3, IC-U1.", "v1.2.4, IC-U2.", 1)
	m["docs/guide.ko.md"] = strings.Replace(m["docs/guide.ko.md"], "cp <작업 폴더>/a", "mv <작업 폴더>/a", 1)
	expect(t, run(m), "versions differ (only in English: ; only in Korean: v1.2.4)", "case ids differ (only in English: IC-U1; only in Korean: IC-U2)",
		"docs/guide.md ↔ docs/guide.ko.md: shell commands differ at command 1")
}

// IC-DC5 — **한국어 문서는 한국어 판을, 영어 문서는 영어 판을 가리킨다.** 짝이 있는 문서를 다른 말의 판으로 가리키면 막는다. 언어
// 전환 줄과 짝이 없는 문서로의 링크는 그대로 둔다. 다른 말의 판을 일부러 가리키는 링크는 글자가 언어 이름(「[English](x.md)」)이거나
// 그 파일 이름(「[x.md](x.md)」)이면 받는다. single.txt에 ko로 적은 문서는 한국어 문서로 본다.
func TestLinksStayInTheirLanguage(t *testing.T) {
	m := good()
	m["README.ko.md"] += "\n[영어 안내](docs/guide.md)\n"
	m["README.md"] += "\n[Korean guide](docs/guide.ko.md)\n"
	got := run(m)
	expect(t, got, "README.ko.md:", "link the Korean page docs/guide.ko.md", "README.md:", "link the English page docs/guide.md")
	for _, p := range got {
		if strings.Contains(p, "NOTES.md") || strings.Contains(p, ":1:") {
			t.Errorf("a language line or a link to an unpaired page was flagged: %s", p)
		}
	}

	m = good()
	m["README.ko.md"] += "\n영문본: [English](docs/guide.md) · [guide.md](docs/guide.md) · [EN-ED](docs/guide.md)\n"
	m["README.md"] += "\nKorean: [KO](docs/guide.ko.md) · [guide.ko.md](docs/guide.ko.md)\n"
	expect(t, run(m))

	m = good()
	m["NOTES.md"] = "# 노트\n\n[개요](README.md)\n"
	cfg := testCfg
	cfg.koSingle = map[string]bool{"NOTES.md": true}
	expect(t, checkIn(m, m.files(), cfg), "NOTES.md:3: link the Korean page README.ko.md")
}

// IC-DC6 — **앵커는 GitHub의 규칙으로 센다.** 소문자로 바꾸고, 글자 · 숫자 · `-` · `_`만 남기며, 공백 하나는 `-` 하나다(엠대시
// 둘레의 공백은 `--`가 된다). 링크와 강조는 글자만 남긴다. 같은 제목은 `-1` · `-2`가 붙는다.
func TestSlugFollowsGitHub(t *testing.T) {
	for in, want := range map[string]string{
		"2. Installation":                     "2-installation",
		"v0.27.1에서 v0.28.0으로":                 "v0271에서-v0280으로",
		"A — B":                               "a--b",
		"Contributor License Agreement (CLA)": "contributor-license-agreement-cla",
		"**Bold** and `code` [link](x.md)":    "bold-and-code-link",
		"기여자 라이선스 동의(CLA)":                    "기여자-라이선스-동의cla",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
	d, err := readDoc(memFS{"a.md": "# Same\n\n## Same\n\n### Same\n"}, "a.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"same", "same-1", "same-2"} {
		if !d.anchors[a] {
			t.Errorf("anchor %q missing: %v", a, fmt.Sprint(d.anchors))
		}
	}
}

// IC-DC7 — **사이트(docs/의 페이지)의 링크를 Pages 산출물 기준으로 잰다.** 상대 경로가 docs/ 밖을 가리키면 Pages에서 끊기므로 막고,
// 없는 파일 · 그 페이지에 없는 `#앵커`도 막는다. 같은 폴더의 파일, `./`, 바깥 URL은 받는다.
func TestPagesLinksResolveInTheArtifact(t *testing.T) {
	m := good()
	m["docs/index.html"] = `<a href="./">home</a> <a href="#top">top</a> <h1 id="top">x</h1> <a href="guide.md">g</a>
<a href="https://example.com/">ext</a> <a href="../README.md">up</a> <img src="logo.svg"> <a href="#gone">g</a>`
	expect(t, run(m), `docs/index.html:2: "../README.md" leaves docs/`, `docs/index.html:2: "logo.svg" points to nothing`,
		"docs/index.html:2: the anchor #gone is not an id on this page")
	for _, p := range run(m) {
		if strings.Contains(p, `"./"`) || strings.Contains(p, "#top") || strings.Contains(p, "guide.md") || strings.Contains(p, "example.com") {
			t.Errorf("a good link was flagged: %s", p)
		}
	}
}

// IC-DC8 — **이 리포 main의 절대 URL도 있는 자리를 가리켜야 한다.** md와 페이지 안의 `github.com/sntsoftgit/pqcaton/blob|tree/main/…`가
// 없는 경로나 없는 제목의 앵커를 가리키면 막는다. 다른 리포나 태그의 URL은 보지 않는다.
func TestRepositoryURLsResolve(t *testing.T) {
	m := good()
	m["NOTES.md"] += "\n[a](https://github.com/sntsoftgit/pqcaton/blob/main/docs/guide.md#2-install)" +
		"\n[b](https://github.com/sntsoftgit/pqcaton/blob/main/docs/design.md)" +
		"\n[c](https://github.com/sntsoftgit/pqcaton/tree/main/docs)" +
		"\n[d](https://github.com/sntsoftgit/pqcaton/blob/main/docs/guide.md#nope)" +
		"\n[e](https://github.com/sntsoftgit/pqcaton/blob/v0.1.0/gone.md)\n"
	got := run(m)
	expect(t, got, "points to docs/design.md, which is not in the repository", "the anchor #nope is not a heading of docs/guide.md")
	for _, p := range got {
		if strings.Contains(p, "#2-install") || strings.Contains(p, "tree/main/docs") || strings.Contains(p, "v0.1.0") {
			t.Errorf("a good or out-of-scope URL was flagged: %s", p)
		}
	}
}

// IC-DC9 — **요약판은 내용을 견주지 않는 대신 요약임을 밝혀야 한다.** summary.txt에 적은 영어판은 판 번호 · 케이스 ID · 셸 명령이
// 원문과 달라도 되지만, 제목 뒤 열 줄 안에 「summary」가 없으면 막는다. 적지 않은 짝은 그대로 견준다.
func TestSummaryPairs(t *testing.T) {
	m := good()
	m["docs/guide.md"] = "English · [KO](guide.ko.md)\n\n# Guide\n\n> This page is a summary of the Korean original.\n\n## 2. Install\n"
	cfg := testCfg
	cfg.summary = map[string]bool{"docs/guide.md": true}
	expect(t, checkIn(m, m.files(), cfg))
	m["docs/guide.md"] = "English · [KO](guide.ko.md)\n\n# Guide\n\n## 2. Install\n"
	expect(t, checkIn(m, m.files(), cfg), "docs/guide.md: listed in summary.txt but does not say near the top that it is a summary")
	expect(t, run(m), "docs/guide.md ↔ docs/guide.ko.md: shell commands differ")
}

// tablePair — 케이스 표가 바르게 짝지은 두 판.
func tablePair() memFS {
	m := good()
	m["docs/cases.md"] = "English · [KO](cases.ko.md)\n\n# Cases\n\n## A\n\n| TC | Given | Then |\n|---|---|---|\n" +
		"| **[IC-A1](../a_test.go) ✅** | input `x|y` | see [guide](guide.md#2-install) |\n| [IC-A2](../a_test.go) ✅ | two | ok |\n"
	m["docs/cases.ko.md"] = "[English](cases.md) · KO\n\n# 케이스\n\n## 가\n\n| TC | Given | Then |\n|---|---|---|\n" +
		"| **[IC-A1](../a_test.go) ✅** | 입력 `x|y` | [안내](guide.ko.md#2-설치) 참고 |\n| [IC-A2](../a_test.go) ✅ | 둘 | 된다 |\n"
	m["a_test.go"], m["b_test.go"], m["c_test.go"] = "package a\n", "package a\n", "package a\n"
	return m
}

func runTables(m memFS) []string {
	cfg := testCfg
	cfg.tables = map[string]bool{"docs/cases.md": true}
	return checkIn(m, m.files(), cfg)
}

// IC-DC10 — **두 판의 케이스 표는 행마다 같은 사실이다.** ID로 짝지은 행의 첫 칸(ID · 코드 링크 · 상태 표시)이 바이트까지 같고,
// 열 개수(인라인 코드 안의 `|`는 칸을 나누지 않는다)와 행 안의 링크 대상(한국어 판 이름은 영어 판 이름으로, 앵커는 빼고)이 같아야
// 한다. 바르게 짝지은 두 판은 통과하고, 첫 칸 · 열 개수 · 링크가 다르면 막는다.
func TestCaseTablesAgreeRowByRow(t *testing.T) {
	expect(t, runTables(tablePair()))
	m := tablePair()
	m["docs/cases.ko.md"] = strings.Replace(m["docs/cases.ko.md"], "| [IC-A2](../a_test.go) ✅ |", "| [IC-A2](../b_test.go) ✅ |", 1)
	expect(t, runTables(m), "the first cell of IC-A2 differs")
	m = tablePair()
	m["docs/cases.ko.md"] = strings.Replace(m["docs/cases.ko.md"], "| 둘 | 된다 |", "| 둘 | 된다 | 더 |", 1)
	expect(t, runTables(m), "IC-A2 has 3 cells vs 4")
	m = tablePair()
	m["docs/cases.ko.md"] = strings.Replace(m["docs/cases.ko.md"], "[안내](guide.ko.md#2-설치) 참고", "참고", 1)
	expect(t, runTables(m), "the links in the row of IC-A1 differ")
}

// IC-DC11 — **한쪽에만 있는 케이스 · 중복 ID · 차례 차이 · 제목 수의 차이를 막는다.** 케이스는 영어판에서 케이스 관문이 한 번만
// 세고, 이 관문은 두 판이 같은지만 본다.
func TestCaseTablesMissingDuplicateOrder(t *testing.T) {
	m := tablePair()
	m["docs/cases.ko.md"] = strings.Replace(m["docs/cases.ko.md"], "| [IC-A2](../a_test.go) ✅ | 둘 | 된다 |\n", "", 1)
	expect(t, runTables(m), "the case IC-A2 is missing from docs/cases.ko.md")
	m = tablePair()
	m["docs/cases.md"] += "| [IC-A2](../a_test.go) ✅ | two | ok |\n"
	expect(t, runTables(m), "docs/cases.md: the case IC-A2 appears 2 times")
	m = tablePair()
	ko := m["docs/cases.ko.md"]
	r1 := "| **[IC-A1](../a_test.go) ✅** | 입력 `x|y` | [안내](guide.ko.md#2-설치) 참고 |\n"
	r2 := "| [IC-A2](../a_test.go) ✅ | 둘 | 된다 |\n"
	m["docs/cases.ko.md"] = strings.Replace(ko, r1+r2, r2+r1, 1)
	expect(t, runTables(m), "the case order differs at IC-A1 (English) vs IC-A2 (Korean)")
	m = tablePair()
	m["docs/cases.ko.md"] += "\n## 나\n"
	expect(t, runTables(m), "1 level-2 headings vs 2")
}

// IC-DC12 — **`-sync-cases`는 영어판 케이스 행의 첫 칸만 한국어판에 옮긴다.** 케이스 관문이 영어판의 링크를 다시 찍은 뒤 돌리면
// 한국어판의 같은 ID 행 첫 칸이 영어판과 같아지고, 나머지 칸은 그대로다.
func TestSyncCasesCopiesOnlyTheFirstCell(t *testing.T) {
	dir := t.TempDir()
	en, ko := dir+"/cases.md", dir+"/cases.ko.md"
	m := tablePair()
	if err := os.WriteFile(en, []byte(strings.Replace(m["docs/cases.md"], "[IC-A2](../a_test.go)", "[IC-A2](../c_test.go)", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ko, []byte(m["docs/cases.ko.md"]), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := syncCases(en, ko)
	if err != nil || n != 1 {
		t.Fatalf("synced %d, %v", n, err)
	}
	b, _ := os.ReadFile(ko)
	if !strings.Contains(string(b), "| [IC-A2](../c_test.go) ✅ | 둘 | 된다 |") || !strings.Contains(string(b), "입력 `x|y`") {
		t.Errorf("after sync:\n%s", b)
	}
}
