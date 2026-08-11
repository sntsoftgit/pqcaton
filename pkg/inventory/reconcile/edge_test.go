package reconcile

import (
	"testing"

	commonv1 "github.com/pqcota/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/pqcota/pqcota/gen/pqcota/discovery/v1"
)

func oe(src, dstNode, dstAddr string, port uint32, proto discoveryv1.NetworkProtocol, group string) *discoveryv1.ObservedEdge {
	return &discoveryv1.ObservedEdge{
		SrcNodeId: src, DstNodeId: dstNode, DstAddr: dstAddr, Port: port,
		Protocol: proto, NegotiatedGroup: group,
		DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION,
	}
}

func ek(src, dst string, port uint32, proto string) EdgeKey {
	return EdgeKey{Src: src, Dst: dst, Port: port, Proto: proto}
}

// IC-E1: 관측 엣지 vs 선언 엣지 → 3-상태(CONFIRMED / UNDECLARED shadow / UNOBSERVED).
func TestReconcileEdges_threeState(t *testing.T) {
	declared := []EdgeKey{
		ek("web-01", "app-01", 8443, "TLS"), // 관측될 것 → CONFIRMED
		ek("app-01", "db-01", 5432, "TLS"),  // 관측 안 됨 → UNOBSERVED
	}
	scope := map[string]bool{"web-01": true, "app-01": true, "db-01": true}
	observed := []*discoveryv1.ObservedEdge{
		oe("web-01", "app-01", "", 8443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "X25519MLKEM768"),
		oe("app-01", "app-99-shadow", "", 22, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_SSH, "ecdh-sha2"), // 관측 only, 스코프 내 → UNDECLARED
	}

	got := map[EdgeKey]ReconciledEdge{}
	for _, r := range ReconcileEdges(declared, observed, scope, nil) {
		got[r.Key] = r
	}
	if s := got[ek("web-01", "app-01", 8443, "TLS")].State; s != Confirmed {
		t.Errorf("web→app = %s, want CONFIRMED", s)
	}
	if s := got[ek("app-01", "app-99-shadow", 22, "SSH")].State; s != Undeclared {
		t.Errorf("shadow SSH = %s, want UNDECLARED", s)
	}
	if s := got[ek("app-01", "db-01", 5432, "TLS")].State; s != Unobserved {
		t.Errorf("db 선언-only = %s, want UNOBSERVED", s)
	}
}

// IC-E1 (posture): 관측 CONFIRMED 엣지에 양자내성 posture가 부착된다.
func TestReconcileEdges_posture(t *testing.T) {
	scope := map[string]bool{"a": true, "b": true}
	decl := []EdgeKey{ek("a", "b", 443, "TLS")}
	pqc := ReconcileEdges(decl, []*discoveryv1.ObservedEdge{
		oe("a", "b", "", 443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "X25519MLKEM768"),
	}, scope, nil)[0]
	if pqc.Posture != discoveryv1.QuantumPosture_QUANTUM_POSTURE_PQC_HYBRID {
		t.Errorf("MLKEM 협상 = %v, want PQC_HYBRID(🟢)", pqc.Posture)
	}
	classical := ReconcileEdges(decl, []*discoveryv1.ObservedEdge{
		oe("a", "b", "", 443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "ECDHE-RSA"),
	}, scope, nil)[0]
	if classical.Posture != discoveryv1.QuantumPosture_QUANTUM_POSTURE_CLASSICAL {
		t.Errorf("ECDHE 협상 = %v, want CLASSICAL(🔴)", classical.Posture)
	}
}

// IC-E3: 스코프 밖 관측 상대 → off-scope 표기(등재 판정 요청).
func TestReconcileEdges_offScope(t *testing.T) {
	scope := map[string]bool{"web-01": true}
	// dst_node_id 미해소(원시 주소만) → off-scope.
	unresolved := ReconcileEdges(nil, []*discoveryv1.ObservedEdge{
		oe("web-01", "", "203.0.113.5:443", 443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "X25519"),
	}, scope, nil)[0]
	if !unresolved.OffScopeDst {
		t.Error("미해소 상대는 off-scope여야(등재 판정 요청)")
	}
	if !unresolved.NeedsReview {
		t.Error("off-scope 상대는 리뷰 필요")
	}
	// 해소됐지만 스코프 미등재 → 여전히 off-scope.
	notInScope := ReconcileEdges(nil, []*discoveryv1.ObservedEdge{
		oe("web-01", "ext-cdn", "", 443, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, "X25519"),
	}, scope, nil)[0]
	if !notInScope.OffScopeDst {
		t.Error("스코프 미등재 상대는 off-scope여야")
	}
}

// IC-R4(엣지판): UNOBSERVED 엣지 + 네트워크 계층 갭 → 재수집 후보.
func TestReconcileEdges_unobservedNetGap(t *testing.T) {
	decl := []EdgeKey{ek("a", "b", 443, "TLS")}
	withGap := ReconcileEdges(decl, nil, nil, []string{commonv1.CollectionLayer_COLLECTION_LAYER_NETWORK.String()})[0]
	if !withGap.RescanCandidate {
		t.Error("네트워크 갭 있는 UNOBSERVED 엣지는 재수집 후보여야")
	}
	noGap := ReconcileEdges(decl, nil, nil, nil)[0]
	if noGap.RescanCandidate {
		t.Error("갭 없으면 재수집 후보 아님")
	}
}
