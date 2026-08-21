// Package reconcile implements the Inventory 3-state reconciliation engine (규정서 §3.3).
//
// pqcota가 만들지 않기로 한 계층이다 — 관측은 그쪽이 하고, 대조는 여기서 한다.
// 선언(declared) 증거원과 관측(observed) 증거원을 대조해 각 자산·엣지를 3-상태로 분류한다.
// 대조 엔진은 "정답"이 아니라 판정 대상을 구조화한다 — 확정은 사람(리뷰-확정, §3.1).
package reconcile

import "github.com/randyinthedev-hash/pqcota/pkg/org"

// State — 3-상태 reconciliation 결과(§3.3).
//
// 어휘의 SSOT는 pqcota의 계약(`inventoryv1.ReconState`)이다 — 밖으로 나갈 때 그것으로 바꾼다
// (contract.go). 안에서 문자열을 쓰는 것은 리포트·CSV로 그대로 나가기 때문이다.
type State string

const (
	// Confirmed — 선언 ∩ 관측. 신뢰도 최상.
	Confirmed State = "CONFIRMED"
	// Undeclared — 관측 only. 선언에 없는데 실재하는 것 — 보안 최우선 발견.
	Undeclared State = "UNDECLARED"
	// Unobserved — 선언 only. 실존(DR/배치) vs stale vs 커버리지 갭 — 기계 확정 불가(MANUAL).
	Unobserved State = "UNOBSERVED"
)

// AssetKey — reconciliation 대상의 동일성. 노드는 스코프 마스터 앵커(§0.4).
type AssetKey struct {
	// Org — 어느 조직의 자산인가. **동일성의 일부다** — 노드 이름이 같아도 조직이 다르면
	// 다른 자산이다. 열쇠에 박아 두면 섞인 입력이 맞아떨어지는 일이 구조적으로 없다.
	// 판정 원장의 대상 id 에는 넣지 않는다 — 원장이 이미 조직별로 갈려 있어 중복이다.
	Org       org.ID
	NodeID    string
	Runtime   string // openssl | jca
	Component string // libcrypto.so.3, jca-provider-chain 등
}

// Observed — 관측된 자산 + 증거강도(§2.4). Evidence는 confidence를 좌우한다(§3.5).
type Observed struct {
	Key      AssetKey
	Evidence string // confirmed | inferred-high | inferred-low | "" (unknown)
}

// Reconciled — 한 대상의 대조 결과.
type Reconciled struct {
	Key             AssetKey
	State           State
	Confidence      float64 // §3.5 (상태 + 관측 evidence 기반. 실측 캘리브레이션은 §11)
	NeedsReview     bool    // UNDECLARED·UNOBSERVED은 사람 판정 필수(§3.5 MANUAL)
	RescanCandidate bool    // UNOBSERVED인데 커버리지 갭으로 설명됨 → 재수집 후보(§3.3, §2.7)
}

// reconcileAssets — 선언 집합 vs 관측 집합을 대조해 3-상태로 분류한다(§3.3①).
//
// **조직 검사를 지난 뒤에만 불린다**([Engine.Reconcile]). 여기서 다시 보지 않는 것은
// 검사가 두 곳에 있으면 한쪽만 고쳐지는 날이 오기 때문이다.
// gapLayers(관측 완전성 맵의 미커버 계층)가 있으면, UNOBSERVED는 "실제 없음"이 아니라
// "원리상 못 봄일 수 있음" → RescanCandidate로 표시한다(IC-R4, §2.7 갭≠부재).
func reconcileAssets(declared []AssetKey, observed []Observed, gapLayers []string) []Reconciled {
	dset := toSet(declared)
	oset := make(map[AssetKey]bool, len(observed))
	for _, o := range observed {
		oset[o.Key] = true
	}
	hasGaps := len(gapLayers) > 0
	seen := map[AssetKey]bool{}
	var out []Reconciled

	// 관측 기준: 선언에도 있으면 CONFIRMED, 없으면 UNDECLARED.
	for _, o := range observed {
		if seen[o.Key] {
			continue
		}
		seen[o.Key] = true
		if dset[o.Key] {
			out = append(out, Reconciled{Key: o.Key, State: Confirmed, Confidence: confidence(Confirmed, o.Evidence)})
		} else {
			out = append(out, Reconciled{Key: o.Key, State: Undeclared, Confidence: confidence(Undeclared, o.Evidence), NeedsReview: true})
		}
	}
	// 선언만 있고 관측 안 됨 → UNOBSERVED. 커버리지 갭이면 재수집 후보.
	for _, k := range declared {
		if seen[k] {
			continue
		}
		seen[k] = true
		if !oset[k] {
			out = append(out, Reconciled{Key: k, State: Unobserved, Confidence: ConfidenceFor(Unobserved), NeedsReview: true, RescanCandidate: hasGaps})
		}
	}
	return out
}

// ConfidenceFor — 상태 기반 기본 confidence(§3.5). CONFIRMED > UNDECLARED > UNOBSERVED.
func ConfidenceFor(s State) float64 {
	switch s {
	case Confirmed:
		return 0.9
	case Undeclared:
		return 0.6
	case Unobserved:
		return 0.3
	default:
		return 0
	}
}

// confidence — 관측 evidence_strength로 상한을 조정한다(IC-C2). inferred-low는 confidence를 누른다.
// UNOBSERVED(관측 없음)에는 evidence 무관.
func confidence(state State, evidence string) float64 {
	base := ConfidenceFor(state)
	switch evidence {
	case "confirmed":
		return base
	case "inferred-high":
		return base * 0.9
	case "inferred-low":
		return base * 0.6 // 불확실 관측 → 신뢰 하향
	default:
		return base * 0.8 // unknown
	}
}

func toSet(ks []AssetKey) map[AssetKey]bool {
	m := make(map[AssetKey]bool, len(ks))
	for _, k := range ks {
		m[k] = true
	}
	return m
}
