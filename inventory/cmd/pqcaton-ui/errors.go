// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 브라우저로 가는 오류.
//
// **모든 오류를 새 껍데기의 페이지로 그린다.** 평문 http.Error를 쓰지 않는다. 화면에는 종류마다 정해 둔
// 그 말의 문구(무엇이 안 됐나 · 다음에 할 일)와 역할 · 파일 이름만 보인다. 오류 원문과 전체 경로는 표준오류에
// 참조 번호와 함께 남기고, 화면에는 그 번호만 보인다. 처리기 밖의 404 · 405 · panic도 같다.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// newRef: 참조 번호. 화면과 표준오류를 잇는 것뿐이라 짧다.
func newRef() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}

// logRef: 오류 원문을 참조 번호와 함께 표준오류에 남기고 번호를 돌려준다.
func logRef(r *http.Request, err error) string {
	ref := newRef()
	fmt.Fprintf(os.Stderr, "error ref=%s %s %s: %v\n", ref, r.Method, r.URL.Path, err)
	return ref
}

// errorPage: 오류 페이지를 그린다. err가 있으면 참조 번호를 붙여 남긴다. 화면에 번호를 보이는 것은 까닭을
// 화면 말로 옮길 수 없는 종류(입력을 읽지 못함 · 내부 오류)뿐이다.
func errorPage(w http.ResponseWriter, r *http.Request, status int, kind string, err error, files ...ui.FileRef) {
	l := ui.PickLang(r)
	p := ui.Page{Lang: l, LangHref: ui.SwitchHref(r, l.Other())}
	v := ui.NewRefused(p, kind, "/", files)
	if err != nil {
		ref := logRef(r, err)
		if kind == ui.ErrInternal || kind == ui.ErrInputBroken {
			v.Ref = ref
		}
	}
	writePage(w, status, func(out io.Writer) error { return ui.RenderRefused(out, v) })
}

// writePage: 버퍼에 다 그린 뒤 보낸다. 그리다 실패하면 반쯤 그린 페이지 대신 내부 오류 페이지를 낸다.
func writePage(w http.ResponseWriter, status int, render func(io.Writer) error) {
	var buf bytes.Buffer
	if err := render(&buf); err != nil {
		fmt.Fprintf(os.Stderr, "error rendering a page: %v\n", err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "500\n")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// html: 화면 한 장을 그린다(200). 그리다 실패하면 내부 오류 페이지다.
func html(w http.ResponseWriter, r *http.Request, render func(io.Writer) error) {
	var buf bytes.Buffer
	if err := render(&buf); err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInternal, fmt.Errorf("rendering: %w", err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// notFound · methodNotAllowed: 라우터에 없는 주소와 방식.
func notFound(w http.ResponseWriter, r *http.Request) {
	errorPage(w, r, http.StatusNotFound, ui.ErrNotFound, nil)
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	errorPage(w, r, http.StatusMethodNotAllowed, ui.ErrMethod, nil)
}

// recoverPanic: 처리기가 터져도 서버는 계속 돌고, 그 요청에는 내부 오류 페이지를 낸다. 화면 하나가 터져도
// 나머지 절차는 다뤄야 한다(적던 판정이 통째로 날아가지 않게).
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(v)
			}
			errorPage(w, r, http.StatusInternalServerError, ui.ErrInternal, fmt.Errorf("panic: %v\n%s", v, debug.Stack()))
		}()
		next.ServeHTTP(w, r)
	})
}

// redirect: 화면으로 돌려보낸다. 알림은 코드로만 싣는다(ui.EncodeNotices).
func redirect(w http.ResponseWriter, r *http.Request, to string, msgs, problems []ui.Notice) {
	u := to
	sep := "?"
	if len(msgs) > 0 {
		u += sep + ui.MsgParam + "=" + url.QueryEscape(ui.EncodeNotices(msgs))
		sep = "&"
	}
	if len(problems) > 0 {
		u += sep + ui.ProblemParam + "=" + url.QueryEscape(ui.EncodeNotices(problems))
	}
	http.Redirect(w, r, u, http.StatusSeeOther)
}

// problemsFor: 저장 · 확정이 막힌 까닭을 알림으로. 관문 오류는 그 말로 옮기고, 나머지는 참조 번호를 붙인
// 내부 오류다(원문은 표준오류에만).
func problemsFor(r *http.Request, err error) []ui.Notice {
	if ps := ui.ProblemsOf(err); ps != nil {
		return ps
	}
	return []ui.Notice{ui.N(ui.ProblemInternal, logRef(r, err))}
}
