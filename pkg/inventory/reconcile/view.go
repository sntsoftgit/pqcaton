package reconcile

import (
	"fmt"
	"strings"
)

// RenderView — 리컨실리에이션 뷰(§3.7): 상태 요약 + 자동통과 후보 수 + 필수 리뷰 큐(우선순위 순).
// 판단은 하지 않는다 — 판정 대상을 구조화해 사람에게 넘긴다(§3.1).
func RenderView(recs []Reconciled) string {
	autopass, review := BuildReviewQueue(recs)
	counts := map[State]int{}
	for _, r := range recs {
		counts[r.State]++
	}

	var b strings.Builder
	fmt.Fprintf(&b, "대조: CONFIRMED %d · UNDECLARED(shadow) %d · UNOBSERVED %d\n",
		counts[Confirmed], counts[Undeclared], counts[Unobserved])
	fmt.Fprintf(&b, "자동통과 후보(일괄 승인 제안): %d\n\n", len(autopass))
	if len(review) == 0 {
		fmt.Fprintf(&b, "필수 리뷰 없음.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "필수 리뷰 큐 (우선순위 순):\n")
	fmt.Fprintf(&b, "%-3s %-12s %-6s %-8s %s\n", "#", "state", "conf", "runtime", "component")
	for i, it := range review {
		r := it.Rec
		fmt.Fprintf(&b, "%-3d %-12s %-6.2f %-8s %s\n",
			i+1, r.State, r.Confidence, r.Key.Runtime, r.Key.Component)
	}
	return b.String()
}
