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
		TargetAlgorithm: "ML-KEM (FIPS 203)", FindingID: "f-1",
		Activate: "systemctl reload app", Restart: "systemctl restart app",
	}}, "test-rules/v1", "sess-1")
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
	if a.GetFindingId() != "f-1" {
		t.Errorf("근거 관측이 계약까지 가지 않았다: %q — 조치가 무엇을 보고 정해졌는지 대지 못한다", a.GetFindingId())
	}
	if a.GetActivation().GetActivate() != "systemctl reload app" {
		t.Errorf("활성화 훅이 갈렸다: %+v", a.GetActivation())
	}
	// 도구가 아는 추적 정보는 도구가 채운다 — 사람에게 물을 값이 아니다.
	if got.GetId() == "" {
		t.Error("계획 id가 비었다 — 상류 레코드가 plan_id로 되짚는다")
	}
	if got.GetFinalizedAt() != nil {
		t.Error("확정 시각을 이쪽이 찍었다 — 그것은 상류의 첫 승인이 찍는 값이다")
	}
}

// ★ 훅을 하나도 적지 않으면 훅 묶음 자체를 붙이지 않는다.
//
// 빈 묶음을 붙이면 상류가 「훅이 있는데 명령이 비었다」와 「훅 자체가 없다」를 가리지 못한다.
// 상류는 L3에서 무엇이 **일어나지 않는지**를 알리는 쪽을 택했으므로, 그 구분이 살아 있어야 한다.
func TestNoHooksMeansNoHookBlock(t *testing.T) {
	s := &decision.Session{Status: decision.Finalized, Scope: "ring-0", Signature: "reviewer-1:sig"}
	p, _ := decision.BuildPlan(s, []decision.PlanItem{{NodeID: "n1", DeployAutomationLevel: "L2"}})
	got, err := review.ToContract(p, []review.Item{{Runtime: "openssl", Kind: "REMEDIATION_KIND_CONFIG_ONLY"}}, "test-rules/v1", "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.GetActions()[0].GetActivation() != nil {
		t.Error("훅을 적지 않았는데 빈 묶음이 붙었다")
	}
}
