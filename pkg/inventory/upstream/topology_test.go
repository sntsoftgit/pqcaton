// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package upstream_test

import (
	"strings"
	"testing"
	"time"

	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	inventoryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/inventory/v1"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

func topoEdge(src, addr string, port uint32, role discoveryv1.EdgeRole, app string) *discoveryv1.ObservedEdge {
	e := &discoveryv1.ObservedEdge{SrcNodeId: src, DstAddr: addr, Port: port, Role: role,
		Protocol: discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_TLS, NegotiatedGroup: "X25519MLKEM768", ObservedCount: 3, AppKey: app}
	if app != "" {
		e.AppKeyKind = "exe-path"
	}
	if role == discoveryv1.EdgeRole_EDGE_ROLE_UNSPECIFIED {
		e.Protocol, e.NegotiatedGroup, e.ObservedCount = discoveryv1.NetworkProtocol_NETWORK_PROTOCOL_SSH, "", 0
	}
	return e
}

func lib(name, version string, apps ...string) *discoveryv1.Finding {
	return &discoveryv1.Finding{AppKeys: apps,
		RuntimeAxes: &discoveryv1.Finding_Openssl{Openssl: &discoveryv1.OpensslAxes{Lib: name, Version: version}}}
}

// topoInput — 경우를 다 든 이력 하나: 양쪽에서 본 연결, 받은 쪽만 본 연결, 역할 미확인, 등록되지 않은 주소, 등록 노드 둘이
// 함께 쓰는 IP, IP가 아닌 주소, 기록이 노드 ID를 든 상대(스냅샷 없음), 앱이 없는 기록, 같은 앱 키의 라이브러리.
func topoInput(eps []*inventoryv1.MachineEndpoint) inventory.TopologyInput {
	client, server, none := discoveryv1.EdgeRole_EDGE_ROLE_CLIENT, discoveryv1.EdgeRole_EDGE_ROLE_SERVER, discoveryv1.EdgeRole_EDGE_ROLE_UNSPECIFIED
	named := topoEdge("web", "10.0.0.99:9000", 9000, client, "")
	named.DstNodeId = "batch"
	return inventory.TopologyInput{
		Snapshots: []*history.Snapshot{
			{ID: "snap-web-000000000000", NodeID: "web", RulesetVersion: "pqcota-enrich/v4", Edges: []*discoveryv1.ObservedEdge{
				topoEdge("web", "10.0.0.2:8443", 8443, client, "/usr/bin/curl"),
				topoEdge("web", "203.0.113.9:443", 443, client, "/usr/bin/curl"),
				topoEdge("web", "10.0.0.7", 0, none, ""),
				topoEdge("web", "service.internal:443", 443, client, ""),
				named,
			}, Findings: []*discoveryv1.Finding{lib("libssl.so.3", "", "/usr/bin/curl"), lib("libcrypto.so.3", "3.5.8", "/usr/bin/curl", "/other")}},
			{ID: "snap-app-000000000000", NodeID: "app", RulesetVersion: "pqcota-enrich/v4", Edges: []*discoveryv1.ObservedEdge{
				topoEdge("app", "10.0.0.1", 8443, server, "/usr/sbin/nginx"),
				topoEdge("app", "::ffff:10.0.0.1", 22, server, ""),
				topoEdge("app", "10.0.0.50", 5432, server, "/usr/sbin/nginx"),
			}, Findings: []*discoveryv1.Finding{lib("libssl.so.3", "", "/usr/sbin/nginx")}},
		},
		Times: map[string]inventory.NodeTime{
			"web": {At: time.Date(2026, 11, 24, 10, 18, 31, 0, time.UTC), Loaded: true},
			"app": {At: time.Date(2026, 11, 20, 1, 2, 3, 0, time.UTC)},
		},
		Endpoints: eps,
	}
}

func ep(node, name, ip string) *inventoryv1.MachineEndpoint {
	return &inventoryv1.MachineEndpoint{NodeId: node, Name: name, Ip: ip}
}

// IC-S29 — **요약 표는 상류가 그린 그림과 같은 것을 센다.**
//
// 그림은 상류 RenderTopology가 그리고 표는 화면이 같은 입력에서 만든다. 표의 규칙(앱 키, 같은 앱 키의 라이브러리, 상대 IP를
// 등록 엔드포인트로 잇기)은 상류의 것을 옮긴 것이라, 어긋나면 그림과 표가 서로 다른 말을 한다. 기록 수 · 노드로 잇지 못한
// 상대의 수가 상류가 센 수와 같고, 표의 앱 · 라이브러리 · 상대가 모두 DOT에 있음을 본다. 엔드포인트는 준 것만 쓴다.
func TestTopologyTableAgreesWithTheUpstreamPicture(t *testing.T) {
	eps := []*inventoryv1.MachineEndpoint{
		ep("web", "Web", "10.0.0.1"), ep("app", "app", "10.0.0.2"),
		ep("db-a", "", "10.0.0.50"), ep("db-b", "", "10.0.0.50"), // 한 IP를 등록 노드 둘이 쓴다
	}
	top := upstream.BuildTopology(topoInput(eps))
	records := 0
	for _, n := range top.Nodes {
		records += len(n.Records)
	}
	if records != top.Summary.Edges || records != 8 {
		t.Errorf("the table has %d records, upstream drew %d lines, want 8", records, top.Summary.Edges)
	}
	if top.Addresses() != top.Summary.Addresses {
		t.Errorf("the table has %d peers not tied to a node, upstream drew %d address boxes", top.Addresses(), top.Summary.Addresses)
	}
	if len(top.Nodes) != 2 || top.Nodes[0].ID != "app" || top.Nodes[1].ID != "web" || top.Endpoints != 4 {
		t.Fatalf("nodes %+v, endpoints %d", top.Nodes, top.Endpoints)
	}
	app, web := top.Nodes[0], top.Nodes[1]
	if web.Name != "Web" || app.Name != "" || !web.Loaded || app.Loaded || web.Snapshot != "snap-web-000000000000" {
		t.Errorf("web %+v / app name %q loaded %v", web, app.Name, app.Loaded)
	}
	for _, n := range top.Nodes {
		if len(n.Digest) != 64 {
			t.Errorf("%s: the digest is %q, want the v1 content hash", n.ID, n.Digest)
		}
	}

	peers := map[string]upstream.TopoRecord{}
	for _, n := range top.Nodes {
		for _, r := range n.Records {
			peers[n.ID+" "+r.Role+" "+r.Peer] = r
		}
	}
	for key, want := range map[string]struct {
		kind    string
		hasSnap bool
		shared  int
	}{
		"web client app":                  {upstream.PeerNode, true, 0},          // 등록 IP 하나에만 속한다
		"web client 203.0.113.9":          {upstream.PeerUnregistered, false, 0}, // 등록되지 않은 주소
		"web  10.0.0.7":                   {upstream.PeerUnregistered, false, 0}, // 역할 미확인, 등록되지 않은 주소
		"web client service.internal:443": {upstream.PeerNotIP, false, 0},
		"web client batch":                {upstream.PeerNode, false, 0}, // 기록이 노드 ID를 들었고 스냅샷은 없다
		"app server web":                  {upstream.PeerNode, true, 0},  // 받은 쪽 기록의 포트 없는 IP와 ::ffff: 표기
		"app server 10.0.0.50":            {upstream.PeerShared, false, 2},
	} {
		got, ok := peers[key]
		if !ok || got.PeerKind != want.kind || got.PeerHasSnapshot != want.hasSnap || got.SharedBy != want.shared {
			t.Errorf("%q: %+v (found %v), want %+v", key, got, ok, want)
		}
	}

	if len(web.Apps) != 1 || web.Apps[0].Key != "/usr/bin/curl" || strings.Join(web.Apps[0].Kinds, ",") != "exe-path" ||
		strings.Join(web.Apps[0].Libraries, "|") != "libcrypto.so.3 3.5.8|libssl.so.3" {
		t.Errorf("web apps %+v", web.Apps)
	}
	if len(app.Apps) != 1 || strings.Join(app.Apps[0].Libraries, "|") != "libssl.so.3" {
		t.Errorf("app apps %+v", app.Apps)
	}
	// 표에 적는 것은 모두 그림에도 있다.
	for _, n := range top.Nodes {
		for _, a := range n.Apps {
			for _, s := range append([]string{a.Key}, a.Libraries...) {
				if !strings.Contains(top.DOT, s) {
					t.Errorf("%s: %q is in the table and not in the DOT", n.ID, s)
				}
			}
		}
		for _, r := range n.Records {
			if !strings.Contains(top.DOT, r.Peer) {
				t.Errorf("%s: the peer %q is in the table and not in the DOT", n.ID, r.Peer)
			}
		}
	}

	// 엔드포인트가 없으면 IP로는 아무 노드에도 잇지 않는다. 기록이 든 노드 ID만 노드다.
	bare := upstream.BuildTopology(topoInput(nil))
	nodes := 0
	for _, n := range bare.Nodes {
		if n.Name != "" {
			t.Errorf("%s has the name %q without a registered endpoint", n.ID, n.Name)
		}
		for _, r := range n.Records {
			if r.PeerKind == upstream.PeerNode {
				nodes++
				if r.Peer != "batch" {
					t.Errorf("without endpoints the peer %q was tied to a node", r.Peer)
				}
			}
		}
	}
	if nodes != 1 || bare.Endpoints != 0 || bare.Addresses() != bare.Summary.Addresses {
		t.Errorf("without endpoints: %d node peers, %d endpoints, %d vs %d addresses", nodes, bare.Endpoints, bare.Addresses(), bare.Summary.Addresses)
	}
}

// IC-S29 — 이력에서 읽는 길. 메모리 이력과 메모리 등록 목록에서 읽고, 노드를 늘어놓을 수 없는 저장소는 거절한다.
func TestReadTopologyFromTheHistory(t *testing.T) {
	st := history.NewMemStore()
	for _, s := range topoInput(nil).Snapshots {
		if err := st.Append(s); err != nil {
			t.Fatal(err)
		}
	}
	meta := inventory.NewMemMetaStore()
	if err := meta.UpsertEndpoint(ep("app", "", "10.0.0.2")); err != nil {
		t.Fatal(err)
	}
	top, err := upstream.ReadTopology(st, meta)
	if err != nil {
		t.Fatal(err)
	}
	if len(top.Nodes) != 2 || top.Endpoints != 1 || !strings.Contains(top.DOT, "digraph pqcota_topology") {
		t.Errorf("read %d nodes and %d endpoints", len(top.Nodes), top.Endpoints)
	}
	empty, err := upstream.ReadTopology(history.NewMemStore(), nil)
	if err != nil || len(empty.Nodes) != 0 || empty.Summary.Edges != 0 {
		t.Errorf("an empty history: %+v, %v", empty.Summary, err)
	}
	if _, err := upstream.ReadTopology(lookupOnly{st}, nil); err == nil {
		t.Error("a store that cannot list its nodes was accepted")
	}
}

// lookupOnly — Lookup만 내는 저장소(노드를 늘어놓지 못한다).
type lookupOnly struct{ l upstream.Lookup }

func (o lookupOnly) ByContentHashV1(n, r, d string) (*history.Snapshot, error) {
	return o.l.ByContentHashV1(n, r, d)
}
func (o lookupOnly) Latest(n string) (*history.Snapshot, error) { return o.l.Latest(n) }
