// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 02 관측 화면. 작업 폴더 모드에서만 열린다 — 접근 정보(`hosts.csv`)의 자리가 그 폴더 안에 정해져 있다.
//
// **접근 정보는 비밀을 담을 수 있다**(`ssh_pass`). 그래서 화면은 비밀번호를 보이지도 받지도 않고, 파일은
// 저장 경로가 소유자 전용(0600)으로 쓴다.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

const kindHosts = "hosts"

// loadHosts: 지금의 접근 정보. 파일이 없으면 빈 목록이다(아직 적지 않았다).
func (s *server) loadHosts() ([]inventory.Host, error) {
	b, err := os.ReadFile(s.hosts)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// 빈 파일은 아직 적지 않은 것이다. 상류는 머리글 없는 파일을 거절한다.
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, nil
	}
	return inventory.ParseHosts(bytes.NewReader(b))
}

// hostsInputs: 관측 화면이 그려질 때 읽는 것.
func (s *server) hostsInputs() []store.Input {
	return []store.Input{{Name: "hosts", Path: s.hosts}}
}

func (s *server) observe(w http.ResponseWriter, r *http.Request) {
	if s.hosts == "" {
		notFound(w, r)
		return
	}
	var hosts []inventory.Host
	rev, err := s.snapshotScreen(ui.ScreenObserve, func() error {
		var err error
		hosts, err = s.loadHosts()
		return err
	})
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	wd := s.load()
	st := s.steps(wd)
	page := s.nextPage(r, ui.ScreenObserve, s.hosts, st)
	page.Rev = rev
	v := ui.NewObserveView(hosts, page)
	v.Scope = s.observeScope(page.Lang, st)
	v.Run = s.runView()
	if wd.res != nil {
		v.Run.JCAUnconfirmed = wd.res.JCAUnconfirmed
	}
	if s.results != "" {
		v.HasResults, v.Ingest = true, s.ingestView(r.URL.Query().Get(ui.IngestRunParam))
		v.Policy = s.policyView()
	}
	html(w, r, func(out io.Writer) error { return ui.RenderObserve(out, v) })
}

// observeScope: 관측 화면의 「자산 스코프」 구역. 현재 활성 정책의 파일 · 규칙 수 · 지문과, 규칙 편집의 상태 · 확정 상태다.
// 활성 정책 파일이 없으면 규칙 0개의 정책이다. 읽지 못하면 그렇다고 적는다(없는 것으로 치지 않는다).
func (s *server) observeScope(l ui.Lang, st ui.StepState) *ui.ObserveScope {
	o := &ui.ObserveScope{ActiveFile: filepath.Base(s.scopeAssets)}
	digest, err := scope.PolicyFileDigest(s.scopeAssets)
	if err == nil && s.scopeAssets != "" {
		if _, serr := os.Stat(s.scopeAssets); serr == nil {
			p, perr := review.LoadAssetPolicy(s.scopeAssets)
			if err = perr; err == nil && p != nil {
				o.Rules = len(p.Rules)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "observe: the active asset scope policy:", err)
		o.PolicyUnknown = true
	} else {
		o.ActiveDigest = digest
	}
	if sc := s.screens(); sc.Scope {
		o.Href = ui.ScreenScope
		o.State, o.Dot, o.Final = ui.ScopeState(l, sc, st)
	}
	return o
}

// runView: 관측 실행 구역. 저장하지 않는 서버는 점검을 하지 않는다(시작할 수 없다).
func (s *server) runView() ui.ObserveRun {
	v := ui.ObserveRun{Writable: s.st != nil && !s.recovering && len(s.diag) == 0, Window: defaultWindow, Status: s.run.status()}
	if v.Writable {
		v.Checks = s.preflight()
	}
	if m := s.currentRun(); m != nil {
		v.Current = &ui.RunCurrent{ID: m.ID, Ended: m.Ended.Local(), Nodes: len(m.Nodes), Files: m.Received}
	}
	if v.Status != nil && v.Status.Window > 0 {
		v.Window = v.Status.Window
	}
	return v
}

// runStatusFragment: 실행 상태 조각(GET). 화면이 돌고 있는 동안 몇 초마다 받는다.
func (s *server) runStatusFragment(w http.ResponseWriter, r *http.Request) {
	if s.hosts == "" {
		notFound(w, r)
		return
	}
	st := s.run.status()
	// 도는 동안에는 요약과 로그 조각만 따로 다시 받는다(중지 버튼이 갈리지 않게). 요청이 실어 온 상태와 지금 상태가 같을
	// 때만 그 조각을 준다. 달라졌으면 요약 조각의 응답이 상자 전체를 갈아 끼우거나(마무리 중이 됐다) 화면을 새로 받게
	// 하고(끝났다), 로그 조각은 아무것도 바꾸지 않는다(204).
	if part := r.URL.Query().Get(ui.RunPartParam); part == ui.RunPartLive || part == ui.RunPartLog {
		same := st != nil && !st.Over() && string(st.State) == r.URL.Query().Get(ui.RunStateParam)
		switch {
		case same:
			html(w, r, func(out io.Writer) error { return ui.RenderRunPart(out, ui.PickLang(r), part, st) })
			return
		case part == ui.RunPartLog:
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// 실행이 끝났으면 화면을 통째로 새로 받게 한다. 이 상자 밖에도 달라지는 것이 있다: 02 카드의 「관측 중」이 걷히고
		// 「관측 시작」 양식이 다시 나온다. 조각만 갈면 새로 고칠 때까지 카드는 「관측 중」으로 남는다.
		if st == nil || st.Over() {
			w.Header().Set("HX-Refresh", "true")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("HX-Retarget", "#run-status")
		w.Header().Set("HX-Reswap", "outerHTML")
	}
	html(w, r, func(out io.Writer) error {
		return ui.RenderRunStatus(out, ui.PickLang(r), csrfToken(r), st)
	})
}

// runStart: 관측을 시작한다(POST). 막는 점검이 남았으면 409로 거절하고, 이미 돌고 있어도 409다.
func (s *server) runStart(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenObserve
	if s.hosts == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	window := defaultWindow
	if v := strings.TrimSpace(r.PostForm.Get("window")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < minWindow || n > maxWindow {
			errorPage(w, r, http.StatusBadRequest, ui.ErrBadRequest, nil)
			return
		}
		window = n
	}
	if !(ui.ObserveRun{Writable: true, Checks: s.preflight()}).CanStart() {
		s.refuse(w, r, http.StatusConflict, ui.RefusedRunNotReady, back, nil)
		return
	}
	if err := s.startRun(window); err != nil {
		if errors.Is(err, errRunBusy) {
			s.refuse(w, r, http.StatusConflict, ui.RefusedRunBusy, back, nil)
			return
		}
		if errors.Is(err, errRunStuck) {
			s.refuse(w, r, http.StatusConflict, ui.RefusedRunNotReady, back, nil)
			return
		}
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInternal, err)
		return
	}
	http.Redirect(w, r, back+"?"+ui.LangParam+"="+string(ui.PickLang(r))+"#observe-run", http.StatusSeeOther)
}

// runStop: 돌고 있는 관측을 멈춘다(POST). 돌고 있지 않으면 아무것도 하지 않고 화면으로 돌아간다.
func (s *server) runStop(w http.ResponseWriter, r *http.Request) {
	if s.hosts == "" {
		notFound(w, r)
		return
	}
	s.stopRun()
	http.Redirect(w, r, ui.ScreenObserve+"?"+ui.LangParam+"="+string(ui.PickLang(r))+"#observe-run", http.StatusSeeOther)
}

// hostsSave: 접근 정보를 저장한다. 막는 자리가 있으면 쓰지 않고, **받은 값 그대로** 문제와 함께 다시 그린다
// (422). 리다이렉트하면 적던 것이 날아간다.
func (s *server) hostsSave(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenObserve
	if s.hosts == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	var (
		saved    []inventory.Host
		problems []ui.HostProblem
		prev     []inventory.Host
	)
	errProblems := errors.New("hosts: problems")
	id := store.Identity{Kind: kindHosts, Org: s.orgName(), SessionID: "hosts"}
	err := s.st.SaveChecked(id, []string{s.hosts}, s.check(r), s.waitTime(), func() ([]store.Change, error) {
		var err error
		if prev, err = s.loadHosts(); err != nil {
			return nil, err
		}
		if saved, problems = ui.ApplyHosts(prev, r.PostForm); len(problems) > 0 {
			return nil, errProblems
		}
		b, err := ui.EncodeHosts(saved)
		return []store.Change{{Path: s.hosts, Content: b}}, err
	})
	if errors.Is(err, errProblems) {
		page := s.nextPage(r, back, s.hosts, s.steps(s.load()))
		page.Rev = r.PostForm.Get(ui.RevField)
		v := ui.ObserveView{Page: page, Hosts: ui.HostRowsFrom(prev, r.PostForm), Problems: problems, Run: s.runView()}
		writePage(w, http.StatusUnprocessableEntity, func(out io.Writer) error { return ui.RenderObserve(out, v) })
		return
	}
	if err != nil {
		s.fail(w, r, back, err, file(ui.PickLang(r), "hosts", s.hosts))
		return
	}
	redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeHostsSaved, len(saved))}, nil)
}
