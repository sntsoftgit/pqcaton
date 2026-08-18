package main

import (
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// IC-C1 — **스코프가 URI인 노드도 그대로 겨눈다.**
//
// v0.1.0 은 항목 id 를 `/` 로 쪼개 노드를 되찾았다. 스코프가 `host://local` 이면 첫 조각이
// `host:` 가 되어 조치가 있지도 않은 노드를 겨누고, 런타임이 빈 문자열이 되어 기본값
// openssl 로 조용히 떨어졌다 — 관측이 jca 여도 그랬다. 되찾지 않고 들고 다니게 고쳤다.
func TestContractKeepsURINodeAndRuntime(t *testing.T) {
	items := []itemFile{
		{ID: "host://local/jca/provider", Node: "host://local", Runtime: "jca", Conclusion: "교체한다"},
	}
	p := &decision.FinalizedPlan{
		Scope:       "host://local",
		ApprovalSig: "sig",
		Items:       []decision.PlanItem{{NodeID: "host://local", DeployAutomationLevel: "L2"}},
	}

	got := toContract(p, items)
	if n := len(got.GetActions()); n != 1 {
		t.Fatalf("조치가 1건이어야 하는데 %d건이다", n)
	}
	a := got.GetActions()[0]
	if a.GetTargetNodeId() != "host://local" {
		t.Errorf("겨눈 노드가 %q다 — 쪼개서 잃었다", a.GetTargetNodeId())
	}
	if a.GetCryptoRuntime() != commonv1.CryptoRuntime_CRYPTO_RUNTIME_JCA {
		t.Errorf("런타임이 %v다 — jca 인데 기본값으로 떨어졌다", a.GetCryptoRuntime())
	}
}

// IC-C2 — **node 가 빈 항목은 계획으로 나가지 않는다.**
//
// v0.1.0 이 낸 세션 파일에는 node 가 없다. 빈 채로 통과시키면 이름 없는 노드에 조치가
// 걸리므로, 확정 직전에 끊고 무엇을 해야 하는지 말한다.
func TestPlanRefusesItemWithoutNode(t *testing.T) {
	if err := requireNode(itemFile{ID: "host://local/openssl/libssl"}); err == nil {
		t.Fatal("node 가 없는데 통과했다")
	}
	if err := requireNode(itemFile{ID: "host://local/openssl/libssl", Node: "host://local"}); err != nil {
		t.Fatalf("node 가 있는데 막았다: %v", err)
	}
}
