// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package rulesetround_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	kscope "github.com/randyinthedev-hash/pqcota-common/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/rulesetround"
	invscope "github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

// policyDigest: 규칙이 있는 정책의 지문. 규칙 0개의 지문과 달라야 「정책이 다르다」를 만들 수 있다.
var policyDigest = invscope.PolicyDigest(&kscope.AssetPolicy{Rules: []kscope.AssetRule{{Exclude: true, AppKey: "/usr/sbin/sshd*"}}})

// legacyWorld: 정책 지문이 없는 옛 세션(이 판보다 먼저 만든 세션)과 상태 폴더. 지금 규칙 판이고, 항목 둘 가운데 하나는 새
// 정책에서 빠진다.
func legacyWorld(t *testing.T, fault func(point, path string) error) (string, *store.State, []byte) {
	t.Helper()
	path, st := world(t, fault)
	sf := oldSession()
	sf.RulesetVersion, sf.ScopePolicyDigestV1 = review.RulesetVersion, ""
	sf.Items = append(sf.Items, review.Item{ID: "n1/openssl/libcrypto", Node: "n1", Runtime: "openssl", Policy: "openssl/libssl",
		State: "UNDECLARED", Mandatory: true, Conclusion: "둔다"})
	if err := review.Save(path, sf); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, st, raw
}

func rebuiltUnder(digest string) func() (review.Session, error) {
	return func() (review.Session, error) {
		sf, err := rebuilt()
		sf.ScopePolicyDigestV1 = digest
		return sf, err
	}
}

func baselineRequest(path, digest string) rulesetround.Request {
	return rulesetround.Request{OldPath: path, OutPath: path, Org: "acme", NewID: newID, Rebuild: rebuiltUnder(digest), Baseline: digest,
		Upstream: upstreamCheck}
}

// upstreamCheck — 정책 회차가 거친 pqcota 이력 대조의 기록(시험용).
var upstreamCheck = &review.UpstreamCheck{CheckedAt: "2026-11-12T00:00:00Z", Org: "default",
	Snapshots: []review.UpstreamSnapshot{{Node: "n1", Ruleset: review.RulesetVersion, Digest: "d1"}}}

var oldID = store.Identity{Kind: rulesetround.Kind, Org: "acme", SessionID: "00000000-0000-4000-8000-000000000001"}

// IC-P41 — **기준선 회차는 지문 없는 옛 세션을 지금 정책으로 다시 세운 새 세션에 넘긴다.** 새 세션은 지금 정책의 지문과
// 기준선 기록을 들고, 남은 항목에는 옛 결론이 참고값(Previous)으로만 붙으며 현재 결론은 비어 있다. 새 정책에서 빠진 항목은
// 새 세션에 없고 보고서에만 「only-old」로 남는다. 옛 세션은 바이트 그대로 보관된다. 회차와 한 트랜잭션으로 scope-baseline
// 형식 2 영수증이 생기고, 조직 · 옛 세션 해시 · 지금 정책 지문 · 새 세션 ID를 잇는다. 같은 옛 세션에서 다시 열면 ErrStale이고
// 파일이 늘지 않는다.
func TestBaselineRoundAdoptsTheCurrentPolicy(t *testing.T) {
	path, st, raw := legacyWorld(t, nil)
	defer st.Close()
	res, err := rulesetround.Open(st, baselineRequest(path, policyDigest), store.Check{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	next, err := review.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if next.SessionID != newID || next.ScopePolicyDigestV1 != policyDigest || next.ScopeChange == nil ||
		next.ScopeChange.Reason != review.ScopeRoundBaseline || next.ScopeChange.FromPolicy != "" ||
		next.ScopeChange.Basis != rulesetround.BasisUpstream {
		t.Fatalf("new session: id %s digest %s change %+v", next.SessionID, next.ScopePolicyDigestV1, next.ScopeChange)
	}
	for _, it := range review.All(next) {
		if it.ID == "n1/openssl/libcrypto" {
			t.Error("an item the new policy no longer manages was carried into the new session")
		}
		if it.ID == "n1/openssl/libssl" && (it.Conclusion != "" || it.Previous == nil || it.Previous.Conclusion != "교체한다") {
			t.Errorf("carried item: conclusion %q previous %+v; want empty with the old one as previous", it.Conclusion, it.Previous)
		}
	}
	if b, err := os.ReadFile(res.Archive); err != nil || string(b) != string(raw) {
		t.Fatalf("archive is not the old bytes: %v", err)
	}
	rb, err := os.ReadFile(res.Report)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := review.DecodeReport(rb)
	if err != nil {
		t.Fatal(err)
	}
	dropped := false
	for _, c := range rep.Items {
		if c.ID == "n1/openssl/libcrypto" && c.Change == review.ChangeOnlyOld && c.PreviousConclusion == "둔다" {
			dropped = true
		}
	}
	if !dropped || rep.Reason != review.ScopeRoundBaseline || rep.ToPolicy != policyDigest {
		t.Errorf("report: dropped item listed %v, reason %q, to %q", dropped, rep.Reason, rep.ToPolicy)
	}
	rc, err := st.RoundReceiptOf(rulesetround.ScopeBaselineReceipt, oldID)
	if err != nil || rc == nil {
		t.Fatalf("no scope-baseline receipt: %v", err)
	}
	sum := sha256.Sum256(raw)
	if got, ok := rc.PolicyDigest(); rc.Format != 2 || !ok || got != policyDigest || rc.Org != "acme" || rc.Round == nil ||
		rc.Round.FromSessionSHA256 != hex.EncodeToString(sum[:]) || rc.Round.ToSessionID != newID {
		t.Errorf("receipt %+v does not link the organization, old session, policy and new session", rc)
	}
	before, _ := os.ReadDir(filepath.Dir(path))
	again := baselineRequest(path, policyDigest)
	again.NewID = "99999999-2222-4333-8444-555555555555"
	if _, err := rulesetround.Open(st, again, store.Check{}, time.Second); err == nil {
		t.Error("a second baseline round opened")
	}
	if after, _ := os.ReadDir(filepath.Dir(path)); len(after) != len(before) {
		t.Error("a refused second round made files")
	}
}

// IC-P42 — **기준선 회차의 전제.** 이미 지문이 있는 세션은 ErrNotBaseline이다. 운영자가 확인한 정책과 다시 세운 세션이
// 읽은 정책이 다르면(그 사이 파일이 바뀜) ErrStale이다. 둘 다 아무것도 쓰지 않는다.
func TestBaselineRoundPreconditions(t *testing.T) {
	path, st := world(t, nil)
	sf := oldSession()
	sf.RulesetVersion = review.RulesetVersion
	if err := review.Save(path, sf); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesetround.Open(st, baselineRequest(path, policyDigest), store.Check{}, time.Second); !errors.Is(err, rulesetround.ErrNotBaseline) {
		t.Errorf("a session with a digest: err = %v, want ErrNotBaseline", err)
	}
	st.Close()

	p2, st2, raw := legacyWorld(t, nil)
	defer st2.Close()
	req := baselineRequest(p2, policyDigest)
	req.Rebuild = rebuiltUnder(invscope.PolicyDigest(nil))
	if _, err := rulesetround.Open(st2, req, store.Check{}, time.Second); !errors.Is(err, store.ErrStale) {
		t.Errorf("policy changed during the round: err = %v, want ErrStale", err)
	}
	if b, _ := os.ReadFile(p2); string(b) != string(raw) {
		t.Error("the old session was rewritten")
	}
	if rc, _ := st2.RoundReceiptOf(rulesetround.ScopeBaselineReceipt, oldID); rc != nil {
		t.Error("a receipt was written for a refused round")
	}
}

// IC-P43 — **규칙 전환 회차는 정책을 운영자 확인 없이 채택하지 않는다.** 지문 없는 세션은 기준선 잠금(ScopeChanged,
// baseline)으로, 다시 세운 세션의 지문이 옛 세션과 다르면 승인되지 않은 변경(ScopeChanged, unactivated)으로 거절한다.
func TestRulesetRoundRefusesPolicyChanges(t *testing.T) {
	path, st := world(t, nil)
	defer st.Close()
	sf := oldSession()
	sf.ScopePolicyDigestV1 = ""
	if err := review.Save(path, sf); err != nil {
		t.Fatal(err)
	}
	var sc *review.ScopeChanged
	if _, err := rulesetround.Open(st, request(path), store.Check{}, time.Second); !errors.As(err, &sc) || sc.Lock != review.ScopeLockBaseline {
		t.Errorf("no digest: err = %v, want the baseline lock", err)
	}
	if err := review.Save(path, oldSession()); err != nil {
		t.Fatal(err)
	}
	req := request(path)
	req.Rebuild = rebuiltUnder(policyDigest)
	if _, err := rulesetround.Open(st, req, store.Check{}, time.Second); !errors.As(err, &sc) || sc.Lock != review.ScopeLockUnactivated {
		t.Errorf("rebuilt under another policy: err = %v, want the unactivated lock", err)
	}
}

// IC-W61 — **기준선 회차는 보관본 · 새 세션 · 보고서 · 표지 · scope-baseline 영수증이 한 트랜잭션이다.** 적용 중이나 끝난
// 자리에서 멈춘 뒤 다시 열어 복구하면 다섯이 다 있다. 영수증만 남거나 영수증만 빠지는 일이 없다.
func TestBaselineRoundIsOneTransaction(t *testing.T) {
	for _, point := range []string{store.FaultApply, store.FaultDone} {
		t.Run(point, func(t *testing.T) {
			fired := false
			path, st, raw := legacyWorld(t, func(pt, _ string) error {
				if pt == point && !fired {
					fired = true
					return errors.New("injected")
				}
				return nil
			})
			_, err := rulesetround.Open(st, baselineRequest(path, policyDigest), store.Check{}, time.Second)
			st.Close()
			if err == nil {
				t.Fatal("the injected stop did not stop the round")
			}
			st2 := openState(t, filepath.Dir(path), nil)
			defer st2.Close()
			next, err := review.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			rc, err := st2.RoundReceiptOf(rulesetround.ScopeBaselineReceipt, oldID)
			if err != nil {
				t.Fatal(err)
			}
			archives, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "session.scope-baseline-"+oldID.SessionID+".json"))
			reports, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "session.scope-baseline-report-*.json"))
			done := next.SessionID == newID && rc != nil && len(archives) == 1 && len(reports) == 1
			none := next.ScopePolicyDigestV1 == "" && rc == nil && len(reports) == 0
			if !done && !none {
				t.Fatalf("partly applied: session %s receipt %v archives %d reports %d", next.SessionID, rc != nil, len(archives), len(reports))
			}
			if none {
				if b, _ := os.ReadFile(path); string(b) != string(raw) {
					t.Error("nothing applied, but the old session changed")
				}
			}
		})
	}
}
