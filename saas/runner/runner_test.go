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

// count — 본문의 그 배열이 몇 개인가. 없으면 0이다.
func count(body map[string]any, key string) int {
	v, ok := body[key].([]any)
	if !ok {
		return 0
	}
	return len(v)
}

// enrollFileOn — 연결확인 파일 하나를 놓는다. 플레이북이 쓰는 자리다.
func enrollFileOn(t *testing.T, dir, name, body string) {
	t.Helper()
	d := filepath.Join(dir, "enroll")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// enrollments — 러너가 보낸 연결확인들.
func enrollments(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["enrollments"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		m, ok := e.(map[string]any)
		if !ok {
			t.Fatalf("연결확인이 객체가 아니다: %#v", e)
		}
		out = append(out, m)
	}
	return out
}

// plane — 컨트롤 플레인 흉내. 러너가 **무엇을 보냈나**를 그대로 붙든다.
type plane struct {
	gotAuth  string
	gotQuery string
	gotBody  map[string]any // 마지막 결과 업로드
	gotEnr   map[string]any // 마지막 연결확인 업로드
	fail     bool           // 결과 업로드를 실패시킨다
	failEnr  bool           // 연결확인 업로드를 실패시킨다
}

func (p *plane) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/runner/results", func(w http.ResponseWriter, r *http.Request) {
		p.gotAuth, p.gotQuery = r.Header.Get("Authorization"), r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&p.gotBody)
		if p.fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": count(p.gotBody, "results")})
	})
	mux.HandleFunc("POST /v1/runner/enrollments", func(w http.ResponseWriter, r *http.Request) {
		p.gotAuth, p.gotQuery = r.Header.Get("Authorization"), r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&p.gotEnr)
		if p.failEnr {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"enrolled": count(p.gotEnr, "enrollments")})
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

// RUN-2 — **컨트롤 플레인에 할 일을 묻지 않는다.**
//
// 스케줄이 곧 관측 주기다. 러너가 매번 "할 일이 있나"를 물어야 돈다면, 그 자리가 비어 있는
// 동안 **관측이 통째로 멈춘다** — 스케줄을 둔 이유가 사라진다.
func TestNothingIsAskedOfTheControlPlane(t *testing.T) {
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json", "db-01-openssl.json")

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("실행: %v", err)
	}
	if rep.Files != 2 {
		t.Fatalf("스케줄만으로 올리는 길이 막혔다: %+v", rep)
	}
	if _, has := p.gotBody["job_id"]; has {
		t.Fatalf("작업 개념이 남아 있다: %v", p.gotBody["job_id"])
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

// RUN-12 — **플레이북이 실패해도 생긴 결과는 올린다.** 다만 실패를 숨기지 않는다.
//
// 반쯤 나온 것을 버리면 그 관측은 사라진다. 그렇다고 조용히 0으로 끝내면 스케줄러가 잘 돈
// 것으로 읽는다 — 무엇이 왜 안 됐는지는 완전성 맵에서 봐야 한다.
func TestFailedPlaybookStillUploadsWhatExists(t *testing.T) {
	p := &plane{}
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
	if p.gotBody == nil || rep.Files != 1 {
		t.Fatalf("생긴 결과까지 버렸다: %+v", rep)
	}
}

// RUN-13 — 플레이북이 설정되지 않았으면 **돌리지 않고 올리기만 한다.**
//
// 관측을 다른 방식으로 돌리는 고객이 있을 수 있다. 러너의 값은 「올리는 입」이지
// 「돌리는 손」이 아니다.
func TestNoPlaybookStillUploads(t *testing.T) {
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json")

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("실행: %v", err)
	}
	if rep.Played || rep.Files != 1 {
		t.Fatalf("올리기만 하는 길이 막혔다: %+v", rep)
	}
}

// RUN-18 — 연결확인은 **관측과 다른 자리로 간다.** 하나가 실패해도 다른 하나는 올라간다.
//
// 둘은 같은 때에 올라오지 않는다 — 연결확인은 대상 목록이 바뀔 때, 관측은 매 스케줄이다.
// 한 본문에 묶으면 **두 종류의 실패가 한 응답에 섞이고**, 하나가 막히면 나머지도 묵힌다.
func TestEnrollmentsGoToTheirOwnEndpoint(t *testing.T) {
	p := &plane{failEnr: true}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "obs.json")
	enrollFileOn(t, cfg.ResultsDir, "web-01.json",
		`{"node_id":"web-01","fingerprint":"SHA256:abc","display_name":"웹 1호"}`)

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("연결확인이 막혔다고 관측까지 멈췄다: %v", err)
	}
	if count(p.gotBody, "results") != 1 || rep.Files != 1 {
		t.Fatalf("관측이 안 올라갔다: %+v", rep)
	}
	if count(p.gotBody, "enrollments") != 0 {
		t.Fatalf("연결확인이 결과에 섞여 갔다: %#v", p.gotBody)
	}
	// 못 올린 연결확인은 그대로 남는다 — 다음 실행이 다시 올린다.
	if _, err := os.Stat(filepath.Join(cfg.ResultsDir, "enroll", "web-01.json")); err != nil {
		t.Fatalf("못 올렸는데 치웠다: %v", err)
	}

	// 막지 않으면 제 자리로 올라가고 파일이 옮겨진다.
	p.failEnr = false
	rep, err = runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("돌지 못했다: %v", err)
	}
	if rep.Enrollments != 1 || rep.Enrolled != 1 || count(p.gotEnr, "enrollments") != 1 {
		t.Fatalf("연결확인이 안 올라갔다: %+v %#v", rep, p.gotEnr)
	}
	if _, err := os.Stat(filepath.Join(cfg.ResultsDir, "enroll", "sent", "web-01.json")); err != nil {
		t.Fatalf("올린 연결확인이 안 옮겨졌다: %v", err)
	}
}

// RUN-19 — **주소는 토큰이 되어 나가고, 원본은 어디에도 없다.**
//
// 이 제품이 파는 성질입니다 — 우리 DB가 털려도 고객 내부 주소 지도가 나오지 않습니다.
// 같은 주소가 늘 같은 토큰이 되어야 영역 간에 같은 상대를 이어 붙일 수 있습니다(§6.3.1).
func TestAddrBecomesATokenAndNeverLeaves(t *testing.T) {
	const addr = "10.20.3.14:22"
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv)
	cfg.AddrKey = "조직-키"
	enrollFileOn(t, cfg.ResultsDir, "web-01.json",
		`{"node_id":"web-01","fingerprint":"SHA256:abc","addr":"`+addr+`"}`)

	if _, err := runner.RunOnce(cfg, cl, quiet()); err != nil {
		t.Fatalf("돌지 못했다: %v", err)
	}
	body, _ := json.Marshal(p.gotEnr)
	if strings.Contains(string(body), addr) {
		t.Fatalf("주소가 그대로 나갔다: %s", body)
	}
	got := enrollments(t, p.gotEnr)
	tok, _ := got[0]["addr_token"].(string)
	if tok == "" {
		t.Fatal("주소 토큰이 안 붙었다 — 영역 간 엣지를 이어 붙일 표가 안 생긴다")
	}

	// 같은 키·같은 주소면 같은 토큰이어야 한다. 매번 다르면 이어 붙일 수 없다.
	p2 := &plane{}
	srv2 := p2.start(t)
	cfg2, cl2 := setup(t, srv2)
	cfg2.AddrKey = cfg.AddrKey
	enrollFileOn(t, cfg2.ResultsDir, "other.json",
		`{"node_id":"other","fingerprint":"SHA256:zzz","addr":"`+addr+`"}`)
	if _, err := runner.RunOnce(cfg2, cl2, quiet()); err != nil {
		t.Fatalf("돌지 못했다: %v", err)
	}
	if again, _ := enrollments(t, p2.gotEnr)[0]["addr_token"].(string); again != tok {
		t.Fatalf("같은 주소가 다른 토큰이 됐다: %q ≠ %q", again, tok)
	}
}

// RUN-20 — **붙었다는데 지문이 없으면 사유를 붙여 올린다.**
//
// 그대로 올리면 지문 없는 노드가 등재되어 클론 검출을 통째로 빠져나갑니다. 그렇다고 조용히
// 버리면 운영자는 그 대상이 등재된 줄 압니다.
func TestConnectedWithoutFingerprintIsReportedAsFailure(t *testing.T) {
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv)
	enrollFileOn(t, cfg.ResultsDir, "web-01.json", `{"node_id":"web-01"}`)

	if _, err := runner.RunOnce(cfg, cl, quiet()); err != nil {
		t.Fatalf("돌지 못했다: %v", err)
	}
	got := enrollments(t, p.gotEnr)
	if len(got) != 1 {
		t.Fatalf("조용히 버렸다: %#v", p.gotEnr)
	}
	if e, _ := got[0]["error"].(string); e == "" {
		t.Fatalf("지문 없이 그대로 올라갔다: %#v", got[0])
	}
}

// RUN-21 — 관측 결과가 없어도 **연결확인만으로 올린다.**
//
// 등재가 관측의 게이트라, 첫 연결확인은 결과 파일이 하나도 없는 상태에서 일어납니다.
// 결과가 있어야만 올린다면 **아무도 등재되지 못합니다.**
func TestEnrollmentsGoUpWithoutAnyResults(t *testing.T) {
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv)
	enrollFileOn(t, cfg.ResultsDir, "web-01.json",
		`{"node_id":"web-01","fingerprint":"SHA256:abc"}`)

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("돌지 못했다: %v", err)
	}
	if rep.Enrollments != 1 || count(p.gotEnr, "enrollments") != 1 {
		t.Fatalf("연결확인만으로는 안 올렸다: %+v %#v", rep, p.gotEnr)
	}
}

// RUN-22 — `node_id`가 없는 연결확인은 **보내지 않고 치운다.**
//
// 컨트롤 플레인이 쓸 수 없는 파일입니다. 그대로 두면 다음 실행마다 걸려 그 디렉터리가 영영
// 안 올라갑니다 — 지우지는 않습니다. 왜 그렇게 나왔는지의 유일한 증거입니다.
func TestEnrollmentWithoutNodeIdIsSetAside(t *testing.T) {
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv)
	enrollFileOn(t, cfg.ResultsDir, "ok.json", `{"node_id":"web-01","fingerprint":"SHA256:abc"}`)
	enrollFileOn(t, cfg.ResultsDir, "broken.json", `{"fingerprint":"SHA256:zzz"}`)

	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil {
		t.Fatalf("돌지 못했다: %v", err)
	}
	if rep.Enrollments != 1 {
		t.Fatalf("나머지까지 버렸거나 깨진 것을 보냈다: %+v", rep)
	}
	if _, err := os.Stat(filepath.Join(cfg.ResultsDir, "enroll", "bad", "broken.json")); err != nil {
		t.Fatalf("증거가 안 남았다: %v", err)
	}
}
