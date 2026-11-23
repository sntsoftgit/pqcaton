// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"net"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// codes: 막은 칸을 「폼 이름=종류」로.
func codes(bad []ui.DeclFieldError) string {
	var out []string
	for _, e := range bad {
		out = append(out, e.Field+"="+e.Code)
	}
	return strings.Join(out, " ")
}

// IC-UI61 — **이름을 비운 줄은 지운 줄이 아니다.** 기존 노드 · 자산의 이름을 비웠거나, 새 줄에 다른 값을 적고 이름만
// 비웠으면 그 칸을 짚고 선언을 만들지 않는다. 아무것도 적지 않은 새 줄만 버린다. 새 자산 줄은 런타임이 기본값 그대로일
// 때만 쓰지 않은 줄이고, 런타임을 골랐으면 컴포넌트가 빠졌다고 짚는다. 엣지는 한쪽 끝만 적은 줄을 짚는다.
func TestApplyDeclRefusesAnEmptiedName(t *testing.T) {
	for _, c := range []struct {
		name string
		form url.Values
		want string
	}{
		{"an existing node, name emptied", url.Values{
			"node.was.0": {"web"}, "node.name.0": {""}, "node.ips.0": {"10.0.0.1"},
			"asset.0.0.was": {"libssl"}, "asset.0.0.runtime": {"openssl"}, "asset.0.0.component": {"libssl"},
		}, "node.name.0=name-missing"},
		{"an existing node, every field emptied", url.Values{
			"node.was.0": {"web"}, "node.name.0": {""}, "node.ips.0": {""}, "node.seen.0": {""},
		}, "node.name.0=name-missing node.ips.0=ip-missing"},
		{"a new row with an IP and no name", url.Values{
			"node.name.4": {" "}, "node.ips.4": {"10.0.0.9"},
		}, "node.name.0=name-missing"},
		{"a new row with only an asset", url.Values{
			"node.name.0": {""}, "node.ips.0": {""}, "asset.0.0.runtime": {"openssl"}, "asset.0.0.component": {"libssl"},
		}, "node.name.0=name-missing node.ips.0=ip-missing"},
		{"a new asset row with a runtime picked and no component", url.Values{
			"node.was.0": {"web"}, "node.name.0": {"web"}, "node.ips.0": {"10.0.0.1"},
			"asset.0.4.runtime": {"jca"}, "asset.0.4.component": {" "},
		}, "asset.0.0.component=component-missing"},
		{"a new node row with only a runtime picked", url.Values{
			"node.name.2": {""}, "node.ips.2": {""}, "asset.2.0.runtime": {"cng"}, "asset.2.0.component": {""},
		}, "node.name.0=name-missing node.ips.0=ip-missing asset.0.0.component=component-missing"},
		{"an existing asset, component emptied", url.Values{
			"node.was.0": {"web"}, "node.name.0": {"web"}, "node.ips.0": {"10.0.0.1"},
			"asset.0.0.was": {"libssl"}, "asset.0.0.runtime": {"openssl"}, "asset.0.0.component": {""},
		}, "asset.0.0.component=component-missing"},
		{"an edge with one end", url.Values{
			"edge.src.0": {"web"}, "edge.dst.0": {""}, "edge.port.0": {"443"}, "edge.proto.0": {"TLS"},
			"edge.src.1": {""}, "edge.dst.1": {"db"}, "edge.port.1": {""}, "edge.proto.1": {""},
		}, "edge.dst.0=edge-dst-missing edge.src.1=edge-src-missing"},
	} {
		d, bad := ui.ApplyDecl(sample(), c.form)
		if got := codes(bad); got != c.want {
			t.Errorf("%s: refused %q, want %q", c.name, got, c.want)
		}
		if len(d.Nodes)+len(d.Assets)+len(d.Edges)+len(d.Scope) != 0 || d.Org != "" {
			t.Errorf("%s: a declaration was built although the form was refused: %+v", c.name, d)
		}
	}
	// 쓰지 않은 새 줄(노드 · 자산 · 엣지)은 막지 않고 버린다. 다른 줄은 그대로 저장된다.
	d, bad := ui.ApplyDecl(sample(), url.Values{
		"node.was.0": {"web"}, "node.name.0": {"web"}, "node.ips.0": {"10.0.0.1"},
		"asset.0.0.was": {"libssl"}, "asset.0.0.runtime": {"openssl"}, "asset.0.0.component": {"libssl"},
		"asset.0.1.runtime": {"openssl"}, "asset.0.1.component": {""},
		"node.name.1": {""}, "node.ips.1": {""}, "node.seen.1": {""},
		"edge.src.0": {""}, "edge.dst.0": {""}, "edge.port.0": {""}, "edge.proto.0": {""},
	})
	if len(bad) != 0 || len(d.Nodes) != 1 || len(d.Assets) != 1 || len(d.Edges) != 0 {
		t.Errorf("unused new rows: refused %q, built %+v", codes(bad), d)
	}
}

// IC-UI62 — **틀린 값을 다른 값으로 바꿔 저장하지 않는다.** IP 칸은 주소만 받는다(호스트명 · 포트 붙인 주소 · 글자는
// 거절). IPv4를 끼운 IPv6는 받고 적은 표기 그대로 저장한다. 포트는 빈 칸(0)과 해석할 수 없는 입력을 가른다: 숫자가 아니거나 65535를 넘으면 0으로 저장하지 않고 거절한다.
func TestApplyDeclRefusesMalformedValues(t *testing.T) {
	node := func(ips string) url.Values {
		return url.Values{"node.name.0": {"web"}, "node.ips.0": {ips}}
	}
	for _, ips := range []string{"abc", "10.0.0.1:443", "web.corp.example", "10.0.0.256", "010.0.0.1", "10.0.0.1;10.0.0.2", "10.0.0.1, db", "::ffff:10.0.0.256", "::ffff:010.0.0.1", "1:2:3:4:5:6:7:10.0.0.1", "fe80::1%eth0"} {
		if _, bad := ui.ApplyDecl(decl.Declaration{}, node(ips)); codes(bad) != "node.ips.0=ip-malformed" {
			t.Errorf("IP %q: refused %q, want ip-malformed", ips, codes(bad))
		}
	}
	for _, ips := range []string{"10.0.0.1", " 10.0.0.1 , 10.0.1.1 ", "10.0.0.1\t10.0.1.1", "fe80::1", "2001:db8::8a2e:370:7334, 10.0.0.1", "::1",
		"::ffff:10.0.0.1", "64:ff9b::192.0.2.1", "::10.0.0.1", "1:2:3:4:5:6:10.0.0.1", "1:2:3:4:5::10.0.0.1", "2001:db8::ffff:10.0.0.1, 10.0.0.1"} {
		d, bad := ui.ApplyDecl(decl.Declaration{}, node(ips))
		if len(bad) != 0 || len(d.Nodes) != 1 {
			t.Errorf("IP %q: refused %q", ips, codes(bad))
			continue
		}
		// 적은 표기 그대로 든다. 대조가 글자 그대로 견주므로 다른 표기로 바꾸면 이어지던 것이 끊긴다.
		if got := strings.Join(d.Nodes[0].IPs, " "); got != strings.Join(strings.FieldsFunc(ips, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }), " ") {
			t.Errorf("IP %q was saved as %q", ips, got)
		}
	}
	edge := func(port string) url.Values {
		return url.Values{"edge.src.0": {"web"}, "edge.dst.0": {"db"}, "edge.port.0": {port}, "edge.proto.0": {"TLS"}}
	}
	for _, port := range []string{"xyz", "8443x", "-1", "65536", "70000", "443.0", "4 43", "0x1bb"} {
		d, bad := ui.ApplyDecl(decl.Declaration{}, edge(port))
		if codes(bad) != "edge.port.0=port-malformed" || len(d.Edges) != 0 {
			t.Errorf("port %q: refused %q, built %+v", port, codes(bad), d.Edges)
		}
		if len(bad) == 1 && bad[0].Value != port {
			t.Errorf("port %q: the refusal names %q", port, bad[0].Value)
		}
	}
	for port, want := range map[string]uint32{"": 0, " ": 0, "0": 0, "443": 443, " 8443 ": 8443, "65535": 65535, "00443": 443} {
		d, bad := ui.ApplyDecl(decl.Declaration{}, edge(port))
		if len(bad) != 0 || len(d.Edges) != 1 || d.Edges[0].Port != want {
			t.Errorf("port %q: refused %q, built %+v, want port %d", port, codes(bad), d.Edges, want)
		}
	}
}

// IC-UI63 — **브라우저와 서버가 같은 기준을 쓴다.** 칸의 pattern은 서버가 쓰는 식 그대로이고, 기존 줄의 이름 칸에는
// required와 「기존 줄」 표시(숨은 칸)가 붙는다. 「행 추가」가 주는 새 줄에는 둘 다 없다. 식이 받는 주소는 net.ParseIP도
// 받는다(식이 서버보다 넓으면 브라우저가 통과시킨 값을 서버가 거절한다).
func TestDeclFormCarriesTheServerRules(t *testing.T) {
	var b strings.Builder
	if err := ui.RenderDecl(&b, ui.NewDeclView(sample(), ui.Page{Lang: ui.EN})); err != nil {
		t.Fatal(err)
	}
	body := b.String()
	attr := func(p string) string {
		return `pattern="` + strings.NewReplacer("&", "&amp;", `"`, "&#34;", "<", "&lt;", ">", "&gt;").Replace(p) + `"`
	}
	for _, want := range []string{
		attr(ui.IPListPattern), attr(ui.PortPattern),
		`name="node.was.0" value="web"`, `name="asset.0.0.was" value="libssl"`,
		`id="f-node.name.0"`, `id="f-edge.port.0"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the form has no %s", want)
		}
	}
	name := regexp.MustCompile(`<input[^>]*name="node\.name\.0"[^>]*>`).FindString(body)
	comp := regexp.MustCompile(`<input[^>]*name="asset\.0\.0\.component"[^>]*>`).FindString(body)
	if !strings.Contains(name, " required") || !strings.Contains(comp, " required") {
		t.Errorf("an existing row's name is not required:\n%s\n%s", name, comp)
	}
	if strings.Contains(body, "aria-invalid") || strings.Contains(body, `id="save-refused"`) {
		t.Error("a screen drawn from the file shows a refusal")
	}
	for _, kind := range []string{ui.KindNode, ui.KindAsset, ui.KindEdge} {
		var row strings.Builder
		if err := ui.RenderRow(&row, ui.EN, kind, 0, 7); err != nil {
			t.Fatal(err)
		}
		if got := row.String(); strings.Contains(got, ".was") || strings.Contains(got, " required") {
			t.Errorf("a new %s row is marked as an existing one:\n%s", kind, got)
		}
	}
	ipRe := regexp.MustCompile(`^(?:` + ui.IPListPattern + `)$`)
	for _, ip := range []string{"0.0.0.0", "255.255.255.255", "192.168.1.10", "::", "::1", "fe80::1", "2001:db8::1", "2001:db8:0:0:0:0:2:1", "1:2:3:4:5:6:7:8", "1::8", "1:2:3:4:5:6:7::",
		"::ffff:10.0.0.1", "::10.0.0.1", "64:ff9b::192.0.2.1", "1:2:3:4:5:6:10.0.0.1", "1::10.0.0.1", "1:2::3:10.0.0.1", "1:2:3::4:10.0.0.1", "1:2:3:4::5:10.0.0.1", "1:2:3:4:5::10.0.0.1"} {
		if !ipRe.MatchString(ip) || net.ParseIP(ip) == nil {
			t.Errorf("%q: pattern %v, net.ParseIP %v; both must accept it", ip, ipRe.MatchString(ip), net.ParseIP(ip) != nil)
		}
	}
	for _, ip := range []string{"1.2.3", "1.2.3.4.5", "256.1.1.1", "01.2.3.4", "1:2:3:4:5:6:7:8:9", ":::", "1::2::3", "12345::1", "g::1", "web", "10.0.0.1:443",
		"::ffff:10.0.0.256", "1:2:3:4:5:6:7:10.0.0.1", "1:2:3:4:5:6::10.0.0.1", "::ffff:10.0.0", "10.0.0.1::"} {
		if ipRe.MatchString(ip) {
			t.Errorf("the pattern accepts %q", ip)
		}
		if net.ParseIP(ip) != nil {
			t.Errorf("net.ParseIP accepts %q, which the pattern refuses: an existing declaration with it could not be saved", ip)
		}
	}
	// 기존 줄은 IP도 required다. 새 줄에는 없다(위의 새 줄 검사가 본다).
	if ipIn := regexp.MustCompile(`<input[^>]*name="node\.ips\.0"[^>]*>`).FindString(body); !strings.Contains(ipIn, " required") {
		t.Errorf("an existing node's IP is not required:\n%s", ipIn)
	}
}

// IC-UI64 — **거절한 화면은 적은 값을 그대로 돌려준다.** 파일에서 다시 그리지 않는다. 틀린 값(`xyz` · `abc`)과 다른 줄 ·
// 소속 자산이 그대로 있고, 맨 위에 「저장하지 않았습니다」가 서고, 줄마다 그 칸으로 가는 링크가 있으며, 막은 칸은
// aria-invalid와 aria-describedby로 그 줄을 가리킨다. 두 말 모두.
func TestRefusedDeclViewKeepsWhatWasTyped(t *testing.T) {
	form := url.Values{
		"org":        {"acme"},
		"node.was.0": {"web"}, "node.name.0": {""}, "node.ips.0": {"10.0.0.1,  10.0.1.1"}, "node.seen.0": {"node:1a2b"},
		"asset.0.0.was": {"libssl"}, "asset.0.0.runtime": {"openssl"}, "asset.0.0.component": {"libssl"},
		"asset.0.3.runtime": {"jca"}, "asset.0.3.component": {"sunjce-typed"},
		"node.name.5": {"db-typed"}, "node.ips.5": {"abc"},
		"edge.src.2": {"web"}, "edge.dst.2": {"db-typed"}, "edge.port.2": {"xyz"}, "edge.proto.2": {"TLS"},
	}
	for _, c := range []struct {
		lang               ui.Lang
		head, name, remove string
	}{
		{ui.KO, "저장하지 않았습니다", "노드 1 이름", "「제거」"},
		{ui.EN, "Not saved", "Node 1 Name", "use Remove"},
	} {
		v := ui.NewDeclFormView(form, ui.Page{Lang: c.lang})
		if !v.Refused || codes(v.FieldErrors) != "node.name.0=name-missing node.ips.1=ip-malformed edge.port.0=port-malformed" {
			t.Fatalf("%s: refused %v with %q", c.lang, v.Refused, codes(v.FieldErrors))
		}
		var b strings.Builder
		if err := ui.RenderDecl(&b, v); err != nil {
			t.Fatal(err)
		}
		body := b.String()
		for _, want := range []string{
			`id="save-refused"`, `role="alert"`, c.head, c.name, c.remove,
			`href="#f-node.name.0"`, `href="#f-node.ips.1"`, `href="#f-edge.port.0"`,
			`id="e-node.name.0"`, `aria-describedby="e-edge.port.0"`,
			`value="10.0.0.1,  10.0.1.1"`, `value="node:1a2b"`, // 적은 글자 그대로
			`name="node.was.0" value="web"`, // 기존 줄이라는 표시가 한 바퀴 돈다
			`name="asset.0.0.component" value="libssl"`, `name="asset.0.1.component" value="sunjce-typed"`,
			`name="node.name.1" value="db-typed"`, `name="node.ips.1" value="abc"`,
			`name="edge.port.0" value="xyz"`, `name="edge.dst.0" value="db-typed"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the refused screen has no %s", c.lang, want)
			}
		}
		if n := strings.Count(body, `aria-invalid="true"`); n != 3 {
			t.Errorf("%s: %d fields are marked invalid, want 3", c.lang, n)
		}
		if c.lang == ui.EN {
			for _, r := range body {
				if r >= 0xAC00 && r <= 0xD7A3 {
					t.Fatalf("en: Korean on the English refused screen")
				}
			}
		}
	}
	// 거절한 화면을 고쳐 다시 보내면 저장된다: 기존 줄 표시와 번호가 그대로 읽힌다.
	form.Set("node.name.0", "web")
	form.Set("node.ips.5", "10.0.0.2")
	form.Set("edge.port.2", "5432")
	d, bad := ui.ApplyDecl(sample(), form)
	if len(bad) != 0 || len(d.Nodes) != 2 || len(d.Assets) != 2 || len(d.Edges) != 1 || d.Edges[0].Port != 5432 {
		t.Errorf("after fixing: refused %q, built %+v", codes(bad), d)
	}
}
