package main

import (
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
)

func edge(dstNode, dstAddr string) *discoveryv1.ObservedEdge {
	return &discoveryv1.ObservedEdge{SrcNodeId: "web-gw", DstNodeId: dstNode, DstAddr: dstAddr}
}

func completeness(covered, missing []commonv1.CollectionLayer) *discoveryv1.CollectionResult {
	return &discoveryv1.CollectionResult{
		Completeness: &commonv1.Completeness{LayersCovered: covered, LayersMissing: missing},
	}
}

// IC-P8 — **관측 IP를 스코프 노드로 해소한다**(§0.4).
//
// 해소되지 않으면 선언 엣지와 영영 맞지 않아 **CONFIRMED 여야 할 것이 shadow 로 올라온다** —
// 틀린 답이 아니라 그럴듯한 답이라 눈으로는 안 잡힌다. 포트가 붙은 주소와 다중 IP 노드가
// 그 자리다.
func TestResolveEdgeDsts(t *testing.T) {
	nodes := []declNode{
		{Name: "pay-db", IPs: []string{"172.19.0.2", "172.18.0.2"}}, // 망 둘에 걸친 노드
		{Name: "pay-app", IPs: []string{"172.19.0.4"}},
	}
	edges := []*discoveryv1.ObservedEdge{
		edge("", "172.19.0.4:8443"), // 포트가 붙어 있다
		edge("", "172.18.0.2"),      // 두 번째 IP 로도 해소돼야 한다
		edge("", "10.9.9.9:22"),     // 스코프 밖 — 그대로 둔다
		edge("이미-해소", "172.19.0.4"), // 이미 있는 것은 덮지 않는다
	}

	resolveEdgeDsts(edges, nodes)

	want := []string{"pay-app", "pay-db", "", "이미-해소"}
	for i, w := range want {
		if got := edges[i].GetDstNodeId(); got != w {
			t.Errorf("%d번 엣지 = %q, want %q", i, got, w)
		}
	}
}

// IC-P9 — **커버와 강등을 가른다.**
//
// `networkCovered` 는 실제로 캡처한 것만 참이어야 한다. 강등(layers_missing)까지 참으로
// 보면 **못 본 노드가 「봤다」로 세어져** 토폴로지에서 점선이 실선이 된다 — 미관측과 부재를
// 가르는 선이 거기서 무너진다.
func TestNetworkCoveredVsDegraded(t *testing.T) {
	net := commonv1.CollectionLayer_COLLECTION_LAYER_NETWORK
	art := commonv1.CollectionLayer_COLLECTION_LAYER_ARTIFACT

	covered := completeness([]commonv1.CollectionLayer{net}, nil)
	degraded := completeness([]commonv1.CollectionLayer{art}, []commonv1.CollectionLayer{net})
	unrelated := completeness([]commonv1.CollectionLayer{art}, nil)

	if !networkCovered(covered) {
		t.Error("실제로 캡처한 것을 커버로 세지 않았다")
	}
	if networkCovered(degraded) {
		t.Error("강등된 것을 커버로 셌다 — 못 본 노드가 「봤다」가 된다")
	}
	// hasNetworkLayer 는 레인 분류용이라 강등도 네트워크 레인으로 본다.
	if !hasNetworkLayer(degraded) {
		t.Error("강등된 네트워크 결과가 자산 레인으로 흘러간다")
	}
	if hasNetworkLayer(unrelated) {
		t.Error("네트워크와 무관한 결과를 네트워크 레인으로 봤다")
	}
}

// IC-P10 — 같은 것을 두 번 세지 않되 **처음 순서를 지킨다.**
//
// 노드 하나를 collector 둘이 보면 목록에 두 번 나온다 — 그대로 두면 「무엇이 이 노드를
// 봤나」가 부풀어 오른다. 정렬하지 않는 것은 부르는 쪽이 이미 정렬해 넘기기 때문이고,
// 여기서 다시 흔들면 리포트 줄 순서가 실행마다 달라진다.
func TestUniqKeepsFirstOrder(t *testing.T) {
	got := uniq([]string{"openssl-collector", "jvm-collector", "openssl-collector"})
	want := []string{"openssl-collector", "jvm-collector"}
	if len(got) != len(want) {
		t.Fatalf("%d개: %v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("순서가 달라졌다: %v, want %v", got, want)
		}
	}
	// 입력을 덮어쓰지 않는다 — 같은 슬라이스를 뒤에서 또 쓰면 조용히 짧아진다.
	in := []string{"a", "a", "b"}
	_ = uniq(in)
	if len(in) != 3 || in[1] != "a" {
		t.Errorf("입력이 덮였다: %v", in)
	}
}
