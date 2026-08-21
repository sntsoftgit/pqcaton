package decl_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
)

func whats(ps []decl.Problem) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString(string(p.Code) + " @ " + p.Where + ": " + p.What() + "\n")
	}
	return b.String()
}

// raised — 그 코드가 몇 번 올라왔나. **문구가 아니라 코드를 잰다** — 문구는 명령이
// 영어로, 화면이 보는 사람의 말로 내므로 케이스가 붙잡을 것이 아니다.
func raised(ps []decl.Problem, c decl.Code) int {
	n := 0
	for _, p := range ps {
		if p.Code == c {
			n++
		}
	}
	return n
}

// IC-D9 — **IP 없는 노드를 짚는다.**
//
// 잇는 근거가 그 표뿐이라, 없으면 관측 IP가 노드로 이어지지 않는다. 그러면 선언한 엣지는
// 미관측으로, 관측된 엣지는 shadow로 구분된다 — **오류가 아니라 그럴듯한 결과**라 눈으로는
// 안 잡힌다(IC-N1).
func TestCheckFlagsNodeWithoutIP(t *testing.T) {
	d := decl.Declaration{Scope: []string{"web", "db"}, Nodes: []decl.Node{{Name: "web"}}}
	ps := decl.Check(d)
	if raised(ps, decl.NodeHasNoIP) != 1 {
		t.Errorf("IP 없는 노드를 안 짚는다:\n%s", whats(ps))
	}
	if raised(ps, decl.NodeMissingIP) != 1 {
		t.Errorf("표에 아예 없는 스코프 노드를 안 짚는다:\n%s", whats(ps))
	}
	// 왜 문제인지 말해야 한다 — 안 그러면 사람이 고칠 이유를 모른다.
	for _, p := range decl.Check(d) {
		if p.Why() == "" {
			t.Errorf("사유가 비었다: %+v", p)
		}
	}
}

// IC-D10 — **같은 IP를 둘이 주장하면 짚는다.** IP가 뒤에 오는 노드로 이어져 통신이
// 엉뚱한 노드에 붙는다.
func TestCheckFlagsDuplicateIP(t *testing.T) {
	d := decl.Declaration{
		Scope: []string{"a", "b"},
		Nodes: []decl.Node{{Name: "a", IPs: []string{"10.0.0.1"}}, {Name: "b", IPs: []string{"10.0.0.1"}}},
	}
	if ps := decl.Check(d); raised(ps, decl.IPClaimedTwice) != 1 {
		t.Errorf("겹친 IP를 안 짚는다:\n%s", whats(ps))
	}
}

// IC-D17 — **같은 관측 이름을 둘이 주장하면 짚는다.**
//
// IP를 겹쳐 주장하는 것과 같은 사태입니다 — 이어지는 자리만 엣지에서 자산으로 옮겨
// 갔을 뿐입니다. 한쪽만 이기므로 그 기계의 자산이 남의 노드에 붙고, 진 노드의 선언한
// 자산은 미관측으로 남습니다. 대소문자는 가리지 않습니다 — 잇는 쪽이 안 가립니다.
func TestCheckFlagsDuplicateObservedName(t *testing.T) {
	d := decl.Declaration{
		Scope: []string{"a", "b"},
		Nodes: []decl.Node{
			{Name: "a", IPs: []string{"10.0.0.1"}, ObservedAs: []string{"node:1a2b"}},
			{Name: "b", IPs: []string{"10.0.0.2"}, ObservedAs: []string{"NODE:1A2B"}},
		},
	}
	if ps := decl.Check(d); raised(ps, decl.ObservedAsTwice) != 1 {
		t.Errorf("겹친 관측 이름을 안 짚는다:\n%s", whats(ps))
	}
}

// IC-D18 — **다른 노드의 이름을 관측 이름으로 적어도 짚는다.** 그 기계의 관측이 어느
// 쪽에 붙을지를 적은 순서가 정하게 된다.
func TestCheckFlagsObservedNameCollidingWithANodeName(t *testing.T) {
	d := decl.Declaration{
		Scope: []string{"a", "pay-db"},
		Nodes: []decl.Node{
			{Name: "a", IPs: []string{"10.0.0.1"}, ObservedAs: []string{"pay-db"}},
			{Name: "pay-db", IPs: []string{"10.0.0.2"}},
		},
	}
	if ps := decl.Check(d); raised(ps, decl.ObservedAsTwice) != 1 {
		t.Errorf("이름과 부딪히는 관측 이름을 안 짚는다:\n%s", whats(ps))
	}
	// 겹치지 않으면 짚지 않는다 — 관측 이름을 적었다는 것만으로 무는 검사가 되면 안 된다.
	ok := decl.Declaration{
		Scope: []string{"a"},
		Nodes: []decl.Node{{Name: "a", IPs: []string{"10.0.0.1"},
			ObservedAs: []string{"node:1a2b", "a.corp.example"}}},
	}
	if ps := decl.Check(ok); raised(ps, decl.ObservedAsTwice) != 0 {
		t.Errorf("겹치지 않는데 짚는다:\n%s", whats(ps))
	}
}

// IC-D11 — **IP 자리에 IP가 아닌 것이 오면 짚는다.** 잇기는 문자열이 정확히 맞을 때만
// 되므로, 포트가 붙거나 호스트명을 적으면 영영 안 맞는다.
func TestCheckFlagsBadIP(t *testing.T) {
	d := decl.Declaration{Scope: []string{"a"}, Nodes: []decl.Node{
		{Name: "a", IPs: []string{"10.0.0.1:8443", "db.internal"}},
	}}
	got := decl.Check(d)
	n := raised(got, decl.IPMalformed)
	if n != 2 {
		t.Errorf("형식이 아닌 것 %d개를 짚었다, want 2:\n%s", n, whats(got))
	}
}

// IC-D12 — **스코프 밖을 가리키는 자산·엣지를 짚는다.** 관측이 붙지 않아 늘 미관측으로
// 남는데, 그것이 「없다」로 읽힌다.
func TestCheckFlagsOutOfScopeRefs(t *testing.T) {
	d := decl.Declaration{
		Scope:  []string{"a"},
		Nodes:  []decl.Node{{Name: "a", IPs: []string{"10.0.0.1"}}},
		Assets: []decl.Asset{{Node: "없는노드", Runtime: "openssl", Component: "libssl"}},
		Edges:  []decl.Edge{{Src: "없는노드", Dst: "a", Port: 443, Proto: "TLS"}},
	}
	ps := decl.Check(d)
	if raised(ps, decl.AssetOffScope) != 1 {
		t.Errorf("스코프 밖 자산을 안 짚는다:\n%s", whats(ps))
	}
	if raised(ps, decl.EdgeSrcOffScope) != 1 {
		t.Errorf("스코프 밖 엣지를 안 짚는다:\n%s", whats(ps))
	}
}

// IC-D13 — **포트 0을 짚는다.** 엣지 동일성에 포트가 들어가므로 관측된 엣지와 맞지 않는다.
func TestCheckFlagsZeroPort(t *testing.T) {
	d := decl.Declaration{
		Scope: []string{"a", "b"},
		Nodes: []decl.Node{{Name: "a", IPs: []string{"10.0.0.1"}}, {Name: "b", IPs: []string{"10.0.0.2"}}},
		Edges: []decl.Edge{{Src: "a", Dst: "b", Proto: "TLS"}},
	}
	if ps := decl.Check(d); raised(ps, decl.EdgePortZero) != 1 {
		t.Errorf("포트 0을 안 짚는다:\n%s", whats(ps))
	}
}

// IC-D14 — **맞는 선언은 조용하다.** 막는 것만 재고 통과를 안 재면, 전부 짚어도 케이스는
// 통과한다.
func TestCheckQuietWhenSound(t *testing.T) {
	d := decl.Declaration{
		Org:    "acme",
		Scope:  []string{"web", "db"},
		Nodes:  []decl.Node{{Name: "web", IPs: []string{"10.0.0.1", "10.0.1.1"}}, {Name: "db", IPs: []string{"10.0.0.2"}}},
		Assets: []decl.Asset{{Node: "web", Runtime: "openssl", Component: "libssl"}},
		Edges:  []decl.Edge{{Src: "web", Dst: "db", Port: 5432, Proto: "TLS"}},
	}
	if got := decl.Check(d); len(got) != 0 {
		t.Errorf("맞는 선언인데 %d곳을 짚는다:\n%s", len(got), whats(got))
	}
}

// IC-D15 — 파일 왕복. 화면이 쓴 것을 명령이 그대로 읽어야 한다.
func TestLoadSaveRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "declaration.json")
	in := decl.Declaration{
		Comment: "생성 도구가 남긴 머리말", Org: "acme", Scope: []string{"web"},
		Nodes:  []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}}},
		Assets: []decl.Asset{{Node: "web", Runtime: "jca", Component: "provider-chain"}},
		Edges:  []decl.Edge{{Src: "web", Dst: "web", Port: 443, Proto: "TLS"}},
	}
	if err := decl.Save(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := decl.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.Org != in.Org || out.Comment != in.Comment ||
		len(out.Nodes) != 1 || out.Nodes[0].IPs[0] != "10.0.0.1" ||
		len(out.Assets) != 1 || len(out.Edges) != 1 || out.Edges[0].Port != 443 {
		t.Fatalf("왕복에서 달라졌다: %+v", out)
	}
}

// IC-D16 — 조직을 안 적으면 `local` 이다. 한 조직만 다루는 자리의 기본값이다.
func TestOrgOrDefault(t *testing.T) {
	if got := (decl.Declaration{}).OrgOrDefault(); got != decl.DefaultOrg {
		t.Errorf("빈 조직 = %q, want %q", got, decl.DefaultOrg)
	}
	if got := (decl.Declaration{Org: "  "}).OrgOrDefault(); got != decl.DefaultOrg {
		t.Errorf("공백 조직 = %q", got)
	}
	if got := (decl.Declaration{Org: "acme"}).OrgOrDefault(); got != "acme" {
		t.Errorf("적힌 조직 = %q", got)
	}
}
