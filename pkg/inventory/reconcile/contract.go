package reconcile

import inventoryv1 "github.com/pqcota/pqcota/gen/pqcota/inventory/v1"

// 계약과의 경계 — 어휘의 SSOT는 pqcota의 contracts다.
//
// 안에서는 State(문자열)로 다룬다. 리포트·CSV로 나가는 값이라 그 편이 다루기 쉽고, 대조
// 로직이 proto 타입에 묶이지 않는다. 대신 **밖으로 나가는 자리에서는 계약 어휘로 바꾼다** —
// 그래야 다른 소비자가 같은 말을 듣는다.
//
// 매핑이 빠지면 contract_test.go가 잡는다. 계약에 상태가 하나 더 생기면 그 테스트가 먼저 깨진다.

// ToProto — 내부 상태를 계약 어휘로.
func (s State) ToProto() inventoryv1.ReconState {
	switch s {
	case Confirmed:
		return inventoryv1.ReconState_RECON_STATE_CONFIRMED
	case Undeclared:
		return inventoryv1.ReconState_RECON_STATE_UNDECLARED
	case Unobserved:
		return inventoryv1.ReconState_RECON_STATE_UNOBSERVED
	}
	return inventoryv1.ReconState_RECON_STATE_UNSPECIFIED
}

// StateFromProto — 계약 어휘를 내부 상태로. 모르는 값은 빈 State다(추측하지 않는다).
func StateFromProto(p inventoryv1.ReconState) (State, bool) {
	switch p {
	case inventoryv1.ReconState_RECON_STATE_CONFIRMED:
		return Confirmed, true
	case inventoryv1.ReconState_RECON_STATE_UNDECLARED:
		return Undeclared, true
	case inventoryv1.ReconState_RECON_STATE_UNOBSERVED:
		return Unobserved, true
	}
	return "", false
}
