// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"

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

// markedProcs: 이 실행의 프로세스와 그 시작 시각. kern.procargs2로 환경을 읽어 표지로 가리고, 같은 사용자인데 환경을
// 읽을 수 없는 프로세스(덤프 금지 · 보호)는 부모나 프로세스 묶음이 이 실행의 것일 때 넣는다(Linux와 같은 규칙). 목록을
// 얻지 못하거나 예상하지 못한 이유로 읽지 못하면 오류다. 그 사이 끝난 것과 좀비는 뺀다.
func markedProcs(rm runMark) ([]markedProc, error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Getuid())
	if err != nil {
		return nil, fmt.Errorf("cannot list processes: %w", err)
	}
	want := []byte("\x00" + rm.Marker + "\x00")
	self := os.Getpid()
	var all []procInfo
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
			p.marked = bytes.Contains(buf, want)
		default:
			if _, serr := procStart(pid); serr != nil {
				continue // 그 사이 끝났다
			}
			if !errors.Is(err, unix.EPERM) && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.EACCES) {
				return nil, fmt.Errorf("cannot read the environment of process %d: %w", pid, err)
			}
			p.unreadable = true
		}
		all = append(all, p)
	}
	return belonging(all, rm.Groups), nil
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
