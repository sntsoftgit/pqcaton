// Command pqcaton-decide — 리뷰 큐를 사람이 판정하고 확정하는 자리.
//
// 대조 엔진은 정답을 주지 않는다. 판정 대상을 구조화해 사람에게 넘기고, **확정은 사람이
// 한다**(규정서 §3.1). 그 「사람이 하는 자리」를 파일 왕복으로 연다.
//
//	pqcaton-decide open  <declaration.csv> [node] > session.json
//	  … 사람이 session.json 을 편집한다 (결론 · 승인자 · 서명)
//	pqcaton-decide close <session.json> [-judgments 파일] [-org 이름] > plan.json
//	pqcaton-decide delta <judgments.jsonl> <declaration.csv> [node]
//
// **파일이 곧 감사 기록이다.** 대화형으로 물어보면 무엇을 근거로 무엇을 정했는지가 화면에서
// 사라진다 — 편집한 파일이 그대로 남는 편이 낫다.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/randyinthedev-hash/pqcota/discovery/collectors/openssl"
	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	provisioningv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	"github.com/randyinthedev-hash/pqcota/pkg/inventory/declaration"
	"github.com/randyinthedev-hash/pqcota/pkg/kernel/registry"
	"github.com/randyinthedev-hash/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

const usage = `usage:
  pqcaton-decide open  <declaration.csv> [node]
        대조 → 리뷰 세션(초안)을 낸다

  pqcaton-decide close <session.json> [-judgments <파일>] [-org <이름>]
        판정을 확인하고 확정 계획을 낸다. -judgments 를 주면 판정을 그 파일에
        append-only 로 남긴다 (감사 기록)

  pqcaton-decide delta <judgments.jsonl> <declaration.csv> [node] [-org <이름>]
        쌓인 판정을 지금 관측과 대조해 **근거가 바뀐 것만** 골라 낸다

  open 이 낸 파일을 편집한 뒤 close 에 넣는다. 결론이 빈 필수 항목이 하나라도 있거나
  서명이 없으면 close 는 확정하지 않고 왜 안 되는지 말한다.`

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	sub, args := os.Args[1], os.Args[2:]

	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	judgments := fs.String("judgments", "", "판정을 남길 파일(JSONL, append-only)")
	orgName := fs.String("org", "local", "판정을 묶을 조직")
	// 위치 인자를 먼저 걷고 나머지를 플래그로 넘긴다 - 순서를 사람이 외우지 않게.
	var pos []string
	var flags []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flags = append(flags, args[i:]...)
			break
		}
		pos = append(pos, args[i])
	}
	if err := fs.Parse(flags); err != nil {
		os.Exit(2)
	}
	need := func(n int) {
		if len(pos) < n {
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
	}

	var err error
	switch sub {
	case "open":
		need(1)
		node := "host://local"
		if len(pos) > 1 {
			node = pos[1]
		}
		err = open(pos[0], node)
	case "close":
		need(1)
		err = closeSession(pos[0], *judgments, *orgName)
	case "delta":
		need(2)
		node := "host://local"
		if len(pos) > 2 {
			node = pos[2]
		}
		err = delta(pos[0], pos[1], node, *orgName)
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
	Note      string `json:"_읽는_법"`
	Scope     string `json:"scope"`
	Reviewer  string `json:"reviewer"`
	Signature string `json:"signature"`
	// PolicyDecisions - 정책 하나에 결론 하나. **이것이 기본 단위다**(§3.4) - 수천 대를
	// 한 건씩 보는 리뷰는 끝나지 않는다. 개별 항목의 conclusion 은 예외를 위한 자리다.
	PolicyDecisions map[string]string `json:"정책_판정"`
	Items           []itemFile        `json:"items"`
	Autopass        []string          `json:"autopass_후보"`
}

type itemFile struct {
	ID string `json:"id"`
	// Node · Runtime — **id 에서 되찾지 않고 여기 적어 둔다.** 노드가 `host://local` 같은
	// URI면 `/` 로 쪼개 복원할 수 없다 - 조치가 엉뚱한 노드를 겨누고, 런타임이 비어
	// 기본값으로 조용히 떨어진다. 대조할 때 이미 알던 값이므로 그대로 들고 간다.
	Node    string `json:"node"`
	Runtime string `json:"runtime"`
	// Policy - 같은 정책의 항목은 한 번에 판정한다(§3.4). 런타임과 컴포넌트에서 만든다 -
	// 버전 해시가 붙은 것들이 같은 묶음으로 모인다.
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
	// Config — provider 설정 조각. **도구가 지어내지 않는다** — 무엇을 넣을지는 계획을
	// 쓰는 사람이 정한다(상류 프로비저닝 설계와 같은 선).
	Config string `json:"config_artifact,omitempty"`
}

const note = "정책_판정 에 정책별 결론을 적으면 같은 정책의 항목이 한 번에 판정됩니다(권장). " +
	"예외만 항목의 conclusion 으로 따로 적습니다. reviewer 와 signature 를 채운 뒤 " +
	"`pqcaton-decide close` 에 넣으세요. 확정 계획에 넣을 항목은 `확정_계획에_넣는다`를 true 로."

// ── open ───────────────────────────────────────────────────────────────────

// open - 대조해서 리뷰 세션을 낸다.
func open(declPath, node string) error {
	sf, err := session(declPath, node)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "리뷰 %d개(필수 %d) · 정책 %d개 · 자동통과 후보 %d개\n",
		len(sf.Items), countMandatory(sf.Items), len(sf.PolicyDecisions), len(sf.Autopass))
	return write(sf)
}

// session - 지금 이 머신을 관측해 선언과 대조하고 리뷰 세션을 만든다.
//
// **open 과 delta 가 같은 함수를 쓴다.** 근거 해시가 이 결과에서 나오므로, 두 곳이 갈리면
// 델타 리뷰가 "바뀌지 않은 것"을 바뀌었다고 부른다.
func session(declPath, node string) (sessionFile, error) {
	var sf sessionFile
	dets, st := openssl.ScanHost(registry.DefaultForkSignatures)
	res := openssl.BuildResult(node, dets)
	snap, err := normalize.Normalize([]*discoveryv1.CollectionResult{res}, "snap-1", node, "ruleset-1", nil, nil)
	if err != nil {
		return sf, fmt.Errorf("정규화: %w", err)
	}
	f, err := os.Open(declPath)
	if err != nil {
		return sf, err
	}
	defer f.Close()
	decl, err := declaration.ImportCSV(f)
	if err != nil {
		return sf, fmt.Errorf("선언 읽기: %w", err)
	}
	declared, err := reconcile.AssetsFromResults(decl)
	if err != nil {
		return sf, fmt.Errorf("선언 자산: %w", err)
	}

	recs := reconcile.Reconcile(declared, reconcile.AssetsFromSnapshot(snap), reconcile.GapLayers(snap))
	autopass, review := reconcile.BuildReviewQueue(recs)

	sf = sessionFile{Note: note, Scope: node, PolicyDecisions: map[string]string{}}
	for _, it := range review {
		pol := policyOf(it.Rec.Key)
		sf.Items = append(sf.Items, itemFile{
			ID: key(it.Rec.Key), Policy: pol,
			Node: it.Rec.Key.NodeID, Runtime: it.Rec.Key.Runtime,
			State: string(it.Rec.State), Conf: it.Rec.Confidence,
			Mandatory: it.Mandatory, Rescan: it.Rec.RescanCandidate,
		})
		if _, ok := sf.PolicyDecisions[pol]; !ok {
			sf.PolicyDecisions[pol] = "" // 사람이 채울 자리를 미리 열어 둔다
		}
	}
	for _, a := range autopass {
		sf.Autopass = append(sf.Autopass, key(a.Key))
	}
	sort.Strings(sf.Autopass)
	fmt.Fprintf(os.Stderr, "스캔: 접근가능 %d · 거부 %d\n", st.Accessible, st.Denied)
	return sf, nil
}

// ── close ──────────────────────────────────────────────────────────────────

func closeSession(path, judgmentPath, orgName string) error {
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
		items = append(items, decision.Item{ID: it.ID, Policy: it.Policy, Mandatory: it.Mandatory})
	}
	s := decision.NewSession(sf.Scope, items)
	if err := s.StartReview(); err != nil {
		return err
	}
	// **정책이 먼저다**(§3.4). 개별 결론은 그 뒤에 얹혀 예외를 만든다.
	for pol, c := range sf.PolicyDecisions {
		if strings.TrimSpace(c) != "" {
			if n := s.DecidePolicy(pol, c); n > 0 {
				fmt.Fprintf(os.Stderr, "정책 %s: %d개 일괄 판정\n", pol, n)
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
		if err := requireNode(it); err != nil {
			return err
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
	// **게이트를 지난 뒤에만 남긴다.** 확정되지 않은 것을 판정 이력에 쌓으면 그 기록이
	// "누가 무엇을 확정했나"를 더는 답하지 못한다.
	if judgmentPath != "" {
		n, err := saveJudgments(judgmentPath, orgName, sf, decidedOf(s))
		if err != nil {
			return fmt.Errorf("판정 기록: %w", err)
		}
		fmt.Fprintf(os.Stderr, "판정 %d건을 %s 에 남겼습니다 (append-only)\n", n, judgmentPath)
	}
	fmt.Fprintf(os.Stderr, "확정: %s · 조치 %d건 — `pqcota-provision plan.json` 의 입력입니다\n",
		p.Scope, len(out.GetActions()))
	_, err = os.Stdout.Write(append(raw, '\n'))
	return err
}

// decidedOf — 세션에서 실제로 판정된 항목의 결론. 정책 일괄로 붙은 것도 여기 들어온다.
func decidedOf(s *decision.Session) map[string]string {
	out := map[string]string{}
	for _, it := range s.Items {
		if it.Decided {
			out[it.ID] = it.Conclusion
		}
	}
	return out
}

// saveJudgments — 판정을 append-only 로 남긴다.
//
// **근거 해시를 함께 적는다.** 그것이 없으면 나중에 「근거가 바뀌었나」를 물을 수 없고,
// 델타 리뷰가 성립하지 않는다(§3.6).
func saveJudgments(path, orgName string, sf sessionFile, decided map[string]string) (int, error) {
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
			ID:         fmt.Sprintf("%s@%d", it.ID, now),
			Subject:    it.ID,
			Conclusion: c,
			Reviewer:   sf.Reviewer,
			Signature:  sf.Signature,
			BasisHash:  basisOf(it),
			Confidence: it.Conf,
			DecidedAt:  now,
		}
		if err := store.Save(j); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// basisOf — 이 판정이 무엇을 보고 내려졌나. 대조 상태와 확신도가 근거다.
//
// **관측이 바뀌면 이 값이 바뀐다** — 그때 델타 리뷰가 걸린다. 반대로 관측이 그대로면
// 몇 번을 다시 돌려도 걸리지 않는다(§3.6, IC-D2/D3).
func basisOf(it itemFile) string {
	return decision.HashBasis(
		"state="+it.State,
		fmt.Sprintf("conf=%.2f", it.Conf),
		"policy="+it.Policy,
	)
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
		out.Actions = append(out.Actions, &provisioningv1.RemediationAction{
			Id:              fmt.Sprintf("a%d", i+1),
			TargetNodeId:    p.Items[i].NodeID,
			CryptoRuntime:   runtimeOf(it.Runtime),
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

// delta - 쌓인 판정을 지금 관측과 대조해 **근거가 바뀐 것만** 고른다.
//
// 전면 재리뷰가 아니다. 재관측할 때마다 전부 다시 보게 하면 아무도 안 본다 - 바뀐 것만
// 걸어야 그 큐가 읽힌다(§3.6).
func delta(judgmentPath, declPath, node, orgName string) error {
	store, err := decision.NewFileJudgmentStore(org.ID(orgName), judgmentPath)
	if err != nil {
		return err
	}
	saved, err := store.All()
	if err != nil {
		return err
	}
	if len(saved) == 0 {
		return fmt.Errorf("%s 에 판정이 없다 - close 를 -judgments 와 함께 돌렸는가", judgmentPath)
	}
	prior := make([]decision.Judgment, 0, len(saved))
	for _, j := range saved {
		prior = append(prior, *j)
	}
	prior = decision.LatestPerSubject(prior) // append-only 로그에서 대상별 최신만

	// 지금 관측으로 근거를 다시 만든다. open 이 쓰는 것과 같은 경로여야 값이 맞는다.
	sf, err := session(declPath, node)
	if err != nil {
		return err
	}
	basis := make(map[string]string, len(sf.Items))
	for _, it := range sf.Items {
		basis[it.ID] = basisOf(it)
	}

	out := decision.DeltaReview(prior, basis)
	// **빈 결과도 배열이다.** nil 로 두면 `null` 이 찍혀 받는 쪽이 길이를 셀 수 없다.
	need := make([]decision.Judgment, 0)
	for _, j := range out {
		if j.NeedsReReview {
			need = append(need, j)
		}
	}
	fmt.Fprintf(os.Stderr, "판정 %d건 중 근거가 바뀐 것 %d건\n", len(out), len(need))
	if len(need) == 0 {
		fmt.Fprintln(os.Stderr, "다시 볼 것이 없습니다 - 관측이 그대로입니다")
	}
	return write(need)
}

// policyOf - 같은 정책으로 묶는 열쇠. 런타임과 컴포넌트 이름에서 만든다.
//
// 컴포넌트에 붙는 **버전 해시만** 뗀다(`libssl-e2f2d68a` → `libssl`). 같은 라이브러리의 여러
// 판이 한 묶음이 되는 것이 정책 단위 리뷰가 뜻하는 것이다(§3.4).
//
// **해시처럼 생긴 것만 뗀다.** 길이로만 자르면 `jca-provider-chain`의 `-chain`까지 떼어
// 이름이 다른 컴포넌트가 한 정책으로 묶인다.
func policyOf(k reconcile.AssetKey) string {
	c := k.Component
	if i := strings.LastIndex(c, "-"); i > 0 && isHex(c[i+1:]) {
		c = c[:i]
	}
	return k.Runtime + "/" + c
}

// isHex - 버전 해시로 볼 만한가. 짧은 것은 이름의 일부일 수 있어 8자 이상만 본다.
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

// requireNode — **지어내지 않고 끊는다.** node 가 비면 겨눌 곳을 모르는 것이고, 빈 채로
// 내보내면 상류가 이름 없는 노드에 조치를 건다. v0.1.0 이 낸 세션 파일이 여기 걸린다.
func requireNode(it itemFile) error {
	if it.Node == "" {
		return fmt.Errorf("항목 %s 에 node 가 없다 - `pqcaton-decide open` 을 다시 돌려 세션을 새로 받아라", it.ID)
	}
	return nil
}

func key(k reconcile.AssetKey) string {
	return k.NodeID + "/" + k.Runtime + "/" + k.Component
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
