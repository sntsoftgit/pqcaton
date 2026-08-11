package reconcile

import (
	"fmt"
	"sort"
	"strings"

	discoveryv1 "github.com/pqcota/pqcota/gen/pqcota/discovery/v1"
	"github.com/pqcota/pqcota/pkg/kernel/posture"
)

// RenderTopologyDOT — 대조된 통신 엣지를 Graphviz DOT 토폴로지로 렌더한다(§12.3, IC-E2).
//
// 정직성 규정(§12.2)을 그래프 문법으로 강제한다:
//   - 색  = 양자내성 posture: 🟢 green(PQC) / 🔴 red(고전) / ⚪ gray(불명·미관측)
//   - 선형 = reconciliation 상태: 실선=CONFIRMED, 굵은선=UNDECLARED(shadow 경고), 점선=UNOBSERVED
//   - 미관측 엣지는 "연결 없음"이 아니라 점선으로 그린다(미관측≠부재).
//   - off-scope 상대(스코프 미등재)는 점선 박스 + "판정요청" 표기(§0.4).
//   - uncovered(collector 미설치) 노드는 회색 처리 — 그 노드의 엣지는 반쪽만 보임(§12.2).
func RenderTopologyDOT(edges []ReconciledEdge, uncovered map[string]bool) string {
	// 노드 수집: 등장한 모든 src/dst. off-scope dst는 별도 표기.
	offScope := map[string]bool{}
	nodeSet := map[string]bool{}
	for _, e := range edges {
		nodeSet[e.Key.Src] = true
		nodeSet[e.Key.Dst] = true
		if e.OffScopeDst {
			offScope[e.Key.Dst] = true
		}
	}
	nodes := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	var b strings.Builder
	b.WriteString("digraph crypto_topology {\n")
	// rankdir=TB(위→아래) 세로 배치 + 하단 캡션 범례(옆으로 안 퍼지게). 폭 축소.
	b.WriteString("  rankdir=TB;\n  ranksep=0.5;\n  nodesep=0.3;\n")
	b.WriteString(`  labelloc="b"; fontsize=11;` + "\n")
	b.WriteString(`  label="색=posture: 🟢 PQC · 🔴 고전 · ⚪ 불명\n선형: 실선 CONFIRMED · 굵은선 shadow · 점선 UNOBSERVED";` + "\n")
	b.WriteString(`  node [shape=box, style="rounded,filled", fillcolor="#eeeeff", fontname="sans"];` + "\n")
	b.WriteString(`  edge [fontname="sans", fontsize=10];` + "\n\n")

	for _, n := range nodes {
		attrs := ""
		switch {
		case offScope[n]:
			attrs = `, fillcolor="#eeeeee", style="rounded,filled,dashed"`
			b.WriteString(fmt.Sprintf("  %q [label=%q%s];\n", n, n+"\n(미등재→판정요청)", attrs))
		case uncovered[n]:
			attrs = `, fillcolor="#dddddd"` // collector 미설치 → 회색(반쪽만 관측)
			b.WriteString(fmt.Sprintf("  %q [label=%q%s];\n", n, n+"\n(collector 미설치)", attrs))
		default:
			b.WriteString(fmt.Sprintf("  %q [label=%q];\n", n, n))
		}
	}
	b.WriteString("\n")

	for _, e := range edges {
		b.WriteString("  " + dotEdge(e) + "\n")
	}

	b.WriteString("}\n")
	return b.String()
}

func dotEdge(e ReconciledEdge) string {
	color := postureColor(e.Posture)
	label := edgeLabel(e)

	var style string
	switch e.State {
	case Confirmed:
		style = fmt.Sprintf(`color=%q, penwidth=2`, color)
	case Undeclared:
		// shadow 통신 — 굵은 경고선. posture 색 유지하되 강조.
		style = fmt.Sprintf(`color=%q, penwidth=3`, color)
	case Unobserved:
		// 미관측 — 점선(≠부재). posture는 불명이므로 회색.
		style = `color="#999999", style=dashed`
	}
	return fmt.Sprintf("%q -> %q [label=%q, %s];", e.Key.Src, e.Key.Dst, label, style)
}

func edgeLabel(e ReconciledEdge) string {
	if e.State == Unobserved {
		return "미관측(UNOBSERVED)"
	}
	parts := []string{}
	if e.Key.Proto != "" {
		parts = append(parts, e.Key.Proto)
	}
	parts = append(parts, shortGroup(e.Group))
	s := posture.Symbol(e.Posture) + " " + strings.Join(parts, " ")
	if e.State == Undeclared {
		s += "  ⚠UNDECLARED"
	}
	return s
}

func postureColor(p discoveryv1.QuantumPosture) string {
	switch p {
	case discoveryv1.QuantumPosture_QUANTUM_POSTURE_PQC_HYBRID:
		return "#22aa22" // 🟢
	case discoveryv1.QuantumPosture_QUANTUM_POSTURE_CLASSICAL:
		return "#cc2222" // 🔴
	default:
		return "#999999" // ⚪
	}
}

// shortGroup — 라벨 폭을 줄이려고 협상 그룹의 긴 접미사(@openssh.com·-sha512)를 정리한다.
func shortGroup(g string) string {
	if g == "" {
		return "불명"
	}
	if i := strings.IndexByte(g, '@'); i > 0 {
		g = g[:i]
	}
	for _, suf := range []string{"-sha512", "-sha256", "-sha384"} {
		g = strings.TrimSuffix(g, suf)
	}
	return g
}
