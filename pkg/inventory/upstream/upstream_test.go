// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package upstream_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

func result(node string) *discoveryv1.CollectionResult {
	cbom := `{"bomFormat":"CycloneDX","specVersion":"1.6","components":[
      {"type":"cryptographic-asset","name":"libcrypto","properties":[{"name":"pqcota:crypto_runtime","value":"openssl"},
        {"name":"pqcota:openssl.version","value":"3.0.2"},{"name":"pqcota:openssl.fork","value":"OpenSSL"}]},
      {"type":"cryptographic-asset","name":"libssl","properties":[{"name":"pqcota:crypto_runtime","value":"openssl"},
        {"name":"pqcota:openssl.version","value":"3.0.2"},{"name":"pqcota:openssl.fork","value":"OpenSSL"}]}]}`
	return &discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{TargetNodeId: node, CollectorId: "openssl-collector",
			CollectedAt:     timestamppb.New(time.Unix(1700000100, 0).UTC()),
			DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION},
		CbomCyclonedx: []byte(cbom), CyclonedxSpecVersion: "1.6",
		Completeness: &commonv1.Completeness{LayersCovered: []commonv1.CollectionLayer{commonv1.CollectionLayer_COLLECTION_LAYER_PROCESS}},
	}
}

// IC-S21 — **상류 이력 확인은 그 정책으로 적재한 스냅샷만 찾는다.** 정책 B로 적재한 상류에서, B로 대조한 근거는 모두 찾히고
// A(정책 없음)로 대조한 근거는 빠진 것으로 나온다. 근거가 하나도 없으면 증명된 것이 아니다.
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
	if _, err := upstream.Verify(store, nil); !errors.Is(err, upstream.ErrNothingToVerify) {
		t.Errorf("no evidence: err = %v, want ErrNothingToVerify", err)
	}
}
