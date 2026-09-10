// Package review — 사람이 판정하는 세션의 파일 형식과 **확정 관문**.
//
// 명령에서 떼어 둔 것은 관문이 둘이 되면 안 되기 때문이다. `pqcaton-decide close`와
// 화면의 확정 버튼이 각자 관문을 들고 있으면 언젠가 한쪽만 고쳐지고, 그날 화면으로는
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
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

// Session — 사람이 편집하는 파일. **라이브러리 타입을 그대로 쓰지 않는다** — 편집하는
// 사람에게 필요한 것과 상태기계가 필요한 것이 다르다.
type Session struct {
	Note      string `json:"_how_to_read"`
	Scope     string `json:"scope"`
	Reviewer  string `json:"reviewer"`
	Signature string `json:"signature"`
	// PolicyDecisions — 정책 하나에 결론 하나. **이것이 기본 단위다**(§3.4) — 수천 대를
	// 한 건씩 보는 리뷰는 끝나지 않는다. 개별 항목의 Conclusion 은 예외를 위한 자리다.
	PolicyDecisions map[string]string `json:"policy_decisions"`
	// RulesetVersion — 이 세션을 연 시점의 규칙 판. **여는 순간 박고 확정할 때 다시 찍지
	// 않는다.** 검토 도중 도구가 올라가도 실제 검토 근거가 보존되어야 하기 때문이다.
	RulesetVersion string   `json:"ruleset_version,omitempty"`
	Items          []Item   `json:"items"`
	Autopass       []string `json:"autopass_candidates"`
}

// Item — 판정 대상 하나.
type Item struct {
	ID string `json:"id"`
	// Node · Runtime — **id 에서 되찾지 않고 여기 적어 둔다.** 노드가 `host://local` 같은
	// URI면 `/` 로 쪼개 복원할 수 없다 — 조치가 엉뚱한 노드를 겨누고, 런타임이 비어
	// 기본값으로 조용히 떨어진다. 대조할 때 이미 알던 값이므로 그대로 들고 간다.
	Node    string `json:"node"`
	Runtime string `json:"runtime"`
	// FindingID — 이 항목을 낸 관측의 id. **조치의 근거로 상류까지 간다**(계약의 `finding_id`).
	// 사람이 채우는 자리가 아니라 대조가 들고 온 사실이다. UNOBSERVED 는 관측이 없어 빈다.
	FindingID string `json:"finding_id,omitempty"`
	// Policy — 같은 정책의 항목은 한 번에 판정한다(§3.4).
	Policy string  `json:"policy"`
	State  string  `json:"state"`
	Conf   float64 `json:"confidence"`
	// Mandatory — 이 항목은 결론 없이 확정할 수 없다(§3.3②).
	Mandatory bool `json:"mandatory"`
	// Rescan — UNOBSERVED인데 커버리지 갭으로 설명된다. **「없다」가 아니라 「못 봤다」**이므로
	// 재수집이 먼저다(§2.7).
	Rescan bool `json:"rescan_candidate,omitempty"`

	// ── 사람이 채우는 자리 ──
	Conclusion string `json:"conclusion"`
	Plan       bool   `json:"include_in_plan"`
	Level      string `json:"deploy_level,omitempty"` // L1 | L2 | L3
	FIPS       bool   `json:"fips_required,omitempty"`
	// Kind — 조치 종류. 계약의 통제 어휘다(`REMEDIATION_KIND_*`). **비우면 확정되지 않는다.**
	Kind string `json:"remediation_kind,omitempty"`
	// TargetAlgorithm — 무엇으로 바꾸는가. 비우면 상류가 낸 config 조각의 `Groups` 줄이 주석으로
	// 나가 **배치해도 아무것도 켜지지 않는다.** 도구가 고르지 않는 값이라 사람이 적는다.
	TargetAlgorithm string `json:"target_algorithm,omitempty"`
	// Config — provider 설정 조각. **도구가 지어내지 않는다.**
	Config string `json:"config_artifact,omitempty"`
	// 활성화 훅 — L3에서만 쓰인다. **도구가 추측하지 않는다**(상류 §2.5): 활성화 지점은 앱
	// 기동 방식에 달려 있어 관측으로 알 수 없다. 비면 상류가 무엇이 일어나지 않는지 알린다.
	Pre        string `json:"activation_pre,omitempty"`
	Activate   string `json:"activation_activate,omitempty"`
	Deactivate string `json:"activation_deactivate,omitempty"`
	Restart    string `json:"activation_restart,omitempty"`
}

// RulesetVersion — **이 판정의 근거가 된 규칙 판.** 상류의 강화 규칙과 이 리포의 대조·계획
// 규칙을 합친 것이다. 파생 결과를 바꿀 수 있는 규칙이 어느 쪽에서든 바뀌면 값이 달라진다.
//
// **릴리스 버전과 같지 않다.** 화면을 고치거나 문구를 다듬는다고 같은 관측에서 다른 판정이
// 나오지 않는다. 반대로 대조 규칙이나 계획 변환이 바뀌면, 릴리스를 내지 않아도 이 값은 올라야
// 한다 — 그때 옛 판정의 근거가 지금 규칙과 다르다는 사실이 이 문자열로 드러난다.
//
// **사용자가 입력하지 않는다.** 도구가 부여하고, 세션을 **열 때** 박아 둔다(확정할 때가 아니다).
// 검토 도중 도구가 올라가도 실제로 검토한 근거가 그대로 남아야 하기 때문이다.
const RulesetVersion = normalize.RulesetVersion + "+pqcaton-plan/v1"

// Note — 세션 파일 첫 줄에 적히는 사용법.
const Note = "Write one conclusion per policy under policy_decisions and every item in that " +
	"policy is judged at once (recommended). Use an item's own conclusion only for exceptions. " +
	"Fill in reviewer and signature, then feed this to `pqcaton-decide close`. " +
	"Set include_in_plan to true for items that go into the finalized plan — each of those also needs " +
	"deploy_level and remediation_kind chosen, target_algorithm when the kind delivers through config, " +
	"and the activation hooks when the level is L3. New items start at deploy_level L2 for you to confirm " +
	"or change; nothing else is filled in, and nothing empty is corrected at finalize time. The approval " +
	"signature covers all of it, so a silent default would put your name on a decision you did not make."

// Load — 세션 파일을 읽는다.
func Load(path string) (Session, error) {
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

// Finalize — **이 리포에서 반드시 거쳐야 하는 관문**(§3.7).
//
// 필수 항목의 결론과 승인 서명이 모두 있어야 통과한다. 통과하지 못하면 왜 안 되는지
// [Pending]이 말한다 — 무엇을 더 채워야 하는지 모르면 사람은 파일도 화면도 고칠 수 없다.
//
// **명령과 화면이 이 함수 하나를 쓴다.** 관문이 둘이면 언젠가 한쪽만 고쳐진다.
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
		return nil, &decision.NotFinalized{Err: err, Missing: Pending(sf)}
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
		if err := RequireDecisions(it); err != nil {
			return nil, err
		}
		plan = append(plan, decision.PlanItem{
			NodeID: it.Node, RemediationClass: it.Conclusion,
			DeployAutomationLevel: it.Level,
			ProviderChoice:        decision.RouteProvider(it.Runtime, it.FIPS),
		})
		picked = append(picked, it)
	}
	// 규칙 판은 **세션이 들고 있던 것**을 쓴다. 여기서 지금 실행 파일의 것을 찍으면, 검토 도중
	// 도구가 올라갔을 때 실제로 검토한 근거가 아닌 판이 계획에 박힌다.
	if sf.RulesetVersion == "" {
		return nil, fmt.Errorf("this session records no ruleset_version — it was raised by an older build. " +
			"raise it again from the results (`pqcaton-decide open`) rather than stamping today's rules onto " +
			"a judgement made under rules we cannot name")
	}

	// **관문은 여기다.** finalized 아닌 세션에서는 계획 자체가 만들어지지 않는다.
	p, err := decision.BuildPlan(s, plan)
	if err != nil {
		return nil, err
	}
	if err := decision.AcceptForDeploy(p); err != nil {
		return nil, err
	}
	out, err := ToContract(p, picked, sf.RulesetVersion)
	if err != nil {
		return nil, err
	}
	return &Result{Plan: out, Decided: decidedOf(s), Batched: batched, Scope: p.Scope}, nil
}

// Pending — 무엇이 남았는지. 「안 된다」만 말하면 사람은 고칠 수 없다.
func Pending(sf Session) []decision.Missing {
	var out []decision.Missing
	if sf.Signature == "" {
		out = append(out, decision.Missing{Code: decision.MissingSignature})
	}
	for _, it := range sf.Items {
		if it.Mandatory && strings.TrimSpace(it.Conclusion) == "" &&
			strings.TrimSpace(sf.PolicyDecisions[it.Policy]) == "" {
			out = append(out, decision.Missing{
				Code: decision.MissingConclusion, Subject: it.ID, Detail: it.State,
			})
		}
	}
	return out
}

// RequireNode — **지어내지 않고 끊는다.** node 가 비면 겨눌 곳을 모르는 것이고, 빈 채로
// 내보내면 pqcota가 이름 없는 노드에 조치를 건다. v0.1.0 이 낸 세션 파일이 여기 걸린다.
func RequireNode(it Item) error {
	if it.Node == "" {
		return fmt.Errorf("item %s has no node — this is a v0.1.0 format session. "+
			"run `pqcaton-decide open` again, or give the screen -results and let it raise a new one", it.ID)
	}
	return nil
}

// RequireDecisions — 계획에 넣는 항목이 **검토자가 골랐어야 할 것을 다 골랐는지** 본다.
//
// pqcaton 은 승인·확정 계층이다. 여기서 비운 값을 기본값으로 채우면 사용자가 하지 않은 정책
// 결정을 도구가 대신 내리고 **그 결과에 승인 서명이 붙는다.** 서명은 조치의 내용을 덮으므로,
// 승인자는 자기가 고르지 않은 수준과 조치에 책임을 지게 된다.
//
// 상류(pqcota)도 빈칸을 이름으로 알리고 종료 상태 3으로 끝내지만, 그것은 **직접 쓴 계획을 위한
// 최후 안전장치**다. 여기서 통과시키고 거기서 걸리게 두면 계층의 역할이 뒤바뀐다.
//
// 필수의 범위는 상류의 완결성 기준을 따른다. 목표 알고리즘은 **config 로 배달하는 조치**에만
// 필요하다 — 그 조치의 조각에만 `Groups` 줄이 있고, 비면 배치해도 아무것도 켜지지 않는다.
// 포크 교체나 폐기에는 적을 자리가 없으므로 요구하지 않는다.
func RequireDecisions(it Item) error {
	if it.Level == "" {
		return fmt.Errorf("item %s: no deploy level — pick L1, L2 or L3. "+
			"an older session has none recorded, so it has to be chosen again", it.ID)
	}
	if _, err := levelOf(it.Level); err != nil {
		return fmt.Errorf("item %s: %w", it.ID, err)
	}
	if it.Kind == "" {
		return fmt.Errorf("item %s: no remediation kind — what to do is a review decision, not a default", it.ID)
	}
	if _, err := kindOf(it.Kind); err != nil {
		return fmt.Errorf("item %s: %w", it.ID, err)
	}
	switch it.Kind {
	case "REMEDIATION_KIND_CONFIG_ONLY", "REMEDIATION_KIND_PROVIDER_INJECT":
		if strings.TrimSpace(it.TargetAlgorithm) == "" {
			return fmt.Errorf("item %s: %s delivers its change through a config fragment, "+
				"so it needs a target_algorithm — without one the fragment turns nothing on", it.ID, it.Kind)
		}
	}
	// L3 는 활성화까지 간다. 상류가 무엇이 **일어나지 않는지** 알리는 자리를 여기서 먼저 막는다:
	// activate 가 없으면 조각이 놓이기만 하고 참조되지 않으며, restart 가 없으면 새 provider 가
	// 로드되지 않고, deactivate 가 없으면 롤백이 활성화를 되돌리지 못한다.
	if strings.ToUpper(it.Level) == "L3" {
		for _, h := range []struct {
			name, cmd, why string
		}{
			{"activation.activate", it.Activate, "the fragment is placed but never referenced"},
			{"activation.restart", it.Restart, "the new provider may never be loaded"},
			{"activation.deactivate", it.Deactivate, "rollback cannot undo the activation"},
		} {
			if strings.TrimSpace(h.cmd) == "" {
				return fmt.Errorf("item %s is L3 but has no %s — %s", it.ID, h.name, h.why)
			}
		}
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

// BasisOf — 이 판정이 무엇을 보고 내려졌나. 대조 상태와 신뢰도가 근거다.
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
func ToContract(p *decision.FinalizedPlan, items []Item, rulesetVersion string) (*provisioningv1.FinalizedPlan, error) {
	// 되짚을 근거 가운데 **도구가 아는 것은 도구가 채운다.** 계획 id와 확정 시각이 그렇다 —
	// 사람에게 물을 값이 아니고, 비어 있으면 상류가 「이 실행을 그것을 일으킨 계획에 묶을 수
	// 없다」로 알린다. 관측 스냅샷 id와 규칙 버전은 아직 이 함수에 오지 않아 비운다.
	out := &provisioningv1.FinalizedPlan{
		Id:                 "pqcaton:" + p.Scope + ":" + p.ApprovalSig[:min(8, len(p.ApprovalSig))],
		Scope:              p.Scope,
		Status:             provisioningv1.PlanStatus_PLAN_STATUS_FINALIZED,
		ApprovalSignatures: []string{p.ApprovalSig},
		FinalizedAt:        timestamppb.Now(),
		RulesetVersion:     rulesetVersion,
	}
	for i, it := range items {
		kind, err := kindOf(it.Kind)
		if err != nil {
			return nil, err
		}
		lvl, err := levelOf(p.Items[i].DeployAutomationLevel)
		if err != nil {
			return nil, err
		}
		out.Actions = append(out.Actions, &provisioningv1.RemediationAction{
			Id:              fmt.Sprintf("a%d", i+1),
			TargetNodeId:    p.Items[i].NodeID,
			CryptoRuntime:   runtimeOf(it.Runtime),
			Kind:            kind,
			AutomationLevel: lvl,
			TargetAlgorithm: it.TargetAlgorithm,
			ProviderChoice:  p.Items[i].ProviderChoice,
			FindingId:       it.FindingID,
			Activation:      hooksOf(it),
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

// kindOf — **모르는 값도 빈 값도 지어내지 않고 끊는다.** 계약의 통제 어휘라 오타가 조용히
// UNSPECIFIED로 떨어지면 그 조치는 아무것도 하지 않고, 빈 값을 기본 조치로 채우면 사용자가
// 하지 않은 결정에 승인 서명이 붙는다.
func kindOf(s string) (provisioningv1.RemediationKind, error) {
	if s == "" {
		return 0, fmt.Errorf("no remediation_kind — what to do is a review decision, not a default")
	}
	if v, ok := provisioningv1.RemediationKind_value[s]; ok && v != 0 {
		return provisioningv1.RemediationKind(v), nil
	}
	return 0, fmt.Errorf("unknown remediation_kind: %q — must be one of the contract's REMEDIATION_KIND_*", s)
}

// levelOf — 위임 수준. **모르는 값을 L2로 삼키지 않는다.** 전에는 미지정도 오타도 모두 L2가
// 되어, 사용자가 하지 않은 정책 결정을 이 도구가 대신 내리고 그 결과에 승인 서명이 붙었다.
// 확정 전에 RequireDecisions 가 막으므로 여기까지 빈 값이 오면 그것은 배선이 샌 것이다.
func levelOf(s string) (provisioningv1.DeployAutomationLevel, error) {
	switch strings.ToUpper(s) {
	case "L1":
		return provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L1_STAGE_ONLY, nil
	case "L2":
		return provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L2_STAGE_INSTALL, nil
	case "L3":
		return provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L3_FULL_AUTO, nil
	default:
		return 0, fmt.Errorf("unknown deploy level %q — it has to be L1, L2 or L3", s)
	}
}

// hooksOf — 사람이 적은 활성화 명령. **하나도 없으면 nil을 준다** — 빈 훅 묶음을 붙이면
// 상류가 「훅이 있는데 명령이 비었다」와 「훅 자체가 없다」를 가리지 못한다.
func hooksOf(it Item) *provisioningv1.ActivationHooks {
	if it.Pre == "" && it.Activate == "" && it.Deactivate == "" && it.Restart == "" {
		return nil
	}
	return &provisioningv1.ActivationHooks{
		Pre: it.Pre, Activate: it.Activate, Deactivate: it.Deactivate, Restart: it.Restart,
	}
}
