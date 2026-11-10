// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/ingest"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/normalize"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/resultio"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/rulesetround"
	invscope "github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

const (
	cmdPolicyA = "action,runtime,lib,app_key,note\nexclude,*,*,/usr/sbin/sshd*,os patching\n"
	cmdPolicyB = "action,runtime,lib,app_key,note\nexclude,openssl,libcrypto*,*,not managed\n"
)

// scopeWorld: 선언 · 관측 결과(web 노드, libssl과 libcrypto) · 정책 A로 연 세션 · 활성 정책 A · 02에서 확정한 정책 B(그
// 출력을 적은 scope 영수증과 함께)가 있는 폴더.
type scopeWorld struct {
	dir, sess, decl, results, active, candidate, scopeSess string
}

func newScopeWorld(t *testing.T) scopeWorld {
	t.Helper()
	dir, _, declPath, results := oldRound(t)
	w := scopeWorld{dir: dir, sess: filepath.Join(dir, "session.json"), decl: declPath, results: results,
		active: filepath.Join(dir, "scope-assets.csv"), candidate: filepath.Join(dir, "asset-scope.csv"),
		scopeSess: filepath.Join(dir, "scope-session.json")}
	cbom := `{"bomFormat":"CycloneDX","specVersion":"1.6","components":[
      {"type":"cryptographic-asset","name":"libssl","properties":[{"name":"pqcota:crypto_runtime","value":"openssl"},
        {"name":"pqcota:openssl.version","value":"3.0.2"},{"name":"pqcota:openssl.fork","value":"OpenSSL"}]},
      {"type":"cryptographic-asset","name":"libcrypto","properties":[{"name":"pqcota:crypto_runtime","value":"openssl"},
        {"name":"pqcota:openssl.version","value":"3.0.2"},{"name":"pqcota:openssl.fork","value":"OpenSSL"}]}]}`
	res := &discoveryv1.CollectionResult{Envelope: &commonv1.Envelope{TargetNodeId: "web", CollectorId: "openssl-collector",
		CollectedAt: timestamppb.New(time.Unix(1700000100, 0).UTC()), DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION},
		CbomCyclonedx: []byte(cbom), CyclonedxSpecVersion: "1.6",
		Completeness: &commonv1.Completeness{LayersCovered: []commonv1.CollectionLayer{commonv1.CollectionLayer_COLLECTION_LAYER_PROCESS}}}
	b, _ := protojson.Marshal(res)
	for p, c := range map[string][]byte{filepath.Join(results, "r0.json"): b, w.active: []byte(cmdPolicyA), w.candidate: []byte(cmdPolicyB)} {
		if err := os.WriteFile(p, c, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sf, _, err := sessionFromResults(declPath, "local", results, w.active)
	if err != nil {
		t.Fatal(err)
	}
	if err := review.Save(w.sess, sf); err != nil {
		t.Fatal(err)
	}
	if err := invscope.SaveSession(w.scopeSess, invscope.Session{SessionID: "scope-1"}); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(store.DefaultDir(w.sess), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	cb := []byte(cmdPolicyB)
	if _, err := st.FinalizeChecked(store.Identity{Kind: "scope", Org: "local", SessionID: "scope-1"}, []string{w.candidate}, store.Check{},
		store.Sum(cb), time.Second, func() (store.Parts, error) {
			return store.Parts{PayloadSHA256: store.Sum(cb), Replace: []store.Change{{Path: w.candidate, Content: cb}}}, nil
		}); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w scopeWorld) activate(verify, confirm bool) error {
	scopeSessionPath = w.scopeSess
	return activateScope(w.sess, w.decl, w.results, w.active, w.candidate, "local", "", "local", verify, confirm)
}

func roundReceipt(t *testing.T, w scopeWorld, kind, oldID string) *store.Receipt {
	t.Helper()
	st, err := store.Open(store.DefaultDir(w.sess), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rc, err := st.RoundReceiptOf(kind, store.Identity{Kind: "review", Org: "local", SessionID: oldID})
	if err != nil {
		t.Fatal(err)
	}
	return rc
}

// upstreamWith: 결과를 정책 policy로 적재한 메모리 상류 이력을 openUpstream에 건다.
func upstreamWith(t *testing.T, results, policyPath string) {
	t.Helper()
	t.Setenv("PQCOTA_DSN", "memory://test")
	policy, err := review.LoadAssetPolicy(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, flaws := resultio.LoadDir(results)
	if len(flaws) != 0 {
		t.Fatal(flaws)
	}
	mem := history.NewMemStore()
	if _, err := ingest.IngestWith(loaded, ingest.IngestOptions{SnapshotPrefix: "t", RulesetVersion: normalize.RulesetVersion,
		Store: mem, AssetPolicy: policy}); err != nil {
		t.Fatal(err)
	}
	prev := openUpstream
	openUpstream = func(string, string) (upstream.Lookup, func(), error) { return mem, func() {}, nil }
	t.Cleanup(func() { openUpstream = prev })
}

// IC-D33 — **activate-scope는 화면과 같은 활성화를 명령으로 한다.** 뒷받침은 -verify-upstream과 -confirm-ingest 가운데 꼭
// 하나다(없거나 둘이면 거절). 진술로 활성화하면 정책이 바뀌고 새 세션이 새 지문을 들며 영수증의 basis가 operator-statement다.
// 02 확정이 쓴 그대로가 아닌 후보는 거절하고 아무것도 쓰지 않는다. 두 정책이 같으면 아무것도 쓰지 않는다.
func TestActivateScopeCommand(t *testing.T) {
	w := newScopeWorld(t)
	old, _ := review.Load(w.sess)
	for name, flags := range map[string][2]bool{"neither": {false, false}, "both": {true, true}} {
		if err := w.activate(flags[0], flags[1]); err == nil {
			t.Errorf("%s basis: accepted", name)
		}
	}
	if err := os.WriteFile(w.candidate, []byte(cmdPolicyB+"exclude,*,*,/opt/*,hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.activate(false, true); !errors.Is(err, rulesetround.ErrNoCandidate) {
		t.Errorf("hand-edited candidate: err = %v, want ErrNoCandidate", err)
	}
	if read(t, w.active) != cmdPolicyA {
		t.Fatal("a refused activation touched the active policy")
	}
	if err := os.WriteFile(w.candidate, []byte(cmdPolicyB), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.activate(false, true); err != nil {
		t.Fatal(err)
	}
	if read(t, w.active) != cmdPolicyB {
		t.Error("the active policy was not replaced")
	}
	next, _ := review.Load(w.sess)
	to, _ := invscope.PolicyFileDigest(w.candidate)
	if next.SessionID == old.SessionID || next.ScopePolicyDigestV1 != to {
		t.Errorf("new session: id %s digest %s", next.SessionID, next.ScopePolicyDigestV1)
	}
	if rc := roundReceipt(t, w, rulesetround.ScopeActivationReceipt, old.SessionID); rc == nil || rc.Round == nil || rc.Round.Basis != rulesetround.BasisOperator {
		t.Errorf("receipt basis: %+v", rc)
	}
	before := read(t, w.sess)
	if err := w.activate(false, true); err != nil || read(t, w.sess) != before {
		t.Errorf("activating the same policy again: err %v, session changed %v", err, read(t, w.sess) != before)
	}
}

// IC-D34 — **-verify-upstream은 새 정책의 스냅샷이 상류 이력에 모두 있어야 활성화한다.** 상류를 새 정책으로 적재했으면
// 활성화하고 영수증의 basis가 upstream-verified다. 상류가 옛 정책으로만 적재돼 있으면 거절하고 아무것도 쓰지 않는다.
// PQCOTA_DSN이 없으면 거절한다.
func TestActivateScopeVerifiesUpstream(t *testing.T) {
	w := newScopeWorld(t)
	old, _ := review.Load(w.sess)
	t.Setenv("PQCOTA_DSN", "")
	if err := w.activate(true, false); err == nil {
		t.Error("verify without PQCOTA_DSN: accepted")
	}
	upstreamWith(t, w.results, w.active)
	if err := w.activate(true, false); !errors.Is(err, upstream.ErrMissing) {
		t.Errorf("upstream ingested under the old policy: err = %v, want ErrMissing", err)
	}
	if read(t, w.active) != cmdPolicyA {
		t.Fatal("a refused activation touched the active policy")
	}
	upstreamWith(t, w.results, w.candidate)
	if err := w.activate(true, false); err != nil {
		t.Fatal(err)
	}
	if rc := roundReceipt(t, w, rulesetround.ScopeActivationReceipt, old.SessionID); rc == nil || rc.Round == nil || rc.Round.Basis != rulesetround.BasisUpstream {
		t.Errorf("receipt basis: %+v", rc)
	}
}

// IC-D35 — **baseline은 정책 기록이 없는 옛 세션에서만, 뒷받침과 함께 기준선 회차를 연다.** 진술로 열면 새 세션이 지금 정책의
// 지문을 들고 scope-baseline 영수증의 basis가 operator-statement다. 지문이 이미 있는 세션은 거절한다. -verify-upstream으로
// 상류에 없으면 거절하고 아무것도 쓰지 않는다.
func TestBaselineCommand(t *testing.T) {
	w := newScopeWorld(t)
	if err := baselineRound(w.sess, w.decl, w.results, w.active, "local", "", "local", false, true); !errors.Is(err, rulesetround.ErrNotBaseline) {
		t.Errorf("a session with a digest: err = %v, want ErrNotBaseline", err)
	}
	sf, _ := review.Load(w.sess)
	sf.ScopePolicyDigestV1 = ""
	if err := review.Save(w.sess, sf); err != nil {
		t.Fatal(err)
	}
	legacy := read(t, w.sess)
	upstreamWith(t, w.results, w.candidate)
	if err := baselineRound(w.sess, w.decl, w.results, w.active, "local", "", "local", true, false); !errors.Is(err, upstream.ErrMissing) {
		t.Errorf("upstream under another policy: err = %v, want ErrMissing", err)
	}
	if read(t, w.sess) != legacy {
		t.Fatal("a refused baseline rewrote the session")
	}
	if err := baselineRound(w.sess, w.decl, w.results, w.active, "local", "", "local", false, true); err != nil {
		t.Fatal(err)
	}
	next, _ := review.Load(w.sess)
	want, _ := invscope.PolicyFileDigest(w.active)
	if next.ScopePolicyDigestV1 != want || next.SessionID == sf.SessionID {
		t.Errorf("new session: id %s digest %s", next.SessionID, next.ScopePolicyDigestV1)
	}
	if rc := roundReceipt(t, w, rulesetround.ScopeBaselineReceipt, sf.SessionID); rc == nil || rc.Round == nil || rc.Round.Basis != rulesetround.BasisOperator {
		t.Errorf("receipt basis: %+v", rc)
	}
}
