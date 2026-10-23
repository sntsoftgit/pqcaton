// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"strings"
	"testing"
	"unicode"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
)

func hangul(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

func verbs(s string) int { return strings.Count(s, "%") - 2*strings.Count(s, "%%") }

// IC-UI49 — **알림 · 거절 · 오류의 문구 목록이 두 말로 다 있다.** 코드마다 한국어와 영어가 있고, 영어에 한글이
// 없고, 두 말의 자리표시자 수가 같고 인자 종류의 수와도 같다. 계획 칸 거절의 사유 코드(decision.Plan*)는 모두
// 문구가 있다.
func TestNoticeCatalogIsComplete(t *testing.T) {
	check := func(name string, tt T, args int) {
		t.Helper()
		if tt.KO == "" || tt.EN == "" {
			t.Errorf("%s: a language is missing", name)
		}
		if hangul(tt.EN) {
			t.Errorf("%s: Korean in the English text", name)
		}
		if verbs(tt.KO) != args || verbs(tt.EN) != args {
			t.Errorf("%s: placeholders ko %d en %d, want %d", name, verbs(tt.KO), verbs(tt.EN), args)
		}
	}
	for code, spec := range noticeText {
		check(code, spec.t, len(spec.kinds))
	}
	for kind, txt := range refusedText {
		check(kind, txt.body, 0)
		if txt.head.EN != "" {
			check(kind+" head", txt.head, 0)
		}
	}
	for _, code := range []string{decision.PlanExcluded, decision.PlanNoLevel, decision.PlanBadLevel, decision.PlanNoKind,
		decision.PlanBadKind, decision.PlanNoTarget, decision.PlanL3 + "activation.activate",
		decision.PlanL3 + "activation.restart", decision.PlanL3 + "activation.deactivate"} {
		txt, ok := planReasonText[code]
		if !ok {
			t.Errorf("plan reason %s has no text", code)
			continue
		}
		check("plan "+code, txt, 0)
	}
}

// IC-UI50 — **알림은 인코딩해도 그대로 돌아오고, 맞지 않으면 그리지 않는다.** 인자에 쉼표 · 물결 · 퍼센트 · 한글이
// 있어도 되돌아온다. 인자 수가 다르거나 숫자 자리에 글이 오거나 모르는 역할이면 「알 수 없는 알림」이다.
func TestNoticesRoundTrip(t *testing.T) {
	in := []Notice{N(NoticeDeclSaved, 3, 5, 2), N(ProblemMissingConclusionOf, "a,b~c%d/정책", "layer corp"), N(NoticeFinalizedPlan, 1, "plan")}
	out := DecodeNotices(EncodeNotices(in))
	if len(out) != len(in) {
		t.Fatalf("decoded %d notices, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i].Code != in[i].Code || strings.Join(out[i].Args, "|") != strings.Join(in[i].Args, "|") {
			t.Errorf("notice %d: %+v, want %+v", i, out[i], in[i])
		}
	}
	if got := out[2].Text(KO, func(string) string { return "plan.json" }); !strings.Contains(got, "plan.json") {
		t.Errorf("the role was not turned into the file name: %q", got)
	}
	unknown := tNoticeUnknown.In(EN)
	for _, n := range []Notice{
		{Code: "nope"},
		{Code: NoticeDeclSaved, Args: []string{"1", "2"}},
		{Code: NoticeDeclSaved, Args: []string{"1", "2", "x"}},
		{Code: NoticeFinalizedPlan, Args: []string{"1", "/etc/passwd"}},
		{Code: ProblemInternal, Args: []string{"a\nb"}},
	} {
		if got := n.Text(EN, nil); got != unknown {
			t.Errorf("%+v drew %q, want the unknown notice", n, got)
		}
	}
}

// IC-UI51 — **선언 문제는 코드마다 두 말의 문장과 칸이 있다.** 한국어 문장이 비거나 값 자리가 남지 않고, 영어는
// 강조 표시(**) 없이 `decl`의 문장이다. 자리 표기는 그 말의 행 이름으로 옮긴다.
func TestDeclProblemsHaveBothLanguages(t *testing.T) {
	codes := []decl.Code{decl.NodeNotInScope, decl.NodeHasNoIP, decl.IPMalformed, decl.IPClaimedTwice,
		decl.ObservedAsTwice, decl.NodeMissingIP, decl.AssetOffScope, decl.EdgeSrcOffScope, decl.EdgePortZero}
	for _, c := range codes {
		p := decl.Problem{Where: "nodes/a+b", Code: c, Detail: "10.0.0.9"}
		for _, l := range []Lang{KO, EN} {
			got := declProblems(l, []decl.Problem{p})[0]
			if got.What == "" || got.Why == "" || got.Field == "" || strings.Contains(got.What, "%") || strings.Contains(got.Why, "**") {
				t.Errorf("%s (%s): %+v", c, l, got)
			}
			if (l == EN) == hangul(got.What+got.Why+got.Field+got.Where) {
				t.Errorf("%s (%s): wrong language: %+v", c, l, got)
			}
		}
	}
	for where, want := range map[string]string{
		"nodes/a+b": "노드 a · b", "scope/a": "노드 a", "assets/a/openssl": "자산 a · openssl", "edges/a→b": "엣지 a → b",
	} {
		if got := declProblems(KO, []decl.Problem{{Where: where, Code: decl.EdgePortZero}})[0].Where; got != want {
			t.Errorf("%s → %q, want %q", where, got, want)
		}
	}
}
