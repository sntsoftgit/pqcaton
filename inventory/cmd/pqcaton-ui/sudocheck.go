// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 02 관측 전 노드 점검: 노드 계정의 sudo 정책(`LC_ALL=C sudo -n -l`)을 읽어, 서명 키를 보내도 되는지 판정한다.
//
// 키는 pipelining으로 모듈 꾸러미와 함께 sudo의 표준 입력에 실려 간다. 노드 sudo가 입출력 기록을 켰으면 그 기록에 키가 남는다
// (검증: pqcaton-aai reports/2026-11-18-v0.31.0-key-delivery-paths.md). 그래서 기록 설정이 보이면 거절한다.
//
// **목록만으로 실제 적용을 정확히 가릴 수 없다**(같은 보고의 후속 reports/2026-11-18-v0.31.0-ansible-isolation.md). 사용자별
// `!log_input`은 파일 순서에 따라 결과가 달랐는데 목록에는 같은 꼴로 나왔고, 명령별 · runas별 설정은 별칭까지 생각하면
// `/bin/sh`에 적용되는지 알 수 없다. 그래서 규칙은 안전한 쪽으로 틀린다: 켜는 설정이 보이면 거절, 켜고 끄는 것이 섞였거나
// 명령별 설정이 있거나 조회가 실패하면 「확인 불가」로 중단한다. **통과는 「지원하는 출력 꼴에서 기록 설정을 찾지 못했다」는
// 뜻일 뿐이다.** sudo 밖의 감사 도구는 이 점검이 보지 못한다(운영자 안내서의 신뢰 전제).

import (
	"regexp"
	"strings"
)

// sudoVerdict: 노드 하나의 판정.
type sudoVerdict string

const (
	sudoPass    sudoVerdict = "pass"
	sudoRefuse  sudoVerdict = "refuse"
	sudoUnknown sudoVerdict = "unknown"
)

// sudoIO: 입출력 기록을 켜는 sudoers 옵션. log_input · log_output과 그 세부 항목이다.
var sudoIO = []string{"log_input", "log_output", "log_stdin", "log_stdout", "log_stderr", "log_ttyin", "log_ttyout"}

var (
	sudoIOPos   = regexp.MustCompile(`(?:^|[^!\w])(` + strings.Join(sudoIO, "|") + `)\b`)
	sudoIONeg   = regexp.MustCompile(`!(` + strings.Join(sudoIO, "|") + `)\b`)
	sudoRuleHdr = regexp.MustCompile(`^User \S+ may run the following commands on `)
	// sudoAllRule: root로 모든 명령을 실행하는 규칙. 이 꼴만 「/bin/sh를 root로」에 쓰이는 것으로 본다. 비밀번호 없이 목록을 읽었는데
	// NOPASSWD가 없는 규칙이면(listpw 설정 등) 권한 상승이 실패할 뿐 기록 판정은 같다.
	sudoAllRule = regexp.MustCompile(`^\((?:ALL|root)(?: : (?:ALL|root))?\) (?:NOPASSWD: )?ALL$`)
)

// sudoCheck: 판정과 운영자에게 보일 까닭, 그리고 까닭이 된 정책 줄.
type sudoCheck struct {
	Verdict sudoVerdict
	Why     string
	Lines   []string
}

// classifySudo: `LC_ALL=C sudo -n -l`의 종료 코드와 출력(표준 출력과 표준 오류를 이은 것)으로 판정한다.
func classifySudo(rc int, out string) sudoCheck {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	if rc != 0 {
		switch {
		case strings.Contains(out, "must have a tty"):
			return sudoCheck{sudoRefuse, "sudo requires a terminal (requiretty), so the key could not be passed without one", nil}
		case strings.Contains(out, "password is required"):
			return sudoCheck{sudoUnknown, "sudo asks this account for a password, so the policy that runs /bin/sh as root cannot be read; give it NOPASSWD", nil}
		}
		return sudoCheck{sudoUnknown, "`sudo -n -l` failed on the node, so its sudo policy could not be read", nonEmpty(lines)}
	}
	var sec string
	var defaults, specific, rules []string
	seenRules := false
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "Matching Defaults entries for "):
			sec = "d"
			continue
		case strings.HasPrefix(l, "Runas and Command-specific defaults for "):
			sec = "s"
			continue
		case sudoRuleHdr.MatchString(l):
			sec, seenRules = "r", true
			continue
		}
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		switch sec {
		case "d":
			defaults = append(defaults, t)
		case "s":
			specific = append(specific, t)
		case "r":
			rules = append(rules, t)
		}
	}
	if !seenRules {
		return sudoCheck{sudoUnknown, "the output of `sudo -n -l` is not in the form this check reads", nonEmpty(lines)}
	}
	var toks []string
	for _, d := range defaults {
		for _, t := range strings.Split(d, ",") {
			if t = strings.TrimSpace(t); t != "" {
				toks = append(toks, t)
			}
		}
	}
	var pos, neg []string
	for _, t := range toks {
		if t == "requiretty" {
			return sudoCheck{sudoRefuse, "sudo requires a terminal (requiretty), so the key could not be passed without one", []string{t}}
		}
		if contains(sudoIO, t) {
			pos = append(pos, t)
		}
		if strings.HasPrefix(t, "!") && contains(sudoIO, t[1:]) {
			neg = append(neg, t)
		}
	}
	var specIO, specNeg []string
	for _, s := range specific {
		if sudoIOPos.MatchString(s) || strings.Contains(s, "requiretty") {
			specIO = append(specIO, s)
		} else if sudoIONeg.MatchString(s) {
			specNeg = append(specNeg, s)
		}
	}
	var tagPos, tagNeg, other []string
	for _, r := range rules {
		switch {
		case strings.Contains(r, "LOG_INPUT:") && !strings.Contains(r, "NOLOG_INPUT:"), strings.Contains(r, "LOG_OUTPUT:") && !strings.Contains(r, "NOLOG_OUTPUT:"):
			tagPos = append(tagPos, r)
		case strings.Contains(r, "NOLOG_INPUT:"), strings.Contains(r, "NOLOG_OUTPUT:"):
			tagNeg = append(tagNeg, r)
		case !sudoAllRule.MatchString(r):
			other = append(other, r)
		}
	}
	mixed := (len(pos) > 0 && (len(neg) > 0 || len(tagNeg) > 0 || len(specNeg) > 0)) || (len(tagPos) > 0 && (len(tagNeg) > 0 || len(specNeg) > 0))
	switch {
	case len(specIO) > 0:
		return sudoCheck{sudoUnknown, "sudo has runas- or command-specific settings for I/O logging or a terminal; whether they apply to /bin/sh run as root cannot be told from this list", specIO}
	case mixed:
		return sudoCheck{sudoUnknown, "sudo I/O logging is both turned on and turned off; which one applies depends on the order sudo applies them, which this list does not show", append(append(append(pos, neg...), tagNeg...), specNeg...)}
	case len(pos) > 0:
		return sudoCheck{sudoRefuse, "sudo records the input and output of commands on this node (I/O logging), so the key would be kept in those records", pos}
	case len(tagPos) > 0:
		return sudoCheck{sudoRefuse, "a sudo rule records the input and output of commands (LOG_INPUT or LOG_OUTPUT), so the key would be kept in those records", tagPos}
	case len(other) > 0 || len(rules) == 0:
		return sudoCheck{sudoUnknown, "this check reads only a rule that runs every command as root without a password; the sudo rules here are different, so which one runs /bin/sh cannot be told", other}
	}
	return sudoCheck{Verdict: sudoPass, Why: "no sudo I/O logging setting was found in the `sudo -n -l` output form this check supports"}
}

func nonEmpty(lines []string) []string {
	var out []string
	for _, l := range lines {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
