package review_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// 판정 축과 계획 축을 나눈다. 자동통과는 「사람이 볼 필요가 없다」이지 「바꿀 필요가 없다」가
// 아니다. 관리 축은 대조 축과 다른 것을 재고, 세션 파일에 들어가 해시가 덮고 계획 차단이 본다.

const (
	v2 = "pqcota-enrich/v2+pqcaton-plan/v2"
	v3 = "pqcota-enrich/v2+pqcaton-plan/v3"
)

func autopassItem(id string) review.Item {
	return review.Item{ID: id, Node: "n1", Runtime: "openssl", Policy: "openssl/libcrypto",
		State: "CONFIRMED", Conf: 0.9, ConfEvaluated: true, Managed: string(reconcile.Managed),
		FindingID: "f-" + id, Fingerprint: "fp-" + id,
		Sources: []review.EvidenceSource{{FindingID: "f-" + id, Fingerprint: "fp-" + id, SourceNodeID: "n1", SnapshotDigest: strings.Repeat("a", 64), SnapshotRuleset: "pqcota-enrich/v2"}}}
}

// ★ IC-P11 — 자동통과 항목만 고른 세션에서도 실제 계획이 생긴다.
//
// 전에는 자동통과가 식별자 문자열 목록이라 계획 칸을 들 자리가 없었고, Finalize 는 Items 만 봐서
// 켜도 조용히 빠졌다. 판정은 요구하지 않는다 - 결론이 비어도 Pending 에 오르지 않는다.
func TestPlanFromAutopassOnly(t *testing.T) {
	sf := judgedSession()
	sf.Items[0].Plan = false // 리뷰 항목은 판정만 하고 계획에는 넣지 않는다
	a := autopassItem("n1/openssl/libcrypto")
	a.Plan, a.Level, a.Kind, a.TargetAlgorithm = true, "L2", "REMEDIATION_KIND_CONFIG_ONLY", "ML-KEM (FIPS 203)"
	sf.Autopass = []review.Item{a}

	if p := review.Pending(sf); len(p) != 0 {
		t.Fatalf("자동통과에 결론을 요구했다: %+v", p)
	}
	res, err := review.Finalize(sf)
	if err != nil {
		t.Fatalf("확정되지 않았다: %v", err)
	}
	acts := res.Plan.GetActions()
	if len(acts) != 1 || acts[0].GetTargetNodeId() != "n1" || acts[0].GetFindingId() != "f-n1/openssl/libcrypto" {
		t.Fatalf("자동통과 항목이 계획에 없다: %+v", acts)
	}
	if len(acts[0].GetEvidenceSources()) != 1 {
		t.Errorf("근거가 계약으로 나가지 않았다: %+v", acts[0])
	}
}

// ★ IC-P12 — 제외 전용 항목은 계획에 넣을 수 없다. 세션 파일을 손으로 고쳐 켜도 확정이 막는다.
//
// 이미 한 번 잡힌 결함이다 - 정책을 무시하던 동안 뺀 자산에 조치 계획을 세우고 있었다. 관리 축이
// 세션 파일에 있어야 이 검사가 가능하다.
func TestExcludedItemCannotBePlannedEvenByHandEditing(t *testing.T) {
	sf := judgedSession()
	x := autopassItem("n1/openssl/libcrypto")
	x.Managed, x.Plan, x.Level, x.Kind, x.TargetAlgorithm = string(reconcile.ExcludedByPolicy), true, "L2", "REMEDIATION_KIND_CONFIG_ONLY", "ML-KEM (FIPS 203)"
	x.Sources, x.FindingID, x.Fingerprint = nil, "", ""
	x.ExcludedSources = []review.ExcludedSource{{FindingID: "f-x", SourceNodeID: "n1", AppKeys: []string{"/usr/sbin/sshd"}}}
	sf.Autopass = []review.Item{x}

	if _, err := review.Finalize(sf); err == nil || !strings.Contains(err.Error(), "excluded by the asset-scope policy") {
		t.Fatalf("제외 전용이 계획으로 나갔다: %v", err)
	}
	var found bool
	for _, m := range review.Pending(sf) {
		if m.Code == decision.MissingPlanField && m.Subject == x.ID {
			found = true
		}
	}
	if !found {
		t.Error("Pending 이 제외 전용의 계획 선택을 알리지 않는다")
	}
}

// ★ IC-P13 — 옛 세션 파일의 자동통과 목록(문자열 배열)은 열리되 계획에 넣을 수 없는 상태로 남는다.
//
// 형을 바로 []Item 으로 바꾸면 json.Unmarshal 이 형 오류로 먼저 실패해 변환 코드에 닿지 못한다.
// wire 구조가 먼저 받는다. 빈 칸의 일반 항목으로 조용히 올리면 사람이 새 후보로 읽는다. 모르는
// 형식은 오류다 - 「모르니 비워 둔다」로 넘기면 검토한 목록이 사라진다.
func TestDecodeKeepsLegacyAutopassAsDisplayOnly(t *testing.T) {
	legacy := []byte(`{"scope":"org://acme","items":[],"autopass_candidates":["n1/openssl/libcrypto","n2/jca/jca-provider-chain"]}`)
	sf, err := review.Decode(legacy)
	if err != nil {
		t.Fatalf("옛 파일이 열리지 않는다: %v", err)
	}
	if len(sf.Autopass) != 0 || len(sf.LegacyAutopass) != 2 {
		t.Fatalf("옛 목록이 표시용으로 보존되지 않았다: 구조화 %d · 옛 %d", len(sf.Autopass), len(sf.LegacyAutopass))
	}

	fresh := []byte(`{"scope":"org://acme","items":[],"autopass_candidates":[{"id":"n1/openssl/libcrypto","node":"n1","runtime":"openssl","policy":"p","state":"CONFIRMED","confidence":0.9}]}`)
	if sf, err = review.Decode(fresh); err != nil || len(sf.Autopass) != 1 || sf.Autopass[0].Node != "n1" {
		t.Fatalf("새 형식이 읽히지 않는다: %v %+v", err, sf.Autopass)
	}

	if _, err := review.Decode([]byte(`{"autopass_candidates":42}`)); err == nil {
		t.Error("모르는 형식을 조용히 넘겼다")
	}

	// Save 는 새 형식만 쓴다. 옛 목록은 별도 이름으로 나간다.
	dir := t.TempDir()
	sf, _ = review.Decode(legacy)
	path := filepath.Join(dir, "s.json")
	if err := review.Save(path, sf); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	if string(m["autopass_candidates"]) != "null" && string(m["autopass_candidates"]) != "[]" {
		t.Errorf("옛 목록이 새 이름으로 다시 나갔다: %s", m["autopass_candidates"])
	}
	if _, ok := m["autopass_candidates_legacy"]; !ok {
		t.Error("옛 목록이 별도 이름으로 보존되지 않았다")
	}
	if sf2, err := review.Load(path); err != nil || len(sf2.LegacyAutopass) != 2 {
		t.Errorf("저장한 것을 다시 읽으니 옛 목록이 없다: %v %+v", err, sf2.LegacyAutopass)
	}
}

// ★ IC-P14 — 같은 자산이 리뷰 항목과 자동통과 사이를 옮겨 다녀도 계획 선택값은 따라오고, 근거가
// 바뀌면 서명은 지워진다. 옛 식별자는 새 세션에 같은 ID 가 있으면 치환된 셈이고 없으면 남는다.
func TestCarryFollowsItemsAcrossCollections(t *testing.T) {
	prev := judgedSession()
	prev.RulesetVersion = v3
	moved := autopassItem("n1/openssl/libcrypto")
	moved.Plan, moved.Kind, moved.Level, moved.TargetAlgorithm, moved.RollbackNote = true, "REMEDIATION_KIND_CONFIG_ONLY", "L3", "ML-KEM (FIPS 203)", "cnf 조각 제거"
	prev.Autopass = []review.Item{moved}
	prev.LegacyAutopass = []string{"n1/openssl/libcrypto", "n9/openssl/libgone"}

	// 확신이 떨어져 리뷰 항목이 됐다. 근거(지문)도 달라졌다.
	next := judgedSession()
	next.RulesetVersion = v3
	next.Signature = ""
	back := moved
	back.Plan, back.Kind, back.Level, back.TargetAlgorithm, back.RollbackNote = false, "", "", "", ""
	back.Conf, back.Fingerprint, back.Mandatory = 0.7, "fp-changed", false
	next.Items = append(next.Items, back)

	got := review.Carry(prev, next)
	var carried *review.Item
	for i := range got.Items {
		if got.Items[i].ID == moved.ID {
			carried = &got.Items[i]
		}
	}
	if carried == nil || !carried.Plan || carried.Level != "L3" || carried.RollbackNote != "cnf 조각 제거" {
		t.Fatalf("옮겨 간 항목의 계획 선택값이 따라오지 않았다: %+v", carried)
	}
	if got.Signature != "" {
		t.Error("근거가 바뀌었는데 서명이 살아남았다")
	}
	if len(got.LegacyAutopass) != 1 || got.LegacyAutopass[0] != "n9/openssl/libgone" {
		t.Errorf("옛 식별자: 같은 ID 는 치환되고 없는 것만 남아야 한다: %v", got.LegacyAutopass)
	}

	// 아무것도 안 바뀌면 서명이 남는다 - 자리(index)가 아니라 ID 로 맞춘다.
	same := review.Carry(prev, prev)
	if same.Signature != prev.Signature {
		t.Error("근거가 그대로인데 서명이 지워졌다")
	}
	// 자동통과의 근거만 바뀌어도 서명이 무효다.
	only := judgedSession()
	only.RulesetVersion, only.Signature = v3, "" // 새로 세운 세션은 서명이 없다. Carry 가 옮겨 주는지가 물음이다
	only.Autopass = []review.Item{moved}
	only.Autopass[0].Fingerprint = "fp-other"
	if review.Carry(prev, only).Signature != "" {
		t.Error("자동통과의 근거만 바뀌었는데 서명이 살아남았다")
	}
}

// ★ IC-P15 — v3 근거 해시는 관리 판정·제외 근거·미평가 여부를 덮는다. v2 해시는 그대로다.
//
// 관리 근거만 덮으면 정책이 바뀌어 근거 구성이 달라져도 서명이 살아남는다. 미평가와 0.00 을 해시가
// 가르지 못하면 정책이 바뀌어 평가 대상에서 빠진 것이 서명을 살려 둔다.
func TestBasisCoversTheManagedAxisFromV3(t *testing.T) {
	base := autopassItem("n1/openssl/libcrypto")
	h := review.BasisOf(base, v3)

	x := base
	x.Managed = string(reconcile.ExcludedByPolicy)
	if review.BasisOf(x, v3) == h {
		t.Error("관리 판정이 바뀌었는데 해시가 같다")
	}
	e := base
	e.ExcludedSources = []review.ExcludedSource{{FindingID: "f-z", Fingerprint: "fp-z", SourceNodeID: "host-z"}}
	if review.BasisOf(e, v3) == h {
		t.Error("제외 근거가 더해졌는데 해시가 같다")
	}
	u := base
	u.ConfEvaluated = false
	if review.BasisOf(u, v3) == h {
		t.Error("미평가로 바뀌었는데 해시가 같다")
	}
	// v2 세션의 해시는 이 칸들을 보지 않는다 - 그 판의 원장 행·델타 비교와 어긋나면 안 된다.
	if review.BasisOf(base, v2) != review.BasisOf(x, v2) || review.BasisOf(base, v2) != review.BasisOf(e, v2) {
		t.Error("v2 해시가 v3 의 칸을 보고 있다")
	}
}

// ★ IC-P16 — rollback_note 는 계획 축의 값이다. v2 이하의 세션에서만 빈 값을 결론으로 채운다.
//
// 전에는 판정 결론을 그대로 넣었는데, 결론은 「어떻게 판정했나」이고 이것은 「어떻게 되돌리나」다.
// v3 사용자가 일부러 비운 자리에 결론이 다시 들어가면 사람의 선택을 덮는 것이다.
func TestRollbackNoteFallsBackOnlyForOldSessions(t *testing.T) {
	note := func(rs, conclusion, rollback string) string {
		sf := judgedSession()
		sf.RulesetVersion = rs
		sf.Items[0].Conclusion, sf.Items[0].RollbackNote = conclusion, rollback
		res, err := review.Finalize(sf)
		if err != nil {
			t.Fatal(err)
		}
		return res.Plan.GetActions()[0].GetRollbackNote()
	}
	if got := note(v2, "결론", ""); got != "결론" {
		t.Errorf("v2 세션의 빈 되돌림 메모는 결론으로 채워야 한다: %q", got)
	}
	if got := note(v3, "결론", ""); got != "" {
		t.Errorf("v3 세션의 빈 되돌림 메모는 빈 값이어야 한다: %q", got)
	}
	if got := note(v3, "결론", "cnf 조각 제거"); got != "cnf 조각 제거" {
		t.Errorf("적은 되돌림 메모가 그대로 나가야 한다: %q", got)
	}
}

// IC-P17 — 한 자산은 한 컬렉션에만 있다. 양쪽에 같은 ID 가 있으면 대조의 결함이므로 확정이 끊는다.
func TestDuplicateIDAcrossCollectionsIsAnError(t *testing.T) {
	sf := judgedSession()
	dup := autopassItem(sf.Items[0].ID)
	sf.Autopass = []review.Item{dup}
	_, err := review.Finalize(sf)
	if err == nil || !strings.Contains(err.Error(), "appears in both") {
		t.Fatalf("중복 ID 를 넘겼다: %v", err)
	}
	var nf *decision.NotFinalized
	if errors.As(err, &nf) {
		t.Error("중복 ID 는 판정 미완이 아니라 구조 오류다")
	}
}

// IC-P18 — **옛 규칙 판의 세션을 그대로 확정하면 서명은 살아 있고 경고만 난다.** 막지 않는 것은
// 검토 중인 세션이 도구 교체로 버려지면 사람이 한 일이 사라지기 때문이다. 다만 그 근거 해시와
// 서명은 옛 규칙의 것이라, 무엇으로 판정됐는지를 값으로 알린다.
func TestFinalizingAnOldSessionWarnsButDoesNotBlock(t *testing.T) {
	sf := judgedSession()
	sf.RulesetVersion = v2

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	res, ferr := review.Finalize(sf)
	w.Close()
	os.Stderr = saved
	var out strings.Builder
	buf := make([]byte, 4096)
	for {
		n, rerr := r.Read(buf)
		out.Write(buf[:n])
		if rerr != nil {
			break
		}
	}
	if ferr != nil {
		t.Fatalf("옛 세션이 막혔다: %v", ferr)
	}
	if res.Plan.GetRulesetVersion() != v2 {
		t.Errorf("계획의 규칙 판이 세션의 것이 아니다: %s", res.Plan.GetRulesetVersion())
	}
	if !strings.Contains(out.String(), "warning") || !strings.Contains(out.String(), v2) || !strings.Contains(out.String(), review.RulesetVersion) {
		t.Errorf("옛 규칙 판이라는 경고가 없거나 값이 빠졌다: %q", out.String())
	}
	// 같은 세션을 v3 으로 다시 열어 Carry 하면 서명은 지워진다 - 근거 해시의 입력이 넓어졌다.
	next := judgedSession()
	next.RulesetVersion, next.Signature = review.RulesetVersion, ""
	if review.Carry(sf, next).Signature != "" {
		t.Error("규칙 판이 올랐는데 서명이 살아남았다")
	}
}
