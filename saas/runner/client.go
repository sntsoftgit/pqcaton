package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// Enrollment — 연결확인 하나의 결과. 대상 하나에 하나다.
//
// **접속에 쓸 수 있는 것이 하나도 없다.** 주소는 러너가 토큰으로 바꾸고 버린다(§6.3.1) —
// 이 구조체에 주소 필드가 아예 없는 것이 그 보장이다.
type Enrollment struct {
	NodeID      string `json:"node_id"`
	Fingerprint string `json:"fingerprint"`
	DisplayName string `json:"display_name"`
	AddrToken   string `json:"addr_token,omitempty"`
	Err         string `json:"error,omitempty"`
}

type resultsRequest struct {
	RunnerID      string            `json:"runner_id"`
	RunnerVersion string            `json:"runner_version"`
	Results       []json.RawMessage `json:"results"`
}

// enrollRequest — 연결확인. **결과와 다른 자리로 간다** — 둘은 같은 때에 올라오지
// 않는다(연결확인은 대상 목록이 바뀔 때, 관측은 매 스케줄).
type enrollRequest struct {
	RunnerID      string       `json:"runner_id"`
	RunnerVersion string       `json:"runner_version"`
	Enrollments   []Enrollment `json:"enrollments"`
}

// Results — 컨트롤 플레인이 무엇을 했는지.
type Results struct {
	Accepted   int      `json:"accepted"`
	Duplicate  int      `json:"duplicate"`
	Rejected   int      `json:"rejected"`
	Unverified int      `json:"unverified"`
	OffScope   int      `json:"off_scope"`
	Nodes      []string `json:"nodes"`
}

// Enrolled — 연결확인의 판정. 셋으로 갈린다(§6.3).
type Enrolled struct {
	Enrolled    int `json:"enrolled"`
	Held        int `json:"held"`
	FailedNodes int `json:"failed_nodes"`
	// Refused — 한도에 걸려 받지 않은 수. 사유가 함께 온다.
	Refused       int    `json:"refused"`
	RefusedReason string `json:"refused_reason"`
}

// SendResults — 관측 결과를 올린다.
//
// 본문은 이미 읽힌 것을 받는다 — 무엇을 보내고 무엇을 치울지는 [RunOnce]가 정한다.
func (c *Client) SendResults(runnerID string, payloads []json.RawMessage) (Results, error) {
	body := resultsRequest{
		RunnerID: runnerID, RunnerVersion: Version, Results: payloads,
	}
	var out Results
	err := c.post("/v1/runner/results", body, &out)
	return out, err
}

// SendEnrollments — 연결확인을 올린다.
func (c *Client) SendEnrollments(runnerID string, items []Enrollment) (Enrolled, error) {
	body := enrollRequest{
		RunnerID: runnerID, RunnerVersion: Version, Enrollments: items,
	}
	var out Enrolled
	err := c.post("/v1/runner/enrollments", body, &out)
	return out, err
}

// post — 본문을 보내고 응답을 읽는다. 두 경로가 같은 규칙을 쓰게 한 자리다 —
// 갈라 두면 한쪽에만 상태 코드 처리가 빠지는 날이 온다.
func (c *Client) post(path string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := c.request(http.MethodPost, path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("응답을 읽을 수 없다: %w", err)
	}
	return nil
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
