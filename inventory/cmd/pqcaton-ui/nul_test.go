// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/sign"
	"github.com/randyinthedev-hash/pqcota-provisioning/pkg/provisioning"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// golden — testdata/plantext/golden-v0.10.6.json. pqcota-common v0.10.6으로 만든 값이다(이 판이 v0.10.7로 올리기 전).
type golden struct {
	PublicKey    string `json:"public_key"`
	CanonicalOK  string `json:"canonical_ok_sha256"`
	ApprovalOK   string `json:"approval_ok"`
	CanonicalNUL string `json:"canonical_nul_sha256"`
	ApprovalNULA string `json:"approval_nul_a"`
}

func loadGolden(t *testing.T) golden {
	t.Helper()
	var g golden
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join("testdata", "plantext", "golden-v0.10.6.json"))), &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func nulFixture(t *testing.T, name string) (*provisioningv1.FinalizedPlan, string) {
	t.Helper()
	raw := readFile(t, filepath.Join("testdata", "plantext", name))
	p := &provisioningv1.FinalizedPlan{}
	if err := protojson.Unmarshal([]byte(raw), p); err != nil {
		t.Fatal(err)
	}
	return p, raw
}

func canonicalSHA(p *provisioningv1.FinalizedPlan) string {
	s := sha256.Sum256(sign.CanonicalPlan(p))
	return hex.EncodeToString(s[:])
}

// goldenKey — golden 값을 만든 키. 씨앗이 고정이라 ed25519 서명도 같은 바이트다.
func goldenKey() string {
	seed := sha256.Sum256([]byte("pqcaton v0.26.2 golden approval key"))
	return base64.StdEncoding.EncodeToString(ed25519.NewKeyFromSeed(seed[:]))
}

// IC-U160 — **NUL이 없는 계획은 v0.10.6과 정규형이 바이트까지 같고, 그 판의 승인이 그대로 유효하다.** v0.10.6으로 만든
// 정규형 해시 · 승인 서명(testdata/plantext/golden-v0.10.6.json)과 견준다. 같은 키로 지금 서명하면 같은 서명이 나오고, 그 판의
// 승인이 지금 VerifyApprovals에서 승인으로 남는다.
func TestNULFreePlanKeepsItsCanonicalFormAndApprovals(t *testing.T) {
	g := loadGolden(t)
	ok, _ := nulFixture(t, "plan-ok.json")
	if err := sign.CheckPlanText(ok); err != nil {
		t.Fatalf("the plan without NUL is refused: %v", err)
	}
	if got := canonicalSHA(ok); got != g.CanonicalOK {
		t.Fatalf("canonical form = %s, want the v0.10.6 bytes %s", got, g.CanonicalOK)
	}
	sig, err := sign.SignApproval(goldenKey(), "golden", ok)
	if err != nil || sig != g.ApprovalOK {
		t.Fatalf("signing again = %q, %v; want the v0.10.6 approval", sig, err)
	}
	ok.ApprovalSignatures = []string{g.ApprovalOK}
	chk := sign.VerifyApprovals(map[string]string{"golden": g.PublicKey}, ok)
	if len(chk.Approved) != 1 || chk.Approved[0] != "golden" || len(chk.Rejected) != 0 {
		t.Fatalf("the v0.10.6 approval is not valid any more: %+v", chk)
	}
}

// IC-U161 — **v0.10.6에서 정규형이 충돌하던 계획 한 쌍을 v0.10.7은 서명 · 승인 확인 · 생성 판정에서 모두 거절한다.** 두
// 계획(provider_choice와 config_artifact 사이로 NUL을 옮긴 것)은 지금도 정규형이 같다(v0.10.6의 충돌 그대로). v0.10.6에서
// 계획 A에 만든 승인은 계획 B에서 승인됐지만(golden), 이제 승인으로 남지 않는다. 두 계획 모두 서명을 만들지 않고, 승인과
// 확정 시각을 갖춰도 실행 근거가 되지 않는다.
func TestCollidingPlansAreRefused(t *testing.T) {
	g := loadGolden(t)
	a, _ := nulFixture(t, "plan-nul-a.json")
	b, _ := nulFixture(t, "plan-nul-b.json")
	if canonicalSHA(a) != g.CanonicalNUL || canonicalSHA(b) != g.CanonicalNUL {
		t.Fatalf("the pair no longer collides as it did in v0.10.6: %s %s", canonicalSHA(a), canonicalSHA(b))
	}
	for name, p := range map[string]*provisioningv1.FinalizedPlan{"A": a, "B": b} {
		if _, err := sign.SignApproval(goldenKey(), "golden", p); !errors.Is(err, sign.ErrNUL) {
			t.Errorf("plan %s: signing err = %v, want sign.ErrNUL", name, err)
		}
		p.ApprovalSignatures = []string{g.ApprovalNULA}
		if chk := sign.VerifyApprovals(map[string]string{"golden": g.PublicKey}, p); len(chk.Approved) != 0 || len(chk.Rejected) == 0 {
			t.Errorf("plan %s: the v0.10.6 approval of plan A still counts: %+v", name, chk)
		}
		p.Status, p.FinalizedAt = provisioningv1.PlanStatus_PLAN_STATUS_FINALIZED, timestamppb.New(time.Unix(1800000000, 0))
		if err := provisioning.Executable(p); !errors.Is(err, sign.ErrNUL) {
			t.Errorf("plan %s: Executable err = %v, want sign.ErrNUL", name, err)
		}
	}
}

// signConfirmFormFor: 확인 페이지가 낼 양식을 서버의 확인 토큰으로 직접 만든다. 서명 확인 처리기는 NUL 계획에 확인
// 페이지를 내지 않으므로, 서명 처리기의 잠금 안 검사만 따로 재려면 이렇게 만든다.
func signConfirmFormFor(t *testing.T, s *server, planID string) url.Values {
	t.Helper()
	b, err := os.ReadFile(s.planOut)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := s.readKey()
	form := withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove))
	r := httptest.NewRequest(http.MethodPost, ui.SignConfirmPath, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.signConfirmPage(w, r, kindApprove, ui.SignPath, ui.ScreenApprove, store.Identity{SessionID: planID},
		form.Get(ui.RevField), signPayload(b, key.Fingerprint, "보안팀장"), nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("building the confirmation form = %d\n%s", w.Code, w.Body.String())
	}
	// 이 요청은 쿠키 없이 그렸으므로 요청 보호 칸의 토큰이 시험 쿠키와 다르다. 비워 두면 postForm이 시험 토큰을 싣는다.
	f := hiddenFields(w.Body.String())
	f.Del(ui.CSRFField)
	return f
}

// IC-U162 — **옛 판에서 확정된 NUL 계획은 06의 네 자리에서 모두 `plan-text`다.** 화면은 서명 · 생성 양식을 내지 않고
// 까닭(새 판정 회차로 고친다)을 보인다. 서명 확인은 409 `plan-text`다. 서명 처리기는 확인 양식을 갖춰 보내도 잠금 안에서
// 409 `plan-text`다. 생성도 409 `plan-text`다. 작업 폴더(계획 · 서명한 계획 · 플레이북 · 영수증)는 바이트 그대로다.
func TestNULPlanIsRefusedInApproval(t *testing.T) {
	s, _ := keyReady(t)
	_, raw := nulFixture(t, "plan-nul-a.json")
	id := bindPlan(t, s, raw)
	dir := filepath.Dir(s.planOut)
	before := snapshotDir(t, dir)

	v, err := s.approveModel(ui.Page{Lang: ui.KO})
	if err != nil {
		t.Fatal(err)
	}
	if !v.TextProblem || v.TextSigned || v.CanSign() || v.CanGenerate() {
		t.Fatalf("view: text problem %v (signed %v), can sign %v, can generate %v", v.TextProblem, v.TextSigned, v.CanSign(), v.CanGenerate())
	}
	for lang, want := range map[string]string{"ko": "새 판정 회차를 열어 해당 값을 고친 뒤 다시 확정하십시오", "en": "Open a new review round, correct the affected value"} {
		page := get(t, s, ui.ScreenApprove+"?lang="+lang).Body.String()
		if strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) || strings.Contains(page, `action="`+ui.GeneratePath+`"`) ||
			!strings.Contains(page, want) {
			t.Errorf("%s: the screen offers to sign or generate, or does not say why not", lang)
		}
	}
	refused := func(t *testing.T, what string, w *httptest.ResponseRecorder) {
		t.Helper()
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedPlanText) {
			t.Errorf("%s = %d, want 409 %s\n%s", what, w.Code, ui.RefusedPlanText, w.Body.String())
		}
	}
	refused(t, "sign confirm", postForm(t, s, ui.SignConfirmPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove))))
	refused(t, "sign", postForm(t, s, ui.SignPath, signConfirmFormFor(t, s, id)))
	refused(t, "generate", postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove))))
	sameDir(t, before, snapshotDir(t, dir))
}

// IC-U163 — **서명한 계획에도 같은 검사를 한다.** 지금 계획(plan.json)은 깨끗해도 서명한 계획(plan.signed.json)에 NUL이
// 있으면, 화면은 서명한 계획의 문제로 그 까닭을 보이고 생성 양식을 내지 않으며 보관은 할 수 있다. 생성은 409 `plan-text`이고
// 파일은 바이트 그대로다.
func TestNULSignedPlanIsRefused(t *testing.T) {
	s, _ := keyReady(t)
	signed, _ := nulFixture(t, "plan-nul-b.json")
	signed.Status, signed.FinalizedAt = provisioningv1.PlanStatus_PLAN_STATUS_FINALIZED, timestamppb.New(time.Unix(1800000000, 0))
	signed.ApprovalSignatures = []string{loadGolden(t).ApprovalNULA}
	b, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(signed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.signedPath(), append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(s.planOut)
	before := snapshotDir(t, dir)

	v, err := s.approveModel(ui.Page{Lang: ui.KO})
	if err != nil {
		t.Fatal(err)
	}
	if !v.TextProblem || !v.TextSigned || v.CanGenerate() || !v.CanArchive() {
		t.Fatalf("view: text problem %v (signed %v), can generate %v, can archive %v", v.TextProblem, v.TextSigned, v.CanGenerate(), v.CanArchive())
	}
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if strings.Contains(page, `action="`+ui.GeneratePath+`"`) || !strings.Contains(page, "서명한 계획(plan.signed.json)의 문자열에 NUL 문자가 있어") {
		t.Error("the screen offers to generate, or does not say the signed plan is the problem")
	}
	w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedPlanText) {
		t.Errorf("generate = %d, want 409 %s", w.Code, ui.RefusedPlanText)
	}
	sameDir(t, before, snapshotDir(t, dir))
}

// IC-U164 — **05는 NUL 계획을 확정하지 않는다.** 되돌림 메모에 NUL을 넣어 확정을 요청하면 확인 페이지부터 409
// `session-text`이다. 세션 · 계획 · 영수증(작업 폴더 전체)은 바이트 그대로다. 거절문은 확정 전이라 그 자리에서 고치라고
// 말한다. 확정 처리기는 같은 계약 생성(review.ToContract)을 거치므로 같은 오류로 막힌다(그 함수의 시험이 잰다).
func TestReviewDoesNotFinalizeANULPlan(t *testing.T) {
	s, dir := newServer(t)
	form := url.Values{
		"policy:openssl/libssl":                {"PQC 라이브러리로 교체한다"},
		"plan:host://local/openssl/libssl":     {"on"},
		"kind:host://local/openssl/libssl":     {"REMEDIATION_KIND_CONFIG_ONLY"},
		"target:host://local/openssl/libssl":   {"ML-KEM (FIPS 203)"},
		"level:host://local/openssl/libssl":    {"L2"},
		"rollback:host://local/openssl/libssl": {"undo\x00it"},
		"reviewer":                             {"보안팀"},
		"signature":                            {"sig"},
	}
	before := snapshotDir(t, dir)
	w := postFormLang(t, s, "/finalize/confirm", "ko", form)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedSessionText) ||
		!strings.Contains(w.Body.String(), "해당 값을 고친 뒤 다시 확정하십시오") {
		t.Fatalf("finalize confirm with NUL = %d, want 409 %s\n%s", w.Code, ui.RefusedSessionText, w.Body.String())
	}
	if _, err := os.Stat(s.planOut); !os.IsNotExist(err) {
		t.Error("a plan file was written")
	}
	sameDir(t, before, snapshotDir(t, dir))
}
