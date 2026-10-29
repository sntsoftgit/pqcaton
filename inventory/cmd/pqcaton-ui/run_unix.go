// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// ownGroup: 감독(과 그 아래 Ansible)을 제 프로세스 묶음에 띄운다. 멈출 때 묶음 전체에 알린다. 화면이 비정상으로
// 끝나면 감독이 표준 입력이 닫힌 것을 보고 마무리한다(supervise.go).
func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func terminate(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }

func kill(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

// syscallKill: 프로세스 하나에 끝내라고 알린다. force면 강제로 끝낸다.
func syscallKill(pid int, force bool) error {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	return syscall.Kill(pid, sig)
}

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
