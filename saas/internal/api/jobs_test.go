package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pqcota/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/saas/internal/api"
	"github.com/sntsoftgit/pqcaton/saas/internal/jobs"
)

// 롱폴 케이스는 진짜 시계로 돈다 — 고정 시계로는 기다림이 끝나지 않는다.
// 짧게 잡아 테스트가 대기로 시간을 쓰지 않게 한다.
func pollCfg() api.Config {
	return api.Config{JobWait: 200 * time.Millisecond, JobPoll: 5 * time.Millisecond}
}

func (h *harness) put(t *testing.T, o org.ID, id string, k jobs.Kind) {
	t.Helper()
	if err := h.jobs.Put(jobs.Job{
		ID: id, Org: o, Kind: k, State: jobs.Pending, Created: t0, Targets: []string{"web-01"},
	}); err != nil {
		t.Fatalf("작업 넣기: %v", err)
	}
}

// get — 작업을 받아가는 요청. token이 비면 acme의 토큰을 쓴다.
func (h *harness) get(query, token string) *httptest.ResponseRecorder {
	if token == "" {
		token = h.token
	}
	r := httptest.NewRequest(http.MethodGet, "/v1/runner/jobs?"+query, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.srv.ServeHTTP(w, r)
	return w
}

type gotJob struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Targets   []string  `json:"targets"`
	LeaseTill time.Time `json:"lease_till"`
	Attempts  int       `json:"attempts"`
}

// CP-HTTP-8 — 대기 중 작업이 있으면 곧장 나가고, **그 자리에서 점유가 된다.**
// 점유가 응답과 함께 남지 않으면 다음 러너가 같은 작업을 또 받아간다.
func TestJobIsLeasedToTheAskingRunner(t *testing.T) {
	h := newHarness(t, pollCfg())
	h.put(t, "acme", "j1", jobs.Observe)

	w := h.get("runner_id=r1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("상태 %d: %s", w.Code, w.Body.String())
	}
	var got gotJob
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("응답: %v", err)
	}
	if got.ID != "j1" || got.Kind != string(jobs.Observe) || got.Attempts != 1 {
		t.Fatalf("작업이 그대로 오지 않았다: %+v", got)
	}
	if len(got.Targets) != 1 || got.Targets[0] != "web-01" {
		t.Fatalf("대상이 오지 않았다 — 러너가 어디로 갈지 모른다: %+v", got)
	}
	if !got.LeaseTill.Equal(t0.Add(api.DefaultJobLease)) {
		t.Fatalf("만료가 오지 않았다 — 러너가 언제까지인지 모른다: %v", got.LeaseTill)
	}

	j, err := h.jobs.Get("acme", "j1")
	if err != nil || j.State != jobs.Leased || j.RunnerID != "r1" {
		t.Fatalf("점유가 남지 않았다: %+v %v", j, err)
	}
	// 그리고 다음 러너에게는 나가지 않는다.
	if w := h.get("runner_id=r2", ""); w.Code != http.StatusNoContent {
		t.Fatalf("점유된 작업이 또 나갔다: %d %s", w.Code, w.Body.String())
	}
}

// CP-HTTP-9 — **작업이 없으면 기다린다.** 기다리는 동안 들어온 작업이 그 요청으로 나간다.
//
// 곧장 「없다」고 답하면 러너가 짧은 간격으로 계속 물어야 하고, 그 간격이 곧 새 작업이
// 러너에 닿는 지연이 된다. 러너는 밖으로만 걸어서 밀어 줄 방법이 없다(§6.1).
func TestLongPollDeliversJobThatArrivesWhileWaiting(t *testing.T) {
	h := newHarness(t, pollCfg())

	go func() {
		time.Sleep(20 * time.Millisecond)
		// 넣기가 실패하면 아래에서 작업이 안 나온 것으로 잡힌다.
		_ = h.jobs.Put(jobs.Job{ID: "late", Org: "acme", Kind: jobs.Observe, State: jobs.Pending, Created: t0})
	}()

	start := time.Now()
	w := h.get("runner_id=r1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("기다리지 않고 답했다: %d %s", w.Code, w.Body.String())
	}
	var got gotJob
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.ID != "late" {
		t.Fatalf("나중에 들어온 작업이 나가지 않았다: %+v", got)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Fatal("작업이 들어오기도 전에 답이 나왔다 — 케이스가 기다림을 재고 있지 않다")
	}
}

// CP-HTTP-10 — **조직은 질의 문자열에서 오지 않는다.**
//
// 다른 조직의 토큰으로 `?org=acme`를 붙여도 acme의 작업은 나가지 않는다. 핸들러가
// 조직을 읽을 자리를 하나도 두지 않는 것이 이 성질을 지킨다(§6.4).
func TestJobsOrgComesFromTokenNotQuery(t *testing.T) {
	h := newHarness(t, pollCfg())
	h.put(t, "acme", "j1", jobs.Observe)

	if w := h.get("runner_id=r1&org=acme", h.beta); w.Code != http.StatusNoContent {
		t.Fatalf("다른 조직의 작업이 나갔다: %d %s", w.Code, w.Body.String())
	}
	j, _ := h.jobs.Get("acme", "j1")
	if j.State != jobs.Pending {
		t.Fatalf("남의 조직 요청이 작업을 건드렸다: %+v", j)
	}
}

// CP-HTTP-11 — 누가 가져가는지 밝히지 않으면 주지 않는다.
//
// 러너를 모르면 완료 보고를 점유와 맞춰 볼 수 없다 — 그 작업은 만료될 때까지 아무도
// 닫지 못한다.
func TestJobsRequireRunnerID(t *testing.T) {
	h := newHarness(t, pollCfg())
	h.put(t, "acme", "j1", jobs.Observe)

	if w := h.get("", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("러너를 밝히지 않았는데 나갔다: %d %s", w.Code, w.Body.String())
	}
	j, _ := h.jobs.Get("acme", "j1")
	if j.State != jobs.Pending {
		t.Fatalf("작업이 점유됐다: %+v", j)
	}
}

// CP-HTTP-12 — 기다림에는 끝이 있고, 끊긴 요청은 붙들지 않는다.
//
// 상한이 없으면 러너가 큰 값을 보내 연결을 오래 붙든다. 끊긴 요청을 붙들면 죽은 러너
// 수만큼 고루틴이 쌓인다.
func TestLongPollGivesUpAndReleasesDisconnected(t *testing.T) {
	h := newHarness(t, pollCfg())

	start := time.Now()
	w := h.get("runner_id=r1&wait=10s", "") // 상한을 넘겨 잡아도 상한까지만
	if w.Code != http.StatusNoContent {
		t.Fatalf("작업이 없는데 %d를 답했다: %s", w.Code, w.Body.String())
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Fatalf("상한을 넘겨 기다렸다: %v", waited)
	}

	// 러너가 끊으면 상한을 기다리지 않고 돌아온다.
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodGet, "/v1/runner/jobs?runner_id=r1", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+h.token)
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.srv.ServeHTTP(httptest.NewRecorder(), r)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("끊긴 요청을 놓지 않는다 — 죽은 러너 수만큼 고루틴이 쌓인다")
	}
}
