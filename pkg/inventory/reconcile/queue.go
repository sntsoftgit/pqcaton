package reconcile

import "sort"

// ReviewItem — 리뷰 큐 항목(§3.3②). 대조 엔진은 판정 대상을 구조화·우선순위화한다.
type ReviewItem struct {
	Rec       Reconciled
	Priority  int  // 높을수록 먼저. 위험도×영향 범위×데이터민감도의 프록시(§3.3② — 실측 모델은 후속)
	Mandatory bool // 필수 개별 리뷰 대상
}

// BuildReviewQueue — 대조 결과를 (자동통과 후보, 필수 리뷰 큐)로 나눈다(§3.3②, §3.5 PROPOSE).
//   - 자동통과 후보: CONFIRMED + 고신뢰 → 일괄 승인 제안(승인은 사람).
//   - 필수 개별 리뷰: UNDECLARED(최우선), UNOBSERVED, 저신뢰 CONFIRMED.
//
// 반환 큐는 우선순위 내림차순 정렬. 확정(finalize)은 여기서 하지 않는다 — Decision 서비스 소관.
//
// **관리 축을 신뢰도보다 먼저 본다.** 제외 전용(EXCLUDED_BY_POLICY)은 신뢰도가 미평가라
// 아래 0.8 비교에 넣으면 0으로 읽혀 필수 리뷰에 올라간다 - 관리하지 않기로 한 자산을 사람이
// 판정하라고 올리는 것이다. 큐에 넣지 않고 리포트에 남긴다(선언과 어긋나면 경고는 따로 난다).
// 혼합(MANAGED + 제외 근거)은 신뢰도가 이미 관리 근거에서만 계산됐으므로 그대로 간다.
// NOT_EVALUATED + UNOBSERVED는 지금대로 필수 리뷰다 - 「선언했는데 못 봤다」는 여전히 사람의 일이다.
func BuildReviewQueue(recs []Reconciled) (autopass []Reconciled, review []ReviewItem) {
	for _, r := range recs {
		if r.Managed == ExcludedByPolicy {
			continue
		}
		switch r.State {
		case Confirmed:
			if r.Confidence >= 0.8 {
				autopass = append(autopass, r) // 일괄 승인 묶음 후보
			} else {
				review = append(review, ReviewItem{Rec: r, Priority: 1, Mandatory: false})
			}
		case Undeclared:
			review = append(review, ReviewItem{Rec: r, Priority: 3, Mandatory: true}) // UNDECLARED = 최우선
		case Unobserved:
			review = append(review, ReviewItem{Rec: r, Priority: 2, Mandatory: true})
		}
	}
	sort.SliceStable(review, func(i, j int) bool { return review[i].Priority > review[j].Priority })
	return autopass, review
}
