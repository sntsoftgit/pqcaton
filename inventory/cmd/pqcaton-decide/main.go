// Command pqcaton-decide — 리뷰 큐를 사람이 판정하고 확정하는 자리.
//
// 대조 엔진은 정답을 주지 않는다. 판정 대상을 구조화해 사람에게 넘기고, **확정은 사람이
// 한다**(규정서 §3.1). 그 「사람이 하는 자리」를 파일 왕복으로 연다.
//
//	pqcaton-decide open  <declaration.json> -results <results-dir> [-org 이름] > session.json
//	pqcaton-decide open  <declaration.csv> [이-기계에-붙일-이름] [-org 이름] > session.json
//	  … 사람이 session.json 을 편집한다 (결론 · 승인자 · 서명)
//	pqcaton-decide close <session.json> [-judgments 파일] [-org 이름] > plan.json
//	pqcaton-decide delta <judgments.jsonl> <declaration.csv> [node]
//
// **파일이 곧 감사 기록이다.** 대화형으로 물어보면 무엇을 근거로 무엇을 정했는지가 화면에서
// 사라진다 — 편집한 파일이 그대로 남는 편이 낫다. 화면(`pqcaton-ui`)도 이 파일을 읽고 쓴다.
//
// 파일 형식과 확정 게이트는 `pkg/inventory/review` 에 있다 — 명령과 화면이 같은 것을 쓴다.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/randyinthedev-hash/pqcota/pkg/inventory/declaration"
	"github.com/randyinthedev-hash/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/localscan"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

const usage = `usage:
  pqcaton-decide open  <declaration.json> -results <results-dir> [-org <이름>]
        **pqcota 가 모은 관측**으로 선언과 대조하고 리뷰 세션(초안)을 낸다.
        여러 노드를 다루는 길이고, 대조 화면이 보는 것과 같은 계산이다

  pqcaton-decide open  <declaration.csv> [이-기계에-붙일-이름] [-org <이름>]
        **이 기계를 스캔해** 선언과 대조하고 리뷰 세션(초안)을 낸다(체험용 지름길).

  두 경로 모두 -view 를 주면 대조 결과를 표로 함께 보여 준다.
        두 번째 인자는 결과에 붙이는 **이름표**이지 관측 대상이 아니다 -
        다른 노드를 관측하려면 pqcota 의 collector 를 그 노드에서 돌리고
        pqcaton-report 로 모은다. /proc 이 없으면(비-리눅스) 끊는다

  pqcaton-decide close <session.json> [-judgments <파일>] [-org <이름>]
        판정을 확인하고 확정 계획을 낸다. -judgments 를 주면 판정을 그 파일에
        append-only 로 남긴다 (감사 기록)

  pqcaton-decide delta <judgments.jsonl> <declaration> [node] [-org <이름>] [-results <dir>]
        쌓인 판정을 지금 관측과 대조해 **근거가 바뀐 것만** 골라 낸다.
        open 과 같은 입력을 주어야 근거가 맞는다

  open 이 낸 파일을 편집한 뒤 close 에 넣는다. 결론이 빈 필수 항목이 하나라도 있거나
  서명이 없으면 close 는 확정하지 않고 왜 안 되는지 말한다.

  화면으로 채우려면 pqcaton-ui 를 쓴다 — 같은 파일, 같은 게이트다.`

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	sub, args := os.Args[1], os.Args[2:]

	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	judgments := fs.String("judgments", "", "판정을 남길 파일(JSONL, append-only)")
	results := fs.String("results", "", "관측 결과 디렉터리. 주면 **이 기계를 스캔하지 않고** 그 결과로 대조한다(선언은 JSON)")
	view := fs.Bool("view", false, "대조 결과를 표로 함께 보여 준다(세션은 그대로 표준출력으로 나간다)")
	orgName := fs.String("org", "local", "대조와 판정을 묶을 조직")
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
		node := localscan.DefaultNode
		if len(pos) > 1 {
			node = pos[1]
		}
		err = open(pos[0], node, *orgName, *results, *view)
	case "close":
		need(1)
		err = closeSession(pos[0], *judgments, *orgName)
	case "delta":
		need(2)
		node := localscan.DefaultNode
		if len(pos) > 2 {
			node = pos[2]
		}
		err = delta(pos[0], pos[1], node, *orgName, *results)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
}

// ── open ───────────────────────────────────────────────────────────────────

// open - 대조해서 리뷰 세션을 낸다.
func open(declPath, node, orgName, resultsDir string, view bool) error {
	sf, recs, err := session(declPath, node, orgName, resultsDir)
	if err != nil {
		return err
	}
	// **표는 표준오류로 낸다.** 세션 JSON 이 표준출력으로 나가므로 섞이면 파이프가 깨진다.
	if view {
		fmt.Fprint(os.Stderr, "\n", reconcile.RenderView(recs), "\n")
	}
	fmt.Fprintf(os.Stderr, "리뷰 %d개(필수 %d) · 정책 %d개 · 자동통과 후보 %d개\n",
		len(sf.Items), countMandatory(sf.Items), len(sf.PolicyDecisions), len(sf.Autopass))
	return write(sf)
}

// session - 지금 이 머신을 관측해 선언과 대조하고 리뷰 세션을 만든다.
//
// **open 과 delta 가 같은 함수를 쓴다.** 근거 해시가 이 결과에서 나오므로, 두 곳이 갈리면
// 델타 리뷰가 "바뀌지 않은 것"을 바뀌었다고 부른다.
// session — 리뷰 세션과 **그것을 만든 대조 결과**를 함께 낸다.
//
// 세션에는 리뷰 대상만 담기므로(자동통과는 이름만), `-view` 가 대조 전체를 표로 보이려면
// 원본이 필요하다.
func session(declPath, node, orgName, resultsDir string) (review.Session, []reconcile.Reconciled, error) {
	if resultsDir != "" {
		return sessionFromResults(declPath, orgName, resultsDir)
	}
	var sf review.Session
	// **대조도 조직에 묶인다.** 엔진이 조직을 들고, 다른 조직의 자산이 섞이면 대조하지
	// 않고 끊는다 - 섞인 채로 돌면 오류가 아니라 그럴듯한 결과가 나온다.
	eng, err := reconcile.For(org.ID(orgName))
	if err != nil {
		return sf, nil, err
	}
	// **이 기계를 스캔한다.** 노드 이름은 결과에 붙이는 이름표일 뿐이고, /proc 을 못 열면
	// 끊는다 - 그 상태로 대조하면 「못 본 것」이 「없는 것」으로 읽힌다.
	scan, err := localscan.Scan(node)
	if err != nil {
		return sf, nil, err
	}
	for _, w := range scan.Warnings {
		fmt.Fprintln(os.Stderr, "⚠", w)
	}
	snap := scan.Snapshot
	f, err := os.Open(declPath)
	if err != nil {
		return sf, nil, err
	}
	defer f.Close()
	decl, err := declaration.ImportCSV(f)
	if err != nil {
		return sf, nil, fmt.Errorf("선언 읽기: %w", err)
	}
	declared, err := eng.AssetsFromResults(decl)
	if err != nil {
		return sf, nil, fmt.Errorf("선언 자산: %w", err)
	}

	recs, err := eng.Reconcile(declared, eng.AssetsFromSnapshot(snap), reconcile.GapLayers(snap))
	if err != nil {
		return sf, nil, err
	}
	autopass, queue := reconcile.BuildReviewQueue(recs)

	sf = review.Session{Note: review.Note, Scope: node, PolicyDecisions: map[string]string{}}
	for _, it := range queue {
		pol := review.PolicyOf(it.Rec.Key)
		sf.Items = append(sf.Items, review.Item{
			ID: review.Key(it.Rec.Key), Policy: pol,
			Node: it.Rec.Key.NodeID, Runtime: it.Rec.Key.Runtime,
			State: string(it.Rec.State), Conf: it.Rec.Confidence,
			Mandatory: it.Mandatory, Rescan: it.Rec.RescanCandidate,
		})
		if _, ok := sf.PolicyDecisions[pol]; !ok {
			sf.PolicyDecisions[pol] = "" // 사람이 채울 자리를 미리 열어 둔다
		}
	}
	for _, a := range autopass {
		sf.Autopass = append(sf.Autopass, review.Key(a.Key))
	}
	sort.Strings(sf.Autopass)
	fmt.Fprintf(os.Stderr, "스캔: 접근가능 %d · 거부 %d (이 기계)\n", scan.Accessible, scan.Denied)
	return sf, recs, nil
}

// sessionFromResults — **pqcota 가 모은 관측**으로 리뷰 세션을 만든다.
//
// 이것이 주경로다. 여러 노드를 관측해 놓고도 판정할 수 없으면 「관측을 판정으로 잇는다」가
// 코드로는 거짓이 된다 — 확정 계획을 낼 수 있는 것이 명령을 돌린 그 기계 하나뿐이었다.
//
// **대조는 `report` 가 한다.** 대조 화면(`pqcaton-ui`)이 보는 것과 같은 계산이라, 화면에서
// 본 shadow 가 리뷰 큐에 그대로 올라온다 — 두 벌이면 사람이 본 것과 판정할 것이 갈린다.
func sessionFromResults(declPath, orgName, resultsDir string) (review.Session, []reconcile.Reconciled, error) {
	var sf review.Session
	d, err := decl.Load(declPath)
	if err != nil {
		return sf, nil, err
	}
	// **선언이 조직을 말한다.** -org 를 따로 주지 않았으면 선언의 것을 쓴다.
	if orgName == "" || orgName == decl.DefaultOrg {
		orgName = d.OrgOrDefault()
	}
	if d.Org != "" && d.Org != orgName {
		return sf, nil, fmt.Errorf("선언은 조직 %q의 것인데 %q로 대조하려 한다", d.Org, orgName)
	}
	// **앞뒤가 안 맞으면 말한다.** 노드↔IP 가 틀리면 CONFIRMED 여야 할 것이 shadow 로 올라온다.
	if p := decl.Check(d); len(p) > 0 {
		fmt.Fprintf(os.Stderr, "⚠ 선언에 맞지 않는 자리 %d곳 — `pqcaton-ui -decl` 로 보십시오\n", len(p))
	}

	r, err := report.Build(resultsDir, d)
	if err != nil {
		return sf, nil, err
	}
	for _, sk := range r.Skipped {
		fmt.Fprintln(os.Stderr, "   건너뜀(읽을 수 없음):", sk)
	}
	autopass, queue := reconcile.BuildReviewQueue(r.Assets)

	sf = review.Session{Note: review.Note, Scope: "org://" + orgName,
		PolicyDecisions: map[string]string{}}
	for _, it := range queue {
		pol := review.PolicyOf(it.Rec.Key)
		sf.Items = append(sf.Items, review.Item{
			ID: review.Key(it.Rec.Key), Policy: pol,
			Node: it.Rec.Key.NodeID, Runtime: it.Rec.Key.Runtime,
			State: string(it.Rec.State), Conf: it.Rec.Confidence,
			Mandatory: it.Mandatory, Rescan: it.Rec.RescanCandidate,
		})
		if _, ok := sf.PolicyDecisions[pol]; !ok {
			sf.PolicyDecisions[pol] = ""
		}
	}
	for _, a := range autopass {
		sf.Autopass = append(sf.Autopass, review.Key(a.Key))
	}
	sort.Strings(sf.Autopass)
	c, u, un := r.Counts()
	fmt.Fprintf(os.Stderr, "관측 %d노드 · 대조 CONFIRMED %d · UNDECLARED %d · UNOBSERVED %d\n",
		len(r.SeenBy), c, u, un)
	return sf, r.Assets, nil
}

// ── close ──────────────────────────────────────────────────────────────────

func closeSession(path, judgmentPath, orgName string) error {
	sf, err := review.Load(path)
	if err != nil {
		return err
	}
	// **게이트는 review.Finalize 하나다.** 화면도 같은 것을 쓴다 — 두 벌이면 언젠가
	// 한쪽만 고쳐지고, 그날 화면과 명령의 확정이 갈린다.
	res, err := review.Finalize(sf)
	if err != nil {
		return err
	}
	for pol, n := range res.Batched {
		fmt.Fprintf(os.Stderr, "정책 %s: %d개 일괄 판정\n", pol, n)
	}

	raw, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(res.Plan)
	if err != nil {
		return err
	}
	// **게이트를 지난 뒤에만 남긴다.** 확정되지 않은 것을 판정 이력에 쌓으면 그 기록이
	// "누가 무엇을 확정했나"를 더는 답하지 못한다.
	if judgmentPath != "" {
		n, err := review.SaveJudgments(judgmentPath, orgName, sf, res.Decided)
		if err != nil {
			return fmt.Errorf("판정 기록: %w", err)
		}
		fmt.Fprintf(os.Stderr, "판정 %d건을 %s 에 남겼습니다 (append-only)\n", n, judgmentPath)
	}
	fmt.Fprintf(os.Stderr, "확정: %s · 조치 %d건 — `pqcota-provision plan.json` 의 입력입니다\n",
		res.Scope, len(res.Plan.GetActions()))
	_, err = os.Stdout.Write(append(raw, '\n'))
	return err
}

// ── delta ──────────────────────────────────────────────────────────────────

// delta - 쌓인 판정을 지금 관측과 대조해 **근거가 바뀐 것만** 고른다.
//
// 전면 재리뷰가 아니다. 재관측할 때마다 전부 다시 보게 하면 아무도 안 본다 - 바뀐 것만
// 걸어야 그 큐가 읽힌다(§3.6).
func delta(judgmentPath, declPath, node, orgName, resultsDir string) error {
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
	sf, _, err := session(declPath, node, orgName, resultsDir)
	if err != nil {
		return err
	}
	basis := make(map[string]string, len(sf.Items))
	for _, it := range sf.Items {
		basis[it.ID] = review.BasisOf(it)
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

// ── 공통 ───────────────────────────────────────────────────────────────────

func countMandatory(items []review.Item) int {
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
