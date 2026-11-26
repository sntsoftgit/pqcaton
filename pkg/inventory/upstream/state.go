// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package upstream

import (
	"fmt"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
)

// NodeState — 기대하는 스냅샷 하나에 대해 pqcota 이력이 지금 말하는 것. **둘을 따로 본다.**
//
//   - Body: 그 지문의 스냅샷 본체가 이력에 있는가(조직 · 노드 · 규칙 판 · 지문으로 찾는다).
//   - Current: 그 노드의 최신 참조가 그 지문인가. 최신은 관측 기록이 정한다(가장 최근 관측 기록이 가리키는 스냅샷).
//
// 본체가 있어도 최신 참조가 아닐 수 있다(과거에 같은 내용을 적재했고 그 뒤 다른 것이 적재됐다). 그래서 「이미 적재됨」은
// Current로만 말한다. 적재가 도중에 끝났을 때도 이 둘로 어느 노드까지 기록됐는지를 본다.
type NodeState struct {
	Node, Ruleset, Want string
	Body                bool
	// Latest — 그 노드의 최신 스냅샷의 지문. 스냅샷이 하나도 없으면 빈 값이다.
	Latest  string
	Current bool
}

// States — refs마다 이력의 지금 상태. 조회 전체가 Timeout 하나 안에 끝나야 하고, 실패하면 ErrUnreachable(분류만)이다.
// 실패했을 때 일부만 돌려주지 않는다: 확인하지 못한 것을 확인한 것과 섞지 않는다.
func States(l Lookup, refs []Ref) ([]NodeState, error) {
	var (
		out  []NodeState
		qErr error
	)
	if err := Within(Timeout, func() error {
		for _, ref := range refs {
			st := NodeState{Node: ref.Node, Ruleset: ref.Ruleset, Want: ref.Digest}
			snap, err := l.ByContentHashV1(ref.Node, ref.Ruleset, ref.Digest)
			if err != nil {
				qErr = err
				return nil
			}
			st.Body = snap != nil
			latest, err := l.Latest(ref.Node)
			if err != nil {
				qErr = err
				return nil
			}
			if latest != nil {
				st.Latest = history.ContentHashV1(latest)
			}
			st.Current = st.Body && st.Latest == ref.Digest
			out = append(out, st)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if qErr != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnreachable, Classify(qErr))
	}
	return out, nil
}

// AllCurrent — 모든 노드의 최신 참조가 기대하는 것인가. 비어 있으면 아니다.
func AllCurrent(states []NodeState) bool {
	for _, s := range states {
		if !s.Current {
			return false
		}
	}
	return len(states) > 0
}
