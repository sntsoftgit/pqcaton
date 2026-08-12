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
)

// DefaultMaxBody — 요청 본문 상한(바이트).
//
// 넘으면 **자르지 않고 거절한다.** 잘라서 받으면 관측 결과가 조용히 훼손되고, 그러면
// "관측했는데 없더라"가 되어 이 제품이 가장 피하는 오답이 된다.
const DefaultMaxBody = 32 << 20 // 32MiB

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
	// Now — 시각. 테스트가 고정한다.
	Now func() time.Time
}

// Server — 러너 API.
type Server struct {
	access access.Store
	seen   intake.SeenStore
	stores StoresFor
	cfg    Config
	log    *slog.Logger
}

// New — 서버를 만든다.
func New(a access.Store, seen intake.SeenStore, stores StoresFor, cfg Config, log *slog.Logger) *Server {
	if cfg.MaxBody == 0 {
		cfg.MaxBody = DefaultMaxBody
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Server{access: a, seen: seen, stores: stores, cfg: cfg, log: log}
}

// Handler — 라우팅.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/runner/results", s.guard(http.HandlerFunc(s.results)))
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
}

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
		s.log.Error("적재 실패", "org", o, "err", err)
		s.fail(w, http.StatusInternalServerError, "적재할 수 없다")
		return
	}

	s.log.Info("결과 수신", "org", o, "accepted", rep.Accepted, "duplicate", rep.Duplicate,
		"rejected", rep.Rejected, "unverified", rep.Unverified, "runner_version", req.RunnerVersion)

	s.ok(w, resultsResponse{
		Accepted: rep.Accepted, Duplicate: rep.Duplicate, Rejected: rep.Rejected,
		Unverified: rep.Unverified, OffScope: rep.OffScope, Nodes: rep.Nodes,
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
