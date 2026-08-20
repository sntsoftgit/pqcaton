package ui

import (
	"errors"
	"strings"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// 확정이 막힌 이유의 한국어. **영어는 여기 없다** — `decision` 패키지가 갖는다.
var koGate = []struct {
	err error
	ko  string
}{
	{decision.ErrMandatoryPending, "확정할 수 없습니다: 아직 판정하지 않은 필수 항목이 남아 있습니다 — 필수 항목은 하나도 빠짐없이 판정해야 합니다"},
	{decision.ErrNoSignature, "확정할 수 없습니다: 승인 서명이 없습니다"},
	{decision.ErrNotInReview, "확정할 수 없습니다: 세션이 in-review 상태가 아닙니다"},
	{decision.ErrNotDraft, "리뷰를 시작할 수 없습니다: 세션이 draft 상태가 아닙니다"},
}

func koMissing(m decision.Missing) string {
	switch m.Code {
	case decision.MissingSignature:
		return "서명을 채우십시오"
	case decision.MissingApproval:
		return "승인자와 서명을 채우십시오"
	case decision.MissingConclusion:
		if m.Detail == "" {
			return "왜 이렇게 정했는지를 적지 않았습니다: " + m.Subject
		}
		return "왜 이렇게 정했는지를 적지 않았습니다: " + m.Subject + " (" + m.Detail + ")"
	}
	return decision.EnglishMissing(m)
}

// Refusal — 「하지 않았습니다」 상자에 들어갈 글을 그 말로 그린다.
//
// **구조를 들고 있는 오류만 옮긴다.** 파일을 못 읽었다거나 하는 것은 있는 그대로 낸다 —
// 어설프게 옮기면 원문이 사라져 붙여 넣어 물어볼 것이 없어진다.
func Refusal(l Lang, err error) string {
	if err == nil {
		return ""
	}
	if l != KO {
		return err.Error()
	}
	head := ""
	for _, g := range koGate {
		if errors.Is(err, g.err) {
			head = g.ko
			break
		}
	}
	missing := decision.MissingOf(err)
	if head == "" && len(missing) == 0 {
		return err.Error()
	}
	if head == "" {
		head = err.Error()
	}
	var b strings.Builder
	b.WriteString(head)
	for _, m := range missing {
		b.WriteString("\n   · " + koMissing(m))
	}
	return b.String()
}
