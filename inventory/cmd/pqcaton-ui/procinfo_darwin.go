// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"golang.org/x/sys/unix"
)

// readProc: kern.proc.pid 로 상태 · 부모 · 묶음 · 사용자 · 시작 시각을, kern.procargs2 로 실행 파일과 인자를 읽는다.
//
// **kern.procargs2 응답 뒤쪽에는 환경이 붙어 온다.** 인자 개수(argc)만큼만 읽고 나머지는 보지 않고 버린다(procArgs).
func readProc(pid int) (procFacts, error) {
	var f procFacts
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	// 없는 PID 에는 커널이 빈 응답을 돌려주고, x/sys 는 그 길이 차이를 EIO 로 바꾼다. 이 조회에서 EIO 는 「그런 프로세스 없음」이다
	// (Intel Mac 에서 확인: 끝난 프로세스가 「조회 실패」로 보였다).
	if errors.Is(err, unix.EIO) {
		return f, errNoSuchProcess
	}
	if err != nil {
		return f, err
	}
	if kp.Proc.P_pid != int32(pid) {
		return f, errNoSuchProcess
	}
	t := kp.Proc.P_starttime
	f.Start = fmt.Sprintf("%d.%06d", t.Sec, t.Usec)
	f.Ended = kp.Proc.P_stat == sZomb
	f.PPID, f.PGID, f.UID = int(kp.Eproc.Ppid), int(kp.Eproc.Pgid), int(kp.Eproc.Ucred.Uid)
	f.Started = time.Unix(int64(t.Sec), int64(t.Usec)*1000)
	if buf, err := unix.SysctlRaw("kern.procargs2", pid); err == nil {
		if exe, args, ok := procArgs(buf); ok {
			f.Exe, f.Args = exe, args
		} else {
			f.ExeDenied, f.ArgsDenied = true, true
		}
	} else {
		f.ExeDenied, f.ArgsDenied = true, true
	}
	return f, nil
}

var errNoSuchProcess = errors.New("no such process")

// procArgs: kern.procargs2 응답의 실행 경로와 인자 argc 개. 그 뒤(환경)는 읽지 않는다.
func procArgs(buf []byte) (exe string, args []string, ok bool) {
	if len(buf) < 4 {
		return "", nil, false
	}
	argc := int(binary.LittleEndian.Uint32(buf[:4]))
	rest := buf[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return "", nil, false
	}
	exe, rest = string(rest[:i]), rest[i:]
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	for n := 0; n < argc; n++ {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			return "", nil, false
		}
		args, rest = append(args, string(rest[:j])), rest[j+1:]
	}
	return exe, args, true
}

func procGone(err error) bool {
	return errors.Is(err, errNoSuchProcess) || errors.Is(err, unix.ESRCH) || errors.Is(err, unix.ENOENT) || errors.Is(err, fs.ErrNotExist)
}

func procDenied(err error) bool {
	return errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) || errors.Is(err, fs.ErrPermission)
}
