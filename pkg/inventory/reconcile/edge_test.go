// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package reconcile

import (
	"errors"
	"strings"
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

// withRole — 역할을 적은 관측 기록. 받은 쪽 기록(상류 v0.18.0부터)은 dst_addr가 포트 없는 상대 IP이고 port가 받은 포트다.
func withRole(e *discoveryv1.ObservedEdge, role discoveryv1.EdgeRole) *discoveryv1.ObservedEdge {
	e.Role = role
	return e
}

// IC-E5: **받은 쪽 기록은 뒤집어 연 쪽 기록과 한 엣지로 합친다.**
//
// 기록은 늘 관측한 노드 아래에 적힌다. 받은 쪽 기록을 연 쪽 기록처럼 읽으면 `app → web`이라는 거꾸로 된 미선언 엣지가 생기고,
// 선언한 `web → app`은 양쪽에서 보고도 한 번만 대조된다.
func TestReconcileEdges_serverSideRecord(t *testing.T) {
	tls := discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS
	client, server := discoveryv1.EdgeRole_EDGE_ROLE_CLIENT, discoveryv1.EdgeRole_EDGE_ROLE_SERVER
	scope := map[string]bool{"web": true, "app": true, "db": true}
	declared := []EdgeKey{ek("web", "app", 8443, "TLS"), ek("app", "db", 5432, "TLS")}
	observed := []*discoveryv1.ObservedEdge{
		// 받은 쪽 기록이 먼저 와도 결과는 같다. 그룹이 빈 연 쪽 기록은 받은 쪽 기록의 그룹으로 채운다.
		withRole(oe("app", "web", "10.0.0.1", 8443, tls, "X25519MLKEM768"), server),
		withRole(oe("web", "app", "10.0.0.2:8443", 8443, tls, ""), client),
		// 연 쪽(app)은 이 연결을 놓쳤고 받은 쪽(db)만 보았다: 선언과 맞는 엣지다.
		withRole(oe("db", "app", "10.0.0.2", 5432, tls, "X25519"), server),
		// 등재되지 않은 상대에서 받은 연결.
		withRole(oe("db", "", "203.0.113.9", 5432, tls, "X25519"), server),
		// 상류 v0.18.0 전의 기록: 연 쪽 기록인데 SERVER가 적혔다. dst_addr에 포트가 있으면 뒤집지 않는다.
		withRole(oe("web", "db", "10.0.0.3:22", 22, discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_SSH, "curve25519-sha256"), server),
	}
	got := map[EdgeKey]ReconciledEdge{}
	all := recEdges(t, declared, observed, scope, nil)
	for _, r := range all {
		got[r.Key] = r
	}
	if len(all) != 4 {
		t.Fatalf("%d edges, want 4: %+v", len(all), all)
	}
	both := got[ek("web", "app", 8443, "TLS")]
	if both.State != Confirmed || !both.SeenByClient || !both.SeenByServer || both.Group != "X25519MLKEM768" ||
		both.Posture != discoveryv1.QuantumPosture_QUANTUM_POSTURE_PQC_HYBRID {
		t.Errorf("the edge seen on both nodes is %+v, want one CONFIRMED edge with the negotiated group of the record that has it", both)
	}
	if _, reversed := got[ek("app", "web", 8443, "TLS")]; reversed {
		t.Error("the accepting node's record became a reversed edge app → web")
	}
	only := got[ek("app", "db", 5432, "TLS")]
	if only.State != Confirmed || only.SeenByClient || !only.SeenByServer || only.OffScopeSrc || only.OffScopeDst {
		t.Errorf("the edge seen on the accepting node only is %+v, want CONFIRMED and seen by the server side only", only)
	}
	ext := got[ek("203.0.113.9", "db", 5432, "TLS")]
	if ext.State != Undeclared || !ext.OffScopeSrc || ext.OffScopeDst || !ext.NeedsReview {
		t.Errorf("a connection accepted from an unlisted peer is %+v, want UNDECLARED with an off-scope source", ext)
	}
	old := got[ek("web", "db", 22, "SSH")]
	if old.State != Undeclared || !old.SeenByClient || old.SeenByServer {
		t.Errorf("a pre-v0.18.0 client record that carries the server role is %+v, want it read as web → db", old)
	}
}

// IC-E6: **역할을 확인하지 못한 기록은 대조하지 않고 따로 모은다.**
//
// 방향과 포트를 모르는 기록을 3-상태 표에 넣으면 포트 0의 미선언 엣지가 되고, 재관측으로 풀릴 것을 사람에게 판정하라고 올리게 된다.
func TestReconcileEdges_unconfirmedRole(t *testing.T) {
	ssh := discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_SSH
	scope := map[string]bool{"web": true, "db": true}
	observed := []*discoveryv1.ObservedEdge{
		oe("web", "db", "10.0.0.3", 0, ssh, ""),      // 역할 없음 · 포트 0
		oe("web", "db", "10.0.0.3", 0, ssh, ""),      // 같은 기록이 또 왔다
		oe("db", "", "198.51.100.7", 0, ssh, ""),     // 잇지 못한 상대
		oe("web", "db", "10.0.0.3:22", 22, ssh, "x"), // 역할 없음 · 포트 있음: 역할을 적지 않던 연 쪽 기록
	}
	edges := recEdges(t, nil, observed, scope, nil)
	if len(edges) != 1 || edges[0].Key != ek("web", "db", 22, "SSH") {
		t.Errorf("reconciled edges %+v, want only web → db:22", edges)
	}
	got := UnconfirmedEdges(observed, scope)
	want := []UnconfirmedEdge{
		{Node: "db", Peer: "198.51.100.7", Proto: "SSH", OffScopePeer: true},
		{Node: "web", Peer: "db", Proto: "SSH"},
	}
	if len(got) != len(want) {
		t.Fatalf("unconfirmed %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unconfirmed[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	dot := RenderTopologyDOT(edges, nil, got)
	for _, sub := range []string{`"web" -> "db" [label="SSH role not confirmed", color="#999999", style=dotted, dir=none];`, "role not confirmed (not reconciled)", "needs decision"} {
		if !strings.Contains(dot, sub) {
			t.Errorf("the topology lacks %q:\n%s", sub, dot)
		}
	}
	if strings.Contains(RenderTopologyDOT(edges, nil, nil), "role not confirmed") {
		t.Error("the legend mentions unconfirmed roles when there are none")
	}
}
