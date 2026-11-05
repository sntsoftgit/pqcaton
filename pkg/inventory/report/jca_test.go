// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package report_test

import (
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
)

func jvmResult(src string, services bool) *discoveryv1.CollectionResult {
	props := `{"name":"pqcota:crypto_runtime","value":"jca"},{"name":"pqcota:jca.provider_set","value":"SUN,SunEC"}`
	if services {
		props += `,{"name":"pqcota:jca.pqc_services","value":"SunEC="}`
	}
	cbom := `{"bomFormat":"CycloneDX","specVersion":"1.6","components":[{"type":"cryptographic-asset","name":"jca-provider-chain","properties":[` + props + `]}]}`
	return &discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{TargetNodeId: src, CollectorId: "jvm-collector", CollectedAt: timestamppb.New(timeAt(1)),
			DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION},
		CbomCyclonedx: []byte(cbom), CyclonedxSpecVersion: "1.6",
		Completeness: &commonv1.Completeness{LayersCovered: []commonv1.CollectionLayer{commonv1.CollectionLayer_COLLECTION_LAYER_PROCESS}},
	}
}

// IC-R27 — **JCA 결과가 있는데 PQC 서비스 목록이 없는 노드만 알린다.** 목록이 있으면(provider가 PQC 서비스를 하나도
// 등록하지 않아 `SunEC=`처럼 비어도) 알리지 않는다. JCA 결과가 아예 없는 노드도 알리지 않는다: 그것은 JCA를 관측하지
// 않은 것이고, 옛 수집기일 수 있다는 경고와 섞으면 안 된다.
func TestOnlyJCAWithoutServicesIsFlagged(t *testing.T) {
	dir := t.TempDir()
	writeResults(t, dir, jvmResult("app-old", false), jvmResult("app-new", true), resultWithApps("web", 1, "libssl"))
	d := decl.Declaration{Org: "acme", Scope: []string{"app-old", "app-new", "web"},
		Nodes: []decl.Node{{Name: "app-old"}, {Name: "app-new"}, {Name: "web"}}}
	r, err := report.BuildWith(dir, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.JCAUnconfirmed) != 1 || r.JCAUnconfirmed[0] != "app-old" {
		t.Errorf("JCAUnconfirmed = %v, want [app-old]", r.JCAUnconfirmed)
	}
}
