// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// signed: 확정을 통과하는 리뷰 양식.
func signed() url.Values {
	return url.Values{
		"policy:openssl/libssl":              {"PQC 라이브러리로 교체한다"},
		"plan:host://local/openssl/libssl":   {"on"},
		"kind:host://local/openssl/libssl":   {"REMEDIATION_KIND_CONFIG_ONLY"},
		"target:host://local/openssl/libssl": {"ML-KEM (FIPS 203)"},
		"level:host://local/openssl/libssl":  {"L2"},
		"reviewer":                           {"보안팀"},
		"signature":                          {"sig"},
	}
}

func withRev(form url.Values, rev string) url.Values {
	out := url.Values{}
	for k, v := range form {
		out[k] = v
	}
	out.Set(ui.RevField, rev)
	return out
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// IC-U56 — **오래된 화면의 저장은 409다.** 화면을 연 뒤에 세션 파일이 바뀌었으면 그 화면이 보낸
// 저장을 쓰지 않는다. 거절 페이지는 역할과 파일 이름만 보이고, 전체 경로와 오류 원문은 싣지 않는다.
// 리뷰 · 스코프 · 선언 저장이 모두 같다.
func TestStaleScreenIsRefused(t *testing.T) {
	s, dir := withDecl(t)
	s2, _ := withScope(t)
	cases := []struct {
		name   string
		srv    *server
		screen string
		post   string
		file   string
		form   url.Values
	}{
		{"review", s, ui.ScreenReview, "/save", s.path, url.Values{"reviewer": {"보안팀"}}},
		{"declaration", s, ui.ScreenDecl, "/decl/save", s.decl, url.Values{"node.0.name": {"web-gw"}, "node.0.ips": {"10.0.0.1"}}},
		{"scope", s2, ui.ScreenScope, "/scope/save", s2.scope, url.Values{"reviewer": {"보안팀"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := c.srv.revFor(c.screen)
			if old == "" {
				t.Fatal("the screen carries no rev")
			}
			// 다른 곳에서 파일이 바뀐다.
			changed := readFile(t, c.file) + " "
			if err := os.WriteFile(c.file, []byte(changed), 0o600); err != nil {
				t.Fatal(err)
			}
			w := postForm(t, c.srv, c.post, withRev(c.form, old))
			if w.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409\n%s", w.Code, w.Body.String())
			}
			if readFile(t, c.file) != changed {
				t.Error("a stale screen's save was written")
			}
			body := w.Body.String()
			if !strings.Contains(body, filepath.Base(c.file)) {
				t.Errorf("the refusal does not name the file %s", filepath.Base(c.file))
			}
			if strings.Contains(body, dir) || strings.Contains(body, "out of date") {
				t.Errorf("the refusal shows a full path or the raw error:\n%s", body)
			}
		})
	}
}

// IC-U57 — **확정된 세션은 저장도 확정도 409다.** 다른 내용으로 다시 확정하거나, 확정한 세션을 고쳐
// 저장하면 아무것도 쓰지 않고 새 세션을 열라고 알린다.
func TestFinalizedSessionIsLocked(t *testing.T) {
	s, dir := newServer(t)
	s.judgments = filepath.Join(dir, "judgments.jsonl")
	if q := location(t, finalizeVia(t, s, "", "/finalize/confirm", signed())); q.Get("problem") != "" {
		t.Fatalf("finalize: %s", q.Get("problem"))
	}
	plan, sess := readFile(t, s.planOut), readFile(t, s.path)

	w := postFormLang(t, s, "/save", "ko", url.Values{"reviewer": {"다른 사람"}})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "새 세션") {
		t.Fatalf("save after finalize: status %d\n%s", w.Code, w.Body.String())
	}
	other := signed()
	other.Set("policy:openssl/libssl", "다른 결론")
	if w := finalizeVia(t, s, "", "/finalize/confirm", other); w.Code != http.StatusConflict {
		t.Fatalf("finalize with other content: status %d", w.Code)
	}
	if readFile(t, s.planOut) != plan || readFile(t, s.path) != sess {
		t.Error("a refused request changed the plan or the session")
	}
}

// IC-U58 — **같은 확정을 다시 눌러도 한 번만 쓴다.** 두 번째는 「이미 확정됨」으로 화면에 돌아가고,
// 판정 원장에는 한 번만 쌓인다. 앞 계획은 plan.json.prev로 남는다.
func TestRepeatedFinalizeWritesOnce(t *testing.T) {
	s, dir := newServer(t)
	s.judgments = filepath.Join(dir, "judgments.jsonl")
	if err := os.WriteFile(s.planOut, []byte("{\"older\":true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := finalizeVia(t, s, "", "/finalize/confirm", signed())
	if q := location(t, first); q.Get("problem") != "" {
		t.Fatalf("finalize: %s", q.Get("problem"))
	}
	ledger := readFile(t, s.judgments)
	again := finalizeVia(t, s, "ko", "/finalize/confirm", signed())
	q := location(t, again)
	if q.Get("msg") != ui.EncodeNotices([]ui.Notice{ui.N(ui.NoticeFinalizedAlready)}) {
		t.Errorf("second finalize says %q, want the already-finalized message", q.Get("msg"))
	}
	if readFile(t, s.judgments) != ledger {
		t.Error("the second finalize appended to the ledger again")
	}
	if readFile(t, s.planOut+".prev") != "{\"older\":true}\n" {
		t.Error("the plan before finalizing was not kept as plan.json.prev")
	}
	if strings.Contains(location(t, first).Get("msg"), dir) {
		t.Error("the finalize message shows a full path; it should name the file only")
	}
}

// IC-U59 — **저장을 지원하지 않는 플랫폼에서는 보기 전용이다.** 화면은 열리고, 저장은 503으로 거절된다.
func TestViewOnlyRefusesSaves(t *testing.T) {
	s, _ := newServer(t)
	s.st.Close()
	s.st = nil
	if w := get(t, s, "/review"); w.Code != http.StatusOK {
		t.Fatalf("GET /review = %d", w.Code)
	}
	before := readFile(t, s.path)
	w := postForm(t, s, "/save", url.Values{"reviewer": {"보안팀"}})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if readFile(t, s.path) != before {
		t.Error("a view-only server wrote the session")
	}
}

// IC-U60 — **다른 저장이 끝나지 않으면 기다리다 409다.** 기다린 끝에도 잠금이 풀리지 않으면 쓰지 않는다.
func TestBusyTargetIsRefused(t *testing.T) {
	s, dir := newServer(t)
	s.wait = 100 * time.Millisecond
	release, err := s.st.LockTargets([]string{dir}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	before := readFile(t, s.path)
	w := postFormLang(t, s, "/save", "ko", url.Values{"reviewer": {"보안팀"}})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "잠시 뒤") {
		t.Fatalf("status %d\n%s", w.Code, w.Body.String())
	}
	if readFile(t, s.path) != before {
		t.Error("the session was written while another save held the lock")
	}
}

// IC-U61 — **저장 기록과 파일이 어긋나 있으면 503이다.** 복구는 운영자가 한다.
func TestNeedsRecoveryIsRefused(t *testing.T) {
	s, dir := newServer(t)
	s.st.Close()
	stop := errors.New("stopped")
	st, err := store.Open(filepath.Join(dir, store.DirName), store.Options{Warn: func(string) {},
		Fault: func(point, path string) error {
			if point == store.FaultApply && path == "0" {
				return stop
			}
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	id := store.Identity{Kind: kindReview, Org: s.org, SessionID: "other"}
	if err := st.Save(id, []store.Change{{Path: s.path, Content: []byte("{}\n")}}, time.Second); !errors.Is(err, stop) {
		t.Fatalf("err = %v, want the injected stop", err)
	}
	st.Close()
	// 그 사이 파일이 밖에서 바뀐다. 기록과 맞지 않으므로 복구하지 않고 멈춰야 한다.
	if err := os.WriteFile(s.path, []byte("edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.st = openStore(t, dir)
	w := postFormLang(t, s, "/save", "ko", url.Values{"reviewer": {"보안팀"}})
	if w.Code != http.StatusServiceUnavailable || !(strings.Contains(w.Body.String(), "「복구」") || strings.Contains(w.Body.String(), `"Recovery"`)) {
		t.Fatalf("status %d\n%s", w.Code, w.Body.String())
	}
	if readFile(t, s.path) != "edited by hand\n" {
		t.Error("the file changed outside pqcaton was overwritten")
	}
}

// IC-U62 — **시작할 때 지킬 것.** 같은 상태 디렉토리로 서버를 둘 띄우지 않는다. 기록에 없는 준비
// 파일이 있으면 지우지 않고 열지 않는다. 입력에서 세운 세션은 파일로 남겨 세션 ID가 서게 한다.
func TestStartupChecks(t *testing.T) {
	s, dir := withLayers(t)
	if err := s.openState(""); err == nil || !errors.Is(err, store.ErrBusy) {
		t.Fatalf("a second server on the same state directory: err = %v, want ErrBusy", err)
	}
	s.st.Close()
	s.st = nil

	stray := store.PreparedPath(s.path, strings.Repeat("ab", 16))
	if err := os.WriteFile(stray, []byte("?"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.openState(""); !errors.Is(err, store.ErrNeedsRecovery) {
		t.Fatalf("with a stray prepared file: err = %v, want ErrNeedsRecovery", err)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Error("the stray prepared file was removed")
	}
	os.Remove(stray)

	if err := s.openState(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.st.Close() })
	sf, err := scope.LoadSession(s.scope)
	if err != nil || sf.SessionID == "" {
		t.Fatalf("the scope session raised from the layers was not written with an id: %v %+v", err, sf.SessionID)
	}
	again, _, err := s.scopeSession()
	if err != nil || again.SessionID != sf.SessionID {
		t.Errorf("reading the scope session again gave another id: %s → %s", sf.SessionID, again.SessionID)
	}
	if _, err := os.Stat(filepath.Join(dir, store.DirName)); err != nil {
		t.Error("no state directory next to the session")
	}
}

// IC-U63 — **양식은 rev를 싣는다.** 리뷰 · 스코프 · 선언 화면이 지금 파일로 잰 rev를 숨은 필드로 보낸다.
func TestFormsCarryRev(t *testing.T) {
	s, _ := withDecl(t)
	s2, _ := withScope(t)
	for _, c := range []struct {
		srv    *server
		screen string
	}{{s, ui.ScreenReview}, {s, ui.ScreenDecl}, {s2, ui.ScreenScope}} {
		body := get(t, c.srv, c.screen).Body.String()
		want := `<input type="hidden" name="` + ui.RevField + `" value="` + c.srv.revFor(c.screen) + `">`
		if !strings.Contains(body, want) {
			t.Errorf("%s does not carry the current rev", c.screen)
		}
	}
	// 세션이 바뀌면 rev도 바뀐다.
	before := s.revFor(ui.ScreenReview)
	sf, _ := review.Load(s.path)
	sf.Reviewer = "someone"
	if err := review.Save(s.path, sf); err != nil {
		t.Fatal(err)
	}
	if s.revFor(ui.ScreenReview) == before {
		t.Error("the review rev did not change with the session")
	}
}
