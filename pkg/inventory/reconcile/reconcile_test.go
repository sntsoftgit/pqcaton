package reconcile

import (
	"errors"
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
)

// testOrg — 케이스가 쓰는 조직. 대조는 조직에 묶이므로 열쇠에도 엔진에도 같은 값이 든다.
const testOrg = org.ID("acme")

func k(node, rt, comp string) AssetKey {
	return AssetKey{Org: testOrg, NodeID: node, Runtime: rt, Component: comp}
}

func eng(t *testing.T) *Engine {
	t.Helper()
	e, err := For(testOrg)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// rec — 대조 결과만 보는 케이스용. 조직이 어긋나는 쪽은 IC-O1·R6이 따로 본다.
func rec(t *testing.T, declared []AssetKey, observed []Observed, gaps []string) []Reconciled {
	t.Helper()
	out, err := eng(t).Reconcile(declared, observed, gaps)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func obs(node, rt, comp, ev string) Observed {
	return Observed{Key: k(node, rt, comp), Evidence: ev}
}

// 3-상태 대조(§3.3): 선언∩관측=CONFIRMED, 관측only=UNDECLARED, 선언only=UNOBSERVED.
func TestReconcile(t *testing.T) {
	declared := []AssetKey{k("n1", "openssl", "libssl"), k("n1", "openssl", "libcrypto")}
	observed := []Observed{obs("n1", "openssl", "libssl", "confirmed"), obs("n1", "openssl", "libpq-shadow", "confirmed")}

	got := map[AssetKey]State{}
	for _, r := range rec(t, declared, observed, nil) {
		got[r.Key] = r.State
	}
	cases := map[AssetKey]State{
		k("n1", "openssl", "libssl"):       Confirmed,  // 양쪽
		k("n1", "openssl", "libpq-shadow"): Undeclared, // 관측 only = shadow
		k("n1", "openssl", "libcrypto"):    Unobserved, // 선언 only
	}
	if len(got) != len(cases) {
		t.Fatalf("결과 %d개, want %d: %+v", len(got), len(cases), got)
	}
	for key, want := range cases {
		if got[key] != want {
			t.Errorf("%v = %s, want %s", key, got[key], want)
		}
	}
}

// UNDECLARED·UNOBSERVED은 사람 판정 필수(§3.5 MANUAL).
func TestReconcile_needsReview(t *testing.T) {
	for _, r := range rec(t, []AssetKey{k("n", "jca", "only-decl")}, []Observed{obs("n", "jca", "only-obs", "confirmed")}, nil) {
		if !r.NeedsReview {
			t.Errorf("%s은 NeedsReview여야: %v", r.State, r.Key)
		}
	}
}

// IC-R4: UNOBSERVED + 완전성 갭 → "재수집 후보"(갭이면 미관측일 뿐). 갭 없으면 실존/stale 사람 판정.
func TestReconcile_unobservedGap(t *testing.T) {
	declared := []AssetKey{k("n", "openssl", "libgone")}

	withGap := rec(t, declared, nil, []string{"COLLECTION_LAYER_ARTIFACT"})
	if len(withGap) != 1 || withGap[0].State != Unobserved {
		t.Fatalf("want 1 UNOBSERVED, got %+v", withGap)
	}
	if !withGap[0].RescanCandidate {
		t.Error("완전성 갭 있는 UNOBSERVED는 재수집 후보여야")
	}

	noGap := rec(t, declared, nil, nil)
	if noGap[0].RescanCandidate {
		t.Error("갭 없으면 재수집 후보 아님(실존/stale 사람 판정)")
	}
}

// IC-C2: 관측 evidence_strength=inferred-low는 confidence 상한을 누른다(불확실 관측은 신뢰 낮춤).
func TestReconcile_evidenceConfidence(t *testing.T) {
	decl := []AssetKey{k("n", "openssl", "lib")}
	hi := rec(t, decl, []Observed{obs("n", "openssl", "lib", "confirmed")}, nil)[0].Confidence
	lo := rec(t, decl, []Observed{obs("n", "openssl", "lib", "inferred-low")}, nil)[0].Confidence
	if !(lo < hi) {
		t.Errorf("inferred-low confidence(%.2f)가 confirmed(%.2f)보다 낮아야", lo, hi)
	}
}

// 리뷰 큐(§3.3②): CONFIRMED 고신뢰=자동통과, shadow=최우선 필수리뷰.
func TestBuildReviewQueue(t *testing.T) {
	recs := []Reconciled{
		{Key: k("n", "openssl", "sys"), State: Confirmed, Confidence: 0.9},
		{Key: k("n", "openssl", "shadow"), State: Undeclared, Confidence: 0.6, NeedsReview: true},
		{Key: k("n", "openssl", "gone"), State: Unobserved, Confidence: 0.3, NeedsReview: true},
	}
	autopass, review := BuildReviewQueue(recs)

	if len(autopass) != 1 || autopass[0].State != Confirmed {
		t.Errorf("autopass = %+v, want 1 CONFIRMED", autopass)
	}
	if len(review) != 2 {
		t.Fatalf("review 큐 = %d, want 2", len(review))
	}
	// shadow(UNDECLARED)가 우선순위 최상단.
	if review[0].Rec.State != Undeclared {
		t.Errorf("리뷰 큐 최상단 = %s, want UNDECLARED(shadow 최우선)", review[0].Rec.State)
	}
	if !review[0].Mandatory {
		t.Error("shadow는 필수 리뷰여야")
	}
}

// IC-O1 — **다른 조직의 자산이 섞이면 대조하지 않는다.**
//
// 그냥 두면 오류가 아니라 그럴듯한 결과가 나온다 — 열쇠가 안 맞아 같은 자산이 UNDECLARED와
// UNOBSERVED 한 쌍으로 갈리고, 리뷰 큐는 그것을 shadow 발견으로 올린다.
func TestReconcileRefusesAnotherOrg(t *testing.T) {
	남 := AssetKey{Org: org.ID("beta"), NodeID: "n", Runtime: "openssl", Component: "libssl"}

	if _, err := eng(t).Reconcile([]AssetKey{남}, nil, nil); !errors.Is(err, ErrOrgMismatch) {
		t.Fatalf("선언 레인: 남의 조직을 그대로 대조했다: %v", err)
	}
	obs := []Observed{{Key: 남, Evidence: "confirmed"}}
	if _, err := eng(t).Reconcile(nil, obs, nil); !errors.Is(err, ErrOrgMismatch) {
		t.Fatalf("관측 레인: 남의 조직을 그대로 대조했다: %v", err)
	}
}

// IC-O2 — **조직 없는 열쇠도 끊는다.** 비면 「아무 조직」이 아니라 「모른다」이고, 모르는
// 것을 이 엔진의 조직으로 지어내면 검사가 있으나 마나다.
func TestReconcileRefusesEmptyOrg(t *testing.T) {
	빈 := AssetKey{NodeID: "n", Runtime: "openssl", Component: "libssl"}
	if _, err := eng(t).Reconcile([]AssetKey{빈}, nil, nil); !errors.Is(err, ErrOrgMismatch) {
		t.Fatalf("조직 없는 열쇠를 그대로 대조했다: %v", err)
	}
	if _, err := For(""); err == nil {
		t.Fatal("빈 조직으로 엔진이 열렸다")
	}
}

// IC-O3 — **엔진이 조직을 찍는다.** 스냅샷에도 계약에도 조직이 없으니, 찍는 자리가 하나가
// 아니면 조직 없는 열쇠가 어딘가에서 만들어진다.
func TestEngineStampsOrg(t *testing.T) {
	in := []Observed{{Key: AssetKey{NodeID: "n", Runtime: "openssl", Component: "libssl"}}}
	for _, o := range stampObserved(testOrg, in) {
		if o.Key.Org != testOrg {
			t.Errorf("조직이 %q다, want %q", o.Key.Org, testOrg)
		}
	}
}

// IC-R16 — **CNG 관측도 자산이 된다**(상류 v0.6.0).
//
// 런타임 갈래를 안 더하면 그 관측은 조용히 버려집니다. Windows 노드의 암호 자산이
// 인벤토리에서 통째로 사라지고, 화면은 「없다」와 같은 얼굴로 그것을 보여 줍니다 —
// 이 도구가 막으려는 바로 그 자리입니다(§2.6).
//
// **모르는 런타임은 그대로 버린다.** 이름을 지어내면 선언과 영영 맞지 않는 자산이
// 인벤토리에 생긴다 — 상류가 새 런타임을 내면 여기에 갈래를 더하는 것이 그 답이다.
func TestObservedFromTakesCNG(t *testing.T) {
	findings := []*discoveryv1.Finding{
		{CryptoRuntime: commonv1.CryptoRuntime_CRYPTO_RUNTIME_WIN_CNG,
			EvidenceStrength: commonv1.EvidenceStrength_EVIDENCE_STRENGTH_CONFIRMED,
			RuntimeAxes: &discoveryv1.Finding_Cng{Cng: &discoveryv1.CngAxes{
				ProviderSet: []string{"Microsoft Primitive Provider"}}}},
		{CryptoRuntime: commonv1.CryptoRuntime_CRYPTO_RUNTIME_UNSPECIFIED},
	}
	got := observedFrom("win-01", findings)
	if len(got) != 1 {
		t.Fatalf("관측 %d개 — CNG 를 버렸거나 모르는 런타임을 주웠다: %+v", len(got), got)
	}
	if got[0].Key.Runtime != RuntimeCNG || got[0].Key.Component != ComponentCNG {
		t.Errorf("CNG 자산의 이름이 다르다: %+v", got[0].Key)
	}
	if got[0].Evidence != "confirmed" {
		t.Errorf("증거 강도가 안 붙었다: %q", got[0].Evidence)
	}
	// 화면이 고르게 하는 이름과 관측 결과에 나오는 이름은 한 목록이어야 한다.
	var hasCNG bool
	for _, rt := range Runtimes() {
		if rt == RuntimeCNG {
			hasCNG = true
		}
	}
	if !hasCNG {
		t.Errorf("관측은 %q 를 내는데 고를 수 있는 이름에 없다: %v", RuntimeCNG, Runtimes())
	}
}
