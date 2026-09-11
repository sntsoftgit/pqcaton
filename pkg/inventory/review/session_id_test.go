package review_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	provisioningv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// 판정과 실행 승인은 다른 단계다. 이 리포가 내는 것은 판정이 끝난 계획이고, 실행 승인은 상류가
// 한다. 그 경계를 계약의 상태가 표시하고, 세션 id 가 계획과 원장을 잇는다.

func judgedSession() review.Session {
	return review.Session{
		Scope: "org://acme", Reviewer: "보안팀", Signature: "kty-2026-09-11",
		RulesetVersion: review.RulesetVersion, SessionID: review.NewSessionID(),
		PolicyDecisions: map[string]string{"openssl/libssl": "교체"},
		Items: []review.Item{{
			ID: "n1/openssl/libssl", Node: "n1", Runtime: "openssl", Policy: "openssl/libssl",
			State: "UNDECLARED", Conf: 0.6, Mandatory: true, Plan: true,
			Level: "L2", Kind: "REMEDIATION_KIND_CONFIG_ONLY", TargetAlgorithm: "ML-KEM (FIPS 203)",
		}},
	}
}

// ★ IC-P8 — 계약으로 나가는 계획은 IN_REVIEW 이고 승인 칸과 확정 시각이 비어 있다.
//
// 전에는 판정자의 자유 문자열을 approval_signatures 에 넣고 FINALIZED 를 달았다. 그 이름표는
// 검증되지 않는 것이라 아무것도 증명하지 않으면서, 상류 구조 관문의 「승인 항목 있음」을
// 충족하는 모양이 됐다.
func TestJudgedPlanLeavesApprovalToUpstream(t *testing.T) {
	res, err := review.Finalize(judgedSession())
	if err != nil {
		t.Fatalf("확정되지 않았다: %v", err)
	}
	p := res.Plan
	if p.GetStatus() != provisioningv1.PlanStatus_PLAN_STATUS_IN_REVIEW {
		t.Errorf("판정이 끝난 계획은 IN_REVIEW 여야 한다: %s", p.GetStatus())
	}
	if len(p.GetApprovalSignatures()) != 0 {
		t.Errorf("승인 칸은 실행 승인의 자리다 — 이쪽이 채우면 안 된다: %v", p.GetApprovalSignatures())
	}
	if p.GetFinalizedAt() != nil {
		t.Errorf("확정 시각은 상류의 첫 승인이 찍는다: %v", p.GetFinalizedAt())
	}
}

// IC-P8 — 계획 id 는 세션 id 를 담고, 검토자가 적은 문자열은 들어가지 않는다.
func TestPlanIDIsTheSessionID(t *testing.T) {
	sf := judgedSession()
	res, err := review.Finalize(sf)
	if err != nil {
		t.Fatal(err)
	}
	want := "pqcaton:" + sf.Scope + ":" + sf.SessionID
	if res.Plan.GetId() != want {
		t.Errorf("계획 id 가 세션 id 를 담지 않는다: %q (want %q)", res.Plan.GetId(), want)
	}
	if strings.Contains(res.Plan.GetId(), sf.Signature[:8]) {
		t.Errorf("검토자가 적은 문자열이 계획 id 에 박혔다: %q", res.Plan.GetId())
	}
}

// IC-P9 — 세션 id 가 없는 세션(옛 빌드가 연 것)은 확정하지 않고 다시 열라고 말한다.
//
// 여기서 새로 찍으면 그 세션의 원장 행들은 빈 세션 id 를 갖고 있어, 계획은 세션을 가리키는데
// 원장에는 그 세션이 없는 상태가 된다.
func TestSessionWithoutIDMustBeReopened(t *testing.T) {
	sf := judgedSession()
	sf.SessionID = ""
	_, err := review.Finalize(sf)
	if err == nil {
		t.Fatal("세션 id 없는 세션을 확정했다")
	}
	if !strings.Contains(err.Error(), "session_id") || !strings.Contains(err.Error(), "raise it again") {
		t.Errorf("무엇이 없고 어떻게 하라는지 말하지 않는다: %v", err)
	}
}

// IC-P9 — 다시 열면 같은 id 다. 새 세션이면 내용이 같아도 다른 id 다. 앞 세션에 id 가 없으면
// 새로 만든 것을 그대로 둔다 — 지금 여는 것이 곧 「다시 열기」다.
func TestSessionIDSurvivesReopenButNotNewSessions(t *testing.T) {
	prev := judgedSession()
	fresh := judgedSession() // 같은 내용, 새로 연 세션
	if prev.SessionID == fresh.SessionID {
		t.Fatal("새 세션인데 id 가 같다")
	}
	carried := review.Carry(prev, fresh)
	if carried.SessionID != prev.SessionID {
		t.Errorf("다시 열었는데 세션 id 가 바뀌었다 — 원장의 판정이 여러 id 로 흩어진다")
	}
	old := judgedSession()
	old.SessionID = "" // 옛 빌드가 연 세션
	again := judgedSession()
	got := review.Carry(old, again)
	if got.SessionID != again.SessionID || got.SessionID == "" {
		t.Errorf("앞 세션에 id 가 없으면 새 id 를 지켜야 한다: %q", got.SessionID)
	}
}

// 세션 id 의 모양. UUID v4 이고 만들 때마다 다르다.
func TestNewSessionIDShape(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := review.NewSessionID()
		if !re.MatchString(id) {
			t.Fatalf("UUID v4 가 아니다: %s", id)
		}
		if seen[id] {
			t.Fatalf("같은 id 가 두 번 나왔다: %s", id)
		}
		seen[id] = true
	}
}

// ★ IC-D22 — 계획에서 원장으로 실제로 되짚어진다.
//
// 계획 id 에서 세션 id 를 읽어 원장을 찾으면 그 세션의 판정이 나오고, 다른 세션의 것은 섞이지
// 않는다. 저장만 하고 찾는 길이 없으면 「원장에서 찾을 수 있다」가 기능이 아니라 가능성이다.
func TestPlanTracesBackToItsJudgments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "judgments.jsonl")
	a, b := judgedSession(), judgedSession()

	resA, err := review.Finalize(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := review.SaveJudgments(path, "acme", a, resA.Decided); err != nil {
		t.Fatal(err)
	}
	resB, _ := review.Finalize(b)
	if _, err := review.SaveJudgments(path, "acme", b, resB.Decided); err != nil {
		t.Fatal(err)
	}

	// 계획 id → 세션 id → 원장.
	sessionID := strings.TrimPrefix(resA.Plan.GetId(), "pqcaton:"+a.Scope+":")
	st, _ := decision.NewFileJudgmentStore(org.ID("acme"), path)
	got, err := st.BySessionID(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SessionID != a.SessionID {
		t.Fatalf("계획이 가리키는 세션의 판정이 나오지 않는다: %+v", got)
	}
	if got[0].Subject != a.Items[0].ID {
		t.Errorf("다른 항목의 판정이 나왔다: %s", got[0].Subject)
	}
}
