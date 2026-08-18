package reconcile

import (
	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	"github.com/randyinthedev-hash/pqcota/pkg/kernel/posture"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
)

// EdgeKey — 통신 엣지 동일성(§2 CommunicationEdge.ID: src·dst·port·proto 정준).
type EdgeKey struct {
	// Org — 자산 열쇠와 같은 이유로 동일성의 일부다([AssetKey]).
	Org   org.ID
	Src   string
	Dst   string // 스코프 노드 ID(해소됨) 또는 원시 주소(미해소=off-scope)
	Port  uint32
	Proto string // TLS | SSH | QUIC | ""
}

// ReconciledEdge — 한 통신 엣지의 대조 결과(§3.3 엣지판 + §12 posture 오버레이).
type ReconciledEdge struct {
	Key             EdgeKey
	State           State                      // CONFIRMED | UNDECLARED | UNOBSERVED
	Confidence      float64                    // §3.5 (상태 + 관측 evidence)
	Posture         discoveryv1.QuantumPosture // 관측 엣지의 양자내성 posture(§12.1). 미관측이면 UNSPECIFIED
	Group           string                     // 협상된 KEX 그룹(표시용)
	NeedsReview     bool                       // UNDECLARED·UNOBSERVED·off-scope는 사람 판정 필수
	RescanCandidate bool                       // UNOBSERVED인데 네트워크 계층 갭으로 설명됨(§2.7)
	OffScopeDst     bool                       // dst가 스코프 마스터 미등재 → "등재 판정 요청"(§0.4/§5, IC-E3)
}

// reconcileEdges — 관측 엣지(network-collector) vs 선언 엣지를 3-상태로 분류한다(IC-E1).
//
// **조직 검사를 지난 뒤에만 불린다**([Engine.ReconcileEdges]). 관측 엣지에는 조직이 없어
// 여기서 찍는다 — 관측은 늘 그 엔진의 조직에서 온 것이다.
//   - 관측 ∩ 선언  → CONFIRMED
//   - 관측 only    → UNDECLARED(shadow 통신 — 보안 최우선)
//   - 선언 only    → UNOBSERVED (네트워크 갭이면 재수집 후보; §12.2 미관측≠부재)
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

	// 관측 기준: 선언에도 있으면 CONFIRMED, 없으면 UNDECLARED(shadow).
	for _, oe := range observed {
		key, offScope := observedEdgeKey(o, oe, scope)
		if seen[key] {
			continue
		}
		seen[key] = true

		ev := evidenceStr(normalize.EvidenceStrength(oe.GetDetectionMethod()))
		post := posture.Classify(oe.GetNegotiatedGroup(), oe.GetCipher())
		re := ReconciledEdge{
			Key:         key,
			Posture:     post,
			Group:       oe.GetNegotiatedGroup(),
			OffScopeDst: offScope,
		}
		if dset[key] {
			re.State = Confirmed
			re.Confidence = confidence(Confirmed, ev)
			re.NeedsReview = offScope // 정상 CONFIRMED는 자동통과 후보, off-scope면 리뷰
		} else {
			re.State = Undeclared // 관측 only = shadow 통신
			re.Confidence = confidence(Undeclared, ev)
			re.NeedsReview = true
		}
		out = append(out, re)
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
	return out
}

// observedEdgeKey — ObservedEdge에서 EdgeKey를 만들고 off-scope 여부를 판정한다.
// dst_node_id가 해소돼 스코프에 등재돼 있으면 in-scope, 아니면(미해소 또는 미등재) off-scope.
func observedEdgeKey(o org.ID, oe *discoveryv1.ObservedEdge, scope map[string]bool) (EdgeKey, bool) {
	dst := oe.GetDstNodeId()
	offScope := false
	if dst == "" {
		dst = oe.GetDstAddr() // 미해소 상대 → 원시 주소로 식별
		offScope = true
	} else if scope != nil && !scope[dst] {
		offScope = true // 해소됐지만 스코프 마스터 미등재
	}
	return EdgeKey{
		Org:   o,
		Src:   oe.GetSrcNodeId(),
		Dst:   dst,
		Port:  oe.GetPort(),
		Proto: protoName(oe.GetProtocol()),
	}, offScope
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
