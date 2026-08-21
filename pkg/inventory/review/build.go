// 이 파일은 **관측 결과에서 리뷰 세션을 세우는 일** 하나를 갖는다.
//
// 명령(`pqcaton-decide open -results`)과 화면(`pqcaton-ui`)이 같은 세션을 만들어야 한다.
// 따로 계산하면 화면에서 본 shadow 와 명령이 올린 리뷰 큐가 달라지고, 사람이 본 것과
// 판정하는 것이 어긋난다 — 오류가 아니라 그럴듯한 결과가 나오는 자리다.
package review

import (
	"fmt"
	"sort"

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
	// 찍어 버리면 화면에서는 아무도 못 본다. 그리고 **문장이 아니라 값이다** — 명령은
	// 영어로, 화면은 보는 사람의 말로 낸다.
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
		// `pqcaton-report` 가 파일 안쪽 표기로 말한다.
		return fmt.Sprintf("%d places where the declaration does not add up — `pqcaton-report` names them", w.Count)
	case WarnUnreadableResult:
		return "skipped (unreadable): " + w.Detail
	}
	return w.Code
}

// FromResults — 모아 둔 관측 결과와 선언으로 리뷰 세션을 세운다.
//
// **대조는 `report` 가 한다.** 대조 화면이 보는 것과 같은 계산이다.
func FromResults(resultsDir string, d decl.Declaration, orgName string) (*Built, error) {
	// **선언이 조직을 말한다.** 따로 주지 않았으면 선언의 것을 쓴다.
	if orgName == "" || orgName == decl.DefaultOrg {
		orgName = d.OrgOrDefault()
	}
	if d.Org != "" && d.Org != orgName {
		return nil, fmt.Errorf("the declaration belongs to organization %q but reconciliation was asked for %q", d.Org, orgName)
	}
	out := &Built{Org: orgName}
	// **앞뒤가 안 맞으면 말한다.** 노드↔IP 가 틀리면 CONFIRMED 여야 할 것이 shadow 로 올라온다.
	if p := decl.Check(d); len(p) > 0 {
		out.Warnings = append(out.Warnings, Warning{Code: WarnDeclProblems, Count: len(p)})
	}

	r, err := report.Build(resultsDir, d)
	if err != nil {
		return nil, err
	}
	for _, sk := range r.Skipped {
		out.Warnings = append(out.Warnings, Warning{Code: WarnUnreadableResult, Detail: sk})
	}
	autopass, queue := reconcile.BuildReviewQueue(r.Assets)

	sf := Session{Note: Note, Scope: "org://" + orgName, PolicyDecisions: map[string]string{}}
	for _, it := range queue {
		pol := PolicyOf(it.Rec.Key)
		sf.Items = append(sf.Items, Item{
			ID: Key(it.Rec.Key), Policy: pol,
			Node: it.Rec.Key.NodeID, Runtime: it.Rec.Key.Runtime,
			State: string(it.Rec.State), Conf: it.Rec.Confidence,
			Mandatory: it.Mandatory, Rescan: it.Rec.RescanCandidate,
		})
		if _, ok := sf.PolicyDecisions[pol]; !ok {
			sf.PolicyDecisions[pol] = ""
		}
	}
	for _, a := range autopass {
		sf.Autopass = append(sf.Autopass, Key(a.Key))
	}
	sort.Strings(sf.Autopass)

	out.Session, out.Assets, out.Nodes = sf, r.Assets, len(r.SeenBy)
	out.Confirmed, out.Undeclared, out.Unobserved = r.Counts()
	return out, nil
}

// Carry — 다시 세운 세션에 **사람이 적은 것을 옮긴다.**
//
// 관측이 갱신되면 리뷰 큐도 갱신되어야 하지만, 그때마다 판정을 처음부터 다시 적게 하면
// 아무도 화면을 안 쓴다. 항목 동일성(ID)과 정책 이름을 열쇠로 옮긴다.
//
// **정책에 못 보던 항목이 생겼으면 그 정책의 일괄 결론을 지운다.** 일괄 판정은 「이 정책의
// 항목들을 보고 내린 결론」인데, 새 항목은 사람이 본 적이 없다 — 그대로 두면 방금 나타난
// shadow 가 누가 승인한 적 없는 근거를 달고 확정을 통과한다. 서명도 지운다: 서명은 그
// 큐에 대한 것이다.
func Carry(prev, next Session) Session {
	was := map[string]Item{}
	for _, it := range prev.Items {
		was[it.ID] = it
	}
	gained := map[string]bool{}
	for i, it := range next.Items {
		old, seen := was[it.ID]
		if !seen {
			gained[it.Policy] = true
			continue
		}
		next.Items[i].Conclusion = old.Conclusion
		next.Items[i].Plan = old.Plan
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
	if sameItems(prev.Items, next.Items) {
		next.Signature = prev.Signature
	}
	return next
}

func sameItems(a, b []Item) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].State != b[i].State {
			return false
		}
	}
	return true
}
