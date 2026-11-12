// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// v2Session: 옛 규칙 판으로 판정을 마친 세션. 항목 둘(하나는 새 규칙에서 사라진다), 정책 결론 하나.
func v2Session() review.Session {
	sf := judgedSession()
	sf.RulesetVersion = oldUpstream()
	gone := sf.Items[0]
	gone.ID, gone.Conclusion = "n1/openssl/libgone", "제거한다"
	sf.Items = append(sf.Items, gone)
	sf.Items[0].Conclusion = "교체한다"
	sf.Items[0].Sources = []review.EvidenceSource{{FindingID: "f-v2", Fingerprint: "fp-v2", SourceNodeID: "n1"}}
	return sf
}

// v3Rebuilt: 지금 규칙으로 결과에서 다시 세운 세션. 같은 자산이 설치 둘로 갈렸고, 새 항목이 하나 생겼다.
func v3Rebuilt() review.Session {
	sf := judgedSession()
	sf.Signature, sf.Reviewer = "", ""
	sf.PolicyDecisions = map[string]string{"openssl/libssl": ""}
	it := &sf.Items[0]
	it.Conclusion, it.Plan, it.Kind, it.TargetAlgorithm = "", false, "", ""
	it.Sources = []review.EvidenceSource{
		{FindingID: "f-sys", Path: "/usr/lib/libssl.so.3", Fingerprint: "fp-sys", SourceNodeID: "n1"},
		{FindingID: "f-app", Path: "/opt/app/libssl.so.3", Fingerprint: "fp-app", SourceNodeID: "n1"},
	}
	fresh := sf.Items[0]
	fresh.ID, fresh.Sources = "n1/openssl/libnew", []review.EvidenceSource{{FindingID: "f-new", Path: "/usr/lib/libnew.so", SourceNodeID: "n1"}}
	sf.Items = append(sf.Items, fresh)
	return sf
}

// IC-P24 — **규칙 전환 회차는 옛 값을 참고값으로만 옮기고 현재 칸은 비운다.** 새 세션은 새 ID이고 서명 · 검토자가 빈다.
// 옛 결론 · 계획 칸은 항목의 previous에, 정책 결론은 previous_policy_decisions에만 있다. 그래서 서명과 검토자만 넣어서는
// 확정되지 않는다.
func TestRulesetRoundCarriesReferenceValuesOnly(t *testing.T) {
	old := v2Session()
	next, _ := review.RulesetRound(old, v3Rebuilt(), "s-new", "ruleset-change-x.json", "session.ruleset-x.json")
	if next.SessionID != "s-new" || next.Signature != "" || next.Reviewer != "" || next.RulesetVersion != review.RulesetVersion {
		t.Fatalf("the new round: id %q signature %q ruleset %q", next.SessionID, next.Signature, next.RulesetVersion)
	}
	it := next.Items[0]
	if it.Conclusion != "" || it.Plan || it.Kind != "" || it.TargetAlgorithm != "" {
		t.Errorf("a current value was carried: %+v", it)
	}
	if it.Previous == nil || it.Previous.Conclusion != "교체한다" || !it.Previous.Plan || it.Previous.SessionID != old.SessionID || it.Previous.RulesetVersion != old.RulesetVersion {
		t.Errorf("the reference values are wrong: %+v", it.Previous)
	}
	if next.PolicyDecisions["openssl/libssl"] != "" || next.PreviousPolicyDecisions["openssl/libssl"] != "교체" {
		t.Errorf("policy: current %q, previous %q", next.PolicyDecisions["openssl/libssl"], next.PreviousPolicyDecisions["openssl/libssl"])
	}
	if next.Items[1].Previous != nil {
		t.Error("a new item got reference values from nowhere")
	}
	next.Signature, next.Reviewer = "kty", "보안팀"
	if _, err := review.Finalize(next); err == nil || !errors.Is(err, decision.ErrMandatoryPending) {
		t.Errorf("only a signature finalized the new round: %v", err)
	}
}

// IC-P25 — **다시 채택은 그 항목 하나만 현재 값으로 옮기고, 참고값은 다시 세워도 남는다. 보고서는 같은 입력에서 같은
// 바이트다.** 보고서는 사라진 항목 · 새 항목 · 설치가 둘로 갈린 항목을 적고, 규칙이 바뀌어 달라졌다는 문장을 맨 위에 둔다.
// 다설치 확인은 채택해도 옮기지 않는다(지금 목록으로 다시 확인한다).
func TestAdoptAndReport(t *testing.T) {
	old := v2Session()
	old.Items[0].InstallationsConfirmed = []string{"/old"}
	next, rep := review.RulesetRound(old, v3Rebuilt(), "s-new", "r.json", "a.json")
	if !review.Adopt(&next, next.Items[0].ID) || next.Items[0].Conclusion != "교체한다" || !next.Items[0].Plan {
		t.Fatalf("adopting did not move the values: %+v", next.Items[0])
	}
	if len(next.Items[0].InstallationsConfirmed) != 0 {
		t.Error("an installation confirmation made for another list was adopted")
	}
	if review.Adopt(&next, next.Items[1].ID) || next.Items[1].Conclusion != "" {
		t.Error("an item without reference values was changed")
	}
	if !review.AdoptPolicy(&next, "openssl/libssl") || next.PolicyDecisions["openssl/libssl"] != "교체" {
		t.Error("adopting the policy conclusion failed")
	}
	carried := review.Carry(next, v3Rebuilt())
	if carried.Items[0].Previous == nil || carried.PreviousPolicyDecisions["openssl/libssl"] != "교체" || carried.RulesetChange == nil {
		t.Error("rebuilding the session dropped the reference values or the ruleset change record")
	}

	b1, err := review.EncodeReport(rep)
	if err != nil {
		t.Fatal(err)
	}
	// 같은 옛 세션 · 같은 결과로 다시(새로 세운 세션의 ID는 회차가 newID로 덮는다).
	_, rep2 := review.RulesetRound(old, v3Rebuilt(), "s-new", "r.json", "a.json")
	b2, _ := review.EncodeReport(rep2)
	if !bytes.Equal(b1, b2) {
		t.Error("the same inputs gave different report bytes")
	}
	back, err := review.DecodeReport(b1)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]review.ItemChange{}
	for _, c := range back.Items {
		got[c.ID] = c
	}
	if got["n1/openssl/libgone"].Change != review.ChangeOnlyOld || got["n1/openssl/libnew"].Change != review.ChangeOnlyNew {
		t.Errorf("removed or added items are not marked: %+v", back.Items)
	}
	if c := got["n1/openssl/libssl"]; c.Change != review.ChangeChanged || len(c.NewInstallations) != 2 || c.PreviousConclusion != "교체한다" {
		t.Errorf("the split installation: %+v", c)
	}
	if back.Note == "" || back.FromRuleset != old.RulesetVersion || back.ToRuleset != review.RulesetVersion {
		t.Errorf("the report head: %+v", back)
	}
}

// IC-P47 — **정책 회차의 기록은 다시 세워도 남는다.** 화면은 세션을 읽을 때마다 입력에서 다시 세우고 옛 세션의 칸을 옮긴다.
// 정책 회차 기록(근거 포함)이 빠지면 05가 그 회차를 운영자 진술로 열었는지 보이지 못하고, 화면이 세션을 다시 쓰면 기록이
// 사라진다.
func TestCarryKeepsTheScopeChange(t *testing.T) {
	const from = "scope-policy-v1:" + "1111111111111111111111111111111111111111111111111111111111111111"
	const to = "scope-policy-v1:" + "2222222222222222222222222222222222222222222222222222222222222222"
	old := v3Rebuilt()
	old.SessionID, old.ScopePolicyDigestV1 = "s-old", from
	rebuilt := v3Rebuilt()
	rebuilt.ScopePolicyDigestV1 = to
	next, _ := review.PolicyRound(old, rebuilt, "s-new", "r.json", "a.json", review.ScopeRoundActivation, review.ScopeBasisUpstream)
	if next.ScopeChange == nil || next.ScopeChange.Basis != review.ScopeBasisUpstream {
		t.Fatalf("the policy round did not record its basis: %+v", next.ScopeChange)
	}
	again := v3Rebuilt()
	again.ScopePolicyDigestV1 = to
	carried := review.Carry(next, again)
	if carried.SessionID != "s-new" || carried.ScopeChange == nil || *carried.ScopeChange != *next.ScopeChange {
		t.Errorf("rebuilding the session dropped the scope change record: %+v", carried.ScopeChange)
	}
}

// IC-P50 — **보고서 형식 판 1은 모르는 필드를 무시하는 계약이다.** 형식 판 1의 보고서에 이 판이 모르는 필드(앞으로 더할 선택
// 필드)가 있어도 DecodeReport가 읽고 아는 필드는 그대로다. upstream_check가 없는 보고서(v0.27.x 이하가 쓴 것)도 읽고 그
// 필드는 nil이다. upstream_check가 있는 보고서는 다시 써도 그 필드가 남는다. 형식 판이 다르면 거절한다.
func TestReportIgnoresUnknownFields(t *testing.T) {
	base := `{"format":1,"_how_to_read":"n","from_ruleset":"a","to_ruleset":"b","from_session_id":"s1","to_session_id":"s2",` +
		`"reason":"scope-activation","items":[{"id":"n1/openssl/libssl","change":"same"}]`
	for name, c := range map[string]struct {
		body     string
		upstream bool
	}{
		"older (no upstream_check)": {base + `}`, false},
		"this version":              {base + `,"upstream_check":{"checked_at":"2026-11-12T00:00:00Z","org":"acme","snapshots":[{"node":"n1","ruleset":"r","digest":"d"}]}}`, true},
		"a later optional field":    {base + `,"upstream_check":{"checked_at":"2026-11-12T00:00:00Z","org":"acme","snapshots":[]},"future_field":{"x":1}}`, true},
	} {
		t.Run(name, func(t *testing.T) {
			r, err := review.DecodeReport([]byte(c.body))
			if err != nil || r.Format != 1 || r.Reason != "scope-activation" || len(r.Items) != 1 || r.Items[0].ID != "n1/openssl/libssl" {
				t.Fatalf("report %+v: %v", r, err)
			}
			if (r.UpstreamCheck != nil) != c.upstream || (c.upstream && r.UpstreamCheck.Org != "acme") {
				t.Errorf("upstream check %+v, want present %v", r.UpstreamCheck, c.upstream)
			}
			if c.upstream {
				b, err := review.EncodeReport(r)
				if err != nil || !strings.Contains(string(b), `"upstream_check"`) {
					t.Errorf("re-encoded report lost upstream_check: %v", err)
				}
			}
		})
	}
	if _, err := review.DecodeReport([]byte(`{"format":2,"items":[]}`)); err == nil {
		t.Error("a report of another format was read")
	}
}
