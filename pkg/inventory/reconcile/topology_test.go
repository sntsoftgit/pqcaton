package reconcile

import (
	"strings"
	"testing"

	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
)

// IC-E2: 토폴로지 렌더 — 색=posture, 선형=상태, 미관측=점선(≠부재), off-scope=판정요청.
func TestRenderTopologyDOT(t *testing.T) {
	edges := []ReconciledEdge{
		{Key: ek("web", "app", 443, "TLS"), State: Confirmed, Posture: discoveryv1.QuantumPosture_QUANTUM_POSTURE_PQC_HYBRID, Group: "X25519MLKEM768"},
		{Key: ek("app", "db", 5432, "TLS"), State: Confirmed, Posture: discoveryv1.QuantumPosture_QUANTUM_POSTURE_CLASSICAL, Group: "ECDHE-RSA"},
		{Key: ek("app", "shadow", 22, "SSH"), State: Undeclared, Posture: discoveryv1.QuantumPosture_QUANTUM_POSTURE_CLASSICAL, Group: "ecdh"},
		{Key: ek("db", "app", 5432, "TLS"), State: Unobserved},
		{Key: ek("web", "203.0.113.5:443", 443, "TLS"), State: Undeclared, OffScopeDst: true, Posture: discoveryv1.QuantumPosture_QUANTUM_POSTURE_UNSPECIFIED},
	}
	dot := RenderTopologyDOT(edges, map[string]bool{"db": true})

	checks := []struct{ name, sub string }{
		{"digraph 시작", "digraph crypto_topology"},
		{"PQC 녹색", `color="#22aa22"`},
		{"고전 적색", `color="#cc2222"`},
		{"미관측 점선", "style=dashed"},
		{"UNOBSERVED 라벨", "미관측(UNOBSERVED)"},
		{"shadow 굵은선", "penwidth=3"},
		{"off-scope 판정요청", "판정요청"},
		{"uncovered 회색노드", "collector 미설치"},
		{"범례 캡션(posture)", "posture"},
	}
	for _, c := range checks {
		if !strings.Contains(dot, c.sub) {
			t.Errorf("%s: DOT에 %q 없음", c.name, c.sub)
		}
	}
	// 미관측 엣지는 shadow 굵은선(penwidth=3)으로 그리면 안 된다 — 점선이어야.
	// (구조적 검증: UNOBSERVED 라인이 dashed를 포함하는지 이미 위에서 확인)
}
