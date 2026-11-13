// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// 관측 실행 시험(observerun_test.go 의 runReady)에 기대므로 그것과 같이 유닉스에서만 돈다. 나머지 pqcota 이력 시험은
// upstream_test.go 에서 모든 플랫폼에서 돈다.

//go:build unix

package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// IC-U200(관측 프로세스) — 화면 · 감독 · Ansible로 이어지는 실제 자식 프로세스의 환경을 본다. 화면의 환경에 PQCOTA_DSN이 있어도
// 가짜 ansible-playbook(관측과 정리 모두)은 그것을 받지 못해야 하고, 관측 로그에도 값이 없다.
func TestObservationProcessesDoNotGetTheDSN(t *testing.T) {
	const secret = "postgres://ro:s3cretpw@db.internal/pqcota"
	t.Setenv(upstream.DSNEnv, secret)
	for mode, want := range map[string]string{"ok": "OBSERVE DSN_ENV=none", "fail": "CLEANUP DSN_ENV=none"} {
		s := runReady(t, mode)
		if w := postForm(t, s, ui.RunPath, url.Values{"window": {"7"}}); w.Code != http.StatusSeeOther {
			t.Fatalf("%s: start = %d", mode, w.Code)
		}
		st := waitRun(t, s)
		if !strings.Contains(st.Log, want) || strings.Contains(st.Log, "DSN_ENV=set") || strings.Contains(st.Log, "s3cretpw") {
			t.Errorf("%s: a child process got PQCOTA_DSN, or the log does not say:\n%s", mode, st.Log)
		}
	}
}
