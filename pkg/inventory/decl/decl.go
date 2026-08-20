// Package decl — 선언 인벤토리의 파일 형식과 그 자체 검사.
//
// 선언은 **고객이 「있다」고 말하는 것**이다. 관측과 맞대는 한쪽 레인이라, 여기가 틀리면
// 대조 전체가 틀린다 — 그것도 **오류가 아니라 그럴듯한 결과**로 나온다.
//
// 명령(`pqcaton-report`)과 화면이 같은 파일을 읽고 쓰므로 형식이 한 곳에 있어야 한다.
// 형식이 두 벌이면 화면으로 쓴 것을 명령이 못 읽는 날이 온다.
package decl

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Declaration — 선언 문서 한 장.
type Declaration struct {
	// Comment — 사람이 읽는 머리말. 생성 도구가 남기는 자리다(데모의 declare.py).
	Comment string `json:"_comment,omitempty"`
	// Org — 이 선언이 어느 조직의 것인가. **선언 문서가 조직의 것이므로 여기 적힌다** —
	// 대조 엔진이 이 값으로 열리고, 다른 조직의 자산이 섞이면 대조하지 않고 끊는다.
	Org string `json:"org,omitempty"`
	// Scope — 등재된 노드 이름. 관측 상대가 여기 없으면 off-scope 로 표기된다(§0.4).
	Scope []string `json:"scope"`
	// Nodes — 스코프 마스터: 노드↔IP. **관측 IP를 노드로 잇는 유일한 근거다.**
	Nodes []Node `json:"nodes"`
	// Assets — 「이 노드에서 이것을 쓴다」. 자산 대조의 선언 레인.
	Assets []Asset `json:"assets"`
	// Edges — 「이 노드가 저 노드와 이렇게 통신한다」. 엣지 대조의 선언 레인.
	Edges []Edge `json:"edges"`
}

// Node — 노드 하나와 그 주소들. 한 노드가 망 둘에 걸치면 IP도 둘이다.
type Node struct {
	Name string   `json:"name"`
	IPs  []string `json:"ips"`
}

// Asset — 선언된 자산 하나.
type Asset struct {
	Node      string `json:"node"`
	Runtime   string `json:"runtime"`
	Component string `json:"component"`
}

// Edge — 선언된 통신 엣지 하나.
type Edge struct {
	Src   string `json:"src"`
	Dst   string `json:"dst"`
	Port  uint32 `json:"port"`
	Proto string `json:"proto"`
}

// DefaultOrg — 조직을 적지 않은 선언의 조직. 한 조직만 다루는 자리다.
const DefaultOrg = "local"

// OrgOrDefault — 적힌 조직, 비었으면 [DefaultOrg].
func (d Declaration) OrgOrDefault() string {
	if strings.TrimSpace(d.Org) == "" {
		return DefaultOrg
	}
	return d.Org
}

// Load — 선언 파일을 읽는다.
func Load(path string) (Declaration, error) {
	var d Declaration
	raw, err := os.ReadFile(path)
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, fmt.Errorf("declaration file: %w", err)
	}
	return d, nil
}

// Save — 선언 파일을 쓴다. 화면이 편집한 것을 되돌려 놓는 자리다.
func Save(path string, d Declaration) error {
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// Code — 무엇이 어긋났나. **문장이 아니라 코드다.**
//
// 같은 문제를 명령은 영어로 말하고 화면은 보는 사람의 말로 말해야 한다. 여기에 문장을
// 담으면 그 둘 중 하나는 반드시 남의 말로 뜬다.
//
// 영어 문장은 이 패키지가 갖고(아래 [Problem.What] · [Problem.Why]), 한국어는 화면
// 카탈로그가 갖는다 — **영어를 두 곳에 두지 않는 것**이 이 갈래의 요점이다.
type Code string

const (
	NodeNotInScope  Code = "node_not_in_scope"
	NodeHasNoIP     Code = "node_has_no_ip"
	IPMalformed     Code = "ip_malformed"
	IPClaimedTwice  Code = "ip_claimed_twice"
	NodeMissingIP   Code = "node_missing_from_ip_table"
	AssetOffScope   Code = "asset_points_off_scope"
	EdgeSrcOffScope Code = "edge_source_off_scope"
	EdgePortZero    Code = "edge_port_zero"
)

// Problem — 선언 자체가 앞뒤가 안 맞는 자리.
type Problem struct {
	// Where — 어디가 문제인가. 화면이 그 자리를 짚는 데 쓴다.
	Where string
	// Code — 무엇이 잘못됐나.
	Code Code
	// Detail — 값이 붙는 것만. 형식이 틀린 IP, 그 IP를 함께 주장하는 노드들.
	Detail string
}

// 영어 문장. **여기가 영어의 유일한 자리다.**
var (
	whatEN = map[Code]string{
		NodeNotInScope:  "this node is not in scope",
		NodeHasNoIP:     "this node has no IP",
		IPMalformed:     "not an IP address: %s",
		IPClaimedTwice:  "%s is claimed by more than one node",
		NodeMissingIP:   "missing from the IP table",
		AssetOffScope:   "points at a node that is not in scope",
		EdgeSrcOffScope: "the sending side is not in scope",
		EdgePortZero:    "the port is 0",
	}
	whyEN = map[Code]string{
		NodeNotInScope: "it is not reconciled, so this IP row is never used",
		NodeHasNoIP: "observed traffic to this node cannot be resolved, so **declared edges " +
			"show as unobserved and observed edges show as shadow**",
		IPMalformed: "resolution only works on an exact string match — a port or a hostname will not match",
		IPClaimedTwice: "resolution flips to whichever node comes last, so **traffic gets " +
			"attached to the wrong node**",
		// IP가 비어 있는 것과 **결과가 같다.** 한쪽만 약하게 말하면 고칠 이유의 무게가
		// 달라 보인다.
		NodeMissingIP: "observed traffic to this node cannot be resolved, so **declared edges " +
			"show as unobserved and observed edges show as shadow**",
		AssetOffScope:   "it is declared but no observation attaches, so it stays **UNOBSERVED** forever",
		EdgeSrcOffScope: "no observation attaches, so it stays unobserved forever",
		EdgePortZero:    "the port is part of an edge's identity, so it will not match an observed edge",
	}
)

// What — 무엇이 잘못됐나(영어).
func (p Problem) What() string { return fill(whatEN[p.Code], p.Detail) }

// Why — 그대로 두면 무슨 일이 생기나(영어).
//
// **이것이 없으면 사람은 고칠 이유를 모른다.**
func (p Problem) Why() string { return whyEN[p.Code] }

// fill — 값 자리가 있는 문장에만 값을 넣는다. 없는 문장에 넣으면 %!(EXTRA …) 가 붙는다.
func fill(tmpl, detail string) string {
	if !strings.Contains(tmpl, "%s") {
		return tmpl
	}
	return fmt.Sprintf(tmpl, detail)
}

// Check — 선언이 스스로 앞뒤가 맞는지 본다.
//
// **여기서 잡지 못하면 대조 결과가 조용히 틀린다.** 노드↔IP가 없거나 겹치면 관측 IP가
// 노드로 이어지지 않고, 그러면 선언 엣지와 영영 맞지 않아 **CONFIRMED여야 할 통신이
// shadow(UNDECLARED)로 올라온다**(§0.4, IC-N1) — 오류가 아니라 그럴듯한 결과라 눈으로는
// 안 잡힌다. 사람이 파일을 저장하기 전에 짚어 주는 것이 이 함수의 일이다.
//
// **막지는 않는다.** 선언은 고객의 문서이고, 아직 IP를 모르는 노드를 적어 두는 것도 정당한
// 상태다 — 무엇이 어긋나는지 말하되 저장을 거부하지 않는다.
func Check(d Declaration) []Problem {
	var out []Problem
	inScope := map[string]bool{}
	for _, n := range d.Scope {
		if strings.TrimSpace(n) == "" {
			continue
		}
		inScope[n] = true
	}

	// 노드↔IP — 잇는 근거다.
	byIP := map[string][]string{}
	named := map[string]bool{}
	for _, n := range d.Nodes {
		named[n.Name] = true
		if !inScope[n.Name] {
			out = append(out, Problem{Where: "nodes/" + n.Name, Code: NodeNotInScope})
		}
		if len(n.IPs) == 0 {
			out = append(out, Problem{Where: "nodes/" + n.Name, Code: NodeHasNoIP})
		}
		for _, ip := range n.IPs {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if net.ParseIP(ip) == nil {
				out = append(out, Problem{
					Where: "nodes/" + n.Name, Code: IPMalformed, Detail: strconv.Quote(ip),
				})
				continue
			}
			byIP[ip] = append(byIP[ip], n.Name)
		}
	}
	for ip, owners := range byIP {
		if len(owners) > 1 {
			sort.Strings(owners)
			out = append(out, Problem{
				Where: "nodes/" + strings.Join(owners, "+"), Code: IPClaimedTwice, Detail: ip,
			})
		}
	}
	for _, n := range d.Scope {
		if n != "" && !named[n] {
			out = append(out, Problem{Where: "scope/" + n, Code: NodeMissingIP})
		}
	}

	// 자산·엣지가 가리키는 노드.
	for _, a := range d.Assets {
		if a.Node != "" && !inScope[a.Node] {
			out = append(out, Problem{
				Where: "assets/" + a.Node + "/" + a.Component, Code: AssetOffScope,
			})
		}
	}
	for _, e := range d.Edges {
		if e.Src != "" && !inScope[e.Src] {
			out = append(out, Problem{
				Where: fmt.Sprintf("edges/%s→%s", e.Src, e.Dst), Code: EdgeSrcOffScope,
			})
		}
		if e.Port == 0 {
			out = append(out, Problem{
				Where: fmt.Sprintf("edges/%s→%s", e.Src, e.Dst), Code: EdgePortZero,
			})
		}
	}
	return out
}
