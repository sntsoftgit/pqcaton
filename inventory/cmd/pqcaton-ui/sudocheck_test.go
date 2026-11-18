// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// sudoSample: 시험 자료의 변형 하나. Logged는 실험실에서 실제로 sudo를 돌렸을 때 입출력 기록이 생겼는가다.
type sudoSample struct {
	Name   string
	RC     int
	Logged bool
	Out    string
}

var sudoSampleHead = regexp.MustCompile(`^=== (\S+) rc=(\d+).*actual: sudo_rc=\d+ iolog_sessions=(\d+) stdin_logged=(\d+) output_logged=(\d+)`)

func sudoSamples(t *testing.T) []sudoSample {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "sudo-l-variants.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var out []sudoSample
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "#") {
			continue
		}
		if m := sudoSampleHead.FindStringSubmatch(l); m != nil {
			rc, _ := strconv.Atoi(m[2])
			out = append(out, sudoSample{Name: m[1], RC: rc, Logged: m[3] != "0" || m[4] != "0" || m[5] != "0"})
			continue
		}
		if len(out) > 0 {
			out[len(out)-1].Out += strings.TrimPrefix(l, "    ") + "\n"
		}
	}
	return out
}

// IC-U209 — **sudo 점검은 실제로 입출력을 기록하는 정책을 통과시키지 않는다.** 실험실에서 sudoers를 18가지로 바꿔 모은
// `LC_ALL=C sudo -n -l` 출력과, 같은 정책에서 실제로 sudo를 돌렸을 때의 기록 여부로 잰다. 기록되는데 「통과」인 변형은 없고,
// 통과는 기본 정책과 `log_servers`만 둔 정책뿐이다. 켜는 설정은 거절, 켜고 끄는 것이 섞였거나 명령별 · runas별 설정이 있거나
// 비밀번호를 요구하거나 다른 규칙이 있으면 확인 불가다. `requiretty`는 거절이다. 통과의 까닭은 「지원하는 출력 꼴에서 찾지 못했다」로
// 좁혀 말하고, 거절과 확인 불가는 까닭과 그 정책 줄을 보인다.
func TestSudoCheckNeverPassesALoggingPolicy(t *testing.T) {
	want := map[string]sudoVerdict{
		"00-plain": sudoPass, "01-log_input": sudoRefuse, "02-log_output": sudoRefuse,
		"03-input-then-neg-user": sudoUnknown, "04-neg-then-input-user": sudoUnknown,
		"05-cmd-sh-input": sudoUnknown, "06-cmd-other-input": sudoUnknown, "07-runas-root-output": sudoUnknown,
		"08-user-output": sudoRefuse, "09-host-input": sudoRefuse, "10-tag-log_input": sudoRefuse,
		"11-default-input-tag-nolog": sudoUnknown, "12-log_stdin": sudoRefuse, "13-log_ttyout": sudoRefuse,
		"14-requiretty": sudoRefuse, "15-password": sudoUnknown, "16-cmd-neg-sh": sudoUnknown, "17-log_servers": sudoPass,
	}
	samples := sudoSamples(t)
	if len(samples) != len(want) {
		t.Fatalf("read %d samples, want %d", len(samples), len(want))
	}
	for _, s := range samples {
		got := classifySudo(s.RC, s.Out)
		if got.Verdict != want[s.Name] {
			t.Errorf("%s: %s (%s), want %s", s.Name, got.Verdict, got.Why, want[s.Name])
		}
		if s.Logged && got.Verdict == sudoPass {
			t.Errorf("%s: the policy records I/O but the check passed it", s.Name)
		}
		switch got.Verdict {
		case sudoPass:
			if !strings.Contains(got.Why, "no sudo I/O logging setting was found") || !strings.Contains(got.Why, "supports") {
				t.Errorf("%s: a pass must say only that nothing was found in the supported form: %q", s.Name, got.Why)
			}
		default:
			if got.Why == "" {
				t.Errorf("%s: no reason", s.Name)
			}
			if s.RC == 0 && len(got.Lines) == 0 {
				t.Errorf("%s: no policy line shown with the reason", s.Name)
			}
		}
	}
	if c := classifySudo(1, "sudo: unexpected trouble\n"); c.Verdict != sudoUnknown {
		t.Errorf("an unknown failure of sudo -n -l = %s, want unknown", c.Verdict)
	}
	if c := classifySudo(0, "something else entirely\n"); c.Verdict != sudoUnknown {
		t.Errorf("output in another form = %s, want unknown", c.Verdict)
	}
}
