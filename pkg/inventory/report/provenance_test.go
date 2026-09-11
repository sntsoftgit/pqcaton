package report_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/history"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/resultio"
	"github.com/randyinthedev-hash/pqcota/pkg/inventory/ingest"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
)

// 되짚기의 전제 — 같은 결과 집합을 이 리포와 상류 적재가 **같은 스냅샷**으로 만들어 같은 지문을
// 낸다. 그 지문이 계약으로 건너가 상류 이력에서 찾힌다. 전에는 결과 하나마다 정규화하고 네트워크
// 결과를 비켜 두어, 어떤 경우에도 같은 지문이 나올 수 없었다.

// 상류 수집기가 내는 모양 그대로: openssl 결과 하나와 네트워크 결과 하나가 같은 원천 노드에서.
func resultFiles(src, collector string, at int64, version string, edges ...*discoveryv1.ObservedEdge) *discoveryv1.CollectionResult {
	cbom := fmt.Sprintf(`{"bomFormat":"CycloneDX","specVersion":"1.6","components":[
      {"type":"cryptographic-asset","name":"libcrypto","properties":[
        {"name":"pqcota:crypto_runtime","value":"openssl"},
        {"name":"pqcota:openssl.version","value":%q},
        {"name":"pqcota:openssl.fork","value":"OpenSSL"}]}]}`, version)
	return &discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{TargetNodeId: src, CollectorId: collector,
			CollectedAt:     timestamppb.New(timeAt(at)),
			DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION},
		CbomCyclonedx: []byte(cbom), CyclonedxSpecVersion: "1.6", ObservedEdges: edges,
		Completeness: &commonv1.Completeness{LayersCovered: []commonv1.CollectionLayer{commonv1.CollectionLayer_COLLECTION_LAYER_PROCESS}},
	}
}

func writeResults(t *testing.T, dir string, rs ...*discoveryv1.CollectionResult) {
	t.Helper()
	for i, r := range rs {
		b, err := protojson.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("r%02d.json", i)), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// ★ IC-R18 — 이 리포의 스냅샷 지문이 상류 적재의 것과 같다.
//
// 상류 `ingest.IngestWith` 를 메모리 이력에 돌려 저장된 스냅샷의 v1 지문을 얻고, 같은 결과 디렉터리를
// 이 리포의 대조가 읽어 낸 근거의 지문과 비교한다. 다르면 계약으로 건너간 참조가 중앙 이력에서
// 찾히지 않는다.
func TestSnapshotDigestMatchesUpstreamIngest(t *testing.T) {
	dir := t.TempDir()
	edge := &discoveryv1.ObservedEdge{SrcNodeId: "web-01.corp", DstNodeId: "db", Port: 5432,
		Protocol: discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, NegotiatedGroup: "x25519", ObservedCount: 2}
	writeResults(t, dir,
		resultFiles("web-01.corp", "openssl-collector", 100, "3.0.2"),
		resultFiles("web-01.corp", "network-collector", 200, "3.0.2", edge),
	)

	// 상류 적재.
	results, flaws := resultio.LoadDir(dir)
	if len(flaws) != 0 {
		t.Fatal(flaws)
	}
	store := history.NewMemStore()
	if _, err := ingest.IngestWith(results, ingest.IngestOptions{
		SnapshotPrefix: "ingest-t", RulesetVersion: normalize.RulesetVersion, Store: store, AssetPolicy: nil}); err != nil {
		t.Fatal(err)
	}
	snap, err := store.Latest("web-01.corp")
	if err != nil || snap == nil {
		t.Fatalf("상류 적재가 스냅샷을 남기지 않았다: %v", err)
	}
	want := history.ContentHashV1(snap)

	// 이 리포의 대조. 선언은 봉투 이름과 다른 이름을 쓰고 observed_as 로 잇는다.
	d := decl.Declaration{Org: "acme", Scope: []string{"web"},
		Nodes:  []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}, ObservedAs: []string{"web-01.corp"}}},
		Assets: []decl.Asset{{Node: "web", Runtime: "openssl", Component: "libcrypto"}}}
	r, err := report.Build(dir, d)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range r.Assets {
		if a.Key.NodeID != "web" || len(a.Sources) == 0 {
			continue
		}
		found = true
		s := a.Sources[0]
		if s.Snapshot.Digest != want {
			t.Errorf("지문이 상류와 다르다 — 참조가 중앙 이력에서 찾히지 않는다\n got  %s\n want %s", s.Snapshot.Digest, want)
		}
		if s.Snapshot.SourceNodeID != "web-01.corp" {
			t.Errorf("원천 노드가 봉투의 이름이 아니다: %q", s.Snapshot.SourceNodeID)
		}
		if s.Snapshot.RulesetVersion != normalize.RulesetVersion {
			t.Errorf("스냅샷 규칙 판이 상류 것이 아니다: %q", s.Snapshot.RulesetVersion)
		}
	}
	if !found {
		t.Fatal("대조 결과에 근거가 없다")
	}
	// 조회 키로 실제로 찾힌다.
	if hit, _ := store.ByContentHashV1("web-01.corp", normalize.RulesetVersion, want); hit == nil {
		t.Error("상류 이력이 이 지문으로 찾지 못한다")
	}
}

// IC-R18 — 결과 파일의 순서를 섞어도 같은 지문이다.
func TestSnapshotDigestIsOrderInvariant(t *testing.T) {
	mk := func(perm []int) string {
		dir := t.TempDir()
		all := []*discoveryv1.CollectionResult{
			resultFiles("n", "openssl-collector", 100, "3.0.2"),
			resultFiles("n", "network-collector", 200, "3.0.2", &discoveryv1.ObservedEdge{SrcNodeId: "n", DstNodeId: "db", Port: 443}),
			resultFiles("n", "jvm-collector", 150, "3.0.2"),
		}
		ordered := make([]*discoveryv1.CollectionResult, len(all))
		for i, p := range perm {
			ordered[i] = all[p]
		}
		writeResults(t, dir, ordered...)
		d := decl.Declaration{Org: "acme", Scope: []string{"n"}, Nodes: []decl.Node{{Name: "n", IPs: []string{"10.0.0.1"}}}}
		r, err := report.Build(dir, d)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range r.Assets {
			if len(a.Sources) > 0 {
				return a.Sources[0].Snapshot.Digest
			}
		}
		t.Fatal("근거가 없다")
		return ""
	}
	want := mk([]int{0, 1, 2})
	rnd := rand.New(rand.NewSource(11))
	for i := 0; i < 6; i++ {
		perm := rnd.Perm(3)
		if got := mk(perm); got != want {
			t.Fatalf("순서 %v 에서 지문이 달라졌다", perm)
		}
	}
}

// ★ IC-R19 — 원천 노드 둘이 선언 노드 하나에 걸리고 같은 자산을 보면, 근거가 **둘 다** 실리고
// 주 근거는 가장 강한 증거다. 전에는 첫 관측만 남아 둘째 원천의 근거가 사라졌다.
func TestAliasedSourcesKeepEveryEvidence(t *testing.T) {
	dir := t.TempDir()
	// 두 원천. 둘째는 symbol-analysis 라 증거가 약하다(inferred). 파일 순서는 약한 쪽이 앞이다.
	weak := resultFiles("web-b.corp", "openssl-collector", 100, "3.0.2")
	weak.Envelope.DetectionMethod = commonv1.DetectionMethod_DETECTION_METHOD_SYMBOL_ANALYSIS
	strong := resultFiles("web-a.corp", "openssl-collector", 200, "3.0.2")
	writeResults(t, dir, weak, strong)

	d := decl.Declaration{Org: "acme", Scope: []string{"web"},
		Nodes:  []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}, ObservedAs: []string{"web-a.corp", "web-b.corp"}}},
		Assets: []decl.Asset{{Node: "web", Runtime: "openssl", Component: "libcrypto"}}}
	r, err := report.Build(dir, d)
	if err != nil {
		t.Fatal(err)
	}
	var web *struct {
		n   int
		src []string
		ev  []string
	}
	for _, a := range r.Assets {
		if a.Key.NodeID == "web" && a.Key.Component == "libcrypto" {
			web = &struct {
				n   int
				src []string
				ev  []string
			}{n: len(a.Sources)}
			for _, s := range a.Sources {
				web.src = append(web.src, s.Snapshot.SourceNodeID)
				web.ev = append(web.ev, s.Evidence)
			}
			if a.FindingID != a.Sources[0].FindingID {
				t.Error("주 근거가 Sources[0] 과 다르다")
			}
		}
	}
	if web == nil || web.n != 2 {
		t.Fatalf("근거가 둘이어야 한다: %+v", web)
	}
	if web.ev[0] != "confirmed" || web.src[0] != "web-a.corp" {
		t.Errorf("주 근거가 가장 강한 증거가 아니다: %v %v", web.ev, web.src)
	}
}

func timeAt(s int64) time.Time { return time.Unix(1700000000+s, 0).UTC() }
