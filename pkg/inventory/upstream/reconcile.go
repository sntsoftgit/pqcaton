// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package upstream

// pqcota 이력 대조. 화면(05)과 명령(pqcaton-decide baseline · activate-scope)이 **같은 이 함수**를 쓴다.
//
// 순서는 부르는 쪽이 지킨다: (1) 작업 폴더 입력의 스냅샷을 뜨고, (2) 그 입력과 정책으로 여기서 대조하고, (3) 실패하면 쓰지
// 않고, (4) 성공하면 곧바로 실행 잠금을 잡고, (5) 1의 스냅샷이 그대로인지 다시 재고, (6) 그대로일 때만 한 트랜잭션으로 쓴다.
// 대조는 잠금 밖에서 한다. pqcota가 선택 사항이라는 뜻이 아니라, 느린 조회 동안 작업 폴더 잠금을 쥐지 않으려는 순서다.
//
// **받아들이는 한계(TOCTOU).** 대조가 끝난 뒤부터 쓰기가 끝날 때까지 pqcota 이력이 바뀌면 잡지 못한다. 5의 재확인은 작업 폴더
// 입력만 본다. 그래서 대조 결과는 「확인 시점에 pqcota 이력과 일치했다」는 뜻이고, 지금도 일치한다거나 적재를 증명한다는
// 뜻이 아니다. pqcota가 안정된 current pointer나 적재 기록을 주기 전에는 이보다 강하게 말할 수 없다.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// DSNEnv — pqcota 이력의 연결 정보. 이 이름으로만 받는다. 값은 어디에도 적지 않는다.
const DSNEnv = "PQCOTA_DSN"

// Timeout — 대조 하나 전체의 제한 시간. 노드마다 새로 주지 않는다. 시작할 때의 연결 확인도 이 시간 안에 끝나야 한다.
var Timeout = 10 * time.Second

// ErrUnreachable — pqcota 이력에 닿지 못했거나 조회가 실패했거나 제한 시간을 넘었다. 원래 오류 문구에는 연결 정보가 섞일 수
// 있어 싣지 않고 분류만 싣는다.
var ErrUnreachable = errors.New("pqcota history: unreachable")

// ErrRevert — 빠진 스냅샷이 모두 pqcota 이력에 **있지만 최신이 아니다.** 이전 정책으로 되돌리는 경우가 이렇다: pqcota는 같은
// 내용의 스냅샷을 다시 적재해도 새로 만들지 않아 최신이 움직이지 않는다(pqcota-inventory#1). 그 뒤 다른 정책으로 다시 적재한
// 경우도 같다. 어느 쪽이든 대조가 통과하지 않으므로 활성화하지 않는다. ErrMissing을 함께 감싼다.
var ErrRevert = errors.New("the snapshots exist but are not the latest for their node (reverting to an earlier policy cannot be reconciled yet)")

// ErrOrg — pqcota 이력의 조직이 판정 조직이나 판정 세션의 조직과 다르다. 조회하기 전에 멈춘다. 다른 조직의 이력으로 정책
// 회차를 열 수 없다.
var ErrOrg = errors.New("pqcota history: the organization does not match")

// OrgFor — 조회할 pqcota 이력의 조직. 판정 조직(org)이다. given(-upstream-org)은 비었거나 같은 값이어야 한다.
func OrgFor(given, org string) (string, error) {
	if given != "" && given != org {
		return "", fmt.Errorf("%w: -upstream-org %q is not the organization of the judgments %q", ErrOrg, given, org)
	}
	return org, nil
}

// SameOrg — 연 이력의 조직(history) · 판정 조직(org) · 판정 세션의 조직이 모두 같은가. sessionScope는 세션 파일의 scope
// 값(`org://<조직>`)이다. 대조하기 전에 부른다.
func SameOrg(history, org, sessionScope string) error {
	if history != org || sessionScope != "org://"+org {
		return fmt.Errorf("%w: pqcota history %q, judgments %q, review session %q", ErrOrg, history, org, sessionScope)
	}
	return nil
}

// ErrNoDSN — PQCOTA_DSN이 비었다.
var ErrNoDSN = errors.New(DSNEnv + " is not set: pqcaton runs only against a working pqcota history")

// Result — 대조 하나의 기록. 비밀값이 없다(전환 보고서에 그대로 싣는다).
type Result struct {
	At   time.Time
	Org  string
	Refs []Ref
}

// Record — 전환 보고서에 싣는 꼴.
func (c Result) Record() *review.UpstreamCheck {
	out := &review.UpstreamCheck{CheckedAt: c.At.UTC().Format(time.RFC3339), Org: c.Org}
	for _, r := range c.Refs {
		out.Snapshots = append(out.Snapshots, review.UpstreamSnapshot{Node: r.Node, Ruleset: r.Ruleset, Digest: r.Digest})
	}
	return out
}

// Reconcile — 결과 폴더를 정책으로 대조해 얻은 노드별 지문이 pqcota 이력의 노드별 **최신** 스냅샷과 모두 같은가. 조회 전체가
// Timeout 하나 안에 끝나야 한다. 실패는 ErrNothingToVerify · ErrMissing(· ErrRevert) · ErrUnreachable 가운데 하나다.
func Reconcile(l Lookup, org string, d decl.Declaration, resultsDir string, policyPath string, now time.Time) (Result, error) {
	policy, err := review.LoadAssetPolicy(policyPath)
	if err != nil {
		return Result{}, err
	}
	r, err := report.BuildWith(resultsDir, d, policy)
	if err != nil {
		return Result{}, err
	}
	refs := Refs(r)
	if len(refs) == 0 {
		return Result{}, ErrNothingToVerify
	}
	var (
		missing []Ref
		vErr    error
	)
	if err := Within(Timeout, func() error {
		missing, vErr = verify(l, refs)
		return nil
	}); err != nil {
		return Result{}, err
	}
	if vErr != nil {
		if len(missing) > 0 {
			return Result{}, vErr
		}
		return Result{}, fmt.Errorf("%w: %s", ErrUnreachable, Classify(vErr))
	}
	return Result{At: now, Org: org, Refs: refs}, nil
}

// verify — Verify와 같되, 빠진 것이 모두 「있지만 최신이 아님」이면 ErrRevert도 감싼다.
func verify(l Lookup, refs []Ref) ([]Ref, error) {
	var missing []Ref
	stale := 0
	for _, ref := range refs {
		snap, err := l.ByContentHashV1(ref.Node, ref.Ruleset, ref.Digest)
		if err != nil {
			return nil, err
		}
		latest, err := l.Latest(ref.Node)
		if err != nil {
			return nil, err
		}
		if snap == nil || latest == nil || history.ContentHashV1(latest) != ref.Digest {
			missing = append(missing, ref)
			if snap != nil {
				stale++
			}
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	err := fmt.Errorf("%w: %d of %d (first: node %s, %s)", ErrMissing, len(missing), len(refs), missing[0].Node, missing[0].Digest)
	if stale == len(missing) {
		err = fmt.Errorf("%w: %w", ErrRevert, err)
	}
	return missing, err
}

// Within — f를 d 안에 끝낸다. 넘으면 ErrUnreachable(timed out)이다. pqcota 이력의 조회 함수는 context를 받지 않아 도중에 끊을
// 수 없으므로, 넘은 조회는 뒤에서 끝나게 두고 그 결과는 버린다.
func Within(d time.Duration, f func() error) error {
	done := make(chan error, 1)
	go func() { done <- f() }()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case err := <-done:
		return err
	case <-t.C:
		return fmt.Errorf("%w: timed out after %s", ErrUnreachable, d)
	}
}

// Classify — 조회 오류를 연결 정보가 없는 한 줄로. 원래 문구(pgconn은 호스트 · 사용자를 싣는다)는 쓰지 않는다.
func Classify(err error) string {
	var pg *pgconn.PgError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, history.ErrSchemaMissing):
		return "the pqcota history schema is missing"
	case errors.As(err, &pg):
		return "server error SQLSTATE " + pg.Code
	}
	return "connection or query failed"
}

// Probe — 시작할 때의 필수 조회. 연결된 이력에서 없는 노드의 최신 스냅샷을 한 번 읽어 스키마와 읽기 권한을 확인한다.
func Probe(l Lookup) error {
	return Within(Timeout, func() error {
		if _, err := l.Latest("pqcaton-probe"); err != nil {
			return fmt.Errorf("%w: %s", ErrUnreachable, Classify(err))
		}
		return nil
	})
}

// Store — 열린 pqcota 이력. Close로 닫는다.
type Store interface {
	Lookup
	Close()
}

// OpenPg — PQCOTA_DSN으로 pqcota 이력을 **조회용으로** 연다(DDL을 돌리지 않는다). 연결과 스키마 확인이 Timeout 안에 끝나야
// 한다. 오류에는 연결 정보를 싣지 않는다.
func OpenPg(dsn, org string) (Store, string, error) {
	if dsn == "" {
		return nil, "", ErrNoDSN
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	st, err := history.OpenPgStoreIn(ctx, dsn, org)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %s", ErrUnreachable, Classify(err))
	}
	return st, string(st.Org()), nil
}
