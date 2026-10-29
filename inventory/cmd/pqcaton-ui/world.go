// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 요청 단위 스냅샷.
//
// 새 화면은 위쪽 단계 카드 넷에 각 단계의 지금 상태를 적는다. 카드를 채우려면 선언 · 대조 결과 ·
// 리뷰 세션 · 스코프 세션을 모두 읽어야 하는데, 본문이 따로 읽고 카드가 또 읽으면 둘이 다른
// 시점의 파일을 볼 수 있고 대조(무겁다)를 몇 번씩 한다. 그래서 **요청 하나에서 입력을 한 번
// 읽어(world) 본문과 카드가 함께 쓴다.** 요청 사이에는 아무것도 남기지 않는다(캐시 없음).
//
// 시점은 snapshotScreen이 지킨다. 그 화면 양식의 입력(rev)과 카드가 읽는 입력 전부(지문)를
// 모델을 만들기 전과 뒤에 재어, 둘 다 같을 때만 화면을 낸다. 양식에는 rev만 싣는다 — 카드만
// 읽는 파일이 바뀌었다고 그 화면의 저장이 막히면 안 된다.

import (
	"fmt"
	"os"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// cardWait: 카드의 확정 상태를 볼 때 대상 잠금을 기다리는 시간. 화면을 그리는 요청이라 저장보다
// 짧게 기다리고, 넘으면 「확인할 수 없음」이다.
const cardWait = time.Second

// world: 요청 하나가 읽은 입력. 화면에 필요한 부분을 읽지 못하면 그 화면은 500이고, 카드만 쓰는
// 부분을 읽지 못하면 그 카드만 「확인할 수 없음」이다.
type world struct {
	decl    decl.Declaration
	declErr error

	res       *report.Result
	resErr    error
	seen      map[string][]ui.DeclAsset
	unmatched []string

	review     review.Session
	reviewWarn []review.Warning
	reviewID   string
	reviewErr  error

	scope      scope.Session
	scopeFiles []scope.LayerFile
	scopeErr   error
	// scopeMissing — 작업 폴더에 계층도 스코프 세션도 아직 없다.
	scopeMissing bool

	reviewFinal, scopeFinal ui.Final
}

// load: 입력을 한 번씩 읽는다. 카드에 적을 확정 상태까지 본다.
func (s *server) load() *world {
	wd := &world{}
	wd.decl, wd.declErr = decl.Load(s.decl)
	if s.results != "" {
		wd.resErr = wd.declErr
		if wd.resErr == nil {
			policy, err := s.assetPolicy()
			if wd.resErr = err; err == nil {
				wd.res, wd.resErr = report.BuildWith(s.results, wd.decl, policy)
			}
		}
		if wd.resErr == nil {
			wd.seen, wd.unmatched = observedFrom(wd.decl, wd.res)
		}
	}

	// 리뷰 세션: 관측 결과가 있으면 같은 대조 결과로 다시 세우고 사람이 적은 것을 옮긴다
	// (reviewSession과 같은 계산이다).
	prev, err := review.Load(s.path)
	wd.reviewID = prev.SessionID
	switch {
	case err != nil && (s.results == "" || !os.IsNotExist(err)):
		wd.reviewErr = err
	case s.results == "":
		wd.review = prev
	case wd.resErr != nil:
		wd.reviewErr = wd.resErr
	default:
		b, err := review.FromReport(wd.res, wd.decl, s.orgName())
		if err != nil {
			wd.reviewErr = err
			break
		}
		wd.review, wd.reviewWarn = review.Carry(prev, b.Session), b.Warnings
	}

	wd.scopeMissing = s.scopeMissing()
	if s.scope != "" && !wd.scopeMissing {
		wd.scope, wd.scopeFiles, wd.scopeErr = s.scopeSession()
	}
	wd.reviewFinal = s.final(kindReview, wd.reviewID, s.reviewTargets(), wd.reviewErr)
	if wd.scopeMissing {
		wd.scopeFinal = ui.FinalNone
	} else if s.scope != "" {
		wd.scopeFinal = s.final(kindScope, wd.scope.SessionID, s.scopeTargets(), wd.scopeErr)
	}
	return wd
}

// final: 카드에 적을 확정 상태. 판정하지 못하면 FinalUnknown이고 원인은 표준오류에 남긴다.
func (s *server) final(kind, session string, targets []string, loadErr error) ui.Final {
	if loadErr != nil || s.st == nil {
		return ui.FinalUnknown
	}
	id := store.Identity{Kind: kind, Org: s.orgName(), SessionID: sessionName(session)}
	v, err := s.st.VerifyFinalized(id, targets, min(cardWait, s.waitTime()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "step card %s: the finalize state is unavailable: %v\n", kind, err)
		return ui.FinalUnknown
	}
	switch v {
	case store.FinalNone:
		return ui.FinalNone
	case store.FinalDone:
		return ui.FinalDone
	case store.FinalSuperseded:
		return ui.FinalSuperseded
	case store.FinalRecovery:
		return ui.FinalRecovery
	}
	return ui.FinalUnknown
}

// steps: 카드에 채울 상태. 읽지 못한 단계는 비워 두고(「확인할 수 없음」) 원인을 표준오류에 남긴다.
func (s *server) steps(wd *world) ui.StepState {
	var st ui.StepState
	unavailable := func(step string, err error) {
		fmt.Fprintf(os.Stderr, "step card %s: unavailable: %v\n", step, err)
	}
	if s.decl != "" {
		if s.workspace != "" && !s.declReady() {
			st.DeclMissing = true
		} else if wd.declErr != nil {
			unavailable("declaration", wd.declErr)
		} else {
			got := ui.NewDeclView(wd.decl, ui.Page{}).WithObserved(wd.seen, wd.unmatched).Summary()
			st.Decl = &got
		}
	}
	if wd.scopeMissing {
		st.LayersMissing = true
	} else if s.scope != "" {
		if wd.scopeErr != nil {
			unavailable("scope", wd.scopeErr)
		} else {
			v := ui.NewScopeView(wd.scope, ui.Page{})
			if len(wd.scopeFiles) > 0 {
				v = v.Editable(wd.scopeFiles)
			}
			got := v.Summary()
			st.Scope = &got
		}
	}
	if s.hosts != "" {
		if hosts, err := s.loadHosts(); err != nil {
			unavailable("observe", err)
		} else {
			got := ui.ObserveSummary{Hosts: len(hosts)}
			st.Observe = &got
		}
	}
	// 작업 폴더에 선언이 아직 없으면 대조 · 판정 카드는 닫혀 있다(screens). 읽지 못한 것으로 적지 않는다.
	if s.workspace != "" && !s.declReady() {
		st.ReviewFinal, st.ScopeFinal = wd.reviewFinal, wd.scopeFinal
		return st
	}
	if s.results != "" {
		if wd.resErr != nil {
			unavailable("survey", wd.resErr)
		} else {
			got := ui.NewSurveyView(wd.res, ui.Page{}).Summary()
			st.Survey = &got
		}
	}
	if wd.reviewErr != nil {
		unavailable("review", wd.reviewErr)
	} else {
		got := ui.NewReviewView(wd.review, ui.Page{}).Summary()
		st.Review = &got
	}
	st.ReviewFinal, st.ScopeFinal = wd.reviewFinal, wd.scopeFinal
	return st
}

// cardInputs: 카드가 읽는 입력 전부. 확정 상태가 견주는 출력(계획 · 정책 · 원장)도 넣는다 — 그 사이
// 확정이 끝나면 본문과 카드가 다른 시점을 보인다.
func (s *server) cardInputs() []store.Input {
	var in []store.Input
	add := func(name, p string, dir bool) {
		if p != "" {
			in = append(in, store.Input{Name: "card:" + name, Path: p, Dir: dir})
		}
	}
	add("declaration", s.decl, false)
	add("results", s.results, true)
	add("scope-assets", s.scopeAssets, false)
	add("session", s.path, false)
	add("scope-session", s.scope, false)
	for i, l := range s.layerFiles() {
		add(fmt.Sprintf("layer:%d", i), l, false)
	}
	add("base", s.base, false)
	add("plan", s.planOut, false)
	add("policy", s.scopeOut, false)
	add("ledger", s.judgments, false)
	// 확정 상태는 상태 디렉토리의 영수증과 기록이 정한다. 출력이 같은 채로 영수증만 바뀌어도 다시 잰다.
	if s.st != nil {
		ids := []store.Identity{s.reviewID()}
		if s.scope != "" {
			ids = append(ids, s.scopeID())
		}
		in = append(in, s.st.StateInputs(ids...)...)
	}
	return in
}

// reviewTargets · scopeTargets: 확정이 쓰는 파일들. 확정 처리기와 카드의 확정 상태 확인이 같은 목록을 쓴다.
func (s *server) reviewTargets() []string {
	t := []string{s.path, s.planOut}
	if s.judgments != "" {
		t = append(t, s.judgments)
	}
	return t
}

func (s *server) scopeTargets() []string {
	t := []string{s.scope, s.scopeOut}
	if s.judgments != "" {
		t = append(t, s.judgments)
	}
	return t
}

// snapshotScreen: 화면 모델을 만들고, 그 모델과 같은 시점의 양식 rev를 돌려준다. 카드가 읽는 입력의
// 지문도 함께 재어 둘 다 같을 때만 짝짓는다. 세 번 해도 맞지 않으면 빈 rev다(snapshot과 같다).
func (s *server) snapshotScreen(here string, build func() error) (string, error) {
	form := s.inputsFor(here)
	card := s.cardInputs()
	if form == nil && card == nil {
		return "", build()
	}
	measure := func() (string, string, error) {
		var f, c string
		var err error
		if form != nil {
			if f, err = store.Rev(form); err != nil {
				return "", "", err
			}
		}
		if card != nil {
			if c, err = store.Rev(card); err != nil {
				return "", "", err
			}
		}
		return f, c, nil
	}
	for i := 0; i < 3; i++ {
		f0, c0, err := measure()
		if err != nil {
			return "", err
		}
		if err := build(); err != nil {
			return "", err
		}
		if s.afterModel != nil {
			s.afterModel()
		}
		f1, c1, err := measure()
		if err != nil {
			return "", err
		}
		if f0 == f1 && c0 == c1 {
			if s.afterSnapshot != nil {
				s.afterSnapshot()
			}
			return f1, nil
		}
	}
	fmt.Fprintf(os.Stderr, "rev for %s: the files kept changing while the screen was drawn; its saves will be refused until it is reloaded\n", here)
	return "", nil
}
