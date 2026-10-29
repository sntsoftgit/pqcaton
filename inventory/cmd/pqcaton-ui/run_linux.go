// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
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

// markedProcs: 표지(환경 변수 한 줄)를 가진 이 사용자의 프로세스. 자기 자신과 좀비는 뺀다. 남의 프로세스는 environ을
// 읽을 수 없어 저절로 빠진다.
func markedProcs(marker string) []int {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	want := []byte("\x00" + marker + "\x00")
	self := os.Getpid()
	var out []int
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		env, err := os.ReadFile("/proc/" + e.Name() + "/environ")
		if err != nil || !bytes.Contains(append([]byte{0}, env...), want) {
			continue
		}
		if _, err := procStart(pid); err != nil {
			continue // 좀비
		}
		out = append(out, pid)
	}
	return out
}
