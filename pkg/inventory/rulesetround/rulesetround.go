// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// Package rulesetround는 규칙 전환 회차를 연다. 화면(pqcaton-ui)과 명령(pqcaton-decide new-round)이 이 함수 하나를
// 쓴다. 판정 쪽 변환(review.RulesetRound)과 저장소의 한 트랜잭션(store.OpenRulesetRound)을 잇는다.
//
// 둘이 다른 것은 「결과에서 지금 규칙으로 세션을 다시 세우는 방법」뿐이라 그것만 부르는 쪽이 넘긴다. 같은 입력이면
// 화면이든 명령이든 같은 세션 · 같은 보고서 바이트가 나온다.
package rulesetround

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	invscope "github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

// Kind — 판정 세션의 영수증 종류. 화면과 명령이 같은 값을 쓴다.
const Kind = "review"

// ErrSameRuleset — 세션이 이미 지금 상류 규칙 판이다. 넘길 것이 없다.
var ErrSameRuleset = errors.New("the session is already under the current upstream ruleset")

// ErrNotBaseline — 기준선 회차를 열려 했는데 세션에 이미 정책 지문이 있다. 기준선은 지문 없는 옛 세션에서 한 번만 연다.
var ErrNotBaseline = errors.New("the session already records an asset-scope policy; a baseline round is only for sessions made before v0.26.0")

// ScopeBaselineReceipt — 기준선 회차의 영수증 종류.
const ScopeBaselineReceipt = review.ScopeRoundBaseline

// ScopeActivationReceipt — 활성화 회차의 영수증 종류.
const ScopeActivationReceipt = review.ScopeRoundActivation

// 정책 회차의 근거(처리 기록의 basis).
const (
	// BasisOperator — 운영자 진술. v0.27.x까지의 과거 기록으로만 읽는다. v0.28.0부터 새로 쓰지 않는다.
	BasisOperator = review.ScopeBasisOperator
	// BasisUpstream — pqcota 이력 대조를 거쳤다(upstream.Reconcile). 새 정책 회차의 근거는 이것뿐이다.
	BasisUpstream = review.ScopeBasisUpstream
)

// ErrNoUpstreamCheck — 정책 회차(기준선 · 활성화)에 pqcota 이력 대조 기록이 없다. 운영자 진술로 대신하지 않으므로 열지 않는다.
var ErrNoUpstreamCheck = errors.New("a scope round needs a pqcota history reconciliation; an operator statement does not replace it")

// ErrSamePolicy — 활성 정책과 후보 정책이 의미상 같다. 활성화할 것이 없다(아무것도 쓰지 않는다).
var ErrSamePolicy = errors.New("the candidate policy is the active policy; nothing to activate")

// ErrNoCandidate — 활성화할 후보 정책 파일이 없거나 읽을 수 없거나 형식이 틀렸다. 활성화하지 않는다.
var ErrNoCandidate = errors.New("the candidate policy is missing or cannot be read")

// Activation — 활성화 회차의 입력. PolicyPath는 활성 정책 파일(scope-assets.csv, 없어도 된다), CandidatePath는 02에서 확정한
// 정책(asset-scope.csv, 있어야 한다). From · To는 확인 페이지가 본 두 지문이다. 잠금 안에서 다시 재어 다르면 ErrStale이다.
type Activation struct {
	PolicyPath, CandidatePath string
	From, To                  string
}

// ErrOrgMismatch — 옛 세션 · 다시 세운 세션 · 요청한 조직이 같지 않다. 항목 ID가 같아도 다른 조직의 결론을 옮기지 않는다.
var ErrOrgMismatch = errors.New("the old session, the rebuilt session and the requested organization are not the same organization")

// Inputs — 회차가 읽는 입력. 판정 화면이 rev로 덮는 넷과 같다(세션 · 선언 · 결과 폴더 · 자산 스코프 정책). 빈 경로는 뺀다.
func Inputs(oldPath, declPath, resultsDir, scopeAssets string) []store.Input {
	in := []store.Input{{Name: "session", Path: oldPath}}
	if declPath != "" {
		in = append(in, store.Input{Name: "declaration", Path: declPath})
	}
	if resultsDir != "" {
		in = append(in, store.Input{Name: "results", Path: resultsDir, Dir: true})
	}
	if scopeAssets != "" {
		in = append(in, store.Input{Name: "scope-assets", Path: scopeAssets})
	}
	return in
}

// Snapshot — 입력의 지문을 지금 재어 Check로 돌려준다. 명령이 쓴다(화면은 폼에 실려 온 rev를 쓴다). 저장소가 다시 세우기
// 전과 쓰기 직전에 같은 입력을 다시 재므로, 그 사이에 하나라도 바뀌면 ErrStale이고 아무것도 쓰지 않는다.
func Snapshot(inputs []store.Input) (store.Check, error) {
	rev, err := store.Rev(inputs)
	if err != nil {
		return store.Check{}, err
	}
	return store.Check{Inputs: inputs, Rev: rev}, nil
}

// Request — 회차 하나.
type Request struct {
	// OldPath: 옛 세션 파일. OutPath: 새 세션을 쓸 자리. 둘이 같으면 옛 파일을 보관본으로 옮긴다(화면이 그렇다).
	// 다르면 OutPath는 없던 파일이어야 하고 옛 파일은 제자리에 남는다.
	OldPath, OutPath string
	// ReportPath: 보고서 자리. 비우면 새 세션 옆에 정해진 이름([Names])이다. 없던 파일이어야 한다.
	ReportPath string
	Org        string
	NewID      string
	// Rebuild: 지금 규칙으로 결과에서 세운 세션. 저장소의 잠금 안에서 부른다.
	Rebuild func() (review.Session, error)
	// Baseline: 비어 있지 않으면 **기준선 회차**다. 값은 pqcota 이력 대조를 거친 지금 정책의 지문이다.
	// 정책 지문이 없는 옛 세션에서만 열고, 다시 세운 세션의 지문이 이 값이어야 하며, scope-baseline 형식 2 영수증을 함께
	// 쓴다. 비어 있으면 규칙 전환 회차다.
	Baseline string
	// Activation: 있으면 **활성화 회차**다. 활성 정책을 후보 정책으로 바꾸고, 옛 정책을 보관하고, 옛 세션을 새 정책으로 다시
	// 세운 새 세션에 넘기며, scope-activation 형식 2 영수증을 쓴다. 모두 한 트랜잭션이다. Rebuild는 **후보 정책으로** 세워야 한다.
	Activation *Activation
	// Upstream: 정책 회차(Baseline · Activation)가 거친 pqcota 이력 대조의 기록. 정책 회차에는 반드시 있어야 하고(없으면
	// ErrNoUpstreamCheck), 처리 기록의 basis는 BasisUpstream, 전환 보고서의 upstream_check는 이 값이다. 규칙 회차는 비운다.
	Upstream *review.UpstreamCheck
}

// Result — 연 회차.
type Result struct {
	NewSessionID    string
	Archive, Report string // 쓴 자리(보관본은 OutPath가 OldPath와 다르면 빈다)
}

var safeKey = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// Names — 보관본과 보고서의 자리. **파일 이름에 규칙 판 문자열을 넣지 않는다**(`pqcota-enrich/v2+…`의 `/`가 경로를
// 가른다). 규칙 판은 해시 앞 12자의 별칭으로, 세션은 그 ID(쓸 수 없는 글자가 있거나 없으면 내용 해시)로 가른다.
// 정확한 규칙 판은 보고서와 회차 표지에 적힌다.
func Names(oldPath, outPath string, old review.Session, oldBytes []byte) (archive, report string) {
	key := old.SessionID
	if !safeKey.MatchString(key) {
		key = strings.TrimPrefix(internalID(oldPath, "", oldBytes), "unnamed-")[:12]
	}
	rh := sha256.Sum256([]byte(old.RulesetVersion))
	alias := hex.EncodeToString(rh[:6])
	base := strings.TrimSuffix(filepath.Base(oldPath), filepath.Ext(oldPath))
	archive = filepath.Join(filepath.Dir(oldPath), base+".ruleset-"+alias+"-"+key+".json")
	outBase := strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath))
	report = filepath.Join(filepath.Dir(outPath), outBase+".ruleset-change-"+key+".json")
	return archive, report
}

// activationNames — 활성화 회차의 보관본 · 보고서 · 정책 보관본 자리. 정책 보관본은 옛 정책 지문과 옛 세션으로 가른다(같은
// 이름이 있으면 ErrExists로 아무것도 쓰지 않는다).
func activationNames(oldPath, outPath, policyPath, from string, old review.Session, oldBytes []byte) (archive, report, policyArchive string) {
	key := old.SessionID
	if !safeKey.MatchString(key) {
		key = strings.TrimPrefix(internalID(oldPath, "", oldBytes), "unnamed-")[:12]
	}
	base := strings.TrimSuffix(filepath.Base(oldPath), filepath.Ext(oldPath))
	outBase := strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath))
	pol := strings.TrimPrefix(from, "scope-policy-v1:")
	if len(pol) > 12 {
		pol = pol[:12]
	}
	pbase := strings.TrimSuffix(filepath.Base(policyPath), filepath.Ext(policyPath))
	return filepath.Join(filepath.Dir(oldPath), base+".scope-activation-"+key+".json"),
		filepath.Join(filepath.Dir(outPath), outBase+".scope-activation-report-"+key+".json"),
		filepath.Join(filepath.Dir(policyPath), pbase+".activation-"+pol+"-"+key+filepath.Ext(policyPath))
}

// ActivationNames — 활성화 회차가 쓸 자리(세션 보관본 · 보고서 · 정책 보관본). 확인 페이지가 보인다.
func ActivationNames(req Request) (archive, report, policyArchive string, err error) {
	if req.Activation == nil {
		return "", "", "", errors.New("not an activation request")
	}
	old, raw, err := load(req.OldPath)
	if err != nil {
		return "", "", "", err
	}
	archive, report, policyArchive = activationNames(req.OldPath, req.OutPath, req.Activation.PolicyPath, req.Activation.From, old, raw)
	return archive, report, policyArchive, nil
}

// ActivationEvidence — 옛 세션(동일성 old, 파일 바이트 oldBytes)이 정책 from에서 to로 정식 활성화됐다는 증거가 있는가.
// scope-activation 영수증이 **이 조직 · 이 옛 세션 · 옛 세션의 바이트 · 옛 지문 · 새 지문**을 모두 정확히 이을 때만 참이다.
// 다른 조직이나 세션의 영수증, 다른 지문을 잇는 영수증은 증거가 아니다.
func ActivationEvidence(st *store.State, old store.Identity, oldBytes []byte, from, to string) (bool, error) {
	rc, err := st.RoundReceiptOf(ScopeActivationReceipt, old)
	if err != nil || rc == nil {
		return false, err
	}
	sum := sha256.Sum256(oldBytes)
	got, ok := rc.PolicyDigest()
	return ok && rc.Org == old.Org && rc.SessionID == old.SessionID && rc.Round != nil &&
		rc.Round.FromSessionSHA256 == hex.EncodeToString(sum[:]) && rc.Round.FromPolicy == from && got == to && from != "", nil
}

// baselineNames — 기준선 회차의 보관본과 보고서 자리. 규칙 전환 회차와 이름이 겹치지 않는다.
func baselineNames(oldPath, outPath string, old review.Session, oldBytes []byte) (archive, report string) {
	key := old.SessionID
	if !safeKey.MatchString(key) {
		key = strings.TrimPrefix(internalID(oldPath, "", oldBytes), "unnamed-")[:12]
	}
	base := strings.TrimSuffix(filepath.Base(oldPath), filepath.Ext(oldPath))
	outBase := strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath))
	return filepath.Join(filepath.Dir(oldPath), base+".scope-baseline-"+key+".json"),
		filepath.Join(filepath.Dir(outPath), outBase+".scope-baseline-report-"+key+".json")
}

// Targets — 잠글 파일들(새 세션 · 보고서 · 보관본). 옛 세션을 읽어 이름을 정한다.
func Targets(req Request) ([]string, error) {
	old, raw, err := load(req.OldPath)
	if err != nil {
		return nil, err
	}
	archive, report := req.names(old, raw)
	// 옛 세션은 쓰지 않을 때도(다른 자리에 새 세션을 쓸 때) 잠그고 남은 준비 파일을 본다.
	t := []string{req.OldPath, req.OutPath, report}
	if same(req.OldPath, req.OutPath) {
		t = append(t[1:], archive)
	}
	if a := req.Activation; a != nil {
		_, _, pa := activationNames(req.OldPath, req.OutPath, a.PolicyPath, a.From, old, raw)
		t = append(t, a.PolicyPath, pa)
	}
	return t, nil
}

// Open — 규칙 전환 회차를 연다. 옛 세션이 이미 지금 상류 규칙 판이면 ErrSameRuleset이다. 저장소의 오류(ErrStale:
// 이미 연 회차, ErrExists: 보관 자리가 있음 등)는 그대로 돌려준다.
func Open(st *store.State, req Request, chk store.Check, wait time.Duration) (Result, error) {
	if (req.Activation != nil || req.Baseline != "") && req.Upstream == nil {
		return Result{}, ErrNoUpstreamCheck
	}
	old, raw, err := load(req.OldPath)
	if err != nil {
		return Result{}, err
	}
	if a := req.Activation; a != nil {
		switch {
		case a.From == a.To:
			return Result{}, ErrSamePolicy
		case old.ScopePolicyDigestV1 == "":
			return Result{}, &review.ScopeChanged{Lock: review.ScopeLockBaseline}
		case old.ScopePolicyDigestV1 != a.From:
			return Result{}, &review.ScopeChanged{Lock: review.ScopeLockUnactivated, Session: old.ScopePolicyDigestV1, Active: a.From}
		}
	}
	switch {
	case req.Activation != nil:
	case req.Baseline != "" && old.ScopePolicyDigestV1 != "":
		return Result{}, ErrNotBaseline
	case req.Baseline == "" && old.ScopePolicyDigestV1 == "":
		// 정책 기록이 없는 세션을 규칙 전환으로 넘기면 지금 정책을 운영자 확인 없이 채택하는 셈이다. 기준선 회차가 규칙도 함께 넘긴다.
		return Result{}, &review.ScopeChanged{Lock: review.ScopeLockBaseline}
	case req.Baseline == "" && !changed(old):
		return Result{}, ErrSameRuleset
	}
	if err := sameOrg(old.Scope, req.Org, "", false); err != nil {
		return Result{}, err
	}
	archive, report := req.names(old, raw)
	targets, err := Targets(req)
	if err != nil {
		return Result{}, err
	}
	res := Result{NewSessionID: req.NewID, Report: report}
	archived := same(req.OldPath, req.OutPath)
	if archived {
		res.Archive = archive
	}
	from := store.Identity{Kind: Kind, Org: req.Org, SessionID: internalID(req.OldPath, old.SessionID, raw)}
	to := store.Identity{Kind: Kind, Org: req.Org, SessionID: req.NewID}
	err = st.OpenRulesetRound(from, to, targets, chk, wait, func() (store.RulesetRound, error) {
		// 잠금 안에서 다시 읽는다. 그 사이에 바뀌었으면 처음 정한 이름과 어긋나므로 멈춘다.
		cur, curRaw, err := load(req.OldPath)
		if err != nil {
			return store.RulesetRound{}, err
		}
		if a, r := req.names(cur, curRaw); a != archive || r != report || cur.ScopePolicyDigestV1 != old.ScopePolicyDigestV1 ||
			(req.Baseline == "" && req.Activation == nil && !changed(cur)) {
			return store.RulesetRound{}, fmt.Errorf("%w: %s changed while the round was being opened", store.ErrStale, req.OldPath)
		}
		rebuilt, err := req.Rebuild()
		if err != nil {
			return store.RulesetRound{}, err
		}
		if err := sameOrg(cur.Scope, req.Org, rebuilt.Scope, true); err != nil {
			return store.RulesetRound{}, err
		}
		archiveName := ""
		if archived {
			archiveName = filepath.Base(archive)
		}
		var (
			next review.Session
			rep  review.RulesetChangeReport
		)
		var act *activationFiles
		if req.Activation != nil {
			// 잠금 안에서 두 정책을 다시 읽어 확인 페이지가 본 지문과 견준다. 후보가 없거나 깨졌으면 활성화하지 않는다.
			af, err := readActivation(*req.Activation)
			if err != nil {
				return store.RulesetRound{}, err
			}
			if rebuilt.ScopePolicyDigestV1 != req.Activation.To {
				return store.RulesetRound{}, fmt.Errorf("%w: the rebuilt session is not under the candidate policy", store.ErrStale)
			}
			act = &af
			next, rep = review.PolicyRound(cur, rebuilt, req.NewID, filepath.Base(report), archiveName, review.ScopeRoundActivation, req.basis())
			rep.UpstreamCheck = req.Upstream
		} else if req.Baseline != "" {
			// 운영자가 확인한 정책과 다시 세운 세션이 읽은 정책이 같아야 한다. 그 사이 파일이 바뀌었으면 쓰지 않는다.
			if rebuilt.ScopePolicyDigestV1 != req.Baseline {
				return store.RulesetRound{}, fmt.Errorf("%w: the asset-scope policy changed while the baseline round was being opened", store.ErrStale)
			}
			next, rep = review.PolicyRound(cur, rebuilt, req.NewID, filepath.Base(report), archiveName, review.ScopeRoundBaseline, req.basis())
			rep.UpstreamCheck = req.Upstream
		} else {
			if rebuilt.ScopePolicyDigestV1 != cur.ScopePolicyDigestV1 {
				return store.RulesetRound{}, &review.ScopeChanged{Lock: review.ScopeLockUnactivated,
					Session: cur.ScopePolicyDigestV1, Active: rebuilt.ScopePolicyDigestV1}
			}
			next, rep = review.RulesetRound(cur, rebuilt, req.NewID, filepath.Base(report), archiveName)
		}
		sb, err := review.Encode(next)
		if err != nil {
			return store.RulesetRound{}, err
		}
		rb, err := review.EncodeReport(rep)
		if err != nil {
			return store.RulesetRound{}, err
		}
		out := store.RulesetRound{FromPath: req.OldPath, Session: store.Change{Path: req.OutPath, Content: sb},
			Report: store.Change{Path: report, Content: rb}, FromRuleset: cur.RulesetVersion, ToRuleset: next.RulesetVersion}
		if archived {
			out.ArchivePath = archive
		}
		if req.Baseline != "" {
			out.Reason, out.ToPolicy = review.ScopeRoundBaseline, req.Baseline
			out.Receipt = &store.RoundReceipt{Kind: ScopeBaselineReceipt, ScopePolicy: req.Baseline, Basis: req.basis()}
		}
		if a := req.Activation; a != nil {
			out.Reason, out.FromPolicy, out.ToPolicy = review.ScopeRoundActivation, a.From, a.To
			out.Receipt = &store.RoundReceipt{Kind: ScopeActivationReceipt, ScopePolicy: a.To, Basis: req.basis()}
			out.PolicyReplace = &store.Change{Path: a.PolicyPath, Content: act.candidate}
			if act.active != nil {
				_, _, pa := activationNames(req.OldPath, req.OutPath, a.PolicyPath, a.From, cur, curRaw)
				out.PolicyArchive = &store.Change{Path: pa, Content: act.active}
			}
		}
		return out, nil
	})
	return res, err
}

func (req Request) basis() string { return BasisUpstream }

// activationFiles — 잠금 안에서 읽은 두 정책의 바이트. active는 활성 정책 파일이 없으면 nil이다.
type activationFiles struct{ active, candidate []byte }

// readActivation — 두 정책을 읽어 지문을 견준다. 활성 정책 파일이 없으면 규칙 0개 정책이다. 후보는 있어야 하고 읽혀야 한다.
func readActivation(a Activation) (activationFiles, error) {
	var af activationFiles
	cand, err := os.ReadFile(a.CandidatePath)
	if err != nil {
		return af, fmt.Errorf("%w: %v", ErrNoCandidate, err)
	}
	to, err := invscope.PolicyFileDigest(a.CandidatePath)
	if err != nil {
		return af, fmt.Errorf("%w: %v", ErrNoCandidate, err)
	}
	from, err := invscope.PolicyFileDigest(a.PolicyPath)
	if err != nil {
		return af, err
	}
	if from != a.From || to != a.To {
		return af, fmt.Errorf("%w: a policy changed after the confirmation", store.ErrStale)
	}
	if b, err := os.ReadFile(a.PolicyPath); err == nil {
		af.active = b
	} else if !errors.Is(err, os.ErrNotExist) {
		return af, err
	}
	af.candidate = cand
	return af, nil
}

func (req Request) names(old review.Session, raw []byte) (archive, report string) {
	if a := req.Activation; a != nil {
		archive, report, _ = activationNames(req.OldPath, req.OutPath, a.PolicyPath, a.From, old, raw)
	} else if req.Baseline != "" {
		archive, report = baselineNames(req.OldPath, req.OutPath, old, raw)
	} else {
		archive, report = Names(req.OldPath, req.OutPath, old, raw)
	}
	if req.ReportPath != "" {
		report = req.ReportPath
	}
	return archive, report
}

func load(p string) (review.Session, []byte, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return review.Session{}, nil, err
	}
	sf, err := review.Load(p)
	return sf, raw, err
}

// changed — 세션의 상류 규칙 판이 지금 도구와 다른가(빈 규칙 판도 다른 것이다).
func changed(sf review.Session) bool {
	return review.UpstreamRuleset(sf.RulesetVersion) != review.UpstreamRuleset(review.RulesetVersion)
}

// sameOrg — 옛 세션의 스코프와 요청한 조직이 같은가(rebuiltToo면 다시 세운 세션의 스코프도).
func sameOrg(oldScope, org, rebuiltScope string, rebuiltToo bool) error {
	want := "org://" + org
	if oldScope != want || (rebuiltToo && rebuiltScope != want) {
		return fmt.Errorf("%w: old session %q, rebuilt %q, requested %q", ErrOrgMismatch, oldScope, rebuiltScope, want)
	}
	return nil
}

// internalID — 저장소가 그 세션을 부르는 이름. 세션 ID가 있으면 그것이고, 없으면(옛 빌드의 파일) **절대 경로와 옛 바이트의
// 해시**다. 하나의 이름(unnamed-session)을 함께 쓰면 한 상태 폴더의 이름 없는 세션 둘 가운데 첫 회차의 표지가 둘째를 막는다.
func internalID(path, id string, raw []byte) string {
	if id != "" {
		return id
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	h := sha256.Sum256(append([]byte(abs+"\x00"), raw...))
	return "unnamed-" + hex.EncodeToString(h[:8])
}

func same(a, b string) bool {
	x, err1 := filepath.Abs(a)
	y, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && x == y
}
