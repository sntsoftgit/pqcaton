// Package decision implements the Inventory 리뷰-확정 상태기계 (규정서 §3.3③, §6).
//
// pqcota가 만들지 않기로 한 계층이다 — 확정된 계획만 그쪽 프로비저닝의 입력이 된다.
// draft → in-review → finalized. finalized 전에는 프로비저닝 실행 불가(§5 — 우회할 수 없는 게이트).
// "인벤토리 확정은 기계적 머지가 아니라 리뷰-계획-확정"(§3.1).
package decision

import "errors"

type Status string

const (
	Draft     Status = "draft"
	InReview  Status = "in-review"
	Finalized Status = "finalized"
)

var (
	ErrNotDraft    = errors.New("cannot start review: the session is not in draft")
	ErrNotInReview = errors.New("cannot finalize: the session is not in review")
	// **확정이 막힐 때 사람이 읽는 문장이다**(영어). 화면은 이것을 그대로 내지 않고,
	// [NotFinalized] 가 들고 있는 값을 보는 사람의 말로 다시 그린다.
	ErrMandatoryPending = errors.New("cannot finalize: mandatory items are still unjudged — every mandatory item must be judged")
	ErrNoSignature      = errors.New("cannot finalize: there is no approval signature")
)

// Item — 리뷰 대상(자산/엣지). 같은 Policy는 정책 단위로 일괄 판정된다(§3.4).
type Item struct {
	ID         string
	Policy     string // 버전×링크모드(및 JDK×provider) 정책 템플릿 id. 빈값=개별 격리
	Mandatory  bool   // 필수 개별 리뷰 항목(§3.3②)
	Decided    bool
	Conclusion string
}

// Session — 링/도메인 단위 리뷰-확정 세션(§6). 세션 단위라 부분 확정이 자연스럽다(§3.3③).
type Session struct {
	Scope     string // ring/domain
	Status    Status
	Items     []Item
	Reviewer  string
	Signature string // 승인 서명 (finalize 전제)
}

// NewSession — 신규 세션은 draft로 시작한다(IC-F1).
func NewSession(scope string, items []Item) *Session {
	return &Session{Scope: scope, Status: Draft, Items: items}
}

// StartReview — draft → in-review (IC-F2).
func (s *Session) StartReview() error {
	if s.Status != Draft {
		return ErrNotDraft
	}
	s.Status = InReview
	return nil
}

// Decide — 개별 항목 판정(§3.4 개별 격리 예외).
func (s *Session) Decide(itemID, conclusion string) bool {
	for i := range s.Items {
		if s.Items[i].ID == itemID {
			s.Items[i].Decided = true
			s.Items[i].Conclusion = conclusion
			return true
		}
	}
	return false
}

// DecidePolicy — 정책 단위 판정: 같은 Policy의 모든 항목을 일괄 판정한다(§3.4 정책 템플릿). 판정 수 반환.
func (s *Session) DecidePolicy(policy, conclusion string) int {
	if policy == "" {
		return 0
	}
	n := 0
	for i := range s.Items {
		if s.Items[i].Policy == policy {
			s.Items[i].Decided = true
			s.Items[i].Conclusion = conclusion
			n++
		}
	}
	return n
}

// Sign — 승인 서명 부착(finalize 전제, §3.3③).
func (s *Session) Sign(reviewer, signature string) {
	s.Reviewer = reviewer
	s.Signature = signature
}

// Finalize — in-review → finalized. 전 필수 항목 판정 + 승인 서명이 있어야만 성공한다(§3.3③).
// 이 게이트는 우회할 수 없다 — 확정되지 않은 계획으로는 프로비저닝이 돌지 않는다.
func (s *Session) Finalize() error {
	if s.Status != InReview {
		return ErrNotInReview
	}
	for _, it := range s.Items {
		if it.Mandatory && !it.Decided {
			return ErrMandatoryPending // IC-F4
		}
	}
	if s.Signature == "" {
		return ErrNoSignature // IC-F5
	}
	s.Status = Finalized // IC-F3
	return nil
}
