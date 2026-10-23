// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	htmlpkg "html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

var hiddenInput = regexp.MustCompile(`<input type="hidden" name="([^"]*)" value="([^"]*)">`)

// hiddenFields: 확인 페이지의 양식이 보낼 값.
func hiddenFields(body string) url.Values {
	out := url.Values{}
	for _, m := range hiddenInput.FindAllStringSubmatch(body, -1) {
		out.Add(htmlpkg.UnescapeString(m[1]), htmlpkg.UnescapeString(m[2]))
	}
	return out
}

// finalizeVia: 화면이 하는 그대로 확정한다. 확인 요청을 보내고, 확인 페이지의 양식을 확정 주소로 보낸다.
// 확인이 페이지를 내지 않으면(거절 · 관문) 그 응답을 돌려준다.
func finalizeVia(t *testing.T, s *server, lang, confirmPath string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	w := postFormLang(t, s, confirmPath, lang, form)
	if w.Code != http.StatusOK {
		return w
	}
	return postFormLang(t, s, strings.TrimSuffix(confirmPath, "/confirm"), lang, hiddenFields(w.Body.String()))
}

// snapshotDir: 디렉토리 아래 파일 전부(상태 디렉토리 포함)의 내용.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		// 잠금 파일은 쓰려고 할 때 생긴다. 내용이 바뀐 것이 아니다.
		if err == nil && !d.IsDir() && d.Name() != "run.lock" && d.Name() != store.TargetLockName {
			b, _ := os.ReadFile(p)
			out[p] = string(b)
		}
		return nil
	})
	return out
}

func sameDir(t *testing.T, before, after map[string]string) {
	t.Helper()
	for p, v := range after {
		if before[p] != v {
			t.Errorf("%s changed or appeared", filepath.Base(p))
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			t.Errorf("%s disappeared", filepath.Base(p))
		}
	}
}

// IC-U64 — **확인 요청은 아무것도 바꾸지 않는다.** 확인 페이지는 무엇이 확정되는지(조치 수 · 판정 수 ·
// 역할과 파일 이름)를 보이고, 받은 폼 값(저장하지 않은 편집)과 서명된 확인 토큰을 숨은 필드로 싣는다.
func TestConfirmChangesNothing(t *testing.T) {
	s, dir := newServer(t)
	s.judgments = filepath.Join(dir, "judgments.jsonl")
	before := snapshotDir(t, dir)
	w := postFormLang(t, s, "/finalize/confirm", "ko", signed())
	if w.Code != http.StatusOK {
		t.Fatalf("status %d\n%s", w.Code, w.Body.String())
	}
	sameDir(t, before, snapshotDir(t, dir))
	body := w.Body.String()
	for _, want := range []string{"계획에 넣는 조치 1건", "판정 원장에 남길 기록", "plan.json", "judgments.jsonl"} {
		if !strings.Contains(body, want) {
			t.Errorf("the confirmation page does not show %q", want)
		}
	}
	if strings.Contains(body, dir) {
		t.Error("the confirmation page shows a full path")
	}
	f := hiddenFields(body)
	if f.Get("signature") != "sig" || f.Get("policy:openssl/libssl") != "PQC 라이브러리로 교체한다" {
		t.Errorf("the unsaved form values are not carried: %v", f)
	}
	for _, name := range []string{ui.ConfirmTokenField, ui.ConfirmExpField, ui.ConfirmPayloadField, ui.ConfirmSessionField, ui.RevField, ui.CSRFField} {
		if f.Get(name) == "" {
			t.Errorf("the confirmation page carries no %s", name)
		}
	}
}

// IC-U65 — **확인 토큰이 없거나 맞지 않으면 403이다.** 토큰 · 지문 · rev · 세션 · 만료 시각 가운데
// 하나라도 바꾸면 서명이 맞지 않는다. 아무것도 쓰지 않는다.
func TestConfirmTokenIsChecked(t *testing.T) {
	s, dir := newServer(t)
	page := postForm(t, s, "/finalize/confirm", signed())
	good := hiddenFields(page.Body.String())
	flip := func(v string) string {
		if v == "" {
			return "x"
		}
		b := []byte(v)
		if b[0] == 'a' {
			b[0] = 'b'
		} else {
			b[0] = 'a'
		}
		return string(b)
	}
	before := snapshotDir(t, dir)
	for _, field := range []string{"", ui.ConfirmTokenField, ui.ConfirmPayloadField, ui.RevField, ui.ConfirmSessionField, ui.ConfirmExpField} {
		f := url.Values{}
		for k, v := range good {
			f[k] = append([]string(nil), v...)
		}
		if field == "" {
			f.Del(ui.ConfirmTokenField)
		} else {
			f.Set(field, flip(f.Get(field)))
		}
		name := field
		if name == "" {
			name = "no token"
		}
		if w := postForm(t, s, "/finalize", f); w.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, w.Code)
		}
	}
	sameDir(t, before, snapshotDir(t, dir))
	if w := postForm(t, s, "/finalize", good); w.Code != http.StatusSeeOther {
		t.Fatalf("the untouched confirmation: status %d\n%s", w.Code, w.Body.String())
	}
}

// IC-U66 — **확인 페이지는 10분 뒤 만료되고(409), 서버를 다시 띄우면 무효다(403).** 만료는 다시 미리
// 보라고 안내한다.
func TestConfirmExpiresAndDiesWithTheServer(t *testing.T) {
	s, dir := newServer(t)
	now := time.Now()
	s.clock = func() time.Time { return now }
	good := hiddenFields(postForm(t, s, "/finalize/confirm", signed()).Body.String())
	before := snapshotDir(t, dir)

	s.clock = func() time.Time { return now.Add(confirmTTL + time.Second) }
	w := postFormLang(t, s, "/finalize", "ko", good)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "만료") {
		t.Fatalf("expired: status %d\n%s", w.Code, w.Body.String())
	}

	s.clock = func() time.Time { return now }
	s.keyOnce, s.key = sync.Once{}, nil // 서버를 다시 띄운 것과 같다
	if w := postForm(t, s, "/finalize", good); w.Code != http.StatusForbidden {
		t.Fatalf("after a restart: status %d, want 403", w.Code)
	}
	sameDir(t, before, snapshotDir(t, dir))
}

// IC-U67 — **확인한 뒤 파일이 바뀌면 확정하지 않는다(409).** 토큰은 맞아도 저장소가 잠금 안에서 rev와
// 지문을 다시 계산해 견준다.
func TestConfirmThenFilesChange(t *testing.T) {
	s, dir := newServer(t)
	good := hiddenFields(postForm(t, s, "/finalize/confirm", signed()).Body.String())
	changed := readFile(t, s.path) + " "
	if err := os.WriteFile(s.path, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotDir(t, dir)
	if w := postForm(t, s, "/finalize", good); w.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", w.Code)
	}
	sameDir(t, before, snapshotDir(t, dir))
}

// IC-U68 — **확인 페이지에서 「저장만」을 고르면 확정하지 않고 저장한다.** 실어 온 편집이 세션에 남고
// 계획은 생기지 않는다.
func TestConfirmSaveOnly(t *testing.T) {
	s, _ := newServer(t)
	f := hiddenFields(postForm(t, s, "/finalize/confirm", signed()).Body.String())
	q := location(t, postForm(t, s, "/save", f))
	if q.Get("problem") != "" {
		t.Fatalf("save: %s", q.Get("problem"))
	}
	if !strings.Contains(readFile(t, s.path), "PQC 라이브러리로 교체한다") {
		t.Error("the carried edit was not saved")
	}
	if _, err := os.Stat(s.planOut); !os.IsNotExist(err) {
		t.Error("saving from the confirmation page finalized")
	}
	if rc, _ := s.st.Finalized(s.reviewID()); rc != nil {
		t.Error("a receipt was written by a save")
	}
}

// IC-U69 — **오래된 화면은 미리 보지도 않는다(409).** 화면을 연 뒤에 파일이 바뀌었으면 확인 페이지를
// 그리지 않는다. 스코프도 같다.
func TestConfirmRefusesStaleScreen(t *testing.T) {
	for _, c := range []struct {
		name, confirm string
		setup         func(t *testing.T) (*server, string)
		form          url.Values
	}{
		{"review", "/finalize/confirm", newServer, signed()},
		{"scope", "/scope/finalize/confirm", withScope,
			url.Values{"reviewer": {"보안팀"}, "signature": {"sig"}, "layer:corp": {"OS 패치로 관리한다"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _ := c.setup(t)
			old := s.revFor(strings.TrimSuffix(c.confirm, "/confirm"))
			target := s.path
			if c.name == "scope" {
				target = s.scope
			}
			if err := os.WriteFile(target, []byte(readFile(t, target)+" "), 0o600); err != nil {
				t.Fatal(err)
			}
			w := postForm(t, s, c.confirm, withRev(c.form, old))
			if w.Code != http.StatusConflict {
				t.Fatalf("status %d, want 409", w.Code)
			}
			if strings.Contains(w.Body.String(), ui.ConfirmTokenField) {
				t.Error("a stale screen got a confirmation token")
			}
		})
	}
}
