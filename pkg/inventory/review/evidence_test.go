package review_test

import (
	"testing"

	provisioningv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/history"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// 근거 여럿이 계약까지 간다. 주 근거가 앞이고, 참조는 내용 지문이며, 보조 근거가 바뀌면 근거 해시가
// 달라진다.

const dg = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func withSources() review.Item {
	it := base()
	it.Sources = []review.EvidenceSource{
		{FindingID: "f-a", Fingerprint: "fp-a", Evidence: "confirmed", SourceNodeID: "web-a.corp", SnapshotDigest: dg, SnapshotRuleset: "pqcota-enrich/v2"},
		{FindingID: "f-b", Fingerprint: "fp-b", Evidence: "inferred-low", SourceNodeID: "web-b.corp", SnapshotDigest: dg, SnapshotRuleset: "pqcota-enrich/v2"},
	}
	it.FindingID, it.Fingerprint = "f-a", "fp-a"
	return it
}

// ★ IC-P10 — 근거가 계약의 evidence_sources 로 나간다. 주 근거가 앞이고 finding_id(호환)와 같다.
// 참조는 내용 지문이고 원천 노드·규칙 판을 든다.
func TestEvidenceReachesTheContract(t *testing.T) {
	it := withSources()
	it.Plan, it.Level, it.Kind, it.TargetAlgorithm, it.Node, it.Runtime = true, "L2", "REMEDIATION_KIND_CONFIG_ONLY", "ML-KEM (FIPS 203)", "web", "openssl"
	sf := judgedSession()
	sf.Items = []review.Item{it}
	res, err := review.Finalize(sf)
	if err != nil {
		t.Fatal(err)
	}
	a := res.Plan.GetActions()[0]
	if len(a.GetEvidenceSources()) != 2 {
		t.Fatalf("근거가 둘이어야 한다: %d", len(a.GetEvidenceSources()))
	}
	if a.GetFindingId() != a.GetEvidenceSources()[0].GetFindingId() {
		t.Error("호환 finding_id 가 주 근거와 다르다")
	}
	ref := a.GetEvidenceSources()[1].GetSnapshot()
	if ref.GetSourceNodeId() != "web-b.corp" {
		t.Errorf("원천 노드가 아니다: %q", ref.GetSourceNodeId())
	}
	c, ok := ref.GetReference().(*provisioningv1.SnapshotReference_Content)
	if !ok || c.Content.GetFormatVersion() != history.SnapshotContentFormatV1 || c.Content.GetDigest() != dg || c.Content.GetRulesetVersion() != "pqcota-enrich/v2" {
		t.Errorf("내용 참조가 아니거나 셋 중 하나가 빠졌다: %+v", ref.GetReference())
	}
}

// IC-P10 — 보조 근거가 더해지거나 빠지거나 바뀌면 근거 해시가 달라진다. 주 근거만 보면 판정 서명이
// 보조 근거의 변화를 지나친다.
func TestSecondaryEvidenceMovesTheBasis(t *testing.T) {
	it := withSources()
	was := review.BasisOf(it, rules)
	only := withSources()
	only.Sources = only.Sources[:1]
	if review.BasisOf(only, rules) == was {
		t.Error("보조 근거가 빠졌는데 근거 해시가 그대로다")
	}
	changed := withSources()
	changed.Sources[1].Fingerprint = "fp-b2"
	if review.BasisOf(changed, rules) == was {
		t.Error("보조 근거의 지문이 바뀌었는데 근거 해시가 그대로다")
	}
	// 순서는 근거가 아니다 — 같은 묶음이면 같다.
	swapped := withSources()
	swapped.Sources[0], swapped.Sources[1] = swapped.Sources[1], swapped.Sources[0]
	if review.BasisOf(swapped, rules) != was {
		t.Error("근거의 순서에 근거 해시가 흔들린다")
	}
	// 스냅샷 지문은 근거가 아니다 — 바뀌어도 그대로다.
	moved := withSources()
	moved.Sources[0].SnapshotDigest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if review.BasisOf(moved, rules) != was {
		t.Error("스냅샷 지문이 근거 해시에 들어갔다 — 위치이지 근거가 아니다")
	}
}

// IC-P10 — 지문이 비면(옛 세션) 참조 없이 finding 만 낸다. 상류가 모양이 틀렸다고 알린다 — 조용히
// 빼지 않는다.
func TestEmptyDigestStillNamesTheFinding(t *testing.T) {
	it := withSources()
	it.Sources[1].SnapshotDigest = ""
	it.Plan, it.Level, it.Kind, it.TargetAlgorithm, it.Node, it.Runtime = true, "L2", "REMEDIATION_KIND_CONFIG_ONLY", "ML-KEM (FIPS 203)", "web", "openssl"
	sf := judgedSession()
	sf.Items = []review.Item{it}
	res, err := review.Finalize(sf)
	if err != nil {
		t.Fatal(err)
	}
	e := res.Plan.GetActions()[0].GetEvidenceSources()[1]
	if e.GetFindingId() != "f-b" || e.GetSnapshot() != nil {
		t.Errorf("지문 없는 근거를 조용히 뺐거나 참조를 지어냈다: %+v", e)
	}
}
