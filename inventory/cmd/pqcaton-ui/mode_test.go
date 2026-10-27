// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/url"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-U92 — **화면 머리의 모드는 실제로 저장하는지와 같다.** 저장하는 서버는 「저장 가능」, 저장을 지원하지 않는
// 플랫폼은 「보기 전용」, 복구가 남은 서버는 「복구 필요」다. 「읽기·검토 모드」와 「원본은 보존」 같은 문구는
// 어느 화면에도 없다. 보기 전용 거절은 Linux와 Intel Mac(검증함), Apple Silicon Mac(검증하지 않음)을 두 말로 적는다.
func TestModeMatchesWhatTheServerDoes(t *testing.T) {
	for _, c := range []struct {
		name, ko, en string
		setup        func(*server)
	}{
		{"write", "저장 가능", "Saving enabled", func(*server) {}},
		{"view-only", "보기 전용: 이 플랫폼에서는 저장하지 않습니다", "View only: this platform does not save", func(s *server) { s.st = nil }},
		{"recovery", "복구 필요: 저장을 멈췄습니다", "Recovery required: saving is stopped", func(s *server) { s.recovering = true }},
	} {
		for _, lang := range []string{"ko", "en"} {
			s, _ := newServer(t)
			c.setup(s)
			body := get(t, s, ui.ScreenReview+"?lang="+lang).Body.String()
			want := map[string]string{"ko": c.ko, "en": c.en}[lang]
			if !strings.Contains(body, `class="mode mode-`+c.name+`">`+want+`<`) {
				t.Errorf("%s (%s): the header does not say %q", c.name, lang, want)
			}
		}
	}

	s := everyScreen(t)
	for _, sc := range ui.AllScreens() {
		for _, lang := range []string{"ko", "en"} {
			body := get(t, s, sc+"?lang="+lang).Body.String()
			for _, stale := range []string{"읽기·검토", "Read and review", "원본은 보존", "originals stay untouched"} {
				if strings.Contains(body, stale) {
					t.Errorf("%s (%s): still says %q", sc, lang, stale)
				}
			}
		}
	}

	v, _ := newServer(t)
	v.st = nil
	for lang, want := range map[string][]string{
		"ko": {"Linux와 Intel Mac(검증함)", "Apple Silicon Mac(검증하지 않음)"},
		"en": {"Linux and Intel Macs (verified)", "Apple Silicon Macs (not verified)"},
	} {
		body := postFormLang(t, v, "/save", lang, url.Values{"reviewer": {"x"}}).Body.String()
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: the view-only refusal does not say %q", lang, w)
			}
		}
	}
}
