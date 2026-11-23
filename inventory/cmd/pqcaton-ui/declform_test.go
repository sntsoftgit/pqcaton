// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	htmlpkg "html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

var (
	textInput      = regexp.MustCompile(`<input\s+type="text"\s+name="([^"]*)"\s+value="([^"]*)"`)
	selectedOption = regexp.MustCompile(`(?s)<select name="([^"]*)"[^>]*>(?:(?:<option[^>]*>[^<]*</option>)*?)<option value="([^"]*)" selected`)
)

func htmlUnescape(s string) string { return htmlpkg.UnescapeString(s) }

// screenForm: 선언 화면이 그린 양식을 브라우저가 보낼 그대로 읽는다(숨은 칸과 글 칸. 런타임은 고른 값).
func screenForm(t *testing.T, body string) url.Values {
	t.Helper()
	out := hiddenFields(body)
	for _, m := range textInput.FindAllStringSubmatch(body, -1) {
		out.Set(htmlUnescape(m[1]), htmlUnescape(m[2]))
	}
	for _, m := range selectedOption.FindAllStringSubmatch(body, -1) {
		out.Set(htmlUnescape(m[1]), htmlUnescape(m[2]))
	}
	// 토큰은 보내는 쪽 도우미가 쿠키와 같은 값으로 싣는다. rev는 화면이 실은 그대로 보낸다.
	out.Del(ui.CSRFField)
	if out.Get("node.name.0") == "" && !strings.Contains(body, `name="node.name.0"`) {
		t.Fatalf("no declaration form in:\n%s", body)
	}
	return out
}

// IC-U219 — **기존 노드의 이름을 비워 저장하면 거절한다(422).** 선언 파일은 바이트까지 그대로이고, 화면은 「저장하지
// 않았습니다」와 그 칸으로 가는 링크를 보이며, 적은 값(다른 줄 · 그 노드의 자산 포함)을 그대로 돌려준다. 그 화면에서
// 이름을 되돌려 다시 보내면 저장되고, 노드의 자산과 함께 적던 새 줄이 남는다.
func TestDeclSaveRefusesAnEmptiedNameAndKeepsTheFile(t *testing.T) {
	s, _ := withDecl(t)
	before := readFile(t, s.decl)
	form := screenForm(t, get(t, s, "/decl?lang=ko").Body.String())
	if form.Get("node.was.0") != "web-gw" || form.Get("asset.0.0.was") != "libssl" {
		t.Fatalf("the screen does not mark its existing rows: %v", form)
	}
	form.Set("node.name.0", "") // 실수로 이름을 지운다
	// 함께 적던 새 줄.
	form.Set("node.name.7", "cache")
	form.Set("node.ips.7", "10.0.0.7")
	form.Set("asset.7.0.runtime", "openssl")
	form.Set("asset.7.0.component", "libcrypto")

	w := postFormLang(t, s, "/decl/save", "ko", form)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422\n%s", w.Code, w.Body.String())
	}
	if readFile(t, s.decl) != before {
		t.Fatal("the declaration file changed although the save was refused")
	}
	body := w.Body.String()
	for _, want := range []string{
		"저장하지 않았습니다", `href="#f-node.name.0"`, `aria-invalid="true"`,
		`name="asset.0.0.component" value="libssl"`,                              // 그 노드의 자산
		`name="node.was.1" value="pay-db"`,                                       // 파일에 IP 없이 있던 줄(withDecl)
		`name="node.name.2" value="cache"`, `name="node.ips.2" value="10.0.0.7"`, // 적던 새 줄
		`name="asset.2.0.component" value="libcrypto"`,
		`name="edge.port.0" value="5432"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the refused screen has no %s", want)
		}
	}
	if got := cards(t, body)["01"]; got == "" {
		t.Error("the refused screen has no step cards")
	}

	// 이름을 되돌려 그 화면의 양식 그대로 다시 보낸다.
	again := screenForm(t, body)
	if again.Get(ui.RevField) == "" || again.Get(ui.RevField) != form.Get(ui.RevField) {
		t.Fatalf("the refused screen does not carry the rev it was sent: %q", again.Get(ui.RevField))
	}
	again.Set("node.name.0", "web-gw")
	// withDecl의 pay-db는 파일에 IP 없이 있던 줄이다. IP를 채워야 저장된다(IC-U221).
	if w := postFormLang(t, s, "/decl/save", "ko", again); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `href="#f-node.ips.1"`) {
		t.Fatalf("a node without an IP was not refused at its IP field: %d", w.Code)
	}
	again.Set("node.ips.1", "10.0.0.2")
	location(t, postFormLang(t, s, "/decl/save", "ko", again))
	d, err := decl.Load(s.decl)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Nodes) != 3 || d.Nodes[0].Name != "web-gw" || d.Nodes[1].Name != "pay-db" || d.Nodes[2].Name != "cache" {
		t.Fatalf("nodes after the second save: %+v", d.Nodes)
	}
	if len(d.Assets) != 2 || d.Assets[0] != (decl.Asset{Node: "web-gw", Runtime: "openssl", Component: "libssl"}) ||
		d.Assets[1] != (decl.Asset{Node: "cache", Runtime: "openssl", Component: "libcrypto"}) {
		t.Fatalf("assets after the second save: %+v", d.Assets)
	}
	if len(d.Edges) != 1 || d.Edges[0].Port != 5432 {
		t.Fatalf("edges after the second save: %+v", d.Edges)
	}
}

// IC-U220 — **브라우저의 검사를 거치지 않은 요청도 서버가 거절한다.** pattern · required를 지나지 않고 온 틀린 IP와
// 포트, 「기존 줄」 표시만 남기고 이름을 뺀 요청이 모두 422이고 파일은 그대로다. 포트 `xyz`가 0으로 저장되지 않는다.
// 영어 화면에는 한글이 없다.
func TestDeclSaveRefusesWhatTheBrowserWouldHaveStopped(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(url.Values)
		want string
	}{
		{"a port that is not a number", func(f url.Values) { f.Set("edge.port.0", "xyz") }, `href="#f-edge.port.0"`},
		{"a hostname in the IP field", func(f url.Values) { f.Set("node.ips.0", "web-gw.corp.example") }, `href="#f-node.ips.0"`},
		{"an existing node with its name field left out", func(f url.Values) { f.Set("node.name.0", " ") }, `href="#f-node.name.0"`},
		{"an existing asset emptied", func(f url.Values) { f.Set("asset.0.0.component", "") }, `href="#f-asset.0.0.component"`},
		{"a new asset with a runtime picked and no component", func(f url.Values) {
			f.Set("asset.0.5.runtime", "jca")
			f.Set("asset.0.5.component", "")
		}, `href="#f-asset.0.1.component"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _ := withDecl(t)
			before := readFile(t, s.decl)
			form := screenForm(t, get(t, s, "/decl?lang=en").Body.String())
			form.Set("node.ips.1", "10.0.0.2") // 파일에 IP 없이 있던 pay-db. 이 케이스가 재는 칸만 틀리게 둔다
			c.edit(form)
			w := postFormLang(t, s, "/decl/save", "en", form)
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422\n%s", w.Code, w.Body.String())
			}
			if readFile(t, s.decl) != before {
				t.Fatal("the declaration file changed although the save was refused")
			}
			body := w.Body.String()
			if !strings.Contains(body, "Not saved") || !strings.Contains(body, c.want) {
				t.Errorf("the refused screen does not say it was not saved or does not link %s", c.want)
			}
			if hasHangul(visible(body)) {
				t.Error("Korean on the English refused screen")
			}
		})
	}
}

// IC-U221 — **기존 노드의 마지막 IP를 지워 저장하면 거절한다(422).** 예전에는 그 노드와 자산을 저장에서 빼고 알림만
// 냈다. 선언 파일은 바이트까지 그대로이고, 화면은 IP 칸으로 가는 링크와 그 노드의 자산을 그대로 보인다. IP를 되돌려
// 다시 보내면 저장되고 자산이 남는다. IPv4를 끼운 IPv6로 적은 주소는 적은 표기 그대로 저장된다.
func TestDeclSaveRefusesANodeWhoseLastIPWasRemoved(t *testing.T) {
	s, _ := withDecl(t)
	form := screenForm(t, get(t, s, "/decl?lang=ko").Body.String())
	form.Set("node.ips.1", "::ffff:10.0.0.2") // 파일에 IP 없이 있던 pay-db를 먼저 채워 저장한다
	location(t, postFormLang(t, s, "/decl/save", "ko", form))
	before := readFile(t, s.decl)
	if !strings.Contains(before, `"::ffff:10.0.0.2"`) {
		t.Fatalf("the IPv4-mapped address was not saved as typed:\n%s", before)
	}

	form = screenForm(t, get(t, s, "/decl?lang=ko").Body.String())
	if form.Get("node.ips.1") != "::ffff:10.0.0.2" {
		t.Fatalf("the screen shows %q for the address", form.Get("node.ips.1"))
	}
	form.Set("node.ips.0", "") // web-gw의 하나뿐인 IP를 지운다
	w := postFormLang(t, s, "/decl/save", "ko", form)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422\n%s", w.Code, w.Body.String())
	}
	if readFile(t, s.decl) != before {
		t.Fatal("the declaration file changed although the save was refused")
	}
	body := w.Body.String()
	for _, want := range []string{
		"저장하지 않았습니다", `href="#f-node.ips.0"`, "IP가 비어 있습니다",
		`name="node.name.0" value="web-gw"`, `name="asset.0.0.component" value="libssl"`,
		`name="node.ips.1" value="::ffff:10.0.0.2"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the refused screen has no %s", want)
		}
	}
	if strings.Contains(body, `href="#f-node.name.0"`) {
		t.Error("the refusal points at the name although only the IP is missing")
	}

	again := screenForm(t, body)
	again.Set("node.ips.0", "10.0.0.1")
	location(t, postFormLang(t, s, "/decl/save", "ko", again))
	if got := readFile(t, s.decl); got != before {
		t.Errorf("putting the IP back did not give the same declaration:\n%s\nwant:\n%s", got, before)
	}
}
