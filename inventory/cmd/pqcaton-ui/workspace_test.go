// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// started: run을 돌려 처리기를 받아 온다. 서버는 띄우지 않는다(serveHTTP를 바꿔 낀다). 브라우저를 연 주소도 돌려준다.
func started(t *testing.T, args ...string) (h http.Handler, code int, opened []string, stderr string) {
	t.Helper()
	oldServe, oldOpen := serveHTTP, openBrowser
	t.Cleanup(func() { serveHTTP, openBrowser = oldServe, oldOpen })
	serveHTTP = func(ln net.Listener, handler http.Handler) error {
		h = handler
		return ln.Close()
	}
	openBrowser = func(u string, _ io.Writer) {
		// 브라우저를 부를 때 포트가 이미 열려 있어야 한다.
		parsed, err := url.Parse(u)
		if err != nil {
			t.Errorf("browser url %q: %v", u, err)
		} else if c, err := net.DialTimeout("tcp", parsed.Host, time.Second); err != nil {
			t.Errorf("the browser was opened before the port: %v", err)
		} else {
			c.Close()
		}
		opened = append(opened, u)
	}
	var out, errb bytes.Buffer
	code = run(append([]string{"-addr", "127.0.0.1:0"}, args...), &out, &errb)
	return h, code, opened, errb.String()
}

// posted: 화면이 보내는 그대로의 POST(요청 보호 토큰을 싣는다).
func posted(target string, form url.Values) *http.Request {
	form.Set(ui.CSRFField, testToken)
	return withForm(goodPost(target), form)
}

func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// IC-U93 — **작업 폴더는 파일 위치가 정해져 있다.** 인자가 없으면 홈의 `pqcaton`을 쓰고, 없으면 소유자 전용(0700)으로
// 만든다. `results/` · `layers/`도 만든다. 계층은 `layers/`의 CSV를 이름 순으로 겹치고, 확정 정책 파일이 있으면
// 그것이 기준이다.
func TestWorkspaceFixesEveryPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var errb bytes.Buffer
	c, err := parseStart(nil, &errb)
	if err != nil {
		t.Fatal(err)
	}
	ws := filepath.Join(home, "pqcaton")
	if c.workspace != ws {
		t.Fatalf("workspace %q, want %q", c.workspace, ws)
	}
	for _, d := range []string{ws, filepath.Join(ws, "results"), filepath.Join(ws, "layers")} {
		fi, err := os.Stat(d)
		if err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: %v %v, want a 0700 folder", d, fi, err)
		}
	}
	want := map[string]string{"session": "session.json", "decl": "declaration.json", "results": "results",
		"plan": "plan.json", "judgments": "judgments.jsonl", "scopeOut": "asset-scope.csv"}
	got := map[string]string{"session": c.session, "decl": c.decl, "results": c.results, "plan": c.plan,
		"judgments": c.judgments, "scopeOut": c.scopeOut}
	for k, name := range want {
		if got[k] != filepath.Join(ws, name) {
			t.Errorf("%s = %q, want %q", k, got[k], filepath.Join(ws, name))
		}
	}
	if c.base != "" || c.scope != "" || len(c.layers) != 0 {
		t.Errorf("an empty workspace opened scope files: base %q scope %q layers %v", c.base, c.scope, c.layers)
	}

	for _, f := range []string{"layers/b-prod.csv", "layers/a-corp.csv", "layers/notes.txt", "asset-scope.csv"} {
		if err := os.WriteFile(filepath.Join(ws, f), []byte("action,runtime,lib,app_key,note\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c, err = parseStart([]string{"-workspace", ws}, &errb)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(c.layers, ",") != filepath.Join(ws, "layers/a-corp.csv")+","+filepath.Join(ws, "layers/b-prod.csv") {
		t.Errorf("layers %v, want the CSVs in name order", c.layers)
	}
	if c.base != filepath.Join(ws, "asset-scope.csv") || c.scope != filepath.Join(ws, "scope-session.json") {
		t.Errorf("base %q scope %q", c.base, c.scope)
	}
}

// IC-U94 — **두 시작 모드를 섞지 않는다.** 작업 폴더와 경로 플래그를 함께 주거나, 세션 파일과 작업 폴더를 함께 주면
// 시작하지 않는다(사용법 오류). 동작 플래그는 함께 쓸 수 있고, 세션 파일과 경로 플래그(명시 모드)는 그대로다.
func TestStartModesDoNotMix(t *testing.T) {
	ws := t.TempDir()
	for _, args := range [][]string{
		{"-workspace", ws, "-decl", "x.json"},
		{"-workspace", ws, "-state-dir", "s"},
		{"session.json", "-workspace", ws},
	} {
		var errb bytes.Buffer
		if _, err := parseStart(args, &errb); err != errUsage {
			t.Errorf("%v: err %v, want a usage error", args, err)
		}
	}
	var errb bytes.Buffer
	if c, err := parseStart([]string{"-workspace", ws, "-addr", "127.0.0.1:0", "-no-browser", "-org", "acme"}, &errb); err != nil || !c.noBrowser || c.org != "acme" {
		t.Errorf("behaviour flags with a workspace: %+v, %v", c, err)
	}
	if c, err := parseStart([]string{"s.json", "-decl", "d.json"}, &errb); err != nil || c.workspace != "" || c.decl != "d.json" {
		t.Errorf("explicit mode: %+v, %v", c, err)
	}
}

// IC-U95 — **포트를 연 뒤에 브라우저를 연다.** 브라우저를 부를 때 그 주소에 이미 접속된다. `-no-browser`면 열지 않는다.
// 브라우저를 열지 못해도(명령이 없음) 경고만 하고 서버는 계속한다.
func TestBrowserOpensAfterThePort(t *testing.T) {
	ws := t.TempDir()
	if _, code, opened, _ := started(t, "-workspace", ws); code != 0 || len(opened) != 1 {
		t.Fatalf("code %d, opened %v", code, opened)
	}
	if _, code, opened, _ := started(t, "-workspace", t.TempDir(), "-no-browser"); code != 0 || len(opened) != 0 {
		t.Errorf("-no-browser: code %d, opened %v", code, opened)
	}
	t.Setenv("PATH", "")
	var errb bytes.Buffer
	realOpenBrowser("http://127.0.0.1:1/", &errb)
	if !strings.Contains(errb.String(), "warning: could not open a browser") {
		t.Errorf("no warning when the browser could not be opened: %q", errb.String())
	}
}

// realOpenBrowser: 바꿔 끼우기 전의 openBrowser.
var realOpenBrowser = openBrowser

// IC-U96 — **같은 작업 폴더로 이미 떠 있으면 새로 띄우지 않고 그 화면을 연다.** 적어 둔 주소가 루프백이 아니면 열지
// 않고 실패한다.
func TestRunningScreenIsReopened(t *testing.T) {
	// 떠 있는 화면 대신 포트만 연다.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	for _, c := range []struct {
		addr string
		code int
		open bool
	}{{ln.Addr().String(), 0, true}, {"192.0.2.1:80", 1, false}} {
		ws := t.TempDir()
		var errb bytes.Buffer
		if _, err := parseStart([]string{"-workspace", ws}, &errb); err != nil {
			t.Fatal(err)
		}
		st, err := store.Open(filepath.Join(ws, ".pqcaton"), store.Options{Warn: func(string) {}})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AcquireRun(); err != nil {
			t.Fatal(err)
		}
		if err := recordAddr(st.Dir, c.addr); err != nil {
			t.Fatal(err)
		}
		h, code, opened, _ := started(t, "-workspace", ws)
		st.Close()
		if code != c.code || h != nil {
			t.Errorf("%s: code %d, served %v; want %d and no second server", c.addr, code, h != nil, c.code)
		}
		if c.open != (len(opened) == 1 && opened[0] == "http://"+c.addr+"/") {
			t.Errorf("%s: opened %v", c.addr, opened)
		}
	}
}

// IC-U97 — **작업 폴더의 입력이 깨졌으면 진단 전용으로 연다.** 서버는 뜨고, 모든 화면이 503 진단 페이지다(읽지 못한
// 파일의 역할과 이름 · 참조 번호, 경로 없음). 저장도 막힌다. 원문과 경로는 표준오류에 같은 참조 번호로 남는다.
func TestBrokenWorkspaceOpensForDiagnostics(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "declaration.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, code, _, stderr := started(t, "-workspace", ws, "-no-browser")
	if code != 0 || h == nil {
		t.Fatalf("the server did not start: code %d\n%s", code, stderr)
	}
	ref := stderr[strings.Index(stderr, "error ref=")+len("error ref=") : strings.Index(stderr, "error ref=")+len("error ref=")+8]
	for _, req := range []*http.Request{newReq(http.MethodGet, "/review?lang=ko", nil), posted("/decl/save?lang=ko", url.Values{})} {
		w := do(h, req)
		body := w.Body.String()
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(body, "진단 전용") || !strings.Contains(body, "declaration.json") || !strings.Contains(body, ref) {
			t.Errorf("%s %s: status %d\n%s", req.Method, req.URL.Path, w.Code, body)
		}
		if strings.Contains(body, ws) {
			t.Errorf("%s: the page shows the workspace path", req.URL.Path)
		}
	}
	if !strings.Contains(stderr, filepath.Join(ws, "declaration.json")) {
		t.Error("the full path is not in the server log")
	}
}

// IC-U98 — **선언이 없는 작업 폴더에서 화면으로 선언을 만든다.** GET은 만들기 폼만 보이고 아무것도 만들지 않는다.
// 대조 · 판정 · 조회는 닫혀 있다. POST가 빈 선언과 리뷰 세션을 만들고, 다시 보내면 덮어쓰지 않고 409다.
func TestDeclarationIsCreatedFromTheScreen(t *testing.T) {
	ws := t.TempDir()
	h, code, _, stderr := started(t, "-workspace", ws, "-no-browser")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, stderr)
	}
	declPath := filepath.Join(ws, "declaration.json")
	body := do(h, newReq(http.MethodGet, "/decl?lang=ko", nil)).Body.String()
	if !strings.Contains(body, `action="`+ui.DeclCreatePath+`"`) || !strings.Contains(body, "아직 선언이 없습니다") {
		t.Fatalf("no create form:\n%s", body)
	}
	if _, err := os.Stat(declPath); err == nil {
		t.Fatal("GET created the declaration")
	}
	if w := do(h, newReq(http.MethodGet, "/review?lang=ko", nil)); w.Code != http.StatusNotFound {
		t.Errorf("review before a declaration: %d, want 404", w.Code)
	}
	got := cards(t, body)
	if !strings.HasPrefix(got["03"], "입력 파일을 주지 않았습니다") || !strings.HasPrefix(got["04"], "입력 파일을 주지 않았습니다") {
		t.Errorf("survey and review are not closed: %v", got)
	}

	w := do(h, posted(ui.DeclCreatePath+"?lang=ko", url.Values{"org": {"acme"}}))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("create: %d\n%s", w.Code, w.Body.String())
	}
	d, err := decl.Load(declPath)
	if err != nil || d.Org != "acme" {
		t.Fatalf("the declaration: %+v, %v", d, err)
	}
	if _, err := os.Stat(filepath.Join(ws, "session.json")); err != nil {
		t.Errorf("the review session was not raised in the same request: %v", err)
	}
	before, _ := os.ReadFile(declPath)
	if w := do(h, posted(ui.DeclCreatePath+"?lang=ko", url.Values{"org": {"other"}})); w.Code != http.StatusConflict {
		t.Errorf("second create: %d, want 409", w.Code)
	}
	if after, _ := os.ReadFile(declPath); string(after) != string(before) {
		t.Error("the second create overwrote the declaration")
	}
	if body := do(h, newReq(http.MethodGet, "/decl?lang=ko", nil)).Body.String(); !strings.Contains(body, `action="/decl/save"`) {
		t.Error("after creating, the declaration screen is not the editor")
	}
}
