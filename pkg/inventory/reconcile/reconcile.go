// Package reconcile implements the Inventory 3-state reconciliation engine (규정서 §3.3).
//
// pqcota가 만들지 않기로 한 계층이다 — 관측은 그쪽이 하고, 대조는 여기서 한다.
// 선언(declared) 증거원과 관측(observed) 증거원을 대조해 각 자산·엣지를 3-상태로 분류한다.
// 대조 엔진은 "정답"이 아니라 판정 대상을 구조화한다 — 확정은 사람(리뷰-확정, §3.1).
package reconcile

import (
	"sort"

	"github.com/randyinthedev-hash/pqcota/pkg/org"
)

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

// ManagedState — 관리 축. **대조 축(State)과 다른 것을 잰다.** State는 선언과 관측의
// 관계이고, 이것은 자산 스코프 정책의 판정이다. 한 열거형에 넣으면 「관측됐는가」와
// 「관리하는가」를 한 값으로 물을 수 없다. 상류 계약(ReconState)에는 자리가 없으므로
// **밖으로 내보내지 않는다** - 리포트·경고·세션 파일에만 나타난다.
//
// 값 하나가 아니라 근거의 구성에서 파생한다([Reconciled.Sources]·[Reconciled.ExcludedSources]).
type ManagedState string

const (
	// Managed — 관리 근거가 하나 이상 있다. 제외 근거가 함께 있어도 관리한다(혼합).
	Managed ManagedState = "MANAGED"
	// ExcludedByPolicy — 관측은 있으나 **전부** 정책이 뺐다. 리뷰 큐·자동통과·계획 후보에
	// 들어가지 않고, 리포트에 남는다. 선언에도 있으면(CONFIRMED) 선언과 정책이 어긋난 것이라
	// 경고를 낸다.
	ExcludedByPolicy ManagedState = "EXCLUDED_BY_POLICY"
	// NotEvaluated — 관측이 없어 정책을 걸 대상이 없었다. MANAGED로 적으면 하지 않은 판정을
	// 기록하는 것이고, EXCLUDED로 적으면 정책이 뺀 것과 구별되지 않는다.
	NotEvaluated ManagedState = "NOT_EVALUATED"
)

// AssetKey — reconciliation 대상의 동일성. 노드는 스코프 마스터 앵커(§1.4).
type AssetKey struct {
	// Org — 어느 조직의 자산인가. **동일성의 일부다** — 노드 이름이 같아도 조직이 다르면
	// 다른 자산이다. 식별자에 넣어 두면 섞인 입력이 맞아떨어지는 일이 구조적으로 없다.
	// 판정 원장의 대상 id에는 넣지 않는다 — 원장이 이미 조직별로 갈려 있어 중복이다.
	Org       org.ID
	NodeID    string
	Runtime   string // openssl | jca
	Component string // libcrypto.so.3, jca-provider-chain 등
}

// Observed — 관측된 자산 + 증거강도(§2.4). Evidence는 confidence를 좌우한다(§3.5).
type Observed struct {
	Key      AssetKey
	Evidence string // confirmed | inferred-high | inferred-low | "" (unknown)
	// FindingID — 이 자산을 낸 관측 Finding의 id. **조치의 근거로 상류까지 간다**(계약의
	// `finding_id`). 여기서 버리면 계획이 「무엇을 보고 정했나」를 대지 못해, 판정 원장에는
	// 근거가 있는데 실행 계획에는 없는 상태가 된다. UNOBSERVED는 관측이 없으니 빈다.
	FindingID string
	// Fingerprint — 그 관측의 **내용** 지문([Fingerprint]). id는 자산이 같으면 같으므로,
	// 버전이 오르거나 강화 판정이 달라진 것을 id로는 알 수 없다. 판정의 근거가 바뀌었는지는
	// 이 값으로 가른다.
	Fingerprint string
	// appKeys — 이 finding에 걸린 앱 식별자 전부. 제외 근거의 경고문이 쓴다. 관리 근거에는
	// 필요 없어 내보내지 않는다(소문자).
	appKeys []string
	// Snapshot — 이 관측이 속한 스냅샷 상태의 위치. 원천 노드(봉투의 target_node_id — 이력이
	// 스냅샷을 저장한 이름이고, 선언 노드와 다를 수 있다)와 v1 지문, 그 스냅샷의 규칙 판이다.
	// 상류로 건너가 「어느 스냅샷 상태에서 나온 조치인가」를 답한다. 근거 해시에는 넣지
	// 않는다 — 위치이지 근거가 아니다(근거는 Fingerprint가 덮는다).
	Snapshot SnapshotLocation
}

// SnapshotLocation — 관측이 속한 스냅샷 상태가 이력의 어디에 있나.
type SnapshotLocation struct {
	SourceNodeID   string // 봉투의 target_node_id. 이력이 스냅샷을 저장한 이름
	Digest         string // history.ContentHashV1 — 같은 결과·같은 규칙·같은 정책이면 상류와 같은 값
	RulesetVersion string // 그 **스냅샷의** 규칙 판(pqcota-enrich/…). 계획의 결합 판이 아니다
}

// Source — 한 자산의 근거 하나. 같은 자산을 원천 노드 여럿이 봤으면 여럿이고, 주 근거가 앞이다.
type Source struct {
	FindingID   string
	Fingerprint string
	Evidence    string
	Snapshot    SnapshotLocation
}

// ExcludedSource — 정책이 관리 대상에서 뺀 관측. **로컬 진단용이다.**
//
// 스냅샷 위치가 없다: 이 finding은 중앙 이력의 스냅샷에 들어가지 않았으므로 가리킬 위치가
// 없다. Source와 형을 따로 두는 것은 계약 변환 함수가 이 값을 받지 못하게 하려는 것이다 -
// 같은 형이면 언젠가 섞여, 풀릴 수 없는 참조가 계획에 실린다.
type ExcludedSource struct {
	FindingID    string
	Fingerprint  string
	Evidence     string
	SourceNodeID string // 어느 원천 노드에서 봤나. 진단용이고 참조가 아니다
	// AppKeys — 이 finding에 걸린 앱 식별자 **전부**. 계약이 repeated이고, 공유 .so 하나를
	// 여러 앱이 로드하면 여럿이다. 하나만 적으면 사람이 「이 앱만 빠졌다」고 읽는데 실제로는
	// 목록 전부가 함께 빠졌다. 경고문이 쓴다 - 사람이 정책 파일에서 이 식별자들에 걸리는 줄을
	// 찾는다. **규칙 이름은 담지 않는다**: 상류 Managed는 bool만 돌려주므로 알 수 없다.
	AppKeys []string
}

// Excluded — 정책이 뺀 관측 하나. Observed와 짝이고, 대조에 들어가 관리 축을 정한다.
type Excluded struct {
	Key    AssetKey
	Source ExcludedSource
}

// Reconciled — 한 대상의 대조 결과.
type Reconciled struct {
	Key AssetKey
	// FindingID — **주 근거**의 관측(있을 때만). Sources[0]과 같다.
	FindingID string
	// Fingerprint — 주 근거의 내용 지문. Sources[0]과 같다.
	Fingerprint string
	// Sources — 근거 전부. 같은 식별자의 관측을 **버리지 않고** 모은다. 원천 노드 둘이 선언 노드
	// 하나에 걸리고 같은 자산을 보면 둘이다. 주 근거가 앞이다: 가장 강한 증거, 같으면 원천 노드
	// 이름순. 전에는 같은 식별자의 첫 관측만 남겨 둘째 원천의 근거가 사라졌다 — 그러면 되짚기가
	// 관측 하나를 버리고 시작한다.
	Sources []Source
	// ExcludedSources — 정책이 뺀 근거. **계약으로 나가지 않는다**(형이 다르다). 근거 해시에는
	// 들어간다 - 제외 근거가 관리 근거로 바뀌거나 그 반대가 되면 판정 서명이 무효여야 한다.
	ExcludedSources []ExcludedSource
	State           State
	// Managed — 관리 축. Sources와 ExcludedSources의 구성에서 파생한다.
	Managed ManagedState
	// Confidence — §3.5 (상태 + 관측 evidence 기반. 실측 캘리브레이션은 설계 §1.2).
	// **ConfidenceEvaluated가 거짓이면 이 숫자는 잰 값이 아니다.** UNOBSERVED의 상태 기본값이나
	// 제외 전용의 0이 그렇다. 0.00과 「재지 않았다」는 다른 사실이라 따로 든다.
	Confidence          float64
	ConfidenceEvaluated bool
	NeedsReview         bool // UNDECLARED·UNOBSERVED은 사람 판정 필수(§3.5 MANUAL)
	RescanCandidate     bool // UNOBSERVED인데 커버리지 갭으로 설명됨 → 재수집 후보(§3.3, §2.6)
}

// reconcileAssets — 선언 집합 vs 관측 집합을 대조해 3-상태로 분류한다(§3.3①).
//
// **조직 검사를 지난 뒤에만 불린다**([Engine.Reconcile]). 여기서 다시 보지 않는 것은
// 검사가 두 곳에 있으면 한쪽만 고쳐지는 날이 오기 때문이다.
// gapLayers(관측 완전성 맵의 미커버 계층)가 있으면, UNOBSERVED는 "실제 없음"이 아니라
// "원리상 못 봄일 수 있음" → RescanCandidate로 표시한다(IC-R4, §2.6 갭≠부재).
//
// 관리 축은 근거의 구성에서 파생한다.
//
//	관리 근거 ≥ 1            → MANAGED (제외 근거가 함께 있어도. 대표값·신뢰도는 관리 근거에서만)
//	관측은 있으나 전부 제외   → EXCLUDED_BY_POLICY (대표값 비움 · 신뢰도 미평가 · 큐에 안 감)
//	관리·제외 근거 모두 없음  → UNOBSERVED + NOT_EVALUATED (신뢰도는 상태 기본값이되 미평가)
func reconcileAssets(declared []AssetKey, observed []Observed, excluded []Excluded, gapLayers []string) []Reconciled {
	dset := toSet(declared)
	hasGaps := len(gapLayers) > 0
	seen := map[AssetKey]bool{}
	var out []Reconciled

	// 같은 식별자의 관측을 **전부** 모은다. 버리면 둘째 원천의 근거가 사라진다.
	sources := map[AssetKey][]Source{}
	exsources := map[AssetKey][]ExcludedSource{}
	ordered := map[AssetKey]bool{}
	var order []AssetKey
	note := func(k AssetKey) {
		if !ordered[k] {
			ordered[k] = true
			order = append(order, k)
		}
	}
	for _, o := range observed {
		note(o.Key)
		sources[o.Key] = append(sources[o.Key], Source{FindingID: o.FindingID, Fingerprint: o.Fingerprint, Evidence: o.Evidence, Snapshot: o.Snapshot})
	}
	for _, e := range excluded {
		note(e.Key)
		exsources[e.Key] = append(exsources[e.Key], e.Source)
	}

	// 관측 기준: 선언에도 있으면 CONFIRMED, 없으면 UNDECLARED. 상태와 신뢰도는 **주 근거**로
	// 정한다 - 가장 강한 증거, 같으면 원천 노드 이름순. 입력 순서가 아니다.
	for _, k := range order {
		seen[k] = true
		r := Reconciled{Key: k, ExcludedSources: sortExcluded(exsources[k])}
		if dset[k] {
			r.State = Confirmed
		} else {
			r.State = Undeclared
		}
		srcs := sortSources(sources[k])
		if len(srcs) == 0 {
			// 제외 전용. 대표 근거를 비운다 - 최상위 FindingID는 계약의 호환용 finding_id로
			// 나가는데, 제외 근거를 올리면 계약에 없는 근거를 가리키는 호환 필드가 된다.
			// 큐에 가지 않으므로 사람 판정도 요구하지 않는다. 선언과 어긋난 사실은 경고가 맡는다.
			r.Managed = ExcludedByPolicy
			out = append(out, r)
			continue
		}
		p := srcs[0]
		r.Managed = Managed
		r.FindingID, r.Fingerprint, r.Sources = p.FindingID, p.Fingerprint, srcs
		r.Confidence, r.ConfidenceEvaluated = confidence(r.State, p.Evidence), true
		r.NeedsReview = r.State == Undeclared
		out = append(out, r)
	}
	// 선언만 있고 관측 안 됨 → UNOBSERVED. 커버리지 갭이면 재수집 후보. 정책을 걸 finding이
	// 없었으므로 관리 축은 미평가이고, 신뢰도도 상태 기본값일 뿐 잰 값이 아니다.
	for _, k := range declared {
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, Reconciled{Key: k, State: Unobserved, Managed: NotEvaluated,
			Confidence: ConfidenceFor(Unobserved), ConfidenceEvaluated: false,
			NeedsReview: true, RescanCandidate: hasGaps})
	}
	return out
}

// sortExcluded — 제외 근거의 순서. 원천 노드 이름순, 같으면 finding id 순. 근거 해시가 이 목록을
// 넣으므로 순서가 입력에 달리면 같은 근거에서 다른 해시가 난다.
func sortExcluded(es []ExcludedSource) []ExcludedSource {
	out := append([]ExcludedSource(nil), es...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SourceNodeID != out[j].SourceNodeID {
			return out[i].SourceNodeID < out[j].SourceNodeID
		}
		return out[i].FindingID < out[j].FindingID
	})
	return out
}

// evidenceRank — 증거 강도의 순서. 주 근거를 고를 때 쓴다. 미상이 가장 약하다.
func evidenceRank(e string) int {
	switch e {
	case "confirmed":
		return 0
	case "inferred-high":
		return 1
	case "inferred-low":
		return 2
	}
	return 3
}

// sortSources — 주 근거가 앞이다: 가장 강한 증거, 같으면 원천 노드 이름순, 그래도 같으면 finding id 순.
// 「입력 순서의 첫 것」이 아니다 — 그것은 결과 파일의 순서에 달렸다.
func sortSources(ss []Source) []Source {
	out := append([]Source(nil), ss...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := evidenceRank(out[i].Evidence), evidenceRank(out[j].Evidence)
		if ri != rj {
			return ri < rj
		}
		if out[i].Snapshot.SourceNodeID != out[j].Snapshot.SourceNodeID {
			return out[i].Snapshot.SourceNodeID < out[j].Snapshot.SourceNodeID
		}
		return out[i].FindingID < out[j].FindingID
	})
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
