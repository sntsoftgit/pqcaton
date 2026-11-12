// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review

// 규칙 전환 회차의 판정 쪽. 상류 규칙 판이 다른 옛 세션을 지금 규칙으로 다시 세운 세션에 넘긴다.
//
// **옮기는 것은 참고값뿐이다.** 옛 결론 · 정책 결론 · 계획 칸은 항목의 Previous와 세션의 PreviousPolicyDecisions에만
// 들어가고, 현재 칸은 비어 있다. 결론이 차면 그 항목은 판정된 것이라 서명만 넣어도 확정을 지나가기 때문이다. 사람이
// 항목마다 [Adopt]로 다시 채택해야 현재 값이 된다. 한꺼번에 채택하는 함수는 두지 않는다. 그것이 곧 자동 이월이다.
//
// 화면과 명령이 이 함수 하나를 쓴다. 같은 입력이면 같은 세션 · 같은 보고서 바이트가 나온다(시각을 넣지 않는다).

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
)

// RulesetChangeReportFormat — 변환 보고서의 형식 판.
const RulesetChangeReportFormat = 1

// RulesetChangeReport — 변환 보고서. 옛 세션과 새 세션을 자산 키로 짝지어 무엇이 달라졌는지 적는다.
type RulesetChangeReport struct {
	Format        int    `json:"format"`
	Note          string `json:"_how_to_read"`
	FromRuleset   string `json:"from_ruleset"`
	ToRuleset     string `json:"to_ruleset"`
	FromSessionID string `json:"from_session_id"`
	ToSessionID   string `json:"to_session_id"`
	Archive       string `json:"archive,omitempty"`
	// Reason · FromPolicy · ToPolicy — 정책 회차의 보고서면 그 종류와 두 정책 지문(옛 세션에 지문이 없으면 FromPolicy는 빈다).
	Reason     string `json:"reason,omitempty"`
	FromPolicy string `json:"from_policy,omitempty"`
	ToPolicy   string `json:"to_policy,omitempty"`
	// UpstreamCheck — 정책 회차가 거친 pqcota 이력 대조(확인 시각 · 조직 · 노드별 지문). 비밀값은 없다. 형식 판을 올리지 않는
	// 선택 필드다: 이전 판은 모르는 필드를 무시하고 읽는다. v0.28.0 이전의 보고서와 규칙 회차 보고서에는 없다.
	UpstreamCheck *UpstreamCheck `json:"upstream_check,omitempty"`
	Items         []ItemChange   `json:"items"`
}

// UpstreamCheck — pqcota 이력 대조의 기록. **확인 시점에** 노드마다 그 지문이 pqcota 이력의 최신 스냅샷이었다는 뜻이다. 지금도
// 같다는 보장이나 적재의 증명이 아니다.
type UpstreamCheck struct {
	CheckedAt string             `json:"checked_at"`
	Org       string             `json:"org"`
	Snapshots []UpstreamSnapshot `json:"snapshots"`
}

// UpstreamSnapshot — 대조한 스냅샷 하나(노드 · 규칙 판 · v1 지문).
type UpstreamSnapshot struct {
	Node    string `json:"node"`
	Ruleset string `json:"ruleset"`
	Digest  string `json:"digest"`
}

// ItemChange — 항목 하나의 변화. Change는 only-old(새 규칙에서는 없다) · only-new(새 규칙에서 생겼다) · changed · same.
type ItemChange struct {
	ID                 string   `json:"id"`
	Change             string   `json:"change"`
	OldState           string   `json:"old_state,omitempty"`
	NewState           string   `json:"new_state,omitempty"`
	OldFindings        []string `json:"old_findings,omitempty"`
	NewFindings        []string `json:"new_findings,omitempty"`
	OldInstallations   []string `json:"old_installations,omitempty"`
	NewInstallations   []string `json:"new_installations,omitempty"`
	EvidenceChanged    bool     `json:"evidence_changed,omitempty"`
	PreviousConclusion string   `json:"previous_conclusion,omitempty"`
	PreviousInPlan     bool     `json:"previous_in_plan,omitempty"`
}

// 변화 종류.
const (
	ChangeOnlyOld = "only-old"
	ChangeOnlyNew = "only-new"
	ChangeChanged = "changed"
	ChangeSame    = "same"
)

const reportNote = "The upstream ruleset changed between these two sessions. Asset ids and snapshot fingerprints are computed " +
	"by different rules, so an entry that looks removed and added, or changed, may come from the rule change alone and is not " +
	"by itself an installation removed, added or changed. Conclusions of the old session are carried only as reference values " +
	"(previous); nothing here was judged under the new rules."

// RulesetRound — 옛 세션(old)을 지금 규칙으로 다시 세운 세션(rebuilt)에 넘긴다. 새 세션은 newID를 들고, 서명과
// 검토자는 비고, 현재 결론과 계획 칸은 rebuilt 그대로(비어 있다)다. 옛 값은 참고값으로만 붙는다.
func RulesetRound(old, rebuilt Session, newID, reportName, archiveName string) (Session, RulesetChangeReport) {
	next := carryAsPrevious(old, rebuilt, newID)
	next.RulesetChange = &RulesetChangeRef{FromSessionID: old.SessionID, FromRuleset: old.RulesetVersion, Report: reportName, Archive: archiveName}
	return next, reportOf(old, next, archiveName)
}

// ScopeChangeRef — 정책 회차의 기록. 보고서는 세션 파일과 같은 폴더의 그 이름이다.
type ScopeChangeRef struct {
	Reason        string `json:"reason"`
	FromSessionID string `json:"from_session_id"`
	FromPolicy    string `json:"from_policy,omitempty"`
	ToPolicy      string `json:"to_policy"`
	Report        string `json:"report"`
	Archive       string `json:"archive,omitempty"`
	// Basis — 이 회차를 연 근거(ScopeBasis*). 처리 기록의 round.basis와 같은 값이다. v0.28.0부터 새 회차는 언제나
	// ScopeBasisUpstream이다. 비어 있거나 ScopeBasisOperator인 것은 그 이전 판이 운영자 진술로 연 과거 기록이고, 05가 그
	// 한계를 그대로 보인다.
	Basis string `json:"basis,omitempty"`
}

// 정책 회차를 연 근거(영수증의 round.basis · ScopeChangeRef.Basis).
const (
	// ScopeBasisOperator — 운영자 진술. v0.27.x까지의 화면 체크와 명령의 -confirm-ingest가 남긴 **과거 기록**이다. 읽기만 하고
	// v0.28.0부터 새로 쓰지 않는다.
	ScopeBasisOperator = "operator-statement"
	// ScopeBasisUpstream — pqcota 이력 대조를 거쳤다: 확인 시점에 새 정책의 노드별 지문이 pqcota 이력의 노드별 최신 스냅샷과
	// 모두 같았다.
	ScopeBasisUpstream = "upstream-verified"
)

// 정책 회차의 종류.
const (
	ScopeRoundBaseline = "scope-baseline"
	// ScopeRoundActivation — 02에서 확정한 정책을 활성화하며 연 회차.
	ScopeRoundActivation = "scope-activation"
)

const scopeReportNote = "The asset-scope policy this session is judged under changed or was adopted as a baseline. What is managed " +
	"and the snapshot fingerprints follow the policy, so an entry that looks removed, added or changed may come from the policy " +
	"alone. Entries the new policy no longer manages are listed here and are not carried. Conclusions of the old session are " +
	"carried only as reference values (previous); nothing here was judged under the new policy."

// PolicyRound — 옛 세션(old)을 지금 정책(과 지금 규칙)으로 다시 세운 세션(rebuilt)에 넘긴다. [RulesetRound]와 같이 옛 결론은
// 참고값으로만 붙고, 새 정책에서 빠진 항목은 새 세션에 없고 보고서에만 남는다. reason은 ScopeRound*, basis는 ScopeBasis*다.
func PolicyRound(old, rebuilt Session, newID, reportName, archiveName, reason, basis string) (Session, RulesetChangeReport) {
	next := carryAsPrevious(old, rebuilt, newID)
	next.ScopeChange = &ScopeChangeRef{Reason: reason, FromSessionID: old.SessionID, FromPolicy: old.ScopePolicyDigestV1,
		ToPolicy: rebuilt.ScopePolicyDigestV1, Report: reportName, Archive: archiveName, Basis: basis}
	r := reportOf(old, next, archiveName)
	r.Note, r.Reason, r.FromPolicy, r.ToPolicy = scopeReportNote, reason, old.ScopePolicyDigestV1, rebuilt.ScopePolicyDigestV1
	return next, r
}

// carryAsPrevious — 두 회차가 함께 쓰는 넘김. 새 세션은 newID를 들고, 서명과 검토자는 비고, 현재 결론과 계획 칸은 rebuilt
// 그대로(비어 있다)다. 옛 값은 참고값으로만 붙는다.
func carryAsPrevious(old, rebuilt Session, newID string) Session {
	next := rebuilt
	next.SessionID, next.Signature, next.Reviewer = newID, "", ""
	next.Items = append([]Item(nil), rebuilt.Items...)
	next.Autopass = append([]Item(nil), rebuilt.Autopass...)
	next.PolicyDecisions = map[string]string{}
	for pol := range rebuilt.PolicyDecisions {
		next.PolicyDecisions[pol] = ""
	}
	was := map[string]Item{}
	for _, it := range All(old) {
		was[it.ID] = it
	}
	for _, it := range All(next) {
		o, ok := was[it.ID]
		if !ok || !hasHumanValues(o) {
			continue
		}
		p := previousOf(o, old)
		update(&next, it.ID, func(n *Item) { n.Previous = &p })
	}
	next.PreviousPolicyDecisions = nil
	for pol, c := range old.PolicyDecisions {
		if c != "" {
			if next.PreviousPolicyDecisions == nil {
				next.PreviousPolicyDecisions = map[string]string{}
			}
			next.PreviousPolicyDecisions[pol] = c
		}
	}
	return next
}

func hasHumanValues(it Item) bool {
	return it.Conclusion != "" || it.Plan || it.Kind != "" || it.TargetAlgorithm != "" || it.Config != "" ||
		it.RollbackNote != "" || it.Pre != "" || it.Activate != "" || it.Deactivate != "" || it.Restart != "" ||
		len(it.InstallationsConfirmed) > 0 || it.PerInstallation
}

func previousOf(o Item, old Session) Previous {
	return Previous{SessionID: old.SessionID, RulesetVersion: old.RulesetVersion,
		Conclusion: o.Conclusion, Plan: o.Plan, Level: o.Level, FIPS: o.FIPS, Kind: o.Kind, TargetAlgorithm: o.TargetAlgorithm,
		Config: o.Config, RollbackNote: o.RollbackNote, Pre: o.Pre, Activate: o.Activate, Deactivate: o.Deactivate, Restart: o.Restart,
		InstallationsConfirmed: append([]string(nil), o.InstallationsConfirmed...), PerInstallation: o.PerInstallation}
}

func findingsOf(it Item) []string {
	var out []string
	for _, s := range it.Sources {
		if !slices.Contains(out, s.FindingID) {
			out = append(out, s.FindingID)
		}
	}
	if len(out) == 0 && it.FindingID != "" {
		out = append(out, it.FindingID)
	}
	sort.Strings(out)
	return out
}

func fingerprintsOf(it Item) []string {
	var out []string
	for _, s := range it.Sources {
		out = append(out, s.Fingerprint)
	}
	if len(out) == 0 && it.Fingerprint != "" {
		out = append(out, it.Fingerprint)
	}
	sort.Strings(out)
	return out
}

func reportOf(old, next Session, archiveName string) RulesetChangeReport {
	r := RulesetChangeReport{Format: RulesetChangeReportFormat, Note: reportNote, FromRuleset: old.RulesetVersion, ToRuleset: next.RulesetVersion,
		FromSessionID: old.SessionID, ToSessionID: next.SessionID, Archive: archiveName, Items: []ItemChange{}}
	was, now := map[string]Item{}, map[string]Item{}
	var ids []string
	for _, it := range All(old) {
		was[it.ID] = it
		ids = append(ids, it.ID)
	}
	for _, it := range All(next) {
		now[it.ID] = it
		if _, ok := was[it.ID]; !ok {
			ids = append(ids, it.ID)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		o, inOld := was[id]
		n, inNew := now[id]
		c := ItemChange{ID: id}
		if inOld {
			c.OldState, c.OldFindings, c.OldInstallations = o.State, findingsOf(o), Installations(o)
			c.PreviousConclusion, c.PreviousInPlan = o.Conclusion, o.Plan
		}
		if inNew {
			c.NewState, c.NewFindings, c.NewInstallations = n.State, findingsOf(n), Installations(n)
		}
		switch {
		case !inNew:
			c.Change = ChangeOnlyOld
		case !inOld:
			c.Change = ChangeOnlyNew
		default:
			c.EvidenceChanged = !slices.Equal(fingerprintsOf(o), fingerprintsOf(n))
			if c.OldState != c.NewState || c.EvidenceChanged || !slices.Equal(c.OldFindings, c.NewFindings) ||
				!slices.Equal(c.OldInstallations, c.NewInstallations) {
				c.Change = ChangeChanged
			} else {
				c.Change = ChangeSame
			}
		}
		r.Items = append(r.Items, c)
	}
	return r
}

// EncodeReport — 보고서의 바이트. 같은 보고서면 같은 바이트다.
func EncodeReport(r RulesetChangeReport) ([]byte, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// DecodeReport — 보고서를 읽는다. 형식 판이 다르면 거절한다.
//
// **호환 계약: 모르는 필드는 무시한다**(json.Unmarshal, DisallowUnknownFields를 쓰지 않는다). 형식 판 1 안에서는 선택 필드만
// 더하고(omitempty), 그 필드가 없어도 보고서의 뜻이 그대로여야 한다. 이 규칙 덕분에 upstream_check(v0.28.0)를 더해도 형식 판을
// 올리지 않고, v0.27.x 이하의 판독기(같은 함수)가 새 보고서를 읽는다. 필드를 지우거나 뜻을 바꾸거나 엄격하게 읽어야 할 필드를
// 더할 때는 형식 판을 올린다. 시험 IC-P50이 이 계약을 잰다.
func DecodeReport(b []byte) (RulesetChangeReport, error) {
	var r RulesetChangeReport
	if err := json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	if r.Format != RulesetChangeReportFormat {
		return r, fmt.Errorf("ruleset change report: format %d, want %d", r.Format, RulesetChangeReportFormat)
	}
	return r, nil
}

// Adopt — 항목 하나의 참고값(Previous)을 현재 값으로 옮긴다. 사람이 이 회차에서 그 항목을 다시 판정한 것이다.
// 참고값은 남겨 둔다(무엇을 채택했는지 보인다). 없는 항목이거나 참고값이 없으면 false.
func Adopt(sf *Session, id string) bool {
	var ok bool
	update(sf, id, func(n *Item) {
		if n.Previous == nil {
			return
		}
		p := n.Previous
		n.Conclusion, n.Plan, n.Level, n.FIPS, n.Kind, n.TargetAlgorithm = p.Conclusion, p.Plan, p.Level, p.FIPS, p.Kind, p.TargetAlgorithm
		n.Config, n.RollbackNote, n.Pre, n.Activate, n.Deactivate, n.Restart = p.Config, p.RollbackNote, p.Pre, p.Activate, p.Deactivate, p.Restart
		// 다설치 확인은 옮기지 않는다. 설치 목록이 새 규칙으로 달라졌을 수 있어 지금 목록으로 다시 확인해야 한다.
		n.PerInstallation = p.PerInstallation
		ok = true
	})
	return ok
}

// AdoptPolicy — 정책 하나의 옛 결론을 현재 결론으로 옮긴다. 없으면 false.
func AdoptPolicy(sf *Session, pol string) bool {
	c, ok := sf.PreviousPolicyDecisions[pol]
	if !ok || c == "" {
		return false
	}
	if _, here := sf.PolicyDecisions[pol]; !here {
		return false
	}
	sf.PolicyDecisions[pol] = c
	return true
}
