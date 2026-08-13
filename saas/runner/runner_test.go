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
	"time"

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

// RUN-8 — **깨진 파일 하나가 나머지를 막지 않는다.** 다만 조용히 넘기지도 않는다.
//
// 그대로 두면 다음 실행마다 같은 파일에 걸려 그 디렉터리가 영영 안 올라간다. 치우되
// 지우지는 않는다 — **올라간 적이 없어 그 사본이 왜 깨졌는지의 유일한 증거다.**
func TestBrokenFileIsSetAsideNotDropped(t *testing.T) {
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json")
	if err := os.WriteFile(filepath.Join(cfg.ResultsDir, "torn.json"), []byte("{반쯤 쓰다 만"), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("실행: %v", err)
	}
	if rep.Files != 1 || rep.Bad != 1 {
		t.Fatalf("깨진 파일이 나머지를 막았다: %+v", rep)
	}
	if n := len(p.gotBody["results"].([]any)); n != 1 {
		t.Fatalf("깨진 것까지 올렸다: %d개", n)
	}
	if _, err := os.Stat(filepath.Join(cfg.ResultsDir, "bad", "torn.json")); err != nil {
		t.Fatalf("증거가 사라졌다: %v", err)
	}
}

// RUN-9 — 보존 기간이 지난 것만 지운다. `bad`가 `sent`보다 오래 남는다.
//
// 디스크가 차면 플레이북이 결과를 못 써 관측이 멈춘다. 그렇다고 증거까지 같은 기간에
// 지우면, 왜 깨졌는지 볼 것이 없어진다.
func TestOldResultsAreSweptByAge(t *testing.T) {
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv)
	cfg.SentKeepDays, cfg.BadKeepDays = 7, 30

	old := time.Now().AddDate(0, 0, -10) // sent는 지나고 bad는 안 지난 나이
	for _, sub := range []string{"sent", "bad"} {
		dir := filepath.Join(cfg.ResultsDir, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"old.json", "new.json"} {
			f := filepath.Join(dir, n)
			if err := os.WriteFile(f, []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
			if n == "old.json" {
				if err := os.Chtimes(f, old, old); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	if _, err := runner.RunOnce(cfg, cl, quiet()); err != nil {
		t.Fatalf("실행: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.ResultsDir, "sent", "old.json")); err == nil {
		t.Fatal("보존 기간이 지난 것이 남았다")
	}
	for _, keep := range []string{"sent/new.json", "bad/old.json", "bad/new.json"} {
		if _, err := os.Stat(filepath.Join(cfg.ResultsDir, filepath.FromSlash(keep))); err != nil {
			t.Fatalf("아직 둬야 할 것을 지웠다: %s", keep)
		}
	}
}

// RUN-10 — 보존 기간 설정. **0이면 지우지 않고, 음수는 거절한다.**
//
// 오타를 "지우지 않음"으로 삼키면 디스크가 조용히 찬다.
func TestKeepDaysConfig(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "conf")
		if err := os.WriteFile(p, []byte("PQCATON_API=https://cp\nPQCATON_TOKEN="+token+"\n"+body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	c, err := runner.LoadConfig(write(""))
	if err != nil {
		t.Fatalf("읽기: %v", err)
	}
	if c.SentKeepDays != runner.DefaultSentKeepDays || c.BadKeepDays != runner.DefaultBadKeepDays {
		t.Fatalf("기본값이 아니다: %+v", c)
	}
	if c.BadKeepDays <= c.SentKeepDays {
		t.Fatal("bad가 sent보다 오래 남지 않는다 — 증거가 먼저 사라진다")
	}

	c, err = runner.LoadConfig(write("PQCATON_SENT_KEEP_DAYS=0\nPQCATON_BAD_KEEP_DAYS=90\n"))
	if err != nil || c.SentKeepDays != 0 || c.BadKeepDays != 90 {
		t.Fatalf("설정이 안 먹었다: %+v %v", c, err)
	}
	if _, err := runner.LoadConfig(write("PQCATON_SENT_KEEP_DAYS=-1\n")); err == nil {
		t.Fatal("음수가 통과했다")
	}
	if _, err := runner.LoadConfig(write("PQCATON_BAD_KEEP_DAYS=이레\n")); err == nil {
		t.Fatal("일수가 아닌 값이 통과했다")
	}
}

// fakeAnsible — 부를 명령을 가짜로 바꾼다. 실제 exec 경로를 그대로 태워야 인자 전달까지 잰다.
func fakeAnsible(t *testing.T, dir, script string) string {
	t.Helper()
	p := filepath.Join(dir, "fake-ansible")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// RUN-11 — 작업을 받으면 **플레이북을 돌리고, 대상을 `--limit`으로 좁힌다.**
//
// 지목된 노드가 있는데 인벤토리 전체를 돌리면 「이 노드만」이라는 지시가 뜻을 잃고,
// 과금 대상도 늘어난다.
func TestPlaybookRunsLimitedToTargets(t *testing.T) {
	p := &plane{job: map[string]any{"id": "j1", "kind": "observe", "targets": []string{"web-01", "db-01"}}}
	srv := p.start(t)
	cfg, cl := setup(t, srv)

	args := filepath.Join(cfg.ResultsDir, "args")
	cfg.Ansible = fakeAnsible(t, t.TempDir(), `echo "$@" > `+args+`
echo '{"envelope":{"targetNodeId":"web-01"}}' > `+filepath.Join(cfg.ResultsDir, "web-01.json"))
	cfg.Playbook, cfg.Inventory = "discover.yml", "targets.ini"

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("실행: %v", err)
	}
	if !rep.Played || rep.Files != 1 {
		t.Fatalf("돌리고 올리는 데까지 안 갔다: %+v", rep)
	}
	got, _ := os.ReadFile(args)
	for _, want := range []string{"-i targets.ini", "--limit web-01,db-01", "discover.yml"} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("인자가 빠졌다 (%q): %s", want, got)
		}
	}
	if p.gotBody["job_id"] != "j1" {
		t.Fatalf("돌린 작업이 닫히지 않았다: %v", p.gotBody["job_id"])
	}
}

// RUN-12 — **플레이북이 실패하면 그 작업을 닫지 않는다.** 생긴 결과는 그래도 올린다.
//
// 반쯤 된 것을 「끝났다」로 두면 그 노드는 다음 관측까지 빈 채로 남는다. 만료가 회수해
// 다시 배포한다 — 읽기라 안전하다. 그렇다고 이미 생긴 결과를 버릴 이유는 없다.
func TestFailedPlaybookDoesNotCloseTheJob(t *testing.T) {
	p := &plane{job: map[string]any{"id": "j1", "kind": "observe"}}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json")
	cfg.Ansible = fakeAnsible(t, t.TempDir(), "exit 4")
	cfg.Playbook = "discover.yml"

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err == nil {
		t.Fatal("플레이북이 실패했는데 오류가 안 났다 — 조용히 0으로 끝나면 스케줄러가 잘 돈 것으로 읽는다")
	}
	if rep.Played {
		t.Fatalf("실패했는데 돌린 것으로 셌다: %+v", rep)
	}
	if p.gotBody == nil {
		t.Fatal("생긴 결과까지 버렸다")
	}
	if _, has := p.gotBody["job_id"]; has {
		t.Fatalf("반쯤 된 작업을 닫았다: %v", p.gotBody["job_id"])
	}
}

// RUN-13 — 플레이북이 설정되지 않았으면 **돌리지 않고 올리기만 한다.**
//
// 관측을 다른 방식으로 돌리는 고객이 있을 수 있다. 러너의 값은 「올리는 입」이지
// 「돌리는 손」이 아니다.
func TestNoPlaybookStillUploads(t *testing.T) {
	p := &plane{job: map[string]any{"id": "j1", "kind": "observe"}}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json")

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("실행: %v", err)
	}
	if rep.Played || rep.Files != 1 || p.gotBody["job_id"] != "j1" {
		t.Fatalf("올리기만 하는 길이 막혔다: %+v", rep)
	}
}

// RUN-14 — **점유가 곧 만료되면 시작하지 않는다.**
//
// 만료된 뒤에 끝나 봐야 그 작업은 이미 회수됐다 — 도는 동안 대상 노드에 부담만 준다.
func TestExpiringLeaseIsNotStarted(t *testing.T) {
	p := &plane{job: map[string]any{
		"id": "j1", "kind": "observe",
		"lease_till": time.Now().Add(2 * time.Second).UTC().Format(time.RFC3339),
	}}
	srv := p.start(t)
	cfg, cl := setup(t, srv)
	ran := filepath.Join(cfg.ResultsDir, "ran")
	cfg.Ansible = fakeAnsible(t, t.TempDir(), "touch "+ran)
	cfg.Playbook = "discover.yml"

	if _, err := runner.RunOnce(cfg, cl, quiet()); err == nil {
		t.Fatal("만료 직전인데 오류가 안 났다")
	}
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("만료 직전인데 대상 노드에 붙었다")
	}
}

// RUN-16 — **모르는 작업 종류는 아무것도 하지 않는다.**
//
// 모르는 것을 관측 플레이북으로 돌리면, 그것이 쓰기 작업일 때 대상 노드에 무슨 일이 날지
// 우리가 모른다. 새 종류가 생길 때 옛 러너가 조용히 엉뚱한 일을 하는 것을 막는다.
//
// `provision`도 지금은 여기 걸린다 — 적용 경로를 만들지 않았고, 반쯤 만든 것으로 고객
// 서버에 쓰지 않는다.
func TestUnknownKindDoesNothing(t *testing.T) {
	for _, kind := range []string{"provision", "아직-없는-종류"} {
		p := &plane{job: map[string]any{"id": "j1", "kind": kind}}
		srv := p.start(t)
		cfg, cl := setup(t, srv)
		ran := filepath.Join(cfg.ResultsDir, "ran")
		cfg.Ansible = fakeAnsible(t, t.TempDir(), "touch "+ran)
		cfg.Playbook = "discover.yml"

		if _, err := runner.RunOnce(cfg, cl, quiet()); err == nil {
			t.Fatalf("%s: 모르는 종류인데 오류가 안 났다", kind)
		}
		if _, err := os.Stat(ran); err == nil {
			t.Fatalf("%s: 모르는 종류인데 대상 노드에 붙었다", kind)
		}
	}
}

// RUN-17 — `enroll`과 `observe`는 같은 참조 플레이북으로 돈다.
//
// 둘 다 읽기이고, 등재는 그중 지문 확인만 쓴다 — 대상을 좁히는 것 말고 러너가 달리 할
// 일이 없다.
func TestEnrollRunsTheSamePlaybook(t *testing.T) {
	p := &plane{job: map[string]any{"id": "j1", "kind": "enroll", "targets": []string{"web-01"}}}
	srv := p.start(t)
	cfg, cl := setup(t, srv)
	args := filepath.Join(cfg.ResultsDir, "args")
	cfg.Ansible = fakeAnsible(t, t.TempDir(), `echo "$@" > `+args)
	cfg.Playbook = "discover.yml"

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("실행: %v", err)
	}
	if !rep.Played {
		t.Fatalf("등재 작업이 안 돌았다: %+v", rep)
	}
	got, _ := os.ReadFile(args)
	if !strings.Contains(string(got), "--limit web-01") {
		t.Fatalf("대상이 안 좁혀졌다: %s", got)
	}
}
