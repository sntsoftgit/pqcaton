// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package rulesetround_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/rulesetround"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

const newID = "11111111-2222-4333-8444-555555555555"

func oldSession() review.Session {
	return review.Session{Note: review.Note, Scope: "org://acme", SessionID: "00000000-0000-4000-8000-000000000001",
		RulesetVersion: "pqcota-enrich/v0" + strings.TrimPrefix(review.RulesetVersion, review.UpstreamRuleset(review.RulesetVersion)),
		Reviewer:       "보안팀", Signature: "sig", PolicyDecisions: map[string]string{"openssl/libssl": "교체"},
		Items: []review.Item{{ID: "n1/openssl/libssl", Node: "n1", Runtime: "openssl", Policy: "openssl/libssl", State: "UNDECLARED",
			Mandatory: true, Conclusion: "교체한다", Plan: true, Level: "L2", Kind: "REMEDIATION_KIND_CONFIG_ONLY"}}}
}

func rebuilt() (review.Session, error) {
	return review.Session{Note: review.Note, Scope: "org://acme", SessionID: "ignored", RulesetVersion: review.RulesetVersion,
		PolicyDecisions: map[string]string{"openssl/libssl": ""},
		Items: []review.Item{{ID: "n1/openssl/libssl", Node: "n1", Runtime: "openssl", Policy: "openssl/libssl", State: "UNDECLARED",
			Mandatory: true, Level: "L2", Sources: []review.EvidenceSource{{FindingID: "f-v3", Path: "/usr/lib/libssl.so.3", SourceNodeID: "n1"}}}}}, nil
}

// world: 세션 파일 하나와 그 옆의 상태 폴더.
func world(t *testing.T, fault func(point, path string) error) (string, *store.State) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ws")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session.json")
	if err := review.Save(path, oldSession()); err != nil {
		t.Fatal(err)
	}
	return path, openState(t, dir, fault)
}

func openState(t *testing.T, dir string, fault func(point, path string) error) *store.State {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, store.DirName), store.Options{Warn: func(string) {}, Fault: fault})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	return st
}

func request(path string) rulesetround.Request {
	return rulesetround.Request{OldPath: path, OutPath: path, Org: "acme", NewID: newID, Rebuild: rebuilt}
}

func files(t *testing.T, res rulesetround.Result, path string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for k, p := range map[string]string{"session": path, "report": res.Report, "archive": res.Archive} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		out[k] = string(b)
	}
	return out
}

// IC-P26 — **화면과 명령이 쓰는 규칙 전환 함수 하나가, 멈춘 뒤 복구해도 끊김 없이 연 것과 같은 바이트를 낸다.** 새 세션 ·
// 보고서 · 보관본이 같다. 같은 요청을 다시 보내면 ErrStale이고 파일이 늘지 않는다. 이미 지금 규칙 판인 세션은
// ErrSameRuleset이다. 보관본 · 보고서 이름에는 `/`가 없다.
func TestOneFunctionSameBytesAfterRecovery(t *testing.T) {
	path, st := world(t, nil)
	res, err := rulesetround.Open(st, request(path), store.Check{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := files(t, res, path)
	if strings.Contains(filepath.Base(res.Archive), "/") || strings.Contains(filepath.Base(res.Report), "+") {
		t.Errorf("unsafe names: %s %s", res.Archive, res.Report)
	}
	before, _ := os.ReadDir(filepath.Dir(path))
	if _, err := rulesetround.Open(st, request(path), store.Check{}, time.Second); !errors.Is(err, rulesetround.ErrSameRuleset) && !errors.Is(err, store.ErrStale) {
		t.Errorf("the same request again: %v", err)
	}
	if after, _ := os.ReadDir(filepath.Dir(path)); len(after) != len(before) {
		t.Error("a second request made files")
	}
	st.Close()

	for _, point := range []string{store.FaultApply, store.FaultDone} {
		t.Run(point, func(t *testing.T) {
			fired := false
			p2, st2 := world(t, func(pt, _ string) error {
				if pt == point && !fired {
					fired = true
					return errors.New("stopped")
				}
				return nil
			})
			res2, err := rulesetround.Open(st2, request(p2), store.Check{}, time.Second)
			if err == nil {
				t.Fatal("the injected stop did not happen")
			}
			st2.Close()
			again := openState(t, filepath.Dir(p2), nil)
			defer again.Close()
			if err := again.Recover(time.Second); err != nil {
				t.Fatal(err)
			}
			got := files(t, res2, p2)
			for k := range want {
				if got[k] != want[k] {
					t.Errorf("%s after recovery differs from an uninterrupted round", k)
				}
			}
		})
	}
}

// entries: 폴더의 파일 이름. 저장소가 대상 폴더를 잠글 때 처음 한 번 만드는 잠금 파일은 회차의 산출물이 아니라 뺀다.
func entries(dir string) []string {
	e, _ := os.ReadDir(dir)
	var out []string
	for _, x := range e {
		if x.Name() != store.TargetLockName {
			out = append(out, x.Name())
		}
	}
	return out
}

// inputsWorld: 세션 옆에 선언 · 결과 폴더 · 자산 스코프 정책을 둔다. 내용은 시험에 쓰이지 않고 지문만 잰다.
func inputsWorld(t *testing.T) (path string, st *store.State, in map[string]string) {
	t.Helper()
	path, st = world(t, nil)
	dir := filepath.Dir(path)
	in = map[string]string{"session": path, "declaration": filepath.Join(dir, "declaration.json"),
		"results": filepath.Join(dir, "results", "web.json"), "scope-assets": filepath.Join(dir, "scope-assets.csv")}
	if err := os.Mkdir(filepath.Join(dir, "results"), 0o700); err != nil {
		t.Fatal(err)
	}
	for k, p := range in {
		if k != "session" {
			if err := os.WriteFile(p, []byte("v1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return path, st, in
}

// IC-P27 — **입력 넷 가운데 하나라도 회차를 여는 사이에 바뀌면 아무것도 쓰지 않는다.** 옛 세션 · 선언 · 결과 폴더 · 자산
// 스코프 정책의 지문을 먼저 재어 넘기고, 다시 세우는 도중에 하나를 바꾸면 ErrStale이고 보관본 · 새 세션 · 보고서 · 회차
// 표지가 하나도 생기지 않는다(명령의 길이다. 화면은 폼에 실린 rev로 같은 넷을 잰다).
func TestChangedInputStopsTheRound(t *testing.T) {
	for _, which := range []string{"session", "declaration", "results", "scope-assets"} {
		t.Run(which, func(t *testing.T) {
			path, st, in := inputsWorld(t)
			defer st.Close()
			dir := filepath.Dir(path)
			chk, err := rulesetround.Snapshot(rulesetround.Inputs(path, in["declaration"], filepath.Dir(in["results"]), in["scope-assets"]))
			if err != nil {
				t.Fatal(err)
			}
			before := entries(dir)
			req := request(path)
			req.Rebuild = func() (review.Session, error) {
				if err := os.WriteFile(in[which], []byte("changed while the round was opening\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return rebuilt()
			}
			if _, err := rulesetround.Open(st, req, chk, time.Second); !errors.Is(err, store.ErrStale) {
				t.Fatalf("err = %v, want ErrStale", err)
			}
			if after := entries(dir); len(after) != len(before) {
				t.Errorf("files were made: %v → %v", before, after)
			}
			old := oldSession()
			if mark, _ := st.RoundFrom(store.Identity{Kind: rulesetround.Kind, Org: "acme", SessionID: old.SessionID}); mark != nil {
				t.Error("a round marker was left")
			}
		})
	}
}

// IC-P28 — **조직이 다르면 넘기지 않는다.** 항목 ID가 같아도, 옛 세션 · 다시 세운 세션 · 요청한 조직 가운데 하나라도 다르면
// ErrOrgMismatch이고 파일이 생기지 않는다. 다른 조직의 결론이 참고값으로라도 넘어가면 안 된다.
func TestAnotherOrganizationIsRefused(t *testing.T) {
	for name, c := range map[string]struct{ org, rebuiltScope string }{
		"requested":    {"globex", "org://globex"},
		"rebuilt only": {"acme", "org://globex"},
	} {
		t.Run(name, func(t *testing.T) {
			path, st := world(t, nil)
			defer st.Close()
			before := entries(filepath.Dir(path))
			req := request(path)
			req.Org = c.org
			req.Rebuild = func() (review.Session, error) {
				sf, err := rebuilt()
				sf.Scope = c.rebuiltScope
				return sf, err
			}
			if _, err := rulesetround.Open(st, req, store.Check{}, time.Second); !errors.Is(err, rulesetround.ErrOrgMismatch) {
				t.Fatalf("err = %v, want ErrOrgMismatch", err)
			}
			if after := entries(filepath.Dir(path)); len(after) != len(before) {
				t.Errorf("files were made for another organization: %v → %v", before, after)
			}
		})
	}
}

// IC-P29 — **세션 ID가 없는 옛 파일 둘은 한 상태 폴더에서 저마다 회차를 연다.** 내부 이름을 경로와 바이트의 해시로 갈라,
// 첫 회차의 표지가 둘째를 막지 않는다.
func TestTwoUnnamedSessionsEachOpenARound(t *testing.T) {
	path, st := world(t, nil)
	defer st.Close()
	dir := filepath.Dir(path)
	second := filepath.Join(dir, "other.json")
	for _, p := range []string{path, second} {
		sf := oldSession()
		sf.SessionID = ""
		if err := review.Save(p, sf); err != nil {
			t.Fatal(err)
		}
	}
	for i, p := range []string{path, second} {
		req := request(p)
		req.NewID = []string{"aaaaaaaa-0000-4000-8000-000000000001", "aaaaaaaa-0000-4000-8000-000000000002"}[i]
		if _, err := rulesetround.Open(st, req, store.Check{}, time.Second); err != nil {
			t.Fatalf("round %d: %v", i+1, err)
		}
	}
}
