// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 화면과 저장소(pkg/inventory/store)를 잇는 자리.
//
// **처리기는 rev를 따로 견주지 않는다.** 저장소의 SaveChecked · FinalizeChecked가 잠금 · 남은 기록
// 복구 · 확정 여부 · rev · 쓸 내용 만들기 · 쓰기를 한 연산으로 한다. 처리기가 먼저 견주고 저장을
// 부르면 그 사이에 다른 저장이 끼어든다. 처리기가 하는 일은 「무엇을 읽었나(입력)」와 「무엇을 쓰나
// (대상)」를 알려 주고, 잠금 안에서 불릴 build를 넘기는 것뿐이다.
//
// 거절은 상태 코드로 가른다: 오래된 화면 · 확정된 세션 · 잠금 대기 초과는 409, 복구 필요와 보기
// 전용은 503. 화면에는 역할과 파일 이름만 보이고, 전체 경로와 원래 오류는 표준오류에 남긴다.

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	kscope "github.com/randyinthedev-hash/pqcota-common/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// lockWait: 다른 저장이 대상 디렉토리를 쥐고 있을 때 기다리는 시간.
const lockWait = 5 * time.Second

// waitTime: 이 서버의 잠금 대기 시간. 테스트가 줄인다.
func (s *server) waitTime() time.Duration {
	if s.wait > 0 {
		return s.wait
	}
	return lockWait
}

// 확정의 종류. 영수증 경로의 한 칸이다.
const (
	kindReview = "review"
	kindScope  = "scope"
	kindDecl   = "decl"
)

// unnamed: 세션 ID가 없는 세션(옛 빌드가 쓴 파일, 아직 파일로 쓰지 않은 세션)의 저장에 쓰는 이름.
// 확정은 세션 ID가 있어야 하므로(review.Finalize) 이 이름으로 영수증이 생기지 않는다.
const unnamed = "unnamed-session"

func sessionName(id string) string {
	if id == "" {
		return unnamed
	}
	return id
}

// ── 입력: rev가 덮는 것 ──────────────────────────────────────────────────────

// reviewInputs: 리뷰 화면이 그려질 때 읽는 것 전부.
func (s *server) reviewInputs() []store.Input {
	in := []store.Input{{Name: "session", Path: s.path}}
	if s.decl != "" {
		in = append(in, store.Input{Name: "declaration", Path: s.decl})
	}
	if s.results != "" {
		in = append(in, store.Input{Name: "results", Path: s.results, Dir: true})
	}
	if s.scopeAssets != "" {
		in = append(in, store.Input{Name: "scope-assets", Path: s.scopeAssets})
	}
	return in
}

// scopeInputs: 스코프 화면이 그려질 때 읽는 것 전부.
func (s *server) scopeInputs() []store.Input {
	in := []store.Input{{Name: "scope-session", Path: s.scope}}
	for i, l := range s.layers {
		in = append(in, store.Input{Name: fmt.Sprintf("layer:%d", i), Path: l})
	}
	if s.base != "" {
		in = append(in, store.Input{Name: "base", Path: s.base})
	}
	return in
}

// assetPolicy: 자산 스코프 정책(-scope-assets)을 **지금** 읽는다. 상류 적재와 같은 것이어야 지문이 맞는다.
//
// 시작할 때 읽어 둔 사본을 쓰면, 서버가 떠 있는 동안 파일이 바뀌어도 화면과 확정은 옛 정책으로 대조한다.
// rev는 이 파일을 덮으므로 모델도 같은 시점의 파일을 읽어야 「모델과 rev가 같은 입력을 본다」가 선다.
// 리뷰 · 선언 · 확인 경로에서는 snapshot 콜백 안에서 불린다.
func (s *server) assetPolicy() (*kscope.AssetPolicy, error) {
	return review.LoadAssetPolicy(s.scopeAssets)
}

// declInputs: 선언 화면이 그려질 때 읽는 것 전부. 관측 결과는 후보 칸에 쓰인다.
func (s *server) declInputs() []store.Input {
	in := []store.Input{{Name: "declaration", Path: s.decl}}
	if s.results != "" {
		in = append(in, store.Input{Name: "results", Path: s.results, Dir: true})
	}
	if s.scopeAssets != "" {
		in = append(in, store.Input{Name: "scope-assets", Path: s.scopeAssets})
	}
	return in
}

// inputsFor: 화면(GET) 또는 저장 주소(POST)가 덮는 입력. 양식이 없는 화면은 nil이다.
func (s *server) inputsFor(where string) []store.Input {
	switch where {
	case ui.ScreenReview, ui.ScreenReviewNext, "/save", "/finalize", "/finalize/confirm":
		return s.reviewInputs()
	case ui.ScreenScope, ui.ScreenScopeNext, "/scope/save", "/scope/rules", "/scope/finalize", "/scope/finalize/confirm":
		if s.scope == "" {
			return nil
		}
		return s.scopeInputs()
	case ui.ScreenDecl, ui.ScreenDeclNext, "/decl/save":
		if s.decl == "" {
			return nil
		}
		return s.declInputs()
	}
	return nil
}

// snapshot: 화면 모델을 만들고, **그 모델과 같은 시점의 rev**를 돌려준다. 만들기 전과 뒤에 rev를 재어
// 같을 때만 짝짓는다. 다르면(그 사이 다른 탭이나 손 편집이 썼다) 다시 만든다. 모델을 만든 뒤에 rev만
// 따로 재면, 옛 내용을 그린 화면이 새 내용의 rev를 싣고 나가 그 위에 옛 폼 값을 덮어쓴다.
//
// 세 번 해도 맞지 않으면 빈 rev를 돌려준다. 그 화면의 저장은 「화면이 오래됐다」로 거부되고, 새로
// 고치면 다시 잰다. 양식이 없는 화면은 rev 없이 모델만 만든다.
func (s *server) snapshot(here string, build func() error) (string, error) {
	in := s.inputsFor(here)
	if in == nil {
		return "", build()
	}
	for i := 0; i < 3; i++ {
		before, err := store.Rev(in)
		if err != nil {
			return "", err
		}
		if err := build(); err != nil {
			return "", err
		}
		if s.afterModel != nil {
			s.afterModel()
		}
		after, err := store.Rev(in)
		if err != nil {
			return "", err
		}
		if before == after {
			return after, nil
		}
	}
	fmt.Fprintf(os.Stderr, "rev for %s: the files kept changing while the screen was drawn; its saves will be refused until it is reloaded\n", here)
	return "", nil
}

// check: 저장 요청이 들고 온 rev와 그 주소가 덮는 입력.
func (s *server) check(r *http.Request) store.Check {
	return store.Check{Inputs: s.inputsFor(r.URL.Path), Rev: r.PostForm.Get(ui.RevField)}
}

// ── 시작 ───────────────────────────────────────────────────────────────────

// targetDirs: 이 화면이 쓰는 파일의 디렉토리. 시작할 때 남은 준비 파일을 여기서 찾는다.
func (s *server) targetDirs() []string {
	var out []string
	for _, p := range append([]string{s.path, s.planOut, s.judgments, s.decl, s.scope, s.scopeOut}, s.layers...) {
		if p != "" {
			out = append(out, filepath.Dir(p))
		}
	}
	return out
}

// materialize: 입력에서 세운 세션을 파일로 남긴다. 시작할 때 한 번 부른다.
//
// 화면은 세션 파일이 없어도 선언과 관측(또는 계층)에서 세션을 세워 연다. 그런데 파일로 남기지
// 않으면 읽을 때마다 세션 ID가 새로 나고, 확정의 동일성(영수증)이 설 자리가 없다.
func (s *server) materialize() error {
	if _, err := os.Stat(s.path); os.IsNotExist(err) && s.results != "" {
		sf, _, err := s.reviewSession()
		if err != nil {
			return err
		}
		b, err := review.Encode(sf)
		if err != nil {
			return err
		}
		id := store.Identity{Kind: kindReview, Org: s.org, SessionID: sessionName(sf.SessionID)}
		if err := s.st.Save(id, []store.Change{{Path: s.path, Content: b}}, s.waitTime()); err != nil {
			return err
		}
	}
	if s.scope == "" || len(s.layers) == 0 {
		return nil
	}
	prev, err := scope.LoadSession(s.scope)
	if err == nil && prev.SessionID != "" {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	sf, _, err := s.scopeSession()
	if err != nil {
		return err
	}
	b, err := scope.EncodeSession(sf)
	if err != nil {
		return err
	}
	id := store.Identity{Kind: kindScope, Org: s.org, SessionID: sessionName(sf.SessionID)}
	return s.st.Save(id, []store.Change{{Path: s.scope, Content: b}}, s.waitTime())
}

// ── 거절 ───────────────────────────────────────────────────────────────────

// refusedKind: 저장소 오류를 화면의 거절 종류와 상태 코드로. 저장소 오류가 아니면 빈 값이다(그때는
// 처리기가 하던 대로 화면에 이유를 실어 돌려보낸다. 확정 관문의 「무엇이 남았나」가 그것이다).
func refusedKind(err error) (string, int) {
	switch {
	case errors.Is(err, store.ErrStale):
		return ui.RefusedStale, http.StatusConflict
	case errors.Is(err, store.ErrSessionFinalized):
		return ui.RefusedFinalized, http.StatusConflict
	case errors.Is(err, store.ErrTargetBusy):
		return ui.RefusedBusy, http.StatusConflict
	case errors.Is(err, store.ErrNeedsRecovery):
		return ui.RefusedRecovery, http.StatusServiceUnavailable
	case errors.Is(err, store.ErrChanged):
		return ui.RefusedStale, http.StatusConflict
	}
	return "", 0
}

// fail: 저장이 되지 않았을 때. 저장소가 거절했으면 거절 페이지를, 아니면 화면으로 돌려보낸다.
func (s *server) fail(w http.ResponseWriter, r *http.Request, back string, err error, files ...ui.FileRef) {
	kind, code := refusedKind(err)
	if kind == "" {
		redirect(w, r, back, "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	// 전체 경로와 원래 오류는 여기에만 남긴다.
	fmt.Fprintf(os.Stderr, "refused %s %s (%d): %v\n", r.Method, r.URL.Path, code, err)
	s.refuse(w, r, code, kind, back, files)
}

func (s *server) refuse(w http.ResponseWriter, r *http.Request, code int, kind, back string, files []ui.FileRef) {
	v := ui.NewRefused(s.page(r, "", ""), kind, back, files)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	if err := ui.RenderRefused(w, v); err != nil {
		fmt.Fprintln(os.Stderr, "rendering a refusal:", err)
	}
}

// writable: 저장할 수 있는 서버인가. 보기 전용이면 503을 내고 false.
func (s *server) writable(w http.ResponseWriter, r *http.Request, back string) bool {
	if s.st != nil {
		return true
	}
	s.refuse(w, r, http.StatusServiceUnavailable, ui.RefusedViewOnly, back, nil)
	return false
}

// file: 화면에 보일 파일. 역할과 이름만이다.
func file(l ui.Lang, role, path string) ui.FileRef {
	return ui.FileRef{Role: ui.RoleLabel(l, role), Name: filepath.Base(path)}
}
