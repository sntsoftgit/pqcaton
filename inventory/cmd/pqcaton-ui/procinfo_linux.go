// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// clockTicks: /proc/<pid>/stat 의 시작 시각 단위(USER_HZ). 리눅스는 사용자 공간에 100으로 고정해 내보낸다.
const clockTicks = 100

// readProc: /proc 에서 읽는다. **environ 은 열지 않는다.** 인자는 cmdline 만 읽는다.
func readProc(pid int) (procFacts, error) {
	var f procFacts
	state, ppid, pgid, start, err := procStat(pid)
	if err != nil {
		return f, err
	}
	f.Start, f.PPID, f.PGID = start, ppid, pgid
	f.Ended = state == "Z" || state == "X"
	dir := "/proc/" + strconv.Itoa(pid)
	fi, err := os.Stat(dir)
	if err != nil {
		return f, err
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		f.UID = int(st.Uid)
	}
	if exe, err := os.Readlink(dir + "/exe"); err == nil {
		f.Exe = exe
	} else {
		f.ExeDenied = true
	}
	if b, err := os.ReadFile(dir + "/cmdline"); err == nil {
		for _, a := range bytes.Split(bytes.TrimRight(b, "\x00"), []byte{0}) {
			f.Args = append(f.Args, string(a))
		}
	} else {
		f.ArgsDenied = true
	}
	if ticks, err := strconv.ParseInt(start, 10, 64); err == nil {
		if boot := bootTime(); !boot.IsZero() {
			f.Started = boot.Add(time.Duration(ticks) * time.Second / clockTicks)
		}
	}
	return f, nil
}

// bootTime: /proc/stat 의 btime(부팅 시각, 초).
func bootTime() time.Time {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				return time.Unix(n, 0)
			}
		}
	}
	return time.Time{}
}

func procGone(err error) bool { return gone(err) }

func procDenied(err error) bool { return errors.Is(err, fs.ErrPermission) }
