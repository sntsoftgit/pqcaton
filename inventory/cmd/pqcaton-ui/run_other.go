// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build !unix

package main

import (
	"errors"
	"os"
	"os/exec"
)

// 저장을 지원하지 않는 플랫폼(보기 전용)에서는 관측을 시작하지 않는다. 컴파일만 되게 둔다.
func ownGroup(*exec.Cmd) {}

func terminate(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

func kill(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

func dirswap(string, string) error {
	return errors.New("swapping directories is not supported on this platform")
}

func ownedByMe(os.FileInfo) bool { return false }

func links(os.FileInfo) uint64 { return 0 }
