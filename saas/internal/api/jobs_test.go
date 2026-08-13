package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pqcota/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/saas/internal/api"
	"github.com/sntsoftgit/pqcaton/saas/internal/jobs"
)

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
	h := newHarness(t, api.Config{})
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

// CP-HTTP-9 — 작업이 없으면 **곧바로** 204다.
//
// 러너는 상주하지 않고 자기 스케줄에 깨어나 물어본다(§3) — 붙들고 기다려 봐야 그
// 프로세스는 곧 끝난다. 빈 몸통을 200으로 주면 "작업 없음"과 "작업이 왔는데 못 읽었다"가
// 러너 쪽에서 같은 모양이 된다.
func TestNoJobAnswersImmediately(t *testing.T) {
	h := newHarness(t, api.Config{})

	start := time.Now()
	w := h.get("runner_id=r1", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("작업이 없는데 %d를 답했다: %s", w.Code, w.Body.String())
	}
	if w.Body.Len() != 0 {
		t.Fatalf("204인데 몸통이 있다: %s", w.Body.String())
	}
	if waited := time.Since(start); waited > 200*time.Millisecond {
		t.Fatalf("기다렸다: %v — 스케줄로 도는 러너를 붙들 이유가 없다", waited)
	}
}

// CP-HTTP-10 — **조직은 질의 문자열에서 오지 않는다.**
//
// 다른 조직의 토큰으로 `?org=acme`를 붙여도 acme의 작업은 나가지 않는다. 핸들러가
// 조직을 읽을 자리를 하나도 두지 않는 것이 이 성질을 지킨다(§6.4).
func TestJobsOrgComesFromTokenNotQuery(t *testing.T) {
	h := newHarness(t, api.Config{})
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
	h := newHarness(t, api.Config{})
	h.put(t, "acme", "j1", jobs.Observe)

	if w := h.get("", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("러너를 밝히지 않았는데 나갔다: %d %s", w.Code, w.Body.String())
	}
	j, _ := h.jobs.Get("acme", "j1")
	if j.State != jobs.Pending {
		t.Fatalf("작업이 점유됐다: %+v", j)
	}
}
