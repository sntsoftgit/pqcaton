package scope

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"
	"github.com/randyinthedev-hash/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// Session — 스코프 변경을 사람이 승인하는 파일.
//
// 명령과 화면이 같은 파일을 읽고 쓰므로 형식이 한 곳에 있어야 한다. 확정 게이트도 여기
// 하나다 — 두 벌이면 언젠가 한쪽만 고쳐지고, 그날 화면으로는 확정되는데 명령으로는 안 되는
// 정책이 생긴다.
type Session struct {
	Note      string `json:"_how_to_read"`
	Org       string `json:"org"`
	Reviewer  string `json:"reviewer"`
	Signature string `json:"signature"`
	// LayerDecisions — 계층 하나에 결론 하나가 기본이다(§3.4). 규칙 한 줄씩 승인하는 리뷰는
	// 수천 대에서 끝나지 않는다. 개별 규칙의 conclusion 은 예외를 위한 자리다.
	LayerDecisions map[string]string `json:"layer_decisions"`
	Changes        []ChangeItem      `json:"changes"`
	// Merged — 확정되면 그대로 CSV 로 나갈 정책 전문. **바뀐 것만 리뷰하되 나가는 것은
	// 전문이다** — pqcota의 집행기는 정책 전체를 받는다.
	Merged []Rule `json:"policy_on_finalize"`
}

// ChangeItem — 사람이 판정하는 변경 하나.
type ChangeItem struct {
	ID    string `json:"id"`
	Layer string `json:"layer"`
	// Kind — [KindAdded] | [KindRemoved].
	//
	// **값은 코드다.** 화면이 두 언어라 여기에 사람이 읽는 말을 담으면, 파일에 담긴
	// 말과 화면에 뜨는 말이 갈린다 — 그리고 그 파일은 다른 언어로 연 화면에서 읽힌다.
	Kind string `json:"change"`
	Rule string `json:"rule"`
	Note string `json:"note,omitempty"`
	// Audited — 결론 없이 확정할 수 없다. exclude 추가가 그것이다.
	Audited bool `json:"reason_required"`

	// ── 사람이 채우는 자리 ──
	Conclusion string `json:"conclusion"`
}

// Rule — 확정될 정책의 한 줄. 상류 형식 그대로다.
type Rule struct {
	Action  string `json:"action"`
	Runtime string `json:"runtime,omitempty"`
	Lib     string `json:"lib,omitempty"`
	AppKey  string `json:"app_key,omitempty"`
	Note    string `json:"note,omitempty"`
}

// 변경의 종류. 화면이 이 값을 보고 그 언어의 말을 고른다.
const (
	KindAdded   = "added"
	KindRemoved = "removed"
)

// SessionNote — 세션 파일 첫 줄에 적히는 사용법.
const SessionNote = "Write one conclusion per layer under layer_decisions and every rule in " +
	"that layer is judged at once (recommended). Use a change's own conclusion only for " +
	"exceptions. Fill in reviewer and signature, then feed this to `pqcaton-scope close`. " +
	"A change marked reason_required is not finalized without a conclusion."

// NewSession — 계층을 겹치고 지금 정책과 견줘 리뷰 세션을 만든다.
func NewSession(layers []Layer, base *kscope.AssetPolicy, orgName string) Session {
	merged := Merge(layers...)
	sf := Session{Note: SessionNote, Org: orgName, LayerDecisions: map[string]string{}}
	for _, c := range Diff(base, layers) {
		kind := KindAdded
		if !c.Added {
			kind = KindRemoved
		}
		sf.Changes = append(sf.Changes, ChangeItem{
			ID: RuleID(c.Rule), Layer: c.Layer, Kind: kind,
			Rule: RuleID(c.Rule), Note: c.Rule.Note, Audited: c.Audited,
		})
		if _, ok := sf.LayerDecisions[c.Layer]; !ok {
			sf.LayerDecisions[c.Layer] = "" // 사람이 채울 자리를 미리 연다
		}
	}
	for _, r := range merged.Rules {
		act := "include"
		if r.Exclude {
			act = "exclude"
		}
		sf.Merged = append(sf.Merged, Rule{Action: act, Runtime: r.Runtime,
			Lib: r.Lib, AppKey: r.AppKey, Note: r.Note})
	}
	return sf
}

// LoadSession — 세션 파일을 읽는다.
func LoadSession(path string) (Session, error) {
	var sf Session
	raw, err := os.ReadFile(path)
	if err != nil {
		return sf, err
	}
	if err := json.Unmarshal(raw, &sf); err != nil {
		return sf, fmt.Errorf("session file: %w", err)
	}
	return sf, nil
}

// SaveSession — 세션 파일을 쓴다. 화면이 판정을 채워 넣는 자리다.
func SaveSession(path string, sf Session) error {
	raw, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// AuditedCount — 근거 없이 확정할 수 없는 변경 수.
func (s Session) AuditedCount() int {
	n := 0
	for _, c := range s.Changes {
		if c.Audited {
			n++
		}
	}
	return n
}

// Policy — 확정될 정책을 상류 타입으로.
func (s Session) Policy() *kscope.AssetPolicy {
	p := &kscope.AssetPolicy{}
	for _, r := range s.Merged {
		p.Rules = append(p.Rules, kscope.AssetRule{
			Exclude: r.Action == "exclude", Runtime: r.Runtime,
			Lib: r.Lib, AppKey: r.AppKey, Note: r.Note,
		})
	}
	return p
}

// FinalizeResult — 확정이 통과한 결과.
type FinalizeResult struct {
	// Batched — 계층별 일괄 판정 건수.
	Batched map[string]int
	// Decided — 실제로 판정된 변경의 결론.
	Decided map[string]string
	Policy  *kscope.AssetPolicy
}

// Finalize — **게이트다.** 근거 필수인 변경에 결론이 없거나 서명이 없으면 정책이 나가지 않는다.
//
// 명령과 화면이 이 함수 하나를 쓴다.
func Finalize(sf Session, orgName string) (*FinalizeResult, error) {
	// **세션에 적힌 조직과 지금 준 조직이 다르면 끊는다.** 남의 조직 정책을 확정하는 것은
	// 사고다 — 대조 엔진·판정 원장과 같은 규칙이다.
	if sf.Org != "" && orgName != "" && sf.Org != orgName {
		return nil, fmt.Errorf("the session belongs to organization %q but finalization was asked for %q", sf.Org, orgName)
	}
	items := make([]decision.Item, 0, len(sf.Changes))
	for _, c := range sf.Changes {
		items = append(items, decision.Item{ID: c.ID, Policy: c.Layer, Mandatory: c.Audited})
	}
	s := decision.NewSession("scope://"+orgName, items)
	if err := s.StartReview(); err != nil {
		return nil, err
	}
	batched := map[string]int{}
	// **계층이 먼저다**(§3.4). 개별 결론은 그 뒤에 얹혀 예외를 만든다.
	for layer, c := range sf.LayerDecisions {
		if strings.TrimSpace(c) != "" {
			if n := s.DecidePolicy(layer, c); n > 0 {
				batched[layer] = n
			}
		}
	}
	for _, c := range sf.Changes {
		if strings.TrimSpace(c.Conclusion) != "" {
			s.Decide(c.ID, c.Conclusion)
		}
	}
	if sf.Reviewer != "" || sf.Signature != "" {
		s.Sign(sf.Reviewer, sf.Signature)
	}
	if err := s.Finalize(); err != nil {
		return nil, &decision.NotFinalized{Err: err, Missing: Pending(sf)}
	}
	decided := map[string]string{}
	for _, it := range s.Items {
		if it.Decided {
			decided[it.ID] = it.Conclusion
		}
	}
	return &FinalizeResult{Batched: batched, Decided: decided, Policy: sf.Policy()}, nil
}

// Pending — 무엇이 남았는지. 모르면 사람은 파일도 화면도 고칠 수 없다.
func Pending(sf Session) []decision.Missing {
	var out []decision.Missing
	if sf.Reviewer == "" || sf.Signature == "" {
		out = append(out, decision.Missing{Code: decision.MissingSignature})
	}
	for _, c := range sf.Changes {
		if c.Audited && strings.TrimSpace(c.Conclusion) == "" &&
			strings.TrimSpace(sf.LayerDecisions[c.Layer]) == "" {
			out = append(out, decision.Missing{
				Code: decision.MissingConclusion, Subject: c.ID, Detail: "layer " + c.Layer,
			})
		}
	}
	return out
}

// SaveJudgments — 확정된 변경을 원장에 남긴다. **게이트를 지난 뒤에만** 남긴다.
func SaveJudgments(path, orgName string, sf Session, decided map[string]string) (int, error) {
	store, err := decision.NewFileJudgmentStore(org.ID(orgName), path)
	if err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	n := 0
	for _, c := range sf.Changes {
		concl, ok := decided[c.ID]
		if !ok {
			continue
		}
		j := &decision.Judgment{
			ID: fmt.Sprintf("%s@%d", c.ID, now), Subject: c.ID, Conclusion: concl,
			Reviewer: sf.Reviewer, Signature: sf.Signature,
			// 근거는 규칙 그 자체다 — 규칙이 달라지면 대상 id 가 달라지므로 새 판정이 된다.
			BasisHash: decision.HashBasis(c.ID), DecidedAt: now,
		}
		if err := store.Save(j); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// LoadPolicyFile — 정책 CSV 를 읽는다. 형식과 판정은 상류 것을 그대로 쓴다.
func LoadPolicyFile(path string) (*kscope.AssetPolicy, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	p, err := kscope.LoadAssetPolicy(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}
