package reconcile_test

import (
	"testing"

	inventoryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/inventory/v1"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

// 계약에 정의된 상태 전부에 대응이 있어야 한다. 상류가 상태를 하나 더하면 여기서 먼저 깨진다 —
// 조용히 UNSPECIFIED로 떨어지면 "모르는 것"이 "확인된 것"처럼 흘러간다.
func TestEveryContractStateMaps(t *testing.T) {
	for v, name := range inventoryv1.ReconState_name {
		p := inventoryv1.ReconState(v)
		if p == inventoryv1.ReconState_RECON_STATE_UNSPECIFIED {
			continue
		}
		got, ok := reconcile.StateFromProto(p)
		if !ok {
			t.Errorf("계약 상태 %s에 대응하는 내부 상태가 없다 — 매핑을 더할 것", name)
			continue
		}
		if back := got.ToProto(); back != p {
			t.Errorf("%s 왕복이 어긋난다: %v → %q → %v", name, p, got, back)
		}
	}
}

// 모르는 값을 추측하지 않는다.
func TestUnknownStateIsNotGuessed(t *testing.T) {
	if _, ok := reconcile.StateFromProto(inventoryv1.ReconState_RECON_STATE_UNSPECIFIED); ok {
		t.Fatal("UNSPECIFIED를 내부 상태로 받아들였다 — 관측하지 못한 것을 확정하면 안 된다")
	}
}
