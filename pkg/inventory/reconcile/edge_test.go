// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package reconcile

import (
	"errors"
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
)

func oe(src, dstNode, dstAddr string, port uint32, proto discoveryv1.NetworkProtocol, group string) *discoveryv1.ObservedEdge {
	return &discoveryv1.ObservedEdge{
		SrcNodeId: src, DstNodeId: dstNode, DstAddr: dstAddr, Port: port,
		Protocol: proto, NegotiatedGroup: group,
		DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION,
	}
}

func ek(src, dst string, port uint32, proto string) EdgeKey {
	return EdgeKey{Org: testOrg, Src: src, Dst: dst, Port: port, Proto: proto}
}

// recEdges — 엣지 대조 결과만 보는 케이스용. 조직 검사는 IC-O4가 따로 본다.
func recEdges(t *testing.T, declared []EdgeKey, observed []*discoveryv1.ObservedEdge,
	scope map[string]bool, gaps []string) []ReconciledEdge {
	t.Helper()
	out, err := eng(t).ReconcileEdges(declared, observed, scope, gaps)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// IC-E1: 관측 엣지 vs 선언 엣지 → 3-상태(CONFIRMED / UNDECLARED / UNOBSERVED).
func TestReconcileEdges_threeState(t *testing.T) {
	declared := []EdgeKey{
		ek("web-01", "app-01", 8443, "TLS"), // 관측될 것 → CONFIRMED
		ek("app-01", "db-01", 5432, "TLS"),  // 관측 안 됨 → UNOBSERVED
	}
	scope := map[string]bool{"web-01": true, "app-01": true, "db-01": true}
	observed := []*discoveryv1.ObservedEdge{
		oe("web-01", "app-01", "", 8443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "X25519MLKEM768"),
		oe("app-01", "app-99-undeclared", "", 22, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_SSH, "ecdh-sha2"), // 관측 only, 스코프 내 → UNDECLARED
	}

	got := map[EdgeKey]ReconciledEdge{}
	for _, r := range recEdges(t, declared, observed, scope, nil) {
		got[r.Key] = r
	}
	if s := got[ek("web-01", "app-01", 8443, "TLS")].State; s != Confirmed {
		t.Errorf("web→app = %s, want CONFIRMED", s)
	}
	if s := got[ek("app-01", "app-99-undeclared", 22, "SSH")].State; s != Undeclared {
		t.Errorf("UNDECLARED SSH = %s, want UNDECLARED", s)
	}
	if s := got[ek("app-01", "db-01", 5432, "TLS")].State; s != Unobserved {
		t.Errorf("db 선언-only = %s, want UNOBSERVED", s)
	}
}

// IC-E1 (posture): 관측 CONFIRMED 엣지에 양자내성 posture가 부착된다.
func TestReconcileEdges_posture(t *testing.T) {
	scope := map[string]bool{"a": true, "b": true}
	decl := []EdgeKey{ek("a", "b", 443, "TLS")}
	pqc := recEdges(t, decl, []*discoveryv1.ObservedEdge{
		oe("a", "b", "", 443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "X25519MLKEM768"),
	}, scope, nil)[0]
	if pqc.Posture != discoveryv1.QuantumPosture_QUANTUM_POSTURE_PQC_HYBRID {
		t.Errorf("MLKEM 협상 = %v, want PQC_HYBRID(🟢)", pqc.Posture)
	}
	classical := recEdges(t, decl, []*discoveryv1.ObservedEdge{
		oe("a", "b", "", 443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "ECDHE-RSA"),
	}, scope, nil)[0]
	if classical.Posture != discoveryv1.QuantumPosture_QUANTUM_POSTURE_CLASSICAL {
		t.Errorf("ECDHE 협상 = %v, want CLASSICAL(🔴)", classical.Posture)
	}
}

// IC-E3: 스코프 밖 관측 상대 → off-scope 표기(등재 판정 요청).
func TestReconcileEdges_offScope(t *testing.T) {
	scope := map[string]bool{"web-01": true}
	// dst_node_id를 잇지 못함(원시 주소만) → off-scope.
	unresolved := recEdges(t, nil, []*discoveryv1.ObservedEdge{
		oe("web-01", "", "203.0.113.5:443", 443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "X25519"),
	}, scope, nil)[0]
	if !unresolved.OffScopeDst {
		t.Error("잇지 못한 상대는 off-scope여야(등재 판정 요청)")
	}
	if !unresolved.NeedsReview {
		t.Error("off-scope 상대는 리뷰 필요")
	}
	// 이어졌지만 스코프 미등재 → 여전히 off-scope.
	notInScope := recEdges(t, nil, []*discoveryv1.ObservedEdge{
		oe("web-01", "ext-cdn", "", 443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "X25519"),
	}, scope, nil)[0]
	if !notInScope.OffScopeDst {
		t.Error("스코프 미등재 상대는 off-scope여야")
	}
}

// IC-R4(엣지판): UNOBSERVED 엣지 + 네트워크 계층 갭 → 재수집 후보.
func TestReconcileEdges_unobservedNetGap(t *testing.T) {
	decl := []EdgeKey{ek("a", "b", 443, "TLS")}
	withGap := recEdges(t, decl, nil, nil, []string{commonv1.CollectionLayer_COLLECTION_LAYER_NETWORK.String()})[0]
	if !withGap.RescanCandidate {
		t.Error("네트워크 갭 있는 UNOBSERVED 엣지는 재수집 후보여야")
	}
	noGap := recEdges(t, decl, nil, nil, nil)[0]
	if noGap.RescanCandidate {
		t.Error("갭 없으면 재수집 후보 아님")
	}
}

// IC-O4 — 엣지도 자산과 같은 규칙이다. 다른 조직의 선언 엣지가 섞이면 대조하지 않는다.
func TestReconcileEdgesRefusesAnotherOrg(t *testing.T) {
	남 := EdgeKey{Org: "beta", Src: "web-01", Dst: "app-01", Port: 8443, Proto: "TLS"}
	if _, err := eng(t).ReconcileEdges([]EdgeKey{남}, nil, nil, nil); !errors.Is(err, ErrOrgMismatch) {
		t.Fatalf("남의 조직 엣지를 그대로 대조했다: %v", err)
	}
}

// IC-O5 — 관측 엣지에는 조직이 없다. **엔진이 찍는다** — 찍지 않으면 선언과 영영 안 맞아
// 모든 관측 엣지가 UNDECLARED로 올라온다.
func TestObservedEdgeGetsOrg(t *testing.T) {
	got := recEdges(t, nil, []*discoveryv1.ObservedEdge{
		oe("web-01", "app-01", "", 8443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "X25519MLKEM768"),
	}, map[string]bool{"web-01": true, "app-01": true}, nil)
	if len(got) != 1 {
		t.Fatalf("엣지 %d개", len(got))
	}
	if got[0].Key.Org != testOrg {
		t.Errorf("관측 엣지의 조직이 %q다, want %q", got[0].Key.Org, testOrg)
	}
}

// IC-E4: 관측 엣지가 들어오는 순서가 달라도 결과의 순서는 같다. 출발지가 같은 엣지는 도착지 · 포트 ·
// 프로토콜 순으로 놓이고, 관측 엣지와 선언만 있는 엣지가 한 줄로 섞여 정렬된다.
func TestReconcileEdges_orderIgnoresInputOrder(t *testing.T) {
	tls, ssh := discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_SSH
	declared := []EdgeKey{ek("app", "db", 5432, "TLS"), ek("web", "aaa", 9, "TLS")}
	scope := map[string]bool{"web": true, "app": true, "db": true, "aaa": true}
	edges := []*discoveryv1.ObservedEdge{
		oe("web", "db", "", 5432, tls, "x25519"),
		oe("web", "db", "", 22, ssh, "curve25519-sha256"), // 출발지·도착지가 같고 포트와 프로토콜만 다르다
		oe("web", "db", "", 5432, ssh, "curve25519-sha256"),
		oe("app", "web", "", 443, tls, "x25519"),
	}
	want := []EdgeKey{
		ek("app", "db", 5432, "TLS"), // 선언만(UNOBSERVED)
		ek("app", "web", 443, "TLS"),
		ek("web", "aaa", 9, "TLS"), // 선언만
		ek("web", "db", 22, "SSH"),
		ek("web", "db", 5432, "SSH"),
		ek("web", "db", 5432, "TLS"),
	}
	perms := [][]int{{0, 1, 2, 3}, {3, 2, 1, 0}, {1, 3, 0, 2}, {2, 0, 3, 1}, {1, 0, 3, 2}}
	for _, p := range perms {
		in := make([]*discoveryv1.ObservedEdge, len(p))
		for i, k := range p {
			in[i] = edges[k]
		}
		got := recEdges(t, declared, in, scope, nil)
		if len(got) != len(want) {
			t.Fatalf("순서 %v: 엣지 %d개, want %d", p, len(got), len(want))
		}
		for i := range want {
			if got[i].Key != want[i] {
				t.Errorf("순서 %v: [%d] = %v, want %v", p, i, got[i].Key, want[i])
			}
		}
	}
}
