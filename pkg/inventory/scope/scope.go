// Package scope implements 자산 스코프 거버넌스 (설계 §1.6).
//
// **규칙의 정의와 집행은 pqcota가 한다.** pqcota의 `kscope.AssetPolicy`가 규칙 형식이고,
// `pqcota-ingest -scope-assets`가 적재 전에 집행한다. 이 패키지는 그것을 재구현하지 않는다 —
// pqcota 타입을 그대로 쓰고, 그 위에 **조직에서만 필요한 것**을 얹는다:
//
//   - 계층 상속 — 조직 → 환경 → 노드군을 겹친다
//   - 변경 리뷰 — 무엇이 늘고 줄었는지 골라 리뷰-확정 상태기계에 태운다
//   - 배포 — 확정된 정책을 pqcota의 집행기가 읽는 CSV로 낸다
//
// **「이 자산은 안 본다」는 결정은 감사 대상이다.** 혼자 쓰면 자기 책임이지만 조직은
// 누가·언제·왜 뺐는지 남겨야 한다 — 사고 뒤에 "왜 이게 인벤토리에 없었나"에 답해야 하기
// 때문이다. 그 기록은 판정 원장(`decision`)이 맡는다.
package scope

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"

	// pqcota의 스코프 커널. **우리 패키지와 이름이 같아 별칭을 둔다** — 규칙 형식과 판정은
	// 저쪽이 갖고, 여기는 그 위의 거버넌스만 갖는다.
	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"
)

// Layer — 정책 계층 하나. 조직 → 환경(prod/dev) → 노드군 순으로 겹친다.
//
// 이름은 **일괄 판정의 열쇠**다(§3.4) — 한 계층에서 온 규칙들은 한 번에 판정된다. 수천 대를
// 규칙 한 줄씩 승인하는 리뷰는 끝나지 않는다.
type Layer struct {
	Name  string
	Rules []kscope.AssetRule
}

// Merge — 계층을 준 순서대로 이어 붙인다.
//
// **상속 규칙을 따로 만들지 않는다.** pqcota가 이미 "매치되는 마지막 규칙이 결정"이라고
// 정해 두었으므로, 상위를 앞에 하위를 뒤에 두면 그대로 하위 우선이 된다 — 판정 규칙이
// 세상에 하나만 존재한다. 우리가 잠금을 따로 두면 내려보낸 CSV를 pqcota가 집행한 결과와
// 우리 화면이 갈라진다.
//
// 하위가 상위의 제외를 include로 되돌릴 수 있다는 뜻이기도 하다. **그것을 막는 자리는
// 여기가 아니라 리뷰-확정 관문다** — 하위 정책 변경도 승인을 거친다.
func Merge(layers ...Layer) *kscope.AssetPolicy {
	p := &kscope.AssetPolicy{}
	for _, l := range layers {
		p.Rules = append(p.Rules, l.Rules...)
	}
	return p
}

// Change — 정책 변경 하나. 리뷰가 다루는 단위다.
type Change struct {
	Layer string           // 어느 계층에서 왔나. 일괄 판정의 열쇠
	Rule  kscope.AssetRule //
	Added bool             // true=이번에 생김, false=이번에 사라짐

	// Audited — 결론 없이는 확정할 수 없는 변경인가.
	//
	// **exclude 추가가 그것이다.** 인벤토리에서 빼는 결정이라, 나중에 "왜 이게 인벤토리에
	// 없었나"에 답할 수 있어야 한다. 규칙이 사라지는 쪽(제외를 거두는 것)은 인벤토리가
	// 넓어지는 방향이라 같은 무게가 아니다.
	Audited bool
}

// Diff — 지금 쓰는 정책과 제안된 계층 합본을 견줘 **바뀐 것만** 낸다.
//
// 전부를 매번 다시 승인하게 하면 아무도 안 본다(델타 리뷰와 같은 이유다). 규칙의 동일성은
// [RuleID]가 정하고, note는 동일성에서 뺀다 — 설명을 고쳤다고 재승인을 받을 일은 아니다.
func Diff(base *kscope.AssetPolicy, layers []Layer) []Change {
	old := map[string]bool{}
	if base != nil {
		for _, r := range base.Rules {
			old[RuleID(r)] = true
		}
	}
	now := map[string]string{} // 규칙 id → 어느 계층
	var order []string
	for _, l := range layers {
		for _, r := range l.Rules {
			id := RuleID(r)
			if _, seen := now[id]; !seen {
				order = append(order, id)
			}
			now[id] = l.Name
		}
	}

	byID := map[string]kscope.AssetRule{}
	for _, l := range layers {
		for _, r := range l.Rules {
			byID[RuleID(r)] = r
		}
	}

	var out []Change
	for _, id := range order {
		if old[id] {
			continue
		}
		r := byID[id]
		out = append(out, Change{Layer: now[id], Rule: r, Added: true, Audited: r.Exclude})
	}
	// 사라진 규칙. 정렬해서 내는 것은 base 의 순서가 계층과 무관하기 때문이다.
	var gone []string
	if base != nil {
		for _, r := range base.Rules {
			if _, still := now[RuleID(r)]; !still {
				gone = append(gone, RuleID(r))
				byID[RuleID(r)] = r
			}
		}
	}
	sort.Strings(gone)
	for _, id := range gone {
		out = append(out, Change{Layer: LayerRemoved, Rule: byID[id], Added: false})
	}
	return out
}

// LayerRemoved — 사라진 규칙이 묶이는 자리. **계층 이름 자리에 들어가는 값이라 코드다** —
// 화면이 보는 사람의 말로 옮긴다.
const LayerRemoved = "(removed)"

// RuleID — 규칙 하나의 동일성. 판정 원장의 대상 키가 된다.
//
// **note 는 넣지 않는다.** 사람이 읽으라고 붙인 설명이라, 문구를 다듬었다고 같은 규칙이
// 다른 규칙으로 보여서는 안 된다. 빈 칸은 pqcota와 같이 `*`로 읽는다.
func RuleID(r kscope.AssetRule) string {
	act := "include"
	if r.Exclude {
		act = "exclude"
	}
	return fmt.Sprintf("%s:%s/%s/%s", act, star(r.Runtime), star(r.Lib), star(r.AppKey))
}

func star(s string) string {
	if strings.TrimSpace(s) == "" {
		return "*"
	}
	return s
}

// WriteCSV — 확정된 정책을 **pqcota의 집행기가 읽는 형식 그대로** 낸다.
//
// `pqcota-ingest -scope-assets` 의 입력이다. 우리 형식을 따로 만들면 「거버넌스가 확정한
// 정책을 pqcota가 집행한다」가 코드로는 거짓이 된다.
func WriteCSV(w io.Writer, p *kscope.AssetPolicy) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"action", "runtime", "lib", "app_key", "note"}); err != nil {
		return err
	}
	if p == nil {
		p = &kscope.AssetPolicy{}
	}
	for _, r := range p.Rules {
		act := "include"
		if r.Exclude {
			act = "exclude"
		}
		if err := cw.Write([]string{act, r.Runtime, r.Lib, r.AppKey, r.Note}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
