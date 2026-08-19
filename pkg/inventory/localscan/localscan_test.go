package localscan_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/localscan"
)

// IC-L5 — **`/proc` 을 못 열면 끊는다.**
//
// 관측 0건과 다르다. 0건은 「아무것도 안 쓰고 있다」일 수 있지만 이것은 「볼 수가 없었다」다.
// 그 상태로 대조하면 선언 자산이 전부 UNOBSERVED 로 나오고 리포트는 「못 본 것: 없습니다」
// 라고까지 말한다 — **관측을 아예 못 한 기계에서.** 이 리포가 내내 경계해 온 그것이다.
func TestCheckRefusesWithoutProc(t *testing.T) {
	_, err := localscan.Check(true, 0, 0)
	if !errors.Is(err, localscan.ErrNoProc) {
		t.Fatalf("끊지 않았다: %v", err)
	}
	// **무엇을 대신 해야 하는지 말해야 한다.** 「안 된다」만 말하면 사람은 멈춘다.
	if !strings.Contains(err.Error(), "pqcaton-report") {
		t.Errorf("대안을 말하지 않는다: %v", err)
	}
}

// IC-L6 — **접근 가능한 프로세스가 0이면 말하되 끊지는 않는다.**
//
// `/proc` 은 열렸으니 결과는 낼 수 있다. 다만 권한 때문에 하나도 못 읽었을 수 있으므로,
// 그 결과를 완전한 관측으로 보지 말라고 말한다.
func TestCheckWarnsWhenNothingReadable(t *testing.T) {
	warn, err := localscan.Check(false, 0, 42)
	if err != nil {
		t.Fatalf("끊었다: %v", err)
	}
	if warn == "" {
		t.Fatal("아무 말도 안 했다")
	}
	if !strings.Contains(warn, "자산이 없는 것이 아닙니다") {
		t.Errorf("「없다」와 「못 봤다」를 가르지 않는다: %s", warn)
	}
}

// IC-L7 — 정상 스캔은 조용하다. 막는 것만 재면 전부 막아도 케이스는 통과한다.
func TestCheckQuietWhenFine(t *testing.T) {
	warn, err := localscan.Check(false, 430, 68)
	if err != nil || warn != "" {
		t.Errorf("정상인데 말했다: warn=%q err=%v", warn, err)
	}
}

// IC-L8 — **다른 이름을 붙이면 경고한다.**
//
// 노드 이름은 이름표일 뿐 대상이 아니다. `pqcaton-decide open decl.csv web-gw` 는 web-gw 를
// 관측하는 것이 아니라 **이 기계를 관측해 web-gw 라고 적는다** — 이름이 맞으면 선언과
// 대조까지 되어 CONFIRMED 가 나온다. 다른 기계의 관측으로.
func TestLabelWarning(t *testing.T) {
	if w := localscan.LabelWarning("web-gw"); w == "" {
		t.Fatal("다른 이름을 붙였는데 조용하다")
	} else if !strings.Contains(w, "스캔한 것은 이 기계입니다") {
		t.Errorf("무엇이 문제인지 말하지 않는다: %s", w)
	}
	// 기본 이름은 이 기계라는 뜻이므로 경고할 것이 없다.
	if w := localscan.LabelWarning(localscan.DefaultNode); w != "" {
		t.Errorf("기본 이름인데 경고했다: %s", w)
	}
	if w := localscan.LabelWarning(""); w != "" {
		t.Errorf("이름을 안 줬는데 경고했다: %s", w)
	}
}
