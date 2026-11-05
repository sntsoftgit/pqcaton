// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// oldRound: 다른 상류 규칙 판으로 판정한 세션 · 선언 · 빈 결과 폴더.
func oldRound(t *testing.T) (dir, sess, declPath, results string) {
	t.Helper()
	dir = t.TempDir()
	sess = ready(t, dir)
	sf, err := review.Load(sess)
	if err != nil {
		t.Fatal(err)
	}
	sf.RulesetVersion = "pqcota-enrich/v0" + strings.TrimPrefix(review.RulesetVersion, review.UpstreamRuleset(review.RulesetVersion))
	sf.Scope = "org://local" // 결과에서 다시 세운 세션의 조직과 같아야 넘어간다
	if err := review.Save(sess, sf); err != nil {
		t.Fatal(err)
	}
	declPath = filepath.Join(dir, "declaration.json")
	if err := os.WriteFile(declPath, []byte(`{"org":"local","scope":["web"],"nodes":[{"name":"web","ips":["10.0.0.1"]}],`+
		`"assets":[{"node":"web","runtime":"openssl","component":"libssl"}],"edges":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	results = filepath.Join(dir, "results")
	if err := os.Mkdir(results, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir, sess, declPath, results
}

// IC-W53 — **명령 new-round는 화면과 같은 규칙 전환 회차를 연다.** -o가 다른 자리면 옛 파일은 바이트 그대로 남고, 새
// 세션(지금 규칙 판, 새 ID)과 그 옆의 보고서가 생긴다. 같은 명령을 다시 돌리면 거절되고 파일이 늘지 않는다. -o가 옛 세션
// 자리면 옛 바이트가 보관본으로 옮겨진다. -decl · -results · -o 없이는 아무것도 하지 않는다.
func TestNewRoundCommand(t *testing.T) {
	dir, sess, declPath, results := oldRound(t)
	before := read(t, sess)
	out := filepath.Join(dir, "session.v3.json")
	if err := newRound(sess, declPath, results, "", "local", "", "", ""); err == nil {
		t.Error("new-round without -o did something")
	}
	if err := newRound(sess, declPath, results, "", "local", out, "", ""); err != nil {
		t.Fatal(err)
	}
	next, err := review.Load(out)
	if err != nil || next.RulesetVersion != review.RulesetVersion || next.RulesetChange == nil || read(t, sess) != before {
		t.Fatalf("new session %q (%v); old file kept %v", next.RulesetVersion, err, read(t, sess) == before)
	}
	if reports, _ := filepath.Glob(filepath.Join(dir, "session.v3.ruleset-change-*.json")); len(reports) != 1 {
		t.Errorf("reports: %v", reports)
	}
	n, _ := os.ReadDir(dir)
	if err := newRound(sess, declPath, results, "", "local", filepath.Join(dir, "other.json"), "", ""); err == nil {
		t.Error("the same round opened twice")
	}
	if m, _ := os.ReadDir(dir); len(m) != len(n) {
		t.Error("a refused second round made files")
	}

	dir2, sess2, decl2, res2 := oldRound(t)
	before2 := read(t, sess2)
	if err := newRound(sess2, decl2, res2, "", "local", sess2, "", ""); err != nil {
		t.Fatal(err)
	}
	archives, _ := filepath.Glob(filepath.Join(dir2, "session.ruleset-*.json"))
	var archive string
	for _, a := range archives {
		if !strings.Contains(a, "ruleset-change-") {
			archive = a
		}
	}
	if archive == "" || read(t, archive) != before2 {
		t.Errorf("the old bytes were not archived: %v", archives)
	}
}
