// Command pqcaton-scope — 자산 스코프 정책을 사람이 승인하고 배포하는 자리(설계 §1.6).
//
// **규칙을 다시 만들지 않는다.** 형식과 집행은 상류의 `scope.AssetPolicy`가 갖고, 여기는
// 그 위의 거버넌스만 갖는다 — 계층 상속, 변경 승인, 감사 기록, 제외분 재검토.
//
//	pqcaton-scope open   <계층.csv>... [-base 현재.csv] [-org 이름] > session.json
//	  … 사람이 session.json 을 편집한다 (결론 · 승인자 · 서명)
//	pqcaton-scope close  <session.json> [-judgments 파일] [-org 이름] > asset-scope.csv
//	pqcaton-scope review <정책.csv> <results-dir> [-judgments 파일] [-org 이름]
//
// 계층은 준 순서대로 겹친다 — 조직, 환경, 노드군 순으로 주면 뒤가 이긴다.
//
// **파일이 곧 감사 기록이다.** `pqcaton-decide` 와 같은 왕복이라 조작을 따로 외울 것이 없다.
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
        계층은 준 순서대로 이깁니다 - 조직 · 환경 · 노드군 순으로 주십시오

  pqcaton-scope close  <session.json> [-judgments <파일>] [-org <이름>]
        승인을 확인하고 **상류 집행기가 읽는 CSV**를 낸다.
        exclude 추가는 결론이 없으면 확정하지 않는다 - 「안 본다」는 감사 대상입니다

  pqcaton-scope review <정책.csv> <results-dir> [-judgments <파일>] [-org <이름>] [-ttl <일>]
        정책이 뺀 자산을 **이름으로** 내고, 승인이 없거나 오래된 것만 다시 올린다.
        제외는 영구 면제가 아닙니다`

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

// ── 파일 형식 ──────────────────────────────────────────────────────────────

type sessionFile struct {
	Note      string `json:"_읽는_법"`
	Org       string `json:"org"`
	Reviewer  string `json:"reviewer"`
	Signature string `json:"signature"`
	// LayerDecisions — 계층 하나에 결론 하나가 기본이다(§3.4). 규칙 한 줄씩 승인하는 리뷰는
	// 수천 대에서 끝나지 않는다. 개별 규칙의 conclusion 은 예외를 위한 자리다.
	LayerDecisions map[string]string `json:"계층_판정"`
	Changes        []changeFile      `json:"changes"`
	// Merged — 확정되면 그대로 CSV 로 나갈 정책 전문. **바뀐 것만 리뷰하되 나가는 것은
	// 전문이다** - 상류 집행기는 정책 전체를 받는다.
	Merged []ruleFile `json:"확정될_정책"`
}

type changeFile struct {
	ID    string `json:"id"`
	Layer string `json:"layer"`
	// Kind — 추가 | 제거. 사람이 읽는 자리라 한글로 둔다.
	Kind string `json:"변경"`
	Rule string `json:"rule"`
	Note string `json:"note,omitempty"`
	// Audited — 결론 없이 확정할 수 없다. exclude 추가가 그것이다.
	Audited bool `json:"근거_필수"`

	// ── 사람이 채우는 자리 ──
	Conclusion string `json:"conclusion"`
}

type ruleFile struct {
	Action  string `json:"action"`
	Runtime string `json:"runtime,omitempty"`
	Lib     string `json:"lib,omitempty"`
	AppKey  string `json:"app_key,omitempty"`
	Note    string `json:"note,omitempty"`
}

const note = "계층_판정 에 계층별 결론을 적으면 그 계층의 규칙이 한 번에 판정됩니다(권장). " +
	"예외만 changes 의 conclusion 으로 따로 적습니다. reviewer 와 signature 를 채운 뒤 " +
	"`pqcaton-scope close` 에 넣으세요. 근거_필수 인 변경은 결론이 없으면 확정되지 않습니다."

// ── open ───────────────────────────────────────────────────────────────────

func open(layerPaths []string, basePath, orgName string) error {
	if _, err := org.Parse(orgName); err != nil {
		return err
	}
	var layers []scope.Layer
	for _, p := range layerPaths {
		rules, err := loadPolicy(p)
		if err != nil {
			return err
		}
		layers = append(layers, scope.Layer{Name: layerName(p), Rules: rules.Rules})
	}
	var basePolicy *kscope.AssetPolicy
	if basePath != "" {
		var err error
		if basePolicy, err = loadPolicy(basePath); err != nil {
			return err
		}
	}

	merged := scope.Merge(layers...)
	changes := scope.Diff(basePolicy, layers)

	sf := sessionFile{Note: note, Org: orgName, LayerDecisions: map[string]string{}}
	for _, c := range changes {
		kind := "추가"
		if !c.Added {
			kind = "제거"
		}
		sf.Changes = append(sf.Changes, changeFile{
			ID: scope.RuleID(c.Rule), Layer: c.Layer, Kind: kind,
			Rule: scope.RuleID(c.Rule), Note: c.Rule.Note, Audited: c.Audited,
		})
		if _, ok := sf.LayerDecisions[c.Layer]; !ok {
			sf.LayerDecisions[c.Layer] = "" // 사람이 채울 자리를 미리 연다
		}
	}
	for _, r := range merged.Rules {
		sf.Merged = append(sf.Merged, ruleFile{Action: action(r), Runtime: r.Runtime,
			Lib: r.Lib, AppKey: r.AppKey, Note: r.Note})
	}

	fmt.Fprintf(os.Stderr, "계층 %d개 · 정책 규칙 %d개 · 변경 %d건(근거 필수 %d)\n",
		len(layers), len(merged.Rules), len(sf.Changes), countAudited(sf.Changes))
	if len(sf.Changes) == 0 {
		fmt.Fprintln(os.Stderr, "바뀐 규칙이 없습니다 - 승인할 것이 없습니다")
	}
	return write(sf)
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
	// **세션에 적힌 조직과 지금 준 조직이 다르면 끊는다.** 남의 조직 정책을 확정하는 것은
	// 사고다 - 대조 엔진·판정 원장과 같은 규칙이다.
	if sf.Org != "" && sf.Org != orgName {
		return fmt.Errorf("세션은 조직 %q의 것인데 %q로 확정하려 한다", sf.Org, orgName)
	}

	items := make([]decision.Item, 0, len(sf.Changes))
	for _, c := range sf.Changes {
		items = append(items, decision.Item{ID: c.ID, Policy: c.Layer, Mandatory: c.Audited})
	}
	s := decision.NewSession("scope://"+orgName, items)
	if err := s.StartReview(); err != nil {
		return err
	}
	// **계층이 먼저다**(§3.4). 개별 결론은 그 뒤에 얹혀 예외를 만든다.
	for layer, c := range sf.LayerDecisions {
		if strings.TrimSpace(c) != "" {
			if n := s.DecidePolicy(layer, c); n > 0 {
				fmt.Fprintf(os.Stderr, "계층 %s: %d건 일괄 판정\n", layer, n)
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

	// **게이트는 여기다.** 근거 필수인 변경에 결론이 없으면 정책이 나가지 않는다.
	if err := s.Finalize(); err != nil {
		return fmt.Errorf("%w\n%s", err, pending(sf))
	}

	if judgmentPath != "" {
		n, err := saveJudgments(judgmentPath, orgName, sf, s)
		if err != nil {
			return fmt.Errorf("판정 기록: %w", err)
		}
		fmt.Fprintf(os.Stderr, "판정 %d건을 %s 에 남겼습니다 (append-only)\n", n, judgmentPath)
	}

	p := &kscope.AssetPolicy{}
	for _, r := range sf.Merged {
		p.Rules = append(p.Rules, kscope.AssetRule{
			Exclude: r.Action == "exclude", Runtime: r.Runtime,
			Lib: r.Lib, AppKey: r.AppKey, Note: r.Note,
		})
	}
	fmt.Fprintf(os.Stderr, "확정: 규칙 %d개 — `pqcota-ingest -scope-assets` 의 입력입니다\n", len(p.Rules))
	return scope.WriteCSV(os.Stdout, p)
}

// saveJudgments — 확정된 변경을 원장에 남긴다. **게이트를 지난 뒤에만** 남긴다 —
// 확정되지 않은 것을 쌓으면 그 기록이 "누가 무엇을 승인했나"를 더는 답하지 못한다.
func saveJudgments(path, orgName string, sf sessionFile, s *decision.Session) (int, error) {
	store, err := decision.NewFileJudgmentStore(org.ID(orgName), path)
	if err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	n := 0
	for _, it := range s.Items {
		if !it.Decided {
			continue
		}
		j := &decision.Judgment{
			ID:      fmt.Sprintf("%s@%d", it.ID, now),
			Subject: it.ID, Conclusion: it.Conclusion,
			Reviewer: sf.Reviewer, Signature: sf.Signature,
			// 근거는 규칙 그 자체다 - 규칙이 달라지면 대상 id 가 달라지므로 새 판정이 된다.
			BasisHash: decision.HashBasis(it.ID),
			DecidedAt: now,
		}
		if err := store.Save(j); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// pending — 무엇이 남아 확정이 안 되는지. 모르면 사람은 파일을 고칠 수 없다.
func pending(sf sessionFile) string {
	var b strings.Builder
	if sf.Reviewer == "" || sf.Signature == "" {
		b.WriteString("   · reviewer 와 signature 를 채우십시오\n")
	}
	for _, c := range sf.Changes {
		if c.Audited && strings.TrimSpace(c.Conclusion) == "" &&
			strings.TrimSpace(sf.LayerDecisions[c.Layer]) == "" {
			fmt.Fprintf(&b, "   · 결론 없음: %s (계층 %s)\n", c.ID, c.Layer)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// ── review ─────────────────────────────────────────────────────────────────

func review(policyPath, dir, judgmentPath, orgName string, ttl int64) error {
	p, err := loadPolicy(policyPath)
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
	// **뺀 것을 이름으로 낸다.** 상류는 수만 세지만, 사고 뒤에 답하려면 이름이 있어야 한다.
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

func loadPolicy(path string) (*kscope.AssetPolicy, error) {
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

func action(r kscope.AssetRule) string {
	if r.Exclude {
		return "exclude"
	}
	return "include"
}

func countAudited(cs []changeFile) int {
	n := 0
	for _, c := range cs {
		if c.Audited {
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
