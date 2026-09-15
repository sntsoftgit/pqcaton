package report_test

import (
	"fmt"
	"strings"
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
)

// 앱 열쇠가 달린 openssl 결과. 데모의 모양이다: libcrypto 를 sshd 가 로드하고 있다.
func resultWithApps(src string, at int64, lib string, apps ...string) *discoveryv1.CollectionResult {
	cbom := fmt.Sprintf(`{"bomFormat":"CycloneDX","specVersion":"1.6","components":[
      {"type":"cryptographic-asset","name":%q,"properties":[
        {"name":"pqcota:crypto_runtime","value":"openssl"},
        {"name":"pqcota:openssl.version","value":"3.0.2"},
        {"name":"pqcota:openssl.fork","value":"OpenSSL"},
        {"name":"pqcota:app_keys","value":%q}]}]}`, lib, strings.Join(apps, ","))
	return &discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{TargetNodeId: src, CollectorId: "openssl-collector",
			CollectedAt:     timestamppb.New(timeAt(at)),
			DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION},
		CbomCyclonedx: []byte(cbom), CyclonedxSpecVersion: "1.6",
		Completeness: &commonv1.Completeness{LayersCovered: []commonv1.CollectionLayer{commonv1.CollectionLayer_COLLECTION_LAYER_PROCESS}},
	}
}

// ★ IC-R23 — 정책이 뺀 선언 자산은 UNOBSERVED 가 아니라 CONFIRMED + EXCLUDED_BY_POLICY 다.
//
// 데모에서 실제로 났던 모양이다. 선언은 openssl/libcrypto 를 관리 대상으로 적었고, 정책은 그것을
// 관측한 통로(sshd)를 앱 열쇠로 뺐다. 전에는 제외된 finding 이 정규화 안에서 사라져 대조가
// 「선언했는데 보지 못했다」로 읽었다. **보았고, 관리하지 않기로 한 것이다.** 상류 적재는 이 구분을
// 지켰고(excluded … not absence) 대조에서 무너졌었다. 이제 대조 축은 CONFIRMED, 관리 축은
// 제외이고, 리포트가 선언·정책의 어긋남을 앱 열쇠와 함께 경고한다.
func TestPolicyExcludedDeclaredAssetIsConfirmedNotUnobserved(t *testing.T) {
	dir := t.TempDir()
	writeResults(t, dir, resultWithApps("pay-db", 100, "libcrypto.so.3", "/usr/sbin/sshd", "/usr/bin/python3"))
	d := decl.Declaration{Org: "acme", Scope: []string{"pay-db"},
		Nodes:  []decl.Node{{Name: "pay-db", IPs: []string{"10.0.0.2"}}},
		Assets: []decl.Asset{{Node: "pay-db", Runtime: "openssl", Component: "libcrypto"}}}
	policy := &scope.AssetPolicy{Rules: []scope.AssetRule{{Runtime: "*", Lib: "*", AppKey: "/usr/sbin/sshd*", Exclude: true}}}

	r, err := report.BuildWith(dir, d, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Assets) != 1 {
		t.Fatalf("자산 %d, want 1: %+v", len(r.Assets), r.Assets)
	}
	a := r.Assets[0]
	if a.State != reconcile.Confirmed {
		t.Errorf("대조 축 = %s, want CONFIRMED - 제외는 부재가 아니다", a.State)
	}
	if a.Managed != reconcile.ExcludedByPolicy {
		t.Errorf("관리 축 = %s, want EXCLUDED_BY_POLICY", a.Managed)
	}
	if len(a.ExcludedSources) != 1 || len(a.ExcludedSources[0].AppKeys) != 2 {
		t.Errorf("제외 근거에 앱 열쇠 전부가 실려야 한다: %+v", a.ExcludedSources)
	}
	if len(r.PolicyConflicts) != 1 {
		t.Fatalf("선언·정책 어긋남 %d건, want 1", len(r.PolicyConflicts))
	}

	view := reconcile.RenderView(r.Assets)
	for _, want := range []string{"excluded by the asset-scope policy: 1", "declared as managed, but the asset-scope policy excludes it",
		"pay-db/openssl/libcrypto", "/usr/sbin/sshd, /usr/bin/python3", "same policy code"} {
		if !strings.Contains(view, want) {
			t.Errorf("리포트에 %q 가 없다:\n%s", want, view)
		}
	}
	if strings.Contains(view, "UNOBSERVED   n/a") || strings.Contains(view, "mandatory review queue") {
		t.Errorf("제외 전용이 리뷰 큐에 올랐다:\n%s", view)
	}

	// 같은 입력에 정책을 걸지 않으면 관리 대상이고 어긋남도 없다.
	r2, err := report.BuildWith(dir, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Assets[0].Managed != reconcile.Managed || len(r2.PolicyConflicts) != 0 {
		t.Errorf("정책 없이: %s · 어긋남 %d", r2.Assets[0].Managed, len(r2.PolicyConflicts))
	}
}

// IC-R24 — 관리 근거는 정책을 건 스냅샷에서만 나온다. 정책 없이 정규화한 스냅샷은 제외분을
// 찾는 데만 쓰고 그 지문은 어디에도 실리지 않는다 - 적재되지 않은 스냅샷의 지문은 아무것도
// 가리키지 않는다. 그래서 정책을 걸었을 때의 관리 근거 지문은 TestSnapshotDigestMatchesUpstreamIngest 가
// 재는 상류 지문과 같아야 하고,
// 제외 근거에는 지문이 아예 없다.
func TestExcludedSourcesCarryNoSnapshotLocation(t *testing.T) {
	dir := t.TempDir()
	writeResults(t, dir,
		resultWithApps("web", 100, "libssl.so.3", "/opt/app/bin/app"),
		resultWithApps("web", 200, "libcrypto.so.3", "/usr/sbin/sshd"))
	d := decl.Declaration{Org: "acme", Scope: []string{"web"},
		Nodes: []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}}},
		Assets: []decl.Asset{{Node: "web", Runtime: "openssl", Component: "libssl"},
			{Node: "web", Runtime: "openssl", Component: "libcrypto"}}}
	policy := &scope.AssetPolicy{Rules: []scope.AssetRule{{Runtime: "*", Lib: "*", AppKey: "/usr/sbin/sshd*", Exclude: true}}}
	r, err := report.BuildWith(dir, d, policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r.Assets {
		switch a.Key.Component {
		case "libssl":
			if a.Managed != reconcile.Managed || a.Sources[0].Snapshot.Digest == "" {
				t.Errorf("관리 근거에 스냅샷 지문이 없다: %+v", a)
			}
		case "libcrypto":
			if a.Managed != reconcile.ExcludedByPolicy || len(a.Sources) != 0 {
				t.Errorf("제외 전용에 관리 근거가 실렸다: %+v", a)
			}
		}
	}
}

// IC-R25 — **관측 자산 수는 정책이 뺀 것도 센다.** 보았으므로. 관리 근거의 수로 세면 머리에서
// 「관측 2」라 하고 바로 아래 런타임별 합계는 5 라고 하는 리포트가 나온다 - 제외를 부재로 세는 것이고,
// 이 판이 닫으려는 바로 그 결함이다. 관리 수는 따로 든다.
func TestObservedCountIncludesPolicyExcludedAssets(t *testing.T) {
	dir := t.TempDir()
	writeResults(t, dir,
		resultWithApps("web", 100, "libssl.so.3", "/opt/app/bin/app"),
		resultWithApps("web", 200, "libcrypto.so.3", "/usr/sbin/sshd"))
	d := decl.Declaration{Org: "acme", Scope: []string{"web"},
		Nodes:  []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}}},
		Assets: []decl.Asset{{Node: "web", Runtime: "openssl", Component: "libssl"}, {Node: "web", Runtime: "openssl", Component: "libpq"}}}
	policy := &scope.AssetPolicy{Rules: []scope.AssetRule{{Runtime: "*", Lib: "*", AppKey: "/usr/sbin/sshd*", Exclude: true}}}
	r, err := report.BuildWith(dir, d, policy)
	if err != nil {
		t.Fatal(err)
	}
	// 관측 둘(libssl 관리 · libcrypto 제외) · 미관측 하나(libpq).
	if r.ObservedAssets != 2 || r.ManagedAssets != 1 {
		t.Errorf("관측 %d · 관리 %d, want 2 · 1", r.ObservedAssets, r.ManagedAssets)
	}
	sum := 0
	for _, n := range r.ObservedByRuntime() {
		sum += n
	}
	if sum != r.ObservedAssets {
		t.Errorf("머리의 관측 수(%d)와 런타임별 합계(%d)가 다르다", r.ObservedAssets, sum)
	}
}
