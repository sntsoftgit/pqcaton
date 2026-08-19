package ui_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func sample() decl.Declaration {
	return decl.Declaration{
		Org: "acme", Scope: []string{"web"},
		Nodes:  []decl.Node{{Name: "web", IPs: []string{"10.0.0.1", "10.0.1.1"}}},
		Assets: []decl.Asset{{Node: "web", Runtime: "openssl", Component: "libssl"}},
		Edges:  []decl.Edge{{Src: "web", Dst: "web", Port: 443, Proto: "TLS"}},
	}
}

// IC-UI1 — **화면이 있는 것을 다 보여 준다.** 표에 안 나온 줄은 폼으로 돌아오지 않으므로
// 저장하는 순간 사라진다 — 화면이 곧 데이터 손실의 자리가 된다.
func TestDeclRenderShowsEverything(t *testing.T) {
	var b strings.Builder
	if err := ui.RenderDecl(&b, ui.NewDeclView(sample(), ui.Page{Title: "선언"})); err != nil {
		t.Fatal(err)
	}
	body := b.String()
	for _, want := range []string{
		`name="org"`, `name="scope"`,
		`name="node.name.0"`, "10.0.0.1, 10.0.1.1", // 여러 IP를 한 칸에
		`name="asset.component.0"`, "libssl",
		`name="edge.port.0"`, "443",
		`name="node.name.1"`, // 빈 줄이 열려 있어야 새로 넣을 수 있다
	} {
		if !strings.Contains(body, want) {
			t.Errorf("화면에 %q 가 없다", want)
		}
	}
}

// IC-UI2 — **폼을 다시 읽으면 같은 선언이 나온다.** 그리기와 읽기가 어긋나면 저장할 때마다
// 조용히 달라진다.
func TestApplyDeclRoundTrip(t *testing.T) {
	in := sample()
	f := url.Values{
		"org": {in.Org}, "scope": {"web"},
		"node.name.0": {"web"}, "node.ips.0": {"10.0.0.1, 10.0.1.1"},
		"asset.node.0": {"web"}, "asset.runtime.0": {"openssl"}, "asset.component.0": {"libssl"},
		"edge.src.0": {"web"}, "edge.dst.0": {"web"}, "edge.port.0": {"443"}, "edge.proto.0": {"TLS"},
	}
	got := ui.ApplyDecl(in, f)
	if got.Org != "acme" || len(got.Scope) != 1 {
		t.Fatalf("머리 부분이 다르다: %+v", got)
	}
	if len(got.Nodes) != 1 || len(got.Nodes[0].IPs) != 2 || got.Nodes[0].IPs[1] != "10.0.1.1" {
		t.Fatalf("노드가 다르다: %+v", got.Nodes)
	}
	if len(got.Assets) != 1 || len(got.Edges) != 1 || got.Edges[0].Port != 443 {
		t.Fatalf("자산·엣지가 다르다: %+v %+v", got.Assets, got.Edges)
	}
	if len(decl.Check(got)) != 0 {
		t.Errorf("맞는 선언인데 짚는다: %+v", decl.Check(got))
	}
}

// IC-UI3 — **IP는 쉼표로도 공백으로도 받는다.** 사람이 어느 쪽으로 적는지 외우게 하지 않는다.
func TestApplyDeclSplitsIPsEitherWay(t *testing.T) {
	for _, in := range []string{"10.0.0.1, 10.0.1.1", "10.0.0.1 10.0.1.1", "10.0.0.1,10.0.1.1"} {
		got := ui.ApplyDecl(decl.Declaration{}, url.Values{
			"node.name.0": {"web"}, "node.ips.0": {in},
		})
		if len(got.Nodes) != 1 || len(got.Nodes[0].IPs) != 2 {
			t.Errorf("%q → %+v", in, got.Nodes)
		}
	}
}

// IC-UI4 — **머리말은 사람이 편집할 자리가 아니다.** 생성 도구(declare.py)가 남긴 것이라
// 폼에 없다 — 그래도 저장에서 사라지면 안 된다.
func TestApplyDeclKeepsComment(t *testing.T) {
	prev := decl.Declaration{Comment: "declare.py 가 생성했습니다"}
	got := ui.ApplyDecl(prev, url.Values{"org": {"acme"}})
	if got.Comment != prev.Comment {
		t.Errorf("머리말이 사라졌다: %q", got.Comment)
	}
}

// IC-UI5 — 리뷰 큐는 **정책으로 묶여** 그려진다(§3.4). 한 줄씩 늘어놓으면 화면이 있어도
// 수천 대에서 리뷰가 끝나지 않는다.
func TestReviewViewGroupsByPolicy(t *testing.T) {
	sf := review.Session{Scope: "host://local",
		PolicyDecisions: map[string]string{"openssl/libssl": "교체한다"},
		Items: []review.Item{
			{ID: "a", Policy: "openssl/libssl", Mandatory: true},
			{ID: "b", Policy: "openssl/libssl"},
			{ID: "c", Policy: "jca/provider"},
		},
	}
	v := ui.NewReviewView(sf, ui.Page{Title: "리뷰 큐"})
	if len(v.Policies) != 2 {
		t.Fatalf("정책 묶음 %d개: %+v", len(v.Policies), v.Policies)
	}
	// 정렬해야 실행마다 순서가 흔들리지 않는다.
	if v.Policies[0].Name != "jca/provider" {
		t.Errorf("정렬되지 않았다: %s", v.Policies[0].Name)
	}
	for _, g := range v.Policies {
		if g.Name == "openssl/libssl" {
			if len(g.Items) != 2 || g.Mandatory != 1 || g.Conclusion != "교체한다" {
				t.Errorf("묶음이 다르다: %+v", g)
			}
		}
	}
}

// IC-UI6 — 리뷰 폼을 읽으면 세션에 그대로 얹힌다.
func TestApplyReview(t *testing.T) {
	sf := review.Session{
		PolicyDecisions: map[string]string{"p": ""},
		Items:           []review.Item{{ID: "a", Policy: "p"}},
	}
	got := ui.ApplyReview(sf, url.Values{
		"reviewer": {"보안팀"}, "signature": {"sig"},
		"policy:p": {"교체한다"}, "item:a": {"예외"}, "plan:a": {"on"},
	})
	if got.Reviewer != "보안팀" || got.Signature != "sig" {
		t.Errorf("승인 정보가 안 얹혔다: %+v", got)
	}
	if got.PolicyDecisions["p"] != "교체한다" || got.Items[0].Conclusion != "예외" || !got.Items[0].Plan {
		t.Errorf("판정이 안 얹혔다: %+v", got)
	}
}
