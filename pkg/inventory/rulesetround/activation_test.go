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

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/rulesetround"
	invscope "github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

const (
	policyAText = "action,runtime,lib,app_key,note\nexclude,*,*,/usr/sbin/sshd*,os patching\n"
	policyBText = "action,runtime,lib,app_key,note\nexclude,*,*,/usr/sbin/sshd*,os patching\nexclude,*,*,/usr/bin/python*,runtime\n"
)

// activationWorld: 정책 A로 판정한 세션, 활성 정책 A(scope-assets.csv), 02에서 확정한 정책 B(asset-scope.csv). active가
// false면 활성 정책 파일이 없다(규칙 0개 정책으로 판정한 세션).
func activationWorld(t *testing.T, active bool, fault func(point, path string) error) (path string, st *store.State, req rulesetround.Request, raw []byte) {
	t.Helper()
	path, st = world(t, fault)
	dir := filepath.Dir(path)
	pol, cand := filepath.Join(dir, "scope-assets.csv"), filepath.Join(dir, "asset-scope.csv")
	if err := os.WriteFile(cand, []byte(policyBText), 0o644); err != nil {
		t.Fatal(err)
	}
	if active {
		if err := os.WriteFile(pol, []byte(policyAText), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	from, _ := invscope.PolicyFileDigest(pol)
	to, _ := invscope.PolicyFileDigest(cand)
	sf := oldSession()
	sf.RulesetVersion, sf.ScopePolicyDigestV1 = review.RulesetVersion, from
	sf.Items = append(sf.Items, review.Item{ID: "n1/openssl/libcrypto", Node: "n1", Runtime: "openssl", Policy: "openssl/libssl",
		State: "UNDECLARED", Mandatory: true, Conclusion: "둔다"})
	if err := review.Save(path, sf); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	req = rulesetround.Request{OldPath: path, OutPath: path, Org: "acme", NewID: newID, Rebuild: rebuiltUnder(to),
		Activation: &rulesetround.Activation{PolicyPath: pol, CandidatePath: cand, From: from, To: to}}
	return path, st, req, raw
}

// IC-P44 — **활성화 회차는 정책 교체와 새 회차를 한 번에 연다.** 활성 정책이 후보의 바이트로 바뀌고 옛 정책은 보관된다. 새
// 세션은 새 정책의 지문과 활성화 기록을 들고, 남은 항목은 옛 결론이 참고값으로만 붙으며, 새 정책에서 빠진 항목은 보고서에만
// 남는다. scope-activation 형식 2 영수증이 조직 · 옛 세션 해시 · 옛 지문 · 새 지문 · 새 세션 ID를 잇는다. 같은 요청을 다시
// 보내면 거절되고 파일이 늘지 않는다.
func TestActivationRoundReplacesThePolicy(t *testing.T) {
	path, st, req, raw := activationWorld(t, true, nil)
	defer st.Close()
	res, err := rulesetround.Open(st, req, store.Check{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	a := req.Activation
	if b, _ := os.ReadFile(a.PolicyPath); string(b) != policyBText {
		t.Error("the active policy was not replaced by the candidate's bytes")
	}
	archives, _ := filepath.Glob(filepath.Join(filepath.Dir(a.PolicyPath), "scope-assets.activation-*.csv"))
	if len(archives) != 1 {
		t.Fatalf("policy archives: %v", archives)
	}
	if b, _ := os.ReadFile(archives[0]); string(b) != policyAText {
		t.Error("the old policy was not archived byte for byte")
	}
	next, err := review.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if next.SessionID != newID || next.ScopePolicyDigestV1 != a.To || next.ScopeChange == nil ||
		next.ScopeChange.Reason != review.ScopeRoundActivation || next.ScopeChange.FromPolicy != a.From {
		t.Fatalf("new session: id %s digest %s change %+v", next.SessionID, next.ScopePolicyDigestV1, next.ScopeChange)
	}
	for _, it := range review.All(next) {
		if it.ID == "n1/openssl/libcrypto" {
			t.Error("an item the new policy no longer manages was carried")
		}
		if it.ID == "n1/openssl/libssl" && (it.Conclusion != "" || it.Previous == nil) {
			t.Errorf("carried item: conclusion %q previous %+v", it.Conclusion, it.Previous)
		}
	}
	if b, _ := os.ReadFile(res.Archive); string(b) != string(raw) {
		t.Error("the old session was not archived byte for byte")
	}
	rc, err := st.RoundReceiptOf(rulesetround.ScopeActivationReceipt, oldID)
	if err != nil || rc == nil {
		t.Fatalf("no scope-activation receipt: %v", err)
	}
	sum := sha256.Sum256(raw)
	if got, ok := rc.PolicyDigest(); rc.Format != 2 || !ok || got != a.To || rc.Round == nil || rc.Round.FromPolicy != a.From ||
		rc.Round.FromSessionSHA256 != hex.EncodeToString(sum[:]) || rc.Round.ToSessionID != newID || rc.Org != "acme" {
		t.Errorf("receipt %+v does not link all five", rc)
	}
	before, _ := os.ReadDir(filepath.Dir(path))
	if _, err := rulesetround.Open(st, req, store.Check{}, time.Second); err == nil {
		t.Error("the same activation ran twice")
	}
	if after, _ := os.ReadDir(filepath.Dir(path)); len(after) != len(before) {
		t.Error("a repeated activation made files")
	}
}

// IC-P45 — **활성화의 전제.** 두 정책이 의미상 같으면 ErrSamePolicy이고 아무것도 쓰지 않는다. 후보가 없거나 깨졌으면
// ErrNoCandidate이고 아무것도 쓰지 않는다. 활성 정책 파일이 없으면 규칙 0개 정책에서 활성화하고, 보관할 옛 정책은 없다.
func TestActivationPreconditions(t *testing.T) {
	t.Run("same policy", func(t *testing.T) {
		path, st, req, raw := activationWorld(t, true, nil)
		defer st.Close()
		req.Activation.To = req.Activation.From
		if _, err := rulesetround.Open(st, req, store.Check{}, time.Second); !errors.Is(err, rulesetround.ErrSamePolicy) {
			t.Errorf("err = %v, want ErrSamePolicy", err)
		}
		if b, _ := os.ReadFile(path); string(b) != string(raw) {
			t.Error("a no-op activation rewrote the session")
		}
	})
	for name, breakIt := range map[string]func(p string){
		"candidate missing": func(p string) { os.Remove(p) },
		"candidate broken":  func(p string) { os.WriteFile(p, []byte("action,runtime\ndrop,openssl\n"), 0o644) },
	} {
		t.Run(name, func(t *testing.T) {
			path, st, req, raw := activationWorld(t, true, nil)
			defer st.Close()
			breakIt(req.Activation.CandidatePath)
			if _, err := rulesetround.Open(st, req, store.Check{}, time.Second); !errors.Is(err, rulesetround.ErrNoCandidate) {
				t.Errorf("err = %v, want ErrNoCandidate", err)
			}
			if b, _ := os.ReadFile(path); string(b) != string(raw) {
				t.Error("a refused activation rewrote the session")
			}
			if b, _ := os.ReadFile(req.Activation.PolicyPath); string(b) != policyAText {
				t.Error("a refused activation touched the active policy")
			}
		})
	}
	t.Run("no active policy file", func(t *testing.T) {
		path, st, req, _ := activationWorld(t, false, nil)
		defer st.Close()
		if req.Activation.From != invscope.PolicyDigest(nil) {
			t.Fatal("precondition: a missing policy file is the no-rule policy")
		}
		if _, err := rulesetround.Open(st, req, store.Check{}, time.Second); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(req.Activation.PolicyPath); string(b) != policyBText {
			t.Error("the active policy file was not created from the candidate")
		}
		if a, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "scope-assets.activation-*.csv")); len(a) != 0 {
			t.Errorf("an archive was made of a policy file that did not exist: %v", a)
		}
	})
}

// IC-P46 — **활성화 영수증은 정확히 이을 때만 증거다.** 같은 조직 · 같은 옛 세션 · 같은 옛 세션 바이트 · 같은 두 지문이면
// 참이고, 다른 조직 · 다른 세션 · 다른 바이트 · 다른 지문이면 거짓이다. 영수증이 없으면 거짓이다.
func TestActivationEvidenceMustMatchExactly(t *testing.T) {
	_, st, req, raw := activationWorld(t, true, nil)
	defer st.Close()
	a := req.Activation
	if ok, _ := rulesetround.ActivationEvidence(st, oldID, raw, a.From, a.To); ok {
		t.Fatal("evidence before any activation")
	}
	if _, err := rulesetround.Open(st, req, store.Check{}, time.Second); err != nil {
		t.Fatal(err)
	}
	if ok, err := rulesetround.ActivationEvidence(st, oldID, raw, a.From, a.To); err != nil || !ok {
		t.Fatalf("the exact activation is not evidence: %v", err)
	}
	other := oldID
	other.Org = "someone-else"
	session := oldID
	session.SessionID = "00000000-0000-4000-8000-000000000002"
	for name, ok := range map[string]bool{
		"another organization": must(rulesetround.ActivationEvidence(st, other, raw, a.From, a.To)),
		"another session":      must(rulesetround.ActivationEvidence(st, session, raw, a.From, a.To)),
		"other session bytes":  must(rulesetround.ActivationEvidence(st, oldID, append([]byte(nil), append(raw, ' ')...), a.From, a.To)),
		// 규칙 0개의 지문은 A와도 B와도 다르다(policyDigest는 note만 다른 A와 같은 지문이라 쓰지 않는다).
		"another old policy": must(rulesetround.ActivationEvidence(st, oldID, raw, invscope.PolicyDigest(nil), a.To)),
		"another new policy": must(rulesetround.ActivationEvidence(st, oldID, raw, a.From, invscope.PolicyDigest(nil))),
	} {
		if ok {
			t.Errorf("%s: accepted as evidence", name)
		}
	}
}

func must(ok bool, _ error) bool { return ok }

// IC-W62 — **활성화는 정책 보관본 · 활성 정책 교체 · 옛 세션 보관본 · 새 세션 · 보고서 · 표지 · 영수증이 한 트랜잭션이다.**
// 적용 중이나 끝난 자리에서 멈춘 뒤 복구하면 다 있거나 다 없다. 정책 보관 자리에 파일이 이미 있으면 ErrExists이고 아무것도
// 쓰지 않는다.
func TestActivationIsOneTransaction(t *testing.T) {
	for _, point := range []string{store.FaultApply, store.FaultDone} {
		t.Run(point, func(t *testing.T) {
			fired := false
			path, st, req, raw := activationWorld(t, true, func(pt, _ string) error {
				if pt == point && !fired {
					fired = true
					return errors.New("injected")
				}
				return nil
			})
			_, err := rulesetround.Open(st, req, store.Check{}, time.Second)
			st.Close()
			if err == nil {
				t.Fatal("the injected stop did not stop the activation")
			}
			st2 := openState(t, filepath.Dir(path), nil)
			defer st2.Close()
			pol, _ := os.ReadFile(req.Activation.PolicyPath)
			sess, _ := os.ReadFile(path)
			rc, _ := st2.RoundReceiptOf(rulesetround.ScopeActivationReceipt, oldID)
			archives, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "scope-assets.activation-*.csv"))
			done := string(pol) == policyBText && string(sess) != string(raw) && rc != nil && len(archives) == 1
			none := string(pol) == policyAText && string(sess) == string(raw) && rc == nil && len(archives) == 0
			if !done && !none {
				t.Fatalf("partly applied: policy replaced %v session replaced %v receipt %v archives %d",
					string(pol) == policyBText, string(sess) != string(raw), rc != nil, len(archives))
			}
		})
	}
	t.Run("archive name taken", func(t *testing.T) {
		path, st, req, raw := activationWorld(t, true, nil)
		defer st.Close()
		_, _, pa, err := rulesetround.ActivationNames(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pa, []byte("someone else's\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := rulesetround.Open(st, req, store.Check{}, time.Second); !errors.Is(err, store.ErrExists) {
			t.Errorf("err = %v, want ErrExists", err)
		}
		pol, _ := os.ReadFile(req.Activation.PolicyPath)
		sess, _ := os.ReadFile(path)
		if string(pol) != policyAText || string(sess) != string(raw) {
			t.Error("a refused activation changed the policy or the session")
		}
		if rc, _ := st.RoundReceiptOf(rulesetround.ScopeActivationReceipt, oldID); rc != nil {
			t.Error("a receipt was written for a refused activation")
		}
	})
}
