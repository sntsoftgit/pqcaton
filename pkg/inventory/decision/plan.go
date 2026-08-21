package decision

import "errors"

var ErrNotFinalized = errors.New("plan: a finalized plan can only be built or run from a finalized session (§5, the strongest gate)")

// PlanItem — 확정 계획의 자산별 실행 항목(인벤토리 설계 §2). 스키마는 contracts 통제 어휘.
type PlanItem struct {
	NodeID                string
	RemediationClass      string // taxonomy 분기(§4.3/§4.4)
	DeployAutomationLevel string // L1/L2/L3 — 자산별 리뷰어 판정(§4.5, IC-P2)
	ProviderChoice        string // FIPS 라우팅 결과(§4.10)
}

// FinalizedPlan — 프로비저닝의 유일 실행 근거(§3.7). finalized 세션에서만 생성된다.
type FinalizedPlan struct {
	Scope       string
	Items       []PlanItem
	ApprovalSig string // 세션 승인 서명 (finalized 증명)
}

// BuildPlan — finalized 세션에서만 확정 계획을 만든다(IC-P1/P5). draft/in-review면 거부(§5).
// 이 생성 제약이 "finalized 아닌 계획은 존재할 수 없다"를 코드로 보장한다.
func BuildPlan(s *Session, items []PlanItem) (*FinalizedPlan, error) {
	if s.Status != Finalized {
		return nil, ErrNotFinalized
	}
	return &FinalizedPlan{Scope: s.Scope, Items: items, ApprovalSig: s.Signature}, nil
}

// AcceptForDeploy — Inventory→Deploy 에서 우회할 수 없는 게이트(§5). finalized(서명 있는) 계획만 실행 허용(IC-P4).
// 프로비저닝은 반드시 이 게이트를 통과한 계획만 받는다 — 우회 불가.
func AcceptForDeploy(p *FinalizedPlan) error {
	if p == nil || p.ApprovalSig == "" {
		return ErrNotFinalized
	}
	return nil
}

// RouteProvider — 규제 대상 자산(fips 요구)은 FIPS 검증 provider로 강제 라우팅한다(§4.10, IC-P3).
// fips 요구가 provider 선택을 강제한다 — 내부 미검증 provider 금지.
func RouteProvider(runtime string, fipsRequired bool) string {
	if fipsRequired {
		switch runtime {
		case "jca":
			return "BC-FJA" // FIPS 140-3
		case "openssl":
			return "openssl-fips-provider"
		case "cng":
			return "" // 아래와 같은 이유다
		}
	}
	switch runtime {
	case "jca":
		return "BouncyCastle"
	case "openssl":
		return "internal-pqc-provider"

	// **CNG 는 provider 를 갈아 끼우는 조치가 아니다.** 빠뜨린 것이 아니라 여기 답이
	// 없는 런타임이라, 갈래를 두어 그 사실을 적어 둔다.
	//
	// 갈아 끼울 대상이 없다 — 관측된 provider 가 전부 Microsoft 이름이고, 알고리즘을
	// 실제로 서비스하는 것은 그중 하나다(pqcota v0.6.0·v0.6.1 실측). PQC 가능 여부는
	// **Windows 빌드**가 정하고, FIPS 는 provider 설치가 아니라 **OS 정책**이다.
	//
	// 게다가 CNG 의 `fips_validation` 은 **알 수 없다**(알고리즘 열거로는 판별되지
	// 않는다, §2.5). 모르는 것을 특정 provider 이름으로 적으면, 계획을 받아 실행하는
	// 쪽은 그것을 검증된 선택으로 읽는다.
	//
	// 무엇을 할지는 사람이 적은 결론(`PlanItem.RemediationClass`)이 담는다.
	case "cng":
		return ""
	}
	return ""
}
