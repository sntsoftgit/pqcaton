// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"fmt"

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
