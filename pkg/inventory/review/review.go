// Package review — 사람이 판정하는 세션의 파일 형식과 **확정 게이트**.
//
// 명령에서 떼어 둔 것은 게이트가 두 벌이 되면 안 되기 때문이다. `pqcaton-decide close`와
// 화면의 확정 버튼이 각자 게이트를 들고 있으면 언젠가 한쪽만 고쳐지고, 그날 화면으로는
// 확정되는데 명령으로는 안 되는(또는 그 반대의) 계획이 생긴다.
//
// **파일 형식이 곧 감사 기록이다.** 화면이 생겨도 산출물은 파일이다 — 무엇을 근거로 무엇을
// 정했는지가 화면에서 사라지지 않는다.
package review

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	provisioningv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

// Session — 사람이 편집하는 파일. **라이브러리 타입을 그대로 쓰지 않는다** — 편집하는
// 사람에게 필요한 것과 상태기계가 필요한 것이 다르다.
type Session struct {
	Note      string `json:"_읽는_법"`
	Scope     string `json:"scope"`
	Reviewer  string `json:"reviewer"`
	Signature string `json:"signature"`
	// PolicyDecisions — 정책 하나에 결론 하나. **이것이 기본 단위다**(§3.4) — 수천 대를
	// 한 건씩 보는 리뷰는 끝나지 않는다. 개별 항목의 Conclusion 은 예외를 위한 자리다.
	PolicyDecisions map[string]string `json:"정책_판정"`
	Items           []Item            `json:"items"`
	Autopass        []string          `json:"autopass_후보"`
}

// Item — 판정 대상 하나.
type Item struct {
	ID string `json:"id"`
	// Node · Runtime — **id 에서 되찾지 않고 여기 적어 둔다.** 노드가 `host://local` 같은
	// URI면 `/` 로 쪼개 복원할 수 없다 — 조치가 엉뚱한 노드를 겨누고, 런타임이 비어
	// 기본값으로 조용히 떨어진다. 대조할 때 이미 알던 값이므로 그대로 들고 간다.
	Node    string `json:"node"`
	Runtime string `json:"runtime"`
	// Policy — 같은 정책의 항목은 한 번에 판정한다(§3.4).
	Policy string  `json:"policy"`
	State  string  `json:"state"`
	Conf   float64 `json:"confidence"`
	// Mandatory — 이 항목은 결론 없이 확정할 수 없다(§3.3②).
	Mandatory bool `json:"mandatory"`
	// Rescan — UNOBSERVED인데 커버리지 갭으로 설명된다. **「없다」가 아니라 「못 봤다」**이므로
	// 재수집이 먼저다(§2.7).
	Rescan bool `json:"rescan_후보,omitempty"`

	// ── 사람이 채우는 자리 ──
	Conclusion string `json:"conclusion"`
	Plan       bool   `json:"확정_계획에_넣는다"`
	Level      string `json:"deploy_level,omitempty"` // L1 | L2 | L3
	FIPS       bool   `json:"fips_요구,omitempty"`
	// Kind — 조치 종류. 계약의 통제 어휘다(`REMEDIATION_KIND_*`). 비우면 PROVIDER_INJECT.
	Kind string `json:"조치_종류,omitempty"`
	// Config — provider 설정 조각. **도구가 지어내지 않는다.**
	Config string `json:"config_artifact,omitempty"`
}

// Note — 세션 파일 첫 줄에 적히는 사용법.
const Note = "정책_판정 에 정책별 결론을 적으면 같은 정책의 항목이 한 번에 판정됩니다(권장). " +
	"예외만 항목의 conclusion 으로 따로 적습니다. reviewer 와 signature 를 채운 뒤 " +
	"`pqcaton-decide close` 에 넣으세요. 확정 계획에 넣을 항목은 `확정_계획에_넣는다`를 true 로."

// Load — 세션 파일을 읽는다.
func Load(path string) (Session, error) {
	var sf Session
	raw, err := os.ReadFile(path)
	if err != nil {
		return sf, err
	}
	if err := json.Unmarshal(raw, &sf); err != nil {
		return sf, fmt.Errorf("세션 파일: %w", err)
	}
	return sf, nil
}

// Save — 세션 파일을 쓴다. 화면이 판정을 채워 넣는 자리다.
//
// **덮어쓰되 형식은 그대로다.** 화면으로 채운 파일을 명령이 그대로 읽을 수 있어야 한다 —
// 아니면 두 길이 갈린다.
func Save(path string, sf Session) error {
	raw, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// Result — 확정이 통과한 결과.
type Result struct {
	Plan *provisioningv1.FinalizedPlan
	// Decided — 실제로 판정된 항목의 결론. 정책 일괄로 붙은 것도 들어온다.
	Decided map[string]string
	// Batched — 정책별 일괄 판정 건수. 무엇이 한 번에 정해졌는지 사람에게 보여 주는 값이다.
	Batched map[string]int
	Scope   string
}

// Finalize — **이 리포의 최강 게이트**(§3.7).
//
// 필수 항목의 결론과 승인 서명이 모두 있어야 통과한다. 통과하지 못하면 왜 안 되는지
// [Pending]이 말한다 — 무엇을 더 채워야 하는지 모르면 사람은 파일도 화면도 고칠 수 없다.
//
// **명령과 화면이 이 함수 하나를 쓴다.** 게이트가 두 벌이면 언젠가 한쪽만 고쳐진다.
func Finalize(sf Session) (*Result, error) {
	items := make([]decision.Item, 0, len(sf.Items))
	for _, it := range sf.Items {
		items = append(items, decision.Item{ID: it.ID, Policy: it.Policy, Mandatory: it.Mandatory})
	}
	s := decision.NewSession(sf.Scope, items)
	if err := s.StartReview(); err != nil {
		return nil, err
	}

	batched := map[string]int{}
	// **정책이 먼저다**(§3.4). 개별 결론은 그 뒤에 얹혀 예외를 만든다.
	for pol, c := range sf.PolicyDecisions {
		if strings.TrimSpace(c) != "" {
			if n := s.DecidePolicy(pol, c); n > 0 {
				batched[pol] = n
			}
		}
	}
	for _, it := range sf.Items {
		if strings.TrimSpace(it.Conclusion) != "" {
			s.Decide(it.ID, it.Conclusion)
		}
	}
	if sf.Reviewer != "" || sf.Signature != "" {
		s.Sign(sf.Reviewer, sf.Signature)
	}
	if err := s.Finalize(); err != nil {
		return nil, fmt.Errorf("%w\n%s", err, Pending(sf))
	}

	plan := make([]decision.PlanItem, 0)
	picked := make([]Item, 0)
	for _, it := range sf.Items {
		if !it.Plan {
			continue
		}
		if err := RequireNode(it); err != nil {
			return nil, err
		}
		lvl := it.Level
		if lvl == "" {
			lvl = "L2"
		}
		plan = append(plan, decision.PlanItem{
			NodeID: it.Node, RemediationClass: it.Conclusion,
			DeployAutomationLevel: lvl,
			ProviderChoice:        decision.RouteProvider(it.Runtime, it.FIPS),
		})
		picked = append(picked, it)
	}
	// **게이트는 여기다.** finalized 아닌 세션에서는 계획 자체가 만들어지지 않는다.
	p, err := decision.BuildPlan(s, plan)
	if err != nil {
		return nil, err
	}
	if err := decision.AcceptForDeploy(p); err != nil {
		return nil, err
	}
	out, err := ToContract(p, picked)
	if err != nil {
		return nil, err
	}
	return &Result{Plan: out, Decided: decidedOf(s), Batched: batched, Scope: p.Scope}, nil
}

// Pending — 무엇이 남았는지. 「안 된다」만 말하면 사람은 고칠 수 없다.
func Pending(sf Session) string {
	var b strings.Builder
	if sf.Signature == "" {
		b.WriteString("   · signature 가 비어 있습니다\n")
	}
	for _, it := range sf.Items {
		if it.Mandatory && strings.TrimSpace(it.Conclusion) == "" &&
			strings.TrimSpace(sf.PolicyDecisions[it.Policy]) == "" {
			fmt.Fprintf(&b, "   · 결론 없음: %s (%s)\n", it.ID, it.State)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// RequireNode — **지어내지 않고 끊는다.** node 가 비면 겨눌 곳을 모르는 것이고, 빈 채로
// 내보내면 pqcota가 이름 없는 노드에 조치를 건다. v0.1.0 이 낸 세션 파일이 여기 걸린다.
func RequireNode(it Item) error {
	if it.Node == "" {
		return fmt.Errorf("항목 %s 에 node 가 없다 — `pqcaton-decide open` 을 다시 돌려 세션을 새로 받아라", it.ID)
	}
	return nil
}

// SaveJudgments — 판정을 append-only 로 남긴다.
//
// **근거 해시를 함께 적는다.** 그것이 없으면 나중에 「근거가 바뀌었나」를 물을 수 없고,
// 델타 리뷰가 성립하지 않는다(§3.6).
func SaveJudgments(path, orgName string, sf Session, decided map[string]string) (int, error) {
	store, err := decision.NewFileJudgmentStore(org.ID(orgName), path)
	if err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	n := 0
	for _, it := range sf.Items {
		c, ok := decided[it.ID]
		if !ok {
			continue
		}
		j := &decision.Judgment{
			ID: fmt.Sprintf("%s@%d", it.ID, now), Subject: it.ID, Conclusion: c,
			Reviewer: sf.Reviewer, Signature: sf.Signature,
			BasisHash: BasisOf(it), Confidence: it.Conf, DecidedAt: now,
		}
		if err := store.Save(j); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// BasisOf — 이 판정이 무엇을 보고 내려졌나. 대조 상태와 확신도가 근거다.
//
// **관측이 바뀌면 이 값이 바뀐다** — 그때 델타 리뷰가 걸린다. 반대로 관측이 그대로면
// 몇 번을 다시 돌려도 걸리지 않는다(§3.6, IC-D2/D3).
func BasisOf(it Item) string {
	return decision.HashBasis(
		"state="+it.State,
		fmt.Sprintf("conf=%.2f", it.Conf),
		"policy="+it.Policy,
	)
}

// ToContract — 확정 계획을 pqcota 계약(`provisioningv1.FinalizedPlan`)으로 옮긴다.
//
// 어휘의 단일 출처는 계약이다 — 이 리포는 그 어휘로 말하고 자기 형식을 새로 만들지 않는다.
func ToContract(p *decision.FinalizedPlan, items []Item) (*provisioningv1.FinalizedPlan, error) {
	out := &provisioningv1.FinalizedPlan{
		Scope:              p.Scope,
		Status:             provisioningv1.PlanStatus_PLAN_STATUS_FINALIZED,
		ApprovalSignatures: []string{p.ApprovalSig},
	}
	for i, it := range items {
		kind, err := kindOf(it.Kind)
		if err != nil {
			return nil, err
		}
		out.Actions = append(out.Actions, &provisioningv1.RemediationAction{
			Id:              fmt.Sprintf("a%d", i+1),
			TargetNodeId:    p.Items[i].NodeID,
			CryptoRuntime:   runtimeOf(it.Runtime),
			Kind:            kind,
			AutomationLevel: levelOf(p.Items[i].DeployAutomationLevel),
			ProviderChoice:  p.Items[i].ProviderChoice,
			ConfigArtifact:  it.Config,
			RollbackNote:    it.Conclusion,
		})
	}
	return out, nil
}

// Key — 자산 열쇠의 문자열 표현. 판정 원장의 대상 id 가 된다.
func Key(k reconcile.AssetKey) string {
	return k.NodeID + "/" + k.Runtime + "/" + k.Component
}

// PolicyOf — 같은 정책으로 묶는 열쇠. 런타임과 컴포넌트 이름에서 만든다.
//
// 컴포넌트에 붙는 **버전 해시만** 뗀다(`libssl-e2f2d68a` → `libssl`). 같은 라이브러리의 여러
// 판이 한 묶음이 되는 것이 정책 단위 리뷰가 뜻하는 것이다(§3.4).
//
// **해시처럼 생긴 것만 뗀다.** 길이로만 자르면 `jca-provider-chain`의 `-chain`까지 떼어
// 이름이 다른 컴포넌트가 한 정책으로 묶인다.
func PolicyOf(k reconcile.AssetKey) string {
	c := k.Component
	if i := strings.LastIndex(c, "-"); i > 0 && isHex(c[i+1:]) {
		c = c[:i]
	}
	return k.Runtime + "/" + c
}

func isHex(s string) bool {
	if len(s) < 8 {
		return false
	}
	for _, r := range s {
		if !('0' <= r && r <= '9' || 'a' <= r && r <= 'f') {
			return false
		}
	}
	return true
}

func decidedOf(s *decision.Session) map[string]string {
	out := map[string]string{}
	for _, it := range s.Items {
		if it.Decided {
			out[it.ID] = it.Conclusion
		}
	}
	return out
}

func runtimeOf(s string) commonv1.CryptoRuntime {
	if s == "jca" {
		return commonv1.CryptoRuntime_CRYPTO_RUNTIME_JCA
	}
	return commonv1.CryptoRuntime_CRYPTO_RUNTIME_OPENSSL
}

// kindOf — 비우면 `PROVIDER_INJECT`. **모르는 값은 지어내지 않고 끊는다** — 계약의 통제
// 어휘라 오타가 조용히 UNSPECIFIED로 떨어지면 그 조치는 아무것도 하지 않는다.
func kindOf(s string) (provisioningv1.RemediationKind, error) {
	if s == "" {
		return provisioningv1.RemediationKind_REMEDIATION_KIND_PROVIDER_INJECT, nil
	}
	if v, ok := provisioningv1.RemediationKind_value[s]; ok && v != 0 {
		return provisioningv1.RemediationKind(v), nil
	}
	return 0, fmt.Errorf("모르는 조치_종류: %q — 계약의 REMEDIATION_KIND_* 중 하나여야 합니다", s)
}

func levelOf(s string) provisioningv1.DeployAutomationLevel {
	switch strings.ToUpper(s) {
	case "L1":
		return provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L1_STAGE_ONLY
	case "L3":
		return provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L3_FULL_AUTO
	default:
		return provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L2_STAGE_INSTALL
	}
}
