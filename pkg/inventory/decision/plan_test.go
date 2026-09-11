package decision

import (
	"errors"
	"testing"
)

func finalizedSession(t *testing.T) *Session {
	t.Helper()
	s := NewSession("ring:canary", []Item{{ID: "a", Mandatory: true}})
	_ = s.StartReview()
	s.Decide("a", "ok")
	s.Sign("alice", "ed25519:sig")
	if err := s.Finalize(); err != nil {
		t.Fatal(err)
	}
	return s
}

// IC-P1·P2: 판정이 끝난 세션 → 계획. deploy_automation_level은 자산별.
func TestBuildPlan(t *testing.T) {
	s := finalizedSession(t)
	items := []PlanItem{
		{NodeID: "pay-01", RemediationClass: "provider-inject", DeployAutomationLevel: "L2"},
		{NodeID: "worker-09", RemediationClass: "provider-inject", DeployAutomationLevel: "L3"},
	}
	p, err := BuildPlan(s, items)
	if err != nil {
		t.Fatal(err)
	}
	if p.ReviewerSig == "" {
		t.Error("계획에 판정자 표시가 없다")
	}
	if p.Items[0].DeployAutomationLevel != "L2" || p.Items[1].DeployAutomationLevel != "L3" {
		t.Error("deploy_automation_level 자산별 판정 반영 안 됨(IC-P2)")
	}
}

// IC-P4: 판정이 끝나지 않은 세션 → 계획 생성 거부.
func TestBuildPlan_notFinalized(t *testing.T) {
	s := NewSession("r", []Item{{ID: "a", Mandatory: true}})
	_ = s.StartReview() // in-review (not finalized)
	if _, err := BuildPlan(s, nil); !errors.Is(err, ErrNotJudged) {
		t.Errorf("err = %v, want ErrNotJudged", err)
	}
}

// IC-P4·P5: 넘김 관문 — 판정자 표시가 있는 계획만 계약으로 넘긴다. 실행 근거가 되는 것은
// 상류의 승인이 FINALIZED 로 올린 뒤다(§3.7). 이 관문은 실행을 허용하지 않는다.
func TestReadyForApproval(t *testing.T) {
	s := finalizedSession(t)
	p, _ := BuildPlan(s, []PlanItem{{NodeID: "n"}})
	if err := ReadyForApproval(p); err != nil {
		t.Errorf("판정이 끝난 계획인데 거부됐다: %v", err)
	}
	// 판정자 표시 없는(가짜) 계획 → 거부
	if err := ReadyForApproval(&JudgedPlan{Scope: "r"}); !errors.Is(err, ErrNotJudged) {
		t.Errorf("판정자 표시 없는 계획이 통과됐다: %v", err)
	}
	if err := ReadyForApproval(nil); !errors.Is(err, ErrNotJudged) {
		t.Error("nil 계획 통과됨")
	}
}

// IC-P3: 규제 자산 → FIPS provider 라우팅 강제.
func TestRouteProvider(t *testing.T) {
	if got := RouteProvider("jca", true); got != "BC-FJA" {
		t.Errorf("규제 JCA = %q, want BC-FJA(FIPS 140-3)", got)
	}
	if got := RouteProvider("jca", false); got != "BouncyCastle" {
		t.Errorf("일반 JCA = %q, want BouncyCastle", got)
	}
	if got := RouteProvider("openssl", true); got != "openssl-fips-provider" {
		t.Errorf("규제 OpenSSL = %q", got)
	}
	// **CNG 는 provider 라우팅으로 답하지 않는다.** 갈아 끼울 대상이 관측에 없고,
	// FIPS 여부는 알 수 없다(§2.5) — 이름을 지어내면 계획을 받는 쪽이 그것을 검증된
	// 선택으로 읽는다. 무엇을 할지는 사람이 적은 결론이 담는다.
	for _, fips := range []bool{true, false} {
		if got := RouteProvider("cng", fips); got != "" {
			t.Errorf("CNG(fips=%v) = %q — provider 이름을 지어냈다", fips, got)
		}
	}
}
