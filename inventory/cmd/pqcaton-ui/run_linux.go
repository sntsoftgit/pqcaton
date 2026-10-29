// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// deathSignal: 화면 프로세스가 끝나면 ansible-playbook도 끝나게 한다.
func deathSignal(a *syscall.SysProcAttr) { a.Pdeathsig = syscall.SIGTERM }

// dirswap: 두 디렉터리를 한 번에 맞바꾼다(renameat2 RENAME_EXCHANGE). 중간 상태가 없다.
func dirswap(a, b string) error {
	return unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE)
}
