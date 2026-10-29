// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

// 화면 주소. **이동 링크와 제목이 같은 값을 본다** — 문자열을 두 곳에 적으면 한쪽만
// 옮겨지는 날 「지금 여기」 표시가 사라진다.
const (
	ScreenDecl      = "/decl"
	ScreenScope     = "/scope"
	ScreenObserve   = "/observe"
	ScreenSurvey    = "/survey"
	ScreenReview    = "/review"
	ScreenInventory = "/inventory"
)

// screens6 — 화면 여섯. 순서는 절차 순서다.
var screens6 = []string{ScreenDecl, ScreenScope, ScreenObserve, ScreenSurvey, ScreenReview, ScreenInventory}

// AllScreens — 화면 여섯의 주소.
func AllScreens() []string { return append([]string(nil), screens6...) }

// MovedScreens — 옛 주소와 지금 주소. v0.21의 `-next`와 v0.22의 `/classic/…`이다. 옛 화면은 v0.23.0에서
// 없어졌고, 북마크가 죽지 않게 301로 넘긴다(질의는 부르는 쪽이 보존한다). 관측 화면은 v0.24.0에 생겨 옛 주소가 없다.
func MovedScreens() map[string]string {
	out := map[string]string{}
	for _, sc := range []string{ScreenDecl, ScreenScope, ScreenSurvey, ScreenReview, ScreenInventory} {
		out[sc+"-next"] = sc
		out["/classic"+sc] = sc
	}
	return out
}

// Screens — 입력을 받아 열린 화면들.
//
// **입력을 주지 않은 자리는 만들지 않는다** — 없는 것을 눌러 보게 하지 않는다.
type Screens struct {
	Decl, Scope, Survey, Inventory bool
	// Observe — 관측 화면. 작업 폴더 모드에서만 연다(접근 정보의 자리가 그 폴더 안에 정해져 있다).
	Observe bool
	// NoReview — 판정 화면을 닫는다. 작업 폴더에 선언이 아직 없으면 판정할 것이 없다(기본은 연다).
	NoReview bool
}

// ScreenTitle — 그 화면의 이름. 탭 이름과 같게 둔다(번호만 뺀다).
func ScreenTitle(here string, l Lang) string {
	switch here {
	case ScreenDecl:
		return tTitleDecl.In(l)
	case ScreenScope:
		return tTitleScope.In(l)
	case ScreenObserve:
		return tTitleObserve.In(l)
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

// Step — 절차 카드 하나. 다음 판의 껍데기가 위쪽에 늘어놓는다.
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
	// Open — 입력이 있어 열리는 화면인가. 닫힌 카드는 누를 수 없다.
	Open bool
	// Final — 확정하는 단계(스코프 · 판정)의 확정 상태. 다른 카드는 비어 있다.
	Final string
}

// Final — 확정 상태. 서버가 저장소의 판정을 옮겨 담는다. FinalUnknown은 일시적으로 판정하지 못한
// 것이다(잠금 대기 초과 · 읽기 실패). 「복구 필요」로 단정하지 않는다.
type Final string

const (
	FinalUnknown    Final = ""
	FinalNone       Final = "none"
	FinalDone       Final = "finalized"
	FinalSuperseded Final = "superseded"
	FinalRecovery   Final = "recovery"
)

// Locked: 영수증이 있다(확정됨 · 대체됨 · 복구 필요). 그 세션은 더 저장할 수 없으므로 화면을 읽기 전용으로 그린다.
func (f Final) Locked() bool { return f == FinalDone || f == FinalSuperseded || f == FinalRecovery }

func (f Final) text(l Lang) string {
	switch f {
	case FinalNone:
		return tFinalNone.In(l)
	case FinalDone:
		return tFinalDone.In(l)
	case FinalSuperseded:
		return tFinalSuperseded.In(l)
	case FinalRecovery:
		return tFinalRecovery.In(l)
	}
	return tStepUnknown.In(l)
}

// StepsFor — 절차 카드 다섯과 조회 하나.
//
// **모든 카드를 채운다**(v0.22.0). 서버가 요청 하나에서 입력을 한 번 읽어 본문과 카드가 같은
// 값을 쓴다. 한 화면의 입력을 읽지 못하면 그 카드만 「확인할 수 없음」이고 다른 카드와 본문은
// 그대로 선다.
func StepsFor(l Lang, here string, s Screens, st StepState) []Step {
	const (
		stepDecl = iota
		stepScope
		stepObserve
		stepSurvey
		stepReview
	)
	steps := []Step{
		{Num: "01", Title: tTitleDecl.In(l), Href: ScreenDecl, Open: s.Decl},
		{Num: "02", Title: tTitleScope.In(l), Href: ScreenScope, Open: s.Scope},
		{Num: "03", Title: tTitleObserve.In(l), Href: ScreenObserve, Open: s.Observe},
		{Num: "04", Title: tTitleSurvey.In(l), Href: ScreenSurvey, Open: s.Survey},
		{Num: "05", Title: tTitleReview.In(l), Href: ScreenReview, Open: !s.NoReview},
	}
	for i := range steps {
		steps[i].Here = here == steps[i].Href
		switch {
		case !steps[i].Open:
			steps[i].State = tStepClosed.In(l)
		case i == stepDecl && st.DeclMissing:
			steps[i].State, steps[i].Dot = tStepDeclMissing.In(l), "warn"
		case i == stepScope && st.LayersMissing:
			steps[i].State, steps[i].Dot = tStepLayersMissing.In(l), "warn"
		case i == stepDecl && st.Decl != nil:
			steps[i].State, steps[i].Dot = declState(l, *st.Decl)
		case i == stepScope && st.Scope != nil:
			steps[i].State, steps[i].Dot = scopeState(l, *st.Scope)
		case i == stepObserve && st.Observe != nil:
			steps[i].State, steps[i].Dot = observeState(l, *st.Observe)
		case i == stepSurvey && st.Survey != nil:
			steps[i].State, steps[i].Dot = surveyState(l, *st.Survey)
		case i == stepReview && st.Review != nil:
			steps[i].State, steps[i].Dot = reviewState(l, *st.Review)
		default:
			// 그 화면의 입력을 읽지 못했다. 원인은 서버가 표준오류에 남긴다.
			steps[i].State = tStepUnknown.In(l)
		}
		switch {
		case !steps[i].Open:
		case i == stepScope:
			steps[i].Final = st.ScopeFinal.text(l)
		case i == stepReview:
			steps[i].Final = st.ReviewFinal.text(l)
		}
	}
	return steps
}

// observeState — 관측 카드의 한 줄. 접근 정보가 없으면 관측을 시작할 수 없다.
func observeState(l Lang, s ObserveSummary) (string, string) {
	if s.Hosts == 0 {
		return tStepObserveNoHosts.In(l), "warn"
	}
	return tStepObserveHosts.F(l, s.Hosts), "ok"
}

// StepState — 카드에 채울 상태. 비어 있는(nil) 자리는 그 입력을 읽지 못한 것이고 「확인할 수
// 없음」으로 적힌다.
type StepState struct {
	Decl    *DeclSummary
	Scope   *ScopeSummary
	Observe *ObserveSummary
	Survey  *SurveySummary
	Review  *ReviewSummary
	// DeclMissing — 작업 폴더에 선언이 아직 없다(만들기를 낸다).
	DeclMissing bool
	// LayersMissing — 작업 폴더에 계층도 스코프 세션도 아직 없다(만들기를 낸다).
	LayersMissing bool
	// ScopeFinal · ReviewFinal — 확정하는 두 단계의 확정 상태.
	ScopeFinal, ReviewFinal Final
}

// reviewState — 판정 카드의 한 줄. **확정을 막는 것이 무엇인지** 알린다.
func reviewState(l Lang, s ReviewSummary) (string, string) {
	switch {
	case s.Ready():
		return tStepReviewReady.In(l), "ok"
	case s.Open == 0:
		return tStepReviewSign.In(l), "warn"
	default:
		return tStepReviewOpen.F(l, s.Mandatory), "danger"
	}
}

// surveyState — 대조 카드의 한 줄. **재수집 후보를 먼저 알린다** — 못 본 것을 없는 것으로
// 확정하는 실수가 이 도구가 막으려는 바로 그 자리다.
func surveyState(l Lang, s SurveySummary) (string, string) {
	if s.Rescan > 0 {
		return tStepSurveyRescan.F(l, s.ToJudge, s.Rescan), "danger"
	}
	if s.ToJudge == 0 {
		return tStepSurveyDone.In(l), "ok"
	}
	return tStepSurveyOpen.F(l, s.ToJudge), "warn"
}

// scopeState — 스코프 카드의 한 줄. **근거가 필요한 것을 따로 센다** — 다 승인해 놓고
// 확정이 막히는 자리가 거기다.
func scopeState(l Lang, s ScopeSummary) (string, string) {
	if s.Changes == 0 {
		return tStepScopeDone.In(l), "ok"
	}
	if s.Audited > 0 {
		return tStepScopeAudited.F(l, s.Changes, s.Audited), "danger"
	}
	return tStepScopeOpen.F(l, s.Changes), "warn"
}

// declState — 선언 카드의 한 줄. 붙지 않은 이름이 그 화면의 급한 일이다.
func declState(l Lang, s DeclSummary) (string, string) {
	if s.Problems > 0 {
		return tStepDeclProblems.F(l, s.Problems), "warn"
	}
	if s.Unlinked == 0 {
		return tStepDeclDone.In(l), "ok"
	}
	return tStepDeclOpen.F(l, s.Unlinked), "warn"
}

// InventoryLabel — 조회 탭에 적을 이름. **번호를 붙이지 않는다**(NavFor와 같은 선).
func InventoryLabel(l Lang) string { return tNavInventory.In(l) }

// reloadHref — 「다시 불러오기」가 받는 주소. 자리를 모르면 새 선언 화면이다.
func reloadHref(l Lang, self string) string {
	if self == "" {
		self = ScreenDecl
	}
	return self + "?" + LangParam + "=" + string(l)
}

// Mode — 서버의 저장 모드. **머리표는 실제 동작과 같아야 한다**: 저장하는 화면에 「읽기」라고 적으면
// 사람은 무엇이 파일에 남는지 알 수 없다.
type Mode string

const (
	ModeWrite    Mode = "write"
	ModeViewOnly Mode = "view-only"
	ModeRecovery Mode = "recovery"
	// ModeDiagnostic — 작업 폴더의 입력을 읽지 못해 진단 전용으로 열었다.
	ModeDiagnostic Mode = "diagnostic"
)

func (m Mode) text(l Lang) string {
	switch m {
	case ModeViewOnly:
		return tModeViewOnly.In(l)
	case ModeRecovery:
		return tModeRecovery.In(l)
	case ModeDiagnostic:
		return tModeDiagnostic.In(l)
	}
	return tModeWrite.In(l)
}
