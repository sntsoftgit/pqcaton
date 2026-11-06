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
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/rulesetround"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// finalizeCandidate: 02 확정이 하는 그대로 asset-scope.csv를 쓴다(그 출력을 적은 영수증과 한 트랜잭션).
func finalizeCandidate(t *testing.T, s *server, body string) {
	t.Helper()
	b := []byte(body)
	if _, err := s.st.FinalizeChecked(s.scopeID(), []string{s.scopeOut}, store.Check{}, store.Sum(b), time.Second, func() (store.Parts, error) {
		return store.Parts{PayloadSHA256: store.Sum(b), Replace: []store.Change{{Path: s.scopeOut, Content: b, Backup: true}}}, nil
	}); err != nil {
		t.Fatal(err)
	}
}

// IC-U157 — **05의 새 정책 활성화.** 02에서 확정한 정책이 활성 정책과 다르면 05에 활성화 상자(운영자 진술 칸과 한계 문구)가
// 있다. 진술 없이 확인을 요청하면 400이다. 확인 페이지는 두 지문 · 바뀌는 파일 · 되돌리기 제한과 백업 위치를 적고 아무것도
// 쓰지 않는다. 활성화하면 활성 정책이 바뀌고 옛 정책과 옛 세션이 보관되며, 새 세션은 새 정책의 지문을 든다. 활성화 전에
// 확정한 계획은 06에서 서명되지 않는다. 같은 활성화를 다시 요청하면 409 `scope-same`이고, 02 확정 뒤 손으로 고친 후보는
// 409 `scope-not-finalized`다.
func TestActivateTheFinalizedPolicy(t *testing.T) {
	s, _ := keyReady(t)
	observed(t, s.results, "web", "libssl.so.3", "/usr/sbin/nginx")
	observed(t, s.results, "web", "libcrypto.so.3", "/usr/bin/python3")
	setPolicy(t, s, policyA)
	old := savedSession(t, s)
	bindPlan(t, s, planJSON)
	finalizeCandidate(t, s, policyB)

	page := get(t, s, ui.ScreenReview+"?lang=ko").Body.String()
	if !hasForm(page, ui.ReviewScopeActivateConfirmPath) || !strings.Contains(page, "상류 적재를 이 정책으로 다시 마쳤음을 확인했습니다") ||
		!strings.Contains(page, "직접 증명하지는 않습니다") {
		t.Fatal("the review screen does not offer the activation with the operator's statement and its limit")
	}
	dir := filepath.Dir(s.path)
	before := snapshotDir(t, dir)
	if w := postForm(t, s, ui.ReviewScopeActivateConfirmPath, url.Values{}); w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), ui.RefusedScopeConfirm) {
		t.Fatalf("confirm without the statement = %d", w.Code)
	}
	w := postFormLang(t, s, ui.ReviewScopeActivateConfirmPath, "ko", url.Values{ui.ScopeActivateConfirmField: {"on"}})
	if w.Code != http.StatusOK {
		t.Fatalf("activation confirm = %d\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	from, _ := scope.PolicyFileDigest(s.scopeAssets)
	to, _ := scope.PolicyFileDigest(s.scopeOut)
	for _, want := range []string{from, to, "v0.25.x 이하로 다시 열 수 없습니다", "운영자 안내서 7절", "scope-assets.activation-"} {
		if !strings.Contains(body, want) {
			t.Errorf("the confirmation page does not show %q", want)
		}
	}
	sameDir(t, before, snapshotDir(t, dir))

	if w := postFormLang(t, s, ui.ReviewScopeActivatePath, "ko", hiddenFields(body)); w.Code != http.StatusSeeOther {
		t.Fatalf("activate = %d\n%s", w.Code, w.Body.String())
	}
	if readFile(t, s.scopeAssets) != policyB {
		t.Error("the active policy was not replaced")
	}
	if a, _ := filepath.Glob(filepath.Join(filepath.Dir(s.scopeAssets), "scope-assets.activation-*.csv")); len(a) != 1 || readFile(t, a[0]) != policyA {
		t.Errorf("old policy archive: %v", a)
	}
	next, err := review.Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if next.SessionID == old.SessionID || next.ScopePolicyDigestV1 != to || s.scopeLockOf(next) != "" {
		t.Fatalf("new session: id %s digest %s", next.SessionID, next.ScopePolicyDigestV1)
	}
	rc, err := s.st.RoundReceiptOf(rulesetround.ScopeActivationReceipt, store.Identity{Kind: kindReview, Org: s.orgName(), SessionID: old.SessionID})
	if err != nil || rc == nil || rc.Round == nil || rc.Round.FromPolicy != from || rc.Round.ToSessionID != next.SessionID {
		t.Fatalf("scope-activation receipt: %+v %v", rc, err)
	}
	if code := signConfirm(t, s); code != http.StatusConflict {
		t.Errorf("signing a plan made before the activation = %d, want 409", code)
	}
	if w := postForm(t, s, ui.ReviewScopeActivateConfirmPath, url.Values{ui.ScopeActivateConfirmField: {"on"}}); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), ui.RefusedScopeSame) {
		t.Errorf("activating the same policy again = %d, want 409 %s", w.Code, ui.RefusedScopeSame)
	}
	if err := os.WriteFile(s.scopeOut, []byte(policyA+"exclude,*,*,/opt/*,hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if w := postForm(t, s, ui.ReviewScopeActivateConfirmPath, url.Values{ui.ScopeActivateConfirmField: {"on"}}); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), ui.RefusedScopeNotFinalized) {
		t.Errorf("activating a hand-edited candidate = %d, want 409 %s", w.Code, ui.RefusedScopeNotFinalized)
	}
}
