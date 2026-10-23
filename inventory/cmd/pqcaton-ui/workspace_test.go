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
	oldServe, oldOpen, oldStop := serveHTTP, openBrowser, stopSignal
	t.Cleanup(func() { serveHTTP, openBrowser, stopSignal = oldServe, oldOpen, oldStop })
	stopSignal = func() <-chan os.Signal { c := make(chan os.Signal); close(c); return c }
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
	code = run(append(append([]string(nil), args...), "-addr", "127.0.0.1:0"), &out, &errb)
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
	if c.base != "" || c.scope != filepath.Join(ws, "scope-session.json") || len(c.layers) != 0 {
		t.Errorf("an empty workspace: base %q scope %q layers %v", c.base, c.scope, c.layers)
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
// 시작하지 않는다(사용법 오류). 플래그 뒤에 세션 파일을 두어도 작업 폴더로 뜨지 않고 사용법 오류다. 동작 플래그는 함께 쓸 수 있고, 세션 파일과 경로 플래그(명시 모드)는 그대로다.
func TestStartModesDoNotMix(t *testing.T) {
	ws := t.TempDir()
	for _, args := range [][]string{
		{"-workspace", ws, "-decl", "x.json"},
		{"-workspace", ws, "-state-dir", "s"},
		{"session.json", "-workspace", ws},
		{"-addr", "127.0.0.1:0", "session.json", "-decl", "d.json"},
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

// IC-U99 — **선언의 앞뒤가 안 맞는 자리를 01 화면이 칸 단위로 짚는다.** 어느 노드 · 엣지의 어느 칸이, 무엇이,
// 그대로 두면 어떻게 되는지를 두 말로 보인다. 01 카드에 그 수가 오르고, 판정 화면의 경고는 `pqcaton-report`로
// 보내지 않고 01 화면을 가리킨다.
func TestDeclProblemsAreListedByField(t *testing.T) {
	ws := t.TempDir()
	d := decl.Declaration{Org: "acme", Scope: []string{"web", "db"},
		Nodes:  []decl.Node{{Name: "web", IPs: []string{"10.0.0.1:443"}}, {Name: "db", IPs: []string{"10.0.0.2"}}},
		Assets: []decl.Asset{}, Edges: []decl.Edge{{Src: "web", Dst: "db", Port: 0, Proto: "tcp"}}}
	b, err := decl.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "declaration.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	h, code, _, stderr := started(t, "-workspace", ws, "-no-browser")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, stderr)
	}
	for _, c := range []struct{ lang, head, ip, port, why, warn string }{
		{"ko", "앞뒤가 맞지 않는 자리", "노드 web</strong> · IP: IP 주소가 아닙니다", "엣지 web → db</strong> · 포트: 포트가 0입니다", "그대로 두면", "01 선언 화면이 보여 줍니다"},
		{"en", "Places that do not add up", "Node web</strong> · IP: not an IP address", "Edge web → db</strong> · Port: the port is 0", "If left as is", "The 01 declaration screen shows where"},
	} {
		body := do(h, newReq(http.MethodGet, "/decl?lang="+c.lang, nil)).Body.String()
		for _, want := range []string{c.head, c.ip, c.port, c.why} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: no %q in the declaration screen", c.lang, want)
			}
		}
		if strings.Contains(body, "**") || strings.Contains(body, "pqcaton-report") {
			t.Errorf("%s: the declaration screen shows emphasis marks or sends people to pqcaton-report", c.lang)
		}
		if c.lang == "en" && hasHangul(visible(body)) {
			t.Error("en: Korean on the English declaration screen")
		}
		if got := cards(t, body)["01"]; !strings.Contains(got, "2") {
			t.Errorf("%s: the 01 card does not count the problems: %q", c.lang, got)
		}
		review := do(h, newReq(http.MethodGet, "/review?lang="+c.lang, nil)).Body.String()
		if strings.Contains(review, "pqcaton-report") || !strings.Contains(review, c.warn) {
			t.Errorf("%s: the review warning does not point at the 01 screen", c.lang)
		}
	}
}

// IC-U100 — **관측 결과 폴더가 비어 있으면 대조 · 판정 화면이 그 폴더를 가리킨다.** 결과 파일을 넣으면 다시
// 불러오는 것만으로 안내가 사라진다(GET은 아무것도 쓰지 않는다).
func TestEmptyResultsPointAtTheFolder(t *testing.T) {
	ws := t.TempDir()
	h, _, _, _ := started(t, "-workspace", ws, "-no-browser")
	if w := do(h, posted(ui.DeclCreatePath, url.Values{"org": {"acme"}})); w.Code != http.StatusSeeOther {
		t.Fatalf("create: %d", w.Code)
	}
	results := filepath.Join(ws, "results")
	for _, path := range []string{"/survey", "/review"} {
		for _, c := range []struct{ lang, want string }{{"ko", "아직 관측 결과가 없습니다"}, {"en", "No observed results yet"}} {
			body := do(h, newReq(http.MethodGet, path+"?lang="+c.lang, nil)).Body.String()
			if !strings.Contains(body, c.want) || !strings.Contains(body, "<code>"+results+"</code>") {
				t.Errorf("%s (%s): no guidance to the results folder", path, c.lang)
			}
		}
	}
	session, _ := os.ReadFile(filepath.Join(ws, "session.json"))
	if err := os.WriteFile(filepath.Join(results, "node-a.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/survey", "/review"} {
		if body := do(h, newReq(http.MethodGet, path+"?lang=ko", nil)).Body.String(); strings.Contains(body, "아직 관측 결과가 없습니다") {
			t.Errorf("%s: the guidance stays after a result arrived", path)
		}
	}
	if after, _ := os.ReadFile(filepath.Join(ws, "session.json")); string(after) != string(session) {
		t.Error("a GET rewrote the review session")
	}
}

// IC-U101 — **계층이 없는 작업 폴더에서 화면으로 계층을 만든다.** GET은 만들기 폼만 보이고 아무것도 만들지 않는다.
// 02 카드는 「아직 계층이 없습니다」다. POST가 `layers/<이름>.csv`와 스코프 세션을 만들고 화면은 규칙 편집이
// 된다. 같은 이름을 다시 보내면 덮어쓰지 않고 409, 틀린 이름은 400이다. 계층이 생기기 전의 스코프 저장 · 확정은
// 404다. 명시 모드에는 이 주소가 없다.
func TestLayerIsCreatedFromTheScreen(t *testing.T) {
	ws := t.TempDir()
	h, _, _, _ := started(t, "-workspace", ws, "-no-browser")
	body := do(h, newReq(http.MethodGet, "/scope?lang=ko", nil)).Body.String()
	if !strings.Contains(body, `action="`+ui.LayerCreatePath+`"`) {
		t.Fatalf("no create form:\n%s", body)
	}
	if got := cards(t, body)["02"]; !strings.HasPrefix(got, "아직 계층이 없습니다") {
		t.Errorf("02 card %q", got)
	}
	if ents, _ := os.ReadDir(filepath.Join(ws, "layers")); len(ents) != 0 {
		t.Fatal("GET created a layer")
	}
	if _, err := os.Stat(filepath.Join(ws, "scope-session.json")); err == nil {
		t.Fatal("GET created the scope session")
	}
	for _, path := range []string{"/scope/save", "/scope/finalize/confirm", "/scope/finalize"} {
		if w := do(h, posted(path+"?lang=ko", url.Values{})); w.Code != http.StatusNotFound {
			t.Errorf("%s before a layer: %d, want 404", path, w.Code)
		}
	}
	if w := do(h, posted(ui.LayerCreatePath+"?lang=ko", url.Values{"name": {"../evil"}})); w.Code != http.StatusBadRequest {
		t.Errorf("bad name: %d, want 400", w.Code)
	}
	w := do(h, posted(ui.LayerCreatePath+"?lang=ko", url.Values{"name": {"corp"}}))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("create: %d\n%s", w.Code, w.Body.String())
	}
	layer := filepath.Join(ws, "layers", "corp.csv")
	before, err := os.ReadFile(layer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws, "scope-session.json")); err != nil {
		t.Errorf("the scope session was not raised in the same request: %v", err)
	}
	if body := do(h, newReq(http.MethodGet, "/scope?lang=ko", nil)).Body.String(); strings.Contains(body, `action="`+ui.LayerCreatePath+`"`) || !strings.Contains(body, `action="/scope/rules"`) {
		t.Error("after creating, the scope screen is not the rule editor")
	}
	if w := do(h, posted(ui.LayerCreatePath+"?lang=ko", url.Values{"name": {"corp"}})); w.Code != http.StatusConflict {
		t.Errorf("second create: %d, want 409", w.Code)
	}
	if after, _ := os.ReadFile(layer); string(after) != string(before) {
		t.Error("the second create overwrote the layer")
	}

	s, _ := newServer(t)
	if w := serve(s.handler(), posted(ui.LayerCreatePath, url.Values{"name": {"corp"}})); w.Code != http.StatusNotFound {
		t.Errorf("explicit mode: %d, want 404", w.Code)
	}
}

// IC-U106 — **화면을 끝내면(Ctrl+C · systemctl stop) 적어 둔 주소를 지우고 0으로 끝난다.** 남겨 두면 다시 띄울 때
// 실행기가 아직 열리지 않은 낡은 주소를 먼저 연다. 끝난 뒤에는 실행 잠금도 풀려 다시 띄울 수 있다.
func TestStopRemovesTheRecordedAddress(t *testing.T) {
	ws := t.TempDir()
	sig := make(chan os.Signal, 1)
	oldStop, oldOpen := stopSignal, openBrowser
	t.Cleanup(func() { stopSignal, openBrowser = oldStop, oldOpen })
	stopSignal = func() <-chan os.Signal { return sig }
	openBrowser = func(string, io.Writer) {}
	done := make(chan int)
	var errb bytes.Buffer
	go func() { done <- run([]string{"-workspace", ws, "-addr", "127.0.0.1:0"}, io.Discard, &errb) }()
	addr := filepath.Join(ws, ".pqcaton", addrFile)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if b, err := os.ReadFile(addr); err == nil && len(b) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the screen did not record its address")
		}
		time.Sleep(20 * time.Millisecond)
	}
	sig <- os.Interrupt
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("stopped with code %d\n%s", code, errb.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the screen did not stop")
	}
	if _, err := os.Stat(addr); !os.IsNotExist(err) {
		t.Errorf("the recorded address is still there: %v", err)
	}
	if _, code, _, stderr := started(t, "-workspace", ws, "-no-browser"); code != 0 {
		t.Errorf("could not start again after stopping: %d\n%s", code, stderr)
	}
}
