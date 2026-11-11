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
	"time"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// 상류 v0.11.0 의 조치 필드(소스 근거 · 가이드)가 든 계획(planfields.go).

const fieldTarget = `"targetAlgorithm":"ML-KEM (FIPS 203)"`

// withSourceEvidence · withGuide: 계획의 첫 조치에 새 필드를 넣는다.
func withSourceEvidence(plan string) string {
	return strings.Replace(plan, fieldTarget, fieldTarget+`,"sourceEvidence":[{"interpretationId":"conv-1","itemId":"item-1","inputSha256":"`+
		strings.Repeat("a", 64)+`"}]`, 1)
}

func withGuide(plan string) string {
	return strings.Replace(plan, fieldTarget, fieldTarget+`,"guide":{"guideId":"pqcota-guide/jca-doc-key-transport-mlkem","revision":"1","bodySha256":"`+
		strings.Repeat("b", 64)+`","purpose":"key-transport"}`, 1)
}

// IC-U193 — **새 필드가 없는 계획은 v0.11.0 모듈에서도 그대로 서명 · 생성한다.**
func TestPlanWithoutNewFieldsStillApproves(t *testing.T) {
	s, _ := keyReady(t)
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d", w.Code)
	}
	if w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("generate = %d", w.Code)
	}
	if _, err := os.Stat(s.provisionPath()); err != nil {
		t.Fatal("no playbook was generated")
	}
}

// IC-U194 — **소스 근거나 가이드가 든 계획은 06 의 네 자리에서 모두 `plan-fields` 이고, 승인 서명이 붙지 않는다.** 화면은 서명 ·
// 생성 양식을 내지 않고 「계획이 손상된 것은 아니며 pqcaton 이 아직 그 내용을 검토 · 생성하지 않는다」는 한계를 보인다. 서명
// 확인은 409, 서명 처리기는 확인 양식을 갖춰 보내도 잠금 안에서 409, 생성도 409 다. 작업 폴더는 바이트 그대로다. 이 작업
// 폴더에서 확정한 계획이라(영수증과 해시가 맞음) 정책 쪽 거부가 아니라 이 판정이 막는다.
func TestPlanWithNewFieldsIsRefusedInApproval(t *testing.T) {
	for name, plan := range map[string]string{"source evidence": withSourceEvidence(planJSON), "guide": withGuide(planJSON)} {
		t.Run(name, func(t *testing.T) {
			s, _ := keyReady(t)
			id := bindPlan(t, s, plan)
			dir := filepath.Dir(s.planOut)
			before := snapshotDir(t, dir)
			v, err := s.approveModel(ui.Page{Lang: ui.KO})
			if err != nil {
				t.Fatal(err)
			}
			if !v.FieldsProblem || v.FieldsSigned || v.ScopeProblem != "" || v.CanSign() || v.CanGenerate() {
				t.Fatalf("view: fields %v (signed %v), scope %q, can sign %v, can generate %v",
					v.FieldsProblem, v.FieldsSigned, v.ScopeProblem, v.CanSign(), v.CanGenerate())
			}
			for lang, want := range map[string][]string{
				"ko": {"계획이 손상된 것은 아닙니다", "pqcaton은 아직 그 내용을 06에서 검토하거나 플레이북에 싣지 않으므로"},
				"en": {"The plan is not damaged", "pqcaton does not yet review that content"},
			} {
				page := get(t, s, ui.ScreenApprove+"?lang="+lang).Body.String()
				if strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) || strings.Contains(page, `action="`+ui.GeneratePath+`"`) {
					t.Errorf("%s: the screen offers to sign or generate", lang)
				}
				for _, w := range want {
					if !strings.Contains(page, w) {
						t.Errorf("%s: the screen lacks %q", lang, w)
					}
				}
			}
			refused := func(what string, w *httptest.ResponseRecorder) {
				t.Helper()
				if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedPlanFields) {
					t.Errorf("%s = %d, want 409 %s", what, w.Code, ui.RefusedPlanFields)
				}
			}
			refused("sign confirm", postForm(t, s, ui.SignConfirmPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove))))
			refused("sign", postForm(t, s, ui.SignPath, signConfirmFormFor(t, s, id)))
			refused("generate", postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove))))
			if _, err := os.Stat(s.signedPath()); err == nil {
				t.Error("an approval signature was written")
			}
			sameDir(t, before, snapshotDir(t, dir))
		})
	}
}

// IC-U195 — **서명한 계획에 소스 근거 · 가이드가 있으면 생성하지 않고 보관만 허용한다.** 지금 계획은 깨끗해도 같다.
func TestSignedPlanWithNewFieldsIsArchivedNotGenerated(t *testing.T) {
	s, _ := keyReady(t)
	signed := &provisioningv1.FinalizedPlan{}
	if err := protojson.Unmarshal([]byte(withGuide(planJSON)), signed); err != nil {
		t.Fatal(err)
	}
	signed.Status, signed.FinalizedAt = provisioningv1.PlanStatus_PLAN_STATUS_FINALIZED, timestamppb.New(time.Unix(1800000000, 0))
	signed.ApprovalSignatures = []string{"보안팀장:c2lnbmF0dXJl"}
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
	if !v.FieldsProblem || !v.FieldsSigned || v.CanGenerate() || !v.CanArchive() {
		t.Fatalf("view: fields %v (signed %v), can generate %v, can archive %v", v.FieldsProblem, v.FieldsSigned, v.CanGenerate(), v.CanArchive())
	}
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if strings.Contains(page, `action="`+ui.GeneratePath+`"`) || !strings.Contains(page, "서명한 계획(plan.signed.json)에 소스 근거나 승인 가이드가") ||
		!strings.Contains(page, `action="`+ui.ArchivePath+`"`) {
		t.Error("the screen offers to generate, does not point at the signed plan, or does not offer to archive")
	}
	w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedPlanFields) {
		t.Errorf("generate = %d, want 409 %s", w.Code, ui.RefusedPlanFields)
	}
	sameDir(t, before, snapshotDir(t, dir))
}
