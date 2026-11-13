// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package upstream_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/ingest"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/normalize"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/resultio"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// policyNone — 규칙이 없는 정책(A). 정책 B(reconcilePolicyB)와 지문이 다르다.
const policyNone = "action,runtime,lib,app_key,note\n"

// ingestPg — 결과 폴더를 정책 파일로 Postgres 이력에 적재한다.
func ingestPg(t *testing.T, st *history.PgStore, results, policyPath, prefix string) {
	t.Helper()
	loaded, flaws := resultio.LoadDir(results)
	if len(flaws) != 0 {
		t.Fatal(flaws)
	}
	p, err := review.LoadAssetPolicy(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	opts := ingest.IngestOptions{SnapshotPrefix: prefix, RulesetVersion: normalize.RulesetVersion, Store: st, AssetPolicy: p}
	if _, err := ingest.IngestWith(loaded, opts); err != nil {
		t.Fatal(err)
	}
}

// IC-S26 — **노드의 최신은 마지막에 적재한 상태다(Postgres).** 실제 pqcota Postgres 이력에 정책 A로 적재하면 최신이 A이고 A의
// 대조가 통과한다. 정책 B로 적재하면 최신이 B이고, A는 이력에 있어도 최신이 아니므로 ErrMissing · ErrRevert다. A로 다시
// 적재하면 스냅샷 본체는 새로 만들지 않지만(노드의 스냅샷 수가 그대로이고 최신이 처음의 A다) 최신 관측이 A를 가리켜 A의 대조가
// 통과하고, 이제 B가 ErrRevert다. pqcaton의 최신 스냅샷 검사와 ErrRevert 판정은 그대로다. 다른 조직의 이력으로 대조하면 A를
// 적재했어도 찾지 못하고(ErrMissing, 과거 결과가 아니므로 ErrRevert는 아니다) 조직 확인(SameOrg)이 ErrOrg다. 메모리 저장소로는
// 이 차이가 드러나지 않아 Postgres로 잰다. PQCOTA_TEST_DSN이 있을 때만 돈다.
func TestLatestIsTheLastLoadedStateOnPostgres(t *testing.T) {
	dsn := os.Getenv("PQCOTA_TEST_DSN")
	if dsn == "" {
		t.Skip("PQCOTA_TEST_DSN is not set: the Postgres reconciliation test is skipped")
	}
	ctx := context.Background()
	org := "pqcaton-s26-" + strconv.FormatInt(time.Now().UnixNano(), 36) // 실행마다 새 조직(이력은 append-only라 지우지 않는다)
	st, err := history.NewPgStoreIn(ctx, dsn, org)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	dir, d, policyB, _ := reconcileWorld(t, true)
	results := filepath.Join(dir, "results")
	policyA := filepath.Join(dir, "policy-a.csv")
	if err := os.WriteFile(policyA, []byte(policyNone), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 13, 1, 2, 3, 0, time.UTC)
	reconcile := func(l upstream.Lookup, policy string) error {
		_, err := upstream.Reconcile(l, org, d, results, policy, now)
		return err
	}
	passes := func(when, name, policy string) {
		t.Helper()
		if err := reconcile(st, policy); err != nil {
			t.Fatalf("%s: reconciling %s = %v, want it to pass", when, name, err)
		}
	}
	reverts := func(when, name, policy string) {
		t.Helper()
		if err := reconcile(st, policy); !errors.Is(err, upstream.ErrMissing) || !errors.Is(err, upstream.ErrRevert) {
			t.Fatalf("%s: reconciling %s = %v, want ErrMissing and ErrRevert", when, name, err)
		}
	}
	nodes := []string{"web-01", "db-01"}
	latest := func(node string) *history.Snapshot {
		t.Helper()
		s, err := st.Latest(node)
		if err != nil || s == nil {
			t.Fatalf("latest of %s: %v %v", node, s, err)
		}
		return s
	}
	count := func(node string) int {
		t.Helper()
		ss, err := st.Snapshots(node)
		if err != nil {
			t.Fatal(err)
		}
		return len(ss)
	}

	ingestPg(t, st, results, policyA, "a1")
	firstA := map[string]string{}
	for _, n := range nodes {
		firstA[n] = latest(n).ID
	}
	passes("after A", "A", policyA)

	time.Sleep(1100 * time.Millisecond)
	ingestPg(t, st, results, policyB, "b1")
	for _, n := range nodes {
		if latest(n).ID == firstA[n] {
			t.Fatalf("after B: the latest of %s is still A", n)
		}
	}
	passes("after B", "B", policyB)
	reverts("after A → B", "A", policyA)
	before := map[string]int{}
	for _, n := range nodes {
		before[n] = count(n)
	}

	time.Sleep(1100 * time.Millisecond)
	ingestPg(t, st, results, policyA, "a2")
	for _, n := range nodes {
		if got := count(n); got != before[n] {
			t.Errorf("loading A again made a new snapshot of %s (%d → %d); want the earlier one reused", n, before[n], got)
		}
		if got := latest(n).ID; got != firstA[n] {
			t.Errorf("after A → B → A the latest of %s is %s, want the first A %s", n, got, firstA[n])
		}
	}
	passes("after A → B → A", "A", policyA)
	reverts("after A → B → A", "B", policyB)

	// 다른 조직의 이력: A를 적재했어도 그 조직의 이력에는 없다.
	other, err := history.NewPgStoreIn(ctx, dsn, org+"-other")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := reconcile(other, policyA); !errors.Is(err, upstream.ErrMissing) || errors.Is(err, upstream.ErrRevert) {
		t.Errorf("another organization's history: err = %v, want ErrMissing only", err)
	}
	if err := upstream.SameOrg(string(other.Org()), org, "org://"+org); !errors.Is(err, upstream.ErrOrg) {
		t.Errorf("SameOrg with another organization's history = %v, want ErrOrg", err)
	}
}
