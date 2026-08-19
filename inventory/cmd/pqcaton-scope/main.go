// Command pqcaton-scope — 자산 스코프 정책을 사람이 승인하고 배포하는 자리(설계 §1.6).
//
// **규칙을 다시 만들지 않는다.** 형식과 집행은 pqcota의 `scope.AssetPolicy`가 갖고, 여기는
// 그 위의 거버넌스만 갖는다 — 계층 상속, 변경 승인, 감사 기록, 제외분 재검토.
//
//	pqcaton-scope open   <계층.csv>... [-base 현재.csv] [-org 이름] > session.json
//	  … 사람이 session.json 을 편집한다 (결론 · 승인자 · 서명)
//	pqcaton-scope close  <session.json> [-judgments 파일] [-org 이름] > asset-scope.csv
//	pqcaton-scope review <정책.csv> <results-dir> [-judgments 파일] [-org 이름]
//
// 계층은 준 순서대로 겹친다 — 조직, 환경, 노드군 순으로 준다. 같은 자산에 규칙이 여럿
// 걸리면 **뒤 계층의 것이 적용된다.**
//
// **파일이 곧 감사 기록이다.** `pqcaton-decide` 와 같은 왕복이라 조작을 따로 외울 것이 없다.
// 파일 형식과 확정 게이트는 `pkg/inventory/scope` 에 있다 — 화면(`pqcaton-ui`)이 같은 것을 쓴다.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

const usage = `usage:
  pqcaton-scope open   <계층.csv>... [-base <현재.csv>] [-org <이름>]
        계층을 겹쳐 **바뀐 규칙만** 골라 리뷰 세션(초안)을 낸다.
        계층은 준 순서대로 겹칩니다 - 조직 · 환경 · 노드군 순으로 주십시오.
        같은 자산에 규칙이 여럿 걸리면 뒤 계층의 것이 적용됩니다

  pqcaton-scope close  <session.json> [-judgments <파일>] [-org <이름>]
        승인을 확인하고 **pqcota의 집행기가 읽는 CSV**를 낸다.
        exclude 추가는 결론이 없으면 확정하지 않는다 - 「안 본다」는 감사 대상입니다

  pqcaton-scope review <정책.csv> <results-dir> [-judgments <파일>] [-org <이름>] [-ttl <일>]
        정책이 뺀 자산을 **이름으로** 내고, 승인이 없거나 오래된 것만 다시 올린다.
        제외는 영구 면제가 아닙니다

  화면으로 다루려면 pqcaton-ui 를 쓴다 — 같은 파일, 같은 게이트다.`

// 제외 승인의 기본 유효기간. 넘으면 「빼둔 사이 달라졌을 수 있다」로 다시 올린다.
const defaultTTLDays = 180

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	sub, args := os.Args[1], os.Args[2:]

	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	base := fs.String("base", "", "지금 쓰는 정책 CSV. 주면 바뀐 규칙만 리뷰에 올린다")
	judgments := fs.String("judgments", "", "판정을 남길 파일(JSONL, append-only)")
	orgName := fs.String("org", "local", "정책과 판정을 묶을 조직")
	ttlDays := fs.Int("ttl", defaultTTLDays, "제외 승인의 유효기간(일)")

	// 위치 인자를 먼저 걷고 나머지를 플래그로 넘긴다 - pqcaton-decide 와 같은 규칙이다.
	var pos, flags []string
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

	var err error
	switch sub {
	case "open":
		if len(pos) < 1 {
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
		err = open(pos, *base, *orgName)
	case "close":
		if len(pos) < 1 {
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
		err = closeSession(pos[0], *judgments, *orgName)
	case "review":
		if len(pos) < 2 {
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
		err = review(pos[0], pos[1], *judgments, *orgName, int64(*ttlDays)*24*3600)
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

func open(layerPaths []string, basePath, orgName string) error {
	if _, err := org.Parse(orgName); err != nil {
		return err
	}
	var layers []scope.Layer
	for _, p := range layerPaths {
		rules, err := scope.LoadPolicyFile(p)
		if err != nil {
			return err
		}
		layers = append(layers, scope.Layer{Name: layerName(p), Rules: rules.Rules})
	}
	var basePolicy *kscope.AssetPolicy
	if basePath != "" {
		var err error
		if basePolicy, err = scope.LoadPolicyFile(basePath); err != nil {
			return err
		}
	}

	sf := scope.NewSession(layers, basePolicy, orgName)
	fmt.Fprintf(os.Stderr, "계층 %d개 · 정책 규칙 %d개 · 변경 %d건(근거 필수 %d)\n",
		len(layers), len(sf.Merged), len(sf.Changes), sf.AuditedCount())
	if len(sf.Changes) == 0 {
		fmt.Fprintln(os.Stderr, "바뀐 규칙이 없습니다 - 승인할 것이 없습니다")
	}
	return write(sf)
}

// ── close ──────────────────────────────────────────────────────────────────

func closeSession(path, judgmentPath, orgName string) error {
	sf, err := scope.LoadSession(path)
	if err != nil {
		return err
	}
	// **게이트는 scope.Finalize 하나다.** 화면도 같은 것을 쓴다 — 두 벌이면 언젠가 한쪽만
	// 고쳐지고, 그날 화면과 명령의 확정이 갈린다.
	res, err := scope.Finalize(sf, orgName)
	if err != nil {
		return err
	}
	for layer, n := range res.Batched {
		fmt.Fprintf(os.Stderr, "계층 %s: %d건 일괄 판정\n", layer, n)
	}
	// **게이트를 지난 뒤에만 남긴다.**
	if judgmentPath != "" {
		n, err := scope.SaveJudgments(judgmentPath, orgName, sf, res.Decided)
		if err != nil {
			return fmt.Errorf("판정 기록: %w", err)
		}
		fmt.Fprintf(os.Stderr, "판정 %d건을 %s 에 남겼습니다 (append-only)\n", n, judgmentPath)
	}
	fmt.Fprintf(os.Stderr, "확정: 규칙 %d개 — `pqcota-ingest -scope-assets` 의 입력입니다\n",
		len(res.Policy.Rules))
	return scope.WriteCSV(os.Stdout, res.Policy)
}

// ── review ─────────────────────────────────────────────────────────────────

func review(policyPath, dir, judgmentPath, orgName string, ttl int64) error {
	p, err := scope.LoadPolicyFile(policyPath)
	if err != nil {
		return err
	}
	results, err := loadResults(dir)
	if err != nil {
		return err
	}

	var ex []scope.Excluded
	for _, res := range results {
		node := res.GetEnvelope().GetTargetNodeId()
		fs, err := normalize.DeriveFindings(res, "", "")
		if err != nil {
			return fmt.Errorf("%s: %w", node, err)
		}
		ex = append(ex, scope.ExcludedFrom(p, node, fs)...)
	}
	sort.Slice(ex, func(i, j int) bool { return ex[i].Subject() < ex[j].Subject() })

	var prior []decision.Judgment
	if judgmentPath != "" {
		store, err := decision.NewFileJudgmentStore(org.ID(orgName), judgmentPath)
		if err != nil {
			return err
		}
		saved, err := store.All()
		if err != nil {
			return err
		}
		for _, j := range saved {
			prior = append(prior, *j)
		}
	}

	need := scope.Review(ex, prior, time.Now().Unix(), ttl)

	fmt.Fprintf(os.Stderr, "결과 %d개 · 정책이 뺀 자산 %d건 · 다시 볼 것 %d건\n",
		len(results), len(ex), len(need))
	if len(ex) > 0 && len(need) == 0 {
		fmt.Fprintln(os.Stderr, "뺀 것은 있으나 전부 승인이 살아 있습니다")
	}
	// **뺀 것을 이름으로 낸다.** pqcota는 수만 세지만, 사고 뒤에 답하려면 이름이 있어야 한다.
	for _, e := range ex {
		fmt.Fprintf(os.Stderr, "   빠짐: %s (%s)\n", e.Subject(), evidenceOrUnknown(e.Evidence))
	}
	return write(need)
}

func evidenceOrUnknown(s string) string {
	if s == "" {
		return "근거 불명"
	}
	return s
}

// ── 공통 ───────────────────────────────────────────────────────────────────

// loadResults — 노드들이 낸 CollectionResult JSON 을 읽는다.
func loadResults(dir string) ([]*discoveryv1.CollectionResult, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []*discoveryv1.CollectionResult
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		res := &discoveryv1.CollectionResult{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, res); err != nil {
			// **한 파일이 깨졌다고 전부 멈추지 않는다.** 다만 조용히 넘기지도 않는다 —
			// 빠진 노드를 모르면 "관측 안 됨"과 "못 읽음"이 뒤섞인다.
			fmt.Fprintf(os.Stderr, "   건너뜀(읽을 수 없음): %s — %v\n", filepath.Base(p), err)
			continue
		}
		out = append(out, res)
	}
	return out, nil
}

// layerName — 계층 이름은 파일 이름에서 온다. `prod.csv` → `prod`.
func layerName(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

func write(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
