package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDoc(t *testing.T, body string) (string, []docCase, []string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "testcases.md")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cases, lines, err := scanDoc(p)
	if err != nil {
		t.Fatal(err)
	}
	return p, cases, lines
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
	got := ids("// IC-R1·R2·R3 — 3-상태 대조\n// IC-F2·F3: 전이\n// IC-P1·P2: 계획\n// IC-K9 하나")
	want := []string{"IC-R1", "IC-R2", "IC-R3", "IC-F2", "IC-F3", "IC-P1", "IC-P2", "IC-K9"}
	if len(got) != len(want) {
		t.Fatalf("펴지 못했다: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d번째가 다르다: %s ≠ %s", i, got[i], want[i])
		}
	}
}

// IC-M2 — **번호 칸의 두 모양을 다 읽는다.** 굵은 것과 안 굵은 것이 섞여 있고, 링크가
// 이미 붙은 것도 있다. 하나라도 못 읽으면 그 케이스는 관문 밖이 된다.
func TestBothCellShapesAreRead(t *testing.T) {
	_, cases, _ := writeDoc(t, strings.Join([]string{
		"| IC-R1 ✅ | 자산이 선언 ∩ 관측 | CONFIRMED |",
		"| **IC-O1 ✅** | 다른 조직이 섞임 | 중단한다 |",
		"| [IC-S1](../pkg/inventory/scope/scope_test.go) ✅ | 상속 | 하위가 이긴다 |",
		"| IC-C3 ⏳ | 실측 | 나중 |",
		"**핵심 인수 기준**: **IC-P4**(표가 아니라 본문이다)",
	}, "\n"))
	if len(cases) != 4 {
		t.Fatalf("표 행만 넷이라야 한다: %d개 %v", len(cases), cases)
	}
	if !cases[1].bold {
		t.Error("굵은 것을 굵다고 읽지 않았다")
	}
	if cases[2].link != "../pkg/inventory/scope/scope_test.go" {
		t.Errorf("붙어 있는 링크를 못 읽었다: %q", cases[2].link)
	}
	if cases[3].status != "⏳" {
		t.Errorf("상태를 못 읽었다: %q", cases[3].status)
	}
}

// IC-M3 — **`-write` 는 굵게와 상태 표시를 지킨다.** 링크를 붙이면서 문서의 모양이 달라지면
// 사람이 그 diff 를 읽지 못하고, 읽지 못하면 다음부터 돌리지 않는다.
func TestWriteKeepsBoldAndStatus(t *testing.T) {
	_, cases, lines := writeDoc(t, strings.Join([]string{
		"| IC-R1 ✅ | 선언 ∩ 관측 | CONFIRMED |",
		"| **IC-O1 ✅** | 섞임 | 중단한다 |",
	}, "\n"))
	tests := map[string][]string{
		"IC-R1": {"pkg/inventory/reconcile/reconcile_test.go"},
		"IC-O1": {"pkg/inventory/reconcile/org_test.go"},
	}
	if n := rewrite(lines, cases, tests); n != 2 {
		t.Fatalf("둘 다 찍어야 한다: %d", n)
	}
	if lines[0] != "| [IC-R1](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 선언 ∩ 관측 | CONFIRMED |" {
		t.Errorf("안 굵은 줄: %s", lines[0])
	}
	if lines[1] != "| **[IC-O1](../pkg/inventory/reconcile/org_test.go) ✅** | 섞임 | 중단한다 |" {
		t.Errorf("굵은 줄: %s", lines[1])
	}
}

// IC-M4 — **찍은 것을 다시 읽어도 같다.** 두 번 돌렸을 때 링크가 겹쳐 쌓이면 문서가 망가진다.
func TestWriteIsIdempotent(t *testing.T) {
	_, cases, lines := writeDoc(t, "| **IC-O1 ✅** | 섞임 | 중단한다 |")
	tests := map[string][]string{"IC-O1": {"pkg/inventory/reconcile/org_test.go"}}
	rewrite(lines, cases, tests)
	first := lines[0]

	p := filepath.Join(t.TempDir(), "again.md")
	if err := os.WriteFile(p, []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}
	cases2, lines2, err := scanDoc(p)
	if err != nil {
		t.Fatal(err)
	}
	if n := rewrite(lines2, cases2, tests); n != 0 {
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
func TestShippedDocAndTestsAgree(t *testing.T) {
	root := filepath.Join("..", "..")
	tests, err := scanTests(root)
	if err != nil {
		t.Fatal(err)
	}
	cases, _, err := scanDoc(filepath.Join(root, docPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 100 {
		t.Fatalf("케이스를 너무 적게 읽었다: %d개", len(cases))
	}
	seen := map[string]bool{}
	for _, c := range cases {
		seen[c.id] = true
		if c.status == "✅" && len(tests[c.id]) == 0 {
			t.Errorf("%s 는 ✅ 인데 테스트에 번호가 없다", c.id)
		}
		if c.status != "✅" && len(tests[c.id]) > 0 {
			t.Errorf("%s 는 %s 인데 %s 가 그 번호를 달고 있다", c.id, c.status, tests[c.id][0])
		}
	}
	for id, files := range tests {
		if !seen[id] {
			t.Errorf("%s 가 %s 에 있는데 문서에 없다", id, files[0])
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
