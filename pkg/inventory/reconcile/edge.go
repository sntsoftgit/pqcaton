// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package reconcile

import (
	"net"
	"sort"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/posture"
	"github.com/randyinthedev-hash/pqcota-common/pkg/org"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/normalize"
)

// EdgeKey — 통신 엣지 동일성(§2 CommunicationEdge.ID: src·dst·port·proto 정준).
type EdgeKey struct {
	// Org — 자산 식별자와 같은 이유로 동일성의 일부다([AssetKey]).
	Org   org.ID
	Src   string
	Dst   string // 스코프 노드 ID(이어짐) 또는 원시 주소(못 이음=off-scope)
	Port  uint32
	Proto string // TLS | SSH | QUIC | ""
}

// ReconciledEdge — 한 통신 엣지의 대조 결과(§3.3 엣지판 + 인벤토리 설계 §6 posture 오버레이).
type ReconciledEdge struct {
	Key             EdgeKey
	State           State                      // CONFIRMED | UNDECLARED | UNOBSERVED
	Confidence      float64                    // §3.5 (상태 + 관측 evidence)
	Posture         discoveryv1.QuantumPosture // 관측 엣지의 양자내성 posture(인벤토리 설계 §6.1). 미관측이면 UNSPECIFIED
	Group           string                     // 협상된 KEX 그룹(표시용)
	NeedsReview     bool                       // UNDECLARED·UNOBSERVED·off-scope는 사람 판정 필수
	RescanCandidate bool                       // UNOBSERVED인데 네트워크 계층 갭으로 설명됨(§2.6)
	OffScopeDst     bool                       // dst가 스코프 마스터 미등재 → "등재 판정 요청"(§1.4/§5, IC-E3)
	// OffScopeSrc — 연결을 연 쪽(src)이 스코프 마스터 미등재. 받은 쪽 기록에서만 생긴다(IC-E5).
	OffScopeSrc bool
	// SeenByClient · SeenByServer — 이 엣지를 관측한 쪽. 연결을 연 노드의 기록과 받은 노드의 기록은 한 엣지로 합친다.
	SeenByClient, SeenByServer bool
}

// UnconfirmedEdge — 수집기가 어느 쪽이 연결을 열었는지 확인하지 못한 기록(상류 role = UNSPECIFIED, 포트 0, 협상 그룹 없음).
// 방향과 포트를 모르므로 선언과 대조하지 않고 따로 보인다(IC-E6).
type UnconfirmedEdge struct {
	Node  string // 관측한 노드
	Peer  string // 상대: 이어진 노드 ID 또는 원시 주소
	Proto string
	// OffScopePeer — 상대를 잇지 못했거나 스코프 마스터에 없다.
	OffScopePeer bool
}

// edgeSide — 관측 기록이 연결의 어느 쪽에서 나왔는가.
type edgeSide int

const (
	sideClient      edgeSide = iota // 관측 노드가 연결을 열었다: src = 관측 노드, dst = 상대
	sideServer                      // 관측 노드가 연결을 받았다(상류 v0.18.0부터): src = 상대, dst = 관측 노드
	sideUnconfirmed                 // 역할을 확인하지 못했다
)

// sideOf — 기록의 역할을 읽는다.
//   - SERVER인데 dst_addr에 포트가 있으면 연 쪽 기록으로 본다. 상류 v0.18.0 전의 netcap은 받은 연결을 기록하지 않았고,
//     자기 ClientHello를 놓친 클라이언트 기록에 SERVER를 적는 일이 있었다. 받은 쪽 기록의 dst_addr는 포트 없는 상대 IP다.
//   - 역할이 없고 포트가 있으면 연 쪽 기록이다(역할을 적지 않던 기록). 포트가 0이면 확인하지 못한 기록이다.
func sideOf(oe *discoveryv1.ObservedEdge) edgeSide {
	switch oe.GetRole() {
	case discoveryv1.EdgeRole_EDGE_ROLE_SERVER:
		if _, _, err := net.SplitHostPort(oe.GetDstAddr()); err == nil {
			return sideClient
		}
		return sideServer
	case discoveryv1.EdgeRole_EDGE_ROLE_UNSPECIFIED:
		if oe.GetPort() == 0 {
			return sideUnconfirmed
		}
	}
	return sideClient
}

// reconcileEdges — 관측 엣지(network-collector) vs 선언 엣지를 3-상태로 분류한다(IC-E1).
// 받은 쪽 기록은 뒤집어 연 쪽 기록과 한 엣지로 합치고(IC-E5), 역할을 확인하지 못한 기록은 넣지 않는다(IC-E6, [UnconfirmedEdges]).
//
// **조직 검사를 지난 뒤에만 불린다**([Engine.ReconcileEdges]). 관측 엣지에는 조직이 없어
// 여기서 찍는다 — 관측은 늘 그 엔진의 조직에서 온 것이다.
//   - 관측 ∩ 선언  → CONFIRMED
//   - 관측 only    → UNDECLARED(UNDECLARED 통신 — 보안 최우선)
//   - 선언 only    → UNOBSERVED (네트워크 갭이면 재수집 후보; 인벤토리 설계 §6.2 미관측≠부재)
//
// scope: 스코프 마스터 등재 노드 집합. 관측 상대(dst)가 여기 없으면 off-scope로 표기한다(IC-E3).
// gapLayers: 관측 완전성 맵의 미커버 계층 — NETWORK 갭이면 UNOBSERVED를 재수집 후보로 본다.
func reconcileEdges(o org.ID, declared []EdgeKey, observed []*discoveryv1.ObservedEdge, scope map[string]bool, gapLayers []string) []ReconciledEdge {
	dset := make(map[EdgeKey]bool, len(declared))
	for _, k := range declared {
		dset[k] = true
	}
	hasNetGap := containsNetworkGap(gapLayers)
	seen := map[EdgeKey]bool{}
	var out []ReconciledEdge

	// 관측 기준: 선언에도 있으면 CONFIRMED, 없으면 UNDECLARED. 연 쪽 기록을 먼저 놓고 받은 쪽 기록을 거기에 합친다.
	// 순서를 정해 두어야 두 기록의 협상 그룹이 다를 때 결과가 입력 순서에 좌우되지 않는다.
	at := map[EdgeKey]int{}
	for _, side := range []edgeSide{sideClient, sideServer} {
		for _, oe := range observed {
			if sideOf(oe) != side {
				continue
			}
			key, offSrc, offDst := observedEdgeKey(o, oe, side, scope)
			if i, ok := at[key]; ok {
				re := &out[i]
				re.SeenByClient = re.SeenByClient || side == sideClient
				re.SeenByServer = re.SeenByServer || side == sideServer
				if re.Group == "" && oe.GetNegotiatedGroup() != "" {
					re.Group = oe.GetNegotiatedGroup()
					re.Posture = posture.Classify(oe.GetNegotiatedGroup(), oe.GetCipher())
				}
				continue
			}
			seen[key] = true
			at[key] = len(out)

			ev := evidenceStr(normalize.EvidenceStrength(oe.GetDetectionMethod()))
			post := posture.Classify(oe.GetNegotiatedGroup(), oe.GetCipher())
			offScope := offSrc || offDst
			re := ReconciledEdge{
				Key:          key,
				Posture:      post,
				Group:        oe.GetNegotiatedGroup(),
				OffScopeDst:  offDst,
				OffScopeSrc:  offSrc,
				SeenByClient: side == sideClient,
				SeenByServer: side == sideServer,
			}
			if dset[key] {
				re.State = Confirmed
				re.Confidence = confidence(Confirmed, ev)
				re.NeedsReview = offScope // 정상 CONFIRMED는 자동통과 후보, off-scope면 리뷰
			} else {
				re.State = Undeclared // 관측 only = UNDECLARED 통신
				re.Confidence = confidence(Undeclared, ev)
				re.NeedsReview = true
			}
			out = append(out, re)
		}
	}

	// 선언만 있고 관측 안 됨 → UNOBSERVED. 네트워크 갭이면 재수집 후보.
	for _, k := range declared {
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, ReconciledEdge{
			Key:             k,
			State:           Unobserved,
			Confidence:      ConfidenceFor(Unobserved),
			NeedsReview:     true,
			RescanCandidate: hasNetGap,
		})
	}
	sortEdges(out)
	return out
}

// sortEdges — 엣지를 키 전체(출발지 · 도착지 · 포트 · 프로토콜)로 정렬한다. 관측 엣지는 수집기가 낸 순서로
// 들어오는데 그 순서는 실행마다 다를 수 있다. 출발지 하나로만 정렬하면 출발지가 같은 엣지끼리 순서가
// 정해지지 않아, 보고서 줄과 토폴로지 SVG의 엣지 번호가 같은 입력에서도 달라진다.
func sortEdges(es []ReconciledEdge) {
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i].Key, es[j].Key
		switch {
		case a.Src != b.Src:
			return a.Src < b.Src
		case a.Dst != b.Dst:
			return a.Dst < b.Dst
		case a.Port != b.Port:
			return a.Port < b.Port
		default:
			return a.Proto < b.Proto
		}
	})
}

// observedEdgeKey — ObservedEdge에서 EdgeKey를 만들고 off-scope 여부를 판정한다.
// dst_node_id가 노드로 이어지고 스코프에 등재돼 있으면 in-scope, 아니면(못 이었거나 미등재) off-scope.
// observedEdgeKey — 관측 기록을 「연 쪽 → 받은 쪽」 방향의 엣지 키로 옮긴다. 기록은 늘 관측한 노드 아래에 적히므로
// (src_node_id = 관측 노드), 받은 쪽 기록은 뒤집어야 연 쪽 기록 · 선언과 같은 키가 된다.
func observedEdgeKey(o org.ID, oe *discoveryv1.ObservedEdge, side edgeSide, scope map[string]bool) (key EdgeKey, offSrc, offDst bool) {
	peer, offPeer := edgePeer(oe, scope)
	key = EdgeKey{Org: o, Src: oe.GetSrcNodeId(), Dst: peer, Port: oe.GetPort(), Proto: protoName(oe.GetProtocol())}
	if side == sideServer {
		key.Src, key.Dst = peer, oe.GetSrcNodeId()
		return key, offPeer, false
	}
	return key, false, offPeer
}

// edgePeer — 기록의 상대. 노드로 이어졌으면 그 ID, 아니면 원시 주소다. 잇지 못했거나 스코프 마스터에 없으면 off-scope다.
func edgePeer(oe *discoveryv1.ObservedEdge, scope map[string]bool) (peer string, offScope bool) {
	peer = oe.GetDstNodeId()
	if peer == "" {
		return oe.GetDstAddr(), true // 잇지 못한 상대 → 원시 주소로 식별
	}
	return peer, scope != nil && !scope[peer] // 이어졌지만 스코프 마스터 미등재
}

// UnconfirmedEdges — 역할을 확인하지 못한 기록을 (관측 노드 · 상대 · 프로토콜)마다 한 줄로 모은다. 순서는 정해져 있다.
func UnconfirmedEdges(observed []*discoveryv1.ObservedEdge, scope map[string]bool) []UnconfirmedEdge {
	seen := map[UnconfirmedEdge]bool{}
	var out []UnconfirmedEdge
	for _, oe := range observed {
		if sideOf(oe) != sideUnconfirmed {
			continue
		}
		peer, off := edgePeer(oe, scope)
		u := UnconfirmedEdge{Node: oe.GetSrcNodeId(), Peer: peer, Proto: protoName(oe.GetProtocol()), OffScopePeer: off}
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.Node != b.Node:
			return a.Node < b.Node
		case a.Peer != b.Peer:
			return a.Peer < b.Peer
		default:
			return a.Proto < b.Proto
		}
	})
	return out
}

func protoName(p discoveryv1.NetworkProtocol) string {
	switch p {
	case discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS:
		return "TLS"
	case discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_SSH:
		return "SSH"
	case discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_QUIC:
		return "QUIC"
	default:
		return ""
	}
}

func containsNetworkGap(layers []string) bool {
	for _, l := range layers {
		if l == commonv1.CollectionLayer_COLLECTION_LAYER_NETWORK.String() {
			return true
		}
	}
	return false
}
