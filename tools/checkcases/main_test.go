package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDoc — 케이스 표 한 조각을 임시 문서로 쓰고 읽어 온다. `kinds` 를 주지 않으면 셋 다 맡는다.
func writeDoc(t *testing.T, body string, kinds ...string) (docSpec, []docCase, []string) {
	t.Helper()
	if len(kinds) == 0 {
		kinds = []string{"IC", "CP", "RUN"}
	}
	d := docSpec{path: filepath.Join(t.TempDir(), "testcases.md"), kinds: kinds}
	if err := os.WriteFile(d.path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cases, lines, err := scanDoc(d)
	if err != nil {
		t.Fatal(err)
	}
	return d, cases, lines
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

// IC-M3 — **`-write` 는 굵게와 상태 표시를 지킨다.** 링크를 붙이면서 문서의 모양이 달라지면
// 사람이 그 diff 를 읽지 못하고, 읽지 못하면 다음부터 돌리지 않는다.
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
	tests, err := scanTests(root)
	if err != nil {
		t.Fatal(err)
	}
	owned, seen, total := map[string]bool{}, map[string]bool{}, 0
	for _, d := range docs {
		for _, k := range d.kinds {
			owned[k] = true
		}
		cs, _, err := scanDoc(docSpec{path: filepath.Join(root, d.path), kinds: d.kinds})
		if err != nil {
			t.Fatal(err)
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
// 테스트 파일로 링크됐다. checktext 가 반대 방향으로 겪은 것과 같은 일이고 답도 같다:
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

// IC-M8 — **문서는 자기가 맡은 접두어만 읽는다.**
//
// 러너 케이스의 테스트는 이 리포에 있고 컨트롤 플레인 케이스의 테스트는 비공개 리포에 있다.
// 문서마다 맡는 접두어를 적어 두지 않으면, 남의 리포에 있는 테스트를 「없다」고 막게 된다.
func TestDocOnlyReadsItsOwnKinds(t *testing.T) {
	body := strings.Join([]string{
		"| [IC-R1](x) ✅ | 인벤토리 | CONFIRMED |",
		"| [RUN-2](y) | 러너 | 묻지 않는다 |",
	}, "\n")
	_, both, _ := writeDoc(t, body)
	if len(both) != 2 {
		t.Fatalf("둘 다 맡으면 둘 다 읽어야 한다: %v", both)
	}
	_, only, _ := writeDoc(t, body, "IC")
	if len(only) != 1 || only[0].id != "IC-R1" {
		t.Fatalf("IC 만 맡으면 IC 만 읽어야 한다: %v", only)
	}
}

// IC-M9 — **링크는 그 문서에서 본 상대 경로다.** 케이스 표가 docs/ 에도 있고 테스트 바로
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
