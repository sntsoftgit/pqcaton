// 이 파일은 **관측 결과에서 리뷰 세션을 세우는 일** 하나만 한다.
//
// 명령(`pqcaton-decide open -results`)과 화면(`pqcaton-ui`)이 같은 세션을 만들어야 한다.
// 따로 계산하면 화면에서 본 UNDECLARED와 명령이 올린 리뷰 큐가 달라지고, 사람이 본 것과
// 판정하는 것이 어긋난다 — 오류가 아니라 그럴듯한 결과가 나오는 자리다.
package review

import (
	"fmt"
	"os"
	"sort"

	"github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
)

// Built — 세운 세션과, 그것을 세우며 알게 된 것.
type Built struct {
	Session Session
	// Assets — 대조 결과 전체. 콘솔 표(`-view`)가 쓴다.
	Assets []reconcile.Reconciled
	// Warnings — 세션은 나왔지만 사람이 알아야 하는 것.
	//
	// **찍지 않고 돌려준다.** 명령은 표준오류로 내고 화면은 알림 상자에 넣는다 — 여기서
	// 찍어 버리면 화면에서는 아무도 못 본다. 그리고 **문장이 아니라 값이다** — 명령에는
	// 영어로, 화면에는 보는 사람의 말로 적는다.
	Warnings []Warning
	// Org — 실제로 쓴 조직. 선언이 말한 것일 수 있다.
	Org string
	// Confirmed · Undeclared · Unobserved — 한 줄 요약에 쓴다.
	Confirmed, Undeclared, Unobserved int
	// Nodes — 관측된 노드 수.
	Nodes int
}

// Warning — 세션은 나왔지만 사람이 알아야 하는 것 하나.
type Warning struct {
	Code   string
	Count  int
	Detail string
}

const (
	WarnDeclProblems     = "declaration_has_problems"
	WarnUnreadableResult = "result_unreadable"
)

// English — 명령이 읽는 문장. **여기가 영어의 유일한 자리다.**
func (w Warning) English() string {
	switch w.Code {
	case WarnDeclProblems:
		// **화면은 어느 자리인지 말하지 않는다** — 선언 화면은 적는 자리다. 어느 자리인지는
		// `pqcaton-report`가 파일 안쪽 표기로 알린다.
		return fmt.Sprintf("%d places where the declaration does not add up — `pqcaton-report` names them", w.Count)
	case WarnUnreadableResult:
		return "skipped (unreadable): " + w.Detail
	}
	return w.Code
}

// LoadAssetPolicy — 자산 스코프 정책 파일(scope-assets.csv)을 읽는다. 빈 경로면 정책 없음(nil).
// 상류 `pqcota-ingest -scope-assets`가 읽는 것과 같은 파서다 — 다르게 읽으면 스냅샷 지문이 어긋난다.
func LoadAssetPolicy(path string) (*scope.AssetPolicy, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening the asset scope: %w", err)
	}
	defer f.Close()
	return scope.LoadAssetPolicy(f)
}

// FromResults — 모아 둔 관측 결과와 선언으로 리뷰 세션을 세운다.
//
// **대조는 `report`가 한다.** 대조 화면이 보는 것과 같은 계산이다.
func FromResults(resultsDir string, d decl.Declaration, orgName string) (*Built, error) {
	return FromResultsWith(resultsDir, d, orgName, nil)
}

// FromResultsWith — 자산 스코프 정책을 걸어 세운다. 상류 적재와 같은 정책을 걸어야 스냅샷 지문이
// 상류와 같아 되짚기가 된다(`report.BuildWith`).
func FromResultsWith(resultsDir string, d decl.Declaration, orgName string, policy *scope.AssetPolicy) (*Built, error) {
	// **선언에 조직이 적혀 있다.** 따로 주지 않았으면 선언의 것을 쓴다.
	if orgName == "" || orgName == decl.DefaultOrg {
		orgName = d.OrgOrDefault()
	}
	if d.Org != "" && d.Org != orgName {
		return nil, fmt.Errorf("the declaration belongs to organization %q but reconciliation was asked for %q", d.Org, orgName)
	}
	out := &Built{Org: orgName}
	// **앞뒤가 안 맞으면 알린다.** 노드↔IP가 틀리면 CONFIRMED여야 할 것이 UNDECLARED로 올라온다.
	if p := decl.Check(d); len(p) > 0 {
		out.Warnings = append(out.Warnings, Warning{Code: WarnDeclProblems, Count: len(p)})
	}

	r, err := report.BuildWith(resultsDir, d, policy)
	if err != nil {
		return nil, err
	}
	for _, sk := range r.Skipped {
		out.Warnings = append(out.Warnings, Warning{Code: WarnUnreadableResult, Detail: sk})
	}
	autopass, queue := reconcile.BuildReviewQueue(r.Assets)

	sf := Session{Note: Note, Scope: "org://" + orgName, PolicyDecisions: map[string]string{},
		RulesetVersion: RulesetVersion, SessionID: NewSessionID()}
	for _, it := range queue {
		item := ItemOf(it.Rec, it.Mandatory)
		sf.Items = append(sf.Items, item)
		if _, ok := sf.PolicyDecisions[item.Policy]; !ok {
			sf.PolicyDecisions[item.Policy] = ""
		}
	}
	// 자동통과도 항목이다 - 판정은 요구하지 않지만 계획 칸을 든다. 순서는 ID 순으로 고정한다.
	for _, a := range autopass {
		sf.Autopass = append(sf.Autopass, ItemOf(a, false))
	}
	sort.Slice(sf.Autopass, func(i, j int) bool { return sf.Autopass[i].ID < sf.Autopass[j].ID })

	out.Session, out.Assets, out.Nodes = sf, r.Assets, len(r.SeenBy)
	out.Confirmed, out.Undeclared, out.Unobserved = r.Counts()
	return out, nil
}

// Carry — 다시 세운 세션에 **사람이 적은 것을 옮긴다.**
//
// 관측이 갱신되면 리뷰 큐도 갱신되어야 하지만, 그때마다 판정을 처음부터 다시 적게 하면
// 아무도 화면을 안 쓴다. 항목 동일성(ID)과 정책 이름을 식별자로 옮긴다.
//
// **정책에 못 보던 항목이 생겼으면 그 정책의 일괄 결론을 지운다.** 일괄 판정은 「이 정책의
// 항목들을 보고 내린 결론」인데, 새 항목은 사람이 본 적이 없다 — 그대로 두면 방금 나타난
// UNDECLARED가 누가 승인한 적 없는 근거를 달고 확정을 통과한다. 서명도 지운다: 서명은 그
// 큐에 대한 것이다.
//
// **리뷰 항목과 자동통과를 합쳐 ID로 찾는다.** 확신이 0.8을 넘나들면 같은 자산이 두 컬렉션
// 사이를 옮겨 다닌다 - 어느 컬렉션에 있었는지는 보지 않고, 사람이 고른 계획 칸은 따라간다.
// 쓰는 자리는 [update]다: [All]이 준 복사본에 쓰면 아무 일도 일어나지 않는다.
//
// 옛 식별자 목록(LegacyAutopass)은 새 세션에 같은 ID의 구조화 후보가 있으면 그것으로 치환된
// 셈이다(새 세션이 이미 들고 있다). 없으면 목록에 남기고 계획 불가를 알린다 - 알리지 않고 지우지 않는다.
func Carry(prev, next Session) Session {
	was := map[string]Item{}
	for _, it := range All(prev) {
		was[it.ID] = it
	}
	gained := map[string]bool{}
	for _, it := range next.Items {
		if _, seen := was[it.ID]; !seen {
			gained[it.Policy] = true
		}
	}
	for _, it := range All(next) {
		old, seen := was[it.ID]
		if !seen {
			continue
		}
		// 사람이 고른 것을 다 옮긴다. 하나라도 빠뜨리면 다시 열 때마다 검토자가 같은 선택을
		// 되풀이하게 되고, 그러다 놓친 칸이 확정에서 막힌다.
		update(&next, it.ID, func(n *Item) {
			n.Conclusion = old.Conclusion
			n.Plan = old.Plan
			n.Level = old.Level
			n.Kind = old.Kind
			n.TargetAlgorithm = old.TargetAlgorithm
			n.FIPS = old.FIPS
			n.Config = old.Config
			n.RollbackNote = old.RollbackNote
			n.Pre, n.Activate = old.Pre, old.Activate
			n.Deactivate, n.Restart = old.Deactivate, old.Restart
		})
	}
	present := map[string]bool{}
	for _, it := range All(next) {
		present[it.ID] = true
	}
	for _, id := range prev.LegacyAutopass {
		if !present[id] {
			next.LegacyAutopass = append(next.LegacyAutopass, id)
		}
	}
	for pol := range next.PolicyDecisions {
		if gained[pol] {
			continue
		}
		if v, ok := prev.PolicyDecisions[pol]; ok {
			next.PolicyDecisions[pol] = v
		}
	}
	next.Reviewer = prev.Reviewer
	// **세션의 동일성을 옮긴다.** 다시 연 것은 같은 세션이다 — 계획 id와 원장 행이 이 값으로
	// 이어지므로, 다시 열 때마다 바뀌면 원장에 같은 세션의 판정이 여러 id로 흩어진다.
	// 앞 세션에 id가 없으면(옛 빌드가 연 것) 새로 만든 것을 그대로 둔다 — 빈 값을 옮겨 오면
	// 확정에서 다시 열라며 막히는데, 지금 여는 것이 곧 그 「다시 열기」다.
	if prev.SessionID != "" {
		next.SessionID = prev.SessionID
	}
	// **근거가 달라지면 서명은 남지 않는다.** 서명은 「이 근거를 이 규칙으로 보고 승인했다」는
	// 뜻이다. 사람이 적은 것은 참고값으로 옮기되(위), 승인만은 옮기지 않는다.
	//
	// 무엇이 근거인지는 [BasisOf] 하나가 정한다. 전에는 여기서 ID와 상태만 비교해서, 같은
	// 규칙 아래 관측이 달라진 것을 통째로 놓쳤다 — 상류의 `finding_id`는 자산 동일성이라
	// 버전이 오르고 강화 판정이 달라져도 그대로이기 때문이다. 델타 리뷰는 걸리는데 승인
	// 서명은 살아남는 상태가 그 자리에서 났다.
	if sameBasis(prev, next) {
		next.Signature = prev.Signature
	}
	return next
}

// sameBasis — 두 세션이 같은 근거 위에 서 있나.
//
// 규칙 판을 따로 한 번 더 보는 것은 **큐가 비었을 때** 때문이다. 항목이 하나도 없으면
// 항목별 비교는 전부 참이라, 규칙이 바뀌어도 서명이 살아남는다.
//
// **자리(index)가 아니라 ID로 맞춘다.** 항목이 컬렉션을 옮기면 길이가 둘 다 달라지고 자리도
// 어긋난다. ID 집합이 다르거나 같은 ID의 근거가 다르면 다른 근거다. 자동통과의 근거만 바뀌어도
// 서명이 무효다 - 그 항목도 계획에 들어갈 수 있다.
func sameBasis(prev, next Session) bool {
	if prev.RulesetVersion != next.RulesetVersion {
		return false
	}
	a, b := All(prev), All(next)
	if len(a) != len(b) {
		return false
	}
	was := map[string]string{}
	for _, it := range a {
		was[it.ID] = BasisOf(it, prev.RulesetVersion)
	}
	for _, it := range b {
		h, ok := was[it.ID]
		if !ok || h != BasisOf(it, next.RulesetVersion) {
			return false
		}
	}
	return true
}
