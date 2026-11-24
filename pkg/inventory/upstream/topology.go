// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package upstream

// pqcota 이력의 관측 토폴로지. **그림은 상류가 그린다**(inventory.LoadTopology · inventory.RenderTopology, 상류 v0.20.0의
// `pqcota-inventory -topology`와 같은 함수다). 여기서는 같은 입력에서 요약 표를 만든다: 노드마다 앱 · 같은 앱 키로 관측된
// 라이브러리 · 저장된 연결 기록. `dot`이 없는 기계에서도 읽을 수 있어야 하기 때문이다.
//
// 표를 만드는 규칙(앱 키 모으기, 같은 앱 키의 자산, 상대 IP를 등록 엔드포인트로 잇기)은 상류 렌더러의 규칙을 옮긴 것이다.
// 상류가 그 규칙을 공개 함수로 내지 않아 옮겼고, 어긋나지 않는지는 시험이 같은 입력의 DOT과 견준다(IC-S29).

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/posture"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
)

// 상대의 종류. 등록 엔드포인트는 **이 이력을 연 조직의 것만** 쓴다. 다른 출처(선언 · 다른 조직)로 이름을 보충하지 않는다.
const (
	PeerNode         = "node"         // 등록된 노드 하나로 이어졌다(또는 기록이 노드 ID를 들고 있다)
	PeerUnregistered = "unregistered" // 그 IP로 등록된 엔드포인트가 없다
	PeerShared       = "shared"       // 그 IP를 등록 노드 여럿이 함께 쓴다
	PeerNotIP        = "not-ip"       // 상대 주소가 IP가 아니다
)

// 기록의 역할. 관측한 노드가 연결을 열었는지 받았는지다.
const (
	RoleClient      = "client"
	RoleServer      = "server"
	RoleUnconfirmed = ""
)

// Topology — 이력에서 읽은 관측 토폴로지.
type Topology struct {
	// DOT — 상류 RenderTopology가 낸 그대로. 조직 전체다.
	DOT string
	// Summary — 상류가 센 수. Edges는 저장된 기록의 수이고 연결의 수가 아니다(같은 연결을 두 노드가 보면 둘이다).
	Summary inventory.TopologySummary
	// Nodes — 현재 스냅샷이 있는 노드. 노드 ID 순.
	Nodes []TopoNode
	// Endpoints — 이 조직에 등록된 엔드포인트의 수. 0이면 모든 상대가 주소로 나온다.
	Endpoints int
}

// TopoNode — 노드 하나의 현재 스냅샷에서 읽은 것.
type TopoNode struct {
	ID string
	// Name — 등록 엔드포인트의 이름. 등록되지 않았거나 ID와 같으면 비어 있다.
	Name string
	// Snapshot · Digest · Ruleset — 이력의 현재 스냅샷. Digest는 v1 내용 지문이고 작업 폴더의 스냅샷과 견주는 값이다.
	Snapshot, Digest, Ruleset string
	// At — Loaded면 마지막 적재 시각, 아니면 스냅샷이 만들어진 시각. 관측 시각이 아니다.
	At     time.Time
	Loaded bool
	Apps   []TopoApp
	// Records — 저장된 연결 기록 하나에 한 줄.
	Records []TopoRecord
}

// TopoApp — 그 노드의 연결 기록에 나온 앱 키 하나.
type TopoApp struct {
	Key   string
	Kinds []string
	// Libraries — 같은 스냅샷에서 같은 앱 키로 관측된 자산. 그 연결이 이것을 썼다는 근거가 아니다.
	Libraries []string
}

// TopoRecord — 저장된 연결 기록 하나.
type TopoRecord struct {
	// App — 관측한 노드 쪽의 앱 키. 귀속되지 않았으면 비어 있다.
	App  string
	Role string
	// Peer — 상대. PeerKind가 PeerNode면 노드 ID, 아니면 주소다.
	Peer     string
	PeerKind string
	// SharedBy — PeerShared일 때 그 IP를 쓰는 등록 노드의 수.
	SharedBy int
	// PeerHasSnapshot — 상대 노드의 스냅샷이 이 이력에 있다.
	PeerHasSnapshot bool
	Proto           string
	Port            uint32
	Group           string
	Count           uint64
	Posture         discoveryv1.QuantumPosture
}

// ReadTopology — 이력에서 노드마다의 현재 스냅샷과 등록 엔드포인트를 읽어 그린다. l은 이력 전체를 읽을 수 있는 저장소여야
// 한다(history.PgStore · history.MemStore). 오류는 Classify를 거쳐 연결 정보를 싣지 않는다.
func ReadTopology(l Lookup, eps inventory.EndpointLister) (Topology, error) {
	store, ok := l.(history.Store)
	if !ok {
		return Topology{}, fmt.Errorf("%w: this history cannot list its nodes", ErrUnreachable)
	}
	var t Topology
	err := Within(Timeout, func() error {
		in, err := inventory.LoadTopology(store, eps)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrUnreachable, Classify(err))
		}
		t = BuildTopology(in)
		return nil
	})
	return t, err
}

// BuildTopology — 같은 입력에서 상류의 DOT과 요약 표를 함께 만든다.
func BuildTopology(in inventory.TopologyInput) Topology {
	var t Topology
	t.DOT, t.Summary = inventory.RenderTopology(in)
	t.Endpoints = len(in.Endpoints)

	names := map[string]string{}
	byIP := map[string][]string{}
	for _, ep := range in.Endpoints {
		if ep.GetName() != "" {
			names[ep.GetNodeId()] = ep.GetName()
		}
		if ip := normIP(ep.GetIp()); ip != "" && !containsStr(byIP[ip], ep.GetNodeId()) {
			byIP[ip] = append(byIP[ip], ep.GetNodeId())
		}
	}
	observed := map[string]bool{}
	for _, s := range in.Snapshots {
		observed[s.NodeID] = true
	}

	snaps := append([]*history.Snapshot(nil), in.Snapshots...)
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].NodeID < snaps[j].NodeID })
	for _, s := range snaps {
		n := TopoNode{ID: s.NodeID, Snapshot: s.ID, Digest: history.ContentHashV1(s), Ruleset: s.RulesetVersion}
		if nm := names[s.NodeID]; nm != s.NodeID {
			n.Name = nm
		}
		if at, ok := in.Times[s.NodeID]; ok {
			n.At, n.Loaded = at.At, at.Loaded
		}

		kinds := map[string]map[string]bool{}
		for _, e := range s.Edges {
			if k := e.GetAppKey(); k != "" {
				if kinds[k] == nil {
					kinds[k] = map[string]bool{}
				}
				if kind := e.GetAppKeyKind(); kind != "" {
					kinds[k][kind] = true
				}
			}
		}
		for k, ks := range kinds {
			app := TopoApp{Key: k, Libraries: assetsWithAppKey(k, s.Findings)}
			for kind := range ks {
				app.Kinds = append(app.Kinds, kind)
			}
			sort.Strings(app.Kinds)
			n.Apps = append(n.Apps, app)
		}
		sort.Slice(n.Apps, func(i, j int) bool { return n.Apps[i].Key < n.Apps[j].Key })

		for _, e := range s.Edges {
			r := TopoRecord{
				App: e.GetAppKey(), Port: e.GetPort(), Group: e.GetNegotiatedGroup(), Count: e.GetObservedCount(),
				Proto:   strings.ToLower(strings.TrimPrefix(e.GetProtocol().String(), "NETWORK_PROTOCOL_")),
				Posture: posture.Classify(e.GetNegotiatedGroup(), e.GetCipher()),
			}
			switch e.GetRole() {
			case discoveryv1.EdgeRole_EDGE_ROLE_CLIENT:
				r.Role = RoleClient
			case discoveryv1.EdgeRole_EDGE_ROLE_SERVER:
				r.Role = RoleServer
			}
			r.Peer, r.PeerKind, r.SharedBy = resolvePeer(e, byIP)
			r.PeerHasSnapshot = r.PeerKind == PeerNode && observed[r.Peer]
			n.Records = append(n.Records, r)
		}
		sort.SliceStable(n.Records, func(i, j int) bool {
			a, b := n.Records[i], n.Records[j]
			switch {
			case a.App != b.App:
				return a.App < b.App
			case a.Peer != b.Peer:
				return a.Peer < b.Peer
			case a.Port != b.Port:
				return a.Port < b.Port
			case a.Proto != b.Proto:
				return a.Proto < b.Proto
			default:
				return a.Role < b.Role
			}
		})
		t.Nodes = append(t.Nodes, n)
	}
	return t
}

// Addresses — 노드로 잇지 못한 상대의 수(서로 다른 것만). 상류 Summary.Addresses와 같은 수다.
func (t Topology) Addresses() int {
	seen := map[string]bool{}
	for _, n := range t.Nodes {
		for _, r := range n.Records {
			if r.PeerKind != PeerNode {
				seen[r.Peer] = true
			}
		}
	}
	return len(seen)
}

// resolvePeer — 기록의 상대. 기록이 노드 ID를 들고 있으면 그것, 아니면 상대 IP가 **등록 엔드포인트 하나에만** 속할 때 그 노드다.
func resolvePeer(e *discoveryv1.ObservedEdge, byIP map[string][]string) (peer, kind string, shared int) {
	if d := e.GetDstNodeId(); d != "" {
		return d, PeerNode, 0
	}
	ip := peerIP(e)
	if ip == "" {
		return e.GetDstAddr(), PeerNotIP, 0
	}
	switch ns := byIP[ip]; len(ns) {
	case 1:
		return ns[0], PeerNode, 0
	case 0:
		return ip, PeerUnregistered, 0
	default:
		return ip, PeerShared, len(ns)
	}
}

// peerIP — 기록의 상대 IP(정규화). 역할을 다 믿지 않고 주소의 꼴로 읽는다. 연 쪽 기록은 「ip:port」이고(괄호 없는 IPv6는
// 끝의 「:포트」를 먼저 뗀다), 받은 쪽 · 미확인 기록은 포트 없는 IP다. 옛 기록은 역할과 꼴이 맞지 않을 수 있다.
func peerIP(e *discoveryv1.ObservedEdge) string {
	a := strings.TrimSpace(e.GetDstAddr())
	stripPort := func() string {
		if p := e.GetPort(); p != 0 {
			if suf := ":" + strconv.Itoa(int(p)); strings.HasSuffix(a, suf) {
				return normIP(strings.TrimSuffix(a, suf))
			}
		}
		return ""
	}
	if e.GetRole() == discoveryv1.EdgeRole_EDGE_ROLE_CLIENT {
		if ip := stripPort(); ip != "" {
			return ip
		}
	}
	if ip := normIP(a); ip != "" {
		return ip
	}
	if ip := stripPort(); ip != "" {
		return ip
	}
	if h, _, err := net.SplitHostPort(a); err == nil {
		return normIP(h)
	}
	return ""
}

// normIP — IP 표기를 하나로(괄호를 떼고, IPv4와 `::ffff:` 매핑은 a.b.c.d로). IP가 아니면 빈 문자열이다.
func normIP(s string) string {
	s = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), "["), "]")
	ip := net.ParseIP(s)
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}

// assetsWithAppKey — 같은 스냅샷의 자산 가운데 앱 키 key를 가진 것의 이름(겹치지 않게, 이름순).
func assetsWithAppKey(key string, findings []*discoveryv1.Finding) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range findings {
		if !containsStr(f.GetAppKeys(), key) {
			continue
		}
		if l := assetLabel(f); l != "" && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

// assetLabel — 자산의 짧은 이름. OpenSSL은 라이브러리와 판이다.
func assetLabel(f *discoveryv1.Finding) string {
	switch {
	case f.GetOpenssl() != nil:
		return strings.TrimSpace(f.GetOpenssl().GetLib() + " " + f.GetOpenssl().GetVersion())
	case f.GetJca() != nil:
		return "JCA (" + f.GetPqcReadiness() + ")"
	case f.GetCng() != nil:
		return "CNG (" + f.GetPqcReadiness() + ")"
	}
	return ""
}

func containsStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
