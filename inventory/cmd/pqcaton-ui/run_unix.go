// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

import (
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
