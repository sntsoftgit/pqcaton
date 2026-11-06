// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-U153 — **화면의 확정도 세션의 정책 지문을 지금 정책과 대조한다.** 정책 기록이 없는 옛 세션 파일은 확인 페이지에서
// 409 `scope-baseline`(기준선 회차로 넘어간다), 정책 A로 세운 세션을 다른 정책이나 정책 없이 확정하려는 요청은 409
// `scope-unactivated`(활성화 기록 없는 변경)이고 어떤 파일도 바뀌지 않는다. 정책 없이 세운 세션을 정책 없이 확정하는 것은
// 그대로 된다.
func TestFinalizeChecksTheSessionPolicy(t *testing.T) {
	policyA := "action,runtime,lib,app_key,note\nexclude,*,*,/usr/sbin/sshd*,managed by OS patching\n"
	for _, c := range []struct {
		name    string
		digest  func(dir string) string
		active  string // 서버의 활성 정책 내용. 비우면 정책 없음
		refused string // 거절 코드. 비면 확인 페이지가 열린다
	}{
		{"a session that records no policy", func(string) string { return "" }, "", ui.RefusedScopeBaseline},
		{"opened under A, finalized under another policy", func(dir string) string {
			p := filepath.Join(dir, "a.csv")
			os.WriteFile(p, []byte(policyA), 0o644)
			d, _ := scope.PolicyFileDigest(p)
			return d
		}, "action,runtime,lib,app_key,note\nexclude,*,*,/usr/bin/python*,package runtime\n", ui.RefusedScopeUnactivated},
		{"opened under A, finalized without a policy", func(dir string) string {
			p := filepath.Join(dir, "a.csv")
			os.WriteFile(p, []byte(policyA), 0o644)
			d, _ := scope.PolicyFileDigest(p)
			return d
		}, "", ui.RefusedScopeUnactivated},
		{"no policy both times", func(string) string { return scope.PolicyDigest(nil) }, "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, dir := newServer(t)
			sf := session()
			sf.ScopePolicyDigestV1 = c.digest(t.TempDir())
			if err := review.Save(s.path, sf); err != nil {
				t.Fatal(err)
			}
			if c.active != "" {
				s.scopeAssets = filepath.Join(dir, "scope-assets.csv")
				if err := os.WriteFile(s.scopeAssets, []byte(c.active), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshotDir(t, dir)
			w := postFormLang(t, s, "/finalize/confirm", "ko", withRev(signed(), s.revFor("/finalize/confirm")))
			if c.refused == "" {
				if w.Code != http.StatusOK {
					t.Fatalf("confirm = %d\n%s", w.Code, w.Body.String())
				}
				return
			}
			if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), c.refused) {
				t.Fatalf("confirm = %d, want 409 %s\n%s", w.Code, c.refused, w.Body.String())
			}
			sameDir(t, before, snapshotDir(t, dir))
		})
	}
}
