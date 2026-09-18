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
	excluded := 0
	for _, r := range recs {
		counts[r.State]++
		if r.Managed == ExcludedByPolicy {
			excluded++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "reconciled: CONFIRMED %d · UNDECLARED %d · UNOBSERVED %d\n",
		counts[Confirmed], counts[Undeclared], counts[Unobserved])
	// 관리 축은 대조 축과 따로 센다. 제외된 자산은 위 수에 **들어 있다**(보았으므로) - 여기서
	// 「그중 몇은 관리하지 않는다」를 말하지 않으면 읽는 사람은 전부 관리 대상으로 읽는다.
	if excluded > 0 {
		fmt.Fprintf(&b, "excluded by the asset-scope policy: %d (observed, not managed - not in any queue below)\n", excluded)
	}
	fmt.Fprintf(&b, "auto-pass candidates (proposed for batch approval): %d\n", len(autopass))
	renderPolicyConflicts(&b, recs)
	b.WriteString("\n")
	if len(review) == 0 {
		fmt.Fprintf(&b, "no mandatory reviews.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "mandatory review queue (by priority):\n")
	fmt.Fprintf(&b, "%-3s %-12s %-6s %-8s %s\n", "#", "state", "conf", "runtime", "component")
	for i, it := range review {
		r := it.Rec
		fmt.Fprintf(&b, "%-3d %-12s %-6s %-8s %s%s\n",
			i+1, r.State, confText(r), r.Key.Runtime, r.Key.Component, mixedNote(r))
	}
	return b.String()
}

// confText — 신뢰도를 글자로. **미평가는 숫자로 보이지 않는다** - 0.30은 「재 봤더니 낮다」로
// 읽히는데, 재지 않은 값이다.
func confText(r Reconciled) string {
	if !r.ConfidenceEvaluated {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", r.Confidence)
}

// mixedNote — 혼합 근거의 고지. 같은 자산의 관측 일부가 정책에 걸렸다는 것은 정책이 의도한
// 만큼만 걸리지 않았다는 신호일 수 있다(공유 .so를 여러 앱이 쓰는 경우).
func mixedNote(r Reconciled) string {
	if r.Managed == Managed && len(r.ExcludedSources) > 0 {
		return fmt.Sprintf("  (managed by %d source(s), %d more excluded by the policy)", len(r.Sources), len(r.ExcludedSources))
	}
	return ""
}

// renderPolicyConflicts — 선언은 관리 대상으로 적었는데 정책이 뺀 자산(CONFIRMED + EXCLUDED_BY_POLICY).
//
// **둘이 어긋난 것이고, 어느 쪽을 고칠지는 기계가 정하지 않는다.** 값으로 알린다: 자산 식별자 ·
// 원천 노드 · 앱 식별자 전부 · 그리고 같은 공용 정책 코드가 이 finding을 제외했다는 사실.
// 어느 규칙인지는 말하지 않는다 - 상류 Managed는 bool만 돌려준다. 앱 식별자를 적으면 사람이
// 정책 파일에서 그 식별자에 걸리는 줄을 찾을 수 있다. 식별자를 **전부** 적는 것은 공유 .so 하나를
// 여러 앱이 로드하면 그 목록 전부가 함께 빠진 것이기 때문이다 - 하나만 적으면 「이 앱만
// 빠졌다」로 읽는다.
func renderPolicyConflicts(b *strings.Builder, recs []Reconciled) {
	var n int
	for _, r := range recs {
		if r.State != Confirmed || r.Managed != ExcludedByPolicy {
			continue
		}
		if n == 0 {
			fmt.Fprintf(b, "\n⚠ declared as managed, but the asset-scope policy excludes it - the two disagree:\n")
		}
		n++
		for _, x := range r.ExcludedSources {
			fmt.Fprintf(b, "  • %s/%s/%s  seen on %s via %s  (excluded by the same policy code the ingest uses)\n",
				r.Key.NodeID, r.Key.Runtime, r.Key.Component, x.SourceNodeID, strings.Join(x.AppKeys, ", "))
		}
	}
	if n > 0 {
		fmt.Fprintf(b, "  fix the declaration or the policy - this tool does not choose which.\n")
	}
}
