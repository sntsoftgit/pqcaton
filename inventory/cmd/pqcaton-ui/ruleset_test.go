// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-U139 — **상류 규칙 판이 다른 세션 파일은 화면이 다시 쓰지 않는다.** 저장 · 확정 · 새 회차가 모두 409
// `ruleset-changed`이고, 파일은 바이트 그대로다. 넘어가는 길은 규칙 전환 회차다.
func TestAnotherUpstreamRulesetIsNotRewritten(t *testing.T) {
	s, _ := newServer(t)
	sf := session()
	sf.RulesetVersion = "pqcota-enrich/v0" + strings.TrimPrefix(review.RulesetVersion, review.UpstreamRuleset(review.RulesetVersion))
	if err := review.Save(s.path, sf); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, s.path)
	for _, c := range []struct{ name, screen, post string }{
		{"save", ui.ScreenReview, "/save"},
		{"finalize", ui.ScreenReview, "/finalize/confirm"},
		{"new round", ui.ScreenReview, ui.ReviewNewPath},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := postForm(t, s, c.post, withRev(signed(), s.revFor(c.screen)))
			if w.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409\n%s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), ui.RefusedRulesetChanged) {
				t.Errorf("the refusal does not carry the code %s", ui.RefusedRulesetChanged)
			}
			if readFile(t, s.path) != before {
				t.Error("the session file was rewritten")
			}
		})
	}
}
