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

// IC-U141 — **화면에서 규칙 전환 회차를 연다.** 다른 상류 규칙 판의 세션이면 판정 화면이 단추를 보이고, 누르면 옛 파일이
// 바이트 그대로 보관되고 그 자리에 지금 규칙의 새 세션(새 ID)이 쓰이며 보고서가 생긴다. 옛 결론은 참고값에만 있고 현재
// 칸은 빈다. 같은 단추를 다시 눌러도 파일이 늘지 않는다. 화면은 보고서와 지난 판정을 보이고, 「다시 채택」은 그 항목만
// 현재 값으로 옮긴다.
func TestRulesetRoundFromTheScreen(t *testing.T) {
	s, dir := withDecl(t)
	s.results = filepath.Join(dir, "results")
	if err := os.MkdirAll(s.results, 0o700); err != nil {
		t.Fatal(err)
	}
	writeResult(t, filepath.Join(s.results, "web.json"), "web-gw")
	sf, _, err := s.reviewSession()
	if err != nil || len(sf.Items) == 0 {
		t.Fatalf("no items to judge: %v", err)
	}
	sf.RulesetVersion = "pqcota-enrich/v0" + strings.TrimPrefix(review.RulesetVersion, review.UpstreamRuleset(review.RulesetVersion))
	id := sf.Items[0].ID
	sf.Items[0].Conclusion = "옛 결론"
	if err := review.Save(s.path, sf); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, s.path)
	if body := get(t, s, ui.ScreenReview+"?lang=ko").Body.String(); !strings.Contains(body, ui.ReviewRulesetRoundPath) || !strings.Contains(body, "지금 규칙으로 새 회차 시작") {
		t.Fatal("the review screen does not offer the ruleset round")
	}
	w := postForm(t, s, ui.ReviewRulesetRoundPath, withRev(url.Values{}, s.revFor(ui.ScreenReview)))
	if q := location(t, w); q.Get("msg") != ui.NoticeRulesetRoundOpened {
		t.Fatalf("opening the round: %v", q)
	}
	next, err := review.Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if next.RulesetVersion != review.RulesetVersion || next.SessionID == sf.SessionID || next.RulesetChange == nil {
		t.Fatalf("the new session: ruleset %q id %q", next.RulesetVersion, next.SessionID)
	}
	var item review.Item
	for _, it := range review.All(next) {
		if it.ID == id {
			item = it
		}
	}
	if item.Conclusion != "" || item.Previous == nil || item.Previous.Conclusion != "옛 결론" {
		t.Errorf("the carried item: conclusion %q previous %+v", item.Conclusion, item.Previous)
	}
	archives, _ := filepath.Glob(filepath.Join(dir, "session.ruleset-*.json"))
	reports, _ := filepath.Glob(filepath.Join(dir, "session.ruleset-change-*.json"))
	var archive string
	for _, a := range archives {
		if !strings.Contains(a, "ruleset-change-") {
			archive = a
		}
	}
	if archive == "" || readFile(t, archive) != before || len(reports) != 1 {
		t.Fatalf("archive %q, reports %v", archive, reports)
	}

	n, _ := os.ReadDir(dir)
	w = postForm(t, s, ui.ReviewRulesetRoundPath, withRev(url.Values{}, s.revFor(ui.ScreenReview)))
	if w.Code != http.StatusSeeOther || location(t, w).Get("msg") != "" {
		t.Errorf("the same button again: %d %v", w.Code, w.Header().Get("Location"))
	}
	if m, _ := os.ReadDir(dir); len(m) != len(n) {
		t.Error("pressing the button again made files")
	}

	body := get(t, s, ui.ScreenReview+"?lang=ko").Body.String()
	for _, want := range []string{"규칙 전환 보고서", "지난 판정", "옛 결론", `name="adopt"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the review screen lacks %q", want)
		}
	}
	w = postForm(t, s, "/save", withRev(url.Values{"adopt": {id}}, s.revFor(ui.ScreenReview)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("adopting: %d\n%s", w.Code, w.Body.String())
	}
	adopted, _ := review.Load(s.path)
	for _, it := range review.All(adopted) {
		if it.ID == id && it.Conclusion != "옛 결론" {
			t.Errorf("adopt again did not take the earlier conclusion: %q", it.Conclusion)
		}
	}
}
