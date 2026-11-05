// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// IC-U140 — **세션 파일이 없는 것과 규칙 판이 빈 옛 파일을 가른다.** 결과가 있는 화면에서, 규칙 판이 빈 옛 세션
// 파일은 옮기지도 저장하지도 않고(409 `ruleset-changed`, 바이트 그대로) 오늘의 규칙 판을 찍지 않는다. 파일이 없으면
// 저장이 지금 규칙 판의 새 세션을 쓴다.
func TestEmptyRulesetFileIsKeptButMissingFileStartsFresh(t *testing.T) {
	s, dir := withDecl(t)
	s.results = filepath.Join(dir, "results")
	if err := os.MkdirAll(s.results, 0o700); err != nil {
		t.Fatal(err)
	}
	old := session()
	old.RulesetVersion = ""
	old.Reviewer = "옛 검토자"
	if err := review.Save(s.path, old); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, s.path)
	form := url.Values{"reviewer": {"보안팀"}}
	w := postForm(t, s, "/save", withRev(form, s.revFor(ui.ScreenReview)))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedRulesetChanged) {
		t.Fatalf("an old file without a ruleset was not refused: %d\n%s", w.Code, w.Body.String())
	}
	if readFile(t, s.path) != before {
		t.Error("the old session file was rewritten")
	}

	if err := os.Remove(s.path); err != nil {
		t.Fatal(err)
	}
	w = postForm(t, s, "/save", withRev(form, s.revFor(ui.ScreenReview)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("saving without a session file: %d\n%s", w.Code, w.Body.String())
	}
	got, err := review.Load(s.path)
	if err != nil || got.RulesetVersion != review.RulesetVersion {
		t.Errorf("the new session does not carry today's ruleset: %q (%v)", got.RulesetVersion, err)
	}
}
