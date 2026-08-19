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
	// Nodes — 스코프 마스터: 노드↔IP. **관측 IP를 노드로 해소하는 유일한 근거다.**
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
		return d, fmt.Errorf("선언 파일: %w", err)
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

// Problem — 선언 자체가 앞뒤가 안 맞는 자리.
type Problem struct {
	// Where — 어디가 문제인가. 화면이 그 자리를 짚는 데 쓴다.
	Where string
	// What — 무엇이 잘못됐나.
	What string
	// Why — 그대로 두면 무슨 일이 생기나. **이것이 없으면 사람은 고칠 이유를 모른다.**
	Why string
}

// Check — 선언이 스스로 앞뒤가 맞는지 본다.
//
// **여기서 잡지 못하면 대조 결과가 조용히 틀린다.** 노드↔IP가 없거나 겹치면 관측 IP가
// 노드로 해소되지 않고, 그러면 선언 엣지와 영영 맞지 않아 **CONFIRMED여야 할 통신이
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

	// 노드↔IP — 해소의 근거다.
	byIP := map[string][]string{}
	named := map[string]bool{}
	for _, n := range d.Nodes {
		named[n.Name] = true
		if !inScope[n.Name] {
			out = append(out, Problem{
				Where: "nodes/" + n.Name,
				What:  "스코프에 없는 노드입니다",
				Why:   "대조 대상이 아니라 이 IP 표는 쓰이지 않습니다",
			})
		}
		if len(n.IPs) == 0 {
			out = append(out, Problem{
				Where: "nodes/" + n.Name,
				What:  "IP가 없습니다",
				Why:   "이 노드로 오는 관측 통신이 해소되지 않아 **선언한 엣지가 미관측으로, 관측된 엣지가 shadow로** 갈립니다",
			})
		}
		for _, ip := range n.IPs {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if net.ParseIP(ip) == nil {
				out = append(out, Problem{
					Where: "nodes/" + n.Name,
					What:  fmt.Sprintf("IP 형식이 아닙니다: %q", ip),
					Why:   "해소는 문자열이 정확히 맞을 때만 됩니다 — 포트나 호스트명을 적으면 맞지 않습니다",
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
				Where: "nodes",
				What:  fmt.Sprintf("%s 를 %s 가 함께 주장합니다", ip, strings.Join(owners, " · ")),
				Why:   "해소가 뒤에 오는 노드로 뒤집혀 **통신이 엉뚱한 노드에 붙습니다**",
			})
		}
	}
	for _, n := range d.Scope {
		if n != "" && !named[n] {
			out = append(out, Problem{
				Where: "scope/" + n,
				What:  "IP 표에 없습니다",
				// IP가 비어 있는 것과 **결과가 같다.** 한쪽만 약하게 말하면 고칠 이유의
				// 무게가 달라 보인다.
				Why: "이 노드로 오는 관측 통신이 해소되지 않아 **선언한 엣지가 미관측으로, 관측된 엣지가 shadow로** 갈립니다",
			})
		}
	}

	// 자산·엣지가 가리키는 노드.
	for _, a := range d.Assets {
		if a.Node != "" && !inScope[a.Node] {
			out = append(out, Problem{
				Where: "assets/" + a.Node + "/" + a.Component,
				What:  "스코프에 없는 노드를 가리킵니다",
				Why:   "선언만 있고 관측이 붙지 않아 **늘 미관측(UNOBSERVED)** 으로 남습니다",
			})
		}
	}
	for _, e := range d.Edges {
		if e.Src != "" && !inScope[e.Src] {
			out = append(out, Problem{
				Where: fmt.Sprintf("edges/%s→%s", e.Src, e.Dst),
				What:  "보내는 쪽이 스코프에 없습니다",
				Why:   "관측이 붙지 않아 늘 미관측으로 남습니다",
			})
		}
		if e.Port == 0 {
			out = append(out, Problem{
				Where: fmt.Sprintf("edges/%s→%s", e.Src, e.Dst),
				What:  "포트가 0입니다",
				Why:   "엣지 동일성에 포트가 들어가므로 관측된 엣지와 맞지 않습니다",
			})
		}
	}
	return out
}
