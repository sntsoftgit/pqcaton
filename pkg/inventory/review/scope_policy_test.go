// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review_test

import (
	"errors"
	"testing"

	kscope "github.com/randyinthedev-hash/pqcota-common/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	invscope "github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

// IC-R28 — **확정 전 정책 대조.** 세션의 지문과 지금 정책의 지문이 같을 때만 통과한다. 세션에 지문이 없으면
// ErrScopePolicyMissing이고(빈 정책으로 읽지 않는다), 다르면 ErrScopePolicy다. 정책 없음도 지문이 있어서, 정책으로 연
// 세션을 정책 없이 닫으면 걸리고 정책 없이 연 세션은 정책 없이 닫힌다.
func TestCheckScopePolicy(t *testing.T) {
	none := invscope.PolicyDigest(nil)
	some := invscope.PolicyDigest(&kscope.AssetPolicy{Rules: []kscope.AssetRule{{Exclude: true, AppKey: "/usr/sbin/sshd*"}}})
	for _, c := range []struct {
		name, session, active string
		want                  error
	}{
		{"no digest, no policy now", "", none, review.ErrScopePolicyMissing},
		{"no digest, a policy now", "", some, review.ErrScopePolicyMissing},
		{"opened with a policy, closed without", some, none, review.ErrScopePolicy},
		{"opened without, closed with a policy", none, some, review.ErrScopePolicy},
		{"same policy", some, some, nil},
		{"no policy both times", none, none, nil},
	} {
		err := review.CheckScopePolicy(review.Session{ScopePolicyDigestV1: c.session}, c.active)
		if (c.want == nil) != (err == nil) || (c.want != nil && !errors.Is(err, c.want)) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if !errors.Is(review.ErrScopePolicyMissing, review.ErrScopePolicy) {
		t.Error("a missing digest is not reported as a policy mismatch")
	}
}
