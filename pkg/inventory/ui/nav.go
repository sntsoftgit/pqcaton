package ui

import "fmt"

// 화면 주소. **이동 링크와 제목이 같은 값을 본다** — 문자열을 두 곳에 적으면 한쪽만
// 옮겨지는 날 「지금 여기」 표시가 사라진다.
const (
	ScreenDecl      = "/decl"
	ScreenScope     = "/scope"
	ScreenSurvey    = "/survey"
	ScreenReview    = "/review"
	ScreenInventory = "/inventory"

	// ScreenDeclNext — 선언 화면의 다음 판. **이동 링크에는 넣지 않는다** — 지금 쓰는
	// 절차 화면과 나란히 두면 어느 쪽이 진짜인지 헷갈린다. 옮기는 동안만 주소로 연다.
	ScreenDeclNext = "/decl-next"
)

// Screens — 재료를 받아 열린 화면들.
//
// **재료를 주지 않은 자리는 만들지 않는다** — 없는 것을 눌러 보게 하지 않는다.
type Screens struct{ Decl, Scope, Survey, Inventory bool }

// NavFor — 위쪽 이동 링크. **절차 순서로 둔다** — 선언 → 자산 스코프 → 대조 → 판정.
// 쓰는 사람이 다음에 무엇을 할지 순서 자체가 말해 준다.
//
// 조립을 여기 둔 것은 컨트롤 플레인도 같은 화면을 쓰기 때문이다 — 거기서 탭 순서가
// 달라지면 같은 제품이 두 절차를 가르치게 된다.
func NavFor(l Lang, here string, s Screens) []Link {
	var links []Link
	if s.Decl {
		links = append(links, Link{Href: ScreenDecl, Text: tNavDecl.In(l), Here: here == ScreenDecl})
	}
	// **자산 스코프는 대조 앞이다.** 무엇을 계속 볼지가 정해져야 관측이 적재되고, 그 뒤에
	// 대조한다 — 절차가 그 순서다.
	if s.Scope {
		links = append(links, Link{Href: ScreenScope, Text: tNavScope.In(l), Here: here == ScreenScope})
	}
	if s.Survey {
		links = append(links, Link{Href: ScreenSurvey, Text: tNavSurvey.In(l), Here: here == ScreenSurvey})
	}
	links = append(links, Link{Href: ScreenReview, Text: tNavReview.In(l), Here: here == ScreenReview})
	// **번호를 붙이지 않는다.** 조회는 절차의 한 단계가 아니라 아무 때나 들어오는
	// 자리다 — 번호를 달면 ④ 다음에 해야 하는 일로 읽힌다.
	if s.Inventory {
		links = append(links, Link{Href: ScreenInventory, Text: tNavInventory.In(l), Here: here == ScreenInventory})
	}
	return links
}

// ScreenTitle — 그 화면의 이름. 탭 이름과 같게 둔다(번호만 뺀다).
func ScreenTitle(here string, l Lang) string {
	switch here {
	// 다음 판도 같은 화면이다 — 제목이 갈리면 브라우저 기록에서 둘이 다른 것으로 남는다.
	case ScreenDecl, ScreenDeclNext:
		return tTitleDecl.In(l)
	case ScreenScope:
		return tTitleScope.In(l)
	case ScreenSurvey:
		return tTitleSurvey.In(l)
	case ScreenInventory:
		return tTitleInventory.In(l)
	}
	return tTitleReview.In(l)
}

// 부제에 붙는 이름표. 부제는 「무엇을 보고 있나」라 값은 그대로 두고 이름만 옮긴다.
func LabelOrg(l Lang) string     { return tSubOrg.In(l) }
func LabelSession(l Lang) string { return tSubSession.In(l) }
func LabelResults(l Lang) string { return tSubResults.In(l) }

// Step — 절차 카드 하나. 다음 판의 껍데기가 위쪽에 넷을 늘어놓는다.
//
// **번호와 이름만으로는 어디부터 볼지 알 수 없다.** 그래서 카드마다 지금 상태를 한 줄로
// 달고, 점 색으로 급한 정도를 나눈다. 상태를 아직 세지 않는 화면은 그렇다고 적는다 —
// 빈 자리는 「할 일이 없다」로 읽힌다.
type Step struct {
	Num, Title, State string
	// Dot — 점 색을 고르는 이름(`ok`·`warn`·`danger`). 비면 점을 그리지 않는다.
	Dot  string
	Href string
	Here bool
	// Open — 재료가 있어 열리는 화면인가. 닫힌 카드는 누를 수 없다.
	Open bool
}

// StepsFor — 절차 카드 넷과 조회 하나.
//
// **상태는 옮긴 화면부터 채운다.** 카드 하나를 채우려면 그 화면의 계산을 매 요청마다
// 돌려야 하는데, 대조는 계산이 무겁고 한 화면의 재료가 어긋나면 다른 화면까지 막힌다.
// 그래서 화면을 옮기는 걸음마다 그 카드를 채운다(지금은 선언 하나다).
func StepsFor(l Lang, here string, s Screens, decl *DeclSummary) []Step {
	steps := []Step{
		{Num: "01", Title: tTitleDecl.In(l), Href: ScreenDeclNext, Open: s.Decl},
		{Num: "02", Title: tTitleScope.In(l), Href: ScreenScope, Open: s.Scope},
		{Num: "03", Title: tTitleSurvey.In(l), Href: ScreenSurvey, Open: s.Survey},
		{Num: "04", Title: tTitleReview.In(l), Href: ScreenReview, Open: true},
	}
	for i := range steps {
		steps[i].Here = here == steps[i].Href
		switch {
		case !steps[i].Open:
			steps[i].State = tStepClosed.In(l)
		case i == 0 && decl != nil:
			steps[i].State, steps[i].Dot = declState(l, *decl)
		default:
			steps[i].State = tStepUnknown.In(l)
		}
	}
	return steps
}

// declState — 선언 카드의 한 줄. 붙지 않은 이름이 그 화면의 급한 일이다.
func declState(l Lang, s DeclSummary) (string, string) {
	if s.Unlinked == 0 {
		return tStepDeclDone.In(l), "ok"
	}
	return fmt.Sprintf(tStepDeclOpen.In(l), s.Unlinked), "warn"
}
