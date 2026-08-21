// Package ui — 화면을 그리고 폼을 읽는 것.
//
// **어디서 읽고 어디에 쓰는지, 누가 들어올 수 있는지는 부르는 쪽이 정한다.** 로컬 명령은
// 파일에서 읽고 인증이 없으며, 컨트롤 플레인은 DB에서 읽고 앞에 인증이 선다 — 두 배포
// 형태의 차이가 그 둘뿐이라, 나머지(화면)를 여기 한 곳에만 둔다.
//
// 화면을 명령 안에 두면 고칠 곳이 둘이 되고, 고객마다 다른 화면이 생길 길이 열린다.
//
// 템플릿은 templ 로 쓰고 생성된 `*_templ.go` 를 리포에 함께 둔다 — **빌드에 생성기가
// 필요하지 않다.** 화면을 고치는 사람만 `make generate` 를 돌린다.
//
// 자바스크립트는 htmx 한 파일뿐이고 바이너리에 박혀 나간다. 쓰는 자리는 「행 추가」처럼
// **페이지를 다시 띄우면 적던 것이 날아가는 자리**로 한정한다 — 화면의 뼈대는 여전히
// 폼과 링크라, 스크립트가 막힌 환경에서도 읽고 고칠 수 있다.
package ui

import (
	"context"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// Page — 화면 한 장에 공통으로 실리는 것.
type Page struct {
	// Title — 브라우저 제목이자 머리글.
	Title string
	// Subtitle — 무엇을 보고 있는지. 대개 대상 파일이다.
	Subtitle string
	// Message · Problem — 방금 무엇이 됐고 무엇이 막았나.
	//
	// **막힌 이유를 안 보여 주면 사람은 화면에서도 고칠 수 없다.** 명령이 무엇이 남았는지
	// 말하는 것과 같은 자리다.
	Message string
	Problem string
	// Warnings — 하기는 했는데 사람이 알아야 하는 것.
	//
	// **Problem 과 다르다.** Problem 은 하지 않은 것이고, 이것은 한 것이다. 둘을 섞으면
	// 「선언에 맞지 않는 자리가 있다」가 「저장이 안 됐다」로 읽힌다.
	Warnings []string
	// Nav — 화면 사이를 오가는 자리. 부르는 쪽이 무엇을 열었는지에 따라 다르다.
	Nav []Link
	// Lang — 이 화면을 그릴 말. **화면만 두 말을 쓴다** — 명령의 출력과 로그는 영어다.
	Lang Lang
	// LangHref — 지금 자리 그대로 말만 바꾸는 주소.
	LangHref string
}

// Link — 위쪽 이동 링크.
type Link struct {
	Href string
	Text string
	// Here — 지금 보고 있는 화면인가.
	Here bool
}

// ── 리뷰 큐 ────────────────────────────────────────────────────────────────

// ReviewView — 리뷰 큐 화면이 보는 것. 세션을 **정책으로 묶어** 보여 준다 — 그것이 판정의
// 기본 단위다(§3.4). 한 줄씩 늘어놓으면 화면이 있어도 수천 대에서 리뷰가 끝나지 않는다.
type ReviewView struct {
	Page
	Scope     string
	Reviewer  string
	Signature string
	Policies  []PolicyGroup
	Autopass  int
}

// PolicyGroup — 한 정책과 그에 묶인 항목들.
type PolicyGroup struct {
	Name       string
	Conclusion string
	Items      []review.Item
	Mandatory  int
}

// NewReviewView — 세션을 화면이 보는 모양으로 옮긴다.
func NewReviewView(sf review.Session, page Page) ReviewView {
	byPolicy := map[string]*PolicyGroup{}
	var order []string
	for _, it := range sf.Items {
		g, ok := byPolicy[it.Policy]
		if !ok {
			g = &PolicyGroup{Name: it.Policy, Conclusion: sf.PolicyDecisions[it.Policy]}
			byPolicy[it.Policy] = g
			order = append(order, it.Policy)
		}
		g.Items = append(g.Items, it)
		if it.Mandatory {
			g.Mandatory++
		}
	}
	sort.Strings(order)
	v := ReviewView{Page: page, Scope: sf.Scope, Reviewer: sf.Reviewer,
		Signature: sf.Signature, Autopass: len(sf.Autopass)}
	for _, name := range order {
		v.Policies = append(v.Policies, *byPolicy[name])
	}
	return v
}

// RenderReview — 리뷰 큐 화면을 쓴다.
func RenderReview(w io.Writer, v ReviewView) error {
	return reviewPage(v).Render(context.Background(), w)
}

// ApplyReview — 폼에서 온 값을 세션에 얹는다.
//
// **받은 세션을 고쳐서 돌려준다** — 부르는 쪽이 방금 읽어 온 것에 얹으므로, 그 사이 파일이나
// DB를 고친 사람의 편집이 사라지지 않는다.
func ApplyReview(sf review.Session, f url.Values) review.Session {
	sf.Reviewer = strings.TrimSpace(f.Get("reviewer"))
	sf.Signature = strings.TrimSpace(f.Get("signature"))
	if sf.PolicyDecisions == nil {
		sf.PolicyDecisions = map[string]string{}
	}
	for pol := range sf.PolicyDecisions {
		sf.PolicyDecisions[pol] = strings.TrimSpace(f.Get("policy:" + pol))
	}
	for i, it := range sf.Items {
		sf.Items[i].Conclusion = strings.TrimSpace(f.Get("item:" + it.ID))
		sf.Items[i].Plan = f.Get("plan:"+it.ID) != ""
	}
	return sf
}

// ── 선언 ───────────────────────────────────────────────────────────────────

// DeclView — 선언 편집 화면이 보는 것.
//
// **노드마다 그 노드의 암호 자산을 함께 묶는다.** 자산은 「이 노드에서 이 모듈이
// 쓰인다」는 말이라, 노드와 떼어 놓은 표에서는 노드 이름을 사람이 다시 적어야 했다 —
// 오타 하나면 선언은 있는데 아무 노드에도 붙지 않는 자산이 된다.
type DeclView struct {
	Page
	Org string
	// Nodes — 관리 대상 노드와, 그 노드에 선언한 자산.
	Nodes []DeclNode
	// Edges — 통신 엣지. 노드 둘을 잇는 것이라 노드 안에 넣지 않는다.
	Edges []decl.Edge
	// Comment — 생성 도구가 남긴 머리말. 화면에 칸은 없지만 저장에서 사라지면 안 된다.
	Comment string
	// Unmatched — 관측에는 있는데 **어느 선언 노드에도 붙지 않은** 노드 이름.
	//
	// 「관측 이름」 칸의 후보다. 붙지 않았다는 것은 그 노드의 자산이 통째로 shadow 로
	// 오른다는 뜻이고, 그것은 선언이 틀려서가 아니라 이름이 서로 달라서다.
	Unmatched []string
}

// DeclNode — 노드 한 대와 그 안의 자산.
type DeclNode struct {
	Name string
	IPs  []string
	// Assets — 그 노드에서 쓰인다고 선언한 암호 런타임과 컴포넌트.
	Assets []DeclAsset
	// ObservedAs — 관측이 이 노드를 부르는 이름. 호스트명이 이름과 같으면 비워 둔다.
	ObservedAs []string
	// Seen — 그 노드에서 **관측된** 자산. 컴포넌트 칸의 후보로 내놓는다.
	//
	// **옮겨 적다 틀리는 자리를 없앤다.** 대조는 컴포넌트가 글자 그대로 같을 때만
	// 맞는데, 관측 이름은 `.so` 뒤가 떼인 채로 온다 — 화면에 보이는 대로 적으면 맞지
	// 않고, 그것이 오류 없이 미관측·shadow 로 구분된다. 여기 있는 이름이 맞는 이름이다.
	Seen []DeclAsset
}

// DeclAsset — 자산 한 줄. 노드는 묶인 자리가 말하므로 여기 없다.
type DeclAsset struct {
	Runtime   string
	Component string
}

// IPText · ObservedAsText — 여러 값을 한 칸에 넣을 문자열.
func (n DeclNode) IPText() string         { return strings.Join(n.IPs, ", ") }
func (n DeclNode) ObservedAsText() string { return strings.Join(n.ObservedAs, ", ") }

// WithObserved — 관측된 자산을 노드마다 후보로 붙인다. 관측이 없으면(결과 디렉터리를
// 주지 않았으면) 붙일 것이 없고, 화면은 그대로 손으로 적는 자리가 된다.
func (v DeclView) WithObserved(seen map[string][]DeclAsset, unmatched []string) DeclView {
	sort.Strings(unmatched)
	v.Unmatched = unmatched
	for i := range v.Nodes {
		got := seen[v.Nodes[i].Name]
		if len(got) == 0 {
			continue
		}
		sort.Slice(got, func(a, b int) bool {
			if got[a].Runtime != got[b].Runtime {
				return got[a].Runtime < got[b].Runtime
			}
			return got[a].Component < got[b].Component
		})
		var out []DeclAsset
		for j, a := range got {
			if j > 0 && a == got[j-1] {
				continue // 노드 하나에서 같은 것이 여러 번 관측된다
			}
			out = append(out, a)
		}
		v.Nodes[i].Seen = out
	}
	return v
}

// NewDeclView — 선언을 화면이 보는 모양으로 옮긴다.
//
// **앞뒤가 맞는지는 여기서 말하지 않는다.** 선언 화면은 적는 자리다 — 어긋남은
// `decl.Check` 를 쓰는 검토 화면과 `pqcaton-report` 가 짚는다.
func NewDeclView(d decl.Declaration, page Page) DeclView {
	return DeclView{Page: page, Org: d.Org, Comment: d.Comment,
		Nodes: groupByNode(d), Edges: d.Edges}
}

// groupByNode — 파일의 세 목록(scope · nodes · assets)을 노드마다 한 덩어리로 묶는다.
//
// **같은 것을 여러 번 적는 자리였다.** 스코프에 이름, 주소 표에 같은 이름, 자산 표에 또
// 같은 이름 — 한 곳에 적었으면 애초에 생기지 않을 어긋남을 그동안 화면이 짚어 왔다.
//
// 손으로 고친 파일에 **IP 없는 스코프 이름**이나 **어느 노드에도 없는 자산의 노드
// 이름**이 있으면 그것도 올린다. 저장하면 관리 대상에서 빠지지만, 화면에서 지워 버리면
// 고쳐 넣을 자리조차 없어진다.
func groupByNode(d decl.Declaration) []DeclNode {
	at := map[string]int{}
	var out []DeclNode
	add := func(name string) int {
		name = strings.TrimSpace(name)
		if name == "" {
			return -1
		}
		if i, seen := at[name]; seen {
			return i
		}
		at[name] = len(out)
		out = append(out, DeclNode{Name: name})
		return at[name]
	}
	// **스코프 순서를 앞세운다** — 사람이 관리 대상을 적은 차례가 그 사람의 순서다.
	for _, n := range d.Scope {
		add(n)
	}
	for _, n := range d.Nodes {
		if i := add(n.Name); i >= 0 {
			out[i].IPs = append(out[i].IPs, n.IPs...)
			out[i].ObservedAs = append(out[i].ObservedAs, n.ObservedAs...)
		}
	}
	for _, a := range d.Assets {
		if i := add(a.Node); i >= 0 {
			out[i].Assets = append(out[i].Assets, DeclAsset{Runtime: a.Runtime, Component: a.Component})
		}
	}
	return out
}

// RenderDecl — 선언 편집 화면을 쓴다.
func RenderDecl(w io.Writer, v DeclView) error {
	return declPage(v).Render(context.Background(), w)
}

// RenderRow — 「행 추가」가 돌려주는 조각: 빈 줄 하나와, 번호가 하나 오른 버튼.
//
// **화면과 같은 조각을 쓴다**(decl.templ 의 nodeBlock·assetRow·edgeRow). 폼 이름이 곧
// 저장 경로라, 둘로 갈라지면 새로 넣은 줄만 조용히 저장되지 않는다.
//
// node 는 자산일 때만 쓴다 — 자산은 어느 노드의 것인지가 폼 이름에 들어간다.
func RenderRow(w io.Writer, l Lang, kind string, node, i int) error {
	return rowFragment(l, kind, node, i).Render(context.Background(), w)
}

// ApplyDecl — 폼에서 온 값으로 선언을 다시 만든다.
//
// **얹는 것이 아니라 다시 만든다.** 표에서 줄을 지우는 방법이 「이름을 비우는 것」이므로,
// 기존 것에 얹으면 지운 줄이 되살아난다. `_comment` 는 생성 도구가 남긴 머리말이라 그대로
// 들고 간다 — 사람이 편집할 자리가 아니다.
//
// dropped — IP를 적지 않아 관리 대상에서 뺀 이름. 화면이 그 사실을 말해야 한다 —
// 저장하면 그 줄이 표에서 사라지므로, 말하지 않으면 지워진 것으로 보인다.
func ApplyDecl(prev decl.Declaration, f url.Values) (d decl.Declaration, dropped []string) {
	d = decl.Declaration{Comment: prev.Comment, Org: strings.TrimSpace(f.Get("org"))}

	// **IP를 적은 줄만 관리 대상이 된다.** IP가 없으면 관측에 찍힌 주소를 이 이름과
	// 이을 근거가 없어, 선언한 엣지는 미관측으로 관측된 엣지는 shadow 로 구분된다 —
	// 대조가 막히지 않은 채로 틀린다. 이름만 적어 두는 것은 관리가 아니다.
	//
	// **자산은 그 노드에 묶여 있다.** 노드가 빠지면 그 노드의 자산도 함께 빠진다 —
	// 어느 노드의 것인지 없는 자산은 대조할 자리가 없다.
	for _, ni := range formRows(f, "node.name.", "") {
		nm := strings.TrimSpace(f.Get("node.name." + strconv.Itoa(ni)))
		if nm == "" {
			continue // 이름을 비우면 지운 것이다
		}
		ips := splitList(f.Get("node.ips." + strconv.Itoa(ni)))
		if len(ips) == 0 {
			dropped = append(dropped, nm)
			continue
		}
		d.Scope = append(d.Scope, nm)
		d.Nodes = append(d.Nodes, decl.Node{Name: nm, IPs: ips,
			ObservedAs: splitList(f.Get("node.seen." + strconv.Itoa(ni)))})
		for _, i := range formRows(f, "asset."+strconv.Itoa(ni)+".", ".component") {
			c := strings.TrimSpace(f.Get(assetField(ni, i, "component")))
			if c == "" {
				continue // 컴포넌트를 비우면 지운 줄이다
			}
			d.Assets = append(d.Assets, decl.Asset{Node: nm,
				Runtime:   strings.TrimSpace(f.Get(assetField(ni, i, "runtime"))),
				Component: c})
		}
	}
	for _, i := range formRows(f, "edge.src.", "") {
		src := strings.TrimSpace(f.Get("edge.src." + strconv.Itoa(i)))
		dst := strings.TrimSpace(f.Get("edge.dst." + strconv.Itoa(i)))
		if src == "" || dst == "" {
			continue
		}
		port, _ := strconv.ParseUint(strings.TrimSpace(f.Get("edge.port."+strconv.Itoa(i))), 10, 32)
		d.Edges = append(d.Edges, decl.Edge{
			Src: src, Dst: dst, Port: uint32(port),
			Proto: strings.TrimSpace(f.Get("edge.proto." + strconv.Itoa(i))),
		})
	}
	return d, dropped
}

// formRows — 폼에 실제로 온 줄 번호를 차례대로.
//
// **번호가 끊겨도 읽는다.** 화면에서 「제거」로 가운데 줄을 지우면 그 번호가 빈 채로
// 남는다. 예전처럼 끊기는 자리에서 읽기를 멈추면, 지운 줄 뒤의 것이 **오류 없이 저장되지
// 않는다** — 사람은 지운 줄 하나만 사라졌다고 읽는다.
func formRows(f url.Values, prefix, suffix string) []int {
	var rows []int
	for k := range f {
		if len(k) <= len(prefix)+len(suffix) || !strings.HasPrefix(k, prefix) || !strings.HasSuffix(k, suffix) {
			continue
		}
		n, err := strconv.Atoi(k[len(prefix) : len(k)-len(suffix)])
		if err != nil || n < 0 {
			continue
		}
		rows = append(rows, n)
	}
	sort.Ints(rows)
	return rows
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

// splitLines — 줄 단위 입력. 빈 줄은 버린다.
func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// splitList — 쉼표나 공백으로 나눈 목록. 사람이 어느 쪽으로 적어도 받는다.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
