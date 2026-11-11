// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 「회수 필요」로 남긴 프로세스의 지금 상태(03). 운영자가 `ps -p` 를 돌리지 않고도 그 프로세스가 무엇인지 보게 한다.
//
// 안전 조건(설계: pqcaton-aai plans/2026-11-11-v0.27.0-collector-key-reclaim.md C 절):
//
//   - **PID 와 시작 시각을 함께 견준다.** 남길 때 찾은 그 짝과 같을 때만 이 실행의 프로세스로 보인다. 시작 시각이 다르면
//     PID 가 다른 프로세스에 다시 쓰인 것이고, 회수 대상으로 안내하지 않는다. 시작 시각을 남기지 않은 옛 기록은 같은
//     프로세스인지 확인할 수 없는 것으로 둔다.
//   - **인자는 여기서 가린 뒤에만 화면에 넘긴다.** 접어 두어도 HTML 에는 들어가므로 원문을 템플릿에 넘기지 않는다.
//   - **환경 변수는 화면에 싣지 않는다.** Linux 는 environ 을 열지 않는다. macOS 는 인자를 kern.procargs2 로만 읽을 수 있는데 그
//     응답 뒤쪽에 환경이 붙어 오므로, 인자 개수만큼만 읽고 나머지는 보지 않고 버린다.
//   - 조회 실패 · 권한 없음 · 이미 종료됨은 오류가 아니라 그 상태로 보인다. 다른 사용자의 프로세스는 세부를 싣지 않는다.
//   - 읽기만 한다. 파일 · 영수증 · 실행 기록을 바꾸지 않고 프로세스에 신호를 보내지 않는다.

import (
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// procFacts: 플랫폼이 읽은 프로세스의 사실. readProc 가 채운다(procinfo_linux.go · procinfo_darwin.go).
type procFacts struct {
	Start      string // procStart 와 같은 꼴(PID 재사용을 가리는 값)
	Ended      bool   // 좀비 등 이미 끝났다
	PPID, PGID int
	UID        int
	Exe        string
	ExeDenied  bool
	Args       []string
	ArgsDenied bool
	Started    time.Time
}

// inspectProc: 남긴 프로세스 하나의 지금 상태. 시험이 바꾼다.
var inspectProc = inspect

// readProcFn: 플랫폼의 readProc. 시험이 조회 실패를 주입한다.
var readProcFn = readProc

func inspect(p ui.ProcRecord) ui.ProcInfo {
	info := ui.ProcInfo{PID: p.PID}
	if p.Start == "" {
		info.State = ui.ProcUnconfirmed
		return info
	}
	f, err := readProcFn(p.PID)
	switch {
	case err == nil && f.Ended, err != nil && procGone(err):
		info.State = ui.ProcGone
		return info
	case err != nil && procDenied(err):
		info.State = ui.ProcDenied
		return info
	case err != nil:
		info.State = ui.ProcFailed
		return info
	case f.Start != p.Start:
		info.State = ui.ProcReused
		return info
	case f.UID != os.Getuid():
		info.State = ui.ProcDenied
		return info
	}
	info.State, info.PPID, info.PGID = ui.ProcSame, f.PPID, f.PGID
	info.User = strconv.Itoa(f.UID)
	if u, err := user.LookupId(info.User); err == nil {
		info.User = u.Username
	}
	info.Exe, info.ExeDenied = f.Exe, f.ExeDenied
	info.Args, info.ArgsDenied = redactArgs(f.Args), f.ArgsDenied
	if !f.Started.IsZero() {
		info.Started = f.Started.Local().Format("2006-01-02 15:04:05")
	}
	return info
}

// stuckInfo: 실행 기록이 남긴 프로세스마다 지금 상태. 시작 시각을 남기지 않은 옛 기록은 PID만으로 둔다.
func stuckInfo(records []ui.ProcRecord, pids []int) []ui.ProcInfo {
	if len(records) == 0 {
		for _, pid := range pids {
			records = append(records, ui.ProcRecord{PID: pid})
		}
	}
	var out []ui.ProcInfo
	for _, r := range records {
		out = append(out, inspectProc(r))
	}
	return out
}

func procRecords(ps []stuckProc) []ui.ProcRecord {
	var out []ui.ProcRecord
	for _, p := range ps {
		out = append(out, ui.ProcRecord{PID: p.PID, Start: p.Start})
	}
	return out
}

// 인자 가림. 발행 관문의 tools/releasecheck/redact.sh 와 같은 꼴을 가리고, 인자의 모양(「--password 값」)도 본다.
var (
	argURLUser = regexp.MustCompile(`(://)[^/@\s]+@`)
	argSecret  = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:pass|passwd|password|pwd|token|secret|apikey|api_key|dsn)[A-Za-z0-9_.-]*=)[^&\s"']+`)
	argKey     = regexp.MustCompile(`(([A-Za-z0-9_]*[_-](?:KEY|Key|key)|KEY)=)[^\s"']+`)
	argFlag    = regexp.MustCompile(`(?i)^--?[A-Za-z0-9_-]*(?:pass|password|passwd|token|secret|key|dsn)[A-Za-z0-9_-]*$`)
	argPEM     = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
)

// redactArgs: 인자를 가려 한 줄로 잇는다. 값을 받는 비밀 플래그(「--password 값」)는 다음 인자를 가린다.
func redactArgs(args []string) string {
	out := make([]string, 0, len(args))
	maskNext := false
	for _, a := range args {
		switch {
		case maskNext:
			a, maskNext = "***", false
		case argPEM.MatchString(a):
			a = "[private key redacted]"
		case argFlag.MatchString(a):
			maskNext = true
		default:
			a = argURLUser.ReplaceAllString(a, "$1***@")
			a = argSecret.ReplaceAllString(a, "$1***")
			a = argKey.ReplaceAllString(a, "$1***")
		}
		out = append(out, a)
	}
	return strings.Join(out, " ")
}
