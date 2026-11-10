// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package upstream_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	kscope "github.com/randyinthedev-hash/pqcota-common/pkg/kernel/scope"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/ingest"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/normalize"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/resultio"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

func result(node string) *discoveryv1.CollectionResult {
	return resultLibs(node, "libcrypto", "libssl")
}

// resultLibs — 노드 하나의 openssl 수집 결과. 라이브러리 이름을 노드마다 달리 줄 수 있다.
func resultLibs(node string, libs ...string) *discoveryv1.CollectionResult {
	var comps []string
	for _, l := range libs {
		comps = append(comps, `{"type":"cryptographic-asset","name":"`+l+`","properties":[{"name":"pqcota:crypto_runtime","value":"openssl"},
        {"name":"pqcota:openssl.version","value":"3.0.2"},{"name":"pqcota:openssl.fork","value":"OpenSSL"}]}`)
	}
	cbom := `{"bomFormat":"CycloneDX","specVersion":"1.6","components":[` + strings.Join(comps, ",") + `]}`
	return &discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{TargetNodeId: node, CollectorId: "openssl-collector",
			CollectedAt:     timestamppb.New(time.Unix(1700000100, 0).UTC()),
			DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION},
		CbomCyclonedx: []byte(cbom), CyclonedxSpecVersion: "1.6",
		Completeness: &commonv1.Completeness{LayersCovered: []commonv1.CollectionLayer{commonv1.CollectionLayer_COLLECTION_LAYER_PROCESS}},
	}
}

// IC-S21 — **상류 이력 확인은 그 정책으로 적재한 스냅샷이 노드의 최신일 때만 통과한다.** 정책 B로 적재한 상류에서, B로
// 대조한 근거는 모두 찾히고 A(정책 없음)로 대조한 근거는 빠진 것으로 나온다. 그 뒤 상류를 정책 없이 다시 적재하면, B의
// 스냅샷이 이력에 남아 있어도 최신이 아니므로 빠진 것으로 나온다(과거 실행의 같은 지문으로 통과하지 않는다). 근거가 하나도
// 없으면 증명된 것이 아니다.
func TestVerifyFindsOnlyTheIngestedPolicy(t *testing.T) {
	dir := t.TempDir()
	for i, n := range []string{"web-01", "db-01"} {
		b, _ := protojson.Marshal(result(n))
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("r%d.json", i)), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	policyB := &kscope.AssetPolicy{Rules: []kscope.AssetRule{{Exclude: true, Runtime: "openssl", Lib: "libssl*"}}}
	results, flaws := resultio.LoadDir(dir)
	if len(flaws) != 0 {
		t.Fatal(flaws)
	}
	store := history.NewMemStore()
	if _, err := ingest.IngestWith(results, ingest.IngestOptions{SnapshotPrefix: "t", RulesetVersion: normalize.RulesetVersion,
		Store: store, AssetPolicy: policyB}); err != nil {
		t.Fatal(err)
	}
	d := decl.Declaration{Org: "acme", Scope: []string{"web", "db"},
		Nodes: []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}, ObservedAs: []string{"web-01"}},
			{Name: "db", IPs: []string{"10.0.0.2"}, ObservedAs: []string{"db-01"}}}}
	underB, err := report.BuildWith(dir, d, policyB)
	if err != nil {
		t.Fatal(err)
	}
	refs := upstream.Refs(underB)
	if len(refs) != 2 {
		t.Fatalf("refs under B: %v", refs)
	}
	if missing, err := upstream.Verify(store, refs); err != nil || len(missing) != 0 {
		t.Errorf("snapshots ingested under B were not found: %v %v", missing, err)
	}
	underNone, err := report.BuildWith(dir, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if missing, err := upstream.Verify(store, upstream.Refs(underNone)); !errors.Is(err, upstream.ErrMissing) || len(missing) != 2 {
		t.Errorf("snapshots under another policy: missing %v, err %v; want both missing", missing, err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := ingest.IngestWith(results, ingest.IngestOptions{SnapshotPrefix: "t2", RulesetVersion: normalize.RulesetVersion,
		Store: store}); err != nil {
		t.Fatal(err)
	}
	if missing, err := upstream.Verify(store, refs); !errors.Is(err, upstream.ErrMissing) || len(missing) != 2 {
		t.Errorf("B's snapshots after a newer ingest without the policy: missing %v, err %v; want both (not latest)", missing, err)
	}
	if _, err := upstream.Verify(store, nil); !errors.Is(err, upstream.ErrNothingToVerify) {
		t.Errorf("no evidence: err = %v, want ErrNothingToVerify", err)
	}
}

// IC-S21 — **관리 자산이 0개가 된 노드도 확인한다.** web-01은 libcrypto · libssl, db-01은 libssl · libgcrypt를 쓴다. 정책 B
// (libssl 제외)로 적재한 상류에서, 정책 C(B + libgcrypt 제외)는 web-01을 B와 같게 두고 db-01의 관리 자산만 0개로 만든다.
// 다시 적재하지 않았으면 db-01의 빈 스냅샷이 상류의 최신이 아니므로 반드시 거절한다(자산 근거만 보면 web-01만 남아 통과해
// 버린다). C로 다시 적재하면 상류가 빈 스냅샷도 적재하므로 통과한다.
func TestVerifyChecksNodesLeftWithNoManagedAssets(t *testing.T) {
	dir := t.TempDir()
	for i, r := range []*discoveryv1.CollectionResult{resultLibs("web-01", "libcrypto", "libssl"), resultLibs("db-01", "libssl", "libgcrypt")} {
		b, _ := protojson.Marshal(r)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("r%d.json", i)), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	policyB := &kscope.AssetPolicy{Rules: []kscope.AssetRule{{Exclude: true, Runtime: "openssl", Lib: "libssl*"}}}
	policyC := &kscope.AssetPolicy{Rules: append(append([]kscope.AssetRule(nil), policyB.Rules...),
		kscope.AssetRule{Exclude: true, Runtime: "openssl", Lib: "libgcrypt*"})}
	results, flaws := resultio.LoadDir(dir)
	if len(flaws) != 0 {
		t.Fatal(flaws)
	}
	store := history.NewMemStore()
	if _, err := ingest.IngestWith(results, ingest.IngestOptions{SnapshotPrefix: "t", RulesetVersion: normalize.RulesetVersion,
		Store: store, AssetPolicy: policyB}); err != nil {
		t.Fatal(err)
	}
	d := decl.Declaration{Org: "acme", Scope: []string{"web", "db"},
		Nodes: []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}, ObservedAs: []string{"web-01"}},
			{Name: "db", IPs: []string{"10.0.0.2"}, ObservedAs: []string{"db-01"}}}}
	underC, err := report.BuildWith(dir, d, policyC)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range underC.Assets {
		for _, src := range a.Sources {
			if src.Snapshot.SourceNodeID == "db-01" && a.Managed == reconcile.Managed {
				t.Fatalf("db-01 still has a managed asset under C: %+v", a.Key)
			}
		}
	}
	refs := upstream.Refs(underC)
	if len(refs) != 2 {
		t.Fatalf("refs under C = %v, want both nodes (db-01 has no managed asset but is still checked)", refs)
	}
	missing, err := upstream.Verify(store, refs)
	if !errors.Is(err, upstream.ErrMissing) || len(missing) != 1 || missing[0].Node != "db-01" {
		t.Fatalf("db-01 left with no managed assets, not re-ingested: missing %v, err %v; want db-01 refused", missing, err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := ingest.IngestWith(results, ingest.IngestOptions{SnapshotPrefix: "t2", RulesetVersion: normalize.RulesetVersion,
		Store: store, AssetPolicy: policyC}); err != nil {
		t.Fatal(err)
	}
	if missing, err := upstream.Verify(store, refs); err != nil || len(missing) != 0 {
		t.Errorf("after re-ingesting under C: missing %v, err %v; want all found (the empty snapshot is stored upstream)", missing, err)
	}
}
