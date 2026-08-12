package org_test

import (
	"errors"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/org"
)

// 빈 조직으로는 아무것도 열 수 없어야 한다. 이걸 허용하면 격리가 선택 사항이 된다.
func TestEmptyIsRejected(t *testing.T) {
	if _, err := org.Parse(""); !errors.Is(err, org.ErrEmpty) {
		t.Fatalf("빈 값을 받아들였다: %v", err)
	}
}

func TestShape(t *testing.T) {
	ok := []string{"acme", "acme-corp", "a1", "kt-ds-2026"}
	for _, s := range ok {
		if _, err := org.Parse(s); err != nil {
			t.Errorf("%q는 유효해야 한다: %v", s, err)
		}
	}
	// 대문자는 막는다 — Acme와 acme가 다른 조직이 되면 데이터가 조용히 갈린다.
	bad := []string{"A", "Acme", "a", "-acme", "acme_corp", "acme.corp", "한글"}
	for _, s := range bad {
		if _, err := org.Parse(s); err == nil {
			t.Errorf("%q는 거절돼야 한다", s)
		}
	}
}
