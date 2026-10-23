// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

// 화면 위 알림(성공 · 막힌 까닭).
//
// 저장 · 확정 뒤에는 화면으로 리다이렉트하고, 알림은 그 주소의 질의(`msg` · `problem`)에 실려 간다.
// **질의에는 문장을 싣지 않는다.** 코드와 허용된 인자(숫자 · 파일 역할 · 항목 식별자 · 사유 코드)만 싣고,
// 화면이 문구 목록(text_notice.go)으로 그 말의 문장을 만든다. 그래서 주소에 오류 원문이나 경로가 실리지
// 않고, 손으로 만든 주소가 임의의 문장을 화면에 띄우지도 못한다. 맞지 않는 값은 「알 수 없는 알림」이다.

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// 질의 이름.
const (
	MsgParam     = "msg"
	ProblemParam = "problem"
)

// 알림 코드.
const (
	NoticeSaved               = "saved"
	NoticeFinalizedPlan       = "finalized-plan"
	NoticeFinalizedPolicy     = "finalized-policy"
	NoticeJudgmentsSaved      = "judgments-saved"
	NoticeRulesSaved          = "rules-saved"
	NoticeSignatureCleared    = "signature-cleared"
	NoticeDeclSaved           = "decl-saved"
	NoticeDeclDropped         = "decl-dropped"
	NoticeDeclStillOff        = "decl-still-off"
	NoticeFinalizedAlready    = "finalized-already"
	NoticeFinalizedSuperseded = "finalized-superseded"
	NoticeDeclCreated         = "decl-created"
	NoticeLayerCreated        = "layer-created"

	ProblemCannotFinalize      = "gate-cannot-finalize"
	ProblemMandatory           = "gate-mandatory"
	ProblemNoSignature         = "gate-no-signature"
	ProblemNotInReview         = "gate-not-in-review"
	ProblemNotDraft            = "gate-not-draft"
	ProblemMissingSignature    = "missing-signature"
	ProblemMissingApproval     = "missing-approval"
	ProblemMissingConclusion   = "missing-conclusion"
	ProblemMissingConclusionOf = "missing-conclusion-of"
	ProblemPlanField           = "missing-plan"
	ProblemInternal            = "internal"
)

// Notice — 알림 하나. 문장이 아니라 코드와 인자다.
type Notice struct {
	Code string
	Args []string
}

// N — 알림을 만든다. 정수는 십진수로 적는다.
func N(code string, args ...any) Notice {
	n := Notice{Code: code}
	for _, a := range args {
		switch v := a.(type) {
		case int:
			n.Args = append(n.Args, strconv.Itoa(v))
		default:
			n.Args = append(n.Args, fmt.Sprint(v))
		}
	}
	return n
}

// EncodeNotices — 질의 값 하나로. 알림은 「,」로, 인자는 「~」로 가르고 인자는 퍼센트 인코딩한다.
func EncodeNotices(ns []Notice) string {
	parts := make([]string, 0, len(ns))
	for _, n := range ns {
		p := n.Code
		for _, a := range n.Args {
			// 「~」 · 「,」는 가르는 글자라 늘 퍼센트로 바꾼다(PathEscape는 「~」를 남긴다).
			p += "~" + strings.NewReplacer("~", "%7E", ",", "%2C").Replace(url.PathEscape(a))
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ",")
}

// DecodeNotices — EncodeNotices의 반대. 모양이 맞지 않는 조각도 돌려주고, 판단은 Text가 한다.
func DecodeNotices(raw string) []Notice {
	if raw == "" {
		return nil
	}
	var out []Notice
	for _, part := range strings.Split(raw, ",") {
		fields := strings.Split(part, "~")
		n := Notice{Code: fields[0]}
		for _, f := range fields[1:] {
			a, err := url.PathUnescape(f)
			if err != nil {
				a = "\x00" // 깨진 인자: Text가 알 수 없는 알림으로 그린다
			}
			n.Args = append(n.Args, a)
		}
		out = append(out, n)
	}
	return out
}

// maxNotices · maxArg — 주소 하나에 실을 수 있는 양. 넘으면 버린다.
const (
	maxNotices = 32
	maxArg     = 200
)

// Text — 알림 하나를 그 말의 문장으로. file은 역할을 그 서버의 파일 이름으로 바꾼다(nil이면 역할 이름).
// 모르는 코드, 인자 수나 모양이 맞지 않는 알림은 「알 수 없는 알림」이다.
func (n Notice) Text(l Lang, file func(role string) string) string {
	spec, ok := noticeText[n.Code]
	if !ok || len(n.Args) != len(spec.kinds) {
		return tNoticeUnknown.In(l)
	}
	args := make([]any, 0, len(n.Args))
	for i, k := range spec.kinds {
		a := n.Args[i]
		switch k {
		case 'n':
			v, err := strconv.Atoi(a)
			if err != nil || v < 0 || len(a) > 9 {
				return tNoticeUnknown.In(l)
			}
			args = append(args, v)
		case 'r':
			if _, known := roleText[a]; !known {
				return tNoticeUnknown.In(l)
			}
			name := RoleLabel(l, a)
			if file != nil {
				if f := file(a); f != "" {
					name = f
				}
			}
			args = append(args, name)
		case 'i':
			if !identifier(a) {
				return tNoticeUnknown.In(l)
			}
			args = append(args, a)
		case 'p':
			args = append(args, planReason(l, a))
		}
	}
	return fmt.Sprintf(spec.t.In(l), args...)
}

// identifier — 화면에 그대로 보여도 되는 식별자인가: 길이 제한 안이고 제어 문자가 없다.
func identifier(s string) bool {
	if s == "" || utf8.RuneCountInString(s) > maxArg || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func planReason(l Lang, code string) string {
	if t, ok := planReasonText[code]; ok {
		return t.In(l)
	}
	return tPlanReasonUnknown.In(l)
}

// NoticeLines — 알림들을 문장으로. 성공 알림은 이어 붙여 한 문장으로, 막힌 까닭은 줄마다 쓴다.
func NoticeLines(l Lang, ns []Notice, file func(role string) string, joinWith string) string {
	if len(ns) > maxNotices {
		ns = ns[:maxNotices]
	}
	parts := make([]string, 0, len(ns))
	for _, n := range ns {
		parts = append(parts, n.Text(l, file))
	}
	return strings.Join(parts, joinWith)
}

// 확정 관문의 거절(decision 패키지의 센티널).
var gateCodes = []struct {
	err  error
	code string
}{
	{decision.ErrMandatoryPending, ProblemMandatory},
	{decision.ErrNoSignature, ProblemNoSignature},
	{decision.ErrNotInReview, ProblemNotInReview},
	{decision.ErrNotDraft, ProblemNotDraft},
}

// ProblemsOf — 확정 관문이 막은 까닭을 알림으로. 관문 오류가 아니면 nil이다(부르는 쪽이 참조 번호를 붙인
// ProblemInternal로 바꾼다). 오류 문자열을 해석하지 않는다. 센티널과 decision.Missing의 값만 본다.
func ProblemsOf(err error) []Notice {
	var out []Notice
	for _, g := range gateCodes {
		if errors.Is(err, g.err) {
			out = append(out, N(g.code))
			break
		}
	}
	missing := decision.MissingOf(err)
	if len(out) == 0 && len(missing) == 0 {
		return nil
	}
	if len(out) == 0 {
		out = append(out, N(ProblemCannotFinalize))
	}
	for _, m := range missing {
		switch m.Code {
		case decision.MissingSignature:
			out = append(out, N(ProblemMissingSignature))
		case decision.MissingApproval:
			out = append(out, N(ProblemMissingApproval))
		case decision.MissingConclusion:
			if m.Detail == "" {
				out = append(out, N(ProblemMissingConclusion, m.Subject))
			} else {
				out = append(out, N(ProblemMissingConclusionOf, m.Subject, m.Detail))
			}
		case decision.MissingPlanField:
			out = append(out, N(ProblemPlanField, m.Subject, m.Reason))
		}
	}
	return out
}
