package ui

import (
	"context"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

// InventoryView — **찾아보는 자리.**
//
// 절차(선언 → 스코프 → 대조 → 리뷰)의 한 단계가 아니다. 대조 화면은 지금 대조한 것을
// 전부 늘어놓는데, 수천 대에서는 그 자체가 못 쓰는 화면이 된다 — 여기서는 좁혀 본다.
//
// **경계 안에서만 한다.** 여기 있는 것은 전부 손에 든 파일에서 나온다(관측 결과 · 판정
// 원장 · 정책 CSV). 스냅샷을 여러 개 보관해 시간축으로 견주는 일은 컨트롤 플레인이다.
type InventoryView struct {
	Page
	Filter Filter

	Assets []AssetRow
	Edges  []EdgeRow
	// Total — 좁히기 전 전체 수. **몇 개 중 몇 개를 보고 있는지** 말하지 않으면
	// 걸러 낸 것을 전부로 읽는다.
	TotalAssets, TotalEdges int

	// Unseen — 자산 스코프가 뺀 것. 「내가 뭘 안 보고 있나」.
	Unseen []UnseenRow
	// Stale — 근거가 바뀐 판정. 재관측 뒤 다시 볼 것.
	Stale []JudgmentRow
	// Subject · History — 자산 하나를 열었을 때 그 자산의 판정 이력.
	Subject string
	History []JudgmentRow

	// 재료를 주지 않은 절은 만들지 않는다.
	HasLedger, HasPolicy bool
}

// Filter — 좁히는 조건. **주소에 실린다** — 그래야 그 화면을 그대로 남에게 보낼 수 있다.
type Filter struct {
	Q     string // 노드 · 런타임 · 컴포넌트 · 상대에 걸리는 자유 문자열
	State string // CONFIRMED · UNDECLARED · UNOBSERVED · "" (전부)
}

// UnseenRow — 정책이 뺀 자산 한 줄.
type UnseenRow struct {
	Subject, Evidence string
	// StillObserved — 지금도 관측되는가. **뺐다고 사라진 것이 아니다.**
	StillObserved bool
	// Reason — 다시 봐야 하는 사유 코드. 비면 살아 있는 승인이 있다는 뜻이다.
	Reason string
}

// JudgmentRow — 판정 하나.
type JudgmentRow struct {
	Subject, Conclusion, Reviewer string
	// DecidedAt — 사람이 읽는 시각(RFC3339). 판정 원장이 시간축을 갖는 유일한 자리다.
	DecidedAt string
	Basis     string
}

// NewInventoryView — 손에 든 것으로 조회 화면을 세운다.
func NewInventoryView(r *report.Result, f Filter, page Page) InventoryView {
	v := InventoryView{Page: page, Filter: f}
	full := NewSurveyView(r, page)
	v.TotalAssets, v.TotalEdges = len(full.Assets), len(full.Edges)
	for _, a := range full.Assets {
		if f.match(a.State, a.Node, a.Runtime, a.Component) {
			v.Assets = append(v.Assets, a)
		}
	}
	for _, e := range full.Edges {
		if f.match(e.State, e.Src, e.Dst, e.Proto, e.Group) {
			v.Edges = append(v.Edges, e)
		}
	}
	return v
}

// match — 좁히기. 자유 문자열은 **아무 칸에나** 걸린다 — 어느 칸인지 미리 고르게 하면
// 찾는 사람이 그 칸을 알아야 한다.
func (f Filter) match(state string, fields ...string) bool {
	if f.State != "" && !strings.EqualFold(state, f.State) {
		return false
	}
	if f.Q == "" {
		return true
	}
	q := strings.ToLower(f.Q)
	for _, x := range fields {
		if strings.Contains(strings.ToLower(x), q) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(state), q)
}

// FilterFrom — 주소에서 조건을 읽는다.
func FilterFrom(q url.Values) Filter {
	return Filter{
		Q:     strings.TrimSpace(q.Get("q")),
		State: strings.ToUpper(strings.TrimSpace(q.Get("state"))),
	}
}

// WithUnseen — 정책이 뺀 자산을 얹는다. 다시 볼 것에는 사유가 붙는다.
func (v InventoryView) WithUnseen(ex []scope.Excluded, need []scope.ReviewItem) InventoryView {
	v.HasPolicy = true
	why := map[string]string{}
	for _, n := range need {
		why[n.Subject()] = n.Reason
	}
	for _, e := range ex {
		if !v.Filter.match("", e.Node, e.Runtime, e.Asset) {
			continue
		}
		v.Unseen = append(v.Unseen, UnseenRow{
			Subject: e.Subject(), Evidence: e.Evidence,
			StillObserved: e.StillObserved, Reason: why[e.Subject()],
		})
	}
	return v
}

// WithLedger — 판정 원장을 얹는다.
//
//   - Stale: 근거가 바뀐 것. **재관측이 판정을 뒤집을 수 있다**는 사실이 여기서 보인다.
//   - History: 자산 하나를 열었을 때 그 자산에 내려진 판정 전부.
func (v InventoryView) WithLedger(all []decision.Judgment, currentBasis map[string]string, subject string) InventoryView {
	v.HasLedger = true
	for _, j := range decision.DeltaReview(all, currentBasis) {
		v.Stale = append(v.Stale, judgmentRow(j))
	}
	v.Subject = subject
	if subject == "" {
		return v
	}
	var mine []decision.Judgment
	for _, j := range all {
		if j.Subject == subject {
			mine = append(mine, j)
		}
	}
	// **새것이 위로.** 이력을 여는 이유는 대개 「지금 왜 이런가」이다.
	sort.SliceStable(mine, func(i, j int) bool { return mine[i].DecidedAt > mine[j].DecidedAt })
	for _, j := range mine {
		v.History = append(v.History, judgmentRow(j))
	}
	return v
}

// RenderInventory — 조회 화면을 쓴다.
func RenderInventory(w io.Writer, v InventoryView) error {
	return inventoryPage(v).Render(context.Background(), w)
}

// judgmentRow — 판정 하나를 화면이 보는 모양으로.
//
// **시각을 사람이 읽는 꼴로 옮긴다.** 원장에는 unix 초로 들어 있는데, 그대로 두면
// 「언제 정했나」를 눈으로 셀 수 없다 — 조회 화면의 시간축이 이것뿐이다.
func judgmentRow(j decision.Judgment) JudgmentRow {
	at := ""
	if j.DecidedAt > 0 {
		at = time.Unix(j.DecidedAt, 0).UTC().Format(time.RFC3339)
	}
	return JudgmentRow{
		Subject: j.Subject, Conclusion: j.Conclusion, Reviewer: j.Reviewer,
		DecidedAt: at, Basis: j.BasisHash,
	}
}

// 조회 화면 안의 주소들. **좁힌 조건을 잃지 않는다** — 이력을 열었다가 돌아왔을 때
// 처음부터 다시 좁히게 하면 아무도 이력을 안 연다.
func (v InventoryView) query(extra map[string]string) string {
	q := url.Values{}
	if v.Filter.Q != "" {
		q.Set("q", v.Filter.Q)
	}
	if v.Filter.State != "" {
		q.Set("state", v.Filter.State)
	}
	q.Set(LangParam, string(v.Page.Lang))
	for k, val := range extra {
		if val == "" {
			q.Del(k)
			continue
		}
		q.Set(k, val)
	}
	return "/inventory?" + q.Encode()
}

func subjectHref(v InventoryView, subject string) string {
	return v.query(map[string]string{"subject": subject})
}

func clearSubjectHref(v InventoryView) string { return v.query(map[string]string{"subject": ""}) }

// historyHref — 자산 한 줄에서 그 자산의 이력으로. 원장의 대상 키와 같은 모양이어야
// 한다 — 다르면 이력이 늘 비어 보인다.
func historyHref(v InventoryView, a AssetRow) string {
	return subjectHref(v, a.Node+"/"+a.Runtime+"/"+a.Component)
}

// shortHash — 근거 해시는 눈으로 견주는 값이지 읽는 값이 아니다.
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12] + "…"
	}
	return h
}

// reasonLabel — 다시 볼 사유를 그 말로. **영어는 scope 패키지가 갖는다.**
func reasonLabel(l Lang, code string) string {
	switch code {
	case "":
		return tReasonSettled.In(l)
	case scope.ReasonNeverJudged:
		return tReasonNever.In(l)
	case scope.ReasonStale:
		return tReasonStaleKO.In(l)
	}
	return scope.EnglishReason(code)
}
