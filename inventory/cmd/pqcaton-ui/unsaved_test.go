// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

var (
	guardedForm = regexp.MustCompile(`<form[^>]*\sdata-guard="([^"]*)"[^>]*>`)
	stickyRow   = regexp.MustCompile(`(?s)<div class="actions sticky">.*?</div>`)
)

// IC-U222 — **편집 양식 넷은 저장하지 않은 입력을 지키는 표시를 달고 나간다.** 01 선언 · 02 스코프 · 03 접근 정보 ·
// 05 판정의 양식마다 `data-guard`가 하나씩 있고, 화면 아래에 붙는 버튼 줄이 하나씩 있으며, 그 줄에 저장 버튼과 「저장하지
// 않은 변경」 표시가 설 자리가 그 말로 있다. 처음 그린 양식은 깨끗한 상태(`data-guard=""`)이고, 저장을 거절한 01 화면만
// 처음부터 저장하지 않은 입력이 있는 상태(`dirty`)다. 편집 양식이 아닌 화면(04 · 06 · 조회)에는 없다.
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

// IC-U223 — **02에서 규칙을 고친 채 「저장만」이나 「확정」을 누르면 쓰지 않고 「규칙 저장」을 먼저 누르라고 안내한다(422).**
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
