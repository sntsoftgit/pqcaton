package ui

import (
	"context"
	"io"
	"sort"

	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
)

// SurveyView — 대조 결과 화면이 보는 것.
//
// **관측을 먼저 보인다.** 대조 결과만 보면 UNOBSERVED가 「없다」인지 「원리상 못 봤다」인지
// 읽는 사람이 가를 수 없다(§2.7 갭 ≠ 부재) — 그 답이 관측 절에 있다.
type SurveyView struct {
	Page
	R *report.Result

	Confirmed, Undeclared, Unobserved int
	PQC, Classical, Unknown           int

	Assets []AssetRow
	Edges  []EdgeRow

	// DOT — 토폴로지 원문. `dot` 이 없는 기계에서도 **무엇을 그리려 했는지는 보여 준다.**
	DOT string
	// SVG — 그려진 토폴로지. `dot` 이 있으면 채운다. 우리가 만든 DOT 에서 `dot` 이
	// 낸 것이라 밖에서 온 값이 아니다 — 그래서 그대로 내보낸다.
	SVG string
}

// AssetRow — 자산 한 줄.
type AssetRow struct {
	Node, Runtime, Component, State string
	Conf                            float64
	Rescan                          bool
}

// EdgeRow — 엣지 한 줄.
type EdgeRow struct {
	Src, Dst, Proto, State string
	Port                   uint32
	// Mark — 등급 표시. 색이 아니라 기호로 둔다 — 흑백으로 인쇄해도 읽힌다.
	Mark, Grade string
	Group       string
	OffScope    bool
	// Uncovered — 보내는 쪽을 관측하지 못했다. **부재가 아니라 미관측**이다.
	Uncovered bool
}

// NewSurveyView — 대조 결과를 화면이 보는 모양으로 옮긴다.
func NewSurveyView(r *report.Result, page Page) SurveyView {
	v := SurveyView{Page: page, R: r}
	v.Confirmed, v.Undeclared, v.Unobserved = r.Counts()
	v.PQC, v.Classical, v.Unknown = r.Postures()

	for _, rec := range r.Assets {
		v.Assets = append(v.Assets, AssetRow{
			Node: rec.Key.NodeID, Runtime: rec.Key.Runtime, Component: rec.Key.Component,
			State: string(rec.State), Conf: rec.Confidence, Rescan: rec.RescanCandidate,
		})
	}
	// **필수 리뷰가 위로.** UNDECLARED 가 이 도구가 주는 첫 번째 쓸모라 맨 앞에 있어야 한다(§3.3②).
	sort.SliceStable(v.Assets, func(i, j int) bool {
		return statePriority(v.Assets[i].State) < statePriority(v.Assets[j].State)
	})

	for _, e := range r.Edges {
		mark, grade := posture(e.Posture)
		v.Edges = append(v.Edges, EdgeRow{
			Src: e.Key.Src, Dst: e.Key.Dst, Port: e.Key.Port, Proto: e.Key.Proto,
			State: string(e.State), Mark: mark, Grade: grade, Group: e.Group,
			OffScope: e.OffScopeDst, Uncovered: r.Uncovered[e.Key.Src],
		})
	}
	sort.SliceStable(v.Edges, func(i, j int) bool { return v.Edges[i].Src < v.Edges[j].Src })

	v.DOT = reconcile.RenderTopologyDOT(r.Edges, r.Uncovered)
	return v
}

// statePriority — 리뷰가 필요한 것부터. 자동으로 넘어가는 CONFIRMED 가 위에 오면 정작 볼
// 것이 아래로 밀린다.
func statePriority(s string) int {
	switch s {
	case string(reconcile.Undeclared):
		return 0
	case string(reconcile.Unobserved):
		return 1
	default:
		return 2
	}
}

// posture — 등급 표시와 **등급 코드**.
//
// 사람이 읽는 말은 화면이 고른다 — 여기서 문장을 만들면 그 말이 대조를 계산하는 자리에
// 눌러앉아, 다른 말로 연 화면에서도 그대로 뜬다.
func posture(p discoveryv1.QuantumPosture) (mark, grade string) {
	switch p {
	case discoveryv1.QuantumPosture_QUANTUM_POSTURE_PQC_HYBRID:
		return "🟢", GradePQC
	case discoveryv1.QuantumPosture_QUANTUM_POSTURE_CLASSICAL:
		return "🔴", GradeClassical
	default:
		return "⚪", GradeUnknown
	}
}

// 양자내성 등급 코드.
const (
	GradePQC       = "pqc"
	GradeClassical = "classical"
	GradeUnknown   = "unknown"
)

// gradeLabel — 등급을 그 말로.
func gradeLabel(l Lang, grade string) string {
	switch grade {
	case GradePQC:
		return "PQC"
	case GradeClassical:
		return tGradeClassical.In(l)
	case GradeUnknown:
		return tGradeUnknown.In(l)
	}
	return grade
}

// RenderSurvey — 대조 결과 화면을 쓴다.
func RenderSurvey(w io.Writer, v SurveyView) error {
	return surveyPage(v).Render(context.Background(), w)
}
