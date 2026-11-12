// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 정책 회차의 명령 길. 화면과 같은 절차(rulesetround.Open)와 **같은 대조 함수**(upstream.Reconcile)를 쓴다.
//
//   - baseline: 정책 기록이 없는 옛 세션에서, 지금 정책을 기준선으로 채택하는 회차를 연다.
//   - activate-scope: 02에서 확정한 정책을 활성화하며 회차를 연다.
//
// **pqcota 이력 대조는 언제나 한다.** pqcaton은 pqcota가 연결되어 동작하는 환경에서만 실행하며(PQCOTA_DSN), 운영자 진술은
// 대조를 대신하지 못한다. -verify-upstream은 호환을 위해 받지만 동작을 고르지 않는다. -confirm-ingest는 지원을 끝냈고 주면
// 아무것도 쓰지 않고 멈춘다.
//
// 순서: (1) 입력 스냅샷을 뜬다, (2) 그 입력과 정책으로 pqcota 이력을 대조한다(전체 10초), (3) 실패하면 쓰지 않는다, (4) 곧바로
// 실행 잠금을 잡는다, (5) 1의 스냅샷이 그대로인지 다시 잰다, (6) 그대로일 때만 한 트랜잭션으로 쓴다. 대조가 끝난 뒤부터 쓰기가
// 끝날 때까지의 pqcota 변경은 감지하지 못한다(upstream/reconcile.go).

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/rulesetround"
	invscope "github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// openUpstream — pqcota 이력(조회용). 시험이 메모리 이력으로 바꾼다.
var openUpstream = upstream.OpenPg

// errConfirmIngest — -confirm-ingest(운영자 진술)는 v0.28.0에서 지원을 끝냈다.
var errConfirmIngest = errors.New("-confirm-ingest is no longer supported: an operator statement does not replace the pqcota history " +
	"reconciliation, which always runs (set " + upstream.DSNEnv + "); nothing was written")

// reconcileUpstream — 결과 폴더를 policyPath로 대조한 노드별 지문이 pqcota 이력(PQCOTA_DSN, 조직 upstreamOrg)의 노드별 최신
// 스냅샷과 모두 같은가. 화면과 같은 upstream.Reconcile이다. 오류에는 연결 정보를 싣지 않는다.
func reconcileUpstream(sessPath, declPath, resultsDir, policyPath, upstreamOrg, org string) (*review.UpstreamCheck, error) {
	// 조회하기 전에 조직을 맞춘다: -upstream-org는 판정 조직과 같아야 하고, 판정 세션도 그 조직이어야 한다.
	uorg, err := upstream.OrgFor(upstreamOrg, org)
	if err != nil {
		return nil, fmt.Errorf("%w; nothing was written", err)
	}
	sf, err := review.Load(sessPath)
	if err != nil {
		return nil, err
	}
	d, err := decl.Load(declPath)
	if err != nil {
		return nil, err
	}
	l, orgName, err := openUpstream(os.Getenv(upstream.DSNEnv), uorg)
	if err != nil {
		return nil, fmt.Errorf("%w; nothing was written", err)
	}
	defer l.Close()
	if err := upstream.SameOrg(orgName, org, sf.Scope); err != nil {
		return nil, fmt.Errorf("%w; nothing was written", err)
	}
	res, err := upstream.Reconcile(l, orgName, d, resultsDir, policyPath, time.Now())
	if err != nil {
		if errors.Is(err, upstream.ErrMissing) {
			return nil, fmt.Errorf("%w; re-ingest pqcota with this policy (pqcota-ingest -scope-assets) before going on; nothing was written", err)
		}
		return nil, fmt.Errorf("%w; nothing was written", err)
	}
	fmt.Fprintf(os.Stderr, "pqcota history: all %d snapshots under this policy matched the latest for their node at %s\n",
		len(res.Refs), res.At.UTC().Format(time.RFC3339))
	return res.Record(), nil
}

// openScopeRound — 회차를 연다. 입력 스냅샷(1) → pqcota 이력 대조(2 · 3) → 실행 잠금(4) → 잠금 안의 재확인과 쓰기(5 · 6).
// policyPath는 대조에 쓰는 정책이다(기준선은 지금 정책, 활성화는 후보 정책).
func openScopeRound(req rulesetround.Request, declPath, resultsDir, scopeAssets, policyPath, upstreamOrg, stateDir string) (rulesetround.Result, error) {
	if stateDir == "" {
		stateDir = store.DefaultDir(req.OldPath)
	}
	targets, err := rulesetround.Targets(req)
	if err != nil {
		return rulesetround.Result{}, err
	}
	chk, err := rulesetround.Snapshot(rulesetround.Inputs(req.OldPath, declPath, resultsDir, scopeAssets))
	if err != nil {
		return rulesetround.Result{}, err
	}
	if req.Upstream, err = reconcileUpstream(req.OldPath, declPath, resultsDir, policyPath, upstreamOrg, req.Org); err != nil {
		return rulesetround.Result{}, err
	}
	st, err := store.OpenCommand(stateDir, targets, store.Options{})
	if err != nil {
		return rulesetround.Result{}, err
	}
	defer st.Close()
	if a := req.Activation; a != nil {
		if err := candidateFinalized(st, a.CandidatePath, req.Org); err != nil {
			return rulesetround.Result{}, err
		}
	}
	return rulesetround.Open(st, req, chk, store.CommandWait)
}

// scopeSessionPath — 후보 정책을 확정한 02 세션 파일. candidateFinalized가 쓴다.
var scopeSessionPath string

// candidateFinalized — 후보 정책이 02 확정이 쓴 그대로인가(그 출력을 적은 scope 영수증이 있다).
func candidateFinalized(st *store.State, candidate, org string) error {
	if scopeSessionPath == "" {
		return errors.New("activate-scope needs -scope-session (the 02 scope session that finalized the candidate policy)")
	}
	sf, err := invscope.LoadSession(scopeSessionPath)
	if err != nil {
		return err
	}
	if sf.SessionID == "" {
		return errors.New("the scope session records no session_id; finalize the policy again in 02")
	}
	if _, err := st.OutputReceipt(store.Identity{Kind: "scope", Org: org, SessionID: sf.SessionID}, candidate); err != nil {
		return fmt.Errorf("%w: %v — finalize the policy in 02; a hand-edited file is not activated", rulesetround.ErrNoCandidate, err)
	}
	return nil
}

func baselineRound(oldPath, declPath, resultsDir, scopeAssets, orgName, stateDir, upstreamOrg string, confirm bool) error {
	if confirm {
		return errConfirmIngest
	}
	if declPath == "" || resultsDir == "" {
		return errors.New("baseline needs -decl and -results: the new session is rebuilt from the results under the current policy")
	}
	digest, err := invscope.PolicyFileDigest(scopeAssets)
	if err != nil {
		return err
	}
	req := rulesetround.Request{OldPath: oldPath, OutPath: oldPath, Org: orgName, NewID: review.NewSessionID(), Baseline: digest,
		Rebuild: func() (review.Session, error) {
			sf, _, err := sessionFromResults(declPath, orgName, resultsDir, scopeAssets)
			return sf, err
		}}
	res, err := openScopeRound(req, declPath, resultsDir, scopeAssets, scopeAssets, upstreamOrg, stateDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "baseline round %s under %s (%s)\n   archive: %s\n   report:  %s\n", res.NewSessionID, digest, rulesetround.BasisUpstream, res.Archive, res.Report)
	fmt.Fprintln(os.Stderr, "   earlier conclusions are reference values only (\"previous\"); judge each item again before close")
	return nil
}

// dryRun — 쓰기 전에 지역 전제만 본다(상류는 보지 않고 아무것도 쓰지 않는다). 데모가 상류 적재 전에 부른다.
var dryRun bool

func activateScope(oldPath, declPath, resultsDir, scopeAssets, candidate, orgName, stateDir, upstreamOrg string, confirm bool) error {
	if confirm {
		return errConfirmIngest
	}
	if declPath == "" || resultsDir == "" || scopeAssets == "" || candidate == "" {
		return errors.New("activate-scope needs -decl, -results, -scope-assets (the active policy file, may not exist yet) and -candidate (asset-scope.csv finalized in 02)")
	}
	if _, err := os.Stat(candidate); err != nil {
		return fmt.Errorf("%w: %v", rulesetround.ErrNoCandidate, err)
	}
	to, err := invscope.PolicyFileDigest(candidate)
	if err != nil {
		return fmt.Errorf("%w: %v", rulesetround.ErrNoCandidate, err)
	}
	from, err := invscope.PolicyFileDigest(scopeAssets)
	if err != nil {
		return err
	}
	if from == to {
		fmt.Fprintln(os.Stderr, "the candidate policy is already the active policy; nothing to activate, nothing was written")
		if dryRun {
			return rulesetround.ErrSamePolicy
		}
		return nil
	}
	if dryRun {
		return activationPreflight(oldPath, candidate, orgName, stateDir, from)
	}
	req := rulesetround.Request{OldPath: oldPath, OutPath: oldPath, Org: orgName, NewID: review.NewSessionID(),
		Activation: &rulesetround.Activation{PolicyPath: scopeAssets, CandidatePath: candidate, From: from, To: to},
		Rebuild: func() (review.Session, error) {
			sf, _, err := sessionFromResults(declPath, orgName, resultsDir, candidate)
			return sf, err
		}}
	res, err := openScopeRound(req, declPath, resultsDir, scopeAssets, candidate, upstreamOrg, stateDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "activated %s (was %s, %s)\n   new round %s\n   archive: %s\n   report:  %s\n", to, from, rulesetround.BasisUpstream, res.NewSessionID, res.Archive, res.Report)
	fmt.Fprintln(os.Stderr, "   earlier conclusions are reference values only (\"previous\"); plans made under the old policy are not signed or generated")
	return nil
}

// activationPreflight — 활성화의 지역 전제: 판정 세션이 지금 활성 정책으로 판정됐고(잠금 없음), 후보가 02 확정이 쓴 그대로다.
// 상태 폴더를 읽기만 한다.
func activationPreflight(oldPath, candidate, orgName, stateDir, from string) error {
	sf, err := review.Load(oldPath)
	if err != nil {
		return err
	}
	if l := review.ScopeLock(sf, from); l != "" {
		return &review.ScopeChanged{Lock: l, Session: sf.ScopePolicyDigestV1, Active: from}
	}
	if stateDir == "" {
		stateDir = store.DefaultDir(oldPath)
	}
	st, err := store.OpenCommand(stateDir, []string{oldPath, candidate}, store.Options{})
	if err != nil {
		return err
	}
	defer st.Close()
	if err := candidateFinalized(st, candidate, orgName); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "dry run: ready to activate (nothing was written, upstream not looked at)")
	return nil
}

// scopeStatus — 활성 정책 · 판정 세션 · 정책 잠금을 적는다. 데모와 종단 검증이 읽는다(한 줄에 이름과 값).
func scopeStatus(sessPath, scopeAssets string) error {
	active, err := invscope.PolicyFileDigest(scopeAssets)
	if err != nil {
		return err
	}
	sf, err := review.Load(sessPath)
	if err != nil {
		return err
	}
	lock := review.ScopeLock(sf, active)
	if lock == "" {
		lock = "none"
	}
	fmt.Printf("active %s\nsession %s\nsession_id %s\nlock %s\n", active, sf.ScopePolicyDigestV1, sf.SessionID, lock)
	return nil
}
