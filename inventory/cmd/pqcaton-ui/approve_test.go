// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"net/http/httptest"
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
	s.approvalKey = filepath.Join(t.TempDir(), "approval.key")
	return s, writeKey(t, s.approvalKey)
}

// writeKey: 새 키 쌍을 만들어 pqcota-keygen의 출력 모양으로 쓰고 공개 키를 돌려준다.
func writeKey(t *testing.T, p string) string {
	t.Helper()
	pub, priv, err := sign.Generate()
	if err != nil {
		t.Fatal(err)
	}
	out := "# sign on the node scanner: export PQCOTA_SIGN_KEY=<priv>\nPQCOTA_SIGN_KEY=" + priv + "\n\nPQCOTA_VERIFY_KEY=" + pub + "\n"
	if err := os.WriteFile(p, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	return pub
}

func signForm(approver string) url.Values {
	return url.Values{"approver": {approver}, "reviewed": {"on"}}
}

// signVia: 화면이 하는 그대로 서명한다. 확인 요청을 보내고 확인 페이지의 양식을 서명 주소로 보낸다.
func signVia(t *testing.T, s *server, approver string) *httptest.ResponseRecorder {
	t.Helper()
	return finalizeVia(t, s, "ko", ui.SignConfirmPath, withRev(signForm(approver), s.revFor(ui.ScreenApprove)))
}

// IC-U120 — **화면의 승인 서명은 확인을 거치고, 상류가 그대로 확인한다.** 06 화면은 계획의 조치를 보이고, 서명 양식은
// 먼저 확인 페이지로 간다. 확인 페이지는 계획 · 계획 파일 해시 · 승인자 · 키 지문을 보이고 아무것도 쓰지 않는다. 확인을
// 거친 서명은 계획 파일 옆 plan.signed.json에 FINALIZED 계획을 쓰고 plan.json은 그대로다. 서명은 상류 VerifyApprovals가
// 그 승인자의 공개 키로 통과시키고 Executable도 지난다. 화면은 서명한 뒤에만 확인 키 줄을 보이고 06 카드는 「플레이북을
// 만들 수 있습니다」다. 같은 계획에 다시 서명하는 양식은 없고, 직접 보내도 409(`signed-exists`)다.
func TestApprovalIsVerifiedUpstream(t *testing.T) {
	s, pub := keyReady(t)
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	for _, want := range []string{"ML-KEM (FIPS 203)", "config_only", "IN_REVIEW", `action="` + ui.SignConfirmPath + `"`, "단일 키"} {
		if !strings.Contains(page, want) {
			t.Errorf("the 06 screen does not show %q", want)
		}
	}
	if strings.Contains(page, "PQCOTA_APPROVAL_KEYS=") {
		t.Error("the verify line is shown before anyone signed (it has no approver yet)")
	}
	before := readFile(t, s.planOut)
	w := postForm(t, s, ui.SignConfirmPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove)))
	key, _ := s.readKey()
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), key.Fingerprint) || !strings.Contains(w.Body.String(), "보안팀장") ||
		!strings.Contains(w.Body.String(), "plan-1") {
		t.Fatalf("the confirmation page = %d\n%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(s.signedPath()); err == nil {
		t.Fatal("the confirmation wrote the signed plan")
	}
	if w := postFormLang(t, s, ui.SignPath, "ko", hiddenFields(w.Body.String())); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d\n%s", w.Code, w.Body.String())
	}
	if readFile(t, s.planOut) != before {
		t.Error("signing rewrote plan.json")
	}
	if filepath.Base(s.signedPath()) != "plan.signed.json" {
		t.Errorf("signed plan path %s", s.signedPath())
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
	if strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) || strings.Contains(page, `action="`+ui.ArchivePath+`"`) {
		t.Error("a valid signature is offered for re-signing or archiving")
	}
	if got := cards(t, page)["06"]; !strings.HasPrefix(got, "플레이북을 만들 수 있습니다") {
		t.Errorf("card 06 = %q", got)
	}
	if w := postForm(t, s, ui.SignConfirmPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove))); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), ui.RefusedSignedExists) {
		t.Errorf("signing again = %d, want 409 %s", w.Code, ui.RefusedSignedExists)
	}
}

// IC-U121 — **쓸 수 없는 승인 키로는 서명하지 않는다.** 키를 주지 않았거나, 다른 사용자가 읽을 수 있거나(0644), 심볼릭
// 링크이거나, 하드 링크가 더 있거나, 작업 폴더(명시 모드면 계획이 놓이는 폴더) 안에 있거나(그 폴더를 가리키는 링크를
// 거쳐도), pqcota-keygen의 개인 키가 아니면 06 화면이 그 까닭과 할 일을 두 말로 적고 서명 양식을 내지 않는다. 주소로
// 직접 보내도 409(`sign-not-ready`)이고 아무것도 쓰지 않는다. 승인자 이름에 콜론이 있거나 검토 확인이 없어도 같다.
func TestUnusableKeysDoNotSign(t *testing.T) {
	cases := []struct {
		name   string
		set    func(t *testing.T, s *server)
		ko, en string
	}{
		{"not given", func(t *testing.T, s *server) { s.approvalKey = "" }, "-approval-key &lt;파일&gt;로 시작하십시오", "Start with -approval-key &lt;file&gt;"},
		{"open", func(t *testing.T, s *server) { _ = os.Chmod(s.approvalKey, 0o644) }, "chmod 600", "chmod 600"},
		{"symlink", func(t *testing.T, s *server) {
			l := filepath.Join(t.TempDir(), "link.key")
			if err := os.Symlink(s.approvalKey, l); err != nil {
				t.Fatal(err)
			}
			s.approvalKey = l
		}, "심볼릭 링크입니다", "symbolic link"},
		{"hard link", func(t *testing.T, s *server) {
			if err := os.Link(s.approvalKey, filepath.Join(filepath.Dir(s.approvalKey), "other.key")); err != nil {
				t.Fatal(err)
			}
		}, "하드 링크가 더 있습니다", "more hard links"},
		{"inside", func(t *testing.T, s *server) {
			in := filepath.Join(filepath.Dir(s.planOut), "approval.key")
			writeKey(t, in)
			s.approvalKey = in
		}, "작업 폴더 밖으로 옮기십시오", "move it out"},
		{"inside through a link", func(t *testing.T, s *server) {
			via := filepath.Join(t.TempDir(), "ws")
			if err := os.Symlink(filepath.Dir(s.planOut), via); err != nil {
				t.Fatal(err)
			}
			writeKey(t, filepath.Join(filepath.Dir(s.planOut), "approval.key"))
			s.approvalKey = filepath.Join(via, "approval.key")
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
				if !strings.Contains(page, want) || strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) {
					t.Errorf("%s: the screen does not say %q or still offers signing", lang, want)
				}
			}
			w := postForm(t, s, ui.SignConfirmPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove)))
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
		if w := postForm(t, s, ui.SignConfirmPath, withRev(form, s.revFor(ui.ScreenApprove))); w.Code != http.StatusConflict {
			t.Errorf("form %v = %d, want 409", form, w.Code)
		}
	}
}

// IC-U125 — **확인한 것과 다른 것에는 서명하지 않는다.** 확인 토큰은 계획 파일 해시 · 키 지문 · 승인자 · 만료 시각을
// 묶는다. 확인 뒤에 승인자를 바꿔 보내거나 승인 키 파일이 다른 키로 바뀌었으면 409(오래된 화면)이고, 확인 없이
// 서명 주소로 바로 보내도 403이다. 어느 경우에도 서명한 계획 파일이 생기지 않는다.
func TestSignatureMatchesWhatWasConfirmed(t *testing.T) {
	s, _ := keyReady(t)
	confirm := func() url.Values {
		w := postForm(t, s, ui.SignConfirmPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove)))
		if w.Code != http.StatusOK {
			t.Fatalf("confirm = %d", w.Code)
		}
		return hiddenFields(w.Body.String())
	}
	f := confirm()
	f.Set("approver", "다른사람")
	if w := postForm(t, s, ui.SignPath, f); w.Code != http.StatusConflict {
		t.Errorf("approver changed after confirmation = %d, want 409", w.Code)
	}
	f = confirm()
	writeKey(t, s.approvalKey)
	if w := postForm(t, s, ui.SignPath, f); w.Code != http.StatusConflict {
		t.Errorf("key changed after confirmation = %d, want 409", w.Code)
	}
	if w := postForm(t, s, ui.SignPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove))); w.Code != http.StatusForbidden {
		t.Errorf("signing without confirmation = %d, want 403", w.Code)
	}
	if _, err := os.Stat(s.signedPath()); err == nil {
		t.Error("a signed plan was written")
	}
}

// IC-U126 — **서명한 계획 파일은 덮지 않고, 보관은 지우지 않는다.** 지금 계획과 맞지 않거나 다른 키로 서명한
// plan.signed.json이 있으면 서명은 409(`signed-exists`)이고 파일은 바이트까지 그대로다. 「지난 서명 보관」은 그 파일과
// 플레이북을 지우지 않고 이름에 시각을 붙여 옆에 둔다(내용은 그대로). 그 뒤에야 새로 서명할 수 있다. 맞는 서명은
// 보관하지 않는다(409).
func TestSignedPlansAreNeverOverwritten(t *testing.T) {
	s, _ := keyReady(t)
	other := filepath.Join(t.TempDir(), "other.key")
	writeKey(t, other)
	mine := s.approvalKey
	s.approvalKey = other
	if w := signVia(t, s, "예전 승인자"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign with the other key = %d", w.Code)
	}
	old := readFile(t, s.signedPath())
	s.approvalKey = mine
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) || !strings.Contains(page, `action="`+ui.ArchivePath+`"`) {
		t.Fatalf("with another key's signature the screen should offer archiving, not signing")
	}
	w := postForm(t, s, ui.SignConfirmPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedSignedExists) || readFile(t, s.signedPath()) != old {
		t.Fatalf("signing over another key's file = %d, file kept %v", w.Code, readFile(t, s.signedPath()) == old)
	}
	if w := postForm(t, s, ui.ArchivePath, withRev(url.Values{}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("archive = %d\n%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(s.signedPath()); err == nil {
		t.Fatal("archiving left the signed plan in place")
	}
	kept, _ := filepath.Glob(strings.TrimSuffix(s.signedPath(), ".json") + ".*.json")
	if len(kept) != 1 || readFile(t, kept[0]) != old {
		t.Fatalf("archived files %v", kept)
	}
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign after archiving = %d", w.Code)
	}
	if w := postForm(t, s, ui.ArchivePath, withRev(url.Values{}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusConflict {
		t.Errorf("archiving a valid signature = %d, want 409", w.Code)
	}
}

// IC-U122 — **플레이북은 상류 생성기가 만드는 것과 같고, 계획이 바뀌면 낡은 것으로 보인다.** 서명한 뒤 「플레이북
// 생성」(L3)은 plan.provision.yml과 plan.rollback.yml을 계획 파일 옆에 쓰고, 두 파일은 같은 계획 · 수준으로
// provisioning.Generate…Playbook이 내는 것과 바이트까지 같다. 생성기가 짚은 빈 곳을 06 카드가 센다. 그 뒤 05에서
// 계획이 달라지면 06 카드는 「서명이 지금 계획과 맞지 않습니다」이고 생성은 409(`generate-not-ready`)다.
func TestPlaybooksMatchUpstreamAndGoStale(t *testing.T) {
	s, _ := keyReady(t)
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d", w.Code)
	}
	if w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l9"}}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusBadRequest {
		t.Errorf("level l9 = %d, want 400", w.Code)
	}
	w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l3"}}, s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("generate = %d\n%s", w.Code, w.Body.String())
	}
	if filepath.Base(s.provisionPath()) != "plan.provision.yml" || filepath.Base(s.rollbackPath()) != "plan.rollback.yml" {
		t.Errorf("playbook paths %s %s", s.provisionPath(), s.rollbackPath())
	}
	signed, _ := loadPlanFile(s.signedPath())
	l3 := provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L3_FULL_AUTO
	if readFile(t, s.provisionPath()) != provisioning.GenerateProvisioningPlaybook(signed, l3) {
		t.Error("the apply playbook differs from the upstream generator")
	}
	if readFile(t, s.rollbackPath()) != provisioning.GenerateRollbackPlaybook(signed, l3) {
		t.Error("the rollback playbook differs from the upstream generator")
	}
	warnings := genWarnings(signed, l3)
	if len(warnings) == 0 {
		t.Fatal("the sample plan should leave gaps at L3")
	}
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if got := cards(t, page)["06"]; got != ui.StepApproveIncompleteText(ui.KO, len(warnings)) {
		t.Errorf("card 06 = %q", got)
	}
	if !strings.Contains(page, "plan.rollback.yml") || !strings.Contains(page, "L3") {
		t.Errorf("the screen does not show the generated files and level")
	}

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
