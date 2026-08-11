package decision

import (
	"errors"
	"testing"
)

// IC-F1: 신규 세션은 draft.
func TestNewSession_draft(t *testing.T) {
	s := NewSession("ring:canary", []Item{{ID: "a", Mandatory: true}})
	if s.Status != Draft {
		t.Errorf("status = %s, want draft", s.Status)
	}
}

// IC-F2·F3: draft→in-review→finalized (전 필수 판정 + 서명).
func TestFinalize_happy(t *testing.T) {
	s := NewSession("ring:canary", []Item{{ID: "a", Mandatory: true}, {ID: "b", Mandatory: false}})
	if err := s.StartReview(); err != nil {
		t.Fatal(err)
	}
	if s.Status != InReview {
		t.Fatalf("status = %s, want in-review", s.Status)
	}
	s.Decide("a", "승인")
	s.Sign("alice", "ed25519:sig")
	if err := s.Finalize(); err != nil {
		t.Fatalf("finalize 실패: %v", err)
	}
	if s.Status != Finalized {
		t.Errorf("status = %s, want finalized", s.Status)
	}
}

// IC-F4: 필수 항목 미판정 → finalize 거부.
func TestFinalize_mandatoryPending(t *testing.T) {
	s := NewSession("r", []Item{{ID: "a", Mandatory: true}})
	_ = s.StartReview()
	s.Sign("alice", "sig")
	if err := s.Finalize(); !errors.Is(err, ErrMandatoryPending) {
		t.Errorf("err = %v, want ErrMandatoryPending", err)
	}
	if s.Status == Finalized {
		t.Error("필수 미판정인데 finalized 됨")
	}
}

// IC-F5: 승인 서명 없음 → finalize 거부.
func TestFinalize_noSignature(t *testing.T) {
	s := NewSession("r", []Item{{ID: "a", Mandatory: true}})
	_ = s.StartReview()
	s.Decide("a", "승인")
	if err := s.Finalize(); !errors.Is(err, ErrNoSignature) {
		t.Errorf("err = %v, want ErrNoSignature", err)
	}
}

// in-review 아닌데 finalize → 거부.
func TestFinalize_notInReview(t *testing.T) {
	s := NewSession("r", nil) // draft
	if err := s.Finalize(); !errors.Is(err, ErrNotInReview) {
		t.Errorf("err = %v, want ErrNotInReview", err)
	}
}

// IC-F6: 세션(링/도메인) 단위 부분 확정 — 한 세션 finalize해도 다른 세션은 draft 유지.
func TestPartialFinalize(t *testing.T) {
	s1 := NewSession("ring:canary", []Item{{ID: "a", Mandatory: true}})
	s2 := NewSession("ring:prod", []Item{{ID: "b", Mandatory: true}})
	_ = s1.StartReview()
	s1.Decide("a", "ok")
	s1.Sign("alice", "sig")
	if err := s1.Finalize(); err != nil {
		t.Fatal(err)
	}
	if s2.Status != Draft {
		t.Errorf("s2 = %s, 부분 확정인데 다른 세션이 영향받음", s2.Status)
	}
}

// IC-F7: 정책 단위 판정 — 같은 정책 항목 일괄 판정으로 finalize 성립.
func TestDecidePolicy(t *testing.T) {
	s := NewSession("domain:web", []Item{
		{ID: "n1", Policy: "openssl-3.0-dynamic", Mandatory: true},
		{ID: "n2", Policy: "openssl-3.0-dynamic", Mandatory: true},
		{ID: "n3", Policy: "openssl-3.0-dynamic", Mandatory: true},
	})
	_ = s.StartReview()
	if n := s.DecidePolicy("openssl-3.0-dynamic", "provider 주입 승인"); n != 3 {
		t.Fatalf("정책 일괄 판정 = %d개, want 3", n)
	}
	s.Sign("alice", "sig")
	if err := s.Finalize(); err != nil {
		t.Fatalf("정책 일괄 판정 후 finalize 실패: %v", err)
	}
}
