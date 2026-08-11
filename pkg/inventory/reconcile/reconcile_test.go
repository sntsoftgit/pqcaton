package reconcile

import "testing"

func k(node, rt, comp string) AssetKey { return AssetKey{NodeID: node, Runtime: rt, Component: comp} }
func obs(node, rt, comp, ev string) Observed {
	return Observed{Key: k(node, rt, comp), Evidence: ev}
}

// 3-상태 대조(§3.3): 선언∩관측=CONFIRMED, 관측only=UNDECLARED, 선언only=UNOBSERVED.
func TestReconcile(t *testing.T) {
	declared := []AssetKey{k("n1", "openssl", "libssl"), k("n1", "openssl", "libcrypto")}
	observed := []Observed{obs("n1", "openssl", "libssl", "confirmed"), obs("n1", "openssl", "libpq-shadow", "confirmed")}

	got := map[AssetKey]State{}
	for _, r := range Reconcile(declared, observed, nil) {
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
	for _, r := range Reconcile([]AssetKey{k("n", "jca", "only-decl")}, []Observed{obs("n", "jca", "only-obs", "confirmed")}, nil) {
		if !r.NeedsReview {
			t.Errorf("%s은 NeedsReview여야: %v", r.State, r.Key)
		}
	}
}

// IC-R4: UNOBSERVED + 완전성 갭 → "재수집 후보"(갭이면 미관측일 뿐). 갭 없으면 실존/stale 사람 판정.
func TestReconcile_unobservedGap(t *testing.T) {
	declared := []AssetKey{k("n", "openssl", "libgone")}

	withGap := Reconcile(declared, nil, []string{"COLLECTION_LAYER_ARTIFACT"})
	if len(withGap) != 1 || withGap[0].State != Unobserved {
		t.Fatalf("want 1 UNOBSERVED, got %+v", withGap)
	}
	if !withGap[0].RescanCandidate {
		t.Error("완전성 갭 있는 UNOBSERVED는 재수집 후보여야")
	}

	noGap := Reconcile(declared, nil, nil)
	if noGap[0].RescanCandidate {
		t.Error("갭 없으면 재수집 후보 아님(실존/stale 사람 판정)")
	}
}

// IC-C2: 관측 evidence_strength=inferred-low는 confidence 상한을 누른다(불확실 관측은 신뢰 낮춤).
func TestReconcile_evidenceConfidence(t *testing.T) {
	decl := []AssetKey{k("n", "openssl", "lib")}
	hi := Reconcile(decl, []Observed{obs("n", "openssl", "lib", "confirmed")}, nil)[0].Confidence
	lo := Reconcile(decl, []Observed{obs("n", "openssl", "lib", "inferred-low")}, nil)[0].Confidence
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
