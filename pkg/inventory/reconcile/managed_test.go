package reconcile

import (
	"reflect"
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/history"
	"github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"
)

func exc(node, rt, comp, finding, src string, keys ...string) Excluded {
	return Excluded{Key: k(node, rt, comp), Source: ExcludedSource{
		FindingID: finding, Fingerprint: "fp-" + finding, Evidence: "confirmed", SourceNodeID: src, AppKeys: keys}}
}

func recWith(t *testing.T, declared []AssetKey, observed []Observed, excluded []Excluded) []Reconciled {
	t.Helper()
	out, err := eng(t).Reconcile(declared, observed, excluded, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func find(t *testing.T, rs []Reconciled, key AssetKey) Reconciled {
	t.Helper()
	for _, r := range rs {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("%v 가 대조 결과에 없다: %+v", key, rs)
	return Reconciled{}
}

// IC-R20 — 관리 축은 근거의 구성에서 파생한다. 대조 축(State)은 그대로다.
//
//	관리 근거 ≥ 1          → MANAGED. 대표값·신뢰도는 관리 근거에서, 평가됨
//	관측은 있으나 전부 제외 → EXCLUDED_BY_POLICY. 대조 축은 여전히 CONFIRMED(보았다)이고,
//	                         대표 근거는 비며 신뢰도는 미평가. 사람 판정을 요구하지 않는다
//	관리·제외 근거 모두 없음 → UNOBSERVED + NOT_EVALUATED. 신뢰도는 상태 기본값이되 미평가
//
// 전에는 제외된 finding이 정규화 안에서 사라져 선언 자산이 UNOBSERVED로 읽혔다 - 「보았고
// 관리하지 않기로 한 것」을 「보지 못했다」고 말한 것이다.
func TestManagedAxisDerivesFromSources(t *testing.T) {
	managed, excluded, gone := k("n", "openssl", "libssl"), k("n", "openssl", "libcrypto"), k("n", "openssl", "libpq")
	rs := recWith(t, []AssetKey{managed, excluded, gone},
		[]Observed{obs("n", "openssl", "libssl", "confirmed")},
		[]Excluded{exc("n", "openssl", "libcrypto", "f-crypto", "n", "/usr/sbin/sshd")})

	m := find(t, rs, managed)
	if m.State != Confirmed || m.Managed != Managed || !m.ConfidenceEvaluated || m.Confidence <= 0 {
		t.Errorf("관리 근거가 있는 자산: %+v, want CONFIRMED · MANAGED · 평가된 신뢰도", m)
	}

	x := find(t, rs, excluded)
	if x.State != Confirmed {
		t.Errorf("제외 전용 자산의 대조 축 = %s, want CONFIRMED - 보았다는 사실은 정책과 무관하다", x.State)
	}
	if x.Managed != ExcludedByPolicy {
		t.Errorf("제외 전용 자산의 관리 축 = %s, want EXCLUDED_BY_POLICY", x.Managed)
	}
	if x.FindingID != "" || x.Fingerprint != "" || len(x.Sources) != 0 {
		t.Errorf("제외 전용 자산에 대표 근거가 실렸다: %+v - 계약의 호환용 finding_id 로 나가는 값이라 비어야 한다", x)
	}
	if x.ConfidenceEvaluated || x.NeedsReview {
		t.Errorf("제외 전용 자산이 평가됐거나(%t) 판정을 요구한다(%t)", x.ConfidenceEvaluated, x.NeedsReview)
	}
	if len(x.ExcludedSources) != 1 || x.ExcludedSources[0].FindingID != "f-crypto" ||
		!reflect.DeepEqual(x.ExcludedSources[0].AppKeys, []string{"/usr/sbin/sshd"}) {
		t.Errorf("제외 근거가 보존되지 않았다: %+v", x.ExcludedSources)
	}

	g := find(t, rs, gone)
	if g.State != Unobserved || g.Managed != NotEvaluated {
		t.Errorf("관측이 없는 선언 자산: %s · %s, want UNOBSERVED · NOT_EVALUATED", g.State, g.Managed)
	}
	if g.ConfidenceEvaluated {
		t.Error("정책을 걸 finding 이 없었는데 신뢰도가 평가됐다고 적혔다")
	}
	if g.Confidence != ConfidenceFor(Unobserved) || !g.NeedsReview {
		t.Errorf("UNOBSERVED 의 상태 기본값·필수 리뷰가 달라졌다: %+v", g)
	}
}

// IC-R21 — 혼합 근거: 같은 자산을 원천 노드 둘이 봤고 한쪽만 정책에 걸림.
//
// 관리 근거가 하나라도 있으면 MANAGED 다 - 관리할 근거가 있는데 관리하지 않으면 실재하는
// 관리 대상을 놓친다. 대표값과 신뢰도는 **관리 근거에서만** 계산하고, 제외 근거는 따로
// 보존해 리포트가 「관리 근거 n · 제외 근거 m」으로 알린다. 제외 근거의 순서는 입력이 아니라
// 원천 노드·finding 순이다 - 근거 해시가 이 목록을 넣는다.
func TestMixedSourcesAreManagedAndKeepTheExcludedOnes(t *testing.T) {
	key := k("cmdb-01", "openssl", "libcrypto")
	o := obs("cmdb-01", "openssl", "libcrypto", "inferred-low")
	o.FindingID, o.Fingerprint, o.Snapshot.SourceNodeID = "f-b", "fp-b", "host-b"
	rs := recWith(t, []AssetKey{key}, []Observed{o}, []Excluded{
		exc("cmdb-01", "openssl", "libcrypto", "f-z", "host-z", "/usr/bin/python3"),
		exc("cmdb-01", "openssl", "libcrypto", "f-a", "host-a", "/usr/sbin/sshd"),
	})
	r := find(t, rs, key)
	if r.Managed != Managed || r.State != Confirmed {
		t.Fatalf("혼합 근거 = %s · %s, want MANAGED · CONFIRMED", r.Managed, r.State)
	}
	if r.FindingID != "f-b" || !r.ConfidenceEvaluated || r.Confidence != confidence(Confirmed, "inferred-low") {
		t.Errorf("대표값·신뢰도가 관리 근거에서 오지 않았다: %+v", r)
	}
	if len(r.Sources) != 1 || len(r.ExcludedSources) != 2 {
		t.Fatalf("근거 수: 관리 %d · 제외 %d, want 1 · 2", len(r.Sources), len(r.ExcludedSources))
	}
	if r.ExcludedSources[0].SourceNodeID != "host-a" || r.ExcludedSources[1].SourceNodeID != "host-z" {
		t.Errorf("제외 근거가 입력 순서대로다: %+v", r.ExcludedSources)
	}
}

// IC-R22 — 정책 없이 정규화한 스냅샷에서 제외 근거를 뽑는다.
//
// **정책 판정은 상류 코드(scope.AssetPolicy.Managed)가 한다** - 이 리포가 정책을 다시 해석하지
// 않는다. Managed가 거짓인 finding만 나오고, 앱 식별자는 **전부** 실리며(공유 .so를 여러 앱이
// 로드하면 그 목록 전부가 함께 빠진 것이다), 조직이 찍힌다. 정책이 nil 이면 아무것도 빠지지 않는다.
func TestExcludedFromSnapshotUsesTheUpstreamPolicy(t *testing.T) {
	lib := func(id, name string, keys ...string) *discoveryv1.Finding {
		return &discoveryv1.Finding{Id: id, CryptoRuntime: commonv1.CryptoRuntime_CRYPTO_RUNTIME_OPENSSL,
			EvidenceStrength: commonv1.EvidenceStrength_EVIDENCE_STRENGTH_CONFIRMED,
			RuntimeAxes:      &discoveryv1.Finding_Openssl{Openssl: &discoveryv1.OpensslAxes{Lib: name}},
			AppKeys:          keys}
	}
	snap := &history.Snapshot{NodeID: "host-a", Findings: []*discoveryv1.Finding{
		lib("f-ssh", "libcrypto.so.3", "/usr/sbin/sshd", "/usr/bin/python3"),
		lib("f-app", "libssl.so.3", "/opt/app/bin/app"),
	}}
	policy := &scope.AssetPolicy{Rules: []scope.AssetRule{{Runtime: "*", Lib: "*", AppKey: "/usr/sbin/sshd*", Exclude: true}}}

	got := eng(t).ExcludedFromSnapshotAs(snap, "cmdb-a", policy)
	if len(got) != 1 {
		t.Fatalf("제외 근거 %d개, want 1: %+v", len(got), got)
	}
	x := got[0]
	if x.Key != (AssetKey{Org: testOrg, NodeID: "cmdb-a", Runtime: "openssl", Component: "libcrypto"}) {
		t.Errorf("열쇠 = %+v: 선언 노드 이름으로, 조직을 찍어", x.Key)
	}
	if x.Source.SourceNodeID != "host-a" || x.Source.FindingID != "f-ssh" {
		t.Errorf("원천·finding = %s · %s", x.Source.SourceNodeID, x.Source.FindingID)
	}
	if !reflect.DeepEqual(x.Source.AppKeys, []string{"/usr/sbin/sshd", "/usr/bin/python3"}) {
		t.Errorf("앱 열쇠 전부가 실려야 한다: %v", x.Source.AppKeys)
	}
	if n := len(eng(t).ExcludedFromSnapshotAs(snap, "cmdb-a", nil)); n != 0 {
		t.Errorf("정책이 없는데 %d개가 빠졌다", n)
	}
}

// IC-Q8 — 큐는 관리 축을 신뢰도보다 먼저 본다.
//
// 제외 전용은 신뢰도가 미평가라 0.8 비교에 넣으면 0으로 읽혀 필수 리뷰에 올라간다 - 관리하지
// 않기로 한 자산을 판정하라고 올리는 것이다. 큐에도 자동통과에도 넣지 않는다. 혼합은 관리
// 근거의 신뢰도로 가고, NOT_EVALUATED + UNOBSERVED는 지금대로 필수 리뷰다.
func TestQueueLooksAtTheManagedAxisFirst(t *testing.T) {
	recs := []Reconciled{
		{Key: k("n", "openssl", "excluded"), State: Confirmed, Managed: ExcludedByPolicy},
		{Key: k("n", "openssl", "mixed"), State: Confirmed, Managed: Managed, Confidence: 0.9, ConfidenceEvaluated: true,
			ExcludedSources: []ExcludedSource{{FindingID: "f-x"}}},
		{Key: k("n", "openssl", "gone"), State: Unobserved, Managed: NotEvaluated, Confidence: 0.3, NeedsReview: true},
	}
	autopass, review := BuildReviewQueue(recs)
	for _, a := range autopass {
		if a.Managed == ExcludedByPolicy {
			t.Errorf("제외 전용이 자동통과에 올랐다: %+v", a.Key)
		}
	}
	for _, it := range review {
		if it.Rec.Managed == ExcludedByPolicy {
			t.Errorf("제외 전용이 리뷰 큐에 올랐다: %+v", it.Rec.Key)
		}
	}
	if len(autopass) != 1 || autopass[0].Key.Component != "mixed" {
		t.Errorf("혼합 근거는 관리 신뢰도로 자동통과여야: %+v", autopass)
	}
	if len(review) != 1 || review[0].Rec.Key.Component != "gone" || !review[0].Mandatory {
		t.Errorf("NOT_EVALUATED + UNOBSERVED 는 필수 리뷰여야: %+v", review)
	}
}
