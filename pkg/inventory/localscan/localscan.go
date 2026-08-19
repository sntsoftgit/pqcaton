// Package localscan — **이 기계를** 관측하는 지름길.
//
// collector 가 할 일을 로컬에서 흉내 낸 것이다. 원래 관측은 pqcota 의 collector 가 대상
// 노드에서 하고, 이 리포는 그 결과를 받아 대조한다 — 그 경로가 `pqcaton-report` 다.
// 여기 있는 것은 **「체크아웃만으로 한 바퀴」를 위한 편의**이고, 그래서 두 가지 제약을
// 물려받는다:
//
//   - `/proc` 을 읽으므로 **Linux 에서만** 된다
//   - 대상은 언제나 **이 기계**다. 노드 이름은 결과에 붙이는 이름표일 뿐이다
//
// 그 두 가지를 조용히 넘기면 **관측하지 못한 것이 「없는 것」으로 읽힌다** — 이 리포가
// 내내 경계해 온 바로 그것이다. 그래서 여기서 말하고 끊는다.
package localscan

import (
	"errors"
	"fmt"

	"github.com/randyinthedev-hash/pqcota/discovery/collectors/openssl"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/history"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	"github.com/randyinthedev-hash/pqcota/pkg/kernel/registry"
)

// ErrNoProc — `/proc` 을 열 수 없다. 비-리눅스이거나 마운트되지 않았다.
//
// **관측이 0건인 것과 다르다.** 0건은 「아무것도 안 쓰고 있다」일 수 있지만, 이것은
// 「볼 수가 없었다」다. 그 상태로 대조하면 선언한 자산이 전부 UNOBSERVED 로 나오고,
// 리포트는 「못 본 것: 없습니다」라고까지 말한다 — **관측을 아예 못 한 기계에서.**
var ErrNoProc = errors.New("이 기계의 /proc 을 열 수 없다 — 이 OS 에서는 로컬 관측이 되지 않는다")

// DefaultNode — 이름을 주지 않았을 때 붙이는 이름.
const DefaultNode = "host://local"

// Check — 스캔 통계를 보고 이 결과를 믿어도 되는지 정한다.
//
// **끊는 것과 경고하는 것을 가른다.** `/proc` 이 아예 없으면 결과가 무의미하므로 끊고,
// 열리긴 했는데 하나도 못 읽었으면(권한) 결과는 낼 수 있으니 말만 한다.
func Check(procUnavailable bool, accessible, denied int) (warn string, err error) {
	if procUnavailable {
		return "", fmt.Errorf("%w\n"+
			"   관측은 Linux 노드에서 pqcota 의 collector 가 합니다.\n"+
			"   그 결과를 모아 대조하려면 `pqcaton-report <results-dir> <declaration.json>` 을 쓰십시오",
			ErrNoProc)
	}
	if accessible == 0 {
		return fmt.Sprintf("접근 가능한 프로세스가 0개입니다(거부 %d) — **관측이 안 된 것이지 "+
			"자산이 없는 것이 아닙니다.** 권한을 올려 다시 돌리거나, 이 결과를 완전한 관측으로 "+
			"보지 마십시오", denied), nil
	}
	return "", nil
}

// LabelWarning — 이 기계를 스캔해 놓고 다른 이름을 붙일 때의 경고.
//
// 노드 이름은 **이름표일 뿐 대상이 아니다.** `pqcaton-decide open decl.csv web-gw` 는
// `web-gw` 를 관측하는 것이 아니라 **이 기계를 관측해 `web-gw` 라고 적는다** — 이름이
// 맞으면 선언과 대조까지 되어 CONFIRMED 가 나온다. 다른 기계의 관측으로.
func LabelWarning(node string) string {
	if node == "" || node == DefaultNode {
		return ""
	}
	return fmt.Sprintf("결과를 %q 로 기록하지만 **스캔한 것은 이 기계입니다** — "+
		"%s 를 관측한 것이 아닙니다. 다른 노드를 관측하려면 pqcota 의 collector 를 그 노드에서 "+
		"돌리고 `pqcaton-report` 로 모으십시오", node, node)
}

// Result — 이 기계를 관측한 결과.
type Result struct {
	Snapshot *history.Snapshot
	// Warnings — 결과는 냈지만 사람이 알아야 하는 것.
	Warnings []string
	// Accessible · Denied — 스캔이 무엇을 볼 수 있었나.
	Accessible, Denied int
}

// Scan — 이 기계를 관측해 스냅샷으로 만든다. `node` 는 결과에 붙일 이름이다.
//
// 두 명령(`pqcaton-decide open`·`pqcaton-reconcile`)이 같은 것을 하므로 여기 하나만 둔다 —
// 두 벌이면 한쪽만 고쳐지는 날이 온다.
func Scan(node string) (*Result, error) {
	if node == "" {
		node = DefaultNode
	}
	dets, st := openssl.ScanHost(registry.DefaultForkSignatures)
	warn, err := Check(st.ProcUnavailable, st.Accessible, st.Denied)
	if err != nil {
		return nil, err
	}
	res := openssl.BuildResult(node, dets)
	snap, err := normalize.Normalize([]*discoveryv1.CollectionResult{res},
		"snap-1", node, "ruleset-1", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("정규화: %w", err)
	}
	out := &Result{Snapshot: snap, Accessible: st.Accessible, Denied: st.Denied}
	if warn != "" {
		out.Warnings = append(out.Warnings, warn)
	}
	if w := LabelWarning(node); w != "" {
		out.Warnings = append(out.Warnings, w)
	}
	return out, nil
}
