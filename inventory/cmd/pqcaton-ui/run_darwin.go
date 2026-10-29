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

// markedProcs: 표지(환경 변수 한 줄)를 가진 이 사용자의 프로세스. kern.procargs2로 환경을 읽는다(같은 사용자의 것만
// 읽힌다). 자기 자신과 좀비는 뺀다.
func markedProcs(marker string) []int {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Getuid())
	if err != nil {
		return nil
	}
	want := []byte("\x00" + marker + "\x00")
	self := os.Getpid()
	var out []int
	for _, kp := range kps {
		pid := int(kp.Proc.P_pid)
		if pid == self || kp.Proc.P_stat == sZomb {
			continue
		}
		buf, err := unix.SysctlRaw("kern.procargs2", pid)
		if err != nil || !bytes.Contains(buf, want) {
			continue
		}
		out = append(out, pid)
	}
	return out
}
