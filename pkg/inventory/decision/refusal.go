package decision

import (
	"errors"
	"fmt"
	"strings"
)

// Missing — 확정을 막은 것 하나.
//
// **문장이 아니라 값이다.** 같은 거절을 명령은 영어로 말하고 화면은 보는 사람의 말로
// 말해야 한다 — 여기에 문장을 담으면 그 둘 중 하나는 남의 말로 뜬다.
type Missing struct {
	// Code — [MissingSignature] | [MissingConclusion].
	Code string
	// Subject · Detail — 무엇이 빠졌나. 항목 id 와, 그 항목을 알아볼 곁말(상태·계층).
	Subject, Detail string
}

const (
	MissingSignature  = "signature"
	MissingConclusion = "conclusion"
)

// NotFinalized — 확정이 게이트를 지나지 못했다. **무엇이 남았는지를 들고 있다.**
//
// 문장으로 감싸 버리면 화면이 그것을 다시 말로 옮길 수 없다 — 그래서 사유를 값으로
// 들고 다니고, 영어 문장은 [NotFinalized.Error] 가 만든다.
type NotFinalized struct {
	Err     error
	Missing []Missing
}

func (e *NotFinalized) Unwrap() error { return e.Err }

// Error — 명령이 읽는 영어 문장. **여기가 영어의 유일한 자리다.**
func (e *NotFinalized) Error() string {
	var b strings.Builder
	b.WriteString(e.Err.Error())
	for _, m := range e.Missing {
		b.WriteString("\n   · " + EnglishMissing(m))
	}
	return b.String()
}

// EnglishMissing — 빠진 것 하나를 영어로.
func EnglishMissing(m Missing) string {
	switch m.Code {
	case MissingSignature:
		return "the approver and signature are not filled in"
	case MissingConclusion:
		if m.Detail == "" {
			return fmt.Sprintf("no record of why this was decided: %s", m.Subject)
		}
		return fmt.Sprintf("no record of why this was decided: %s (%s)", m.Subject, m.Detail)
	}
	return m.Code
}

// MissingOf — 오류에서 빠진 것들을 꺼낸다. 화면이 그 말로 다시 그리는 자리다.
func MissingOf(err error) []Missing {
	var nf *NotFinalized
	if errors.As(err, &nf) {
		return nf.Missing
	}
	return nil
}
