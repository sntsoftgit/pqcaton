package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

func session() review.Session {
	return review.Session{
		Note: review.Note, Scope: "host://local",
		PolicyDecisions: map[string]string{"openssl/libssl": ""},
		Items: []review.Item{
			{ID: "host://local/openssl/libssl", Policy: "openssl/libssl",
				Node: "host://local", Runtime: "openssl", State: "UNDECLARED",
				Conf: 0.6, Mandatory: true},
		},
		Autopass: []string{"host://local/openssl/libcrypto"},
	}
}

func newServer(t *testing.T) (*server, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	if err := review.Save(path, session()); err != nil {
		t.Fatal(err)
	}
	return &server{path: path, org: "acme", planOut: filepath.Join(dir, "plan.json")}, dir
}

func postForm(t *testing.T, s *server, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	s.routes(mux)
	mux.ServeHTTP(w, req)
	return w
}

// location — 리다이렉트가 실어 보낸 메시지. 화면이 무엇을 말하는지는 여기 담긴다.
func location(t *testing.T, w *httptest.ResponseRecorder) url.Values {
	t.Helper()
	if w.Code != http.StatusSeeOther {
		t.Fatalf("상태 %d (본문: %s)", w.Code, w.Body.String())
	}
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

// IC-U1 — **정책으로 묶어 보여 준다.** 그것이 판정의 기본 단위다(§3.4) — 항목을 한 줄씩
// 늘어놓으면 화면이 있어도 수천 대에서 리뷰가 끝나지 않는다.
func TestIndexGroupsByPolicy(t *testing.T) {
	s, _ := newServer(t)
	mux := http.NewServeMux()
	s.routes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("상태 %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"openssl/libssl",               // 정책 이름
		`name="policy:openssl/libssl"`, // 정책 단위 입력칸
		"host://local/openssl/libssl",  // 항목
		"자동통과 후보 1개",                   // 자동통과는 세어서 고지한다
	} {
		if !strings.Contains(body, want) {
			t.Errorf("화면에 %q 가 없다", want)
		}
	}
}

// IC-U2 — **저장은 확정이 아니다.** 채우다 만 것을 확정으로 오해하면 감사 기록이 거짓이 된다.
func TestSaveDoesNotFinalize(t *testing.T) {
	s, _ := newServer(t)
	q := location(t, postForm(t, s, "/save", url.Values{
		"policy:openssl/libssl": {"교체한다"},
		"reviewer":              {"보안팀"},
	}))
	if q.Get("problem") != "" {
		t.Fatalf("저장이 실패했다: %s", q.Get("problem"))
	}

	sf, err := review.Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if sf.PolicyDecisions["openssl/libssl"] != "교체한다" {
		t.Errorf("결론이 세션 파일에 안 남았다: %+v", sf.PolicyDecisions)
	}
	if _, err := os.Stat(s.planOut); !os.IsNotExist(err) {
		t.Error("저장만 눌렀는데 확정 계획이 생겼다")
	}
}

// IC-U3 — **화면도 같은 게이트를 탄다.** 서명이 없으면 확정되지 않고, **무엇이 남았는지**
// 화면에 그대로 보인다 — 안 보여 주면 사람은 화면에서도 고칠 수 없다.
func TestFinalizeRefusesWithoutSignature(t *testing.T) {
	s, _ := newServer(t)
	q := location(t, postForm(t, s, "/finalize", url.Values{
		"policy:openssl/libssl": {"교체한다"},
		"reviewer":              {"보안팀"}, // 서명 없음
	}))
	if q.Get("problem") == "" {
		t.Fatal("서명 없이 확정됐다")
	}
	if !strings.Contains(q.Get("problem"), "signature") {
		t.Errorf("무엇이 남았는지 말하지 않는다: %s", q.Get("problem"))
	}
	if _, err := os.Stat(s.planOut); !os.IsNotExist(err) {
		t.Error("확정되지 않았는데 계획 파일이 생겼다")
	}
}

// IC-U4 — 통과하면 **계약 형식 계획**이 파일로 나오고 판정이 원장에 남는다.
func TestFinalizeWritesPlanAndJudgments(t *testing.T) {
	s, dir := newServer(t)
	s.judgments = filepath.Join(dir, "judgments.jsonl")

	q := location(t, postForm(t, s, "/finalize", url.Values{
		"policy:openssl/libssl":            {"PQC 라이브러리로 교체한다"},
		"plan:host://local/openssl/libssl": {"on"},
		"reviewer":                         {"보안팀"},
		"signature":                        {"sig"},
	}))
	if p := q.Get("problem"); p != "" {
		t.Fatalf("확정되지 않았다: %s", p)
	}

	raw, err := os.ReadFile(s.planOut)
	if err != nil {
		t.Fatalf("확정 계획이 없다: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "PLAN_STATUS_FINALIZED") {
		t.Errorf("확정 상태가 아니다:\n%s", body)
	}
	// v0.1.1 에서 고친 자리다 — 겨눈 노드가 쪼개져 사라지면 안 된다.
	if !strings.Contains(body, `"targetNodeId":  "host://local"`) {
		t.Errorf("겨눈 노드가 틀렸다:\n%s", body)
	}

	led, err := os.ReadFile(s.judgments)
	if err != nil {
		t.Fatalf("판정 원장이 없다: %v", err)
	}
	if !strings.Contains(string(led), `"org":"acme"`) {
		t.Errorf("판정이 조직에 묶이지 않았다: %s", led)
	}
}

// IC-U5 — **화면이 쓴 파일을 명령이 그대로 읽는다.** 두 길이 갈리면 화면으로 채운 것을
// close 가 못 읽는 날이 온다.
func TestSavedSessionStaysReadable(t *testing.T) {
	s, _ := newServer(t)
	location(t, postForm(t, s, "/save", url.Values{
		"item:host://local/openssl/libssl": {"예외로 둔다"},
		"plan:host://local/openssl/libssl": {"on"},
		"reviewer":                         {"보안팀"},
		"signature":                        {"sig"},
	}))

	sf, err := review.Load(s.path)
	if err != nil {
		t.Fatalf("명령이 읽지 못한다: %v", err)
	}
	if len(sf.Items) != 1 || sf.Items[0].Conclusion != "예외로 둔다" || !sf.Items[0].Plan {
		t.Fatalf("항목이 온전하지 않다: %+v", sf.Items)
	}
	// 기계가 채우는 값이 화면 왕복에서 사라지면 안 된다 — node 가 비면 확정이 끊긴다.
	if sf.Items[0].Node == "" || sf.Items[0].Runtime == "" || sf.Items[0].State == "" {
		t.Errorf("기계가 채운 값이 사라졌다: %+v", sf.Items[0])
	}
	if _, err := review.Finalize(sf); err != nil {
		t.Fatalf("화면으로 채운 세션이 확정되지 않는다: %v", err)
	}
}

// IC-U6 — **루프백을 가려낸다.** 리뷰 큐에는 어느 노드가 무엇을 쓰는지가 그대로 있다 —
// 실수로 밖에 열리는 쪽이 편의보다 훨씬 비싸므로, 밖으로 열리면 경고한다.
func TestLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8765": true,
		"localhost:8765": true,
		"[::1]:8765":     true,
		"0.0.0.0:8765":   false,
		":8765":          false,
		"10.0.0.5:8765":  false,
	} {
		if got := loopback(addr); got != want {
			t.Errorf("loopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

// IC-U8 — **위치 인자 뒤의 플래그도 먹는다.**
//
// 표준 flag 는 첫 비플래그에서 파싱을 멈춘다. 그냥 두면 `pqcaton-ui session.json -addr ...`
// 의 -addr 이 **조용히 무시되고** 기본 주소로 뜬다 — 밖으로 열려고 준 값이 안 먹는 것도,
// 안쪽으로 좁히려던 값이 안 먹는 것도 같은 자리다.
func TestSplitArgs(t *testing.T) {
	pos, flags := splitArgs([]string{"session.json", "-addr", "127.0.0.1:9", "-org", "acme"})
	if len(pos) != 1 || pos[0] != "session.json" {
		t.Fatalf("위치 인자 = %v", pos)
	}
	if len(flags) != 4 || flags[0] != "-addr" {
		t.Fatalf("플래그 = %v", flags)
	}
	if p, f := splitArgs([]string{"-addr", "x"}); len(p) != 0 || len(f) != 2 {
		t.Errorf("플래그만 준 경우 = %v / %v", p, f)
	}
}

// IC-U7 — GET 만 받는 자리에 POST 를, POST 만 받는 자리에 GET 을 던져도 조용히 넘어가지
// 않는다. 새로고침으로 확정이 다시 도는 것을 막는 것도 같은 이유다.
func TestMethodGuards(t *testing.T) {
	s, _ := newServer(t)
	mux := http.NewServeMux()
	s.routes(mux)
	for _, path := range []string{"/save", "/finalize"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s = %d, want 405", path, w.Code)
		}
	}
}

// ── 선언 편집 화면 ─────────────────────────────────────────────────────────

const declJSON = `{"org":"acme","scope":["web-gw","pay-db"],
 "nodes":[{"name":"web-gw","ips":["10.0.0.1"]}],
 "assets":[{"node":"web-gw","runtime":"openssl","component":"libssl"}],
 "edges":[{"src":"web-gw","dst":"pay-db","port":5432,"proto":"TLS"}]}`

func withDecl(t *testing.T) (*server, string) {
	t.Helper()
	s, dir := newServer(t)
	s.decl = filepath.Join(dir, "declaration.json")
	if err := os.WriteFile(s.decl, []byte(declJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func get(t *testing.T, s *server, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	s.routes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

// IC-U9 — **선언 파일을 주지 않으면 그 자리를 만들지 않는다.** 없는 것을 눌러 보게 하면
// 쓰는 사람이 무엇이 되고 무엇이 안 되는지 헷갈린다.
func TestDeclHiddenWithoutFile(t *testing.T) {
	s, _ := newServer(t)
	if body := get(t, s, "/").Body.String(); strings.Contains(body, `href="/decl"`) {
		t.Error("선언 파일이 없는데 이동 링크가 보인다")
	}
	if code := get(t, s, "/decl").Code; code != http.StatusNotFound {
		t.Errorf("/decl = %d, want 404", code)
	}

	s2, _ := withDecl(t)
	if body := get(t, s2, "/").Body.String(); !strings.Contains(body, `href="/decl"`) {
		t.Error("선언 파일을 줬는데 이동 링크가 없다")
	}
}

// IC-U10 — **틀린 자리를 저장 전에 짚는다.**
//
// 노드↔IP 가 없으면 관측 IP 가 해소되지 않아 CONFIRMED 여야 할 통신이 shadow 로 올라온다 —
// 오류가 아니라 그럴듯한 결과라 눈으로는 안 잡힌다. 화면이 미리 말해 주는 자리다.
func TestDeclShowsProblems(t *testing.T) {
	s, _ := withDecl(t)
	body := get(t, s, "/decl").Body.String()
	// pay-db 는 스코프에 있는데 IP 표에 없다.
	if !strings.Contains(body, "IP 표에 없습니다") {
		t.Errorf("해소되지 않을 노드를 짚지 않는다:\n%s", body)
	}
	if !strings.Contains(body, "shadow") {
		t.Error("그대로 두면 무슨 일이 생기는지 말하지 않는다")
	}
}

// IC-U11 — **저장한 것을 명령이 그대로 읽는다.** 두 길이 갈리면 화면으로 고친 선언을
// pqcaton-report 가 못 읽는 날이 온다.
func TestDeclSaveRoundTrips(t *testing.T) {
	s, _ := withDecl(t)
	q := location(t, postForm(t, s, "/decl/save", url.Values{
		"org":          {"acme"},
		"scope":        {"web-gw\npay-db"},
		"node.name.0":  {"web-gw"},
		"node.ips.0":   {"10.0.0.1, 10.0.1.1"},
		"node.name.1":  {"pay-db"},
		"node.ips.1":   {"10.0.0.2"},
		"asset.node.0": {"web-gw"}, "asset.runtime.0": {"openssl"}, "asset.component.0": {"libssl"},
		"edge.src.0": {"web-gw"}, "edge.dst.0": {"pay-db"}, "edge.port.0": {"5432"}, "edge.proto.0": {"TLS"},
	}))
	if p := q.Get("problem"); p != "" {
		t.Fatalf("저장이 실패했다: %s", p)
	}

	d, err := decl.Load(s.decl)
	if err != nil {
		t.Fatalf("명령이 읽지 못한다: %v", err)
	}
	if len(d.Nodes) != 2 || len(d.Nodes[0].IPs) != 2 {
		t.Fatalf("노드가 온전하지 않다: %+v", d.Nodes)
	}
	if len(d.Scope) != 2 || len(d.Assets) != 1 || len(d.Edges) != 1 {
		t.Fatalf("선언이 온전하지 않다: %+v", d)
	}
	// 다 맞췄으므로 짚을 것이 없어야 한다.
	if p := decl.Check(d); len(p) != 0 {
		t.Errorf("맞는 선언인데 %d곳을 짚는다: %+v", len(p), p)
	}
}

// IC-U12 — **이름을 비우면 그 줄이 지워진다.** 표에서 줄을 없애는 유일한 방법이라, 얹기만
// 하면 지운 줄이 되살아난다.
func TestDeclBlankNameRemovesRow(t *testing.T) {
	s, _ := withDecl(t)
	location(t, postForm(t, s, "/decl/save", url.Values{
		"org": {"acme"}, "scope": {"web-gw"},
		"node.name.0": {""}, "node.ips.0": {"10.0.0.1"}, // 지운다
		"asset.node.0": {"web-gw"}, "asset.runtime.0": {"openssl"}, "asset.component.0": {"libssl"},
	}))
	d, err := decl.Load(s.decl)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Nodes) != 0 {
		t.Fatalf("지운 줄이 남았다: %+v", d.Nodes)
	}
	if len(d.Edges) != 0 {
		t.Errorf("보내지 않은 엣지가 남았다: %+v", d.Edges)
	}
}
