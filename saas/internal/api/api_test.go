package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/pqcota/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/pqcota/pqcota/gen/pqcota/discovery/v1"
	"github.com/pqcota/pqcota/pkg/discovery/history"
	"github.com/pqcota/pqcota/pkg/kernel/sign"
	"github.com/pqcota/pqcota/pkg/org"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/saas/internal/access"
	"github.com/sntsoftgit/pqcaton/saas/internal/api"
	"github.com/sntsoftgit/pqcaton/saas/internal/intake"
)

var t0 = time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)

const collector = "openssl-collector"

type harness struct {
	srv    http.Handler
	acc    *access.MemStore
	stores map[org.ID]*history.MemStore
	token  string // acme의 유효 토큰 평문
	priv   string // acme에 등록된 collector 개인키
}

func newHarness(t *testing.T, cfg api.Config) *harness {
	t.Helper()
	acc := access.NewMemStore()
	h := &harness{acc: acc, stores: map[org.ID]*history.MemStore{}}

	tok, err := access.NewToken()
	if err != nil {
		t.Fatalf("발급: %v", err)
	}
	if err := acc.PutToken(access.TokenRecord{
		Lookup: tok.Lookup, Org: "acme", Digest: tok.Digest(), IssuedAt: t0,
	}); err != nil {
		t.Fatalf("토큰 저장: %v", err)
	}
	h.token = tok.Plaintext

	pub, priv, _ := sign.Generate()
	h.priv = priv
	if err := acc.RegisterKey(access.CollectorKey{
		Org: "acme", CollectorID: collector, PublicKey: pub, Registered: t0,
	}); err != nil {
		t.Fatalf("키 등록: %v", err)
	}

	if cfg.Now == nil {
		cfg.Now = func() time.Time { return t0 }
	}
	stores := func(o org.ID) (history.Store, error) {
		if _, ok := h.stores[o]; !ok {
			h.stores[o] = history.NewMemStore()
		}
		return h.stores[o], nil
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	h.srv = api.New(acc, intake.NewMemSeen(), stores, cfg, quiet).Handler()
	return h
}

// body — 서명된 결과 하나를 담은 요청 본문. extra는 본문에 더 끼워 넣을 필드.
func (h *harness) body(t *testing.T, node string, extra map[string]any) []byte {
	t.Helper()
	res := &discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{
			CollectorId: collector, CollectorVersion: "0.1.0",
			DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION,
			CollectedAt:     timestamppb.New(t0), TargetNodeId: node,
		},
		RawCapture: []byte(node), RawFormat: "openssl-collector/native-v1",
	}
	sig, err := sign.Sign(h.priv, res)
	if err != nil {
		t.Fatalf("서명: %v", err)
	}
	res.Envelope.Signature = sig
	raw, err := protojson.Marshal(res)
	if err != nil {
		t.Fatalf("직렬화: %v", err)
	}
	payload := map[string]any{"runner_version": "0.1.0", "results": []json.RawMessage{raw}}
	for k, v := range extra {
		payload[k] = v
	}
	b, _ := json.Marshal(payload)
	return b
}

func (h *harness) post(body []byte, mut ...func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/v1/runner/results", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+h.token)
	r.Header.Set("Content-Type", "application/json")
	for _, m := range mut {
		m(r)
	}
	w := httptest.NewRecorder()
	h.srv.ServeHTTP(w, r)
	return w
}

// CP-HTTP-1 — 유효한 토큰과 서명된 결과가 통과해 그 조직에 쌓인다.
func TestResultsAcceptedAndStoredInTokensOrg(t *testing.T) {
	h := newHarness(t, api.Config{})
	w := h.post(h.body(t, "web-01", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("상태 %d: %s", w.Code, w.Body.String())
	}
	var got struct{ Accepted, Rejected, Duplicate int }
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Accepted != 1 || got.Rejected != 0 {
		t.Fatalf("통과하지 못했다: %+v", got)
	}
	if snap, _ := h.stores["acme"].Latest("web-01"); snap == nil {
		t.Fatal("acme 저장소에 쌓이지 않았다")
	}
}

// CP-HTTP-2 — **조직은 본문에서 오지 않는다.**
//
// 본문에 다른 조직을 적어도 토큰의 조직에 쌓인다. 핸들러가 본문에서 조직을 읽을 방법을
// 두지 않는 것이 이 성질을 지킨다(§6.4).
func TestOrgComesFromTokenNotBody(t *testing.T) {
	h := newHarness(t, api.Config{})
	w := h.post(h.body(t, "web-01", map[string]any{"org": "beta"}))
	if w.Code != http.StatusOK {
		t.Fatalf("상태 %d: %s", w.Code, w.Body.String())
	}
	if _, ok := h.stores["beta"]; ok {
		t.Fatal("본문이 주장한 조직에 저장소가 열렸다")
	}
	if snap, _ := h.stores["acme"].Latest("web-01"); snap == nil {
		t.Fatal("토큰의 조직에 쌓이지 않았다")
	}
}

// CP-HTTP-3 — 인증 실패는 401이고, **응답이 사유를 구분하지 않는다.**
//
// 어느 쪽이 틀렸는지 알려 주면 시도하는 쪽에 정보를 준다. 구분은 기록에서만 한다.
func TestAuthFailuresAreIndistinguishable(t *testing.T) {
	h := newHarness(t, api.Config{})

	other, _ := access.NewToken() // 등록되지 않은 토큰
	unknown := h.post(h.body(t, "web-01", nil), func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+other.Plaintext)
	})

	lookup, _, _ := access.SplitToken(h.token)
	forged := access.Prefix + "_" + lookup + "_" + strings.Repeat("z", 32)
	wrongSecret := h.post(h.body(t, "web-01", nil), func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+forged)
	})

	none := h.post(h.body(t, "web-01", nil), func(r *http.Request) {
		r.Header.Del("Authorization")
	})

	for name, w := range map[string]*httptest.ResponseRecorder{
		"모르는 토큰": unknown, "비밀 불일치": wrongSecret, "헤더 없음": none,
	} {
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: 상태 %d", name, w.Code)
		}
	}
	if unknown.Body.String() != wrongSecret.Body.String() || unknown.Body.String() != none.Body.String() {
		t.Fatalf("응답이 사유를 드러낸다:\n%s%s%s",
			unknown.Body.String(), wrongSecret.Body.String(), none.Body.String())
	}
}

// CP-HTTP-4 — 본문이 상한을 넘으면 **자르지 않고 거절한다.**
//
// 잘라서 받으면 관측 결과가 조용히 훼손되고 "관측했는데 없더라"가 된다.
func TestOversizedBodyIsRejectedNotTruncated(t *testing.T) {
	h := newHarness(t, api.Config{MaxBody: 512})
	big := h.body(t, "web-01", map[string]any{"padding": strings.Repeat("x", 4096)})

	w := h.post(big)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("상태 %d: %s", w.Code, w.Body.String())
	}
	if s, ok := h.stores["acme"]; ok {
		if snap, _ := s.Latest("web-01"); snap != nil {
			t.Fatal("잘린 본문이 적재됐다")
		}
	}
}

// CP-HTTP-5 — 앞단을 신뢰하는 배포에서 평문 요청은 거절한다.
// 조용히 받으면 프록시 설정이 풀린 것을 아무도 모른다.
func TestTrustProxyRejectsPlainRequest(t *testing.T) {
	h := newHarness(t, api.Config{TrustProxy: true})
	if w := h.post(h.body(t, "web-01", nil)); w.Code != http.StatusBadRequest {
		t.Fatalf("평문 요청이 통과했다: %d", w.Code)
	}
	w := h.post(h.body(t, "web-01", nil), func(r *http.Request) {
		r.Header.Set("X-Forwarded-Proto", "https")
	})
	if w.Code != http.StatusOK {
		t.Fatalf("https 요청이 막혔다: %d %s", w.Code, w.Body.String())
	}
}

// CP-HTTP-6 — 신뢰하지 않는 배포에서는 그 헤더를 **보지 않는다.**
// 아무나 붙일 수 있는 헤더라, 믿으면 평문 요청이 HTTPS로 둔갑한다.
func TestForwardedProtoIgnoredWhenNotTrusted(t *testing.T) {
	h := newHarness(t, api.Config{TrustProxy: false})
	w := h.post(h.body(t, "web-01", nil), func(r *http.Request) {
		r.Header.Set("X-Forwarded-Proto", "http")
	})
	if w.Code != http.StatusOK {
		t.Fatalf("헤더를 보고 막았다: %d %s", w.Code, w.Body.String())
	}
}

// CP-HTTP-7 — 계약에 맞지 않는 결과가 섞여도 나머지는 들어간다.
func TestBrokenResultDoesNotDropTheBatch(t *testing.T) {
	h := newHarness(t, api.Config{})
	good := h.body(t, "web-01", nil)

	var payload map[string]any
	_ = json.Unmarshal(good, &payload)
	items := payload["results"].([]any)
	payload["results"] = append([]any{json.RawMessage(`{"envelope":{"collectedAt":"이건 시각이 아니다"}}`)}, items...)
	mixed, _ := json.Marshal(payload)

	w := h.post(mixed)
	if w.Code != http.StatusOK {
		t.Fatalf("상태 %d: %s", w.Code, w.Body.String())
	}
	var got struct{ Accepted int }
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Accepted != 1 {
		t.Fatalf("멀쩡한 결과가 함께 버려졌다: %+v", got)
	}
}
