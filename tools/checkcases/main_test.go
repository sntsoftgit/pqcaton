// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDoc — 케이스 표 한 조각을 임시 문서로 쓰고 읽어 온다. `owned`를 주지 않으면 셋 다 맡는다.
func writeDoc(t *testing.T, body string, owned ...string) (docSpec, []docCase, []string) {
	t.Helper()
	if len(owned) == 0 {
		owned = kinds
	}
	d, cases, lines, _ := scanBody(t, body, owned, nil)
	return d, cases, lines
}

// scanBody — writeDoc 과 같되 external 을 주고, 어느 쪽에도 없는 접두어의 행까지 돌려받는다.
func scanBody(t *testing.T, body string, owned, external []string) (docSpec, []docCase, []string, []string) {
	t.Helper()
	d := docSpec{path: filepath.Join(t.TempDir(), "testcases.md"), owned: owned, external: external}
	if err := os.WriteFile(d.path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cases, lines, strays, err := scanDoc("", d)
	if err != nil {
		t.Fatal(err)
	}
	return d, cases, lines, strays
}

// writeConf — docs.tsv 를 임시 자리에 쓰고 그 경로를 돌려준다.
func writeConf(t *testing.T, body string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "docs.tsv")
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// IC-M1 — **축약한 번호를 편다.**
//
// 테스트가 앞머리를 한 번만 적고 뒤 번호를 가운뎃점으로 이어 붙이는 자리가 있다. 이것을 못
// 펴면 「✅ 인데 테스트가 없다」가 거짓으로 뜬다. 실제로 이 도구를 만들기 전에 손으로 세다가
// 여섯 건을 그렇게 잘못 셌다.
//
// **예시를 주석이 아니라 아래 입력에 둔다.** 주석에 실제 번호를 적으면 그것이 표식으로 잡혀
// 이 파일이 남의 케이스를 재는 자리로 등록된다.
func TestShorthandIdsAreExpanded(t *testing.T) {
	got := ids("// IC-R1·R2·R3 — 3-상태\n// IC-F2·F3: 전이\n// CP-TOKEN-4·5·6: 거절\n// RUN-2 하나")
	want := []string{"IC-R1", "IC-R2", "IC-R3", "IC-F2", "IC-F3", "CP-TOKEN-4", "CP-TOKEN-5", "CP-TOKEN-6", "RUN-2"}
	if len(got) != len(want) {
		t.Fatalf("펴지 못했다: %v", got)
	}
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("%s 를 못 폈다: %v", w, got)
		}
	}
}

// IC-M2 — **번호 칸의 여러 모양을 다 읽는다.**
//
// 굵게가 대괄호 밖일 수도 안일 수도 있고, 상태 표시가 없을 수도 있다(컨트롤 플레인 명세가
// 그렇다). 하나라도 못 읽으면 그 케이스만 관문 밖이 된다.
func TestEveryCellShapeIsRead(t *testing.T) {
	_, cases, _ := writeDoc(t, strings.Join([]string{
		"| IC-R1 ✅ | 자산이 선언 ∩ 관측 | CONFIRMED |",
		"| **IC-O1 ✅** | 다른 조직이 섞임 | 중단한다 |",
		"| [IC-S1](../pkg/inventory/scope/scope_test.go) ✅ | 상속 | 하위가 이긴다 |",
		"| [**CP-PG-5**](../internal/intake/seen_pg_test.go) | 동시 확보 | 하나만 성공 |",
		"| [RUN-2](runner_test.go) | 스케줄만으로 | 묻지 않는다 |",
		"| IC-C3 ⏳ | 실측 | 나중 |",
		"**핵심 인수 기준**: **IC-P4**(표가 아니라 본문이다)",
	}, "\n"))
	if len(cases) != 6 {
		t.Fatalf("표 행만 여섯이라야 한다: %d개 %v", len(cases), cases)
	}
	if !cases[1].boldOut {
		t.Error("대괄호 밖의 굵게를 못 읽었다")
	}
	if !cases[3].boldIn {
		t.Error("대괄호 안의 굵게를 못 읽었다")
	}
	if cases[3].status != "" {
		t.Errorf("상태 표시가 없는 행인데 무언가 읽었다: %q", cases[3].status)
	}
	if cases[2].link != "../pkg/inventory/scope/scope_test.go" {
		t.Errorf("붙어 있는 링크를 못 읽었다: %q", cases[2].link)
	}
}

// IC-M3 — **`-write`는 굵게와 상태 표시를 지킨다.** 링크를 붙이면서 문서의 모양이 달라지면
// 사람이 그 diff를 읽지 못하고, 읽지 못하면 다음부터 돌리지 않는다.
func TestWriteKeepsBoldAndStatus(t *testing.T) {
	d, cases, lines := writeDoc(t, strings.Join([]string{
		"| IC-R1 ✅ | 선언 ∩ 관측 | CONFIRMED |",
		"| **IC-O1 ✅** | 섞임 | 중단한다 |",
		"| [**CP-PG-5**](x) | 동시 확보 | 하나만 |",
	}, "\n"))
	tests := map[string][]string{
		"IC-R1":   {"pkg/inventory/reconcile/reconcile_test.go"},
		"IC-O1":   {"pkg/inventory/reconcile/org_test.go"},
		"CP-PG-5": {"internal/intake/seen_pg_test.go"},
	}
	if n := rewrite(d, lines, cases, tests); n != 3 {
		t.Fatalf("셋 다 찍어야 한다: %d", n)
	}
	for i, want := range []string{
		"| [IC-R1](",
		"| **[IC-O1](",
		"| [**CP-PG-5**](",
	} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("%d번째 줄의 모양이 달라졌다: %s", i, lines[i])
		}
	}
	if !strings.Contains(lines[1], ") ✅** |") {
		t.Errorf("굵게 안의 상태 표시를 잃었다: %s", lines[1])
	}
	if strings.Contains(lines[2], "✅") {
		t.Errorf("없던 상태 표시를 만들었다: %s", lines[2])
	}
}

// IC-M4 — **찍은 것을 다시 읽어도 같다.** 두 번 돌렸을 때 링크가 겹쳐 쌓이면 문서가 망가진다.
func TestWriteIsIdempotent(t *testing.T) {
	d, cases, lines := writeDoc(t, "| **IC-O1 ✅** | 섞임 | 중단한다 |")
	tests := map[string][]string{"IC-O1": {"pkg/inventory/reconcile/org_test.go"}}
	rewrite(d, lines, cases, tests)
	first := lines[0]

	d2, cases2, lines2 := writeDoc(t, first)
	if n := rewrite(d2, lines2, cases2, tests); n != 0 {
		t.Errorf("이미 맞는데 또 찍었다: %d", n)
	}
	if lines2[0] != first {
		t.Errorf("두 번째에 달라졌다:\n  %s\n  %s", first, lines2[0])
	}
}

// IC-M5 — **한 번호를 두 파일이 재도 된다.** 엣지판과 본판이 같은 번호를 쓰는 자리가 있다.
// 링크는 정렬해서 첫 파일로 걸되, 그것을 어긋남으로 세지 않는다.
func TestOneIdMayLiveInTwoFiles(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a_test.go": "package p\n\n// IC-R4: 본판\nfunc TestA(t *testing.T) {}\n",
		"b_test.go": "package p\n\n// IC-R4(엣지판)\nfunc TestB(t *testing.T) {}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := scanTests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got["IC-R4"]) != 2 {
		t.Fatalf("두 파일을 다 잡아야 한다: %v", got["IC-R4"])
	}
	if !strings.HasSuffix(got["IC-R4"][0], "a_test.go") {
		t.Errorf("정렬해서 첫 파일을 골라야 한다: %v", got["IC-R4"])
	}
}

// IC-M6 — **함께 나가는 문서와 테스트가 실제로 맞는다.** 위 케이스들은 만들어 낸 입력으로
// 재는 것이라, 진짜 리포에서도 맞는지는 여기서 잰다. 이 케이스가 이 도구의 존재 이유다.
func TestShippedDocsAndTestsAgree(t *testing.T) {
	root := filepath.Join("..", "..")
	docs, err := loadDocs("docs.tsv")
	if err != nil {
		t.Fatal(err)
	}
	tests, err := scanTests(root)
	if err != nil {
		t.Fatal(err)
	}
	owned, seen, total := map[string]bool{}, map[string]bool{}, 0
	for _, d := range docs {
		for _, k := range d.owned {
			owned[k] = true
		}
		cs, _, strays, err := scanDoc(root, d)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range strays {
			t.Error(s)
		}
		total += len(cs)
		for _, c := range cs {
			for _, id := range c.covers {
				seen[id] = true
			}
			if c.status != "🔜" && c.status != "⏳" && len(tests[c.covers[0]]) == 0 {
				t.Errorf("%s: %s 가 테스트를 주장하는데 그 번호를 단 테스트가 없다", d.path, c.id)
			}
		}
	}
	if total < 150 {
		t.Fatalf("케이스를 너무 적게 읽었다: %d개", total)
	}
	for id, files := range tests {
		if owned[kindOf(id)] && !seen[id] {
			t.Errorf("%s 가 %s 에 있는데 어느 표에도 없다", id, files[0])
		}
	}
}

// IC-M7 — **주석만 본다.**
//
// 이 파일이 픽스처로 케이스 표의 한 줄을 문자열에 담고 있다. 파일 전체를 정규식으로 훑으면
// 그 문자열의 번호가 표식으로 잡힌다. 실제로 그렇게 해서 미구현 케이스 하나가 이 도구의
// 테스트 파일로 링크됐다. checktext가 반대 방향으로 겪은 것과 같은 일이고 답도 같다:
// **정규식이 아니라 파서로 본다.**
func TestStringLiteralsAreNotMarkers(t *testing.T) {
	dir := t.TempDir()
	src := "package p\n\n" +
		"// IC-M1 은 주석이라 잡힌다.\n" +
		"var fixture = \"| IC-M2 ⏳ | 이건 문자열이라 잡히면 안 된다 |\"\n"
	if err := os.WriteFile(filepath.Join(dir, "x_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := scanTests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got["IC-M1"]) != 1 {
		t.Errorf("주석의 번호를 놓쳤다: %v", got)
	}
	if len(got["IC-M2"]) != 0 {
		t.Errorf("문자열의 번호를 잡았다: %v", got["IC-M2"])
	}
}

// IC-M8 — **external 접두어의 행은 케이스로 읽지 않는다.**
//
// 컨트롤 플레인 명세의 러너 행은 테스트가 다른 리포에 있다. 그 행을 케이스로 읽으면 남의
// 리포에 있는 테스트를 「없다」고 막게 된다. 그렇다고 목록에서 빼기만 하면 왜 재지 않는지가
// 남지 않으므로, external 로 적고 건너뛴다.
func TestDocSkipsExternalKinds(t *testing.T) {
	body := strings.Join([]string{
		"| [IC-R1](x) ✅ | 인벤토리 | CONFIRMED |",
		"| [RUN-2](y) | 러너 | 묻지 않는다 |",
	}, "\n")
	_, both, _ := writeDoc(t, body)
	if len(both) != 2 {
		t.Fatalf("둘 다 맡으면 둘 다 읽어야 한다: %v", both)
	}
	_, only, _, strays := scanBody(t, body, []string{"IC"}, []string{"RUN"})
	if len(only) != 1 || only[0].id != "IC-R1" {
		t.Fatalf("IC 만 맡으면 IC 만 읽어야 한다: %v", only)
	}
	if len(strays) != 0 {
		t.Errorf("external 로 적은 행을 짚었다: %v", strays)
	}
}

// IC-M9 — **링크는 그 문서에서 본 상대 경로다.** 케이스 표가 docs/에도 있고 테스트 바로
// 옆에도 있다. 한 가지로 적으면 한쪽이 깨진다.
func TestLinkIsRelativeToItsOwnDoc(t *testing.T) {
	for _, c := range []struct{ doc, test, want string }{
		{"docs/testcases.md", "pkg/inventory/reconcile/reconcile_test.go", "../pkg/inventory/reconcile/reconcile_test.go"},
		{"saas/runner/README.md", "saas/runner/runner_test.go", "runner_test.go"},
		{"saas/runner/README.md", "saas/runner/lock_unix_test.go", "lock_unix_test.go"},
	} {
		if got := linkTo(c.doc, c.test); got != c.want {
			t.Errorf("%s → %s: %q, 기대 %q", c.doc, c.test, got, c.want)
		}
	}
}

// IC-M10 — **어느 쪽에도 없는 접두어의 행은 막는다.**
//
// 예전에는 문서가 맡지 않은 접두어의 행을 아무 표시 없이 건너뛰었다. 그러면 표에 새 접두어가
// 들어와도 아무도 모르고, 그 행들은 처음부터 관문 밖에 있다.
func TestUnclassifiedKindIsFlagged(t *testing.T) {
	body := strings.Join([]string{
		"| [IC-R1](x) ✅ | 인벤토리 | CONFIRMED |",
		"| [RUN-2](y) | 러너 | 묻지 않는다 |",
		"| [CP-PG-5](z) | 동시 확보 | 하나만 |",
	}, "\n")
	_, cases, _, strays := scanBody(t, body, []string{"IC"}, []string{"RUN"})
	if len(cases) != 1 {
		t.Fatalf("IC 행 하나만 케이스여야 한다: %v", cases)
	}
	if len(strays) != 1 || !strings.Contains(strays[0], ":3 ") || !strings.Contains(strays[0], "prefix CP") {
		t.Fatalf("셋째 줄의 CP 를 짚어야 한다: %v", strays)
	}
}

// IC-M11 — **docs.tsv 의 잘못된 행은 고쳐 읽지 않고 막는다.**
//
// 설정이 틀렸는데 관문이 통과하면, 그 통과는 아무것도 재지 않은 결과일 수 있다.
func TestMalformedConfigIsRejected(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"칸이 둘", "docs/a.md\tIC\n", "want 3"},
		{"칸이 넷", "docs/a.md\tIC\t-\tRUN\n", "want 3"},
		{"탭 대신 공백", "docs/a.md   IC   -\n", "want 3"},
		{"절대 경로", "/etc/a.md\tIC\t-\n", "clean path"},
		{"리포 밖", "../a.md\tIC\t-\n", "clean path"},
		{"정규형이 아님", "docs/./a.md\tIC\t-\n", "clean path"},
		{"같은 경로 둘", "docs/a.md\tIC\t-\ndocs/a.md\tRUN\t-\n", "listed twice"},
		{"모르는 접두어", "docs/a.md\tIC,XY\t-\n", "unknown prefix"},
		{"소문자", "docs/a.md\tic\t-\n", "unknown prefix"},
		{"빈 항목", "docs/a.md\tIC,\t-\n", "empty prefix"},
		{"중복 항목", "docs/a.md\tIC,IC\t-\n", "repeated"},
		{"아무것도 안 다룸", "docs/a.md\t-\t-\n", "classifies no prefix"},
		{"행이 없음", "# 주석만\n\n", "no case tables"},
	} {
		_, err := loadDocs(writeConf(t, c.body))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %q 로 막아야 한다: %v", c.name, c.want, err)
		}
	}
	if _, err := loadDocs(filepath.Join(t.TempDir(), "docs.tsv")); err == nil {
		t.Error("설정 파일이 없는데 막지 않았다")
	}

	docs, err := loadDocs(writeConf(t, "# 머리\n\ndocs/testcases.md\t\tCP,IC\t\tRUN\nsaas/runner/README.md\tRUN\t-\n"))
	if err == nil {
		t.Fatalf("RUN 이 두 행에서 owned·external 로 갈렸는데 통과했다: %v", docs)
	}
	docs, err = loadDocs(writeConf(t, "# 머리\n\ndocs/testcases.md\t\tCP,IC\t\tRUN\n"))
	if err != nil {
		t.Fatalf("맞는 설정을 막았다: %v", err)
	}
	if len(docs) != 1 || strings.Join(docs[0].owned, ",") != "CP,IC" || strings.Join(docs[0].external, ",") != "RUN" {
		t.Errorf("탭을 여럿 써 맞춘 행을 잘못 읽었다: %+v", docs)
	}
}

// IC-M12 — **한 접두어를 owned 와 external 에 함께 적으면 막는다.** 한 행 안이든 두 행에
// 걸쳐서든 같다. 이 리포가 재는 접두어이면서 남이 잰다는 말은 둘 중 하나가 틀렸다는 뜻이다.
func TestPrefixCannotBeBothOwnedAndExternal(t *testing.T) {
	for _, body := range []string{
		"docs/a.md\tIC,RUN\tRUN\n",
		"docs/a.md\tRUN\t-\ndocs/b.md\tIC\tRUN\n",
		"docs/a.md\tIC\tRUN\ndocs/b.md\tRUN\t-\n",
	} {
		_, err := loadDocs(writeConf(t, body))
		if err == nil || !strings.Contains(err.Error(), "both owned and external") {
			t.Errorf("%q 를 막아야 한다: %v", body, err)
		}
	}
	if _, err := loadDocs(writeConf(t, "docs/a.md\t-\tRUN\ndocs/b.md\tIC\tRUN\n")); err != nil {
		t.Errorf("두 문서가 같은 접두어를 external 로 적는 것은 된다: %v", err)
	}
}

// IC-M13 — **docs.tsv 의 경로는 리포 루트 기준이다.** 설정 파일은 tools/checkcases/ 에
// 있지만 거기서 이어 붙이면 문서를 못 찾는다. 링크도 리포 루트 기준 경로에서 계산해야
// 문서에서 본 상대 경로가 맞는다.
func TestConfigPathsAreRepoRootRelative(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"docs", "tools/checkcases"} {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "testcases.md"), []byte("| IC-R1 ✅ | 선언 | 맞다 |\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(root, "tools", "checkcases", "docs.tsv")
	if err := os.WriteFile(conf, []byte("docs/testcases.md\tIC\t-\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	docs, err := loadDocs(conf)
	if err != nil {
		t.Fatal(err)
	}
	if docs[0].path != "docs/testcases.md" {
		t.Errorf("경로를 적힌 그대로 들고 있어야 한다: %q", docs[0].path)
	}
	cases, _, _, err := scanDoc(root, docs[0])
	if err != nil {
		t.Fatalf("리포 루트에서 문서를 못 찾았다: %v", err)
	}
	if len(cases) != 1 || cases[0].doc != "docs/testcases.md" {
		t.Fatalf("케이스에 리포 루트 기준 경로가 적혀야 한다: %+v", cases)
	}
	if got := linkTo(cases[0].doc, "pkg/a/a_test.go"); got != "../pkg/a/a_test.go" {
		t.Errorf("링크가 문서에서 본 상대 경로가 아니다: %q", got)
	}
}

// IC-M14 — **축약한 번호의 행도 `-write`가 링크를 찍는다.**
//
// 게이트는 링크를 행의 첫 번호로 걸고, `-write`는 행의 글자 전체로 테스트를 찾았다. 축약한 행은
// 글자 전체로는 어느 테스트도 없어서, 링크를 벗기면 관문이 막고 `-write`는 0개를 찍은 채 끝났다.
// 그 행만 사람이 손으로 붙여야 했는데, 손으로 붙이지 않는 것이 이 도구의 약속이다.
func TestWriteRestoresShorthandRows(t *testing.T) {
	d, cases, lines := writeDoc(t, strings.Join([]string{
		"| CP-TOKEN-4·5·6 | 거절 셋 | 같은 응답 |",
		"| [**CP-RUNNER-1·2**](틀린/링크) | 등록과 갱신 | 된다 |",
		"| **CP-TOKEN-7 ✅** | 마지막 사용 | 기록한다 |",
	}, "\n"))
	tests := map[string][]string{
		"CP-TOKEN-4":  {"internal/access/access_test.go"},
		"CP-RUNNER-1": {"internal/access/access_test.go"},
		"CP-TOKEN-7":  {"internal/access/access_test.go"},
	}
	if n := rewrite(d, lines, cases, tests); n != 3 {
		t.Fatalf("축약한 행까지 셋을 다 찍어야 한다: %d\n%s", n, strings.Join(lines, "\n"))
	}
	for i, want := range []string{
		"| [CP-TOKEN-4·5·6](internal/access/access_test.go) |",
		"| [**CP-RUNNER-1·2**](internal/access/access_test.go) |",
		"| **[CP-TOKEN-7](internal/access/access_test.go) ✅** |",
	} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("%d번째 줄이 기대와 다르다: %s", i, lines[i])
		}
	}

	d2, cases2, lines2 := writeDoc(t, strings.Join(lines, "\n"))
	if n := rewrite(d2, lines2, cases2, tests); n != 0 {
		t.Errorf("찍은 것을 다시 찍었다: %d", n)
	}
}

// IC-M15 — **번호에 괄호만 씌운 행도 케이스 행이다.**
//
// 번호 칸은 세 모양으로 온다: 맨 번호, 괄호만 씌운 번호, 링크가 붙은 번호. 가운데 것을 못
// 읽으면 그 행은 아무 표시 없이 관문 밖으로 나간다. 테스트가 없는 ✅ 행을 써 넣어도 막히지 않았다.
// 괄호만 씌운 번호는 **아직 링크되지 않은 케이스**로 읽고, `-write`가 링크를 붙인다.
func TestBracketedIdWithoutLinkIsARow(t *testing.T) {
	d, cases, lines := writeDoc(t, strings.Join([]string{
		"| CP-X-1 ✅ | 맨 번호 | 읽는다 |",
		"| [CP-X-2] ✅ | 괄호만 | 읽는다 |",
		"| [CP-X-3](t_test.go) ✅ | 링크까지 | 읽는다 |",
		"| [**CP-X-4**] | 굵게가 괄호 안 | 읽는다 |",
		"| **[CP-X-5] ✅** | 굵게가 괄호 밖 | 읽는다 |",
	}, "\n"))
	if len(cases) != 5 {
		t.Fatalf("다섯 행을 모두 읽어야 한다: %d개 %v", len(cases), cases)
	}
	for i, linked := range []bool{false, false, true, false, false} {
		if (cases[i].link != "") != linked {
			t.Errorf("%d번째 행의 링크 유무가 다르다: %q", i, cases[i].link)
		}
	}
	if !cases[3].boldIn || !cases[4].boldOut {
		t.Error("괄호만 씌운 번호에서 굵게의 자리를 못 읽었다")
	}
	if cases[1].status != "✅" {
		t.Errorf("괄호만 씌운 번호의 상태 표시를 못 읽었다: %q", cases[1].status)
	}

	tests := map[string][]string{}
	for i := 1; i <= 5; i++ {
		tests["CP-X-"+string(rune('0'+i))] = []string{"t_test.go"}
	}
	rewrite(d, lines, cases, tests)
	for i, want := range []string{
		"| [CP-X-1](t_test.go) ✅ |",
		"| [CP-X-2](t_test.go) ✅ |",
		"| [CP-X-3](t_test.go) ✅ |",
		"| [**CP-X-4**](t_test.go) |",
		"| **[CP-X-5](t_test.go) ✅** |",
	} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("%d번째 줄을 링크로 못 바꿨다: %s", i, lines[i])
		}
	}
}
