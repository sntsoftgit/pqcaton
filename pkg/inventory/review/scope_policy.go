// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review

// 확정 전 정책 대조. 세션이 기록한 정책 지문(세울 때 실제로 읽은 정책)과 확정할 때 다시 읽은 정책의 지문이 같아야
// 확정한다. 같을 때만 그 지문이 계획과 형식 2 영수증의 근거가 된다.

import (
	"errors"
	"fmt"
)

// ErrScopePolicy: 세션의 정책 지문을 확정할 때의 정책과 대조하지 못했다. 아무것도 쓰지 않는다.
var ErrScopePolicy = errors.New("the asset-scope policy does not match the session")

// ErrScopePolicyMissing: 세션에 정책 지문이 없다(이 칸이 생기기 전의 파일). ErrScopePolicy를 감싼다.
var ErrScopePolicyMissing = fmt.Errorf("%w: the session records no asset-scope policy — open a new session with "+
	"pqcaton-decide open (give -scope-assets if the ingest was given one)", ErrScopePolicy)

// CheckScopePolicy — 세션 sf를 지금 정책(지문 active)으로 확정해도 되는가. 되면 nil이다.
//
// 세션에 지문이 없으면 ErrScopePolicyMissing이다. **빈 정책으로 읽지 않는다**: 그렇게 읽으면 정책 파일로 세운 옛
// 세션을 정책 없이 닫아도 통과한다. 지문이 다르면 ErrScopePolicy다. 정책 파일로 연 세션을 정책 없이 닫으면 여기서
// 걸린다(정책 없음도 지문이 있다).
func CheckScopePolicy(sf Session, active string) error {
	if sf.ScopePolicyDigestV1 == "" {
		return ErrScopePolicyMissing
	}
	if sf.ScopePolicyDigestV1 != active {
		return fmt.Errorf("%w: the session was opened under %s, the policy given now is %s — give the same "+
			"-scope-assets the session was opened with, or open a new session", ErrScopePolicy, sf.ScopePolicyDigestV1, active)
	}
	return nil
}
