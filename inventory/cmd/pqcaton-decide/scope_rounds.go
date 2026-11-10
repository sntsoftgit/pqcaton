// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 정책 회차의 명령 길. 화면과 같은 절차(rulesetround.Open)를 쓴다.
//
//   - baseline: 정책 기록이 없는 옛 세션에서, 지금 정책을 기준선으로 채택하는 회차를 연다.
//   - activate-scope: 02에서 확정한 정책을 활성화하며 회차를 연다.
//
// **상류 적재를 무엇으로 뒷받침하는지 하나를 고른다.** -verify-upstream은 새 정책으로 대조한 노드별 지문이 상류 이력에 모두
// 있는지 실제로 찾아보고(PQCOTA_DSN), 하나라도 없으면 아무것도 쓰지 않는다. -confirm-ingest는 운영자 진술이며 증명이 아니다.
// 어느 쪽이었는지는 회차 영수증의 basis에 남는다.

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/rulesetround"
	invscope "github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// openUpstream — 상류 이력. 시험이 메모리 이력으로 바꾼다.
var openUpstream = func(dsn, org string) (upstream.Lookup, func(), error) {
	st, err := history.NewPgStoreIn(context.Background(), dsn, org)
	if err != nil {
		return nil, nil, err
	}
	return st, st.Close, nil
}

// scopeBasis — 둘 가운데 하나만 골라야 한다.
func scopeBasis(verify, confirm bool) (string, error) {
	switch {
	case verify && confirm, !verify && !confirm:
		return "", errors.New("give exactly one of -verify-upstream (look the snapshots up in the upstream history, " +
			"needs PQCOTA_DSN) or -confirm-ingest (your statement that the upstream ingest used this policy; not a proof)")
	case verify:
		return rulesetround.BasisUpstream, nil
	}
	return rulesetround.BasisOperator, nil
}

// verifyUpstream — policyPath로 대조한 노드별 v1 지문이 상류 이력(PQCOTA_DSN, 조직 upstreamOrg)에 모두 있는가.
func verifyUpstream(declPath, resultsDir, policyPath, upstreamOrg string) error {
	dsn := os.Getenv("PQCOTA_DSN")
	if dsn == "" {
		return errors.New("-verify-upstream needs PQCOTA_DSN (the upstream history the ingest wrote to)")
	}
	d, err := decl.Load(declPath)
	if err != nil {
		return err
	}
	policy, err := review.LoadAssetPolicy(policyPath)
	if err != nil {
		return err
	}
	r, err := report.BuildWith(resultsDir, d, policy)
	if err != nil {
		return err
	}
	refs := upstream.Refs(r)
	l, closeFn, err := openUpstream(dsn, upstreamOrg)
	if err != nil {
		return err
	}
	defer closeFn()
	if _, err := upstream.Verify(l, refs); err != nil {
		return fmt.Errorf("%w — re-ingest upstream with this policy (pqcota-ingest -scope-assets) before going on; nothing was written", err)
	}
	fmt.Fprintf(os.Stderr, "upstream: all %d snapshots under this policy are in the upstream history\n", len(refs))
	return nil
}

// openScopeRound — 회차를 연다(대상 잠금 · 입력 지문 · 실행 잠금은 다른 명령과 같다).
func openScopeRound(req rulesetround.Request, declPath, resultsDir, scopeAssets, stateDir string) (rulesetround.Result, error) {
	if stateDir == "" {
		stateDir = store.DefaultDir(req.OldPath)
	}
	targets, err := rulesetround.Targets(req)
	if err != nil {
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
	chk, err := rulesetround.Snapshot(rulesetround.Inputs(req.OldPath, declPath, resultsDir, scopeAssets))
	if err != nil {
		return rulesetround.Result{}, err
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

func baselineRound(oldPath, declPath, resultsDir, scopeAssets, orgName, stateDir, upstreamOrg string, verify, confirm bool) error {
	if declPath == "" || resultsDir == "" {
		return errors.New("baseline needs -decl and -results: the new session is rebuilt from the results under the current policy")
	}
	basis, err := scopeBasis(verify, confirm)
	if err != nil {
		return err
	}
	digest, err := invscope.PolicyFileDigest(scopeAssets)
	if err != nil {
		return err
	}
	if verify {
		if err := verifyUpstream(declPath, resultsDir, scopeAssets, upstreamOrg); err != nil {
			return err
		}
	}
	req := rulesetround.Request{OldPath: oldPath, OutPath: oldPath, Org: orgName, NewID: review.NewSessionID(), Baseline: digest, Basis: basis,
		Rebuild: func() (review.Session, error) {
			sf, _, err := sessionFromResults(declPath, orgName, resultsDir, scopeAssets)
			return sf, err
		}}
	res, err := openScopeRound(req, declPath, resultsDir, scopeAssets, stateDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "baseline round %s under %s (%s)\n   archive: %s\n   report:  %s\n", res.NewSessionID, digest, basis, res.Archive, res.Report)
	fmt.Fprintln(os.Stderr, "   earlier conclusions are reference values only (\"previous\"); judge each item again before close")
	return nil
}

// dryRun — 쓰기 전에 지역 전제만 본다(상류는 보지 않고 아무것도 쓰지 않는다). 데모가 상류 적재 전에 부른다.
var dryRun bool

func activateScope(oldPath, declPath, resultsDir, scopeAssets, candidate, orgName, stateDir, upstreamOrg string, verify, confirm bool) error {
	if declPath == "" || resultsDir == "" || scopeAssets == "" || candidate == "" {
		return errors.New("activate-scope needs -decl, -results, -scope-assets (the active policy file, may not exist yet) and -candidate (asset-scope.csv finalized in 02)")
	}
	basis := rulesetround.BasisOperator
	if !dryRun {
		var err error
		if basis, err = scopeBasis(verify, confirm); err != nil {
			return err
		}
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
	if verify {
		if err := verifyUpstream(declPath, resultsDir, candidate, upstreamOrg); err != nil {
			return err
		}
	}
	req := rulesetround.Request{OldPath: oldPath, OutPath: oldPath, Org: orgName, NewID: review.NewSessionID(), Basis: basis,
		Activation: &rulesetround.Activation{PolicyPath: scopeAssets, CandidatePath: candidate, From: from, To: to},
		Rebuild: func() (review.Session, error) {
			sf, _, err := sessionFromResults(declPath, orgName, resultsDir, candidate)
			return sf, err
		}}
	res, err := openScopeRound(req, declPath, resultsDir, scopeAssets, stateDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "activated %s (was %s, %s)\n   new round %s\n   archive: %s\n   report:  %s\n", to, from, basis, res.NewSessionID, res.Archive, res.Report)
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
