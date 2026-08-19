package scope

import (
	"sort"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// Excluded — 정책이 뺀 자산 하나.
//
// **pqcota는 제외분을 세기만 한다**(`Snapshot.ExcludedByScope`) — 잡음을 거르는 것이 목적이라
// 그것으로 충분하다. 거버넌스는 셈이 아니라 **이름**이 필요하다. 사고 뒤에 "왜 이게
// 인벤토리에 없었나"에 답하려면 무엇이 어느 규칙으로 빠졌는지 말할 수 있어야 한다.
type Excluded struct {
	Node     string
	Runtime  string
	Asset    string
	Evidence string // confirmed | inferred-high | inferred-low | "" (unknown)

	// StillObserved — 지금도 관측되는가.
	//
	// **여기 있는 것은 전부 그렇다.** 관측된 finding 을 정책에 통과시켜 걸러낸 것이므로,
	// 이 목록은 곧 **실재하는데 우리가 안 보고 있는 것**이다. 그래서 재검토 대상이다.
	StillObserved bool
}

// ExcludedFrom — 한 노드의 관측 finding 을 정책에 통과시켜 **빠지는 것을 이름으로** 낸다.
//
// 판정은 pqcota의 `Managed` 가 한다 — 여기서 glob 을 다시 구현하면 내려보낸 CSV 를 pqcota가
// 집행한 결과와 우리 화면이 갈라진다.
func ExcludedFrom(p *kscope.AssetPolicy, node string, findings []*discoveryv1.Finding) []Excluded {
	var out []Excluded
	for _, f := range findings {
		if p.Managed(f) {
			continue
		}
		rt, asset := name(f)
		if rt == "" {
			continue // 우리가 이름을 붙일 수 없는 런타임은 세지 않는다
		}
		out = append(out, Excluded{
			Node: node, Runtime: rt, Asset: asset,
			Evidence: evidence(f.GetEvidenceStrength()), StillObserved: true,
		})
	}
	return out
}

// Subject — 이 제외 자산의 판정 대상 키. `reconcile` 의 자산 키와 같은 모양이다 —
// 제외분 재검토도 결국 같은 자산에 대한 판정이라 대상이 갈리면 이력이 끊긴다.
func (e Excluded) Subject() string { return e.Node + "/" + e.Runtime + "/" + e.Asset }

// ReviewItem — 제외분 재검토 큐의 한 줄.
type ReviewItem struct {
	Excluded
	// Reason — 왜 지금 다시 보라는가.
	Reason string
}

// 재검토 사유. 문자열이 그대로 화면에 나가므로 여기 하나로 모은다.
const (
	ReasonNeverJudged = "제외를 승인한 판정이 없다"
	ReasonStale       = "승인한 지 오래됐다 — 빼둔 사이 달라졌을 수 있다"
)

// Review — 제외된 자산을 판정 이력과 맞대어 **다시 봐야 할 것만** 낸다.
//
// **제외는 영구 면제가 아니다.** 한 번 뺀 자산이 그대로 잊히면, 그 사이 그 자산이 위험해져도
// 인벤토리는 아무 말도 하지 않는다 — 「제외 ≠ 부재」가 적재 시점에만 참이고 시간 축에서는
// 거짓이 되는 자리다.
//
// 두 가지를 본다. 승인한 판정이 아예 없는 것(정책만 있고 결정이 없다), 그리고 승인은
// 있었지만 ttl 을 넘긴 것. 둘 다 아니면 조용히 둔다 — 매번 전부 올리면 아무도 안 본다.
func Review(ex []Excluded, prior []decision.Judgment, now, ttlSeconds int64) []ReviewItem {
	latest := map[string]decision.Judgment{}
	for _, j := range decision.LatestPerSubject(prior) {
		latest[j.Subject] = j
	}
	// 만료 판정은 pqcota가 아니라 우리 원장의 규칙이다(IC-D4) — 여기서 다시 세지 않는다.
	stale := map[string]bool{}
	for _, j := range decision.ExpireStale(decision.LatestPerSubject(prior), now, ttlSeconds, 1) {
		if j.Stale {
			stale[j.Subject] = true
		}
	}

	var out []ReviewItem
	for _, e := range ex {
		s := e.Subject()
		if _, ok := latest[s]; !ok {
			out = append(out, ReviewItem{Excluded: e, Reason: ReasonNeverJudged})
			continue
		}
		if stale[s] {
			out = append(out, ReviewItem{Excluded: e, Reason: ReasonStale})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Subject() < out[j].Subject() })
	return out
}

func name(f *discoveryv1.Finding) (runtime, asset string) {
	switch f.GetCryptoRuntime() {
	case commonv1.CryptoRuntime_CRYPTO_RUNTIME_OPENSSL:
		return "openssl", f.GetOpenssl().GetLib()
	case commonv1.CryptoRuntime_CRYPTO_RUNTIME_JCA:
		return "jca", "jca-provider-chain"
	}
	return "", ""
}

func evidence(e commonv1.EvidenceStrength) string {
	switch e {
	case commonv1.EvidenceStrength_EVIDENCE_STRENGTH_CONFIRMED:
		return "confirmed"
	case commonv1.EvidenceStrength_EVIDENCE_STRENGTH_INFERRED_HIGH:
		return "inferred-high"
	case commonv1.EvidenceStrength_EVIDENCE_STRENGTH_INFERRED_LOW:
		return "inferred-low"
	}
	return ""
}
