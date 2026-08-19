package report_test

import (
	"os"
	"path/filepath"
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
)

func edge(dstNode, dstAddr string) *discoveryv1.ObservedEdge {
	return &discoveryv1.ObservedEdge{SrcNodeId: "web-gw", DstNodeId: dstNode, DstAddr: dstAddr}
}

func completeness(covered, missing []commonv1.CollectionLayer) *discoveryv1.CollectionResult {
	return &discoveryv1.CollectionResult{
		Completeness: &commonv1.Completeness{LayersCovered: covered, LayersMissing: missing},
	}
}

// IC-R8 — **관측 IP를 스코프 노드로 해소한다**(§0.4).
//
// 해소되지 않으면 선언 엣지와 영영 맞지 않아 **CONFIRMED 여야 할 것이 shadow 로 올라온다** —
// 틀린 답이 아니라 그럴듯한 답이라 눈으로는 안 잡힌다. 포트가 붙은 주소와 망 둘에 걸친
// 노드가 그 자리다.
func TestResolveEdgeDsts(t *testing.T) {
	nodes := []decl.Node{
		{Name: "pay-db", IPs: []string{"172.19.0.2", "172.18.0.2"}},
		{Name: "pay-app", IPs: []string{"172.19.0.4"}},
	}
	edges := []*discoveryv1.ObservedEdge{
		edge("", "172.19.0.4:8443"),
		edge("", "172.18.0.2"),
		edge("", "10.9.9.9:22"),
		edge("이미-해소", "172.19.0.4"),
	}
	report.ResolveEdgeDsts(edges, nodes)
	want := []string{"pay-app", "pay-db", "", "이미-해소"}
	for i, w := range want {
		if got := edges[i].GetDstNodeId(); got != w {
			t.Errorf("%d번 엣지 = %q, want %q", i, got, w)
		}
	}
}

// IC-R9 — **커버와 강등을 가른다.** 강등을 커버로 세면 못 본 노드가 「봤다」가 되어
// 토폴로지의 점선이 실선이 된다.
func TestNetworkCoveredVsDegraded(t *testing.T) {
	nw := commonv1.CollectionLayer_COLLECTION_LAYER_NETWORK
	art := commonv1.CollectionLayer_COLLECTION_LAYER_ARTIFACT

	if !report.NetworkCovered(completeness([]commonv1.CollectionLayer{nw}, nil)) {
		t.Error("실제로 캡처한 것을 커버로 세지 않았다")
	}
	degraded := completeness([]commonv1.CollectionLayer{art}, []commonv1.CollectionLayer{nw})
	if report.NetworkCovered(degraded) {
		t.Error("강등된 것을 커버로 셌다")
	}
	if !report.HasNetworkLayer(degraded) {
		t.Error("강등된 네트워크 결과가 자산 레인으로 흘러간다")
	}
	if report.HasNetworkLayer(completeness([]commonv1.CollectionLayer{art}, nil)) {
		t.Error("네트워크와 무관한 결과를 네트워크 레인으로 봤다")
	}
}

// IC-R10 — 중복은 지우되 **처음 순서를 지키고 입력을 덮지 않는다.**
func TestUniqKeepsFirstOrder(t *testing.T) {
	in := []string{"openssl-collector", "jvm-collector", "openssl-collector"}
	got := report.Uniq(in)
	if len(got) != 2 || got[0] != "openssl-collector" || got[1] != "jvm-collector" {
		t.Fatalf("%v", got)
	}
	if len(in) != 3 {
		t.Errorf("입력이 덮였다: %v", in)
	}
}

// IC-R11 — **못 읽은 파일을 조용히 넘기지 않는다.** 빠진 노드를 모르면 「관측 안 됨」과
// 「못 읽음」이 뒤섞인다.
func TestLoadResultsReportsSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, skipped, err := report.LoadResults(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 || len(skipped) != 1 {
		t.Fatalf("결과 %d · 건너뜀 %d", len(got), len(skipped))
	}
}

// IC-R12 — 결과가 하나도 없어도 **선언만으로 대조가 돈다.** 전부 미관측으로 나오는 것이
// 맞는 답이고, 그것이 「없다」가 아니라 「아직 못 봤다」다.
func TestBuildWithNoResults(t *testing.T) {
	d := decl.Declaration{
		Org: "acme", Scope: []string{"web"},
		Nodes:  []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}}},
		Assets: []decl.Asset{{Node: "web", Runtime: "openssl", Component: "libssl"}},
		Edges:  []decl.Edge{{Src: "web", Dst: "web", Port: 443, Proto: "TLS"}},
	}
	r, err := report.Build(t.TempDir(), d)
	if err != nil {
		t.Fatal(err)
	}
	c, u, un := r.Counts()
	if c != 0 || u != 0 || un != 1 {
		t.Errorf("CONFIRMED %d · UNDECLARED %d · UNOBSERVED %d — 관측이 없으면 전부 미관측이어야", c, u, un)
	}
	if !r.Uncovered["web"] {
		t.Error("통신을 관측하지 못한 노드로 세지 않았다")
	}
	if r.Org != "acme" {
		t.Errorf("조직 = %q", r.Org)
	}
}
