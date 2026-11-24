// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"context"
	"io"
	"sort"

	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
)

// SurveyView — 대조 결과 화면이 보는 것.
//
// **관측을 먼저 보인다.** 대조 결과만 보면 UNOBSERVED가 「없다」인지 「원리상 못 봤다」인지
// 읽는 사람이 가를 수 없다(§2.6 갭 ≠ 부재) — 그 답이 관측 절에 있다.
type SurveyView struct {
	Page
	R *report.Result

	Confirmed, Undeclared, Unobserved int
	PQC, Classical, Unknown           int

	Assets []AssetRow
	Edges  []EdgeRow
	// Unconfirmed — 어느 쪽이 연결을 열었는지 확인하지 못한 기록. 대조 표에 넣지 않고 그 아래에 따로 보인다.
	Unconfirmed []reconcile.UnconfirmedEdge

	// DOT — 토폴로지 원문. `dot`이 없는 기계에서도 **무엇을 그리려 했는지는 보여 준다.**
	DOT string

	// Activation — 02에서 확정한 정책이 활성 정책과 달라 활성화할 수 있으면 그 두 지문(05의 활성화 상자와 같은 판정). 04는
	// 이 대조가 활성 정책으로 한 것이라고 알리고 05의 활성화 상자로 잇기만 한다. 활성화 양식은 05에만 둔다. 없으면 nil이다.
	Activation *ActivationOffer
}

// AssetRow — 자산 한 줄.
type AssetRow struct {
	Node, Runtime, Component, State string
	Conf                            float64
	Rescan                          bool
	// Excluded — 02 정책이 관리 대상에서 뺐다(EXCLUDED_BY_POLICY). 판정 큐에 오르지 않는다.
	Excluded bool
}

// EdgeRow — 엣지 한 줄.
type EdgeRow struct {
	Src, Dst, Proto, State string
	Port                   uint32
	// Mark — 등급 표시. 색이 아니라 기호로 둔다 — 흑백으로 인쇄해도 읽힌다.
	Mark, Grade string
	Group       string
	OffScope    bool
	// OffScopeSrc — 연결을 연 쪽이 등재되지 않았다. 받은 쪽 기록에서만 생긴다.
	OffScopeSrc bool
	// ServerOnly — 받은 노드의 기록으로만 본 엣지다. 연 쪽 노드를 관측하지 못했거나 그쪽 수집기가 이 연결을 놓쳤다.
	ServerOnly bool
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
			Excluded: rec.Managed == reconcile.ExcludedByPolicy,
		})
	}
	// **필수 리뷰가 위로.** UNDECLARED가 이 도구가 주는 첫 번째 쓸모라 맨 앞에 있어야 한다(§3.3②).
	sort.SliceStable(v.Assets, func(i, j int) bool {
		return statePriority(v.Assets[i].State) < statePriority(v.Assets[j].State)
	})

	for _, e := range r.Edges {
		mark, grade := posture(e.Posture)
		v.Edges = append(v.Edges, EdgeRow{
			Src: e.Key.Src, Dst: e.Key.Dst, Port: e.Key.Port, Proto: e.Key.Proto,
			State: string(e.State), Mark: mark, Grade: grade, Group: e.Group,
			OffScope: e.OffScopeDst, OffScopeSrc: e.OffScopeSrc, ServerOnly: e.SeenByServer && !e.SeenByClient,
			Uncovered: r.Uncovered[e.Key.Src],
		})
	}
	sort.SliceStable(v.Edges, func(i, j int) bool { return v.Edges[i].Src < v.Edges[j].Src })

	v.Unconfirmed = r.UnconfirmedEdges
	v.DOT = reconcile.RenderTopologyDOT(r.Edges, r.Uncovered, r.UnconfirmedEdges)
	return v
}

// statePriority — 리뷰가 필요한 것부터. 자동으로 넘어가는 CONFIRMED가 위에 오면 정작 볼
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

// SurveySummary — 대조 요약 화면(`/survey-next`)이 쓰는 숫자.
type SurveySummary struct {
	Confirmed, Undeclared, Unobserved int
	// ToJudge — 판정 큐에 필수로 오르는 항목(reconcile.BuildReviewQueue의 Mandatory와 같은 대상). 선언과 어긋난
	// 자산(UNDECLARED · UNOBSERVED) 가운데 정책이 뺀 것은 세지 않는다. 05의 「필수 판정」이 세는 것과 같은 항목이고,
	// 05는 그 가운데 결론이 아직 빈 것만 센다.
	ToJudge int
	// Excluded — 선언과 어긋났지만 정책이 관리 대상에서 빼 판정 큐에 오르지 않는 자산.
	Excluded int
	// Rescan — **다시 관측해 봐야 하는 자산.** 이 수가 0이 아니면 UNOBSERVED를 부재로
	// 읽으면 안 된다 — 못 본 것인지 없는 것인지 아직 갈리지 않았다.
	Rescan int
}

// Summary — 요약의 숫자.
func (v SurveyView) Summary() SurveySummary {
	s := SurveySummary{
		Confirmed: v.Confirmed, Undeclared: v.Undeclared, Unobserved: v.Unobserved,
	}
	for _, a := range v.Assets {
		if a.Rescan {
			s.Rescan++
		}
		if a.State == string(reconcile.Confirmed) {
			continue
		}
		if a.Excluded {
			s.Excluded++
		} else {
			s.ToJudge++
		}
	}
	return s
}

// surveyEditAnchor — 요약에서 원본 표로 보내는 자리.
const surveyEditAnchor = "survey-detail"

// RenderSurvey — 대조 화면(`/survey`).
// RenderSurveyPicture — 04의 그림 조각. 화면이 뜬 뒤에 따로 받아 끼운다. svg는 검사를 지난 것이고, 비었으면 state가 까닭이며
// 그때는 DOT 원문을 함께 보인다. unreadable이면 대조 결과를 읽지 못한 것이다.
func RenderSurveyPicture(w io.Writer, l Lang, svg, state, dot string, unreadable bool) error {
	return surveyPictureBody(l, svg, state, dot, unreadable).Render(context.Background(), w)
}

func RenderSurvey(w io.Writer, v SurveyView) error {
	return surveyNextPage(v).Render(context.Background(), w)
}
