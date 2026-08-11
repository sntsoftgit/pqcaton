package decision

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// Judgment — 한 대상(자산/엣지)에 대한 인간 판정. 재수집으로 관측 상태가 바뀌어도
// 결론(인간 판단)은 대상에 부착돼 남는다(§3.6, §0.3 판단 히스토리 = 상태와 분리).
// append-only로 영속화한다(§0.2) — 갱신 대신 새 판정 레코드를 쌓는다.
type Judgment struct {
	ID         string
	Subject    string  // 판정 대상 키(자산/엣지 동일성). reconcile.AssetKey 문자열화 등.
	Conclusion string  // 인간 결론: "실존-DR" | "제거대상" | "허용(예외)" 등
	Reviewer   string  // 승인자
	Signature  string  // 승인 서명(§3.3③)
	BasisHash  string  // 판정 근거 증거의 해시. 근거가 바뀌면 델타 리뷰 대상(§3.6)
	Confidence float64 // 판정 신뢰도. stale 만료 시 감쇠(IC-D4)
	DecidedAt  int64   // 판정 시각(unix). 테스트·재현성을 위해 호출자가 주입

	// 파생 플래그(영속화 대상 아님 — 델타/만료 계산 결과):
	NeedsReReview bool // 근거 변화 또는 만료로 재확인 필요
	Stale         bool // 만료 경과
}

// HashBasis — 판정 근거가 된 증거 항목들을 정준 순서로 해시한다(BasisHash 산출).
// 항목 집합이 실질적으로 바뀌면 해시가 바뀌어 델타 리뷰가 걸린다(IC-D2/D3).
func HashBasis(items ...string) string {
	s := append([]string(nil), items...)
	sort.Strings(s)
	h := sha256.New()
	for _, it := range s {
		h.Write([]byte(it))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// DeltaReview — 기존 판정들을 최신 근거(currentBasis: subject→BasisHash)와 대조한다.
//   - 근거 실질 변화(해시 다름)  → 해당 판정만 NeedsReReview=true (IC-D2). 결론은 유지(IC-D1).
//   - 근거 불변                 → 그대로 유지, 재리뷰 안 함 (IC-D3).
//   - 최신 근거에 subject 없음   → 관측이 사라졌을 뿐, 결론은 부착 유지 (IC-D1). 플래그 안 함.
//
// 순수 함수 — 입력을 변형하지 않고 판정된 사본을 돌려준다.
func DeltaReview(prior []Judgment, currentBasis map[string]string) []Judgment {
	out := make([]Judgment, len(prior))
	for i, j := range prior {
		nb, ok := currentBasis[j.Subject]
		if ok && nb != j.BasisHash {
			j.NeedsReReview = true // 근거 변화 → 델타 리뷰
		}
		out[i] = j
	}
	return out
}

// ExpireStale — 판정 시각으로부터 ttl(초) 경과한 판정을 stale 처리한다(IC-D4).
// 만료 시 신뢰도를 decay(0~1)만큼 감쇠하고 주기 재확인 플래그를 세운다.
// 순수 함수 — now·ttl·decay를 호출자가 주입해 재현 가능.
func ExpireStale(js []Judgment, now, ttlSeconds int64, decay float64) []Judgment {
	out := make([]Judgment, len(js))
	for i, j := range js {
		if now-j.DecidedAt > ttlSeconds {
			j.Stale = true
			j.NeedsReReview = true
			j.Confidence *= decay
		}
		out[i] = j
	}
	return out
}

// LatestPerSubject — append-only 로그에서 subject별 최신(마지막) 판정만 뽑는다.
// 입력은 판정 순서(오래된→최신) 가정. 델타/만료 계산의 입력으로 쓴다.
func LatestPerSubject(all []Judgment) []Judgment {
	idx := map[string]int{}
	var out []Judgment
	for _, j := range all {
		if pos, ok := idx[j.Subject]; ok {
			out[pos] = j
			continue
		}
		idx[j.Subject] = len(out)
		out = append(out, j)
	}
	return out
}
