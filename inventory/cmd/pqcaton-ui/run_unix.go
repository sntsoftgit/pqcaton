// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// ownGroup: 관측 실행을 제 프로세스 묶음에 띄운다. ansible-playbook은 노드마다 ssh를 자식으로 띄우므로, 멈출 때
// 묶음 전체에 알려야 남는 것이 없다. Linux에서는 화면이 비정상으로 끝나도 ansible-playbook이 따라 끝난다(deathSignal).
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	deathSignal(cmd.SysProcAttr)
}

func terminate(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }

func kill(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

// ownedByMe · links: 승인 키 점검(approve.go). 이 사용자의 파일인가, 하드 링크가 몇 개인가.
func ownedByMe(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

func links(fi os.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 0
}
