// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
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

func (w scopeWorld) activate(confirm bool) error {
	scopeSessionPath = w.scopeSess
	return activateScope(w.sess, w.decl, w.results, w.active, w.candidate, "local", "", "local", confirm)
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

// memStore — 메모리 이력을 upstream.Store로. 시험의 openUpstream이 돌려준다.
type memStore struct{ upstream.Lookup }

func (memStore) Close() {}

// ingested: 결과를 정책 policy로 적재한 메모리 이력.
func ingested(t *testing.T, results, policyPath string) *history.MemStore {
	t.Helper()
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
	return mem
}

// useUpstream: openUpstream이 l을 돌려주게 건다. PQCOTA_DSN도 채운다.
func useUpstream(t *testing.T, l upstream.Lookup) {
	t.Helper()
	t.Setenv("PQCOTA_DSN", "memory://test")
	prev := openUpstream
	openUpstream = func(dsn, org string) (upstream.Store, string, error) {
		if dsn == "" {
			return nil, "", upstream.ErrNoDSN
		}
		return memStore{l}, org, nil
	}
	t.Cleanup(func() { openUpstream = prev })
}

// upstreamWith: 결과를 정책 policy로 적재한 메모리 pqcota 이력을 openUpstream에 건다.
func upstreamWith(t *testing.T, results, policyPath string) {
	t.Helper()
	useUpstream(t, ingested(t, results, policyPath))
}

// reportOf — 회차의 전환 보고서.
func reportOf(t *testing.T, w scopeWorld) review.RulesetChangeReport {
	t.Helper()
	sf, err := review.Load(w.sess)
	if err != nil || sf.ScopeChange == nil {
		t.Fatalf("no scope change: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(w.sess), sf.ScopeChange.Report))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := review.DecodeReport(b)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

// IC-D33 — **activate-scope는 pqcota 이력 대조를 언제나 한다.** -confirm-ingest를 주면 지원 종료 오류이고 아무것도 쓰지
// 않는다(운영자 진술로 우회하지 못한다). PQCOTA_DSN이 없으면 거절하고 아무것도 쓰지 않는다. 02 확정이 쓴 그대로가 아닌 후보는
// pqcota가 그 정책으로 적재돼 있어도 거절한다. 대조가 통과하면 정책이 바뀌고 새 세션이 새 지문을 들며 처리 기록의 basis가
// upstream-verified, 보고서에 upstream_check가 실린다. 두 정책이 같으면 아무것도 쓰지 않는다.
func TestActivateScopeCommand(t *testing.T) {
	w := newScopeWorld(t)
	old, _ := review.Load(w.sess)
	before := read(t, w.sess)
	upstreamWith(t, w.results, w.candidate)
	if err := w.activate(true); !errors.Is(err, errConfirmIngest) {
		t.Errorf("-confirm-ingest: err = %v, want errConfirmIngest", err)
	}
	t.Setenv("PQCOTA_DSN", "")
	if err := w.activate(false); !errors.Is(err, upstream.ErrNoDSN) {
		t.Errorf("no PQCOTA_DSN: err = %v, want ErrNoDSN", err)
	}
	if read(t, w.active) != cmdPolicyA || read(t, w.sess) != before {
		t.Fatal("a refused activation touched the policy or the session")
	}
	hand := cmdPolicyB + "exclude,*,*,/opt/*,hand edit\n"
	if err := os.WriteFile(w.candidate, []byte(hand), 0o644); err != nil {
		t.Fatal(err)
	}
	upstreamWith(t, w.results, w.candidate)
	if err := w.activate(false); !errors.Is(err, rulesetround.ErrNoCandidate) {
		t.Errorf("hand-edited candidate: err = %v, want ErrNoCandidate", err)
	}
	if read(t, w.active) != cmdPolicyA {
		t.Fatal("a refused activation touched the active policy")
	}
	if err := os.WriteFile(w.candidate, []byte(cmdPolicyB), 0o644); err != nil {
		t.Fatal(err)
	}
	upstreamWith(t, w.results, w.candidate)
	if err := w.activate(false); err != nil {
		t.Fatal(err)
	}
	if read(t, w.active) != cmdPolicyB {
		t.Error("the active policy was not replaced")
	}
	next, _ := review.Load(w.sess)
	to, _ := invscope.PolicyFileDigest(w.candidate)
	if next.SessionID == old.SessionID || next.ScopePolicyDigestV1 != to || next.ScopeChange.Basis != rulesetround.BasisUpstream {
		t.Errorf("new session: id %s digest %s change %+v", next.SessionID, next.ScopePolicyDigestV1, next.ScopeChange)
	}
	if rc := roundReceipt(t, w, rulesetround.ScopeActivationReceipt, old.SessionID); rc == nil || rc.Round == nil || rc.Round.Basis != rulesetround.BasisUpstream {
		t.Errorf("receipt basis: %+v", rc)
	}
	if rep := reportOf(t, w); rep.UpstreamCheck == nil || rep.UpstreamCheck.Org != "local" || len(rep.UpstreamCheck.Snapshots) == 0 {
		t.Errorf("report upstream check: %+v", rep.UpstreamCheck)
	}
	again := read(t, w.sess)
	if err := w.activate(false); err != nil || read(t, w.sess) != again {
		t.Errorf("activating the same policy again: err %v, session changed %v", err, read(t, w.sess) != again)
	}
}

// IC-D34 — **pqcota가 새 정책으로 적재돼 있지 않으면 활성화하지 않는다.** 옛 정책으로만 적재돼 있으면 ErrMissing이고 아무것도
// 쓰지 않는다. pqcota에 닿지 못하면 ErrUnreachable이고 아무것도 쓰지 않는다.
func TestActivateScopeNeedsTheIngest(t *testing.T) {
	w := newScopeWorld(t)
	before := read(t, w.sess)
	upstreamWith(t, w.results, w.active)
	if err := w.activate(false); !errors.Is(err, upstream.ErrMissing) {
		t.Errorf("pqcota ingested under the old policy: err = %v, want ErrMissing", err)
	}
	useUpstream(t, downLookup{})
	if err := w.activate(false); !errors.Is(err, upstream.ErrUnreachable) {
		t.Errorf("pqcota down: err = %v, want ErrUnreachable", err)
	}
	if read(t, w.active) != cmdPolicyA || read(t, w.sess) != before {
		t.Fatal("a refused activation touched the policy or the session")
	}
}

// downLookup — 조회가 실패하는 pqcota 이력.
type downLookup struct{}

func (downLookup) ByContentHashV1(string, string, string) (*history.Snapshot, error) {
	return nil, errors.New("connection refused")
}
func (downLookup) Latest(string) (*history.Snapshot, error) {
	return nil, errors.New("connection refused")
}

// IC-D35 — **baseline도 같은 대조를 거친다.** -confirm-ingest는 지원 종료 오류다. 지문이 이미 있는 세션은 거절한다. pqcota가
// 다른 정책으로 적재돼 있으면 거절하고 아무것도 쓰지 않는다. 지금 정책으로 적재돼 있으면 새 세션이 지금 정책의 지문을 들고
// scope-baseline 처리 기록의 basis가 upstream-verified다.
func TestBaselineCommand(t *testing.T) {
	w := newScopeWorld(t)
	if err := baselineRound(w.sess, w.decl, w.results, w.active, "local", "", "local", true); !errors.Is(err, errConfirmIngest) {
		t.Errorf("-confirm-ingest: err = %v, want errConfirmIngest", err)
	}
	upstreamWith(t, w.results, w.active)
	if err := baselineRound(w.sess, w.decl, w.results, w.active, "local", "", "local", false); !errors.Is(err, rulesetround.ErrNotBaseline) {
		t.Errorf("a session with a digest: err = %v, want ErrNotBaseline", err)
	}
	sf, _ := review.Load(w.sess)
	sf.ScopePolicyDigestV1 = ""
	if err := review.Save(w.sess, sf); err != nil {
		t.Fatal(err)
	}
	legacy := read(t, w.sess)
	upstreamWith(t, w.results, w.candidate)
	if err := baselineRound(w.sess, w.decl, w.results, w.active, "local", "", "local", false); !errors.Is(err, upstream.ErrMissing) {
		t.Errorf("pqcota under another policy: err = %v, want ErrMissing", err)
	}
	if read(t, w.sess) != legacy {
		t.Fatal("a refused baseline rewrote the session")
	}
	upstreamWith(t, w.results, w.active)
	if err := baselineRound(w.sess, w.decl, w.results, w.active, "local", "", "local", false); err != nil {
		t.Fatal(err)
	}
	next, _ := review.Load(w.sess)
	want, _ := invscope.PolicyFileDigest(w.active)
	if next.ScopePolicyDigestV1 != want || next.SessionID == sf.SessionID {
		t.Errorf("new session: id %s digest %s", next.SessionID, next.ScopePolicyDigestV1)
	}
	if rc := roundReceipt(t, w, rulesetround.ScopeBaselineReceipt, sf.SessionID); rc == nil || rc.Round == nil || rc.Round.Basis != rulesetround.BasisUpstream {
		t.Errorf("receipt basis: %+v", rc)
	}
}

// changingLookup — 대조하는 동안 결과 폴더에 파일을 더한다(대조와 잠금 사이에 입력이 바뀌는 경우).
type changingLookup struct {
	upstream.Lookup
	results string
	once    sync.Once
}

func (c *changingLookup) Latest(node string) (*history.Snapshot, error) {
	c.once.Do(func() { _ = os.WriteFile(filepath.Join(c.results, "late.json"), []byte("{}"), 0o644) })
	return c.Lookup.Latest(node)
}

// IC-D37 — **입력 스냅샷은 대조보다 먼저 뜬다.** 대조가 통과해도 그 사이에 결과 폴더가 바뀌었으면 잠금 안에서 ErrStale로
// 거절하고 아무것도 쓰지 않는다. 대조하지 않은 입력으로 upstream-verified를 쓰지 않는다.
func TestActivateScopeSnapshotsBeforeReconciling(t *testing.T) {
	w := newScopeWorld(t)
	old, _ := review.Load(w.sess)
	before := read(t, w.sess)
	useUpstream(t, &changingLookup{Lookup: ingested(t, w.results, w.candidate), results: w.results})
	if err := w.activate(false); !errors.Is(err, store.ErrStale) {
		t.Errorf("results changed during the reconciliation: err = %v, want ErrStale", err)
	}
	if read(t, w.active) != cmdPolicyA || read(t, w.sess) != before {
		t.Fatal("a refused activation touched the policy or the session")
	}
	if rc := roundReceipt(t, w, rulesetround.ScopeActivationReceipt, old.SessionID); rc != nil {
		t.Error("a record was written for inputs that were not reconciled")
	}
}

// IC-D36 — **-dry-run은 지역 전제만 보고 아무것도 쓰지 않는다.** 준비가 됐으면 nil, 손으로 고친 후보는 ErrNoCandidate, 두
// 정책이 같으면 ErrSamePolicy, 세션이 지금 정책으로 판정되지 않았으면 정책 잠금이다. 어느 경우에도 정책 · 세션 · 상태 폴더의
// 영수증이 그대로다. scope-status는 활성 정책 · 세션 지문 · 잠금을 한 줄씩 적는다.
func TestActivateScopeDryRunAndStatus(t *testing.T) {
	w := newScopeWorld(t)
	dryRun = true
	defer func() { dryRun = false }()
	snap := func() string { return read(t, w.active) + read(t, w.sess) + read(t, w.candidate) }
	before := snap()
	if err := w.activate(false); err != nil {
		t.Fatalf("ready: %v", err)
	}
	if err := os.WriteFile(w.candidate, []byte(cmdPolicyB+"exclude,*,*,/opt/*,hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.activate(false); !errors.Is(err, rulesetround.ErrNoCandidate) {
		t.Errorf("hand-edited: err = %v", err)
	}
	if err := os.WriteFile(w.candidate, []byte(cmdPolicyA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.activate(false); !errors.Is(err, rulesetround.ErrSamePolicy) {
		t.Errorf("same policy: err = %v", err)
	}
	if err := os.WriteFile(w.candidate, []byte(cmdPolicyB), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.active, []byte(cmdPolicyA+"exclude,*,*,/srv/*,moved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sc *review.ScopeChanged
	if err := w.activate(false); !errors.As(err, &sc) {
		t.Errorf("session not under the active policy: err = %v", err)
	}
	if err := os.WriteFile(w.active, []byte(cmdPolicyA), 0o644); err != nil {
		t.Fatal(err)
	}
	if snap() != before {
		t.Error("a dry run changed a file")
	}
	old, _ := review.Load(w.sess)
	if rc := roundReceipt(t, w, rulesetround.ScopeActivationReceipt, old.SessionID); rc != nil {
		t.Error("a dry run wrote a receipt")
	}
	out, err := stdout(t, func() error { return scopeStatus(w.sess, w.active) })
	active, _ := invscope.PolicyFileDigest(w.active)
	if err != nil || out != "active "+active+"\nsession "+old.ScopePolicyDigestV1+"\nsession_id "+old.SessionID+"\nlock none\n" {
		t.Errorf("scope-status = %q, %v", out, err)
	}
}

// IC-D38 — **pqcota 이력의 조직은 판정 조직이다.** -upstream-org에 -org와 다른 조직을 주면 pqcota 이력을 열지 않고 ErrOrg로
// 멈춘다. 같은 값은 받는다. 판정 세션의 조직이 -org와 다르면 조회하지 않고 ErrOrg다. 어느 경우에도 정책 · 세션이 그대로다.
func TestUpstreamOrganizationIsTheJudgmentsOrganization(t *testing.T) {
	w := newScopeWorld(t)
	opened := 0
	mem := ingested(t, w.results, w.candidate)
	t.Setenv("PQCOTA_DSN", "memory://test")
	prev := openUpstream
	t.Cleanup(func() { openUpstream = prev })
	openUpstream = func(_, org string) (upstream.Store, string, error) { opened++; return memStore{mem}, org, nil }
	scopeSessionPath = w.scopeSess
	if err := activateScope(w.sess, w.decl, w.results, w.active, w.candidate, "local", "", "other-corp", false); !errors.Is(err, upstream.ErrOrg) || opened != 0 {
		t.Errorf("another -upstream-org: err = %v, opened %d times; want ErrOrg without opening", err, opened)
	}
	sf, _ := review.Load(w.sess)
	sf.Scope = "org://other-corp"
	if err := review.Save(w.sess, sf); err != nil {
		t.Fatal(err)
	}
	moved := read(t, w.sess)
	counted := &countLookup{Lookup: mem}
	openUpstream = func(_, org string) (upstream.Store, string, error) { opened++; return memStore{counted}, org, nil }
	if err := activateScope(w.sess, w.decl, w.results, w.active, w.candidate, "local", "", "local", false); !errors.Is(err, upstream.ErrOrg) || counted.calls != 0 {
		t.Errorf("a session of another organization: err = %v, looked up %d times; want ErrOrg without a lookup", err, counted.calls)
	}
	if read(t, w.active) != cmdPolicyA || read(t, w.sess) != moved {
		t.Fatal("a refused activation touched the policy or the session")
	}
}

// countLookup — 조회 횟수만 센다.
type countLookup struct {
	upstream.Lookup
	calls int
}

func (c *countLookup) ByContentHashV1(node, ruleset, digest string) (*history.Snapshot, error) {
	c.calls++
	return c.Lookup.ByContentHashV1(node, ruleset, digest)
}

func (c *countLookup) Latest(node string) (*history.Snapshot, error) {
	c.calls++
	return c.Lookup.Latest(node)
}
