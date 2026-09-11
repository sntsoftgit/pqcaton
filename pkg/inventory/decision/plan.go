package decision

import "errors"

// ErrNotJudged — 판정이 끝나지 않은 세션에서 계획을 만들거나 넘기려 할 때(§5, 이 리포의 가장 센 관문).
//
// **상류의 ErrNotFinalized 와 다른 것이다.** 저쪽은 실행 승인이 없다는 뜻이고, 이쪽은 판정이
// 끝나지 않았다는 뜻이다. 판정과 실행 승인은 다른 단계라 이름을 갈라 둔다.
var ErrNotJudged = errors.New("plan: a plan can only be built or handed over from a session whose judging is finished (§5, the strongest gate)")

// PlanItem — 판정이 끝난 계획의 자산별 실행 항목(인벤토리 설계 §2). 스키마는 contracts 통제 어휘.
type PlanItem struct {
	NodeID                string
	RemediationClass      string // taxonomy 분기(§4.3/§4.4)
	DeployAutomationLevel string // L1/L2/L3 — 자산별 리뷰어 판정(§4.5, IC-P2)
	ProviderChoice        string // FIPS 라우팅 결과(§4.10)
}

// JudgedPlan — **판정이 끝난** 계획. 실행 근거가 아니다.
//
// 이 리포가 만드는 것은 여기까지다. 실행 근거(상류의 FINALIZED)가 되려면 상류에서 승인자가
// 자기 키로 서명해야 하고(pqcota-approve), 그 단계는 이 리포 밖이다. 전에는 이 타입이
// FinalizedPlan 이었고 주석이 「프로비저닝의 유일 실행 근거」라고 적혀 있었다 — 계약으로
// IN_REVIEW 를 내보내는 순간 그 이름은 자기가 하지 않는 일을 주장한다.
type JudgedPlan struct {
	Scope string
	Items []PlanItem
	// ReviewerSig — 판정자가 세션에 적은 표시. **승인 서명이 아니다.** 검증되지 않는 자유
	// 문자열이라 「누가 판정했나」가 아니라 「누가 판정했다고 기록됐나」까지만 말한다. 계약의
	// approval_signatures 에는 넣지 않는다 — 그 칸은 실행 승인의 자리다.
	ReviewerSig string
}

// BuildPlan — 판정이 끝난 세션에서만 계획을 만든다(IC-P1/P5). draft/in-review면 거부(§5).
// 이 생성 제약이 "판정이 끝나지 않은 계획은 존재할 수 없다"를 코드로 보장한다.
func BuildPlan(s *Session, items []PlanItem) (*JudgedPlan, error) {
	if s.Status != Finalized {
		return nil, ErrNotJudged
	}
	return &JudgedPlan{Scope: s.Scope, Items: items, ReviewerSig: s.Signature}, nil
}

// ReadyForApproval — 계약으로 넘길 수 있는 계획인가(§5). 판정자 표시가 있는 계획만 넘긴다(IC-P4).
//
// Deploy 를 허용하는 관문이 아니다. 전에는 AcceptForDeploy 였고 「Inventory→Deploy 가 반드시
// 거쳐야 하는 관문 · 실행 허용」이라고 적혀 있었는데, 이 리포는 실행을 허용하지 않는다. 실행
// 허용은 상류의 Executable 과 승인 검증이 한다. 여기서 보는 것은 **판정 세션이 닫혔다는 표시가
// 있는가** 하나다.
func ReadyForApproval(p *JudgedPlan) error {
	if p == nil || p.ReviewerSig == "" {
		return ErrNotJudged
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
