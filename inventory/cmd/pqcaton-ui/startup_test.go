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

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-U70 — **복구 필요 상태로 시작하면 일반 화면도 503이다.** 정적 파일(CSS · JS)만 나가고, 화면 ·
// 조각 · 저장은 모두 503 복구 페이지로 가며 `-recover-status`를 안내한다. 실행 잠금은 쥔 채라
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
	for _, path := range []string{"/", "/review", "/decl", ui.Classic(ui.ScreenDecl), "/review-next", ui.ScreenInventory} {
		w := get(t, s, path+"?lang=ko")
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "-recover-status") {
			t.Errorf("GET %s: status %d, want 503 naming -recover-status", path, w.Code)
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
