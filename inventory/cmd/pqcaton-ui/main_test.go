package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func post(t *testing.T, s *server, target string, form url.Values) *httptest.ResponseRecorder {
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
	q := location(t, post(t, s, "/save", url.Values{
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
	q := location(t, post(t, s, "/finalize", url.Values{
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

	q := location(t, post(t, s, "/finalize", url.Values{
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
	location(t, post(t, s, "/save", url.Values{
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
