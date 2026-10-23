// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-U70 — **복구 필요 상태로 시작하면 일반 화면도 503이다.** 정적 파일(CSS · JS)만 나가고, 화면 ·
// 조각 · 저장은 모두 503 복구 페이지로 가며 운영자 안내서의 「복구」 절을 안내한다. 실행 잠금은 쥔 채라
// 명령도 막힌다. 파일은 바뀌지 않는다.
func TestRecoveryModeBlocksEveryScreen(t *testing.T) {
	s, dir := withDecl(t)
	s.st.Close()
	s.st = nil
	stop := errors.New("stopped")
	st, err := store.Open(filepath.Join(dir, store.DirName), store.Options{Fault: func(point, path string) error {
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
	if err := os.WriteFile(s.path, []byte("edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.openState(""); err != nil {
		t.Fatalf("openState: %v", err)
	}
	t.Cleanup(func() { s.st.Close() })
	if !s.recovering {
		t.Fatal("the server did not enter recovery mode")
	}
	for _, path := range []string{"/", "/review", "/decl", "/classic/decl", "/review-next", ui.ScreenInventory} {
		w := get(t, s, path+"?lang=ko")
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "「복구」") {
			t.Errorf("GET %s: status %d, want 503 pointing at the recovery section", path, w.Code)
		}
	}
	if w := get(t, s, ui.StaticPath+"app.css"); w.Code != http.StatusOK {
		t.Errorf("static CSS: status %d, want 200", w.Code)
	}
	if w := postForm(t, s, "/save", url.Values{"reviewer": {"보안팀"}}); w.Code != http.StatusServiceUnavailable {
		t.Errorf("POST /save: status %d, want 503", w.Code)
	}
	if b, _ := os.ReadFile(s.path); string(b) != "edited by hand\n" {
		t.Error("the file changed outside pqcaton was overwritten")
	}
	cli, err := store.Open(filepath.Join(dir, store.DirName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.AcquireRun(); !errors.Is(err, store.ErrBusy) {
		t.Errorf("a command during recovery mode: err = %v, want ErrBusy", err)
	}
}

// IC-U71 — **입력에서 세운 세션을 파일로 남기지 못하면 서버가 뜨지 않는다.** 그대로 열면 읽을 때마다
// 세션 ID가 새로 나 확정의 동일성이 서지 않는다. 실행 잠금을 놓고, 세션 파일도 준비 파일도 남기지 않는다.
func TestStartupFailsWhenTheSessionCannotBeWritten(t *testing.T) {
	s, dir := withLayers(t)
	s.st.Close()
	s.st = nil
	fail := errors.New("disk full")
	s.storeOpts = store.Options{Fault: func(point, path string) error {
		if point == store.FaultPrepareWrite {
			return fail
		}
		return nil
	}}
	for i := 0; i < 2; i++ {
		if err := s.openState(""); !errors.Is(err, fail) {
			t.Fatalf("attempt %d: err = %v, want the write failure", i+1, err)
		}
		if s.st != nil {
			t.Fatal("the server kept a state directory after failing to start")
		}
	}
	if _, err := os.Stat(s.scope); !os.IsNotExist(err) {
		t.Error("a scope session file was left behind")
	}
	if left, _ := store.FindPrepared(dir); len(left) != 0 {
		t.Errorf("prepared files left: %v", left)
	}
	st, err := store.Open(filepath.Join(dir, store.DirName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.AcquireRun(); err != nil {
		t.Errorf("the run lock was not released: %v", err)
	}
}

// recoveringServer: 저장하다 멈춘 기록을 남기고 그 파일을 밖에서 고친 뒤 연 서버(복구 필요 상태).
func recoveringServer(t *testing.T) (*server, string) {
	t.Helper()
	s, dir := withDecl(t)
	s.st.Close()
	s.st = nil
	stop := errors.New("stopped")
	st, err := store.Open(filepath.Join(dir, store.DirName), store.Options{Fault: func(point, path string) error {
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
	if err := os.WriteFile(s.path, []byte("edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.openState(""); err != nil {
		t.Fatalf("openState: %v", err)
	}
	t.Cleanup(func() { s.st.Close() })
	if !s.recovering {
		t.Fatal("the server did not enter recovery mode")
	}
	return s, dir
}

// IC-U102 — **복구 필요 화면은 무엇이 남았는지 보이고 진단 파일을 내려받게 한다.** 남은 기록마다 파일 이름 ·
// 하려던 일 · 지금 상태(밖에서 바뀜)를 두 말로 보이고, 전체 경로와 해시는 화면에 없다. 진단 파일은
// `-recover-status -diagnostics`와 같은 JSON이고(전체 경로가 든다) 내려받아도 파일은 바뀌지 않는다. 고치는
// 버튼은 없고, 복구 상태가 아니면 내려받는 주소도 없다.
func TestRecoveryScreenShowsStateAndDiagnostics(t *testing.T) {
	s, dir := recoveringServer(t)
	for _, c := range []struct{ lang, head, verdict string }{
		{"ko", "복구가 필요합니다", "<td>pqcaton 밖에서 바뀜: 덮어쓰지 않습니다</td>"},
		{"en", "Recovery required", "<td>changed outside pqcaton: nothing will overwrite it</td>"},
	} {
		w := get(t, s, "/review?lang="+c.lang)
		body := w.Body.String()
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status %d", c.lang, w.Code)
		}
		for _, want := range []string{c.head, c.verdict, "<code>" + filepath.Base(s.path) + "</code>", `href="` + ui.RecoveryDiagPath + `"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: no %q", c.lang, want)
			}
		}
		if strings.Contains(body, dir) || strings.Contains(body, "<form") {
			t.Errorf("%s: the recovery screen shows a path or offers a form", c.lang)
		}
		if c.lang == "en" && hasHangul(visible(body)) {
			t.Error("en: Korean on the English recovery screen")
		}
	}
	w := get(t, s, ui.RecoveryDiagPath)
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("diagnostics: %d %v", w.Code, w.Header())
	}
	var rep store.Report
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil || len(rep.Records) != 1 || rep.Records[0].Items[0].AbsPath != s.path ||
		rep.Records[0].Items[0].Verdict != store.VerdictMismatch {
		t.Errorf("diagnostics: %v %+v", err, rep)
	}
	if b, _ := os.ReadFile(s.path); string(b) != "edited by hand\n" {
		t.Error("the recovery screen changed the file")
	}

	// 실행기가 복구 화면도 열 수 있게 상태 ID는 답한다.
	if w := get(t, s, ui.HealthPath); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != s.st.ID {
		t.Errorf("health during recovery: %d %q", w.Code, w.Body.String())
	}

	ok, _ := newServer(t)
	if w := get(t, ok, ui.RecoveryDiagPath); w.Code != http.StatusNotFound {
		t.Errorf("diagnostics outside recovery: %d, want 404", w.Code)
	}
}
