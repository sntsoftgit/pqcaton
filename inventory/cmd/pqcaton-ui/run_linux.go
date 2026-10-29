// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// dirswap: 두 디렉터리를 한 번에 맞바꾼다(renameat2 RENAME_EXCHANGE). 중간 상태가 없다.
func dirswap(a, b string) error {
	return unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE)
}

// procStart: 프로세스의 시작 시각(부팅 뒤 클록 틱, /proc/<pid>/stat의 22번째 칸). 같은 PID가 다른 프로세스에 다시
// 쓰였는지 가린다. 좀비는 끝난 것으로 본다.
func procStart(pid int) (string, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", err
	}
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return "", errors.New("unexpected /proc stat")
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 20 {
		return "", errors.New("unexpected /proc stat")
	}
	if f[0] == "Z" || f[0] == "X" {
		return "", errors.New("the process has ended")
	}
	return f[19], nil
}
