// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"encoding/json"
	htmlpkg "html"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

var (
	anyInput   = regexp.MustCompile(`<input[^>]*>`)
	attrName   = regexp.MustCompile(` name="([^"]*)"`)
	attrValue  = regexp.MustCompile(` value="([^"]*)"`)
	anySelect  = regexp.MustCompile(`(?s)<select[^>]* name="([^"]*)"[^>]*>(.*?)</select>`)
	anyOption  = regexp.MustCompile(`<option value="([^"]*)"( selected)?`)
	hostsBlock = regexp.MustCompile(`(?s)<form method="post" action="` + regexp.QuoteMeta(ui.HostsSavePath) + `".*?</form>`)
)

// hostsForm: 02의 접근 정보 양식을 브라우저가 보낼 그대로 읽는다(숨은 칸 · 글 칸 · 숫자 칸 · 고른 값).
func hostsForm(t *testing.T, page string) url.Values {
	t.Helper()
	block := hostsBlock.FindString(page)
	if block == "" {
		t.Fatalf("no access details form in:\n%s", page)
	}
	out := url.Values{}
	for _, tag := range anyInput.FindAllString(block, -1) {
		n := attrName.FindStringSubmatch(tag)
		if n == nil {
			continue
		}
		v := ""
		if m := attrValue.FindStringSubmatch(tag); m != nil {
			v = htmlpkg.UnescapeString(m[1])
		}
		out.Set(htmlpkg.UnescapeString(n[1]), v)
	}
	for _, sel := range anySelect.FindAllStringSubmatch(block, -1) {
		opts := anyOption.FindAllStringSubmatch(sel[2], -1)
		v := ""
		for i, o := range opts {
			if i == 0 || o[2] != "" {
				v = htmlpkg.UnescapeString(o[1])
			}
			if o[2] != "" {
				break
			}
		}
		out.Set(htmlpkg.UnescapeString(sel[1]), v)
	}
	// 요청 보호 토큰과 rev는 시험의 보내는 쪽이 싣는다(postForm · withRev).
	out.Del(ui.CSRFField)
	out.Del(ui.RevField)
	return out
}

// hostField: 양식에서 그 노드 ID의 줄 번호(칸 이름의 N). 없으면 시험을 끝낸다.
func hostField(t *testing.T, f url.Values, id string) string {
	t.Helper()
	for k, v := range f {
		if strings.HasPrefix(k, "host.id.") && len(v) == 1 && v[0] == id {
			return strings.TrimPrefix(k, "host.id.")
		}
	}
	t.Fatalf("the form has no row for %s: %v", id, f)
	return ""
}

// externalHosts: 밖에서 만든 접근 정보. 화면이 쓰는 모양과 다르다: 주석 줄, 따옴표로 싼 칸, 비워 둔 OS · 접속 방법, 끝 줄바꿈 없음.
const externalHosts = "node_id,name,ip,port,ssh_user,ssh_key,ssh_pass,os,connection\n" +
	"# web tier\n" +
	"web,\"Web, front\",10.0.0.2,22,deploy,/home/me/.ssh/id_ed25519,,linux,ssh\n" +
	"db,DB,10.0.0.9,,deploy,,s3cret-pw,,"

// IC-U258 — **접근 정보 양식의 줄은 선언의 노드가 정하고, 파일은 「저장」을 눌렀을 때만 바뀐다.**
//
//   - 선언에만 있는 노드는 빈 줄로 보인다. 다른 노드를 고쳐 저장해도 그 빈 줄은 hosts.csv에 생기지 않는다.
//   - 밖에서 만든 파일은 화면을 열거나 다른 화면으로 가는 것으로 바뀌지 않는다. 아무것도 고치지 않고 저장해도 바이트가 같다.
//     한 행을 고치면 그 행만 다시 쓰이고 다른 행과 머리글은 글자 그대로 남는다.
//   - 노드 ID는 입력 칸이 아니다. 화면에서 줄을 더하는 조각도 없다.
func TestAccessRowsAreTheDeclaredNodes(t *testing.T) {
	s := everyScreen(t)
	if err := os.WriteFile(s.hosts, []byte(externalHosts), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, sc := range []string{ui.ScreenObserve, ui.ScreenDecl, ui.ScreenSurvey, ui.ScreenReview, ui.ScreenObserve} {
		if w := get(t, s, sc+"?lang=ko"); w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", sc, w.Code)
		}
	}
	if readFile(t, s.hosts) != externalHosts {
		t.Fatal("opening the screens changed hosts.csv")
	}
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	for _, want := range []string{`data-host-node="web-gw" data-host-access="false"`, `data-host-node="web" data-host-access="true"`,
		`data-host-node="app" data-host-access="false"`, `data-host-node="db" data-host-access="true"`, "접속 정보 미입력"} {
		if !strings.Contains(page, want) {
			t.Errorf("the form lacks %q", want)
		}
	}
	if regexp.MustCompile(`<input type="text" name="host\.id\.`).MatchString(page) || strings.Contains(page, "노드 추가") {
		t.Error("the node ID is still an input, or the form still offers to add a row")
	}
	if strings.Contains(page, "s3cret-pw") {
		t.Fatal("the screen carries the password")
	}
	if w := get(t, s, "/row/"+ui.KindHost+"?i=9&lang=ko"); w.Code == http.StatusOK && strings.Contains(w.Body.String(), "host.id.") {
		t.Error("the add-row fragment still hands out an access row")
	}

	// 아무것도 고치지 않은 저장: 바이트가 같다.
	form := hostsForm(t, page)
	if w := postForm(t, s, ui.HostsSavePath, withRev(form, s.revFor(ui.ScreenObserve))); w.Code != http.StatusSeeOther {
		t.Fatalf("save = %d\n%s", w.Code, w.Body.String())
	}
	if got := readFile(t, s.hosts); got != externalHosts {
		t.Fatalf("a save that changed nothing rewrote the file:\n%q\nwant\n%q", got, externalHosts)
	}

	// db의 주소만 고친다. web의 행(주석 · 따옴표)과 머리글은 글자 그대로이고, 선언에만 있는 노드의 빈 줄은 생기지 않는다.
	form = hostsForm(t, get(t, s, ui.ScreenObserve+"?lang=ko").Body.String())
	form.Set("host.ip."+hostField(t, form, "db"), "10.0.0.19")
	if w := postForm(t, s, ui.HostsSavePath, withRev(form, s.revFor(ui.ScreenObserve))); w.Code != http.StatusSeeOther {
		t.Fatalf("save = %d\n%s", w.Code, w.Body.String())
	}
	// 고친 행은 화면의 모양으로 다시 쓰인다: 비워 둔 포트 · OS · 접속 방법이 읽힌 값(22 · linux · ssh)으로 적힌다.
	want := strings.Replace(externalHosts, "db,DB,10.0.0.9,,deploy,,s3cret-pw,,", "db,DB,10.0.0.19,22,deploy,,s3cret-pw,linux,ssh\n", 1)
	if got := readFile(t, s.hosts); got != want {
		t.Errorf("after changing one row:\n%q\nwant\n%q", got, want)
	}
	// 주석이 있는 행(web)의 주소만 고친다. 그 주석 · 머리글 · 다른 행은 바이트까지 그대로다.
	before := readFile(t, s.hosts)
	form = hostsForm(t, get(t, s, ui.ScreenObserve+"?lang=ko").Body.String())
	form.Set("host.ip."+hostField(t, form, "web"), "10.0.0.22")
	if w := postForm(t, s, ui.HostsSavePath, withRev(form, s.revFor(ui.ScreenObserve))); w.Code != http.StatusSeeOther {
		t.Fatalf("save = %d\n%s", w.Code, w.Body.String())
	}
	want = strings.Replace(before, "web,\"Web, front\",10.0.0.2,22,", "web,\"Web, front\",10.0.0.22,22,", 1)
	if got := readFile(t, s.hosts); got != want || want == before || !strings.Contains(got, "# web tier\nweb,") {
		t.Errorf("after changing the address of a row with a comment:\n%q\nwant the comment, the header and the other rows as they were:\n%q", got, want)
	}
}

// IC-U260 — **주석이 있는 행의 ID만 옮겨도 주석 · 머리글 · 다른 행은 바이트까지 그대로다.** 밖에서 만든 파일에서 선언에 없는
// ID의 행 앞에 주석 줄이 둘 있고 포트 · OS · 접속 방법은 비어 있다. 그 행을 선언 노드로 옮겨 저장하면 그 행의 첫 칸만 바뀐다:
// 주석 두 줄, 비워 둔 칸, 머리글, 앞뒤의 행이 그대로다. 그 행을 지우면 그 주석도 함께 없어지고 다른 행은 그대로다.
func TestCommentsStayWithAMovedRow(t *testing.T) {
	const file = "node_id,name,ip,port,ssh_user,ssh_key,ssh_pass,os,connection\n" +
		"web,Web,10.0.0.2,22,deploy,/home/me/.ssh/id_ed25519,,linux,ssh\n" +
		"# was renamed in the CMDB\n" +
		"# owner: payments\n" +
		"app-old,\"App, legacy\",10.0.0.5,,deploy,,s3cret-pw,,\n" +
		"db,DB,10.0.0.9,,deploy,,,,"
	for name, c := range map[string]struct{ move, want string }{
		"moved":   {ui.HostMoveTo("app"), strings.Replace(file, "app-old,", "app,", 1)},
		"removed": {ui.HostMoveRemove, strings.Replace(file, "# was renamed in the CMDB\n# owner: payments\napp-old,\"App, legacy\",10.0.0.5,,deploy,,s3cret-pw,,\n", "", 1)},
	} {
		t.Run(name, func(t *testing.T) {
			s := everyScreen(t)
			if err := os.WriteFile(s.hosts, []byte(file), 0o600); err != nil {
				t.Fatal(err)
			}
			page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
			// 주석 두 줄 뒤이므로 그 행은 파일의 5행이다.
			if !strings.Contains(page, `data-orphan-row="5" data-orphan-id="app-old"`) {
				t.Fatalf("the orphan row is not shown with its line in the file:\n%s", page)
			}
			form := hostsForm(t, page)
			form.Set("host.move."+hostField(t, form, "app-old"), c.move)
			if w := postForm(t, s, ui.HostsSavePath, withRev(form, s.revFor(ui.ScreenObserve))); w.Code != http.StatusSeeOther {
				t.Fatalf("save = %d\n%s", w.Code, w.Body.String())
			}
			if got := readFile(t, s.hosts); got != c.want {
				t.Errorf("after the row was %s:\n%q\nwant\n%q", name, got, c.want)
			}
		})
	}
}

// IC-U259 — **선언에서 노드 이름을 바꾼 뒤, 옛 접속 행을 운영자가 고른 노드로 옮긴다.** 화면의 선언 저장으로 web을 web-01로
// 바꾸면 02는 새 이름의 빈 줄과 「접속 정보 n행: 선언에 없는 노드 web」을 보인다. 저장하기 전에는 hosts.csv가 바이트까지
// 그대로다. 그 행을 web-01로 옮겨 저장하면 **그 행의 ID만** 바뀌고 주소 · 계정 · 키 경로 · 비밀번호와 다른 행은 그대로다. 다시
// 열면 선언에 없는 행이 없고 관측을 시작할 수 있다.
//
//   - 옮길 대상에 이미 접속 행이 있으면 서버가 덮어쓰지 않고 막는다(422). 적던 다른 값은 화면에 남고, 저장된 비밀번호는
//     응답에 없다. 파일은 그대로다.
//   - 양식을 연 뒤 선언이 다시 바뀌면, 저장 직전에 지금의 선언으로 판정해 막는다(옮길 대상이 없어졌거나, 접속 정보를 적은
//     노드가 선언에서 사라졌다). 파일은 그대로다.
//   - 선언에 없는 행의 값을 고쳐 보내면 막는다. 「지움」을 고르면 그 행만 없어진다.
func TestOldAccessRowIsMovedByTheOperator(t *testing.T) {
	renamed := func(t *testing.T) (*server, string) {
		t.Helper()
		s := runReady(t, "ok")
		before := readFile(t, s.hosts)
		// 화면의 선언 저장으로 이름을 바꾼다(운영자의 경로).
		decl := screenForm(t, get(t, s, ui.ScreenDecl+"?lang=ko").Body.String())
		changed := false
		for k, v := range decl {
			if !strings.HasPrefix(k, "node.name.") || len(v) != 1 {
				continue
			}
			switch v[0] {
			case "web":
				decl.Set(k, "web-01")
				changed = true
			case "pay-db":
				// 시험 선언의 스코프에만 있던 노드다. 선언 화면은 IP가 빈 줄을 저장하지 않으므로 채운다.
				decl.Set("node.ips."+strings.TrimPrefix(k, "node.name."), "10.0.0.3")
			}
		}
		if !changed {
			t.Fatal("the declaration form has no node named web")
		}
		if w := postForm(t, s, "/decl/save", withRev(decl, s.revFor(ui.ScreenDecl))); w.Code != http.StatusSeeOther {
			t.Fatalf("decl save = %d\n%s", w.Code, w.Body.String())
		}
		if readFile(t, s.hosts) != before {
			t.Fatal("saving the declaration changed hosts.csv")
		}
		return s, before
	}

	t.Run("move, save, open again", func(t *testing.T) {
		s, before := renamed(t)
		page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
		for _, want := range []string{`data-host-node="web-01" data-host-access="false"`, `data-orphan-row="2" data-orphan-id="web"`,
			"접속 정보 2행: 선언에 없는 노드 <code>web</code>", "선언 노드 web-01로 옮김(옛 ID web → 새 ID web-01"} {
			if !strings.Contains(page, want) {
				t.Errorf("after the rename 02 lacks %q", want)
			}
		}
		if strings.Contains(page, `action="`+ui.RunPath+`"`) || readFile(t, s.hosts) != before {
			t.Fatal("02 still offers to start, or hosts.csv changed before saving")
		}
		form := hostsForm(t, page)
		form.Set("host.move."+hostField(t, form, "web"), ui.HostMoveTo("web-01"))
		if readFile(t, s.hosts) != before {
			t.Fatal("choosing the target changed hosts.csv")
		}
		if w := postForm(t, s, ui.HostsSavePath, withRev(form, s.revFor(ui.ScreenObserve))); w.Code != http.StatusSeeOther {
			t.Fatalf("save = %d\n%s", w.Code, w.Body.String())
		}
		want := strings.Replace(before, "web,Web,10.0.0.2,22,deploy,/home/me/.ssh/id_ed25519,,linux,ssh\n", "web-01,Web,10.0.0.2,22,deploy,/home/me/.ssh/id_ed25519,,linux,ssh\n", 1)
		if got := readFile(t, s.hosts); got != want || want == before {
			t.Fatalf("after the move:\n%q\nwant only the ID of that row changed:\n%q", got, want)
		}
		page = get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
		if strings.Contains(page, `data-targets="orphans"`) || !strings.Contains(page, `data-host-node="web-01" data-host-access="true"`) ||
			!strings.Contains(page, `action="`+ui.RunPath+`"`) {
			t.Errorf("opened again: want no row outside the declaration, web-01 with access, and the start form:\n%s", section(page, "observe-targets"))
		}
	})

	t.Run("the target already has access details", func(t *testing.T) {
		s, before := renamed(t)
		form := hostsForm(t, get(t, s, ui.ScreenObserve+"?lang=ko").Body.String())
		// db에는 이미 접속 행이 있다(비밀번호도 있다). 예전 화면이나 손으로 만든 요청이 web의 행을 db로 옮기려 한다.
		form.Set("host.move."+hostField(t, form, "web"), ui.HostMoveTo("db"))
		form.Set("host.ip."+hostField(t, form, "app"), "10.9.9.9") // 함께 적던 다른 값
		w := postFormLang(t, s, ui.HostsSavePath, "ko", withRev(form, s.revFor(ui.ScreenObserve)))
		body := w.Body.String()
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "옮길 대상 노드에 이미 접속 정보가 있습니다. 덮어쓰지 않았고") {
			t.Fatalf("move onto a node with access = %d, want 422 saying nothing was overwritten\n%s", w.Code, body)
		}
		if !strings.Contains(body, `value="10.9.9.9"`) || !strings.Contains(body, `selected>선언 노드`) && !strings.Contains(body, `data-orphan-id="web"`) {
			t.Error("the refused page lost what was typed or the row that was being moved")
		}
		if strings.Contains(body, "s3cret-pw") {
			t.Error("the refused page carries the stored password")
		}
		if readFile(t, s.hosts) != before {
			t.Error("a refused move was written")
		}
	})

	t.Run("the declaration changes again while editing", func(t *testing.T) {
		s, before := renamed(t)
		form := hostsForm(t, get(t, s, ui.ScreenObserve+"?lang=ko").Body.String())
		form.Set("host.move."+hostField(t, form, "web"), ui.HostMoveTo("web-01"))
		form.Set("host.ip."+hostField(t, form, "app"), "10.0.0.5")
		// 양식을 연 뒤 선언이 다시 바뀐다: web-01은 web-02가 되고 app은 없어진다.
		var d map[string]any
		if err := json.Unmarshal([]byte(readFile(t, s.decl)), &d); err != nil {
			t.Fatal(err)
		}
		var nodes []any
		for _, n := range d["nodes"].([]any) {
			m := n.(map[string]any)
			switch m["name"] {
			case "app":
				continue
			case "web-01":
				m["name"] = "web-02"
			}
			nodes = append(nodes, m)
		}
		d["nodes"] = nodes
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(s.decl, b, 0o644); err != nil {
			t.Fatal(err)
		}
		w := postFormLang(t, s, ui.HostsSavePath, "ko", withRev(form, s.revFor(ui.ScreenObserve)))
		body := w.Body.String()
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "옮길 대상이 지금 선언에 없습니다") ||
			!strings.Contains(body, "지금 선언에 없는 노드입니다") {
			t.Fatalf("save after the declaration changed = %d, want 422 naming both rows\n%s", w.Code, body)
		}
		if readFile(t, s.hosts) != before {
			t.Error("a save refused against the current declaration was written")
		}
	})

	t.Run("an orphan row is not edited; removing drops only that row", func(t *testing.T) {
		s, before := renamed(t)
		form := hostsForm(t, get(t, s, ui.ScreenObserve+"?lang=ko").Body.String())
		n := hostField(t, form, "web")
		edited := url.Values{}
		for k, v := range form {
			edited[k] = v
		}
		edited.Set("host.ip."+n, "10.6.6.6")
		if w := postFormLang(t, s, ui.HostsSavePath, "ko", withRev(edited, s.revFor(ui.ScreenObserve))); w.Code != http.StatusUnprocessableEntity ||
			!strings.Contains(w.Body.String(), "선언에 없는 노드의 접속 정보는 고칠 수 없습니다") || readFile(t, s.hosts) != before {
			t.Fatalf("editing an orphan row = %d, want 422 and the file unchanged", w.Code)
		}
		form.Set("host.move."+n, ui.HostMoveRemove)
		if w := postForm(t, s, ui.HostsSavePath, withRev(form, s.revFor(ui.ScreenObserve))); w.Code != http.StatusSeeOther {
			t.Fatalf("remove = %d\n%s", w.Code, w.Body.String())
		}
		want := strings.Replace(before, "web,Web,10.0.0.2,22,deploy,/home/me/.ssh/id_ed25519,,linux,ssh\n", "", 1)
		if got := readFile(t, s.hosts); got != want {
			t.Errorf("after removing the orphan row:\n%q\nwant\n%q", got, want)
		}
	})
}
