package reconcile

import (
	"strings"

	commonv1 "github.com/pqcota/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/pqcota/pqcota/gen/pqcota/discovery/v1"
	"github.com/pqcota/pqcota/pkg/discovery/history"
	"github.com/pqcota/pqcota/pkg/discovery/normalize"
)

// AssetsFromSnapshot — 디스커버리 스냅샷(관측 레인)의 Finding에서 Observed(자산+증거강도)를 뽑는다.
func AssetsFromSnapshot(snap *history.Snapshot) []Observed {
	if snap == nil {
		return nil
	}
	return observedFrom(snap.NodeID, snap.Findings)
}

// GapLayers — 스냅샷 완전성 맵의 미커버 계층(문자열)을 뽑는다. UNOBSERVED 재수집 판정 입력(IC-R4).
func GapLayers(snap *history.Snapshot) []string {
	if snap == nil || snap.Completeness == nil {
		return nil
	}
	var out []string
	for _, l := range snap.Completeness.GetLayersMissing() {
		out = append(out, l.String())
	}
	return out
}

// AssetsFromResults — CollectionResult들(선언 레인)을 Finding으로 파생해 선언 AssetKey를 뽑는다.
func AssetsFromResults(results []*discoveryv1.CollectionResult) ([]AssetKey, error) {
	var out []AssetKey
	for _, res := range results {
		fs, err := normalize.DeriveFindings(res, "", "")
		if err != nil {
			return nil, err
		}
		for _, o := range observedFrom(res.GetEnvelope().GetTargetNodeId(), fs) {
			out = append(out, o.Key)
		}
	}
	return out, nil
}

func observedFrom(node string, findings []*discoveryv1.Finding) []Observed {
	var out []Observed
	for _, f := range findings {
		var rt, comp string
		switch f.GetCryptoRuntime() {
		case commonv1.CryptoRuntime_CRYPTO_RUNTIME_OPENSSL:
			rt = "openssl"
			comp = normalizeComponent(f.GetOpenssl().GetLib())
		case commonv1.CryptoRuntime_CRYPTO_RUNTIME_JCA:
			rt = "jca"
			comp = "jca-provider-chain"
		default:
			continue
		}
		out = append(out, Observed{
			Key:      AssetKey{NodeID: node, Runtime: rt, Component: comp},
			Evidence: evidenceStr(f.GetEvidenceStrength()),
		})
	}
	return out
}

func evidenceStr(e commonv1.EvidenceStrength) string {
	switch e {
	case commonv1.EvidenceStrength_EVIDENCE_STRENGTH_CONFIRMED:
		return "confirmed"
	case commonv1.EvidenceStrength_EVIDENCE_STRENGTH_INFERRED_HIGH:
		return "inferred-high"
	case commonv1.EvidenceStrength_EVIDENCE_STRENGTH_INFERRED_LOW:
		return "inferred-low"
	default:
		return ""
	}
}

// normalizeComponent — 파일명에서 버전 접미사를 떼어 동일성을 맞춘다(선언 "libssl" ↔ 관측 "libssl.so.3").
// 단 벤더링 해시는 유지 → "libcrypto-fbc9a285.so.3" → "libcrypto-fbc9a285"(선언 "libcrypto"와 불일치=shadow).
func normalizeComponent(name string) string {
	if i := strings.Index(name, ".so"); i > 0 {
		return name[:i]
	}
	return name
}
