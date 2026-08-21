package reconcile

import (
	"errors"
	"fmt"

	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/history"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
)

// ErrOrgMismatch — 대조할 입력에 다른 조직의 것이 섞여 있다.
//
// **한 프로세스가 여러 조직을 다루는 배포에서 이것이 유일한 방벽이다.** pqcota의 스냅샷에도
// 계약의 `Envelope`에도 조직 필드가 없어(§1.4는 노드까지다) 엔진이 스스로 알아낼 길이
// 없다 — 조직은 부르는 쪽이 단언하고, 그 단언이 어긋나는 순간을 여기서 잡는다.
var ErrOrgMismatch = errors.New("the input contains something from another organization")

// Engine — 조직 하나에 묶인 대조 엔진.
//
// **조직을 인자로 받지 않고 핸들이 든다.** 인자로 받으면 부르는 자리마다 옳게 넘겼는지를
// 다시 확인해야 하고, 한 군데만 빠져도 조용히 섞인다. 판정 저장소가 이미 같은 모양이다
// (`decision.NewFileJudgmentStore`).
type Engine struct{ org org.ID }

// For — 조직을 지정해 엔진을 연다. **빈 조직은 열리지 않는다** — 판정 저장소와 같은 규칙이다.
func For(o org.ID) (*Engine, error) {
	if o == "" {
		return nil, org.ErrEmpty
	}
	return &Engine{org: o}, nil
}

// Org — 이 엔진이 묶인 조직.
func (e *Engine) Org() org.ID { return e.org }

// AssetsFromSnapshot — 관측 레인의 자산을 뽑고 **이 엔진의 조직을 찍는다.**
//
// 스냅샷은 조직을 모른다. 찍는 자리를 여기 하나로 몰아 두면, 조직 없는 열쇠가 만들어질 수
// 있는 곳이 테스트 말고는 남지 않는다.
func (e *Engine) AssetsFromSnapshot(snap *history.Snapshot) []Observed {
	return stampObserved(e.org, observedFromSnapshot(snap))
}

// AssetsFromResults — 선언 레인의 자산을 뽑고 이 엔진의 조직을 찍는다.
func (e *Engine) AssetsFromResults(results []*discoveryv1.CollectionResult) ([]AssetKey, error) {
	out, err := declaredFromResults(results)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Org = e.org
	}
	return out, nil
}

// Reconcile — 3-상태 대조(§3.3①). **다른 조직이 섞였으면 대조하지 않고 끊는다.**
//
// 열쇠에 조직이 들어 있으므로 섞인 입력은 그냥 두면 서로 안 맞아 CONFIRMED가 UNDECLARED와
// UNOBSERVED 한 쌍으로 구분된다 — 오류가 아니라 **그럴듯한 결과**로 나온다. 그래서 대조보다
// 검사가 먼저다.
func (e *Engine) Reconcile(declared []AssetKey, observed []Observed, gapLayers []string) ([]Reconciled, error) {
	for _, k := range declared {
		if err := e.want(k.Org, "declared", k.NodeID); err != nil {
			return nil, err
		}
	}
	for _, o := range observed {
		if err := e.want(o.Key.Org, "observed", o.Key.NodeID); err != nil {
			return nil, err
		}
	}
	return reconcileAssets(declared, observed, gapLayers), nil
}

// ReconcileEdges — 통신 엣지의 3-상태 대조(IC-E1). 자산과 같은 규칙으로 끊는다.
func (e *Engine) ReconcileEdges(declared []EdgeKey, observed []*discoveryv1.ObservedEdge,
	scope map[string]bool, gapLayers []string) ([]ReconciledEdge, error) {
	for _, k := range declared {
		if err := e.want(k.Org, "declared edge", k.Src+"→"+k.Dst); err != nil {
			return nil, err
		}
	}
	return reconcileEdges(e.org, declared, observed, scope, gapLayers), nil
}

// want — 조직이 이 엔진의 것인가. **어긋난 값을 양쪽 다 적는다** — "다른 조직이 섞였다"만
// 보면 어느 쪽을 고쳐야 하는지 알 수 없다.
func (e *Engine) want(got org.ID, lane, what string) error {
	switch got {
	case e.org:
		return nil
	case "":
		return fmt.Errorf("%w: %s %s has no organization - this engine is %q", ErrOrgMismatch, lane, what, e.org)
	default:
		return fmt.Errorf("%w: %s %s is %q but this engine is %q", ErrOrgMismatch, lane, what, got, e.org)
	}
}

func stampObserved(o org.ID, in []Observed) []Observed {
	for i := range in {
		in[i].Key.Org = o
	}
	return in
}
