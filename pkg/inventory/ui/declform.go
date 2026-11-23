// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

// 선언 양식을 읽고, 저장하면 안 되는 칸을 짚는다.
//
// **입력 실수가 삭제나 다른 값으로 바뀌어 저장되지 않게 한다.** 예전에는 이름을 비운 줄을 「지운 줄」로 읽어 노드와 그
// 자산이 표시 없이 사라졌고, 숫자가 아닌 포트는 0으로 저장됐다. 지우려는 뜻은 「제거」 버튼(묻고 지운다)이 받는다. 양식은
// 지운 줄을 아예 싣지 않으므로, 실려 온 줄의 빈 이름은 지운 것이 아니라 잘못 적은 것이다.
//
// 저장을 막는 것은 **빠진 칸(이름 · IP · 컴포넌트 · 엣지의 끝)과 그 칸의 형식이 아닌 값**이다. 선언의 앞뒤가 안 맞는
// 자리(decl.Check: 겹친 IP · 포트 0 등)는 전처럼 저장하고 짚는다.

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

// 브라우저와 서버가 같은 식을 쓴다. 칸의 pattern 속성과 서버의 검사가 이 상수에서 나온다. 한쪽만 고칠 수 없다.
//
// 공백은 빈칸과 탭만 센다. `\s`는 브라우저와 Go가 세는 글자가 다르다.
const (
	octetPattern = `(?:25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])`
	ipv4Pattern  = octetPattern + `(?:\.` + octetPattern + `){3}`
	hexPattern   = `[0-9A-Fa-f]{1,4}`
	sepPattern   = `[, \t]`
)

// ipv6Pattern — 콜론으로 적은 IPv6. 끝의 32비트를 IPv4로 적은 꼴(`::ffff:10.0.0.1` · `64:ff9b::192.0.2.1`)도 받는다:
// net.ParseIP가 받는 주소이고, 기존 선언에 있으면 다른 칸만 고쳐도 저장할 수 있어야 한다. 영역 표기(`%eth0`)는 받지 않는다.
var ipv6Pattern = strings.NewReplacer("H", hexPattern, "V", ipv4Pattern).Replace(
	`(?:(?:H:){7}H|(?:H:){1,7}:|(?:H:){1,6}:H|(?:H:){1,5}(?::H){1,2}|` +
		`(?:H:){1,4}(?::H){1,3}|(?:H:){1,3}(?::H){1,4}|(?:H:){1,2}(?::H){1,5}|H:(?::H){1,6}|:(?:(?::H){1,7}|:)|` +
		`(?:H:){6}V|::(?:H:){0,5}V|H::(?:H:){0,4}V|(?:H:){1}H::(?:H:){0,3}V|(?:H:){2}H::(?:H:){0,2}V|` +
		`(?:H:){3}H::(?:H:){0,1}V|(?:H:){4}H::V)`)

var (
	// IPListPattern — IP 칸: 주소만, 여럿이면 쉼표나 공백으로 나눈다. 빈 칸은 이 식이 아니라 required(브라우저)와
	// FieldIPMissing(서버)이 막는다: 식에 넣으면 아무것도 적지 않은 새 줄까지 막힌다.
	//
	// **호스트명과 포트는 받지 않는다.** 대조는 관측에 찍힌 주소와 글자 그대로 같을 때만 노드로 잇는다(report.ResolveEdgeDsts).
	IPListPattern = sepPattern + `*(?:(?:` + ipv4Pattern + `|` + ipv6Pattern + `)(?:` + sepPattern + `+(?:` + ipv4Pattern + `|` + ipv6Pattern + `))*` + sepPattern + `*)?`
	// PortPattern — 포트 칸: 0부터 65535까지의 숫자. 비워 둘 수 있다(빈 칸은 0이고, 0은 decl.Check가 짚는다).
	PortPattern = `[ \t]*(?:0*(?:6553[0-5]|655[0-2][0-9]|65[0-4][0-9]{2}|6[0-4][0-9]{3}|[1-5][0-9]{4}|[0-9]{1,4}))?[ \t]*`

	ipListRe = regexp.MustCompile(`^(?:` + IPListPattern + `)$`)
	portRe   = regexp.MustCompile(`^(?:` + PortPattern + `)$`)
)

// 저장을 막는 칸의 종류.
const (
	FieldNameMissing      = "name-missing"
	FieldIPMissing        = "ip-missing"
	FieldIPMalformed      = "ip-malformed"
	FieldComponentMissing = "component-missing"
	FieldEdgeSrcMissing   = "edge-src-missing"
	FieldEdgeDstMissing   = "edge-dst-missing"
	FieldPortMalformed    = "port-malformed"
)

// DeclFieldError — 저장을 막은 칸 하나.
type DeclFieldError struct {
	// Field — 폼 이름(`node.name.2`). 칸의 id와 오류 줄의 id가 여기서 나온다.
	Field string
	// Code — 무엇이 틀렸나(Field…).
	Code string
	// Value — 틀린 값. 값이 붙는 종류(IP · 포트)만 찬다.
	Value string

	kind      string // KindNode · KindAsset · KindEdge
	node, row int    // 화면의 줄 번호(0부터). 자산은 node가 그 노드다
	col       T
}

// Label — 「노드 2 이름」. 그 칸의 접근성 이름과 같다.
func (e DeclFieldError) Label(l Lang) string {
	switch e.kind {
	case KindAsset:
		return assetLabel(l, e.node, e.row, e.col.In(l))
	case KindEdge:
		return rowLabel(l, tRowEdge, e.row+1, e.col.In(l))
	case KindRule:
		return ruleLabel(l, e.node, e.row, e.col.In(l))
	}
	return rowLabel(l, tRowNode, e.row+1, e.col.In(l))
}

// Text — 무엇이 틀렸고 어떻게 고치나.
func (e DeclFieldError) Text(l Lang) string {
	switch e.Code {
	case FieldNameMissing:
		return tFieldNameMissing.In(l)
	case FieldIPMissing:
		return tFieldIPMissing.In(l)
	case FieldIPMalformed:
		return tFieldIPMalformed.F(l, strconv.Quote(e.Value))
	case FieldComponentMissing:
		return tFieldComponentMissing.In(l)
	case FieldEdgeSrcMissing:
		return tFieldEdgeSrcMissing.In(l)
	case FieldEdgeDstMissing:
		return tFieldEdgeDstMissing.In(l)
	case FieldPortMalformed:
		return tFieldPortMalformed.F(l, strconv.Quote(e.Value))
	case FieldRuleEmpty:
		return tFieldRuleEmpty.In(l)
	}
	return e.Code
}

// FieldID · FieldErrID — 칸과, 그 칸의 오류 줄. 오류 목록의 링크가 칸으로 가고, 칸의 aria-describedby가 오류 줄을 가리킨다.
func FieldID(name string) string    { return "f-" + name }
func FieldErrID(name string) string { return "e-" + name }

// fieldErrs — 폼 이름으로 찾는 오류.
type fieldErrs map[string]DeclFieldError

func (v DeclView) fieldErrs() fieldErrs {
	if len(v.FieldErrors) == 0 {
		return nil
	}
	out := fieldErrs{}
	for _, e := range v.FieldErrors {
		out[e.Field] = e
	}
	return out
}

// fieldAttrs — 칸의 id와, 저장을 막은 칸이면 그 표시.
func fieldAttrs(fe fieldErrs, name string) templ.Attributes {
	a := templ.Attributes{"id": FieldID(name)}
	if _, bad := fe[name]; bad {
		a["aria-invalid"] = "true"
		a["aria-describedby"] = FieldErrID(name)
	}
	return a
}

// guardState — 편집 양식의 `data-guard` 값. 저장을 거절한 화면은 그릴 때부터 저장하지 않은 입력을 들고 있으므로
// "dirty"다. 그 밖에는 빈 값이고, 달라졌는지는 스크립트가 처음 모습과 견줘 안다(app.js).
func guardState(refused bool) string {
	if refused {
		return "dirty"
	}
	return ""
}

// DeclEdge — 엣지 한 줄. **포트는 적은 글자 그대로 든다**: 숫자가 아닌 값을 되돌려 보여야 고칠 수 있다.
type DeclEdge struct {
	Src, Dst, Port, Proto string
}

// declInput — 양식에서 읽은 것. 줄 번호는 화면에 다시 그릴 차례(0부터)로 새로 매긴다.
type declInput struct {
	org   string
	nodes []DeclNode
	edges []DeclEdge
	errs  []DeclFieldError
}

// readDeclForm — 양식을 읽고 저장을 막을 칸을 모은다.
//
// **완전히 빈 새 줄만 버린다.** 「행 추가」로 열어 두고 쓰지 않은 줄이다. 그 밖의 줄은 틀렸더라도 그대로 든다: 저장을
// 거절한 화면이 적은 값을 그대로 되돌려 보여야 한다.
//
// 새 자산 줄은 런타임이 「행 추가」가 준 기본값 그대로이고 컴포넌트가 비었을 때만 쓰지 않은 줄이다. 런타임을 골랐으면
// 쓰다 만 줄이므로 컴포넌트가 빠졌다고 짚는다.
//
// **IP가 없는 노드도 막는다.** 예전에는 저장에서 빼고 알렸는데, 그러면 마지막 IP를 실수로 지운 기존 노드와 그 자산이
// 함께 빠졌다. IP가 없으면 관측에 찍힌 주소를 그 이름과 이을 근거가 없으므로, 채우거나 「제거」로 지우게 한다.
//
// 기존 줄인지는 숨은 칸(`node.was.N` · `asset.N.M.was`)이 말한다. 화면이 파일에서 그린 줄에만 단다.
func readDeclForm(f url.Values) declInput {
	form := declInput{org: strings.TrimSpace(f.Get("org"))}
	bad := func(field, code, value, kind string, node, row int, col T) {
		form.errs = append(form.errs, DeclFieldError{Field: field, Code: code, Value: value, kind: kind, node: node, row: row, col: col})
	}
	for _, ni := range formRows(f, "node.name.", "") {
		at := strconv.Itoa(ni)
		n := DeclNode{
			Name: strings.TrimSpace(f.Get("node.name." + at)), Was: strings.TrimSpace(f.Get("node.was." + at)),
			Typed: true, IPsTyped: f.Get("node.ips." + at), SeenTyped: f.Get("node.seen." + at),
		}
		n.IPs, n.ObservedAs = splitList(n.IPsTyped), splitList(n.SeenTyped)
		for _, ai := range formRows(f, "asset."+at+".", ".component") {
			a := DeclAsset{
				Runtime:   strings.TrimSpace(f.Get(assetField(ni, ai, "runtime"))),
				Component: strings.TrimSpace(f.Get(assetField(ni, ai, "component"))),
				Was:       strings.TrimSpace(f.Get(assetField(ni, ai, "was"))),
			}
			if a.Component == "" && a.Was == "" && (a.Runtime == "" || a.Runtime == defaultRuntime) {
				continue // 쓰지 않은 새 자산 줄
			}
			n.Assets = append(n.Assets, a)
		}
		if n.Name == "" && n.Was == "" && len(n.IPs) == 0 && len(n.ObservedAs) == 0 && len(n.Assets) == 0 {
			continue // 쓰지 않은 새 노드 줄
		}
		i := len(form.nodes)
		to := strconv.Itoa(i)
		if n.Name == "" {
			bad("node.name."+to, FieldNameMissing, "", KindNode, i, i, tColName)
		}
		if v, ok := badIP(n.IPsTyped); !ok {
			bad("node.ips."+to, FieldIPMalformed, v, KindNode, i, i, tColIP)
		} else if len(n.IPs) == 0 {
			bad("node.ips."+to, FieldIPMissing, "", KindNode, i, i, tColIP)
		}
		for j, a := range n.Assets {
			if a.Component == "" {
				bad(assetField(i, j, "component"), FieldComponentMissing, "", KindAsset, i, j, tColComponent)
			}
		}
		form.nodes = append(form.nodes, n)
	}
	for _, ei := range formRows(f, "edge.src.", "") {
		at := strconv.Itoa(ei)
		e := DeclEdge{
			Src: strings.TrimSpace(f.Get("edge.src." + at)), Dst: strings.TrimSpace(f.Get("edge.dst." + at)),
			Port: f.Get("edge.port." + at), Proto: strings.TrimSpace(f.Get("edge.proto." + at)),
		}
		if e.Src == "" && e.Dst == "" && strings.Trim(e.Port, " \t") == "" && e.Proto == "" {
			continue // 쓰지 않은 엣지 줄
		}
		i := len(form.edges)
		to := strconv.Itoa(i)
		if e.Src == "" {
			bad("edge.src."+to, FieldEdgeSrcMissing, "", KindEdge, 0, i, tColSrc)
		}
		if e.Dst == "" {
			bad("edge.dst."+to, FieldEdgeDstMissing, "", KindEdge, 0, i, tColDst)
		}
		if !portRe.MatchString(e.Port) {
			bad("edge.port."+to, FieldPortMalformed, e.Port, KindEdge, 0, i, tColPort)
		}
		form.edges = append(form.edges, e)
	}
	return form
}

// defaultRuntime — 「행 추가」가 준 자산 줄에 골라져 있는 런타임(목록의 첫 값).
var defaultRuntime = reconcile.Runtimes()[0]

// badIP — IP 칸에 적은 것이 주소 목록인가. 아니면 틀린 값(짚을 수 있으면 그 토막, 아니면 적은 것 전체)을 돌려준다.
//
// 식(IPListPattern)과 net.ParseIP를 둘 다 지나야 한다. 식은 브라우저가 쓰는 것과 같고, ParseIP는 decl.Check가 쓰는 것과 같다.
func badIP(typed string) (string, bool) {
	for _, ip := range splitList(typed) {
		if net.ParseIP(ip) == nil || !ipListRe.MatchString(ip) {
			return ip, false
		}
	}
	if !ipListRe.MatchString(typed) {
		return typed, false
	}
	return "", true
}

// portOf — 포트 칸의 값. 식(PortPattern)을 지난 것만 온다. 빈 칸은 0이다.
func portOf(typed string) uint32 {
	p, _ := strconv.ParseUint(strings.Trim(typed, " \t"), 10, 32)
	return uint32(p)
}

// ApplyDecl — 폼에서 온 값으로 선언을 다시 만든다.
//
// **얹는 것이 아니라 다시 만든다.** 「제거」로 지운 줄은 양식에 실려 오지 않으므로, 기존 것에 얹으면 지운 줄이 되살아난다.
// `_comment`는 생성 도구가 남긴 머리말이라 그대로 들고 간다. 사람이 편집할 자리가 아니다.
//
// errs — 저장을 막는 칸. **하나라도 있으면 선언을 만들지 않는다**(d는 빈 값이다). 부르는 쪽은 아무것도 쓰지 않고, 적은 값을
// 그대로 되돌려 보인다(NewDeclFormView).
//
// 양식에 실려 온 노드는 모두 관리 대상이 된다. IP는 **적은 표기 그대로** 든다(대조가 글자 그대로 견준다).
func ApplyDecl(prev decl.Declaration, f url.Values) (d decl.Declaration, errs []DeclFieldError) {
	form := readDeclForm(f)
	if len(form.errs) > 0 {
		return decl.Declaration{}, form.errs
	}
	d = decl.Declaration{Comment: prev.Comment, Org: form.org}
	for _, n := range form.nodes {
		d.Scope = append(d.Scope, n.Name)
		d.Nodes = append(d.Nodes, decl.Node{Name: n.Name, IPs: n.IPs, ObservedAs: n.ObservedAs})
		for _, a := range n.Assets {
			d.Assets = append(d.Assets, decl.Asset{Node: n.Name, Runtime: a.Runtime, Component: a.Component})
		}
	}
	for _, e := range form.edges {
		d.Edges = append(d.Edges, decl.Edge{Src: e.Src, Dst: e.Dst, Port: portOf(e.Port), Proto: e.Proto})
	}
	return d, nil
}

// NewDeclFormView — 저장을 거절한 양식을 **적은 값 그대로** 화면이 보는 모양으로 옮긴다. 파일에서 읽지 않는다.
//
// 틀린 줄도 그대로 든다. 줄 번호는 다시 매기고, 오류는 그 번호의 칸을 가리킨다.
func NewDeclFormView(f url.Values, page Page) DeclView {
	form := readDeclForm(f)
	return DeclView{Page: page, Org: form.org, Nodes: form.nodes, Edges: form.edges,
		FieldErrors: form.errs, Refused: len(form.errs) > 0}
}

func declEdges(in []decl.Edge) []DeclEdge {
	var out []DeclEdge
	for _, e := range in {
		out = append(out, DeclEdge{Src: e.Src, Dst: e.Dst, Port: portText(e.Port), Proto: e.Proto})
	}
	return out
}
