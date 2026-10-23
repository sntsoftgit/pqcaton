// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 확정 전 확인.
//
// 확정 버튼은 먼저 확인 요청(/finalize/confirm)을 보낸다. 확인 요청은 **아무것도 바꾸지 않는다.**
// 폼 값을 지금 파일에 얹어 무엇이 확정되는지 계산하고, 그 결과의 지문(payload)과 rev, 세션 ID,
// 만료 시각을 서버만 아는 키로 서명해 확인 페이지에 싣는다. 확정 요청은 그 서명을 들고 온다.
//
//   - 서명이 없거나 맞지 않으면 403. 키는 서버가 뜰 때 만들고 디스크에 두지 않으므로, 서버를 다시
//     띄우면 그 전의 확인 페이지는 모두 무효다.
//   - 서명은 맞는데 시간이 지났으면 409. 다시 미리 보라고 안내한다.
//   - 서명이 맞고 유효하면 FinalizeChecked가 잠금 안에서 rev와 payload를 다시 계산해 최종 비교한다.
//     확인과 확정 사이에 파일이 바뀌었으면 409(오래된 화면)다.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// confirmTTL: 확인 페이지가 유효한 시간.
const confirmTTL = 10 * time.Minute

// confirmKey: 확인 토큰의 서명 키. 처음 쓸 때 만든다. 메모리에만 있다.
func (s *server) confirmKey() []byte {
	s.keyOnce.Do(func() {
		s.key = make([]byte, 32)
		if _, err := rand.Read(s.key); err != nil {
			// 난수를 못 얻으면 서명할 수 없다. 약한 키로 대신하지 않는다.
			panic(err)
		}
	})
	return s.key
}

func (s *server) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

// confirmToken: 확정 종류 · 세션 ID · rev · payload 지문 · 만료 시각의 서명.
func (s *server) confirmToken(kind, session, rev, payload, exp string) string {
	m := hmac.New(sha256.New, s.confirmKey())
	fmt.Fprintf(m, "pqcaton confirm/1\n%s\n%s\n%s\n%s\n%s\n", kind, session, rev, payload, exp)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// confirmed: 확정 요청이 들고 온 확인 토큰을 본다. 문제가 있으면 거절을 그리고 false.
func (s *server) confirmed(w http.ResponseWriter, r *http.Request, kind, back string, id store.Identity) (payload string, ok bool) {
	f := r.PostForm
	tok, exp := f.Get(ui.ConfirmTokenField), f.Get(ui.ConfirmExpField)
	payload, session := f.Get(ui.ConfirmPayloadField), f.Get(ui.ConfirmSessionField)
	want := s.confirmToken(kind, session, f.Get(ui.RevField), payload, exp)
	if tok == "" || !hmac.Equal([]byte(tok), []byte(want)) {
		fmt.Fprintf(os.Stderr, "refused %s %s (403): the confirmation token is missing or does not match\n", r.Method, r.URL.Path)
		s.refuse(w, r, http.StatusForbidden, ui.RefusedConfirm, back, nil)
		return "", false
	}
	sec, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || s.now().Unix() > sec {
		s.refuse(w, r, http.StatusConflict, ui.RefusedExpired, back, nil)
		return "", false
	}
	// 확인한 뒤 세션이 다른 세션으로 바뀌었다(파일을 갈아 끼웠다). 확인한 것이 아니다.
	if session != id.SessionID {
		s.refuse(w, r, http.StatusConflict, ui.RefusedStale, back, nil)
		return "", false
	}
	return payload, true
}

// confirmPage: 확인 페이지를 그린다. cur는 미리 보기를 계산한 것과 같은 시점의 rev다(snapshot). 폼의
// rev와 다르면 미리 보지 않고 409다.
func (s *server) confirmPage(w http.ResponseWriter, r *http.Request, kind, action, saveAction, back string,
	id store.Identity, cur, payload string, lines []string, files []ui.FileRef) {
	rev := r.PostForm.Get(ui.RevField)
	if cur == "" || cur != rev {
		s.refuse(w, r, http.StatusConflict, ui.RefusedStale, back, files)
		return
	}
	exp := strconv.FormatInt(s.now().Add(confirmTTL).Unix(), 10)
	p := s.page(r, "", "")
	p.Title = ui.ConfirmTitle(p.Lang)
	p.Rev = rev
	v := ui.ConfirmView{Page: p, Action: action, SaveAction: saveAction, Back: back, Lines: lines, Files: files,
		Hidden: ui.CarryForm(r.PostForm), Exp: exp, Payload: payload, SessionID: id.SessionID,
		Token: s.confirmToken(kind, id.SessionID, rev, payload, exp)}
	html(w, func() error { return ui.RenderConfirm(w, v) })
}

// finalizeConfirm — 리뷰 확정 전 확인. 아무것도 쓰지 않는다.
func (s *server) finalizeConfirm(w http.ResponseWriter, r *http.Request) {
	if !s.writable(w, r, "/review") {
		return
	}
	l := ui.PickLang(r)
	withJudg := s.judgments != ""
	var (
		o       review.Output
		payload string
		id      store.Identity
	)
	// 미리 보기와 그 지문을 rev와 같은 시점에 만든다.
	cur, err := s.snapshot("/finalize/confirm", func() error {
		sf, err := s.applyReview(r)
		if err != nil {
			return err
		}
		if o, err = review.Outputs(sf, s.org, withJudg, 0); err != nil {
			return err
		}
		payload, err = review.PayloadSHA256(sf, s.org, withJudg)
		id = s.reviewID()
		return err
	})
	if err != nil {
		// 확정되지 않을 이유가 있으면 미리 보지 않고 화면에 그 이유를 보인다.
		redirect(w, r, "/review", "", ui.Refusal(l, err))
		return
	}
	lines := []string{ui.LinePlanActions(l, len(o.Result.Plan.GetActions()))}
	files := []ui.FileRef{file(l, "session", s.path), file(l, "plan", s.planOut)}
	if withJudg {
		lines = append(lines, ui.LineJudgments(l, o.Judged))
		files = append(files, file(l, "ledger", s.judgments))
	}
	s.confirmPage(w, r, kindReview, "/finalize", "/save", "/review", id, cur, payload, lines, files)
}

// scopeConfirm — 스코프 확정 전 확인. 아무것도 쓰지 않는다.
func (s *server) scopeConfirm(w http.ResponseWriter, r *http.Request) {
	if s.scope == "" {
		http.Error(w, "no scope session was given", http.StatusNotFound)
		return
	}
	if !s.writable(w, r, "/scope") {
		return
	}
	l := ui.PickLang(r)
	withJudg := s.judgments != ""
	var (
		o       scope.Output
		payload string
		id      store.Identity
	)
	cur, err := s.snapshot("/scope/finalize/confirm", func() error {
		sf, err := s.applyScope(r)
		if err != nil {
			return err
		}
		if o, err = scope.Outputs(sf, s.org, withJudg, 0); err != nil {
			return err
		}
		payload, err = scope.PayloadSHA256(sf, s.org, withJudg)
		id = s.scopeID()
		return err
	})
	if err != nil {
		redirect(w, r, "/scope", "", ui.Refusal(l, err))
		return
	}
	lines := []string{ui.LinePolicyRules(l, len(o.Result.Policy.Rules))}
	files := []ui.FileRef{file(l, "scope-session", s.scope), file(l, "policy", s.scopeOut)}
	if withJudg {
		lines = append(lines, ui.LineJudgments(l, o.Judged))
		files = append(files, file(l, "ledger", s.judgments))
	}
	s.confirmPage(w, r, kindScope, "/scope/finalize", "/scope/save", "/scope", id, cur, payload, lines, files)
}
