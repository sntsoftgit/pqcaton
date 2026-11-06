// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 05의 새 정책 활성화. 02에서 확정한 정책(asset-scope.csv)이 활성 정책(scope-assets.csv)과 의미상 다르면 05 위쪽에 알리고,
// 운영자 진술(상류 재적재를 마쳤다)을 받아 확인 페이지를 거쳐 활성화한다. 활성화는 정책 보관본 · 활성 정책 교체 · 옛 세션
// 보관본 · 새 세션 · 변환 보고서 · 회차 표지 · scope-activation 형식 2 영수증을 한 트랜잭션으로 쓴다(rulesetround.Open).
//
// **진술은 상류 적재를 증명하지 않는다.** 화면과 확인 페이지가 그 한계를 적는다. 실제 상류 지문 확인은 명령과 데모의 몫이다.
//
// 확인 페이지부터 실행까지 활성 정책 · 후보 정책 · 옛 세션 · 결과 폴더가 그대로여야 한다. 확인 페이지의 지문(payload)에 넷을
// 담고, 실행은 다시 재어 견주며, 저장소는 rev(같은 넷과 선언)를 잠금 안에서 다시 잰다.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/rulesetround"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

const kindActivate = "activate"

var (
	// errScopeSame: 활성 정책과 후보 정책이 의미상 같다(활성화할 것이 없다).
	errScopeSame = errors.New("the policy finalized in 02 is already active")
	// errScopeCandidate: 후보 정책이 없거나, 02 확정이 쓴 그대로가 아니거나, 읽을 수 없다.
	errScopeCandidate = errors.New("the policy finalized in 02 is missing, changed after finalizing, or unreadable")
)

// activationCheck: 지금 활성화할 수 있는가. 되면 두 지문이고, 아니면 errScopeSame · errScopeCandidate · 세션 잠금(ScopeChanged)이다.
// 활성 정책 파일이 없으면 규칙 0개 정책이다. 후보는 있어야 하고, 읽혀야 하며, 02 확정 영수증이 적은 내용 그대로여야 한다.
func (s *server) activationCheck() (*ui.ActivationOffer, error) {
	if s.scopeOut == "" || s.scopeAssets == "" || s.st == nil || s.scope == "" {
		return nil, errScopeCandidate
	}
	if _, err := os.Stat(s.scopeOut); err != nil {
		return nil, errScopeCandidate
	}
	to, err := scope.PolicyFileDigest(s.scopeOut)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errScopeCandidate, err)
	}
	from, err := scope.PolicyFileDigest(s.scopeAssets)
	if err != nil {
		return nil, err
	}
	if from == to {
		return nil, errScopeSame
	}
	if _, err := s.st.OutputReceipt(s.scopeID(), s.scopeOut); err != nil {
		return nil, fmt.Errorf("%w: %v", errScopeCandidate, err)
	}
	sf, err := review.Load(s.path)
	if err != nil {
		return nil, err
	}
	if err := s.sameScopePolicy(sf); err != nil {
		return nil, err
	}
	return &ui.ActivationOffer{From: from, To: to}, nil
}

// activationOffer: 05 위쪽에 띄울 활성화. 할 수 없으면 nil이다(까닭은 활성화를 누를 때 거절문으로 알린다).
func (s *server) activationOffer() (*ui.ActivationOffer, error) {
	o, err := s.activationCheck()
	if err != nil {
		return nil, nil
	}
	return o, nil
}

// activationPayload: 확인 페이지가 본 것의 지문. 두 정책 지문 · 옛 세션 바이트 · 결과 폴더를 담는다.
func (s *server) activationPayload(o *ui.ActivationOffer) (string, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return "", err
	}
	rv, err := store.Rev([]store.Input{{Name: "results", Path: s.results, Dir: true}})
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(o.From + "|" + o.To + "|" + store.Sum(b) + "|" + rv))
	return hex.EncodeToString(h[:]), nil
}

// activationRefuse: 활성화 거절을 화면의 거절 종류로. 처리했으면 true다.
func (s *server) activationRefuse(w http.ResponseWriter, r *http.Request, back string, err error) bool {
	switch {
	case errors.Is(err, errScopeSame), errors.Is(err, rulesetround.ErrSamePolicy):
		s.refuse(w, r, http.StatusConflict, ui.RefusedScopeSame, back, nil)
	case errors.Is(err, errScopeCandidate), errors.Is(err, rulesetround.ErrNoCandidate):
		fmt.Fprintln(os.Stderr, "activate:", err)
		s.refuse(w, r, http.StatusConflict, ui.RefusedScopeNotFinalized, back, []ui.FileRef{file(ui.PickLang(r), "policy", s.scopeOut)})
	default:
		return false
	}
	return true
}

func (s *server) activationRequest(o *ui.ActivationOffer) rulesetround.Request {
	return rulesetround.Request{OldPath: s.path, OutPath: s.path, Org: s.orgName(), NewID: review.NewSessionID(),
		Activation: &rulesetround.Activation{PolicyPath: s.scopeAssets, CandidatePath: s.scopeOut, From: o.From, To: o.To},
		// **후보 정책으로** 다시 세운다. 새 세션은 새 정책의 지문을 든다.
		Rebuild: func() (review.Session, error) {
			d, err := decl.Load(s.decl)
			if err != nil {
				return review.Session{}, err
			}
			policy, err := review.LoadAssetPolicy(s.scopeOut)
			if err != nil {
				return review.Session{}, err
			}
			b, err := review.FromResultsWith(s.results, d, s.orgName(), policy)
			if err != nil {
				return review.Session{}, err
			}
			return b.Session, nil
		}}
}

// scopeActivateConfirm: 활성화 전 확인(POST). 아무것도 쓰지 않는다. 운영자 진술이 없으면 400이다.
func (s *server) scopeActivateConfirm(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenReview
	if !s.writable(w, r, back) {
		return
	}
	if s.results == "" || s.decl == "" {
		errorPage(w, r, http.StatusConflict, ui.ErrInputMissing, nil)
		return
	}
	if r.PostForm.Get(ui.ScopeActivateConfirmField) != "on" {
		s.refuse(w, r, http.StatusBadRequest, ui.RefusedScopeConfirm, back, nil)
		return
	}
	var (
		o       *ui.ActivationOffer
		payload string
		names   [3]string
	)
	cur, err := s.snapshot(ui.ReviewScopeActivateConfirmPath, func() error {
		var err error
		if o, err = s.activationCheck(); err != nil {
			return err
		}
		if payload, err = s.activationPayload(o); err != nil {
			return err
		}
		names[0], names[1], names[2], err = rulesetround.ActivationNames(s.activationRequest(o))
		return err
	})
	if err != nil {
		if !s.activationRefuse(w, r, back, err) {
			s.fail(w, r, back, err, file(ui.PickLang(r), "session", s.path))
		}
		return
	}
	l := ui.PickLang(r)
	lines := ui.ActivateConfirmLines(l, o.From, o.To, filepath.Base(s.scopeAssets), filepath.Base(names[2]), filepath.Base(s.scopeOut))
	files := []ui.FileRef{file(l, "policy", s.scopeOut), file(l, "session", s.path)}
	s.activateConfirmPage(w, r, kindActivate, ui.ReviewScopeActivatePath, back, s.reviewID(), cur, payload, lines, files)
}

// scopeActivate: 확인을 거친 활성화(POST). 확인 토큰이 맞고 만료 전이어야 하며, 다시 잰 지문이 확인한 것과 같아야 한다.
func (s *server) scopeActivate(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenReview
	if !s.writable(w, r, back) {
		return
	}
	if s.results == "" || s.decl == "" {
		errorPage(w, r, http.StatusConflict, ui.ErrInputMissing, nil)
		return
	}
	payload, ok := s.confirmed(w, r, kindActivate, back, s.reviewID())
	if !ok {
		return
	}
	if r.PostForm.Get(ui.ScopeActivateConfirmField) != "on" {
		s.refuse(w, r, http.StatusBadRequest, ui.RefusedScopeConfirm, back, nil)
		return
	}
	o, err := s.activationCheck()
	if err != nil {
		if !s.activationRefuse(w, r, back, err) {
			s.fail(w, r, back, err, file(ui.PickLang(r), "session", s.path))
		}
		return
	}
	if now, err := s.activationPayload(o); err != nil || now != payload {
		s.refuse(w, r, http.StatusConflict, ui.RefusedStale, back, nil)
		return
	}
	if _, err := rulesetround.Open(s.st, s.activationRequest(o), s.check(r), s.waitTime()); err != nil {
		if !s.activationRefuse(w, r, back, err) {
			s.fail(w, r, back, err, file(ui.PickLang(r), "session", s.path), file(ui.PickLang(r), "policy", s.scopeAssets))
		}
		return
	}
	redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeRulesetRoundOpened)}, nil)
}
