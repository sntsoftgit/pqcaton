// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// 같은 자산 키 아래 설치가 둘 이상이면 한 판정 · 한 조치가 그 전부에 간다. 이 판은 설치마다 다른 조치를 표현하지 못한다.

func twoInstalls() review.Session {
	sf := judgedSession()
	dg := strings.Repeat("a", 64)
	sf.Items[0].Sources = []review.EvidenceSource{
		{FindingID: "f-sys", Path: "/usr/lib/libssl.so.3", Fingerprint: "fp-sys", SourceNodeID: "web", SnapshotDigest: dg, SnapshotRuleset: "r"},
		{FindingID: "f-app", Path: "/opt/app/libssl.so.3", Fingerprint: "fp-app", SourceNodeID: "web", SnapshotDigest: dg, SnapshotRuleset: "r"},
	}
	sf.Items[0].FindingID, sf.Items[0].Fingerprint = "f-sys", "fp-sys"
	return sf
}

func planReason(t *testing.T, err error) string {
	t.Helper()
	for _, m := range decision.MissingOf(err) {
		if m.Code == decision.MissingPlanField {
			return m.Reason
		}
	}
	var pe interface{ Error() string }
	if errors.As(err, &pe) {
		return pe.Error()
	}
	return ""
}

// IC-P21 — **설치가 둘 이상인 항목은 「전부에 적용」을 지금 목록으로 확인해야 확정된다.** 확인이 없거나, 확인한 뒤
// 설치 목록이 바뀌었거나, 「설치마다 다름」을 골랐으면 계획을 내지 않는다. 확인하면 두 설치가 모두 계획의 근거로 간다.
func TestEveryInstallationMustBeConfirmed(t *testing.T) {
	sf := twoInstalls()
	if _, err := review.Finalize(sf); err == nil || !strings.Contains(planReason(t, err), decision.PlanInstallsUnconfirmed) &&
		!strings.Contains(err.Error(), "confirm") {
		t.Fatalf("an unconfirmed multi-installation item was finalized: %v", err)
	}

	sf.Items[0].InstallationsConfirmed = review.Installations(sf.Items[0])
	res, err := review.Finalize(sf)
	if err != nil {
		t.Fatalf("a confirmed item was refused: %v", err)
	}
	a := res.Plan.GetActions()
	if len(a) != 1 || len(a[0].GetEvidenceSources()) != 2 {
		t.Fatalf("both installations must go to the plan as evidence: %+v", a)
	}
	ids := a[0].GetEvidenceSources()[0].GetFindingId() + " " + a[0].GetEvidenceSources()[1].GetFindingId()
	if !strings.Contains(ids, "f-sys") || !strings.Contains(ids, "f-app") {
		t.Errorf("an installation was dropped from the evidence: %s", ids)
	}

	moved := twoInstalls()
	moved.Items[0].InstallationsConfirmed = review.Installations(sf.Items[0])
	moved.Items[0].Sources[1].Path = "/opt/other/libssl.so.3"
	if _, err := review.Finalize(moved); err == nil {
		t.Error("a confirmation made for another list of installations still counted")
	}

	each := twoInstalls()
	each.Items[0].PerInstallation = true
	if _, err := review.Finalize(each); err == nil {
		t.Error("an item asking for different actions per installation was put in the plan")
	}

	one := judgedSession()
	if _, err := review.Finalize(one); err != nil {
		t.Errorf("a single-installation item now needs a confirmation: %v", err)
	}
}

// IC-P22 — **경로와 확인은 세션 파일을 지나고 다시 열어도 남는다.** 경로는 근거에, 확인은 목록째 옮겨진다.
func TestPathsAndConfirmationSurviveTheSessionFile(t *testing.T) {
	sf := twoInstalls()
	sf.Items[0].InstallationsConfirmed = review.Installations(sf.Items[0])
	b, err := json.Marshal(sf)
	if err != nil {
		t.Fatal(err)
	}
	var back review.Session
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Items[0].Sources[1].Path != "/opt/app/libssl.so.3" || !review.InstallationsConfirmedNow(back.Items[0]) {
		t.Errorf("the path or the confirmation was lost in the file: %+v", back.Items[0])
	}
	next := twoInstalls()
	got := review.Carry(back, next)
	if !review.InstallationsConfirmedNow(got.Items[0]) {
		t.Error("Carry dropped the confirmation")
	}
}

// IC-D28 — **설치 경로가 바뀌면 근거 해시가 바뀐다.** 경로를 모르는 근거의 해시는 경로 칸이 생기기 전과 같다.
func TestPathIsPartOfTheBasis(t *testing.T) {
	sf := twoInstalls()
	it := sf.Items[0]
	before := review.BasisOf(it, sf.RulesetVersion)
	it.Sources = append([]review.EvidenceSource(nil), it.Sources...)
	it.Sources[1].Path = "/opt/other/libssl.so.3"
	if review.BasisOf(it, sf.RulesetVersion) == before {
		t.Error("another installation path left the basis unchanged")
	}
	noPath := it
	noPath.Sources = []review.EvidenceSource{{FindingID: "f", Fingerprint: "fp", SourceNodeID: "web"}}
	withEmpty := noPath
	withEmpty.Sources = []review.EvidenceSource{{FindingID: "f", Fingerprint: "fp", SourceNodeID: "web", Path: ""}}
	if review.BasisOf(noPath, sf.RulesetVersion) != review.BasisOf(withEmpty, sf.RulesetVersion) {
		t.Error("an unknown path moved the basis")
	}
}
