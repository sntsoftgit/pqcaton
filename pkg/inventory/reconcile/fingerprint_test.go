package reconcile

import (
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
)

// 지문은 **동일성이 아니라 내용**을 잰다.
//
// 상류의 `Finding.Id` 는 `sha256(노드|이름|런타임|fork)` 라 자산이 같으면 같다. 그것만 보면
// 버전이 오르고 검출 방법이 바뀌고 강화 판정이 달라져도 「그대로」로 읽힌다.

func finding() *discoveryv1.Finding {
	return &discoveryv1.Finding{
		Id:               "같은-자산-같은-id",
		CryptoRuntime:    commonv1.CryptoRuntime_CRYPTO_RUNTIME_OPENSSL,
		Algorithm:        "X25519",
		DetectionMethod:  commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION,
		EvidenceStrength: commonv1.EvidenceStrength_EVIDENCE_STRENGTH_CONFIRMED,
		PqcReadiness:     "not-ready",
		RemediationClass: "provider-inject",
		RuntimeAxes: &discoveryv1.Finding_Openssl{Openssl: &discoveryv1.OpensslAxes{
			Lib: "libcrypto.so.3", Version: "3.0.2"}},
	}
}

// IC-R5 — 같은 관측을 다시 읽으면 같은 지문이다. 흔들리면 델타 큐가 매번 가득 찬다(IC-D3).
func TestSameFindingSameFingerprint(t *testing.T) {
	if Fingerprint(finding()) != Fingerprint(finding()) {
		t.Error("같은 관측인데 지문이 달라졌다")
	}
}

// IC-R5 — ★ 재수집마다 달라지는 것은 근거가 아니다.
//
// `derived_from_snapshot_id` 는 돌릴 때마다 바뀐다. 지문에 넣으면 아무것도 안 바뀐 관측이
// 매번 「근거가 바뀌었다」로 올라오고, 그런 큐는 아무도 읽지 않는다. 규칙 판은 세션이 따로
// 들고 가므로 여기 넣으면 두 번 센다.
func TestRescanningAloneDoesNotMoveTheFingerprint(t *testing.T) {
	a, b := finding(), finding()
	a.DerivedFromSnapshotId, b.DerivedFromSnapshotId = "snap-1", "snap-2"
	a.RulesetVersion, b.RulesetVersion = "pqcota-enrich/v1", "pqcota-enrich/v2"
	if Fingerprint(a) != Fingerprint(b) {
		t.Error("스냅샷 id 나 규칙 판이 지문을 흔든다")
	}
}

// IC-R6 — id 가 같아도 내용이 달라졌으면 다른 근거다 — 이 검사가 있어야 하는 이유다.
func TestSameIDButDifferentContentIsADifferentBasis(t *testing.T) {
	was := Fingerprint(finding())
	for _, tc := range []struct {
		what string
		f    *discoveryv1.Finding
	}{
		{"라이브러리 버전", func() *discoveryv1.Finding {
			f := finding()
			f.GetOpenssl().Version = "3.5.0"
			return f
		}()},
		{"검출 방법", func() *discoveryv1.Finding {
			f := finding()
			f.DetectionMethod = commonv1.DetectionMethod_DETECTION_METHOD_SYMBOL_ANALYSIS
			return f
		}()},
		{"증거 강도", func() *discoveryv1.Finding {
			f := finding()
			f.EvidenceStrength = commonv1.EvidenceStrength_EVIDENCE_STRENGTH_INFERRED_LOW
			return f
		}()},
		{"강화가 낸 성숙도", func() *discoveryv1.Finding {
			f := finding()
			f.PqcReadiness = "ready"
			return f
		}()},
		{"조치 분류", func() *discoveryv1.Finding {
			f := finding()
			f.RemediationClass = "config-only"
			return f
		}()},
		{"알고리즘", func() *discoveryv1.Finding {
			f := finding()
			f.Algorithm = "ML-KEM-768"
			return f
		}()},
		{"이 자산을 로드한 앱", func() *discoveryv1.Finding {
			f := finding()
			f.AppKeys = []string{"nginx"}
			return f
		}()},
	} {
		if tc.f.GetId() != finding().GetId() {
			t.Fatalf("%s: 이 검사는 id 가 같을 때를 잰다", tc.what)
		}
		if Fingerprint(tc.f) == was {
			t.Errorf("%s가 바뀌었는데 지문이 그대로다 — 판정 근거가 조용히 갈린다", tc.what)
		}
	}
}

// IC-R5 — 관측이 없으면 지문도 없다. UNOBSERVED 는 대조 상태가 이미 그 사실을 말한다.
func TestNoFindingNoFingerprint(t *testing.T) {
	if Fingerprint(nil) != "" {
		t.Error("없는 관측에 지문이 붙었다")
	}
}

// IC-R6 — 대조가 지문을 들고 간다. 여기서 떨어뜨리면 세션까지 오지 못해 근거 비교가 도로 id 뿐이 된다.
func TestReconciledCarriesTheFingerprint(t *testing.T) {
	got := observedFrom("n1", []*discoveryv1.Finding{finding()})
	if len(got) != 1 || got[0].Fingerprint == "" {
		t.Fatalf("관측에 지문이 안 붙었다: %+v", got)
	}
	rec := reconcileAssets(nil, got, nil)
	if len(rec) != 1 || rec[0].Fingerprint != got[0].Fingerprint {
		t.Errorf("대조가 지문을 떨어뜨렸다: %+v", rec)
	}
}
