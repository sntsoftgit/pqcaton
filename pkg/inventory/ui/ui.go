// Package ui — 화면을 그리고 폼을 읽는 것.
//
// **어디서 읽고 어디에 쓰는지, 누가 들어올 수 있는지는 부르는 쪽이 정한다.** 로컬 명령은
// 파일에서 읽고 인증이 없으며, 컨트롤 플레인은 DB에서 읽고 앞에 인증이 선다 — 두 배포
// 형태의 차이가 그 둘뿐이라, 나머지(화면)를 여기 한 벌만 둔다.
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
	// Nav — 화면 사이를 오가는 자리. 부르는 쪽이 무엇을 열었는지에 따라 다르다.
	Nav []Link
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
type DeclView struct {
	Page
	Decl decl.Declaration
	// Problems — 선언이 스스로 앞뒤가 안 맞는 자리. **저장을 막지 않고 짚기만 한다** —
	// 아직 IP를 모르는 노드를 적어 두는 것도 정당한 상태다.
	Problems []decl.Problem
	// Blank — 빈 줄을 몇 개 더 보여 줄까. 없으면 항목을 새로 못 넣는다.
	Blank int
}

// DefaultBlank — 표마다 열어 두는 빈 줄 수.
const DefaultBlank = 3

// NewDeclView — 선언을 화면이 보는 모양으로 옮기고 스스로 검사한다.
func NewDeclView(d decl.Declaration, page Page) DeclView {
	return DeclView{Page: page, Decl: d, Problems: decl.Check(d), Blank: DefaultBlank}
}

// RenderDecl — 선언 편집 화면을 쓴다.
func RenderDecl(w io.Writer, v DeclView) error {
	return declPage(v).Render(context.Background(), w)
}

// RenderRow — 「행 추가」가 돌려주는 조각: 빈 줄 하나와, 번호가 하나 오른 버튼.
//
// **화면과 같은 조각을 쓴다**(decl.templ 의 nodeRow 들). 폼 이름이 곧 저장 경로라, 두
// 벌이 되면 새로 넣은 줄만 조용히 저장되지 않는다.
func RenderRow(w io.Writer, kind string, i int) error {
	return rowFragment(kind, i).Render(context.Background(), w)
}

// ApplyDecl — 폼에서 온 값으로 선언을 다시 만든다.
//
// **얹는 것이 아니라 다시 만든다.** 표에서 줄을 지우는 방법이 「이름을 비우는 것」이므로,
// 기존 것에 얹으면 지운 줄이 되살아난다. `_comment` 는 생성 도구가 남긴 머리말이라 그대로
// 들고 간다 — 사람이 편집할 자리가 아니다.
func ApplyDecl(prev decl.Declaration, f url.Values) decl.Declaration {
	d := decl.Declaration{Comment: prev.Comment, Org: strings.TrimSpace(f.Get("org"))}

	for _, n := range splitLines(f.Get("scope")) {
		d.Scope = append(d.Scope, n)
	}
	for i := 0; ; i++ {
		name, ok := f["node.name."+strconv.Itoa(i)]
		if !ok {
			break
		}
		nm := strings.TrimSpace(first(name))
		if nm == "" {
			continue // 이름을 비우면 지운 것이다
		}
		d.Nodes = append(d.Nodes, decl.Node{
			Name: nm, IPs: splitList(f.Get("node.ips." + strconv.Itoa(i))),
		})
	}
	for i := 0; ; i++ {
		node, ok := f["asset.node."+strconv.Itoa(i)]
		if !ok {
			break
		}
		nd := strings.TrimSpace(first(node))
		comp := strings.TrimSpace(f.Get("asset.component." + strconv.Itoa(i)))
		if nd == "" || comp == "" {
			continue
		}
		d.Assets = append(d.Assets, decl.Asset{
			Node: nd, Runtime: strings.TrimSpace(f.Get("asset.runtime." + strconv.Itoa(i))),
			Component: comp,
		})
	}
	for i := 0; ; i++ {
		src, ok := f["edge.src."+strconv.Itoa(i)]
		if !ok {
			break
		}
		s := strings.TrimSpace(first(src))
		dst := strings.TrimSpace(f.Get("edge.dst." + strconv.Itoa(i)))
		if s == "" || dst == "" {
			continue
		}
		port, _ := strconv.ParseUint(strings.TrimSpace(f.Get("edge.port."+strconv.Itoa(i))), 10, 32)
		d.Edges = append(d.Edges, decl.Edge{
			Src: s, Dst: dst, Port: uint32(port),
			Proto: strings.TrimSpace(f.Get("edge.proto." + strconv.Itoa(i))),
		})
	}
	return d
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
