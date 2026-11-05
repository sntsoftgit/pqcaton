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
