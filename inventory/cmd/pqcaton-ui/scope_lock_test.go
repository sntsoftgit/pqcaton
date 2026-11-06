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
	"github.com/sntsoftgit/pqcaton/pkg/inventory/rulesetround"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// savedSession: 관측이 있는 서버에서 한 번 저장해 세션 파일을 만들고, 그 세션을 돌려준다.
func savedSession(t *testing.T, s *server) review.Session {
	t.Helper()
	// 픽스처의 세션 파일(정책 없음)은 지우고 지금 정책으로 새로 세운다. 남겨 두면 다른 정책을 건 순간 잠긴다(IC-U155).
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if w := postForm(t, s, "/save", url.Values{"reviewer": {"보안팀"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("save = %d\n%s", w.Code, w.Body.String())
	}
	sf, err := review.Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	return sf
}

func hasForm(page, action string) bool { return strings.Contains(page, `action="`+action+`"`) }

// IC-U154 — **정책 기록이 없는 옛 세션은 05가 잠기고, 운영자 확인을 받은 기준선 회차로만 넘어간다.** 화면에 기준선 상자와
// 확인 칸 · 「확인만 기록한다」는 한계가 있고, 저장은 409 `scope-baseline`이며 파일은 그대로다. 확인 없이 보내면 400
// `scope-confirm`이고 아무것도 쓰지 않는다. 확인과 함께 보내면 새 세션(새 ID, 지금 정책의 지문, 현재 결론은 빔)이 생기고
// 옛 세션은 바이트 그대로 보관되며 scope-baseline 형식 2 영수증이 남는다. 같은 요청을 다시 보내면 거절되고 파일이 늘지
// 않는다.
func TestLegacySessionNeedsAConfirmedBaseline(t *testing.T) {
	s := observedServer(t)
	sf := savedSession(t, s)
	sf.ScopePolicyDigestV1, sf.PolicyDecisions = "", map[string]string{}
	for i := range sf.Items {
		sf.Items[i].Conclusion = "교체한다"
	}
	if err := review.Save(s.path, sf); err != nil {
		t.Fatal(err)
	}
	legacy := readFile(t, s.path)
	dir := filepath.Dir(s.path)

	page := get(t, s, ui.ScreenReview+"?lang=ko").Body.String()
	if !hasForm(page, ui.ReviewScopeBaselinePath) || !strings.Contains(page, "상류 적재에 사용한 정책임을 확인했습니다") ||
		!strings.Contains(page, "직접 증명하지는 않습니다") || hasForm(page, ui.ReviewRulesetRoundPath) {
		t.Fatalf("the review screen does not offer the confirmed baseline round")
	}
	if got := cards(t, page)["05"]; !strings.HasPrefix(got, "정책 기준선을 채택해야 합니다") {
		t.Errorf("card 05 = %q", got)
	}
	w := postForm(t, s, "/save", url.Values{"reviewer": {"보안팀"}})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedScopeBaseline) || readFile(t, s.path) != legacy {
		t.Fatalf("save on a legacy session = %d, file kept %v", w.Code, readFile(t, s.path) == legacy)
	}
	before := snapshotDir(t, dir)
	w = postForm(t, s, ui.ReviewScopeBaselinePath, url.Values{})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), ui.RefusedScopeConfirm) {
		t.Fatalf("baseline without confirmation = %d", w.Code)
	}
	sameDir(t, before, snapshotDir(t, dir))

	if w := postForm(t, s, ui.ReviewScopeBaselinePath, url.Values{ui.ScopeBaselineConfirmField: {"on"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("baseline = %d\n%s", w.Code, w.Body.String())
	}
	next, err := review.Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	active := s.scopeLockOf(next)
	if next.SessionID == sf.SessionID || next.ScopePolicyDigestV1 == "" || active != "" || next.ScopeChange == nil {
		t.Fatalf("new session: id %s digest %q lock %q change %+v", next.SessionID, next.ScopePolicyDigestV1, active, next.ScopeChange)
	}
	for _, it := range next.Items {
		if it.Conclusion != "" {
			t.Errorf("%s: an old conclusion became current", it.ID)
		}
	}
	archives, _ := filepath.Glob(filepath.Join(dir, strings.TrimSuffix(filepath.Base(s.path), ".json")+".scope-baseline-"+sf.SessionID+".json"))
	if len(archives) != 1 || readFile(t, archives[0]) != legacy {
		t.Fatalf("archive of the old session: %v", archives)
	}
	rc, err := s.st.RoundReceiptOf(rulesetround.ScopeBaselineReceipt, store.Identity{Kind: kindReview, Org: s.orgName(), SessionID: sf.SessionID})
	if err != nil || rc == nil || rc.Format != 2 || rc.Round == nil || rc.Round.ToSessionID != next.SessionID {
		t.Fatalf("scope-baseline receipt: %+v, %v", rc, err)
	}
	after := snapshotDir(t, dir)
	if w := postForm(t, s, ui.ReviewScopeBaselinePath, url.Values{ui.ScopeBaselineConfirmField: {"on"}}); w.Code < 400 {
		t.Errorf("a second baseline round = %d, want a refusal", w.Code)
	}
	sameDir(t, after, snapshotDir(t, dir))
}

// IC-U155 — **활성화 기록 없이 scope-assets.csv를 바꾸면 05가 잠기고 회차 단추가 없다.** 화면은 이전 정책 복원과 정식
// 활성화만 안내하고, 저장은 409 `scope-unactivated`이며 파일은 그대로다. 이전 정책을 복원하면 다시 저장된다.
func TestHandEditedPolicyLocksTheReview(t *testing.T) {
	s := observedServer(t)
	setPolicy(t, s, policyA)
	savedSession(t, s)
	kept := readFile(t, s.path)
	setPolicy(t, s, policyB)

	page := get(t, s, ui.ScreenReview+"?lang=ko").Body.String()
	if hasForm(page, ui.ReviewScopeBaselinePath) || hasForm(page, ui.ReviewRulesetRoundPath) ||
		!strings.Contains(page, "활성화 없이 바뀌었습니다") || !strings.Contains(page, "이전 정책 파일을 복원") {
		t.Fatalf("the review screen should be locked with the two ways out and no round button")
	}
	w := postForm(t, s, "/save", url.Values{"reviewer": {"다른 사람"}})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedScopeUnactivated) || readFile(t, s.path) != kept {
		t.Fatalf("save after a hand-edited policy = %d, file kept %v", w.Code, readFile(t, s.path) == kept)
	}
	setPolicy(t, s, policyA)
	if w := postForm(t, s, "/save", url.Values{"reviewer": {"다른 사람"}}); w.Code != http.StatusSeeOther {
		t.Errorf("save after restoring the previous policy = %d", w.Code)
	}
}

// IC-U156 — **note · 공백만 바뀐 정책은 같은 정책이다.** 잠기지 않고 저장된다.
func TestPolicyWithOnlyNotesChangedDoesNotLock(t *testing.T) {
	s := observedServer(t)
	setPolicy(t, s, policyA)
	savedSession(t, s)
	setPolicy(t, s, strings.Replace(policyA, "managed by OS patching", "patched by the OS team", 1)+"\n# reviewed\n")
	page := get(t, s, ui.ScreenReview+"?lang=ko").Body.String()
	if hasForm(page, ui.ReviewScopeBaselinePath) || strings.Contains(page, "활성화 없이 바뀌었습니다") {
		t.Error("a notes-only change locked the review")
	}
	if w := postForm(t, s, "/save", url.Values{"reviewer": {"보안팀"}}); w.Code != http.StatusSeeOther {
		t.Errorf("save = %d", w.Code)
	}
	if err := os.Remove(s.scopeAssets); err != nil {
		t.Fatal(err)
	}
}
