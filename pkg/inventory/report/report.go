// Package report — 여러 노드의 관측 결과를 모아 선언과 대조한 것.
//
// **계산만 한다.** 명령은 이것을 글로 찍고 화면은 표로 그린다 — 계산이 두 곳에 있으면 화면과
// 명령이 다른 답을 내는 날이 오고, 그때 어느 쪽이 맞는지 아무도 모른다.
//
// 대조 자체는 `reconcile` 이 한다. 여기가 맡는 것은 **여러 결과를 레인으로 가르고, 관측
// IP를 노드로 잇고, 무엇을 못 봤는지 세는 것** — 한 대짜리 경로(`pqcaton-decide`)에는
// 없고 여러 노드를 모을 때만 생기는 일이다.
package report

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/history"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

// Result — 대조 한 판의 결과.
type Result struct {
	Org string
	// SeenBy — 노드 → 그 노드를 본 collector 들. **누가 무엇을 봤는지**가 없으면 「관측 안 됨」과
	// 「관측하지 못함」을 가를 수 없다.
	SeenBy map[string][]string
	// Uncovered — 스코프에 있으나 네트워크 계층을 관측하지 못한 노드. **부재가 아니라 미관측**이다.
	Uncovered map[string]bool
	// AssetGaps — 완전성 맵의 미커버 계층. UNOBSERVED 를 재수집 후보로 가르는 입력이다.
	AssetGaps []string

	Assets []reconcile.Reconciled
	Edges  []reconcile.ReconciledEdge

	// 센 것. 화면과 글이 같은 수를 말하게 한다.
	ObservedAssets int
	ObservedEdges  int
	DeclaredAssets int
	DeclaredEdges  int
	Nodes          int

	// Skipped — 읽지 못한 결과 파일. **조용히 넘기지 않는다** — 빠진 노드를 모르면
	// 「관측 안 됨」과 「못 읽음」이 뒤섞인다.
	Skipped []string
}

// Build — 결과 디렉터리와 선언을 받아 대조한다.
func Build(dir string, d decl.Declaration) (*Result, error) {
	orgName := d.OrgOrDefault()
	eng, err := reconcile.For(org.ID(orgName))
	if err != nil {
		return nil, err
	}
	results, skipped, err := LoadResults(dir)
	if err != nil {
		return nil, err
	}

	out := &Result{
		Org: orgName, SeenBy: map[string][]string{}, Uncovered: map[string]bool{},
		Nodes: len(d.Scope), Skipped: skipped,
	}

	// 관측 자산(openssl)과 관측 엣지(network)를 레인별로 가른다.
	var observedAssets []reconcile.Observed
	var observedEdges []*discoveryv1.ObservedEdge
	covered := map[string]bool{}
	for _, res := range results {
		node := ResolveAssetNode(res, d.Nodes)
		if len(res.GetObservedEdges()) > 0 || HasNetworkLayer(res) {
			// 네트워크 레인. NETWORK 계층을 **실제로 커버**했으면 covered — 서버 전용 노드는
			// client 엣지가 0이어도 관측은 수행됐다(collector 미설치가 아니라 강등만 미커버).
			observedEdges = append(observedEdges, res.GetObservedEdges()...)
			if NetworkCovered(res) {
				covered[node] = true
			}
			continue
		}
		snap, err := normalize.Normalize([]*discoveryv1.CollectionResult{res},
			"snap", node, "ruleset-demo", history.NewMemStore(), nil)
		if err != nil {
			return nil, fmt.Errorf("normalizing %s: %w", node, err)
		}
		observedAssets = append(observedAssets, eng.AssetsFromSnapshot(snap)...)
		out.AssetGaps = append(out.AssetGaps, reconcile.GapLayers(snap)...)
		out.SeenBy[node] = append(out.SeenBy[node], res.GetEnvelope().GetCollectorId())
	}

	declaredAssets := make([]reconcile.AssetKey, 0, len(d.Assets))
	for _, a := range d.Assets {
		declaredAssets = append(declaredAssets, reconcile.AssetKey{
			Org: org.ID(orgName), NodeID: a.Node, Runtime: a.Runtime, Component: a.Component})
	}
	if out.Assets, err = eng.Reconcile(declaredAssets, observedAssets, out.AssetGaps); err != nil {
		return nil, err
	}

	// 관측 IP → 스코프 노드 잇기(§0.4). 이어지면 CONFIRMED 로 잡히고, 안 되면 off-scope 다.
	ResolveEdgeDsts(observedEdges, d.Nodes)

	declaredEdges := make([]reconcile.EdgeKey, 0, len(d.Edges))
	for _, e := range d.Edges {
		declaredEdges = append(declaredEdges, reconcile.EdgeKey{
			Org: org.ID(orgName), Src: e.Src, Dst: e.Dst, Port: e.Port, Proto: e.Proto})
	}
	scope := map[string]bool{}
	for _, n := range d.Scope {
		scope[n] = true
	}
	if out.Edges, err = eng.ReconcileEdges(declaredEdges, observedEdges, scope, nil); err != nil {
		return nil, err
	}

	for _, n := range d.Scope {
		if !covered[n] {
			out.Uncovered[n] = true
		}
	}
	out.ObservedAssets, out.ObservedEdges = len(observedAssets), len(observedEdges)
	out.DeclaredAssets, out.DeclaredEdges = len(declaredAssets), len(declaredEdges)
	return out, nil
}

// Counts — 3-상태별 자산 수와 자동통과 후보 수.
func (r *Result) Counts() (confirmed, undeclared, unobserved int) {
	for _, rec := range r.Assets {
		switch rec.State {
		case reconcile.Confirmed:
			confirmed++
		case reconcile.Undeclared:
			undeclared++
		case reconcile.Unobserved:
			unobserved++
		}
	}
	return
}

// Postures — 엣지 등급 합계(🟢 PQC · 🔴 고전 · ⚪ 불명).
func (r *Result) Postures() (pqc, classical, unknown int) {
	for _, e := range r.Edges {
		switch e.Posture {
		case discoveryv1.QuantumPosture_QUANTUM_POSTURE_PQC_HYBRID:
			pqc++
		case discoveryv1.QuantumPosture_QUANTUM_POSTURE_CLASSICAL:
			classical++
		default:
			unknown++
		}
	}
	return
}

// SeenNodes — collector 가 본 노드 이름, 정렬해서.
func (r *Result) SeenNodes() []string {
	out := make([]string, 0, len(r.SeenBy))
	for n := range r.SeenBy {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// UncoveredNodes — 통신을 관측하지 못한 노드, 정렬해서.
func (r *Result) UncoveredNodes() []string {
	out := make([]string, 0, len(r.Uncovered))
	for n := range r.Uncovered {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ObservedByRuntime — 런타임별 관측 자산 수. 「무엇을 실제로 쓰고 있나」를 한 줄로 말한다.
func (r *Result) ObservedByRuntime() map[string]int {
	out := map[string]int{}
	for _, rec := range r.Assets {
		if rec.State == reconcile.Confirmed || rec.State == reconcile.Undeclared {
			out[rec.Key.Runtime]++
		}
	}
	return out
}

// GapLayers — 못 본 계층, 중복 없이 정렬해서.
func (r *Result) GapLayers() []string { return Uniq(r.AssetGaps) }

// ── 재료 ───────────────────────────────────────────────────────────────────

// LoadResults — 노드들이 낸 CollectionResult JSON 을 읽는다.
//
// **한 파일이 깨졌다고 전부 멈추지 않는다.** 다만 조용히 넘기지도 않는다 — 빠진 노드를
// 모르면 「관측 안 됨」과 「못 읽음」이 뒤섞인다.
func LoadResults(dir string) (out []*discoveryv1.CollectionResult, skipped []string, err error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(paths)
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			skipped = append(skipped, filepath.Base(p)+": "+err.Error())
			continue
		}
		res := &discoveryv1.CollectionResult{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, res); err != nil {
			skipped = append(skipped, filepath.Base(p)+": "+err.Error())
			continue
		}
		out = append(out, res)
	}
	return out, skipped, nil
}

// ResolveEdgeDsts — 관측 상대의 IP 를 스코프 노드로 바꾼다(§0.4).
//
// **잘못 이으면 CONFIRMED 여야 할 통신이 shadow 로 올라온다** — 오류가 아니라 그럴듯한
// 결과라 눈으로는 안 잡힌다(IC-N1). 이미 이어진 것은 덮지 않는다.
// ResolveAssetNode — 관측 결과 하나가 **선언의 어느 노드**의 것인가.
//
// 자산 대조는 노드 이름이 글자 그대로 같아야 맞는다. 그런데 collector 는 자기가 붙인
// id(`node:<해시>`)나 호스트명으로 보낸다 — 이름이 서로 다르면 선언한 자산은 전부 미관측으로,
// 관측된 자산은 전부 shadow 로 올라온다. **막히지 않고 그럴듯하게 틀린다.**
//
// 그래서 봉투가 들고 온 이름들(대상 노드 id · fqdn · 짧은 호스트명 · machine-id)을 선언의
// 이름 및 「관측 이름」(`observed_as`)과 맞대 본다. 대소문자는 가리지 않는다 — 호스트명은
// 오는 길에 대소문자가 곧잘 바뀐다.
//
// **어디에도 안 걸리면 관측이 부른 이름을 그대로 쓴다.** 화면이 그 이름으로 shadow 를
// 올리므로, 사람이 그것을 보고 「관측 이름」에 적어 넣을 수 있다. 여기서 억지로 하나를
// 고르면 남의 노드 자산이 붙는다.
func ResolveAssetNode(res *discoveryv1.CollectionResult, nodes []decl.Node) string {
	id := res.GetEnvelope().GetTargetNodeId()
	m := res.GetEnvelope().GetMachine()
	seen := []string{id, m.GetFqdn(), shortHost(m.GetFqdn()), m.GetMachineId(), m.GetSelfAssignedId()}

	// **이름이 관측 이름을 이긴다.** 그리고 겹친 관측 이름은 먼저 적힌 노드가 가진다 —
	// 겹친다는 사실 자체는 `decl.Check` 가 짚는다(ObservedAsTwice). 여기서 뒤에 적힌
	// 것으로 뒤집으면 같은 파일이 순서만 바뀌어도 자산이 다른 노드에 붙는다.
	byName := map[string]string{}
	claim := func(key, name string) {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			return
		}
		if _, taken := byName[key]; !taken {
			byName[key] = name
		}
	}
	for _, n := range nodes {
		claim(n.Name, strings.TrimSpace(n.Name))
	}
	for _, n := range nodes {
		for _, a := range n.ObservedAs {
			claim(a, strings.TrimSpace(n.Name))
		}
	}
	for _, s := range seen {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if name, ok := byName[strings.ToLower(s)]; ok {
			return name
		}
	}
	return id
}

// shortHost — `web-gw.corp.example` 의 `web-gw`. 선언에는 짧은 이름을 적고 관측은 fqdn 으로
// 오는 것이 흔하다.
func shortHost(fqdn string) string {
	if i := strings.Index(fqdn, "."); i > 0 {
		return fqdn[:i]
	}
	return fqdn
}

func ResolveEdgeDsts(edges []*discoveryv1.ObservedEdge, nodes []decl.Node) {
	ip2node := map[string]string{}
	for _, n := range nodes {
		for _, ip := range n.IPs {
			ip2node[ip] = n.Name
		}
	}
	for _, e := range edges {
		if e.GetDstNodeId() != "" {
			continue
		}
		host := e.GetDstAddr()
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if node, ok := ip2node[strings.TrimSpace(host)]; ok {
			e.DstNodeId = node
		}
	}
}

// NetworkCovered — NETWORK 계층을 실제로 캡처했는가. **강등은 커버가 아니다** — 세면 못 본
// 노드가 「봤다」가 되어 토폴로지의 점선이 실선이 된다.
func NetworkCovered(res *discoveryv1.CollectionResult) bool {
	for _, l := range res.GetCompleteness().GetLayersCovered() {
		if l == commonv1.CollectionLayer_COLLECTION_LAYER_NETWORK {
			return true
		}
	}
	return false
}

// HasNetworkLayer — 네트워크 레인의 결과인가. 레인 분류용이라 **강등도 포함**한다.
func HasNetworkLayer(res *discoveryv1.CollectionResult) bool {
	if NetworkCovered(res) {
		return true
	}
	for _, l := range res.GetCompleteness().GetLayersMissing() {
		if l == commonv1.CollectionLayer_COLLECTION_LAYER_NETWORK {
			return true
		}
	}
	return false
}

// Uniq — 중복을 지우되 **처음 순서를 지키고 입력을 덮지 않는다.**
func Uniq(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// LayerLabel — 관측 계층 이름을 **사람이 읽는 말로.**
//
// 상류(pqcota)의 enum 상수가 그대로 화면에 뜨면(`COLLECTION_LAYER_ARTIFACT`) 읽는
// 사람은 무엇을 못 봤다는 것인지 알 수 없다. 계층은 **관측이 어디서 왔나**를 말하므로,
// 그것을 그대로 적는다.
//
// **원래 이름을 괄호에 남긴다.** pqcota 의 문서와 로그는 그 이름을 쓰므로, 옮기기만
// 하면 두 쪽을 잇지 못한다. 모르는 값은 그대로 낸다 — 상류에 계층이 늘었을 때
// 조용히 「불명」으로 뭉개지 않는다.
func LayerLabel(name string) string {
	short := strings.TrimPrefix(name, "COLLECTION_LAYER_")
	switch short {
	case "SOURCE":
		return "source code (SOURCE)"
	case "ARTIFACT":
		return "build artifacts — binaries and packages (ARTIFACT)"
	case "PROCESS":
		return "running processes (PROCESS)"
	case "NETWORK":
		return "actual traffic (NETWORK)"
	case "JVM_INTROSPECTION":
		return "inside the JVM — JCA (JVM_INTROSPECTION)"
	case "CNG_INTROSPECTION":
		return "registered CNG providers on the machine (CNG_INTROSPECTION)"
	}
	return name
}
