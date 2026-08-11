// Command pqcota-report — 컨트롤러에서 실행. 각 노드가 낸 CollectionResult JSON들을 모아
// 정규화·대조하고 인벤토리 3-상태 뷰 + 크립토 통신 토폴로지(DOT)를 만든다.
//
// usage: pqcota-report <results-dir> <declaration.json> [topology-out.dot]
//
//	results-dir: *.json (pqcota-nodescan / pqcota-netcap 산출) 모음
//	declaration.json: 고객 선언(assets·edges·scope) — 대조 기준
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	commonv1 "github.com/pqcota/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/pqcota/pqcota/gen/pqcota/discovery/v1"
	"github.com/pqcota/pqcota/pkg/discovery/history"
	"github.com/pqcota/pqcota/pkg/discovery/normalize"
	"github.com/pqcota/pqcota/pkg/kernel/posture"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"google.golang.org/protobuf/encoding/protojson"
)

type declaration struct {
	Scope  []string    `json:"scope"`
	Nodes  []declNode  `json:"nodes"` // 스코프 마스터: 노드↔IP (관측 IP→노드 해소, §0.4)
	Assets []declAsset `json:"assets"`
	Edges  []declEdge  `json:"edges"`
}
type declNode struct {
	Name string   `json:"name"`
	IPs  []string `json:"ips"`
}
type declAsset struct {
	Node, Runtime, Component string
}
type declEdge struct {
	Src, Dst, Proto string
	Port            uint32
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: pqcota-report <results-dir> <declaration.json> [topology-out.dot]")
		os.Exit(2)
	}
	dir, declPath := os.Args[1], os.Args[2]
	dotOut := "topology.dot"
	if len(os.Args) > 3 {
		dotOut = os.Args[3]
	}

	decl := loadDeclaration(declPath)
	results := loadResults(dir)

	// 관측 자산(openssl)과 관측 엣지(network)를 레인별로 분리.
	var observedAssets []reconcile.Observed
	var observedEdges []*discoveryv1.ObservedEdge
	var assetGaps []string
	coveredNodes := map[string]bool{} // netcap이 실제로 관측한 노드
	for _, res := range results {
		if len(res.GetObservedEdges()) > 0 || hasNetworkLayer(res) {
			// 네트워크 레인. NETWORK 계층을 실제 커버(캡처 성공)했으면 covered — 서버 전용 노드는
			// client 엣지가 0이어도 관측은 수행됐다(collector 미설치 아님, 강등만 미커버).
			observedEdges = append(observedEdges, res.GetObservedEdges()...)
			if networkCovered(res) {
				coveredNodes[res.GetEnvelope().GetTargetNodeId()] = true
			}
			continue
		}
		// OpenSSL 자산 레인 → 정규화 → 스냅샷 → 관측 자산.
		snap, err := normalize.Normalize([]*discoveryv1.CollectionResult{res}, "snap", res.GetEnvelope().GetTargetNodeId(), "ruleset-demo", history.NewMemStore(), nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, "normalize:", err)
			continue
		}
		observedAssets = append(observedAssets, reconcile.AssetsFromSnapshot(snap)...)
		assetGaps = append(assetGaps, reconcile.GapLayers(snap)...)
	}

	// ── 자산 3-상태 대조 ──
	declaredAssets := make([]reconcile.AssetKey, 0, len(decl.Assets))
	for _, a := range decl.Assets {
		declaredAssets = append(declaredAssets, reconcile.AssetKey{NodeID: a.Node, Runtime: a.Runtime, Component: a.Component})
	}
	assetRecs := reconcile.Reconcile(declaredAssets, observedAssets, assetGaps)

	// 관측 IP → 스코프 노드 해소(§0.4). 해소되면 CONFIRMED로 잡히고, 안 되면 off-scope 등재판정.
	resolveEdgeDsts(observedEdges, decl.Nodes)

	// ── 엣지 3-상태 대조 + posture ──
	declaredEdges := make([]reconcile.EdgeKey, 0, len(decl.Edges))
	for _, e := range decl.Edges {
		declaredEdges = append(declaredEdges, reconcile.EdgeKey{Src: e.Src, Dst: e.Dst, Port: e.Port, Proto: e.Proto})
	}
	scope := map[string]bool{}
	for _, n := range decl.Scope {
		scope[n] = true
	}
	edgeRecs := reconcile.ReconcileEdges(declaredEdges, observedEdges, scope, nil)

	// coverage: 스코프 노드 중 netcap 미관측 → 회색(반쪽만 보임).
	uncovered := map[string]bool{}
	for _, n := range decl.Scope {
		if !coveredNodes[n] {
			uncovered[n] = true
		}
	}

	// ── 출력 ──
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  pqcota 디스커버리 → 인벤토리 데모 리포트                  ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Printf("\n노드 %d · 관측 자산 %d · 관측 엣지 %d · 선언 자산 %d · 선언 엣지 %d\n\n",
		len(decl.Scope), len(observedAssets), len(observedEdges), len(declaredAssets), len(declaredEdges))

	fmt.Println("──────── ① 자산 인벤토리 (3-상태 대조) ────────")
	fmt.Print(reconcile.RenderView(assetRecs))

	fmt.Println("\n──────── ② 통신 엣지 + 양자내성 posture ────────")
	printEdges(edgeRecs, uncovered)

	dot := reconcile.RenderTopologyDOT(edgeRecs, uncovered)
	if err := os.WriteFile(dotOut, []byte(dot), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write dot:", err)
	} else {
		fmt.Printf("\n토폴로지 DOT 저장: %s  (렌더: dot -Tsvg %s -o topology.svg)\n", dotOut, dotOut)
	}
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
			grp = "(불명)"
		}
		flags := ""
		if e.OffScopeDst {
			flags += " [off-scope→등재판정]"
		}
		if e.RescanCandidate {
			flags += " [재수집후보]"
		}
		fmt.Printf("  %s %-10s → %-18s %-6s %-24s %s%s\n",
			sym, e.Key.Src, e.Key.Dst, e.Key.Proto, grp, e.State, flags)
	}
	fmt.Printf("\n  posture 합계: 🟢 PQC %d · 🔴 고전 %d · ⚪ 불명 %d\n", pqc, classical, unknown)
	if len(uncovered) > 0 {
		fmt.Printf("  collector 미설치(반쪽 관측): %v\n", keys(uncovered))
	}
}

// resolveEdgeDsts — 관측 엣지의 dst_addr(ip:port) IP를 스코프 노드명으로 해소한다(§0.4).
// 해소되면 dst_node_id를 채워 in-scope CONFIRMED 후보가 되고, 안 되면 off-scope(등재 판정)로 남는다.
func resolveEdgeDsts(edges []*discoveryv1.ObservedEdge, nodes []declNode) {
	ip2node := map[string]string{}
	for _, n := range nodes {
		for _, ip := range n.IPs {
			ip2node[ip] = n.Name
		}
	}
	for _, e := range edges {
		if e.GetDstNodeId() != "" {
			continue
		}
		host := e.GetDstAddr()
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.TrimSpace(host)
		if node, ok := ip2node[host]; ok {
			e.DstNodeId = node
		}
	}
}

// networkCovered — NETWORK 계층을 실제로 커버했는지(캡처 성공). 강등(layers_missing)은 false.
func networkCovered(res *discoveryv1.CollectionResult) bool {
	for _, l := range res.GetCompleteness().GetLayersCovered() {
		if l == commonv1.CollectionLayer_COLLECTION_LAYER_NETWORK {
			return true
		}
	}
	return false
}

func hasNetworkLayer(res *discoveryv1.CollectionResult) bool {
	for _, l := range res.GetCompleteness().GetLayersCovered() {
		if l == commonv1.CollectionLayer_COLLECTION_LAYER_NETWORK {
			return true
		}
	}
	for _, l := range res.GetCompleteness().GetLayersMissing() {
		if l == commonv1.CollectionLayer_COLLECTION_LAYER_NETWORK {
			return true
		}
	}
	return false
}

func loadDeclaration(path string) declaration {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read declaration:", err)
		os.Exit(1)
	}
	var d declaration
	if err := json.Unmarshal(b, &d); err != nil {
		fmt.Fprintln(os.Stderr, "parse declaration:", err)
		os.Exit(1)
	}
	return d
}

func loadResults(dir string) []*discoveryv1.CollectionResult {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var out []*discoveryv1.CollectionResult
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		res := &discoveryv1.CollectionResult{}
		if err := protojson.Unmarshal(b, res); err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", p, err)
			continue
		}
		out = append(out, res)
	}
	return out
}

func keys(m map[string]bool) []string {
	var s []string
	for k := range m {
		s = append(s, k)
	}
	sort.Strings(s)
	return s
}
