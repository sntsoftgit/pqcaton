// Command pqcaton-report — 컨트롤러에서 실행. 각 노드가 낸 CollectionResult JSON들을 모아
// 정규화·대조하고 인벤토리 3-상태 뷰 + 크립토 통신 토폴로지(DOT)를 만든다.
//
// usage: pqcaton-report <results-dir> <declaration.json> [topology-out.dot]
//
//	results-dir: *.json (pqcota-nodescan / pqcota-netcap 산출) 모음
//	declaration.json: 고객 선언(assets·edges·scope) — 대조 기준
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/kernel/posture"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: pqcaton-report <results-dir> <declaration.json> [topology-out.dot]")
		os.Exit(2)
	}
	dir, declPath := os.Args[1], os.Args[2]
	dotOut := "topology.dot"
	if len(os.Args) > 3 {
		dotOut = os.Args[3]
	}

	d := loadDeclaration(declPath)
	// **계산은 공용 패키지가 한다.** 화면(`pqcaton-ui`)이 같은 것을 그리므로, 계산이 두
	// 곳에 있으면 화면과 글이 다른 답을 내는 날이 온다.
	r, err := report.Build(dir, d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
	for _, sk := range r.Skipped {
		fmt.Fprintln(os.Stderr, "   skipped (unreadable):", sk)
	}

	// ── 출력 ──
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  pqcota discovery → inventory demo report                ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Printf("\nnodes %d · observed assets %d · observed edges %d · declared assets %d · declared edges %d\n\n",
		r.Nodes, r.ObservedAssets, r.ObservedEdges, r.DeclaredAssets, r.DeclaredEdges)

	// ① 관측 - **여기서 시작하는 사람이 있다.** pqcota 데모를 거치지 않고 이 리포트만 보는
	// 사람에게는 대조 앞에 무엇이 있었는지가 안 보인다. 재료는 이미 손에 있으니 보여 준다.
	fmt.Println("──────── ① Observation — what pqcota saw ────────")
	printObservation(r)

	fmt.Println("\n──────── ② Asset inventory (three-state reconciliation) ────────")
	fmt.Print(reconcile.RenderView(r.Assets))

	fmt.Println("\n──────── ③ Communication edges + quantum-resistance grade ────────")
	printEdges(r.Edges, r.Uncovered)

	dot := reconcile.RenderTopologyDOT(r.Edges, r.Uncovered)
	if err := os.WriteFile(dotOut, []byte(dot), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write dot:", err)
	} else {
		fmt.Printf("\ntopology DOT written: %s\n   to see it drawn, Graphviz (optional): dot -Tsvg %s -o topology.svg\n", dotOut, dotOut)
	}
}

// printObservation — 대조 앞에 무엇이 있었는지. **여기서 처음 보는 사람을 위한 절이다.**
//
// 특히 「못 본 계층」을 보인다 - 그것이 없으면 다음 절의 UNOBSERVED가 「없다」인지
// 「원리상 못 봤다」인지 읽는 사람이 가를 수 없다(§2.7 갭 != 부재).
func printObservation(r *report.Result) {
	seenBy, gaps, uncovered := r.SeenBy, r.GapLayers(), r.Uncovered
	byRuntime := r.ObservedByRuntime()
	nodes := r.SeenNodes()

	fmt.Println("  The collector was carried to each target node, run, and taken back. Nothing is left behind.")
	for _, n := range nodes {
		c := seenBy[n]
		sort.Strings(c)
		fmt.Printf("    %-12s %s\n", n, strings.Join(report.Uniq(c), ", "))
	}
	fmt.Printf("\n  assets observed in actual use: ")
	rts := make([]string, 0, len(byRuntime))
	for r := range byRuntime {
		rts = append(rts, r)
	}
	sort.Strings(rts)
	parts := make([]string, 0, len(rts))
	for _, r := range rts {
		parts = append(parts, fmt.Sprintf("%s %d", r, byRuntime[r]))
	}
	fmt.Println(strings.Join(parts, " · "))
	fmt.Printf("  connections negotiated in a handshake: %d (graded in the next section)\n", r.ObservedEdges)

	fmt.Print("\n  not seen: ")
	if len(gaps) == 0 && len(uncovered) == 0 {
		fmt.Println("nothing — observation is complete for this scope")
	} else {
		if len(gaps) > 0 {
			labels := make([]string, 0, len(gaps))
			for _, g := range gaps {
				labels = append(labels, report.LayerLabel(g))
			}
			fmt.Printf("layer %s", strings.Join(labels, ", "))
		}
		if len(uncovered) > 0 {
			if len(gaps) > 0 {
				fmt.Print(" ")
			}
			un := make([]string, 0, len(uncovered))
			for n := range uncovered {
				un = append(un, n)
			}
			sort.Strings(un)
			fmt.Printf("· nodes whose traffic was not observed: %s", strings.Join(un, ", "))
		}
		fmt.Println()
	}
	fmt.Println("  **Not seen is not the same as not there.** Which one an UNOBSERVED below means")
	fmt.Println("  is settled by the line above — if listed there, collect again first; otherwise a person judges.")
}

func printEdges(recs []reconcile.ReconciledEdge, uncovered map[string]bool) {
	sort.Slice(recs, func(i, j int) bool { return recs[i].Key.Src < recs[j].Key.Src })
	pqc, classical, unknown := 0, 0, 0
	for _, e := range recs {
		sym := posture.Symbol(e.Posture)
		switch e.Posture {
		case discoveryv1.QuantumPosture_QUANTUM_POSTURE_PQC_HYBRID:
			pqc++
		case discoveryv1.QuantumPosture_QUANTUM_POSTURE_CLASSICAL:
			classical++
		default:
			unknown++
		}
		grp := e.Group
		if grp == "" {
			grp = "(unknown)"
		}
		flags := ""
		if e.OffScopeDst {
			flags += " [off-scope → needs enrollment decision]"
		}
		if e.RescanCandidate {
			flags += " [rescan candidate]"
		}
		fmt.Printf("  %s %-10s → %-18s %-6s %-24s %s%s\n",
			sym, e.Key.Src, e.Key.Dst, e.Key.Proto, grp, e.State, flags)
	}
	fmt.Printf("\n  grade totals: 🟢 PQC %d · 🔴 classical %d · ⚪ unknown %d\n", pqc, classical, unknown)
	if len(uncovered) > 0 {
		fmt.Printf("  collectors not installed (partial observation): %v\n", keys(uncovered))
	}
}

// loadDeclaration - 선언을 읽는다. 형식은 `pkg/inventory/decl` 에 있다 - 화면이 같은
// 파일을 편집하므로 형식이 한 곳에 있어야 한다.
func loadDeclaration(path string) decl.Declaration {
	d, err := decl.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "declaration:", err)
		os.Exit(1)
	}
	// **앞뒤가 안 맞으면 말한다.** 막지는 않는다 - 그대로 두면 대조 결과가 오류 없이 틀린다.
	if p := decl.Check(d); len(p) > 0 {
		fmt.Fprintf(os.Stderr, "\u26a0 %d places where the declaration does not add up - open it with `pqcaton-ui -decl`\n", len(p))
		for _, x := range p {
			fmt.Fprintf(os.Stderr, "   %s - %s\n", x.Where, x.What())
		}
	}
	return d
}

func keys(m map[string]bool) []string {
	var s []string
	for k := range m {
		s = append(s, k)
	}
	sort.Strings(s)
	return s
}
