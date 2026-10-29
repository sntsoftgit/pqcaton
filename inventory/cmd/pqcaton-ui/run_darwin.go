// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// dirswap: 두 디렉터리를 한 번에 맞바꾼다(renamex_np RENAME_SWAP). 중간 상태가 없다.
func dirswap(a, b string) error { return unix.RenamexNp(a, b, unix.RENAME_SWAP) }

// sZomb: 좀비 프로세스의 상태(sys/proc.h SZOMB).
const sZomb = 5

// procStart: 프로세스의 시작 시각(kern.proc.pid의 p_starttime). 같은 PID가 다른 프로세스에 다시 쓰였는지 가린다.
// 좀비는 끝난 것으로 본다.
func procStart(pid int) (string, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", err
	}
	if kp.Proc.P_pid != int32(pid) {
		return "", errors.New("no such process")
	}
	if kp.Proc.P_stat == sZomb {
		return "", errors.New("the process has ended")
	}
	t := kp.Proc.P_starttime
	return fmt.Sprintf("%d.%06d", t.Sec, t.Usec), nil
}

// scanProcs: 이 사용자의 프로세스를 훑어 이 실행의 것과 의심 프로세스를 가린다(belonging, Linux와 같은 규칙).
// kern.procargs2로 환경을 읽는다. **macOS는 Apple 플랫폼 바이너리(/bin/sh · /usr/bin/ssh 같은 것)의 환경을 빼고 돌려준다**
// — 오류 없이 인자만 온다. 환경이 빠진 응답은 「읽을 수 없음」으로 다룬다(「표지 없음」이 아니다). 목록을 얻지 못하거나
// 예상하지 못한 이유로 읽지 못하면 오류다. 그 사이 끝난 것과 좀비는 뺀다.
func scanProcs(rm runMark) (ours, suspects []markedProc, err error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Getuid())
	if err != nil {
		return nil, nil, fmt.Errorf("cannot list processes: %w", err)
	}
	self := os.Getpid()
	all := []procInfo{selfInfo(rm.Marker, os.Getppid(), unix.Getpgrp())}
	for _, kp := range kps {
		pid := int(kp.Proc.P_pid)
		if pid == self || kp.Proc.P_stat == sZomb {
			continue
		}
		t := kp.Proc.P_starttime
		p := procInfo{markedProc: markedProc{PID: pid, Start: fmt.Sprintf("%d.%06d", t.Sec, t.Usec)},
			ppid: int(kp.Eproc.Ppid), pgid: int(kp.Eproc.Pgid)}
		buf, err := unix.SysctlRaw("kern.procargs2", pid)
		switch {
		case err == nil:
			if env, ok := procEnv(buf); ok {
				p.marked = hasEnvLine(env, rm.Marker)
			} else {
				p.unreadable = true
			}
		default:
			if _, serr := procStart(pid); serr != nil {
				continue // 그 사이 끝났다
			}
			if !errors.Is(err, unix.EPERM) && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.EACCES) {
				return nil, nil, fmt.Errorf("cannot read the environment of process %d: %w", pid, err)
			}
			p.unreadable = true
		}
		all = append(all, p)
	}
	ours, suspects = belonging(all, rm.Suspect)
	return ours, suspects, nil
}

// procEnv: kern.procargs2 응답의 환경 부분. 응답은 argc(int32) · 실행 경로 · NUL 채움 · 인자 argc개 · 환경 문자열들이다.
// 환경 문자열이 하나도 없으면(플랫폼 바이너리라 빠졌거나 정말 비었거나) ok가 false다 — 둘을 가릴 수 없으므로 읽지 못한 것으로 본다.
func procEnv(buf []byte) (env [][]byte, ok bool) {
	if len(buf) < 4 {
		return nil, false
	}
	argc := int(binary.LittleEndian.Uint32(buf[:4]))
	rest := buf[4:]
	i := bytes.IndexByte(rest, 0) // 실행 경로의 끝
	if i < 0 {
		return nil, false
	}
	rest = rest[i:]
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	for n := 0; n < argc && len(rest) > 0; n++ {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			return nil, false
		}
		rest = rest[j+1:]
	}
	for _, s := range bytes.Split(rest, []byte{0}) {
		if len(s) == 0 {
			break
		}
		env = append(env, s)
	}
	return env, len(env) > 0
}

func hasEnvLine(env [][]byte, line string) bool {
	for _, e := range env {
		if string(e) == line {
			return true
		}
	}
	return false
}

// signalProc: 찾을 때의 그 프로세스에만 신호를 보낸다. 시작 시각이 찾을 때와 같은지 본 뒤 보낸다. 다르면 원래 것은 이미
// 끝났으므로 아무것도 하지 않는다. macOS에는 pidfd가 없어, 견준 순간과 보내는 순간 사이의 아주 짧은 틈은 남는다.
func signalProc(p markedProc, force bool) error {
	if cur, err := procStart(p.PID); err != nil || cur != p.Start {
		return nil
	}
	sig := unix.SIGTERM
	if force {
		sig = unix.SIGKILL
	}
	err := unix.Kill(p.PID, sig)
	if errors.Is(err, unix.ESRCH) {
		return nil
	}
	return err
}

// groupOf: 그 묶음에 넣어 띄우는 설정(시험이 쓴다).
func groupOf(pgid int) *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true, Pgid: pgid} }
