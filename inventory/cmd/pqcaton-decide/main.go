// Command pqcaton-decide — 리뷰 큐를 사람이 판정하고 확정하는 자리.
//
// 대조 엔진은 정답을 주지 않는다. 판정 대상을 구조화해 사람에게 넘기고, **확정은 사람이
// 한다**(규정서 §3.1). 그 「사람이 하는 자리」를 파일 왕복으로 연다.
//
//	pqcaton-decide open  <declaration.csv> [node] > session.json
//	  … 사람이 session.json 을 편집한다 (결론 · 승인자 · 서명)
//	pqcaton-decide close <session.json> > plan.json
//
// **파일이 곧 감사 기록이다.** 대화형으로 물어보면 무엇을 근거로 무엇을 정했는지가 화면에서
// 사라진다 — 편집한 파일이 그대로 남는 편이 낫다.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/randyinthedev-hash/pqcota/discovery/collectors/openssl"
	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	provisioningv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	"github.com/randyinthedev-hash/pqcota/pkg/inventory/declaration"
	"github.com/randyinthedev-hash/pqcota/pkg/kernel/registry"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

const usage = `usage:
  pqcaton-decide open  <declaration.csv> [node]   # 대조 → 리뷰 세션(초안)을 낸다
  pqcaton-decide close <session.json>             # 판정을 확인하고 확정 계획을 낸다

  open  이 낸 파일을 편집한 뒤 close 에 넣는다. 결론이 빈 필수 항목이 하나라도 있거나
        서명이 없으면 close 는 **확정하지 않고 왜 안 되는지 말한다**.`

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "open":
		node := "host://local"
		if len(os.Args) > 3 {
			node = os.Args[3]
		}
		err = open(os.Args[2], node)
	case "close":
		err = closeSession(os.Args[2])
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
}

// ── 파일 형식 ──────────────────────────────────────────────────────────────

// sessionFile — 사람이 편집하는 파일. **라이브러리 타입을 그대로 쓰지 않는다** —
// 편집하는 사람에게 필요한 것과 상태기계가 필요한 것이 다르다.
type sessionFile struct {
	Note      string     `json:"_읽는_법"`
	Scope     string     `json:"scope"`
	Reviewer  string     `json:"reviewer"`
	Signature string     `json:"signature"`
	Items     []itemFile `json:"items"`
	Autopass  []string   `json:"autopass_후보"`
}

type itemFile struct {
	ID    string  `json:"id"`
	State string  `json:"state"`
	Conf  float64 `json:"confidence"`
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
	// Config — provider 설정 조각. **도구가 지어내지 않는다** — 무엇을 넣을지는 계획을
	// 쓰는 사람이 정한다(상류 프로비저닝 설계와 같은 선).
	Config string `json:"config_artifact,omitempty"`
}

const note = "필수(mandatory) 항목의 conclusion 을 채우고, reviewer 와 signature 를 적은 뒤 " +
	"`pqcaton-decide close` 에 넣으세요. 확정 계획에 넣을 항목은 `확정_계획에_넣는다`를 true 로."

// ── open ───────────────────────────────────────────────────────────────────

func open(declPath, node string) error {
	dets, st := openssl.ScanHost(registry.DefaultForkSignatures)
	res := openssl.BuildResult(node, dets)
	snap, err := normalize.Normalize([]*discoveryv1.CollectionResult{res}, "snap-1", node, "ruleset-1", nil, nil)
	if err != nil {
		return fmt.Errorf("정규화: %w", err)
	}
	f, err := os.Open(declPath)
	if err != nil {
		return err
	}
	defer f.Close()
	decl, err := declaration.ImportCSV(f)
	if err != nil {
		return fmt.Errorf("선언 읽기: %w", err)
	}
	declared, err := reconcile.AssetsFromResults(decl)
	if err != nil {
		return fmt.Errorf("선언 자산: %w", err)
	}

	recs := reconcile.Reconcile(declared, reconcile.AssetsFromSnapshot(snap), reconcile.GapLayers(snap))
	autopass, review := reconcile.BuildReviewQueue(recs)

	sf := sessionFile{Note: note, Scope: node}
	for _, it := range review {
		sf.Items = append(sf.Items, itemFile{
			ID: key(it.Rec.Key), State: string(it.Rec.State), Conf: it.Rec.Confidence,
			Mandatory: it.Mandatory, Rescan: it.Rec.RescanCandidate,
		})
	}
	for _, a := range autopass {
		sf.Autopass = append(sf.Autopass, key(a.Key))
	}
	sort.Strings(sf.Autopass)

	fmt.Fprintf(os.Stderr, "스캔: 접근가능 %d · 거부 %d — 리뷰 %d개(필수 %d) · 자동통과 후보 %d개\n",
		st.Accessible, st.Denied, len(sf.Items), countMandatory(sf.Items), len(sf.Autopass))
	return write(sf)
}

// ── close ──────────────────────────────────────────────────────────────────

func closeSession(path string) error {
	var sf sessionFile
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &sf); err != nil {
		return fmt.Errorf("세션 파일: %w", err)
	}

	items := make([]decision.Item, 0, len(sf.Items))
	for _, it := range sf.Items {
		items = append(items, decision.Item{ID: it.ID, Mandatory: it.Mandatory})
	}
	s := decision.NewSession(sf.Scope, items)
	if err := s.StartReview(); err != nil {
		return err
	}
	for _, it := range sf.Items {
		if strings.TrimSpace(it.Conclusion) != "" {
			s.Decide(it.ID, it.Conclusion)
		}
	}
	if sf.Reviewer != "" || sf.Signature != "" {
		s.Sign(sf.Reviewer, sf.Signature)
	}

	// **여기가 이 리포의 최강 게이트다**(§3.7). 왜 안 되는지 말하고 멈춘다 —
	// 무엇을 더 채워야 하는지 모르면 사람은 파일을 고칠 수 없다.
	if err := s.Finalize(); err != nil {
		return fmt.Errorf("%w\n%s", err, pending(sf))
	}

	plan := make([]decision.PlanItem, 0)
	picked := make([]itemFile, 0)
	for _, it := range sf.Items {
		if !it.Plan {
			continue
		}
		node, runtime, _ := split(it.ID)
		lvl := it.Level
		if lvl == "" {
			lvl = "L2"
		}
		plan = append(plan, decision.PlanItem{
			NodeID: node, RemediationClass: it.Conclusion,
			DeployAutomationLevel: lvl,
			ProviderChoice:        decision.RouteProvider(runtime, it.FIPS),
		})
		picked = append(picked, it)
	}
	// **게이트는 여기다.** finalized 아닌 세션에서는 계획 자체가 만들어지지 않는다.
	p, err := decision.BuildPlan(s, plan)
	if err != nil {
		return err
	}
	if err := decision.AcceptForDeploy(p); err != nil {
		return err
	}

	// **계약 형식으로 낸다.** 내부 타입을 그대로 뱉으면 `pqcota-provision`이 못 읽는다 —
	// 그러면 「확정 계획이 프로비저닝의 입력」이라는 말이 코드로는 거짓이 된다.
	out := toContract(p, picked)
	raw, err2 := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(out)
	if err2 != nil {
		return err2
	}
	fmt.Fprintf(os.Stderr, "확정: %s · 조치 %d건 — `pqcota-provision plan.json` 의 입력입니다\n",
		p.Scope, len(out.GetActions()))
	_, err = os.Stdout.Write(append(raw, '\n'))
	return err
}

// pending — 무엇이 남았는지. 「안 된다」만 말하면 사람은 파일을 고칠 수 없다.
func pending(sf sessionFile) string {
	var b strings.Builder
	if sf.Signature == "" {
		b.WriteString("   · signature 가 비어 있습니다\n")
	}
	for _, it := range sf.Items {
		if it.Mandatory && strings.TrimSpace(it.Conclusion) == "" {
			fmt.Fprintf(&b, "   · 결론 없음: %s (%s)\n", it.ID, it.State)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// toContract — 확정 계획을 상류 계약(`provisioningv1.Plan`)으로 옮긴다.
//
// 어휘의 단일 출처는 계약이다(`pkg/inventory/reconcile/contract.go`와 같은 원칙) —
// 이 리포는 그 어휘로 말하고, 자기 형식을 새로 만들지 않는다.
func toContract(p *decision.FinalizedPlan, items []itemFile) *provisioningv1.FinalizedPlan {
	out := &provisioningv1.FinalizedPlan{
		Scope:              p.Scope,
		Status:             provisioningv1.PlanStatus_PLAN_STATUS_FINALIZED,
		ApprovalSignatures: []string{p.ApprovalSig},
	}
	for i, it := range items {
		_, runtime, _ := split(it.ID)
		out.Actions = append(out.Actions, &provisioningv1.RemediationAction{
			Id:              fmt.Sprintf("a%d", i+1),
			TargetNodeId:    p.Items[i].NodeID,
			CryptoRuntime:   runtimeOf(runtime),
			Kind:            kindOf(it.Kind),
			AutomationLevel: levelOf(p.Items[i].DeployAutomationLevel),
			ProviderChoice:  p.Items[i].ProviderChoice,
			ConfigArtifact:  it.Config,
			RollbackNote:    it.Conclusion,
		})
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
func kindOf(s string) provisioningv1.RemediationKind {
	if s == "" {
		return provisioningv1.RemediationKind_REMEDIATION_KIND_PROVIDER_INJECT
	}
	if v, ok := provisioningv1.RemediationKind_value[s]; ok && v != 0 {
		return provisioningv1.RemediationKind(v)
	}
	fmt.Fprintf(os.Stderr, "❌ 모르는 조치_종류: %q — 계약의 REMEDIATION_KIND_* 중 하나여야 합니다\n", s)
	os.Exit(1)
	return 0
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

func key(k reconcile.AssetKey) string {
	return k.NodeID + "/" + k.Runtime + "/" + k.Component
}

func split(id string) (node, runtime, component string) {
	p := strings.SplitN(id, "/", 3)
	for len(p) < 3 {
		p = append(p, "")
	}
	return p[0], p[1], p[2]
}

func countMandatory(items []itemFile) int {
	n := 0
	for _, it := range items {
		if it.Mandatory {
			n++
		}
	}
	return n
}

func write(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
