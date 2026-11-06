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

	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/sign"
	"github.com/randyinthedev-hash/pqcota-provisioning/pkg/provisioning"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

const (
	policyA = "action,runtime,lib,app_key,note\nexclude,*,*,/usr/sbin/sshd*,managed by OS patching\n"
	policyB = "action,runtime,lib,app_key,note\nexclude,*,*,/usr/sbin/sshd*,managed by OS patching\nexclude,*,*,/usr/bin/python*,package runtime\n"
)

// setPolicy: 활성 정책(scope-assets.csv)을 쓴다. 06이 견주는 것은 이 파일이다.
func setPolicy(t *testing.T, s *server, body string) {
	t.Helper()
	if s.scopeAssets == "" {
		s.scopeAssets = filepath.Join(filepath.Dir(s.planOut), "scope-assets.csv")
	}
	if err := os.WriteFile(s.scopeAssets, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeRawPlan: 확정을 거치지 않고 계획 파일만 쓴다(다른 곳에서 옮겨 온 계획). 계획 ID를 id로 바꾼다.
func writeRawPlan(t *testing.T, s *server, id string) {
	t.Helper()
	b := strings.Replace(planJSON, `"id":"plan-1"`, `"id":"`+id+`"`, 1)
	if err := os.WriteFile(s.planOut, []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
}

func signConfirm(t *testing.T, s *server) int {
	t.Helper()
	w := postForm(t, s, ui.SignConfirmPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove)))
	if w.Code == http.StatusConflict && !strings.Contains(w.Body.String(), ui.RefusedPlanScope) {
		t.Errorf("sign confirm refused for another reason:\n%s", w.Body.String())
	}
	return w.Code
}

// IC-P34~P39 — **06은 이 작업 폴더에서 확정한 계획만, 그 계획을 만든 정책이 지금 활성 정책일 때만 서명한다.** 검사
// 순서는 계획 ID 형식 → 동일성 → 계획 파일 해시 → 정책 지문이다. 어긋나면 화면에 서명 양식이 없고 그 계획의 문제를
// 적으며, 서명 확인은 409 `plan-scope`이고 작업 폴더의 어떤 파일도 바뀌지 않는다.
//
//   - IC-P34: 계획을 만든 정책과 지금 활성 정책이 다르다.
//   - IC-P35: 형식 1 영수증(이 판보다 먼저 확정). 활성 정책을 비워 두어 **없는 지문을 빈 정책의 지문으로 읽으면 통과하는**
//     자리에서 잰다.
//   - IC-P36: 계획 ID가 05가 쓰는 모양이 아니다(옛 모양 · 세션이 빠짐 · 조직에 쓸 수 없는 글자).
//   - IC-P37: 계획 ID의 조직이 이 작업 폴더의 조직이 아니다.
//   - IC-P38: 그 세션의 영수증이 없다(다른 작업 폴더에서 옮겨 온 계획).
//   - IC-P39: 확정한 뒤 계획 파일이 바뀌었다.
func TestPlanMustLeadBackToThisWorkspacesFinalize(t *testing.T) {
	for _, c := range []struct {
		name, want string
		prep       func(t *testing.T, s *server)
	}{
		{"IC-P34 another policy", ui.PlanScopeMismatch, func(t *testing.T, s *server) {
			setPolicy(t, s, policyA)
			bindPlan(t, s, planJSON)
			setPolicy(t, s, policyB)
		}},
		{"IC-P35 format 1 receipt", ui.PlanScopeUnverifiable, func(t *testing.T, s *server) {
			s.scopeAssets = ""
			bindPlanUnder(t, s, planJSON, "")
		}},
		{"IC-P36 old plan id", ui.PlanScopeBadID, func(t *testing.T, s *server) { writeRawPlan(t, s, "plan-1") }},
		{"IC-P36 no session", ui.PlanScopeBadID, func(t *testing.T, s *server) { writeRawPlan(t, s, planIDPrefix+s.orgName()) }},
		{"IC-P36 unusable organization", ui.PlanScopeBadID, func(t *testing.T, s *server) { writeRawPlan(t, s, planIDPrefix+"ac/me:plan-1") }},
		{"IC-P37 another organization", ui.PlanScopeIdentity, func(t *testing.T, s *server) {
			writeRawPlan(t, s, planIDPrefix+"someone-else:plan-1")
		}},
		{"IC-P38 no receipt", ui.PlanScopeNoReceipt, func(t *testing.T, s *server) {
			writeRawPlan(t, s, planIDPrefix+s.orgName()+":never-finalized")
		}},
		{"IC-P39 plan changed after finalize", ui.PlanScopePlanHash, func(t *testing.T, s *server) {
			bindPlan(t, s, planJSON)
			f, err := os.OpenFile(s.planOut, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			f.WriteString("\n")
			f.Close()
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _ := keyReady(t)
			c.prep(t, s)
			v, err := s.approveModel(ui.Page{Lang: ui.KO})
			if err != nil {
				t.Fatal(err)
			}
			if v.ScopeProblem != c.want {
				t.Fatalf("ScopeProblem = %q, want %q", v.ScopeProblem, c.want)
			}
			page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
			if strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) || !strings.Contains(page, v.ScopeText()) {
				t.Errorf("the screen offers to sign, or does not say why not")
			}
			before := snapshotDir(t, filepath.Dir(s.planOut))
			if code := signConfirm(t, s); code != http.StatusConflict {
				t.Fatalf("sign confirm = %d, want 409 %s", code, ui.RefusedPlanScope)
			}
			sameDir(t, before, snapshotDir(t, filepath.Dir(s.planOut)))
		})
	}
}

// IC-U149 — **활성화 전에 확정한 계획으로는 서명 확인 · 서명 · 생성이 모두 409 `plan-scope`다.** 서명은 확인 페이지를
// 연 뒤에 계획이 이 작업 폴더의 확정 기록에서 떨어져 나가도 **잠금 안의 재검사**가 막는다. 생성은 잠금 안에서만 잰다.
// 이미 서명된 계획과 플레이북은 바이트 그대로이고 보관은 할 수 있다.
func TestPlansFromBeforeActivationAreNotSignedOrGenerated(t *testing.T) {
	s, _ := keyReady(t)
	setPolicy(t, s, policyA)
	id := bindPlan(t, s, planJSON)

	// 서명: 확인 페이지까지 연 뒤 그 계획의 영수증이 사라진다. rev는 영수증을 덮지 않으므로 잠금 안의 재검사만 남는다.
	w := postForm(t, s, ui.SignConfirmPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusOK {
		t.Fatalf("sign confirm under the plan's own policy = %d\n%s", w.Code, w.Body.String())
	}
	confirmed := hiddenFields(w.Body.String())
	pid, _ := planIdentity(id)
	receipt := filepath.Join(filepath.Dir(s.planOut), store.DirName, "receipts", pid.Kind, pid.Org, pid.SessionID+".json")
	kept := readFile(t, receipt)
	if err := os.Remove(receipt); err != nil {
		t.Fatal(err)
	}
	if w := postForm(t, s, ui.SignPath, confirmed); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedPlanScope) {
		t.Fatalf("sign after the receipt went away = %d, want 409 %s\n%s", w.Code, ui.RefusedPlanScope, w.Body.String())
	}
	if _, err := os.Stat(s.signedPath()); err == nil {
		t.Fatal("a plan that no longer leads back to a finalize was signed")
	}
	if err := os.WriteFile(receipt, []byte(kept), 0o600); err != nil {
		t.Fatal(err)
	}

	// 정책 A에서 서명하고 생성한 뒤 정책이 B로 바뀐다(활성화).
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d", w.Code)
	}
	if w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("generate under the plan's own policy = %d\n%s", w.Code, w.Body.String())
	}
	setPolicy(t, s, policyB)
	files := map[string]string{}
	for _, p := range []string{s.planOut, s.signedPath(), s.provisionPath(), s.rollbackPath()} {
		files[p] = readFile(t, p)
	}
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if strings.Contains(page, `action="`+ui.GeneratePath+`"`) || !strings.Contains(page, `action="`+ui.ArchivePath+`"`) ||
		!strings.Contains(page, "지금 활성 정책(scope-assets.csv)과 다른 자산 스코프 정책") {
		t.Errorf("after activation: generation offered, archiving not offered, or the reason not shown")
	}
	w = postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l3"}}, s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedPlanScope) {
		t.Fatalf("generate after activation = %d, want 409 %s", w.Code, ui.RefusedPlanScope)
	}
	for p, c := range files {
		if readFile(t, p) != c {
			t.Errorf("%s changed after a refused generate", filepath.Base(p))
		}
	}
	if w := postForm(t, s, ui.ArchivePath, withRev(url.Values{}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Errorf("archiving the signed plan made under the old policy = %d", w.Code)
	}
	// 보관은 지우지 않고 옮긴다.
	for _, p := range []string{s.signedPath(), s.provisionPath(), s.rollbackPath()} {
		moved, _ := filepath.Glob(strings.TrimSuffix(p, filepath.Ext(p)) + ".*" + filepath.Ext(p))
		if len(moved) != 1 || readFile(t, moved[0]) != files[p] {
			t.Errorf("%s was not kept byte for byte by archiving: %v", filepath.Base(p), moved)
		}
	}
}

// IC-U151 — **이 판보다 먼저 확정한 계획(형식 1 영수증)은 열람과 보관만 한다.** 서명 전이면 서명 양식이 없고 서명
// 확인이 409다. 서명과 플레이북까지 만든 뒤라면 생성 양식이 없고 생성이 409이며, 플레이북은 바이트 그대로다. 화면은
// 현재 정책에서 05의 새 판정 회차를 열어 다시 확정하라고 안내한다.
func TestPlansFinalizedBeforeThisVersionAreViewedOnly(t *testing.T) {
	s, _ := keyReady(t)
	bindPlanUnder(t, s, planJSON, "")
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) || !strings.Contains(page, "현재 정책에서 05의 새 판정 회차를 열어 다시 확정하십시오") {
		t.Errorf("an old plan before signing: sign form offered, or no guidance")
	}
	if code := signConfirm(t, s); code != http.StatusConflict {
		t.Fatalf("sign confirm = %d, want 409", code)
	}

	// 이미 서명하고 플레이북까지 만든 옛 계획.
	plan, err := loadPlanFile(s.planOut)
	if err != nil {
		t.Fatal(err)
	}
	if err := provisioning.PrepareApproval(plan, s.now()); err != nil {
		t.Fatal(err)
	}
	_, priv := s.readKey()
	sig, err := sign.SignApproval(priv, "보안팀장", plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.ApprovalSignatures = append(plan.ApprovalSignatures, sig)
	b, _ := protojson.Marshal(plan)
	old := map[string]string{s.signedPath(): string(b), s.provisionPath(): "old apply\n", s.rollbackPath(): "old undo\n"}
	for p, c := range old {
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	page = get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if strings.Contains(page, `action="`+ui.GeneratePath+`"`) || !strings.Contains(page, `action="`+ui.ArchivePath+`"`) {
		t.Errorf("a signed old plan: generation offered or archiving not offered")
	}
	w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedPlanScope) {
		t.Fatalf("generate = %d, want 409 %s", w.Code, ui.RefusedPlanScope)
	}
	for p, c := range old {
		if readFile(t, p) != c {
			t.Errorf("%s was changed by a refused generate", filepath.Base(p))
		}
	}
}
