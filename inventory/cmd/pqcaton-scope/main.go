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
	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

const usage = `usage:
  pqcaton-scope open   <layer.csv>... [-base <in-force.csv>] [-org <name>]
        Stack the layers and raise a draft review session with **only the rules that changed**.
        Layers stack in the order given - pass them org, environment, node group.
        When several rules match one asset, the later layer wins

  pqcaton-scope close  <session.json> [-judgments <file>] [-org <name>]
        Check the approval and emit **the CSV pqcota's enforcer reads**.
        An added exclude is not finalized without a conclusion - dropping an asset is auditable

  pqcaton-scope review <policy.csv> <results-dir> [-judgments <file>] [-org <name>] [-ttl <days>]
        Name the assets the policy dropped, and raise again only the ones whose approval
        is missing or stale. An exclusion is not a permanent exemption

  To work through a screen instead, use pqcaton-ui - same files, same gate.`

// 제외 승인의 기본 유효기간. 넘으면 「빼둔 사이 달라졌을 수 있다」로 다시 올린다.
const defaultTTLDays = 180

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	sub, args := os.Args[1], os.Args[2:]

	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	base := fs.String("base", "", "the policy CSV in force. Given this, only changed rules come up for review")
	judgments := fs.String("judgments", "", "file to append judgments to (JSONL, append-only)")
	orgName := fs.String("org", "local", "organization the policy and judgments are bound to")
	ttlDays := fs.Int("ttl", defaultTTLDays, "how long an exclusion approval stays valid (days)")

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
	fmt.Fprintf(os.Stderr, "%d layers · %d policy rules · %d changes (%d need a recorded reason)\n",
		len(layers), len(sf.Merged), len(sf.Changes), sf.AuditedCount())
	if len(sf.Changes) == 0 {
		fmt.Fprintln(os.Stderr, "no rules changed - nothing to approve")
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
		fmt.Fprintf(os.Stderr, "layer %s: %d judged in one batch\n", layer, n)
	}
	// **게이트를 지난 뒤에만 남긴다.**
	if judgmentPath != "" {
		n, err := scope.SaveJudgments(judgmentPath, orgName, sf, res.Decided)
		if err != nil {
			return fmt.Errorf("recording judgments: %w", err)
		}
		fmt.Fprintf(os.Stderr, "%d judgments appended to %s (append-only)\n", n, judgmentPath)
	}
	fmt.Fprintf(os.Stderr, "finalized: %d rules — this is the input to `pqcota-ingest -scope-assets`\n",
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

	// **화면과 같은 계산이다.** 두 벌이면 화면에서 본 「안 보고 있는 것」과 여기 세는
	// 것이 갈린다.
	ex, err := scope.ExcludedFromResults(p, results)
	if err != nil {
		return err
	}

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

	fmt.Fprintf(os.Stderr, "%d results · %d assets dropped by the policy · %d to look at again\n",
		len(results), len(ex), len(need))
	if len(ex) > 0 && len(need) == 0 {
		fmt.Fprintln(os.Stderr, "things were dropped, but every approval is still valid")
	}
	// **뺀 것을 이름으로 낸다.** pqcota는 수만 세지만, 사고 뒤에 답하려면 이름이 있어야 한다.
	for _, e := range ex {
		fmt.Fprintf(os.Stderr, "   dropped: %s (%s)\n", e.Subject(), evidenceOrUnknown(e.Evidence))
	}
	// **왜 다시 보라는지 말한다.** JSON 에는 코드가 담기므로(화면이 그 말로 그린다),
	// 사람이 읽는 줄은 여기서 낸다.
	for _, n := range need {
		fmt.Fprintf(os.Stderr, "   look again: %s — %s\n", n.Subject(), scope.EnglishReason(n.Reason))
	}
	return write(need)
}

func evidenceOrUnknown(s string) string {
	if s == "" {
		return "reason unknown"
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
			fmt.Fprintf(os.Stderr, "   skipped (unreadable): %s — %v\n", filepath.Base(p), err)
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
