// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

var (
	guardedForm = regexp.MustCompile(`<form[^>]*\sdata-guard="([^"]*)"[^>]*>`)
	stickyRow   = regexp.MustCompile(`(?s)<div class="actions sticky">.*?</div>`)
)

// IC-U222 — **편집 양식 넷은 저장하지 않은 입력을 지키는 표시를 달고 나간다.** 01 선언 · 자산 스코프 화면 · 02 접근 정보 ·
// 04 판정의 양식마다 `data-guard`가 하나씩 있고, 화면 아래에 붙는 버튼 줄이 하나씩 있으며, 그 줄에 저장 버튼과 「저장하지
// 않은 변경」 표시가 설 자리가 그 말로 있다. 처음 그린 양식은 깨끗한 상태(`data-guard=""`)이고, 저장을 거절한 01 화면만
// 처음부터 저장하지 않은 입력이 있는 상태(`dirty`)다. 편집 양식이 아닌 화면(03 · 05 · 조회)에는 없다.
func TestEditFormsGuardUnsavedInput(t *testing.T) {
	s, _ := fullServer(t)
	for _, c := range []struct{ lang, note string }{
		{"ko", "저장하지 않은 변경이 있습니다"},
		{"en", "There are unsaved changes"},
	} {
		for _, sc := range []string{ui.ScreenDecl, ui.ScreenScope, ui.ScreenObserve, ui.ScreenReview} {
			body := get(t, s, sc+"?lang="+c.lang).Body.String()
			forms := guardedForm.FindAllStringSubmatch(body, -1)
			if len(forms) != 1 || forms[0][1] != "" {
				t.Errorf("%s (%s): guarded forms = %v, want one clean form", sc, c.lang, forms)
			}
			rows := stickyRow.FindAllString(body, -1)
			if len(rows) != 1 {
				t.Errorf("%s (%s): %d sticky action rows, want 1", sc, c.lang, len(rows))
				continue
			}
			if !strings.Contains(rows[0], "<button") || !strings.Contains(rows[0], `aria-live="polite" data-unsaved-text="`+c.note+`"`) {
				t.Errorf("%s (%s): the sticky row has no save button or no place for the unsaved note:\n%s", sc, c.lang, rows[0])
			}
			if strings.Contains(rows[0], ">"+c.note+"<") {
				t.Errorf("%s (%s): the unsaved note is already written on a screen drawn from the files", sc, c.lang)
			}
		}
		for _, sc := range []string{ui.ScreenSurvey, ui.ScreenApprove, ui.ScreenInventory} {
			if body := get(t, s, sc+"?lang="+c.lang).Body.String(); guardedForm.MatchString(body) || stickyRow.MatchString(body) {
				t.Errorf("%s (%s): a screen without an edit form carries the guard", sc, c.lang)
			}
		}
	}
	// 저장을 거절한 화면: 적은 값이 저장되지 않은 채 남아 있으므로 처음부터 dirty다.
	form := screenForm(t, get(t, s, ui.ScreenDecl+"?lang=ko").Body.String())
	form.Set("node.name.0", "")
	w := postFormLang(t, s, "/decl/save", "ko", form)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
	if forms := guardedForm.FindAllStringSubmatch(w.Body.String(), -1); len(forms) != 1 || forms[0][1] != "dirty" {
		t.Errorf("the refused screen's form is not marked dirty: %v", forms)
	}
}

// IC-UI65 — **이탈 경고 스크립트와 붙는 버튼 줄의 스타일이 바이너리에 담겨 나간다.** `app.js`는 `form[data-guard]`를 보고
// `beforeunload`에서 묻고, 그 양식을 보내는 중(`submit`)에는 묻지 않는다. `app.css`는 버튼 줄을 화면 아래에 붙이고
// (`position: sticky`), 붙은 줄이 포커스가 간 칸을 가리지 않게 스크롤 여백(`scroll-padding-bottom`)을 둔다. 실제로 묻는지는
// 브라우저에서 확인한다(스크립트를 돌리는 시험이 없다).
func TestUnsavedGuardShips(t *testing.T) {
	s, _ := fullServer(t)
	js := get(t, s, ui.StaticPath+"app.js").Body.String()
	for _, want := range []string{`form[data-guard]`, `"beforeunload"`, `"submit"`, `data-unsaved-text`, `"dirty"`} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js has no %s", want)
		}
	}
	css := get(t, s, ui.StaticPath+"app.css").Body.String()
	if !regexp.MustCompile(`\.actions\.sticky\s*\{[^}]*position:\s*sticky;[^}]*bottom:\s*0`).MatchString(css) {
		t.Error("app.css does not stick the action row to the bottom")
	}
	if !regexp.MustCompile(`html\s*\{[^}]*scroll-padding-bottom:`).MatchString(css) {
		t.Error("app.css leaves no scroll padding under the sticky row")
	}
}

// IC-U223 — **자산 스코프 화면에서 규칙을 고친 채 「저장만」이나 「확정」을 누르면 쓰지 않고 「규칙 저장」을 먼저 누르라고 안내한다(422).**
// 그 버튼들은 세션만 쓰고 화면을 파일에서 다시 그리므로, 예전에는 고친 규칙이 알림 없이 사라졌다. 계층 파일과 세션 파일은
// 바이트까지 그대로이고, 화면은 고친 규칙과 적은 승인자를 그대로 돌려주며 처음부터 저장하지 않은 상태(`dirty`)다. 그
// 화면의 양식으로 「규칙 저장」을 누르면 규칙과 승인자가 함께 저장된다. 규칙을 고치지 않은 양식의 「저장만」은 전처럼 된다.
func TestScopeSaveDoesNotDropEditedRules(t *testing.T) {
	s, _ := fullServer(t)
	layer := s.layers[0]
	clean := formOf(get(t, s, ui.ScreenScope+"?lang=ko").Body.String())
	if clean.Get(ui.RulesShownField) == "" || clean.Get("rule.0.0.lib") == "" {
		t.Fatalf("the scope form carries no rule editor: %v", clean)
	}
	// 고치지 않은 양식의 「저장만」은 저장된다.
	location(t, postFormLang(t, s, "/scope/save", "ko", clean))

	for _, post := range []string{"/scope/finalize/confirm", "/scope/save"} {
		form := formOf(get(t, s, ui.ScreenScope+"?lang=ko").Body.String())
		form.Set("rule.0.0.note", "typed-note")
		form.Set("rule.0.1.action", "exclude")
		form.Set("rule.0.1.runtime", "jca")
		form.Set("rule.0.1.lib", "typed-lib")
		form.Set("reviewer", "typed-reviewer")
		rules, session := readFile(t, layer), readFile(t, s.scope)

		w := postFormLang(t, s, post, "ko", form)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status = %d, want 422\n%s", post, w.Code, w.Body.String())
		}
		if readFile(t, layer) != rules || readFile(t, s.scope) != session {
			t.Fatalf("%s: a file changed although the save was refused", post)
		}
		body := w.Body.String()
		for _, want := range []string{
			"저장하지 않았습니다", "먼저 「규칙 저장」을 누르십시오", `href="#rules-save"`, `id="rules-save"`,
			`name="rule.0.0.note" value="typed-note"`, `name="rule.0.1.lib" value="typed-lib"`,
			`name="reviewer" value="typed-reviewer"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the refused screen has no %s", post, want)
			}
		}
		if forms := guardedForm.FindAllStringSubmatch(body, -1); len(forms) != 1 || forms[0][1] != "dirty" {
			t.Errorf("%s: the refused screen's form is not marked dirty: %v", post, forms)
		}

		if post != "/scope/save" {
			continue
		}
		// 그 화면의 양식 그대로 「규칙 저장」.
		again := formOf(body)
		if again.Get(ui.RevField) != form.Get(ui.RevField) {
			t.Fatalf("the refused screen does not carry the rev it was sent")
		}
		location(t, postFormLang(t, s, "/scope/rules", "ko", again))
		if got := readFile(t, layer); !strings.Contains(got, "typed-note") || !strings.Contains(got, "exclude,jca,typed-lib") {
			t.Errorf("the layer file after Save rules:\n%s", got)
		}
		if b, err := os.ReadFile(s.scope); err != nil || !strings.Contains(string(b), "typed-reviewer") {
			t.Errorf("the session after Save rules does not hold the reviewer (%v)", err)
		}
	}
}

// IC-U224 — **자산 스코프 화면의 규칙은 「제거」로 표시한 것만 지워지고, 조건을 비운 규칙은 저장을 거절한다.** 세 규칙의 가운데 줄을
// 「제거」하면 서버는 그 줄을 「삭제 예정」으로 다시 그릴 뿐 아무것도 쓰지 않고, 「되돌리기」는 원래 줄을 돌려준다.
// 표시한 양식으로 「규칙 저장」을 누르면 앞뒤 두 규칙이 그 순서로 남고, 다시 연 화면에 두 줄이 있다. 기존 규칙의 조건
// 세 칸을 비워 「규칙 저장」을 누르면 422이고 계층 파일과 세션 파일은 바이트까지 그대로이며, 화면은 그 줄로 가는 링크와
// 적은 note · 승인자를 보인다. 조건 없이 exclude만 고른 새 줄도 422다. 기본값 그대로인 빈 새 줄은 버리고 저장한다.
func TestScopeRulesAreRemovedOnlyByRemove(t *testing.T) {
	s, _ := fullServer(t)
	layer := s.layers[0]
	three := "action,runtime,lib,app_key,note\n" +
		"exclude,openssl,liba,/a,first\n" +
		"include,jca,libb,/b,second\n" +
		"exclude,cng,libc,/c,third\n"
	if err := os.WriteFile(layer, []byte(three), 0o644); err != nil {
		t.Fatal(err)
	}
	open := func() url.Values { return formOf(get(t, s, ui.ScreenScope+"?lang=ko").Body.String()) }
	form := open()
	if form.Get("rule.0.1.was") == "" || form.Get("rule.0.2.lib") != "libc" {
		t.Fatalf("the scope form does not carry the three existing rules: %v", form)
	}

	// 「제거」: 그 줄의 값을 실어 보내면 삭제 예정 줄이 온다. 파일은 그대로다.
	q := url.Values{"layer": {"0"}, "i": {"1"}, "undo": {"0"}, "lang": {"ko"}}
	for _, k := range []string{"action", "runtime", "lib", "app_key", "note", "was"} {
		q.Set("rule.0.1."+k, form.Get("rule.0.1."+k))
	}
	marked := get(t, s, ui.RuleRemovePath+"?"+q.Encode())
	if marked.Code != http.StatusOK || readFile(t, layer) != three {
		t.Fatalf("Remove: status %d, or the layer file changed", marked.Code)
	}
	for _, want := range []string{`name="rule.0.1.remove" value="1"`, `name="rule.0.1.lib" value="libb"`, "삭제 예정", "되돌리기", `class="removing"`} {
		if !strings.Contains(marked.Body.String(), want) {
			t.Errorf("the marked row has no %s:\n%s", want, marked.Body.String())
		}
	}
	q.Set("undo", "1")
	if undone := get(t, s, ui.RuleRemovePath+"?"+q.Encode()).Body.String(); strings.Contains(undone, ".remove") || !strings.Contains(undone, `name="rule.0.1.lib" value="libb"`) {
		t.Errorf("Undo does not give the row back:\n%s", undone)
	}
	for _, bad := range []string{"?layer=9&i=0", "?layer=0&i=-1", "?layer=x&i=0"} {
		if w := get(t, s, ui.RuleRemovePath+bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", bad, w.Code)
		}
	}

	// 가운데 줄을 표시한 양식으로 「규칙 저장」.
	form.Set("rule.0.1.remove", "1")
	location(t, postFormLang(t, s, "/scope/rules", "ko", form))
	two := "action,runtime,lib,app_key,note\nexclude,openssl,liba,/a,first\nexclude,cng,libc,/c,third\n"
	if got := readFile(t, layer); got != two {
		t.Fatalf("the layer file after removing the middle rule:\n%s", got)
	}
	form = open()
	if form.Get("rule.0.0.lib") != "liba" || form.Get("rule.0.1.lib") != "libc" || form.Get("rule.0.2.action") != "" {
		t.Fatalf("the reopened screen does not show the two remaining rules in order: %v", form)
	}

	// 기존 규칙의 조건 세 칸을 비운다(note는 남는다). 승인자도 함께 적는다.
	session := readFile(t, s.scope)
	form.Set("rule.0.1.runtime", "")
	form.Set("rule.0.1.lib", "")
	form.Set("rule.0.1.app_key", "")
	form.Set("reviewer", "typed-reviewer")
	w := postFormLang(t, s, "/scope/rules", "ko", form)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422\n%s", w.Code, w.Body.String())
	}
	if readFile(t, layer) != two || readFile(t, s.scope) != session {
		t.Fatal("a file changed although Save rules was refused")
	}
	body := w.Body.String()
	for _, want := range []string{
		"저장하지 않았습니다", `href="#f-rule.0.1.runtime"`, `aria-invalid="true"`, "별표(*)를 적으십시오",
		`name="rule.0.1.note" value="third"`, `name="rule.0.0.lib" value="liba"`, `name="reviewer" value="typed-reviewer"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the refused screen has no %s", want)
		}
	}
	// 같은 양식을 「저장만」으로 보내도 쓰지 않는다.
	if w := postFormLang(t, s, "/scope/save", "ko", form); w.Code != http.StatusUnprocessableEntity || readFile(t, s.scope) != session {
		t.Errorf("Save only with an emptied rule: status %d, or the session changed", w.Code)
	}

	// 새 줄: 조건 없이 exclude만 고르면 422, 기본값 그대로면 버리고 저장한다.
	form = open()
	form.Set("rule.0.2.action", "exclude")
	if w := postFormLang(t, s, "/scope/rules", "ko", form); w.Code != http.StatusUnprocessableEntity || readFile(t, layer) != two {
		t.Errorf("a new row with exclude and no conditions: status %d, or the layer file changed", w.Code)
	}
	form.Set("rule.0.2.action", "include")
	form.Set("rule.0.2.runtime", "")
	location(t, postFormLang(t, s, "/scope/rules", "ko", form))
	if got := readFile(t, layer); got != two {
		t.Errorf("an unused new row changed the layer file:\n%s", got)
	}
}

// IC-UI68 — **절차 카드 다섯이 한 줄에 서는 격자와, 02의 SSH 키 경로 칸의 최소 폭이 실려 나간다.** 격자는 카드 다섯에
// 번호 없는 링크 둘까지(`repeat(5, …) auto auto`: 조회, 그리고 관측 화면이 없을 때의 자산 스코프)이고, 좁아지면 세 칸, 더 좁아지면 한 칸이다. 예전의 네 칸 격자는 남아 있지 않다.
// 키 경로 칸은 `wide`이고(포커스가 가면 더 넓어진다) 전체 경로를 title로도 든다. 실제 배치는 브라우저에서 1280px · 375px로 확인한다.
func TestStepGridAndKeyPathWidthShip(t *testing.T) {
	s, _ := fullServer(t)
	css := get(t, s, ui.StaticPath+"app.css").Body.String()
	if !regexp.MustCompile(`body\.wb \.steps\s*\{[^}]*grid-template-columns:\s*repeat\(5, minmax\(0, 1fr\)\) auto auto`).MatchString(css) {
		t.Error("app.css does not lay the five step cards and the unnumbered tabs on one row")
	}
	if strings.Contains(css, "repeat(4, minmax(0, 1fr))") || strings.Contains(css, "repeat(2, 1fr)") {
		t.Error("app.css still has the grid made for four step cards")
	}
	if !regexp.MustCompile(`(?s)@media \(max-width: 1100px\) \{.*?\.steps \{ grid-template-columns: repeat\(3,`).MatchString(css) ||
		!regexp.MustCompile(`input\.wide\s*\{\s*min-width:\s*14rem`).MatchString(css) ||
		!regexp.MustCompile(`input\.wide:focus\s*\{\s*min-width:\s*min\(38rem`).MatchString(css) {
		t.Error("app.css has no three-column step grid for narrower screens, or no minimum width for the key path field")
	}
	body := get(t, s, ui.ScreenObserve+"?lang=en").Body.String()
	key := regexp.MustCompile(`<input[^>]*name="host\.key\.0"[^>]*>`).FindString(body)
	if !strings.Contains(key, `class="wide"`) || !strings.Contains(key, `title="/home/me/.ssh/id_ed25519"`) {
		t.Errorf("the key path field is not wide or does not carry the full path as its title:\n%s", key)
	}
	if n := len(cards(t, body)); n != 5 {
		t.Errorf("%d step cards, want 5", n)
	}
}

// IC-U225 — **04에서 고른 조치 종류와 위임 수준, L3의 훅이 저장되고 다시 열었을 때 그대로 골라져 있다.** 화면의 이름으로
// 고른 조치 종류는 계약 어휘(`REMEDIATION_KIND_PROVIDER_INJECT`)로 세션에 남는다. L3로 저장하면 훅 줄이 펼쳐진 채로
// 다시 그려지고, L2로 바꿔 저장하면 훅 줄은 접히되 적은 값은 세션과 양식에 남아 있고 그 사실을 알리는 줄이 보인다.
// 좁은 화면의 카드 배치와 접힌 줄을 접어 두는 스타일이 실려 나간다.
func TestJudgmentChoicesSurviveASave(t *testing.T) {
	s, _ := withDecl(t)
	const id = "host://local/openssl/libssl"
	form := formOf(get(t, s, ui.ScreenReview+"?lang=ko").Body.String())
	form.Set("kind:"+id, "REMEDIATION_KIND_PROVIDER_INJECT")
	form.Set("level:"+id, "L3")
	form.Set("target:"+id, "ML-KEM (FIPS 203)")
	form.Set("activate:"+id, "systemctl reload nginx")
	form.Set("restart:"+id, "systemctl restart nginx")
	form.Set("rollback:"+id, "typed-rollback")
	location(t, postFormLang(t, s, "/save", "ko", form))
	saved := readFile(t, s.path)
	for _, want := range []string{`"REMEDIATION_KIND_PROVIDER_INJECT"`, `"L3"`, "systemctl reload nginx", "typed-rollback"} {
		if !strings.Contains(saved, want) {
			t.Errorf("the session file has no %s", want)
		}
	}
	hooks := regexp.MustCompile(`<tr[^>]*data-l3-for="level:` + regexp.QuoteMeta(id) + `"[^>]*>`)
	kept := regexp.MustCompile(`<tr[^>]*data-l3-kept-for="level:` + regexp.QuoteMeta(id) + `"[^>]*>`)
	for _, c := range []struct{ lang, name string }{
		{"ko", "프로바이더 추가 및 설정 (provider_inject)"},
		{"en", "Add and configure a provider (provider_inject)"},
	} {
		body := get(t, s, ui.ScreenReview+"?lang="+c.lang).Body.String()
		for _, want := range []string{
			`<option value="REMEDIATION_KIND_PROVIDER_INJECT" selected>` + c.name + `</option>`,
			`<option value="L3" selected>L3</option>`,
			`name="activate:` + id + `" value="systemctl reload nginx"`, `name="rollback:` + id + `" value="typed-rollback"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the reopened screen has no %s", c.lang, want)
			}
		}
		if strings.Contains(hooks.FindString(body), " hidden") || !strings.Contains(kept.FindString(body), " hidden") {
			t.Errorf("%s: at L3 the hooks row is folded, or the kept-values row is shown", c.lang)
		}
	}

	// L2로 바꿔 저장한다. 훅에 적은 값은 지워지지 않는다.
	form = formOf(get(t, s, ui.ScreenReview+"?lang=ko").Body.String())
	if form.Get("kind:"+id) != "REMEDIATION_KIND_PROVIDER_INJECT" || form.Get("level:"+id) != "L3" {
		t.Fatalf("the reopened form does not carry the saved choices: kind %q, level %q", form.Get("kind:"+id), form.Get("level:"+id))
	}
	form.Set("level:"+id, "L2")
	location(t, postFormLang(t, s, "/save", "ko", form))
	body := get(t, s, ui.ScreenReview+"?lang=ko").Body.String()
	if !strings.Contains(hooks.FindString(body), " hidden") || strings.Contains(kept.FindString(body), " hidden") {
		t.Error("at L2 the hooks row is not folded, or the kept-values row is not shown")
	}
	if !strings.Contains(body, `name="restart:`+id+`" value="systemctl restart nginx"`) || !strings.Contains(readFile(t, s.path), "systemctl restart nginx") {
		t.Error("moving to L2 dropped the hook values")
	}

	css := get(t, s, ui.StaticPath+"app.css").Body.String()
	for _, want := range []string{"body.wb table.editable tr[hidden] { display: none; }", "body.wb table.judge tbody.item { border: 1px solid var(--line);"} {
		if !strings.Contains(css, want) {
			t.Errorf("the stylesheet has no %q", want)
		}
	}
	if js := get(t, s, ui.StaticPath+"app.js").Body.String(); !strings.Contains(js, "select[data-level]") || !strings.Contains(js, "data-l3-kept-for") {
		t.Error("app.js does not fold and unfold the L3 hooks")
	}
}

// IC-U226 — **플레이북 셋의 모든 단계가 진행 표시의 단계로 읽히고, 도는 동안 02 카드가 「관측 중」이다.** 임베드된
// 점검 · 관측 · 정리 플레이북의 단계 이름 하나하나가 여섯 단계 가운데 하나로 읽힌다(이름을 바꾸고 화면을 안 고치면
// 진행 표시가 그 단계에서 멈춘다). 실행이 도는 동안에는 어느 화면에서든 02 카드에 「관측 중」이 적히고, 02를 다시 열면
// 같은 실행의 상태 · 시작 시각(시간대 포함) · 지금 단계가 그대로 보인다. 실행이 없으면 카드는 노드 수를 적는다.
func TestObservationProgressFollowsThePlaybooks(t *testing.T) {
	task := regexp.MustCompile(`(?m)^\s+- name: (.+)\n(\s+)(\S+)`)
	for play, book := range map[string][]byte{"pqcaton observation check": checkPlaybook, "pqcaton observation": observePlaybook, "pqcaton observation cleanup": cleanupPlaybook} {
		if !strings.Contains(string(book), "- name: "+play+"\n") {
			t.Fatalf("the playbook is not named %q", play)
		}
		n := 0
		for _, m := range task.FindAllStringSubmatch(string(book), -1) {
			if m[3] == "block:" {
				continue // 묶음의 이름은 단계로 찍히지 않는다
			}
			n++
			if ui.PhaseOfTask(play, m[1]) == "" {
				t.Errorf("%s: the task %q is not read as any phase", play, m[1])
			}
		}
		if n == 0 {
			t.Errorf("%s: no task found to check", play)
		}
	}
	if ui.PhaseOfTask("pqcaton observation", "A task this build does not know") != "" || ui.PhaseOfTask("another play", "Gathering Facts") != "" {
		t.Error("an unknown task or play is read as a phase")
	}

	s, _ := fullServer(t)
	if got := cards(t, get(t, s, ui.ScreenDecl+"?lang=ko").Body.String())["02"]; !strings.Contains(got, "관측할 노드 2대") {
		t.Fatalf("with no run the 02 card says %q", got)
	}
	started := time.Date(2026, 11, 23, 7, 8, 4, 0, time.UTC)
	s.run.job = &runJob{done: make(chan struct{}),
		m: runManifest{ID: "20261123T070804-aaaaaaaaaaaa", State: ui.RunRunning, Started: started, Window: 60,
			Nodes: []runSlot{{Alias: "n1", Node: "web"}, {Alias: "n2", Node: "db"}}},
		log: []string{"PLAY [pqcaton observation] ****", "", "TASK [Make the staging directory] ****", "changed: [n1]", "",
			"TASK [Observe TLS and SSH handshakes] ****"}}
	for _, c := range []struct{ lang, card, running, phase string }{
		{"ko", "관측 중", "관측 중입니다(2026-11-23 07:08:04 UTC 시작)", "지금 단계 5/6"},
		{"en", "observing now", "Observing (started at 2026-11-23 07:08:04 UTC)", "Step 5 of 6 now"},
	} {
		for _, sc := range []string{ui.ScreenDecl, ui.ScreenReview, ui.ScreenObserve} {
			if got := cards(t, get(t, s, sc+"?lang="+c.lang).Body.String())["02"]; !strings.Contains(got, c.card) {
				t.Errorf("%s (%s): while a run is going the 02 card says %q", sc, c.lang, got)
			}
		}
		// 다시 열어도(새로고침) 같은 실행이 이어진다.
		for i := 0; i < 2; i++ {
			body := get(t, s, ui.ScreenObserve+"?lang="+c.lang).Body.String()
			for _, want := range []string{c.running, c.phase, "web: ", `action="` + ui.RunStopPath + `"`} {
				if !strings.Contains(body, want) {
					t.Errorf("%s: the reopened 02 screen has no %s", c.lang, want)
				}
			}
			if strings.Contains(body, "db: ") {
				t.Errorf("%s: a node with no line in the log is shown", c.lang)
			}
		}
	}
	// 도는 동안 다시 받는 것은 요약과 로그 조각이고, 그 사이의 중지 버튼은 다시 받는 조각 밖에 있다.
	body := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	live, stop, logAt := strings.Index(body, `id="run-live"`), strings.Index(body, `action="`+ui.RunStopPath+`"`), strings.Index(body, `id="run-log"`)
	if live < 0 || stop < live || logAt < stop || strings.Count(body, `hx-trigger="every 2s"`) != 2 {
		t.Errorf("the polled summary, the Stop form and the polled log are not in that order (at %d, %d, %d)", live, stop, logAt)
	}
	if strings.Index(body, `id="run-status"`) > strings.Index(body, `<ul class="checks">`) {
		t.Error("the run status sits below the pre-start checks")
	}
	part := func(name, state string) *httptest.ResponseRecorder {
		return get(t, s, ui.RunPath+"?lang=ko&"+ui.RunPartParam+"="+name+"&"+ui.RunStateParam+"="+state)
	}
	if w := part(ui.RunPartLive, "running"); w.Header().Get("HX-Retarget") != "" || !strings.Contains(w.Body.String(), `id="run-live"`) || strings.Contains(w.Body.String(), ui.RunStopPath) {
		t.Errorf("the summary part of a run in the same state is not just the summary:\n%s", w.Body.String())
	}
	if w := part(ui.RunPartLog, "running"); !strings.Contains(w.Body.String(), `id="run-log"`) || !strings.Contains(w.Body.String(), "Observe TLS and SSH handshakes") {
		t.Errorf("the log part does not carry the log:\n%s", w.Body.String())
	}
	// 상태가 바뀌었으면 요약 조각의 응답이 상자 전체를 갈아 끼우고, 로그 조각은 아무것도 바꾸지 않는다.
	if w := part(ui.RunPartLive, "finishing"); w.Header().Get("HX-Retarget") != "#run-status" || w.Header().Get("HX-Reswap") != "outerHTML" || !strings.Contains(w.Body.String(), `id="run-status"`) {
		t.Errorf("a summary part asked for under another state does not replace the whole box: %v", w.Header())
	}
	if w := part(ui.RunPartLog, "finishing"); w.Code != http.StatusNoContent {
		t.Errorf("a log part asked for under another state: status %d, want 204", w.Code)
	}
	// 실행이 끝나면 요약 조각의 응답이 화면을 통째로 새로 받게 한다: 카드의 「관측 중」이 걷히고 시작 양식이 다시 나온다.
	s.run.job.m.State, s.run.job.m.Ended = ui.RunDone, started.Add(time.Minute)
	close(s.run.job.done)
	if w := part(ui.RunPartLive, "running"); w.Header().Get("HX-Refresh") != "true" || w.Header().Get("HX-Retarget") != "" {
		t.Errorf("a summary part asked for after the run ended does not refresh the screen: %v", w.Header())
	}
	if got := cards(t, get(t, s, ui.ScreenObserve+"?lang=ko").Body.String())["02"]; strings.Contains(got, "관측 중") {
		t.Errorf("after the run ended the 02 card still says %q", got)
	}
	js := get(t, s, ui.StaticPath+"app.js").Body.String()
	if !strings.Contains(js, "details[data-keep-open]") {
		t.Error("app.js does not keep the detailed log open across the polling swaps")
	}
}
