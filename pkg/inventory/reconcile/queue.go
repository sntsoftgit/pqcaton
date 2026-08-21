package reconcile

import "sort"

// ReviewItem — 리뷰 큐 항목(§3.3②). 대조 엔진은 판정 대상을 구조화·우선순위화한다.
type ReviewItem struct {
	Rec       Reconciled
	Priority  int  // 높을수록 먼저. 위험도×블라스트반경×데이터민감도의 프록시(§3.3② — 실측 모델은 후속)
	Mandatory bool // 필수 개별 리뷰 대상
}

// BuildReviewQueue — 대조 결과를 (자동통과 후보, 필수 리뷰 큐)로 나눈다(§3.3②, §3.5 PROPOSE).
//   - 자동통과 후보: CONFIRMED + 고신뢰 → 일괄 승인 제안(승인은 사람).
//   - 필수 개별 리뷰: UNDECLARED(최우선), UNOBSERVED, 저신뢰 CONFIRMED.
//
// 반환 큐는 우선순위 내림차순 정렬. 확정(finalize)은 여기서 하지 않는다 — Decision 서비스 소관.
func BuildReviewQueue(recs []Reconciled) (autopass []Reconciled, review []ReviewItem) {
	for _, r := range recs {
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
