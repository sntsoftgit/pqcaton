// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-U113 — **접근 정보를 화면에서 저장해도 비밀번호는 화면에 오가지 않고 그대로 남는다.** 03 관측 화면은
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
		"host.was.1": {"db"}, "host.id.1": {"db-main"}, "host.ip.1": {"10.0.0.9"}, "host.port.1": {"2222"},
		"host.os.1": {"linux"}, "host.conn.1": {"ssh"},
		"host.was.3": {""}, "host.id.3": {"win"}, "host.ip.3": {"10.0.0.11"}, "host.os.3": {"windows"}, "host.conn.3": {"winrm"},
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
	if got["db-main"].SSHPass != "s3cret-pw" || got["db-main"].Port != 2222 {
		t.Errorf("the password did not follow the renamed node: %+v", got["db-main"])
	}
	if got["win"].SSHPass != "" || got["win"].Conn != inventory.ConnWinRM || got["win"].Port != 5985 {
		t.Errorf("the new WinRM node = %+v", got["win"])
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
			"host.was.1": {"db"}, "host.id.1": {"typed-node"}, "host.ip.1": {"10.0.0.9"}, "host.os.1": {"linux"}, "host.conn.1": {"ssh"},
			"host.id.2": {"typed-node"}, "host.ip.2": {"10.0.0.10"}, "host.os.2": {"linux"}, "host.conn.2": {"ssh"},
		}
		w := postFormLang(t, s, ui.HostsSavePath, c.lang, withRev(form, s.revFor(ui.ScreenObserve)))
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status %d, want 422", c.lang, w.Code)
		}
		if readFile(t, s.hosts) != before {
			t.Errorf("%s: a refused save was written", c.lang)
		}
		body := w.Body.String()
		for _, want := range []string{c.winrm, c.dup, `value="typed-node"`, `value="10.0.0.10"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the page does not show %q", c.lang, want)
			}
		}
		if strings.Contains(body, "s3cret-pw") {
			t.Errorf("%s: the re-drawn page carries the password", c.lang)
		}
	}
}

// IC-U115 — **관측 화면은 작업 폴더 모드에서만 열린다.** 명시 모드에는 접근 정보의 자리가 없어 `/observe`가 404이고,
// 03 카드는 닫혀 있다. 작업 폴더 모드에서는 hosts.csv가 아직 없어도 열리고 카드가 「아직 접근 정보가 없습니다」를 적는다.
func TestObserveOpensOnlyInAWorkspace(t *testing.T) {
	s, _ := withDecl(t)
	if w := get(t, s, ui.ScreenObserve); w.Code != http.StatusNotFound {
		t.Errorf("explicit mode: /observe = %d, want 404", w.Code)
	}
	if got := cards(t, get(t, s, ui.ScreenDecl+"?lang=ko").Body.String())["03"]; !strings.HasPrefix(got, "입력 파일을 주지 않았습니다") {
		t.Errorf("explicit mode: card 03 = %q, want closed", got)
	}

	h, code, _, _ := started(t, "-workspace", t.TempDir(), "-no-browser")
	if code != 0 {
		t.Fatalf("start: %d", code)
	}
	w := do(h, newReq(http.MethodGet, ui.ScreenObserve+"?lang=ko", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("workspace: /observe = %d", w.Code)
	}
	if got := cards(t, w.Body.String())["03"]; !strings.HasPrefix(got, "아직 접근 정보가 없습니다") {
		t.Errorf("workspace: card 03 = %q", got)
	}
}
