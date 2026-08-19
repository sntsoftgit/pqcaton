package ui

// 화면 주소. **이동 링크와 제목이 같은 값을 본다** — 문자열을 두 곳에 적으면 한쪽만
// 옮겨지는 날 「지금 여기」 표시가 사라진다.
const (
	ScreenDecl      = "/decl"
	ScreenScope     = "/scope"
	ScreenSurvey    = "/survey"
	ScreenReview    = "/review"
	ScreenInventory = "/inventory"
)

// Screens — 재료를 받아 열린 화면들.
//
// **재료를 주지 않은 자리는 만들지 않는다** — 없는 것을 눌러 보게 하지 않는다.
type Screens struct{ Decl, Scope, Survey, Inventory bool }

// NavFor — 위쪽 이동 링크. **절차 순서로 둔다** — 선언 → 자산 스코프 → 대조 → 리뷰 큐.
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
	// **번호를 붙이지 않는다.** 조회는 절차의 한 걸음이 아니라 아무 때나 들어오는
	// 자리다 — 번호를 달면 ④ 다음에 해야 하는 일로 읽힌다.
	if s.Inventory {
		links = append(links, Link{Href: ScreenInventory, Text: tNavInventory.In(l), Here: here == ScreenInventory})
	}
	return links
}

// ScreenTitle — 그 화면의 이름. 탭 이름과 같게 둔다(번호만 뺀다).
func ScreenTitle(here string, l Lang) string {
	switch here {
	case ScreenDecl:
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
