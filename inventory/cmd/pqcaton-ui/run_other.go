// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build !unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
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

func procStart(int) (string, error) { return "", errors.New("not supported on this platform") }

func syscallKill(int, bool) error { return errors.New("not supported on this platform") }

func markedProcs(string) []int { return nil }

// 저장을 지원하지 않는 플랫폼에서는 관측을 시작하지 않으므로 감독도 없다.
const (
	superviseFlag  = "-observe-supervise"
	supervisorFile = "supervisor.json"
	sshArgs        = ""
)

type supervisorRecord struct {
	PID   int
	Start string
	Stuck []int
}

func runMarker(id string) string { return "PQCATON_RUN=" + id }

func supervise(string, string) int { return 3 }

func killMarked(string, time.Duration) []int { return nil }

func readSupervisor(string) *supervisorRecord { return nil }

func appendRunLog(string) func(string) { return func(string) {} }

func removeRunSecrets(dir string) {
	for _, name := range []string{runInventory, "vars.json", "observe.yml", "cleanup.yml"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
}
