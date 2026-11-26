// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// IC-U232 — **설정 화면은 띄울 때 정해진 값을 보이고, 비밀은 HTML에 싣지 않는다.**
//
// 상단에서 걷어 낸 것(작업 파일의 전체 경로)과 흩어져 있던 것(pqcota 연결의 출처 · 키의 자리와 상태 · 수집기 · 판 · 주소)을 한
// 화면에 모은다. 연결 문자열 · 비밀번호 · 키의 내용은 응답 어디에도 없다. 값을 바꾸는 요청은 받지 않고, 표시 언어만 여기서
// 바꾼다. 어느 화면에서든 상단에서 설정으로 갈 수 있고, 상단에는 「직접 설치」와 경로가 없다.
func TestSettingsShowWhatTheScreenStartedWith(t *testing.T) {
	s, _ := activationReady(t)
	dir := filepath.Dir(s.path)
	s.addr = "127.0.0.1:8765"
	keyDir := t.TempDir()
	s.approvalKey, s.keyWhere, s.keySource = filepath.Join(keyDir, "approval.key"), "pqcaton/approval.key", ui.KeyDefault
	if err := os.WriteFile(s.approvalKey, []byte("PQCOTA_SIGN_KEY=SECRETPRIVATEKEYMATERIAL\nPQCOTA_VERIFY_KEY=SECRETPUBLICKEYMATERIAL\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(upstream.DSNEnv, "postgres://secretuser:secretpw@db.internal:5432/pqcota")
	s.dsnFromEnv = true

	for _, lang := range []string{"ko", "en"} {
		w := get(t, s, ui.ScreenSettings+"?lang="+lang)
		body := w.Body.String()
		if w.Code != http.StatusOK {
			t.Fatalf("settings (%s): %d", lang, w.Code)
		}
		want := []string{"설정", "화면을 다시 띄웁니다", "환경 변수 PQCOTA_DSN", "pqcaton/approval.key", "-approval-key", "-addr", "127.0.0.1:8765", "저장 가능", "<code>-decl</code> 플래그로 줍니다", "<code>-org</code> 플래그로 줍니다"}
		if lang == "en" {
			want = []string{"Settings", "restart the screen after changing it", "the environment variable PQCOTA_DSN", "pqcaton/approval.key", "-approval-key", "-addr", "127.0.0.1:8765", "Saving enabled", "Give it with the <code>-decl</code> flag", "Give it with the <code>-org</code> flag"}
		}
		for _, sub := range append(want, s.results, s.decl, s.orgName(), `data-setting="dsn"`, `data-setting="approval-key"`, `data-setting="file-results"`,
			`data-lang-current="`+lang+`"`, `data-lang-switch=`, `aria-current="page"`) {
			if !strings.Contains(body, sub) {
				t.Errorf("settings (%s) lacks %q", lang, sub)
			}
		}
		for _, secret := range []string{"secretuser", "secretpw", "db.internal", "SECRETPRIVATEKEYMATERIAL", "SECRETPUBLICKEYMATERIAL", "postgres://"} {
			if strings.Contains(body, secret) {
				t.Errorf("settings (%s) carries %q", lang, secret)
			}
		}
		if strings.Contains(body, "`") {
			t.Errorf("settings (%s) shows a raw backtick instead of code", lang)
		}
		if strings.Contains(body, "<form") && strings.Contains(body, `method="post"`) {
			t.Errorf("settings (%s) has a form that changes something", lang)
		}
	}
	// 값을 바꾸는 요청은 없다.
	if w := postForm(t, s, ui.ScreenSettings, nil); w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("POST to the settings: %d", w.Code)
	}

	// 어느 화면에서든 상단에 조직과 설정 링크가 있고, 「직접 설치」와 작업 파일의 전체 경로는 없다.
	for _, sc := range []string{ui.ScreenDecl, ui.ScreenScope, ui.ScreenObserve, ui.ScreenSurvey, ui.ScreenReview, ui.ScreenApprove, ui.ScreenInventory, ui.ScreenHistTopo} {
		for _, lang := range []string{"ko", "en"} {
			body := get(t, s, sc+"?lang="+lang).Body.String()
			head, _, _ := strings.Cut(body, `<main id="main">`)
			for _, sub := range []string{`href="` + ui.ScreenSettings + `" data-settings-link`, `<strong data-org>` + s.orgName() + `</strong>`} {
				if !strings.Contains(head, sub) {
					t.Errorf("%s (%s): the header lacks %q", sc, lang, sub)
				}
			}
			// 절차의 화면이면 머리에 지금 단계도 있다(좁은 화면에서 카드가 밀려도 보이게).
			if sc != ui.ScreenInventory && strings.Count(head, "data-here>") != 1 {
				t.Errorf("%s (%s): the header names the current step %d times, want once", sc, lang, strings.Count(head, "data-here>"))
			}
			for _, gone := range []string{"직접 설치", "self-hosted", `class="source"`, dir, "data-mode="} {
				if strings.Contains(head, gone) {
					t.Errorf("%s (%s): the header still carries %q", sc, lang, gone)
				}
			}
		}
	}

	// 저장하는 자리 가까이에는 대상 파일의 짧은 이름이 남는다(전체 경로는 설정에만 있다).
	for sc, file := range map[string]string{ui.ScreenDecl: s.decl, ui.ScreenReview: s.path} {
		body := get(t, s, sc+"?lang=ko").Body.String()
		if !strings.Contains(body, `data-save-target>저장하는 파일: <code>`+filepath.Base(file)+`</code>`) {
			t.Errorf("%s: the buttons do not name the file they save to (%s)", sc, filepath.Base(file))
		}
		if strings.Contains(body, file) {
			t.Errorf("%s: the full path %s is still on the screen", sc, file)
		}
	}

	// 선언이 아직 없어도 설정은 열린다.
	bare, _ := newServer(t)
	bare.decl = filepath.Join(t.TempDir(), "declaration.json")
	if w := get(t, bare, ui.ScreenSettings+"?lang=ko"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `data-setting="lang"`) {
		t.Errorf("settings without a declaration: %d", w.Code)
	}
}

// IC-U233 — **저장을 멈춘 상태에서 설정을 직접 열면 그 상태의 화면이 뜨고, 지금 상태와 다음 할 일을 적는다.**
//
// 복구 필요 · 진단 전용에서는 모든 화면이 그 상태 화면이다. 설정도 예외가 아니다: 주소를 직접 열어도 503으로 상태 화면이 오고
// 머리에 상태가 보인다. 설정 링크는 두지 않는다(눌러도 같은 화면이다).
func TestSettingsInStoppedStatesShowTheStateScreen(t *testing.T) {
	rec, _ := recoveringServer(t)
	diag, _ := newServer(t)
	diag.diag, diag.diagRef = []diagProblem{{role: "declaration", path: "/work/declaration.json", err: errors.New("unexpected end of JSON input")}}, "ref-1"
	for _, c := range []struct {
		name   string
		s      *server
		ko, en []string
	}{
		{"recovery", rec, []string{"복구가 필요합니다", "진단 파일 내려받기", "화면을 끄고 다시 띄웁니다"}, []string{"Recovery required", "Download the diagnostics file", "Stop the screen and start it again"}},
		{"diagnostic", diag, []string{"진단 전용으로 열렸습니다", "파일을 고치거나 백업에서 되돌린 뒤 화면을 다시 띄우십시오", "declaration.json"}, []string{"opened for diagnostics only", "Fix them or restore them from a backup, then start the screen again", "declaration.json"}},
	} {
		for lang, want := range map[string][]string{"ko": c.ko, "en": c.en} {
			w := get(t, c.s, ui.ScreenSettings+"?lang="+lang)
			body := w.Body.String()
			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("%s (%s): /settings answered %d, want the 503 state screen", c.name, lang, w.Code)
			}
			for _, sub := range append(want, `data-mode="`+c.name+`"`) {
				if !strings.Contains(body, sub) {
					t.Errorf("%s (%s): the state screen at /settings lacks %q", c.name, lang, sub)
				}
			}
			if strings.Contains(body, "data-settings-link") || strings.Contains(body, `data-setting="`) {
				t.Errorf("%s (%s): the settings rows or the settings link show in a stopped state", c.name, lang)
			}
		}
	}
}
