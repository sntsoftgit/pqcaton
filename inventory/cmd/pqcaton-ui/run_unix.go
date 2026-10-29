// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// ownGroup: 관측 실행을 제 프로세스 묶음에 띄운다. ansible-playbook은 노드마다 ssh를 자식으로 띄우므로, 멈출 때
// 묶음 전체에 알려야 남는 것이 없다. 화면이 비정상으로 끝나면 묶음은 그대로 남고, 다음 시작이 기록의 PID와 시작
// 시각으로 찾아 끝낸다(recoverRuns). 부모가 죽을 때 자식만 끝내는 장치(Linux Pdeathsig)는 쓰지 않는다 — 묶음의 나머지가
// 주인 없이 남는다.
func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func terminate(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }

func kill(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

// signalGroup: 우리 자식이 아닌 묶음(지난 실행)에 끝내라고 알린다. force면 강제로 끝낸다.
func signalGroup(pgid int, force bool) {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-pgid, sig)
}

// groupAlive: 그 묶음에 프로세스가 남았는가.
func groupAlive(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
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
