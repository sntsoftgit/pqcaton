package runner_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/saas/runner"
)

const token = "pqcrt_a3f9k2mq_7x4bn8wr2ejd5vh6tzc9pkm3sq0yfla"

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// plane — 컨트롤 플레인 흉내. 러너가 **무엇을 보냈나**를 그대로 붙든다.
type plane struct {
	job      any // nil이면 204
	gotAuth  string
	gotQuery string
	gotBody  map[string]any
	fail     bool // 결과 업로드를 실패시킨다
}

func (p *plane) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/runner/jobs", func(w http.ResponseWriter, r *http.Request) {
		p.gotAuth, p.gotQuery = r.Header.Get("Authorization"), r.URL.RawQuery
		if p.job == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(p.job)
	})
	mux.HandleFunc("POST /v1/runner/results", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&p.gotBody)
		if p.fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		n := len(p.gotBody["results"].([]any))
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": n, "job": "closed"})
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// setup — 결과 파일을 놓고 러너 설정을 만든다.
func setup(t *testing.T, srv *httptest.Server, names ...string) (runner.Config, *runner.Client) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(`{"envelope":{"targetNodeId":"web-01"}}`), 0o600); err != nil {
			t.Fatalf("결과 파일: %v", err)
		}
	}
	cfg := runner.Config{API: srv.URL, Token: token, RunnerID: "r1", ResultsDir: dir}
	return cfg, runner.NewClient(cfg)
}

// RUN-1 — 작업을 받으면 **그 작업 id를 결과와 함께** 올린다.
//
// 올리기와 닫기가 한 왕복이어야 한다 — 나누면 "결과는 올렸는데 닫지 못한" 구간이 생기고,
// 그 작업은 만료돼 한 번 더 배포된다.
func TestJobIdRidesWithTheResults(t *testing.T) {
	p := &plane{job: map[string]any{"id": "j1", "kind": "observe"}}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json")

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("실행: %v", err)
	}
	if rep.JobID != "j1" || rep.Files != 1 || rep.Accepted != 1 {
		t.Fatalf("왕복이 닫히지 않았다: %+v", rep)
	}
	if p.gotBody["job_id"] != "j1" {
		t.Fatalf("작업 id가 결과와 함께 가지 않았다: %v", p.gotBody["job_id"])
	}
	if p.gotBody["runner_id"] != "r1" || p.gotBody["runner_version"] != runner.Version {
		t.Fatalf("누가 무엇으로 올렸는지가 빠졌다: %v", p.gotBody)
	}
	if rep.Job != "closed" {
		t.Fatalf("작업이 닫히지 않았다: %q", rep.Job)
	}
}

// RUN-2 — **작업이 없어도 결과는 올린다.**
//
// 스케줄이 돌린 관측이 남아 있을 수 있고, 다음 작업이 올 때까지 묵혀 둘 이유가 없다.
// 그때는 `job_id`를 붙이지 않는다 — 없는 작업을 닫으려 들면 안 된다.
func TestResultsGoUpWithoutAJob(t *testing.T) {
	p := &plane{job: nil} // 204
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json", "db-01-openssl.json")

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("실행: %v", err)
	}
	if rep.JobID != "" || rep.Files != 2 {
		t.Fatalf("작업 없이 올리는 길이 막혔다: %+v", rep)
	}
	if _, has := p.gotBody["job_id"]; has {
		t.Fatalf("없는 작업을 닫으려 했다: %v", p.gotBody["job_id"])
	}
}

// RUN-3 — 올린 결과는 **다시 올라가지 않는다.**
//
// 안 옮기면 매번 다시 올라간다. 멱등이 접어 주더라도 러너와 경계가 그만큼 헛일을 한다.
func TestSentResultsAreNotResent(t *testing.T) {
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json")

	if _, err := runner.RunOnce(cfg, cl, quiet()); err != nil {
		t.Fatalf("첫 실행: %v", err)
	}
	p.gotBody = nil
	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("두 번째 실행: %v", err)
	}
	if rep.Files != 0 || p.gotBody != nil {
		t.Fatalf("같은 결과가 또 올라갔다: %+v", rep)
	}
	// 지우지는 않는다 — 올린 것이 정말 들어갔는지 확인할 근거가 남아야 한다.
	if _, err := os.Stat(filepath.Join(cfg.ResultsDir, "sent", "web-01-openssl.json")); err != nil {
		t.Fatalf("올린 결과가 사라졌다: %v", err)
	}
}

// RUN-4 — 업로드가 실패하면 **파일을 그대로 둔다.**
//
// 옮겨 버리면 그 관측은 영영 못 올라간다. 다음 실행이 다시 올리고, 같은 결과는 멱등이 접는다.
func TestFailedUploadKeepsTheFiles(t *testing.T) {
	p := &plane{fail: true}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json")

	if _, err := runner.RunOnce(cfg, cl, quiet()); err == nil {
		t.Fatal("실패했는데 오류가 안 났다")
	}
	if _, err := os.Stat(filepath.Join(cfg.ResultsDir, "web-01-openssl.json")); err != nil {
		t.Fatalf("실패했는데 결과를 치웠다: %v", err)
	}
}

// RUN-5 — **토큰은 헤더로만 나가고, 오류에 담기지 않는다.**
//
// 질의 문자열에 실으면 프록시 로그와 브라우저 히스토리에 남는다. 오류 메시지는 로그로 가고
// 로그도 남는다.
func TestTokenNeverLeaksIntoUrlOrError(t *testing.T) {
	p := &plane{fail: true}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json")

	_, err := runner.RunOnce(cfg, cl, quiet())
	if err == nil {
		t.Fatal("실패했는데 오류가 안 났다")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("오류에 토큰이 담겼다: %v", err)
	}
	if strings.Contains(p.gotQuery, token) {
		t.Fatalf("질의 문자열에 토큰이 실렸다: %s", p.gotQuery)
	}
	if p.gotAuth != "Bearer "+token {
		t.Fatalf("토큰이 헤더로 가지 않았다: %q", p.gotAuth)
	}
}

// RUN-6 — 올릴 것도 받은 작업도 없으면 **아무것도 보내지 않는다.**
// 빈 요청은 경계에 비용만 준다.
func TestNothingToDoSendsNothing(t *testing.T) {
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv)

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("실행: %v", err)
	}
	if rep.Files != 0 || p.gotBody != nil {
		t.Fatalf("보낼 것이 없는데 보냈다: %+v", rep)
	}
}

// RUN-7 — 설정은 파일에서 읽는다. **토큰이 없으면 열리지 않는다.**
//
// 토큰에서 조직과 영역이 유도되므로, 없으면 러너가 할 수 있는 일이 없다.
func TestConfigNeedsTokenAndApi(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "conf")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	if _, err := runner.LoadConfig(write("# 주석\nPQCATON_API=https://cp\n")); err != runner.ErrNoToken {
		t.Fatalf("토큰 없이 열렸다: %v", err)
	}
	if _, err := runner.LoadConfig(write("PQCATON_TOKEN=" + token + "\n")); err != runner.ErrNoAPI {
		t.Fatalf("주소 없이 열렸다: %v", err)
	}

	c, err := runner.LoadConfig(write("PQCATON_API = https://cp \nPQCATON_TOKEN=\"" + token + "\"\nPQCATON_RESULTS_DIR=/work/results\n"))
	if err != nil {
		t.Fatalf("읽기: %v", err)
	}
	if c.API != "https://cp" || c.Token != token || c.ResultsDir != "/work/results" {
		t.Fatalf("설정이 그대로 오지 않았다: %+v", c)
	}
	if c.RunnerID == "" {
		t.Fatal("러너 id가 비었다 — 호스트이름으로라도 채워야 완료 보고를 맞춰 볼 수 있다")
	}
}
