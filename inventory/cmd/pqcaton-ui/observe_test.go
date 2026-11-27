// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-U113 — **접근 정보를 화면에서 저장해도 비밀번호는 화면에 오가지 않고 그대로 남는다.** 01 관측 화면은
// 비밀번호를 「있음」으로만 보이고 값은 싣지 않는다. 그 줄의 노드 ID를 고치고, 다른 줄을 고치고, 새 줄을
// 더해 저장하면 비밀번호는 고친 노드 ID를 따라가고 새 줄에는 없다. 저장한 파일은 상류 ParseHosts가 그대로
// 읽고 소유자 전용(0600)이다.
func TestHostsAreSavedKeepingPasswords(t *testing.T) {
	s, _ := fullServer(t)
	body := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	if strings.Contains(body, "s3cret-pw") {
		t.Fatal("the observe screen carries the password")
	}
	if !strings.Contains(body, "있음") {
		t.Error("the observe screen does not say a password is set")
	}
	form := url.Values{
		"host.was.0": {"web"}, "host.id.0": {"web"}, "host.ip.0": {"10.0.0.20"}, "host.user.0": {"ops"},
		"host.key.0": {"/home/me/.ssh/id_ed25519"}, "host.os.0": {"linux"}, "host.conn.0": {"ssh"},
		"host.was.1": {"db"}, "host.id.1": {"db"}, "host.ip.1": {"10.0.0.9"}, "host.port.1": {"2222"},
		"host.os.1": {"linux"}, "host.conn.1": {"ssh"},
		// 선언에만 있던 노드(app)에 접속 정보를 처음 적는다. 선언의 다른 노드(web-gw)는 빈 줄로 온다: 저장되지 않는다.
		"host.was.3": {""}, "host.id.3": {"app"}, "host.ip.3": {"10.0.0.11"}, "host.os.3": {"windows"}, "host.conn.3": {"winrm"},
		"host.was.4": {""}, "host.id.4": {"web-gw"}, "host.os.4": {"linux"}, "host.conn.4": {"ssh"},
	}
	w := postForm(t, s, ui.HostsSavePath, withRev(form, s.revFor(ui.ScreenObserve)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status %d, want 303\n%s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); !strings.HasPrefix(loc, ui.ScreenObserve+"?") || !strings.Contains(loc, ui.NoticeHostsSaved) {
		t.Errorf("redirected to %q", loc)
	}
	b, err := os.ReadFile(s.hosts)
	if err != nil {
		t.Fatal(err)
	}
	hosts, err := inventory.ParseHosts(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("upstream cannot read the saved file: %v\n%s", err, b)
	}
	got := map[string]inventory.Host{}
	for _, h := range hosts {
		got[h.NodeID] = h
	}
	if len(hosts) != 3 || got["web"].IP != "10.0.0.20" || got["web"].SSHUser != "ops" {
		t.Errorf("saved hosts = %+v", hosts)
	}
	if got["db"].SSHPass != "s3cret-pw" || got["db"].Port != 2222 {
		t.Errorf("the password was not kept with its node: %+v", got["db"])
	}
	if got["app"].SSHPass != "" || got["app"].Conn != inventory.ConnWinRM || got["app"].Port != 5985 {
		t.Errorf("the new WinRM node = %+v", got["app"])
	}
	if _, blank := got["web-gw"]; blank {
		t.Error("the blank row of a declared node was written to hosts.csv")
	}
	if fi, err := os.Stat(s.hosts); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("hosts.csv mode = %v (%v), want 0600", fi.Mode().Perm(), err)
	}
}

// IC-U114 — **막는 자리가 있으면 쓰지 않고, 적은 값 그대로 다시 그린다.** WinRM을 Linux 노드에 고르거나 같은
// 노드 ID를 두 번 적으면 422로 답하고 파일은 한 바이트도 바뀌지 않는다. 화면에는 적은 값이 남아 있고, 어느
// 노드의 무엇이 틀렸는지 두 말로 적힌다.
func TestBadHostsAreShownBackUnsaved(t *testing.T) {
	for _, c := range []struct{ lang, winrm, dup string }{
		{"ko", "WinRM은 Windows 노드에만 씁니다", "같은 노드 ID가 두 번 적혔습니다"},
		{"en", "WinRM is only for Windows nodes", "the same node ID is written twice"},
	} {
		s, _ := fullServer(t)
		before := readFile(t, s.hosts)
		form := url.Values{
			"host.was.0": {"web"}, "host.id.0": {"web"}, "host.ip.0": {"10.0.0.2"}, "host.os.0": {"linux"}, "host.conn.0": {"winrm"},
			"host.was.1": {"db"}, "host.id.1": {"db"}, "host.ip.1": {"10.0.0.9"}, "host.os.1": {"linux"}, "host.conn.1": {"ssh"},
			"host.id.2": {"db"}, "host.row.2": {"declared"}, "host.ip.2": {"10.0.0.10"}, "host.os.2": {"linux"}, "host.conn.2": {"ssh"},
		}
		w := postFormLang(t, s, ui.HostsSavePath, c.lang, withRev(form, s.revFor(ui.ScreenObserve)))
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status %d, want 422", c.lang, w.Code)
		}
		if readFile(t, s.hosts) != before {
			t.Errorf("%s: a refused save was written", c.lang)
		}
		body := w.Body.String()
		for _, want := range []string{c.winrm, c.dup, `value="10.0.0.10"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the page does not show %q", c.lang, want)
			}
		}
		if strings.Contains(body, "s3cret-pw") {
			t.Errorf("%s: the re-drawn page carries the password", c.lang)
		}
	}
}

// IC-U115 — **01 관측은 선언이 있으면 열리고, 접속 준비와 원격 관측은 작업 폴더 모드에서만 쓴다.** 작업 폴더 없이 선언만 연
// 서버에서는 01이 「대상 노드와 기대 상태」와 그리로 가는 길을 보이고, 접속 준비와 관측 자리에는 작업 폴더로 시작하라는 안내 한
// 줄이 있으며, 접근 정보 양식도 시작 양식도 없다. 그 선언 화면에서 01 카드가 지금 단계이고 돌아가는 링크가 있다. 선언도 작업
// 폴더도 없으면 `/observe`는 404이고 01 카드는 닫혀 있다. 작업 폴더 모드에서는 선언이 아직 없어도 열리고 카드가 「아직 선언이
// 없습니다」를 적는다.
func TestObserveOpensOnlyInAWorkspace(t *testing.T) {
	s, _ := withDecl(t)
	w0 := get(t, s, ui.ScreenObserve+"?lang=ko")
	if w0.Code != http.StatusOK {
		t.Fatalf("a declaration without a workspace: /observe = %d, want 200", w0.Code)
	}
	page := w0.Body.String()
	for _, want := range []string{`data-observe="no-remote"`, "접속 정보를 입력하고 관측하려면 작업 폴더로 시작하십시오", "대상 노드와 기대 상태를 정하고 관측을 실행합니다.", `href="/decl?lang=ko" data-decl-edit`, "기대 자산 1개 · 기대 통신 1개", "data-decl-counts"} {
		if !strings.Contains(page, want) {
			t.Errorf("a declaration without a workspace: 01 lacks %q", want)
		}
	}
	if strings.Contains(page, `action="`+ui.HostsSavePath+`"`) || strings.Contains(page, `action="`+ui.RunPath+`"`) {
		t.Error("a declaration without a workspace: 01 shows the access form or offers to start")
	}
	if got := cards(t, page)["01"]; got != "작업 폴더 없이 열림" {
		t.Errorf("a declaration without a workspace: card 01 = %q", got)
	}
	decl := get(t, s, ui.ScreenDecl+"?lang=ko").Body.String()
	if !regexp.MustCompile(`href="/observe" aria-current="step"`).MatchString(decl) || !strings.Contains(decl, `href="/observe?lang=ko#observe-decl" data-decl-back`) {
		t.Error("on the target nodes screen the 01 card is not the current step, or there is no way back to 01")
	}
	bare, _ := newServer(t)
	if w := get(t, bare, ui.ScreenObserve); w.Code != http.StatusNotFound {
		t.Errorf("no declaration and no workspace: /observe = %d, want 404", w.Code)
	}
	if got := cards(t, get(t, bare, ui.ScreenReview+"?lang=ko").Body.String())["01"]; !strings.HasPrefix(got, "입력 파일을 주지 않았습니다") {
		t.Errorf("no declaration and no workspace: card 01 = %q, want closed", got)
	}

	h, code, _, _ := started(t, "-workspace", t.TempDir(), "-no-browser")
	if code != 0 {
		t.Fatalf("start: %d", code)
	}
	w := do(h, newReq(http.MethodGet, ui.ScreenObserve+"?lang=ko", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("workspace: /observe = %d", w.Code)
	}
	if got := cards(t, w.Body.String())["01"]; !strings.HasPrefix(got, "아직 선언이 없습니다") {
		t.Errorf("workspace: card 01 = %q", got)
	}
}
