// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// deathSignal: macOS에는 부모가 끝날 때 자식에게 알리는 장치가 없다. 남은 것은 다음 시작이 정리한다(recoverRuns).
func deathSignal(*syscall.SysProcAttr) {}

// dirswap: 두 디렉터리를 한 번에 맞바꾼다(renamex_np RENAME_SWAP). 중간 상태가 없다.
func dirswap(a, b string) error { return unix.RenamexNp(a, b, unix.RENAME_SWAP) }
