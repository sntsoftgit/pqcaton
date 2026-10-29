// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 03 관측 화면. 작업 폴더 모드에서만 열린다 — 접근 정보(`hosts.csv`)의 자리가 그 폴더 안에 정해져 있다.
//
// **접근 정보는 비밀을 담을 수 있다**(`ssh_pass`). 그래서 화면은 비밀번호를 보이지도 받지도 않고, 파일은
// 저장 경로가 소유자 전용(0600)으로 쓴다.

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

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
	page := s.nextPage(r, ui.ScreenObserve, s.hosts, s.steps(s.load()))
	page.Rev = rev
	v := ui.NewObserveView(hosts, page)
	html(w, r, func(out io.Writer) error { return ui.RenderObserve(out, v) })
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
		v := ui.ObserveView{Page: page, Hosts: ui.HostRowsFrom(prev, r.PostForm), Problems: problems}
		writePage(w, http.StatusUnprocessableEntity, func(out io.Writer) error { return ui.RenderObserve(out, v) })
		return
	}
	if err != nil {
		s.fail(w, r, back, err, file(ui.PickLang(r), "hosts", s.hosts))
		return
	}
	redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeHostsSaved, len(saved))}, nil)
}
