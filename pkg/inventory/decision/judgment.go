package decision

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// RecordKind — 원장 행의 종류. **판정과 계획 선택은 같은 append-only 자리에 쌓이되 다른 사실이다.**
//
// 「이 자산을 이렇게 판정했다」와 「이 자산을 이 계획에 넣기로 했다」는 같은 세션에서 나더라도
// 따로 물을 수 있어야 한다. 자동통과 항목은 결론 없이 계획에 들어가므로 판정 행이 없고, 계획 선택
// 행이 없으면 실행 계획에는 있는데 원장에는 아무것도 없는 조치가 생긴다.
//
// **파생 함수(최신·델타·만료)는 판정 행만 본다.** 결론이 빈 계획 선택 행이 사람의 판정을 덮으면
// 안 되고, 특히 「이 대상에 판정이 아예 없다」는 경고(scope.Review)가 사라지면 안 된다.
type RecordKind string

const (
	// RecordJudgment — 사람이 내린 판정. 옛 행은 이 칸이 비어 있고 판정으로 읽는다. 소급하지 않는다.
	RecordJudgment RecordKind = "judgment"
	// RecordPlanSelection — 계획에 넣기로 한 선택. 결론 칸은 비어 있다. Reviewer·Signature 는 세션에
	// 적힌 **자유 문자열**이라, 이 행이 답하는 것은 「누가 넣었나」가 아니라 「누가 넣었다고 기록됐나」다.
	// 검증된 신원은 상류의 실행 승인에만 있다.
	RecordPlanSelection RecordKind = "plan-selection"
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
	// ConfidenceEvaluated — Confidence 가 잰 값인가. 거짓이면 상태 기본값 같은 것이라 만료돼도
	// 감쇠하지 않는다 - 재지 않은 값을 줄이면 「재 봤더니 더 낮아졌다」로 읽힌다. **옛 행은 칸이
	// 없고 전부 평가된 값이다** - 그때는 미평가라는 개념이 없었다. 저장소가 부재를 참으로 읽는다.
	ConfidenceEvaluated bool
	// RecordKind — 이 행의 종류. 빈 값은 판정이다(옛 행).
	RecordKind RecordKind
	DecidedAt  int64 // 판정 시각(unix). 테스트·재현성을 위해 호출자가 주입
	// SessionID — 이 판정이 난 리뷰 세션. **계획과 원장을 잇는 열쇠다.** 계약으로 나가는 계획의
	// id 가 이 값을 담으므로, 계획에서 이 값을 읽어 원장에서 그 세션의 판정들을 찾는다
	// ([JudgmentStore.BySessionID]). 저장만 하고 찾는 길이 없으면 「원장에서 찾을 수 있다」가
	// 기능이 아니라 가능성에 그친다. 옛 행은 비어 있고 소급하지 않는다 — 어느 세션에서 난
	// 판정인지 도구가 알 수 없다.
	SessionID string

	// 파생 플래그(영속화 대상 아님 — 델타/만료 계산 결과):
	NeedsReReview bool // 근거 변화 또는 만료로 재확인 필요
	Stale         bool // 만료 경과
}

// Kind — 이 행의 종류. 옛 행(빈 값)은 판정이다.
func (j Judgment) Kind() RecordKind {
	if j.RecordKind == "" {
		return RecordJudgment
	}
	return j.RecordKind
}

// JudgmentsOnly — 판정 행만. 파생 함수가 입력에 쓴다. **거르는 자리를 파생 함수 안에 둔다** -
// 부르는 쪽에 맡기면 부르는 자리가 여섯이라 하나는 반드시 빠진다.
func JudgmentsOnly(all []Judgment) []Judgment {
	out := make([]Judgment, 0, len(all))
	for _, j := range all {
		if j.Kind() == RecordJudgment {
			out = append(out, j)
		}
	}
	return out
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
//
// 판정 행만 본다([JudgmentsOnly]). 계획 선택 행에 재검토 표시를 붙이면 판정이 아닌 것이 재판정
// 대상으로 보인다.
func DeltaReview(prior []Judgment, currentBasis map[string]string) []Judgment {
	prior = JudgmentsOnly(prior)
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
//
// 판정 행만 본다. **미평가 판정은 재검토 표시만 세우고 숫자는 건드리지 않는다** - 재지 않은 값을
// 줄이면 「재 봤더니 더 낮아졌다」로 읽힌다.
func ExpireStale(js []Judgment, now, ttlSeconds int64, decay float64) []Judgment {
	js = JudgmentsOnly(js)
	out := make([]Judgment, len(js))
	for i, j := range js {
		if now-j.DecidedAt > ttlSeconds {
			j.Stale = true
			j.NeedsReReview = true
			if j.ConfidenceEvaluated {
				j.Confidence *= decay
			}
		}
		out[i] = j
	}
	return out
}

// LatestPerSubject — append-only 로그에서 subject별 최신(마지막) 판정만 뽑는다.
// 입력은 판정 순서(오래된→최신) 가정. 델타/만료 계산의 입력으로 쓴다.
//
// 판정 행만 본다. 종류를 보지 않고 subject 로 덮으면 결론이 빈 계획 선택 행이 사람의 판정을 덮고,
// 「이 대상에 판정이 아예 없다」는 경고가 사라진다.
func LatestPerSubject(all []Judgment) []Judgment {
	idx := map[string]int{}
	var out []Judgment
	for _, j := range JudgmentsOnly(all) {
		if pos, ok := idx[j.Subject]; ok {
			out[pos] = j
			continue
		}
		idx[j.Subject] = len(out)
		out = append(out, j)
	}
	return out
}
