package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func mustRules(t *testing.T, lines ...string) []rule {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "rules.tsv")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs, err := loadRules(p)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func hitsOf(rel string, orig []byte, masked []byte, rs []rule) []hit {
	return match(rel, orig, masked, rs)
}

// IC-K1 — **코드 블록 안은 보지 않는다.**
//
// 지침이 「인용, 코드, 코드 주석에는 적용하지 않는다」고 못 박아 두었다. 코드 블록의
// 엠대시나 영어 낱말까지 세면 관문이 고칠 수 없는 것을 요구하게 되고, 그러면 아무도
// 켜지 않는다.
func TestFencedCodeIsNotCounted(t *testing.T) {
	rs := mustRules(t, "엠대시\t—\t콜론으로")
	src := []byte("본문에 하나 —\n```\n코드 안에 —\n```\n다시 본문\n")
	got := hitsOf("a.md", src, maskMarkdown(src), rs)
	if len(got) != 1 {
		t.Fatalf("코드 블록 안까지 셌다: %d건 %v", len(got), got)
	}
	if got[0].line != 1 {
		t.Errorf("줄 번호가 어긋났다: %d", got[0].line)
	}
}

// IC-K2 — **인라인 코드도 덮는다.** 경로와 플래그가 백틱 안에 들어 있는 문서라, 덮지
// 않으면 `-machine` 같은 이름이 문체 위반으로 잡힌다.
func TestInlineCodeIsNotCounted(t *testing.T) {
	rs := mustRules(t, "머신\t머신\t기계")
	src := []byte("`머신` 은 코드다. 그런데 머신이라고 적었다.\n")
	got := hitsOf("a.md", src, maskMarkdown(src), rs)
	if len(got) != 1 {
		t.Fatalf("인라인 코드까지 셌다: %d건 %v", len(got), got)
	}
}

// IC-K3 — **「헷갈리다」를 「갈리다」로 잡지 않는다.**
//
// RE2 에 뒤보기가 없어 「갈리는」 하나로 재면 「헷갈리는」이 함께 걸린다. 실제 문서에
// 그 자리가 여섯 곳 있었다. 잘못 잡는 관문은 목록에 예외를 쌓게 만들고, 예외가 쌓이면
// 진짜 위반도 함께 묻힌다.
func TestHetgalliDoesNotTripGalli(t *testing.T) {
	rs := mustRules(t, "갈리다\t갈립니|갈리면|갈리는|갈려\t서로 다르다")
	src := []byte("헷갈리는 자리가 남습니다.\n말이 갈리면 곤란합니다.\n")
	got := hitsOf("a.md", src, maskMarkdown(src), rs)
	if len(got) != 1 {
		t.Fatalf("헷갈리는을 함께 잡았다: %d건 %v", len(got), got)
	}
	if got[0].line != 2 {
		t.Errorf("줄 번호가 어긋났다: %d", got[0].line)
	}
}

// IC-K4 — **Go 는 문자열 리터럴만 본다.**
//
// 주석은 한국어로 적는 것이 이 리포의 규칙이다(CONTRIBUTING 「어느 말로 쓰나」). 주석까지
// 막으면 판단 근거를 적어 두는 방식이 통째로 막힌다. checktext 가 반대 방향으로 같은
// 선을 긋고 있으므로 여기서도 그 선을 지킨다.
func TestGoCommentsAreNotCounted(t *testing.T) {
	rs := mustRules(t, "엠대시\t—\t콜론으로")
	dir := t.TempDir()
	p := filepath.Join(dir, "text.go")
	src := "package ui\n\n// 주석에 엠대시 —\nvar s = \"화면 문구에 엠대시 —\"\n"
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, masked, err := maskGo(p)
	if err != nil {
		t.Fatal(err)
	}
	got := hitsOf("text.go", orig, masked, rs)
	if len(got) != 1 {
		t.Fatalf("주석까지 셌다: %d건 %v", len(got), got)
	}
	if got[0].line != 4 {
		t.Errorf("줄 번호가 어긋났다: %d", got[0].line)
	}
}

// IC-K5 — **덮어도 줄 번호와 원문이 살아 있다.**
//
// 덮기가 바이트 수를 바꾸면 걸린 자리를 알려 줄 수 없다. 그리고 보여 주는 글은 덮은 것이
// 아니라 원문이라야 한다: 덮인 줄을 찍으면 어느 문장인지 알아볼 수 없다.
func TestMaskingKeepsOffsetsAndShowsTheOriginal(t *testing.T) {
	for _, src := range []string{
		"가나다 `코드` 라라라 —\n",
		"```\n덮이는 줄\n```\n본문 —\n",
	} {
		if got := len(maskMarkdown([]byte(src))); got != len(src) {
			t.Errorf("바이트 수가 달라졌다: %d → %d (%q)", len(src), got, src)
		}
	}
	rs := mustRules(t, "엠대시\t—\t콜론으로")
	src := []byte("앞줄\n`코드` 를 쓴 줄에 엠대시 —\n")
	got := hitsOf("a.md", src, maskMarkdown(src), rs)
	if len(got) != 1 {
		t.Fatalf("%d건 %v", len(got), got)
	}
	if !strings.Contains(got[0].text, "`코드`") {
		t.Errorf("덮인 줄을 보여 줬다: %q", got[0].text)
	}
}

// IC-K6 — **기준선보다 늘면 막는다.** 이 관문이 있는 이유가 이 한 줄이다.
func TestGrowingPastTheBaselineFails(t *testing.T) {
	base := map[key]int{{"엠대시", "a.md"}: 3}
	now := map[key]int{{"엠대시", "a.md"}: 5}
	grown, shrunk := compare(base, now)
	if len(grown) != 1 {
		t.Fatalf("늘어난 것을 못 잡았다: %v", grown)
	}
	if len(shrunk) != 0 {
		t.Errorf("줄지 않았는데 줄었다고 했다: %v", shrunk)
	}
	if !strings.Contains(grown[0], "a.md") || !strings.Contains(grown[0], "+2") {
		t.Errorf("무엇이 얼마나 늘었는지 말하지 않는다: %s", grown[0])
	}
}

// IC-K7 — **줄어도 막는다.**
//
// 고쳐 놓고 기준선을 안 내리면 그 자리가 도로 채워져도 알 수 없다. 「절차의 한 걸음」을
// 하루 걷어냈다가 다음 날 릴리스 노트에 되살린 적이 있어, 내려가는 쪽도 잠근다.
func TestShrinkingAlsoFailsUntilTheBaselineMovesDown(t *testing.T) {
	base := map[key]int{{"조용히", "a.md"}: 4}
	now := map[key]int{}
	grown, shrunk := compare(base, now)
	if len(grown) != 0 {
		t.Errorf("늘지 않았는데 늘었다고 했다: %v", grown)
	}
	if len(shrunk) != 1 {
		t.Fatalf("줄어든 것을 못 잡았다: %v", shrunk)
	}
	if !strings.Contains(shrunk[0], "-4") {
		t.Errorf("얼마나 줄었는지 말하지 않는다: %s", shrunk[0])
	}
}

// IC-K8 — **기준선을 찍고 다시 읽으면 같아야 한다.** 찍는 쪽과 읽는 쪽이 어긋나면
// 관문이 매번 붉어지고, 그러면 기준선을 지우는 것으로 끝난다.
func TestBaselineRoundTrips(t *testing.T) {
	want := map[key]int{}
	want[key{"엠대시", "docs/design.md"}] = 55
	want[key{"조용히", "README.md"}] = 1
	p := filepath.Join(t.TempDir(), "baseline.tsv")
	if err := writeBaseline(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := readBaseline(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("항목 수가 다르다: %d → %d", len(want), len(got))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%v: %d → %d", k, v, got[k])
		}
	}
	grown, shrunk := compare(got, want)
	if len(grown) != 0 || len(shrunk) != 0 {
		t.Errorf("찍고 읽었더니 달라졌다: %v %v", grown, shrunk)
	}
}

// IC-K9 — **규칙표가 실제로 읽힌다.** 탭이 하나 빠지거나 정규식이 깨지면 관문이 아예
// 서지 않는데, 그 사실을 빌드가 알려 주지 않는다.
func TestShippedRulesLoad(t *testing.T) {
	rs, err := loadRules(filepath.Join("..", "..", rulesFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) < 5 {
		t.Fatalf("규칙이 너무 적다: %d개", len(rs))
	}
	seen := map[string]bool{}
	for _, r := range rs {
		if seen[r.name] {
			t.Errorf("이름이 겹친다: %s", r.name)
		}
		seen[r.name] = true
		if r.fix == "" {
			t.Errorf("%s: 무엇으로 바꿀지 적혀 있지 않다", r.name)
		}
	}
	if !seen["엠대시"] {
		t.Error("엠대시 규칙이 없다")
	}
}

// IC-K10 — **덮는 자리가 목록에 적혀 있다.** 화면 문구는 네 파일에 나뉘어 있어, 규약으로
// 두면 새 파일이 슬그머니 관문 밖이 된다. 목록에 있는 파일이 실제로 있는지 잰다.
func TestListedScreenFilesExist(t *testing.T) {
	for _, rel := range append(append([]string{}, extraGo...), extraHTML...) {
		if _, err := os.Stat(filepath.Join("..", "..", rel)); err != nil {
			t.Errorf("목록에 있는데 없는 파일이다: %s (%v)", rel, err)
		}
	}
}

// IC-K11 — **잘못 잡는 말을 덮어도 바이트 수가 그대로다.** 여기가 어긋나면 그 뒤의 모든
// 줄 번호가 밀린다.
func TestHideOverlapKeepsLength(t *testing.T) {
	for _, s := range []string{"헷갈리는", "헛갈리기 쉽다", "갈리면"} {
		if got := len(hideOverlap(s)); got != len(s) {
			t.Errorf("%q: 바이트 수가 달라졌다 %d → %d", s, len(s), got)
		}
	}
	if regexp.MustCompile("갈리는").MatchString(hideOverlap("헷갈리는")) {
		t.Error("헷갈리는을 덮지 못했다")
	}
}
