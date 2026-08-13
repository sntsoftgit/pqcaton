package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client — 컨트롤 플레인에 거는 쪽.
//
// **아웃바운드만이다.** 러너가 묻고 컨트롤 플레인이 답한다 — 반대 방향의 연결은 없고,
// 그래서 고객 방화벽에 인바운드 예외가 필요 없다.
type Client struct {
	api   string
	token string
	http  *http.Client
}

// DefaultTimeout — 한 번의 왕복 상한.
//
// 결과 업로드는 본문이 클 수 있어 넉넉히 잡는다. 러너는 스케줄에 깨어나므로 오래 붙들어도
// 남는 프로세스가 없다.
const DefaultTimeout = 2 * time.Minute

// NewClient — 클라이언트를 만든다.
func NewClient(c Config) *Client {
	return &Client{
		api:   strings.TrimRight(c.API, "/"),
		token: c.Token,
		http:  &http.Client{Timeout: DefaultTimeout},
	}
}

// Job — 받아 온 작업.
type Job struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Targets   []string  `json:"targets"`
	Payload   []byte    `json:"payload"`
	LeaseTill time.Time `json:"lease_till"`
	Attempts  int       `json:"attempts"`
}

// NextJob — 할 일이 있나. 없으면 (Job{}, false, nil).
//
// **기다리지 않는다.** 러너는 상주하지 않으므로 붙들고 있어 봐야 그 프로세스는 곧 끝난다 —
// 새 작업이 러너에 닿는 지연은 스케줄 간격이고, 그것이 이 배포 형태의 대가다.
func (c *Client) NextJob(runnerID string) (Job, bool, error) {
	q := url.Values{"runner_id": {runnerID}}
	req, err := c.request(http.MethodGet, "/v1/runner/jobs?"+q.Encode(), nil)
	if err != nil {
		return Job{}, false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Job{}, false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return Job{}, false, nil
	case http.StatusOK:
		var j Job
		if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
			return Job{}, false, fmt.Errorf("작업을 읽을 수 없다: %w", err)
		}
		return j, true, nil
	default:
		return Job{}, false, statusError(resp)
	}
}

type resultsRequest struct {
	RunnerID      string            `json:"runner_id"`
	JobID         string            `json:"job_id,omitempty"`
	RunnerVersion string            `json:"runner_version"`
	Results       []json.RawMessage `json:"results"`
}

// Results — 컨트롤 플레인이 무엇을 했는지.
type Results struct {
	Accepted   int      `json:"accepted"`
	Duplicate  int      `json:"duplicate"`
	Rejected   int      `json:"rejected"`
	Unverified int      `json:"unverified"`
	OffScope   int      `json:"off_scope"`
	Nodes      []string `json:"nodes"`
	// Job — 작업 처리 결과. closed · not-found · not-leased · no-runner
	Job string `json:"job"`
}

// SendResults — 결과를 올리고, jobID가 있으면 **그 작업까지 닫는다.**
//
// 결과 본문은 열어 보지 않고 그대로 넘긴다. 계약을 아는 쪽은 collector와 수신 API이고,
// 러너가 중간에서 해석하면 **버전이 어긋날 때 러너가 먼저 깨진다.**
func (c *Client) SendResults(runnerID, jobID string, files []string) (Results, error) {
	body := resultsRequest{RunnerID: runnerID, JobID: jobID, RunnerVersion: Version}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return Results{}, fmt.Errorf("%s: %w", f, err)
		}
		if !json.Valid(raw) {
			// 깨진 파일 하나로 나머지를 버리지 않는다. 다만 **조용히 넘기지도 않는다** —
			// 수신 API가 계약 위반을 세는 것과 같은 이유다.
			return Results{}, fmt.Errorf("%s: JSON이 아니다", f)
		}
		body.Results = append(body.Results, json.RawMessage(raw))
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return Results{}, err
	}
	req, err := c.request(http.MethodPost, "/v1/runner/results", bytes.NewReader(buf))
	if err != nil {
		return Results{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Results{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Results{}, statusError(resp)
	}
	var out Results
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Results{}, fmt.Errorf("응답을 읽을 수 없다: %w", err)
	}
	return out, nil
}

func (c *Client) request(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.api+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return req, nil
}

// statusError — 오류를 만들되 **토큰을 담지 않는다.**
//
// 오류는 로그로 가고 로그는 남는다. 요청 URL에도 토큰이 없다(헤더로만 보낸다) —
// 그 성질이 유지되는지는 케이스가 지킨다.
func statusError(resp *http.Response) error {
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("컨트롤 플레인이 %d로 답했다: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
}
