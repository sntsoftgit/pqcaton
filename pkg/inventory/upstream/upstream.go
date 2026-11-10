// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// Package upstream — 상류 이력에 스냅샷이 실제로 있는지 본다.
//
// 정책 활성화 · 기준선 채택은 「상류 적재를 이 정책으로 마쳤다」를 전제로 한다. 화면의 체크는 운영자 진술일 뿐이라 증명하지
// 못한다. 명령은 여기서 증명한다: 이 리포가 그 정책으로 결과를 대조해 얻은 노드별 v1 지문(history.ContentHashV1)이 상류
// 이력에 **하나도 빠짐없이** 있어야 한다. 지문이 같다는 것은 상류가 같은 결과를 같은 규칙 · 같은 정책으로 적재했다는 뜻이다.
package upstream

import (
	"errors"
	"fmt"
	"sort"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
)

// Ref — 상류 이력에서 찾아야 할 스냅샷 하나. 계획의 근거가 상류에서 찾히는 키와 같다.
type Ref struct{ Node, Ruleset, Digest string }

// Lookup — 상류 이력. history.PgStore와 history.MemStore가 이것을 만족한다.
type Lookup interface {
	ByContentHashV1(node, ruleset, digest string) (*history.Snapshot, error)
}

// ErrNothingToVerify — 대조 결과에 근거가 하나도 없다. 증명할 것이 없으면 증명된 것이 아니다.
var ErrNothingToVerify = errors.New("upstream: the reconciliation carries no evidence to look up")

// ErrMissing — 상류 이력에 없는 스냅샷이 있다. 상류 적재가 이 정책으로 끝나지 않았다.
var ErrMissing = errors.New("upstream: snapshots are missing from the upstream history")

// Refs — 대조 결과의 근거가 가리키는 스냅샷들. 겹치지 않게 정렬해 돌려준다.
func Refs(r *report.Result) []Ref {
	seen := map[Ref]bool{}
	var out []Ref
	for _, a := range r.Assets {
		for _, s := range a.Sources {
			ref := Ref{Node: s.Snapshot.SourceNodeID, Ruleset: s.Snapshot.RulesetVersion, Digest: s.Snapshot.Digest}
			if ref.Digest == "" || seen[ref] {
				continue
			}
			seen[ref] = true
			out = append(out, ref)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Node != out[j].Node {
			return out[i].Node < out[j].Node
		}
		return out[i].Digest < out[j].Digest
	})
	return out
}

// Verify — refs가 모두 상류 이력에 있는가. 없으면 ErrNothingToVerify, 빠진 것이 있으면 그것들과 ErrMissing이다.
func Verify(l Lookup, refs []Ref) ([]Ref, error) {
	if len(refs) == 0 {
		return nil, ErrNothingToVerify
	}
	var missing []Ref
	for _, ref := range refs {
		snap, err := l.ByContentHashV1(ref.Node, ref.Ruleset, ref.Digest)
		if err != nil {
			return nil, err
		}
		if snap == nil {
			missing = append(missing, ref)
		}
	}
	if len(missing) > 0 {
		return missing, fmt.Errorf("%w: %d of %d (first: node %s, %s)", ErrMissing, len(missing), len(refs), missing[0].Node, missing[0].Digest)
	}
	return nil, nil
}
