// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/sign"
	"github.com/randyinthedev-hash/pqcota-provisioning/pkg/provisioning"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// keyReady: 계획(검토 중)이 있는 서버와, 작업 폴더 밖에 둔 소유자 전용 승인 키. pqcota-keygen의 출력 그대로다.
func keyReady(t *testing.T) (*server, string) {
	t.Helper()
	s := everyScreen(t)
	pub, priv, err := sign.Generate()
	if err != nil {
		t.Fatal(err)
	}
	s.approvalKey = filepath.Join(t.TempDir(), "approval.key")
	out := "# sign on the node scanner: export PQCOTA_SIGN_KEY=<priv>\nPQCOTA_SIGN_KEY=" + priv + "\n\nPQCOTA_VERIFY_KEY=" + pub + "\n"
	if err := os.WriteFile(s.approvalKey, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	return s, pub
}

func signForm(approver string) url.Values {
	return url.Values{"approver": {approver}, "reviewed": {"on"}}
}

// IC-U120 — **화면의 승인 서명은 상류가 그대로 확인한다.** 06 화면은 계획의 조치를 보이고, 승인자 이름 · 검토
// 확인과 함께 서명하면 plan.signed.json에 FINALIZED 계획을 쓴다. plan.json은 그대로다. 서명은 pqcota-provision이
// 쓰는 확인 함수(VerifyApprovals)가 그 승인자의 공개 키로 통과시키고, 화면은 그 확인 키 줄을 보인다. 06 카드는
// 「플레이북을 만들 수 있습니다」가 된다.
func TestApprovalIsVerifiedUpstream(t *testing.T) {
	s, pub := keyReady(t)
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	for _, want := range []string{"ML-KEM (FIPS 203)", "REMEDIATION_KIND_CONFIG_ONLY", `action="` + ui.SignPath + `"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the 06 screen does not show %q", want)
		}
	}
	before := readFile(t, s.planOut)
	w := postForm(t, s, ui.SignPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d\n%s", w.Code, w.Body.String())
	}
	if readFile(t, s.planOut) != before {
		t.Error("signing rewrote plan.json")
	}
	signed, err := loadPlanFile(s.signedPath())
	if err != nil || signed == nil {
		t.Fatalf("plan.signed.json: %v", err)
	}
	if signed.GetStatus() != provisioningv1.PlanStatus_PLAN_STATUS_FINALIZED || signed.GetFinalizedAt() == nil {
		t.Errorf("signed plan status %s", signed.GetStatus())
	}
	chk := sign.VerifyApprovals(map[string]string{"보안팀장": pub}, signed)
	if len(chk.Approved) != 1 || len(chk.Rejected) != 0 {
		t.Errorf("upstream verification: %+v", chk)
	}
	if err := provisioning.Executable(signed); err != nil {
		t.Errorf("the signed plan is not executable upstream: %v", err)
	}
	page = get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if !strings.Contains(page, "PQCOTA_APPROVAL_KEYS=보안팀장="+pub) || !strings.Contains(page, `action="`+ui.GeneratePath+`"`) {
		t.Errorf("after signing the screen does not offer the verify line and generation:\n%s", page)
	}
	if got := cards(t, page)["06"]; !strings.HasPrefix(got, "플레이북을 만들 수 있습니다") {
		t.Errorf("card 06 = %q", got)
	}
	// 같은 계획에 다시 서명하지 않는다.
	if w := postForm(t, s, ui.SignPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove))); w.Code != http.StatusConflict {
		t.Errorf("signing again = %d, want 409", w.Code)
	}
}

// IC-U121 — **쓸 수 없는 승인 키로는 서명하지 않는다.** 키를 주지 않았거나, 다른 사용자가 읽을 수 있거나(0644),
// 작업 폴더(명시 모드면 계획이 놓이는 폴더) 안에 있거나, pqcota-keygen의 개인 키가 아니면 06 화면이 그 까닭과 할
// 일을 두 말로 적고 서명 양식을 내지 않는다. 주소로 직접 보내도 409(`sign-not-ready`)이고 아무것도 쓰지 않는다.
// 승인자 이름에 콜론이 있거나 검토 확인이 없어도 같다.
func TestUnusableKeysDoNotSign(t *testing.T) {
	cases := []struct {
		name   string
		set    func(t *testing.T, s *server)
		ko, en string
	}{
		{"not given", func(t *testing.T, s *server) { s.approvalKey = "" }, "-approval-key &lt;파일&gt;로 시작하십시오", "Start with -approval-key &lt;file&gt;"},
		{"open", func(t *testing.T, s *server) { _ = os.Chmod(s.approvalKey, 0o644) }, "chmod 600", "chmod 600"},
		{"inside", func(t *testing.T, s *server) {
			in := filepath.Join(filepath.Dir(s.planOut), "approval.key")
			if err := os.WriteFile(in, []byte(readFile(t, s.approvalKey)), 0o600); err != nil {
				t.Fatal(err)
			}
			s.approvalKey = in
		}, "작업 폴더 밖으로 옮기십시오", "move it out"},
		{"bad", func(t *testing.T, s *server) { _ = os.WriteFile(s.approvalKey, []byte("not a key\n"), 0o600) },
			"PQCOTA_SIGN_KEY", "PQCOTA_SIGN_KEY"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := keyReady(t)
			c.set(t, s)
			for lang, want := range map[string]string{"ko": c.ko, "en": c.en} {
				page := get(t, s, ui.ScreenApprove+"?lang="+lang).Body.String()
				if !strings.Contains(page, want) || strings.Contains(page, `action="`+ui.SignPath+`"`) {
					t.Errorf("%s: the screen does not say %q or still offers signing", lang, want)
				}
			}
			w := postForm(t, s, ui.SignPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove)))
			if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedSignNotReady) {
				t.Errorf("sign = %d, want 409 %s", w.Code, ui.RefusedSignNotReady)
			}
			if _, err := os.Stat(s.signedPath()); err == nil {
				t.Error("a refused signature was written")
			}
		})
	}
	s, _ := keyReady(t)
	for _, form := range []url.Values{signForm("a:b"), {"approver": {"보안팀장"}}} {
		if w := postForm(t, s, ui.SignPath, withRev(form, s.revFor(ui.ScreenApprove))); w.Code != http.StatusConflict {
			t.Errorf("form %v = %d, want 409", form, w.Code)
		}
	}
}

// IC-U122 — **플레이북은 상류 생성기가 만드는 것과 같고, 계획이 바뀌면 낡은 것으로 보인다.** 서명한 뒤 「플레이북
// 생성」(L3)은 provision.yml과 provision-rollback.yml을 plan.json 옆에 쓰고, 두 파일은 같은 계획 · 수준으로
// provisioning.Generate…Playbook이 내는 것과 바이트까지 같다. 생성기가 짚은 빈 곳(L3인데 활성화 훅이 없음 등)을
// 화면이 세어 보인다. 그 뒤 05에서 계획이 달라지면 06 카드는 「서명이 지금 계획과 맞지 않습니다」이고 생성은
// 409(`generate-not-ready`)다.
func TestPlaybooksMatchUpstreamAndGoStale(t *testing.T) {
	s, _ := keyReady(t)
	if w := postForm(t, s, ui.SignPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d", w.Code)
	}
	if w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l9"}}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusBadRequest {
		t.Errorf("level l9 = %d, want 400", w.Code)
	}
	w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l3"}}, s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("generate = %d\n%s", w.Code, w.Body.String())
	}
	signed, _ := loadPlanFile(s.signedPath())
	l3 := provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L3_FULL_AUTO
	if readFile(t, s.provisionPath()) != provisioning.GenerateProvisioningPlaybook(signed, l3) {
		t.Error("provision.yml differs from the upstream generator")
	}
	if readFile(t, s.rollbackPath()) != provisioning.GenerateRollbackPlaybook(signed, l3) {
		t.Error("provision-rollback.yml differs from the upstream generator")
	}
	warnings := genWarnings(signed, l3)
	if len(warnings) == 0 {
		t.Fatal("the sample plan should leave gaps at L3 (no activation hooks)")
	}
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if got := cards(t, page)["06"]; got != ui.StepApproveIncompleteText(ui.KO, len(warnings)) {
		t.Errorf("card 06 = %q", got)
	}
	if !strings.Contains(page, "provision-rollback.yml") || !strings.Contains(page, "L3") {
		t.Errorf("the screen does not show the generated files and level")
	}

	// 05에서 계획이 달라졌다.
	changed := strings.Replace(planJSON, "ML-KEM (FIPS 203)", "ML-KEM-768", 1)
	if err := os.WriteFile(s.planOut, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	page = get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if got := cards(t, page)["06"]; got != "서명이 지금 계획과 맞지 않습니다" {
		t.Errorf("card 06 after the plan changed = %q", got)
	}
	w = postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedGenNotReady) {
		t.Errorf("generate on a stale signature = %d, want 409 %s", w.Code, ui.RefusedGenNotReady)
	}
}
