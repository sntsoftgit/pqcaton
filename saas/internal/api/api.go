// Package api — 러너가 닿는 HTTP 경계.
//
// 하는 일은 셋뿐이다: **누가 말하는지 정하고(토큰→조직), 본문을 받고, [intake]에 넘긴다.**
// 판단은 여기 없다 — 검증·멱등·적재는 intake가 하고, 이 계층은 그것을 부르는 껍데기다.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	discoveryv1 "github.com/pqcota/pqcota/gen/pqcota/discovery/v1"
	"github.com/pqcota/pqcota/pkg/discovery/history"
	"github.com/pqcota/pqcota/pkg/inventory/ingest"
	"github.com/pqcota/pqcota/pkg/org"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/saas/internal/access"
	"github.com/sntsoftgit/pqcaton/saas/internal/intake"
	"github.com/sntsoftgit/pqcaton/saas/internal/jobs"
)

// DefaultMaxBody — 요청 본문 상한(바이트).
//
// 넘으면 **자르지 않고 거절한다.** 잘라서 받으면 관측 결과가 조용히 훼손되고, 그러면
// "관측했는데 없더라"가 되어 이 제품이 가장 피하는 오답이 된다.
const DefaultMaxBody = 32 << 20 // 32MiB

// DefaultJobLease — 나눠 준 작업의 점유 만료.
//
// 짧으면 멀쩡히 도는 작업을 뺏고, 길면 죽은 러너의 작업이 그만큼 묶인다(§6.2.1).
// **러너는 자기 스케줄에 깨어나 물어본다**(§3). 그래서 이 값은 그 간격보다 넉넉해야 한다 —
// 다음 실행 전에 회수되면 같은 작업이 두 번 나간다.
const DefaultJobLease = 10 * time.Minute

// StoresFor — 조직에 묶인 히스토리 저장소를 돌려준다.
//
// 조직마다 다른 핸들이 필요하다(history.NewPgStoreIn). 그 수명·풀 관리는 이 계층의
// 관심사가 아니라 조립하는 쪽(cmd)의 몫이다.
type StoresFor func(o org.ID) (history.Store, error)

// Config — 서버 설정.
type Config struct {
	// TrustProxy — 앞단에서 TLS를 끊는 배포인가.
	//
	// **기본은 false다.** X-Forwarded-Proto는 아무나 붙일 수 있어서, 앞단이 있다고
	// 밝힌 배포에서만 본다. 아니면 평문 요청이 HTTPS로 둔갑한다(§6.2).
	TrustProxy bool
	// MaxBody — 0이면 DefaultMaxBody.
	MaxBody int64
	// JobLease — 나눠 준 작업의 점유 만료. 0이면 DefaultJobLease.
	JobLease time.Duration
	// Now — 시각. 테스트가 고정한다.
	Now func() time.Time
}

// Server — 러너 API.
type Server struct {
	access access.Store
	seen   intake.SeenStore
	jobs   jobs.Store
	stores StoresFor
	cfg    Config
	log    *slog.Logger
}

// New — 서버를 만든다.
func New(a access.Store, seen intake.SeenStore, q jobs.Store, stores StoresFor, cfg Config, log *slog.Logger) *Server {
	if cfg.MaxBody == 0 {
		cfg.MaxBody = DefaultMaxBody
	}
	if cfg.JobLease == 0 {
		cfg.JobLease = DefaultJobLease
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Server{access: a, seen: seen, jobs: q, stores: stores, cfg: cfg, log: log}
}

// Handler — 라우팅.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/runner/results", s.guard(http.HandlerFunc(s.results)))
	mux.Handle("GET /v1/runner/jobs", s.guard(http.HandlerFunc(s.nextJob)))
	return mux
}

// ── 미들웨어 ───────────────────────────────────────────────────────────────

type ctxKey int

const (
	ctxOrg ctxKey = iota
	ctxToken
)

// guard — TLS 확인과 인증. 통과하면 조직이 컨텍스트에 실린다.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.TrustProxy && r.Header.Get("X-Forwarded-Proto") != "https" {
			// 조용히 받지 않는다 — 프록시 설정이 풀린 것을 아무도 모르게 되면
			// 러너 토큰이 평문으로 오가는 동안 그 사실이 어디에도 안 남는다.
			s.log.Warn("평문 요청을 받았다 — 앞단 TLS 설정을 확인해야 한다",
				"proto", r.Header.Get("X-Forwarded-Proto"), "path", r.URL.Path)
			s.fail(w, http.StatusBadRequest, "https로 와야 한다")
			return
		}

		o, rec, err := s.authenticate(r)
		if err != nil {
			// **사유를 응답에 담지 않는다.** 어느 쪽이 틀렸는지 알려 주면 시도하는 쪽에
			// 정보를 준다. 기록에는 갈라서 남긴다 — 폐기된 토큰을 계속 쓰는 러너와
			// 아무 토큰이나 넣어 보는 쪽은 다른 일이다(§6.4.1).
			s.log.Info("인증 거절", "reason", err.Error(), "path", r.URL.Path)
			s.fail(w, http.StatusUnauthorized, "인증할 수 없다")
			return
		}
		ctx := r.Context()
		ctx = ctxWith(ctx, ctxOrg, o)
		ctx = ctxWith(ctx, ctxToken, rec.Lookup)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) authenticate(r *http.Request) (org.ID, access.TokenRecord, error) {
	h := r.Header.Get("Authorization")
	raw, ok := strings.CutPrefix(h, "Bearer ")
	if !ok {
		return "", access.TokenRecord{}, access.ErrMalformed
	}
	return access.Authenticate(s.access, strings.TrimSpace(raw), s.cfg.Now())
}

// ── 결과 수신 ──────────────────────────────────────────────────────────────

type resultsRequest struct {
	// RunnerID·JobID — 이 결과가 어느 작업의 것인가. **둘 다 있으면 그 작업을 닫는다.**
	//
	// 작업을 닫는 엔드포인트를 따로 두지 않는다(§6.2는 엔드포인트가 넷이다). 나누면
	// "결과는 올렸는데 닫지 못한" 구간이 생기고, 그 작업은 만료돼 **한 번 더 배포된다.**
	RunnerID string `json:"runner_id"`
	JobID    string `json:"job_id"`

	RunnerVersion string            `json:"runner_version"`
	Results       []json.RawMessage `json:"results"`
}

type resultsResponse struct {
	Accepted   int      `json:"accepted"`
	Duplicate  int      `json:"duplicate"`
	Rejected   int      `json:"rejected"`
	Unverified int      `json:"unverified"`
	OffScope   int      `json:"off_scope"`
	Nodes      []string `json:"nodes,omitempty"`
	// Job — 작업을 닫으려 했다면 그 결과. 안 닫혔어도 **적재는 그대로다**(아래).
	Job string `json:"job,omitempty"`
}

// 작업을 닫은 결과. 러너가 무엇이 어긋났는지 알아야 다음 요청을 고칠 수 있다.
const (
	jobClosed    = "closed"
	jobNotFound  = "not-found"  // 그런 작업이 없다
	jobNotLeased = "not-leased" // 그 러너가 점유한 작업이 아니다
	jobNoRunner  = "no-runner"  // job_id는 왔는데 runner_id가 없다
	jobCloseFail = "error"      // 저장소가 답하지 않았다. 만료가 회수한다
)

func (s *Server) results(w http.ResponseWriter, r *http.Request) {
	o := orgOf(r)

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBody)
	var req resultsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			s.log.Info("본문이 상한을 넘어 거절한다", "org", o, "limit", s.cfg.MaxBody)
			s.fail(w, http.StatusRequestEntityTooLarge, "본문이 너무 크다")
			return
		}
		s.fail(w, http.StatusBadRequest, "본문을 읽을 수 없다")
		return
	}

	results := make([]*discoveryv1.CollectionResult, 0, len(req.Results))
	for _, raw := range req.Results {
		var res discoveryv1.CollectionResult
		if err := protojson.Unmarshal(raw, &res); err != nil {
			// 하나가 깨졌다고 나머지를 버리지 않는다. 다만 **조용히 넘기지도 않는다.**
			s.log.Info("계약에 맞지 않는 결과를 건너뛴다", "org", o, "err", err)
			continue
		}
		results = append(results, &res)
	}

	hist, err := s.stores(o)
	if err != nil {
		s.log.Error("조직 저장소를 열 수 없다", "org", o, "err", err)
		s.fail(w, http.StatusInternalServerError, "적재할 수 없다")
		return
	}
	rej, _ := hist.(ingest.RejectionStore) // 안 되면 nil — 요약에만 남는다

	rep, err := intake.Receive(intake.Options{
		Org:            o,
		Keys:           s.access,
		History:        hist,
		Rejections:     rej,
		Seen:           s.seen,
		RunnerVersion:  req.RunnerVersion,
		SnapshotPrefix: "api-" + s.cfg.Now().UTC().Format("20060102T150405.000Z"),
		RulesetVersion: "ruleset-v1",
	}, results)
	if err != nil {
		// 작업을 닫지 않는다. 러너가 다시 올리면 그때 닫히고, 러너가 죽으면 만료가
		// 회수한다 — 적재되지 않은 것을 「끝났다」로 두는 편이 훨씬 나쁘다.
		s.log.Error("적재 실패", "org", o, "err", err)
		s.fail(w, http.StatusInternalServerError, "적재할 수 없다")
		return
	}

	s.log.Info("결과 수신", "org", o, "accepted", rep.Accepted, "duplicate", rep.Duplicate,
		"rejected", rep.Rejected, "unverified", rep.Unverified, "runner_version", req.RunnerVersion)

	resp := resultsResponse{
		Accepted: rep.Accepted, Duplicate: rep.Duplicate, Rejected: rep.Rejected,
		Unverified: rep.Unverified, OffScope: rep.OffScope, Nodes: rep.Nodes,
	}
	if req.JobID != "" {
		resp.Job = s.closeJob(o, req.JobID, req.RunnerID)
	}
	s.ok(w, resp)
}

// closeJob — 결과를 올린 러너가 점유한 작업을 닫는다.
//
// **닫지 못해도 결과를 버리지 않는다.** 여기서 요청 전체를 실패시키면 이미 적재된 관측을
// 러너가 다시 올리게 되고, 정작 어긋난 것(작업 id·러너 id)은 그대로다. 무엇이 어긋났는지는
// 응답에 담아 돌려준다 — 적재와 작업은 별개의 일이다.
//
// **거절된 결과가 있어도 닫는다.** 러너는 시킨 일을 했다. 서명이 맞지 않는 것은 다시
// 시킨다고 달라지지 않고(옛 collector는 같은 서명을 다시 만든다), 사유는 `pqcota_rejections`에
// 남는다 — 무한히 다시 배포하는 것이 답이 아니다(§6.6).
func (s *Server) closeJob(o org.ID, id, runnerID string) string {
	if runnerID == "" {
		// 누가 점유했는지 모르면 맞춰 볼 수 없다. 조용히 닫으면 남의 작업을 닫는 길이 된다.
		s.log.Info("작업을 닫을 수 없다 — runner_id가 없다", "org", o, "job", id)
		return jobNoRunner
	}
	err := s.jobs.Complete(o, id, runnerID, s.cfg.Now())
	switch {
	case err == nil:
		s.log.Info("작업을 닫았다", "org", o, "job", id, "runner", runnerID)
		return jobClosed
	case errors.Is(err, jobs.ErrNotFound):
		s.log.Info("없는 작업을 닫으려 했다", "org", o, "job", id, "runner", runnerID)
		return jobNotFound
	case errors.Is(err, jobs.ErrNotLeased):
		// 점유가 이미 만료돼 회수됐거나, 남의 작업이다. 어느 쪽이든 이 러너가 닫을 것이 아니다.
		s.log.Warn("점유하지 않은 작업을 닫으려 했다", "org", o, "job", id, "runner", runnerID)
		return jobNotLeased
	default:
		s.log.Error("작업을 닫지 못했다", "org", o, "job", id, "err", err)
		return jobCloseFail
	}
}

// ── 작업 배포 ──────────────────────────────────────────────────────────────

// jobResponse — 러너에게 넘기는 작업 하나.
//
// **조직을 담지 않는다.** 러너는 자기 조직을 토큰으로만 알고, 우리가 되돌려 주면 그것이
// 다음 요청에 실려 오는 날이 온다 — 조직이 러너가 주장하는 것이 되는 첫 걸음이다(§6.4).
type jobResponse struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Targets   []string  `json:"targets,omitempty"`
	Payload   []byte    `json:"payload,omitempty"`
	LeaseTill time.Time `json:"lease_till"`
	Attempts  int       `json:"attempts"`
}

// nextJob — 할 일을 하나 받아간다. 없으면 204.
//
// **기다리지 않는다.** 러너는 상주하지 않고 **자기 스케줄에 깨어나** 물어본다(§3) —
// 붙들고 기다려 봐야 그 프로세스는 곧 끝난다. 새 작업이 러너에 닿는 지연은 그래서
// **스케줄 간격**이고, 그것이 이 배포 형태의 대가다.
//
// **없으면 204다.** 빈 몸통을 200으로 주면 "작업 없음"과 "작업이 왔는데 못 읽었다"가
// 러너 쪽에서 같은 모양이 된다.
func (s *Server) nextJob(w http.ResponseWriter, r *http.Request) {
	o := orgOf(r)

	runnerID := strings.TrimSpace(r.URL.Query().Get("runner_id"))
	if runnerID == "" {
		// 누가 가져갔는지 모르면 완료 보고와 점유를 맞춰 볼 수 없다 — 그 작업은
		// 만료될 때까지 아무도 닫지 못한다.
		s.fail(w, http.StatusBadRequest, "runner_id가 있어야 한다")
		return
	}

	now := s.cfg.Now()
	j, ok, err := s.jobs.Lease(o, runnerID, now.Add(s.cfg.JobLease), now)
	if err != nil {
		s.log.Error("작업을 꺼낼 수 없다", "org", o, "runner", runnerID, "err", err)
		s.fail(w, http.StatusInternalServerError, "작업을 꺼낼 수 없다")
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.log.Info("작업을 배포했다", "org", o, "runner", runnerID,
		"job", j.ID, "kind", j.Kind, "attempts", j.Attempts)
	s.ok(w, jobResponse{
		ID: j.ID, Kind: string(j.Kind), Targets: j.Targets, Payload: j.Payload,
		LeaseTill: j.LeaseTill, Attempts: j.Attempts,
	})
}

// ── 응답 ───────────────────────────────────────────────────────────────────

func (s *Server) ok(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) fail(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
