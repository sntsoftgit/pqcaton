package review_test

import (
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// ★ 사람이 고른 실행 필드가 계약까지 간다.
//
// 화면에 칸을 열어도 ToContract가 옮기지 않으면 계획은 그대로 빈칸이다. 상류는 그 빈칸을
// 「배치해도 아무것도 켜지지 않는다」로 알리고 종료 상태 3으로 끝낸다. 고른 것이 사라지는
// 자리가 없어야 그 경고가 실제로 꺼진다.
func TestChosenExecutionFieldsReachTheContract(t *testing.T) {
	s := &decision.Session{Status: decision.Finalized, Scope: "ring-0", Signature: "reviewer-1:sig"}
	p, err := decision.BuildPlan(s, []decision.PlanItem{
		{NodeID: "pay-db", DeployAutomationLevel: "L1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := review.ToContract(p, []review.Item{{
		Runtime: "openssl", Kind: "REMEDIATION_KIND_CONFIG_ONLY",
		TargetAlgorithm: "ML-KEM (FIPS 203)",
	}})
	if err != nil {
		t.Fatal(err)
	}
	a := got.GetActions()[0]
	if a.GetTargetAlgorithm() != "ML-KEM (FIPS 203)" {
		t.Errorf("목표 알고리즘이 계약까지 가지 않았다: %q", a.GetTargetAlgorithm())
	}
	if a.GetKind().String() != "REMEDIATION_KIND_CONFIG_ONLY" {
		t.Errorf("조치 종류가 갈렸다: %s", a.GetKind())
	}
	if a.GetAutomationLevel().String() != "DEPLOY_AUTOMATION_LEVEL_L1_STAGE_ONLY" {
		t.Errorf("위임 수준이 갈렸다: %s", a.GetAutomationLevel())
	}
	// 도구가 아는 추적 정보는 도구가 채운다 — 사람에게 물을 값이 아니다.
	if got.GetId() == "" {
		t.Error("계획 id가 비었다 — 상류 레코드가 plan_id로 되짚는다")
	}
	if got.GetFinalizedAt() == nil {
		t.Error("확정 시각이 비었다 — FINALIZED라고 하면서 언제인지 말하지 않는다")
	}
}
