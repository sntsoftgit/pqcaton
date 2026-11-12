// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package upstream_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/ingest"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/normalize"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/resultio"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

const reconcilePolicyB = "action,runtime,lib,app_key,note\nexclude,openssl,libssl*,*,not managed\n"

// reconcileWorld — 두 노드의 결과 폴더 · 선언 · 정책 B 파일, 그리고 결과를 policy(nil이면 정책 없이)로 적재한 메모리 이력.
func reconcileWorld(t *testing.T, ingestUnderB bool) (dir string, d decl.Declaration, policyPath string, mem *history.MemStore) {
	t.Helper()
	dir = t.TempDir()
	results := filepath.Join(dir, "results")
	if err := os.Mkdir(results, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, n := range []string{"web-01", "db-01"} {
		b, _ := protojson.Marshal(result(n))
		if err := os.WriteFile(filepath.Join(results, fmt.Sprintf("r%d.json", i)), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	policyPath = filepath.Join(dir, "asset-scope.csv")
	if err := os.WriteFile(policyPath, []byte(reconcilePolicyB), 0o644); err != nil {
		t.Fatal(err)
	}
	d = decl.Declaration{Org: "acme", Scope: []string{"web", "db"},
		Nodes: []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}, ObservedAs: []string{"web-01"}},
			{Name: "db", IPs: []string{"10.0.0.2"}, ObservedAs: []string{"db-01"}}}}
	mem = history.NewMemStore()
	ingestInto(t, mem, results, policyPath, ingestUnderB, "t")
	return dir, d, policyPath, mem
}

func ingestInto(t *testing.T, mem *history.MemStore, results, policyPath string, underPolicy bool, prefix string) {
	t.Helper()
	loaded, flaws := resultio.LoadDir(results)
	if len(flaws) != 0 {
		t.Fatal(flaws)
	}
	opts := ingest.IngestOptions{SnapshotPrefix: prefix, RulesetVersion: normalize.RulesetVersion, Store: mem}
	if underPolicy {
		p, err := review.LoadAssetPolicy(policyPath)
		if err != nil {
			t.Fatal(err)
		}
		opts.AssetPolicy = p
	}
	if _, err := ingest.IngestWith(loaded, opts); err != nil {
		t.Fatal(err)
	}
}

// slowLookup — 조회마다 delay를 기다린다. calls는 조회 횟수다.
type slowLookup struct {
	upstream.Lookup
	delay time.Duration
	calls atomic.Int32
}

func (s *slowLookup) ByContentHashV1(node, ruleset, digest string) (*history.Snapshot, error) {
	s.calls.Add(1)
	time.Sleep(s.delay)
	return s.Lookup.ByContentHashV1(node, ruleset, digest)
}

func (s *slowLookup) Latest(node string) (*history.Snapshot, error) {
	s.calls.Add(1)
	time.Sleep(s.delay)
	return s.Lookup.Latest(node)
}

// failLookup — 조회가 연결 정보를 실은 오류로 실패한다(pgconn의 연결 오류가 호스트와 사용자를 싣는 것을 흉내 낸다).
type failLookup struct{}

var errWithDSN = errors.New("failed to connect to `host=10.9.9.9 user=secretuser`: postgres://secretuser:secretpw@10.9.9.9/pqcota")

func (failLookup) ByContentHashV1(string, string, string) (*history.Snapshot, error) {
	return nil, errWithDSN
}
func (failLookup) Latest(string) (*history.Snapshot, error) { return nil, errWithDSN }

// IC-S22 — **pqcota 이력 대조(Reconcile)는 화면과 명령이 함께 쓰는 판정이다.** 정책 B로 적재한 이력에서 B로 대조하면 통과하고
// 기록(확인 시각 · 조직 · 노드별 지문)을 돌려준다. 정책 없이만 적재돼 있으면 ErrMissing(지문 불일치)이다. B로 적재한 뒤 정책
// 없이 다시 적재하면 B의 스냅샷은 있어도 최신이 아니므로 ErrMissing이고 ErrRevert도 감싼다(이전 정책 복귀가 이렇게 보인다).
// 결과에 근거가 없으면 ErrNothingToVerify다.
func TestReconcileJudgesAgainstTheLatest(t *testing.T) {
	now := time.Date(2026, 11, 12, 1, 2, 3, 0, time.UTC)
	dir, d, policyPath, mem := reconcileWorld(t, true)
	results := filepath.Join(dir, "results")
	res, err := upstream.Reconcile(mem, "default", d, results, policyPath, now)
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Record()
	if len(res.Refs) != 2 || rec.CheckedAt != "2026-11-12T01:02:03Z" || rec.Org != "default" || len(rec.Snapshots) != 2 ||
		rec.Snapshots[0].Digest == "" || rec.Snapshots[0].Node == "" || rec.Snapshots[0].Ruleset == "" {
		t.Errorf("record %+v (refs %d)", rec, len(res.Refs))
	}

	_, _, _, unpoliced := reconcileWorld(t, false)
	if _, err := upstream.Reconcile(unpoliced, "default", d, results, policyPath, now); !errors.Is(err, upstream.ErrMissing) || errors.Is(err, upstream.ErrRevert) {
		t.Errorf("ingested without the policy: err = %v, want ErrMissing only", err)
	}

	time.Sleep(1100 * time.Millisecond)
	ingestInto(t, mem, results, policyPath, false, "t2")
	if _, err := upstream.Reconcile(mem, "default", d, results, policyPath, now); !errors.Is(err, upstream.ErrMissing) || !errors.Is(err, upstream.ErrRevert) {
		t.Errorf("B's snapshots no longer the latest: err = %v, want ErrMissing and ErrRevert", err)
	}

	empty := filepath.Join(t.TempDir(), "results")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := upstream.Reconcile(mem, "default", d, empty, policyPath, now); !errors.Is(err, upstream.ErrNothingToVerify) {
		t.Errorf("no evidence: err = %v, want ErrNothingToVerify", err)
	}
}

// IC-S23 — **대조 전체가 제한 시간 하나에 묶인다.** 조회 하나하나는 제한보다 짧아도 합이 넘으면 ErrUnreachable(timed out)이다.
// 노드마다 시간을 새로 주지 않는다. 제한 안에 끝나면 통과한다.
func TestReconcileHasOneDeadline(t *testing.T) {
	prev := upstream.Timeout
	t.Cleanup(func() { upstream.Timeout = prev })
	dir, d, policyPath, mem := reconcileWorld(t, true)
	results := filepath.Join(dir, "results")
	upstream.Timeout = 150 * time.Millisecond
	slow := &slowLookup{Lookup: mem, delay: 60 * time.Millisecond} // 조회 넷(두 노드 × 둘) = 240ms
	start := time.Now()
	_, err := upstream.Reconcile(slow, "default", d, results, policyPath, time.Now())
	if !errors.Is(err, upstream.ErrUnreachable) || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want ErrUnreachable (timed out)", err)
	}
	if el := time.Since(start); el > 220*time.Millisecond {
		t.Errorf("returned after %s; the deadline is for the whole reconciliation", el)
	}
	upstream.Timeout = 2 * time.Second
	if _, err := upstream.Reconcile(&slowLookup{Lookup: mem, delay: 10 * time.Millisecond}, "default", d, results, policyPath, time.Now()); err != nil {
		t.Errorf("within the deadline: %v", err)
	}
}

// IC-S24 — **조회 오류는 연결 정보 없이 분류만 남긴다.** 조회가 호스트 · 사용자 · 비밀번호를 실은 오류로 실패해도 Reconcile과
// Probe의 오류는 ErrUnreachable이고 그 값들을 싣지 않는다. 이력이 읽히면 Probe는 통과한다. PQCOTA_DSN이 비면 OpenPg는
// ErrNoDSN이다. 닿지 않는 주소면 ErrUnreachable이고, 오류에 DSN의 사용자 · 비밀번호 · 호스트가 없다.
func TestUnreachableHidesTheDSN(t *testing.T) {
	dir, d, policyPath, mem := reconcileWorld(t, true)
	secret := func(what string, err error) {
		t.Helper()
		if !errors.Is(err, upstream.ErrUnreachable) {
			t.Errorf("%s: err = %v, want ErrUnreachable", what, err)
			return
		}
		for _, s := range []string{"secretuser", "secretpw", "10.9.9.9", "127.0.0.1"} {
			if strings.Contains(err.Error(), s) {
				t.Errorf("%s: the error carries %q: %v", what, s, err)
			}
		}
	}
	_, err := upstream.Reconcile(failLookup{}, "default", d, filepath.Join(dir, "results"), policyPath, time.Now())
	secret("reconcile", err)
	secret("probe", upstream.Probe(failLookup{}))
	if err := upstream.Probe(mem); err != nil {
		t.Errorf("probe on a readable history: %v", err)
	}
	if _, _, err := upstream.OpenPg("", "default"); !errors.Is(err, upstream.ErrNoDSN) {
		t.Errorf("empty DSN: err = %v, want ErrNoDSN", err)
	}
	_, _, err = upstream.OpenPg("postgres://secretuser:secretpw@127.0.0.1:1/pqcota?connect_timeout=2", "default")
	secret("open", err)
}
