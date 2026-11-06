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

// IC-P40 — **정책 지문이 다르거나 저장된 세션에 없으면 Carry가 결론을 옮기지 않는다.** 같으면 옮긴다. note만 바뀐 정책은
// 의미 지문이 같으므로 옮긴다.
func TestCarryStopsAtAPolicyChange(t *testing.T) {
	a := invscope.PolicyDigest(&kscope.AssetPolicy{Rules: []kscope.AssetRule{{Exclude: true, AppKey: "/usr/sbin/sshd*", Note: "first"}}})
	aNote := invscope.PolicyDigest(&kscope.AssetPolicy{Rules: []kscope.AssetRule{{Exclude: true, AppKey: "/usr/sbin/sshd*", Note: "reworded"}}})
	b := invscope.PolicyDigest(&kscope.AssetPolicy{Rules: []kscope.AssetRule{{Exclude: true, AppKey: "/usr/bin/python*"}}})
	item := func(conclusion string) []review.Item {
		return []review.Item{{ID: "n1/openssl/libssl", Policy: "openssl/libssl", Conclusion: conclusion}}
	}
	for _, c := range []struct {
		name          string
		stored, now   string
		wantConcluded bool
	}{
		{"same policy", a, a, true},
		{"only the note changed", a, aNote, true},
		{"no digest stored", "", a, false},
		{"another policy", a, b, false},
	} {
		prev := review.Session{RulesetVersion: review.RulesetVersion, ScopePolicyDigestV1: c.stored, Items: item("교체한다")}
		next := review.Session{RulesetVersion: review.RulesetVersion, ScopePolicyDigestV1: c.now, Items: item("")}
		got := review.Carry(prev, next)
		carried := got.ScopePolicyDigestV1 == c.now && len(got.Items) == 1 && got.Items[0].Conclusion == "교체한다"
		if carried != c.wantConcluded {
			t.Errorf("%s: carried = %v, want %v (got digest %q)", c.name, carried, c.wantConcluded, got.ScopePolicyDigestV1)
		}
		if !c.wantConcluded && got.ScopePolicyDigestV1 != c.stored {
			t.Errorf("%s: a refused carry should hand back the stored session", c.name)
		}
	}
}
