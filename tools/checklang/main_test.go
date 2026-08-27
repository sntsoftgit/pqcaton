package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// page — site/ 아래에 장 하나를 놓는다. 관문이 디렉터리로 잡으므로 자리도 그대로 맞춘다.
func page(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "site")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "<!doctype html>\n<html lang=\"ko\" data-lang=\"ko\">\n<body>\n" + body + "\n</body>\n</html>\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// IC-B1 — **닫는 태그가 빠져 한 말이 다른 말을 품은 자리를 잡는다.**
//
// 2026-08-27 에 실제로 난 고장의 모양입니다. `</span>` 하나가 없으면 파서가 뒤따르는
// 영어를 한국어 안으로 넣고, 영어로 열 때 바깥이 숨겨지면서 안쪽까지 사라집니다.
// **한국어로 보면 멀쩡해서** 사람 눈으로는 걸러지지 않던 자리입니다.
func TestNestedByMissingCloseTag(t *testing.T) {
	root := t.TempDir()
	page(t, root, "a.html", `<table><tr><th><span lang="ko">무엇을 위해
  <span lang="en">What for</span></th></tr></table>`)
	got, _, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].kind != nested {
		t.Fatalf("닫는 태그가 빠진 자리를 못 잡았다: %+v", got)
	}
}

// IC-B2 — **짝이 없는 자리를 잡는다.** 한 말만 적힌 자리는 다른 말로 열면 빕니다.
func TestUnpaired(t *testing.T) {
	root := t.TempDir()
	page(t, root, "a.html", `<p><span lang="ko">한국어만 있다</span></p>`)
	got, _, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].kind != unpaired || got[0].lang != "ko" {
		t.Fatalf("짝 없는 자리를 못 잡았다: %+v", got)
	}
}

// IC-B3 — **나란히 적힌 올바른 짝은 통과시킨다.** 관문이 오탐을 내면 아무도 안 쓴다.
// 실제 두 장의 요소 오백두 개가 모두 이 모양입니다.
func TestPairedPasses(t *testing.T) {
	root := t.TempDir()
	page(t, root, "a.html", `<table><tr>
  <th><span lang="ko">상태</span><span lang="en">State</span></th>
  <th><span lang="ko">정의</span>
  <span lang="en">Definition</span></th></tr></table>
<p><span lang="ko">본문입니다</span>
<span lang="en">This is the body</span></p>`)
	got, n, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("멀쩡한 짝을 잡았다: %+v", got)
	}
	if n != 1 {
		t.Errorf("장을 %d개 셌다 — 하나여야 한다", n)
	}
}

// IC-B4 — **`<html lang="ko">` 는 세지 않는다.**
//
// 그 자리는 문서 전체의 초깃값이라 짝이 없는 것이 맞습니다. 스크립트가 막혀도 한국어가
// 보이게 하는 자리라서, 짝을 요구하면 두 장이 늘 막힙니다.
func TestRootLangIsNotCounted(t *testing.T) {
	root := t.TempDir()
	page(t, root, "a.html", `<p><span lang="ko">가</span><span lang="en">A</span></p>`)
	got, _, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("루트의 lang 을 짝 없는 자리로 셌다: %+v", got)
	}
}

// IC-B5 — **`ko`·`en` 이 아닌 값을 잡는다.**
//
// CSS 두 줄은 그 두 값에만 걸립니다. `kr` 처럼 오타가 나면 어느 규칙도 걸지 못해서
// **두 말에서 모두 보입니다** — 숨겨지는 고장보다 알아채기 어렵습니다.
func TestUnknownLang(t *testing.T) {
	root := t.TempDir()
	page(t, root, "a.html", `<p><span lang="kr">한국어</span><span lang="en">Korean</span></p>`)
	got, _, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].kind != unknown || got[0].lang != "kr" {
		t.Fatalf("모르는 말을 못 잡았다: %+v", got)
	}
}

// IC-B6 — **중첩은 바깥만 짚는다.**
//
// 안쪽까지 세면 원인이 하나인 고장이 두 줄로 나옵니다(`nested` 와 `unpaired`). 처음
// 판에서 실제로 그랬고, 아홉 자리가 열여덟 줄로 나와 무엇을 고쳐야 하는지 흐려졌습니다.
// 바깥의 닫는 태그를 넣으면 둘 다 풀립니다.
func TestNestedReportedOnce(t *testing.T) {
	root := t.TempDir()
	page(t, root, "a.html", `<th><span lang="ko">하는 일
  <span lang="en">What it does</span></th>`)
	got, _, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("한 자리를 %d줄로 냈다: %+v", len(got), got)
	}
}

// IC-B7 — **줄 번호가 원본의 실제 줄을 짚는다.**
//
// 처음 판은 품은 글자를 원본에서 되찾는 방식이었는데, 「정의」·「등급」처럼 짧고 흔한
// 낱말이 문서 앞쪽의 다른 자리에 먼저 걸려 **엉뚱한 줄을 짚었습니다**(437 을 357 로,
// 438 을 242 로). 그래서 토크나이저로 태그가 실제로 나온 줄을 센다.
func TestLineNumberIsTheRealLine(t *testing.T) {
	root := t.TempDir()
	// 같은 낱말을 앞쪽에 멀쩡한 짝으로 한 번 두고, 뒤쪽에서 고장을 낸다.
	page(t, root, "a.html", `<p><span lang="ko">정의</span><span lang="en">Definition</span></p>
<p>사이를 벌리는 줄</p>
<th><span lang="ko">정의
  <span lang="en">Definition</span></th>`)
	got, _, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("잡은 것 %d개: %+v", len(got), got)
	}
	// <!doctype>·<html>·<body> 가 세 줄, 앞의 두 문단이 두 줄이라 고장은 여섯째 줄이다.
	if got[0].line != 6 {
		t.Errorf("줄이 %d — 앞쪽의 같은 낱말이 아니라 실제 자리를 짚어야 한다", got[0].line)
	}
}

// IC-B8 — **줄을 못 맞추면 아예 내지 않는다.**
//
// 트리에서 만난 순서와 원본의 태그 순서가 어긋나면 짝이 흔들립니다. 그때는 **어긋난
// 줄을 내느니 파일 이름만** 보이는 편이 낫습니다. 표 밖의 텍스트처럼 파서가 태그를
// 옮겨 놓는 자리가 그런 경우다.
func TestLineDroppedWhenCountsDisagree(t *testing.T) {
	src := "<p><span lang=\"ko\">가</span></p>"
	if got := lineIndex(src); len(got) != 1 || got[0] != 1 {
		t.Fatalf("한 줄짜리 원본에서 %v — [1] 이어야 한다", got)
	}
	// 여는 태그가 없으면 셀 것도 없다.
	if got := lineIndex("<p>글자만 있다</p>"); len(got) != 0 {
		t.Errorf("lang 이 없는데 %v 를 셌다", got)
	}
}

// IC-B9 — **site/ 에 장이 하나도 없으면 통과가 아니라 오류다.**
//
// 관문이 「볼 것이 없어서 통과」라고 말하면, 경로가 어긋난 날 아무것도 재지 않으면서
// 통과 표시만 낸다. 막을 것을 못 막는 관문은 없는 것만 못하다.
func TestNoPagesIsAnError(t *testing.T) {
	if _, _, err := check(t.TempDir()); err == nil {
		t.Fatal("장이 없는데 통과했다 — 오류여야 한다")
	} else if !strings.Contains(err.Error(), "no pages") {
		t.Errorf("오류 내용이 %q — 장이 없다는 말이어야 한다", err)
	}
}
