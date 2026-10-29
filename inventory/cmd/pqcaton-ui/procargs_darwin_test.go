// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// IC-U138 — **macOS가 환경을 빼고 돌려준 프로세스는 「표지 없음」이 아니라 「읽을 수 없음」이다.** Apple 플랫폼 바이너리
// (/bin/sleep)는 kern.procargs2에 환경이 빠진다. 표지를 주고 띄워도 표지로는 찾히지 않고, 부모나 같은 묶음의 표지 구성원이
// 없으면 이 실행의 것으로 증명되지 않는다. 그 묶음이 기록된 이 실행의 묶음이면 의심 프로세스로 잡힌다(회수 필요). 같은
// 묶음에 환경이 읽히는 표지 프로세스(플랫폼 바이너리가 아닌 것)가 있으면 이 실행의 것으로 찾힌다.
func TestStrippedEnvironmentIsUnknown(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := runMarker("20261029T000007-333333333333")
	platform := exec.Command("/bin/sleep", "300")
	platform.Env = append(os.Environ(), marker)
	ownGroup(platform)
	if err := platform.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platform.Process.Kill(); _ = platform.Wait() })
	time.Sleep(200 * time.Millisecond)
	pg := platform.Process.Pid

	ours, sus, err := scanProcs(runMark{Marker: marker, Suspect: []int{pg}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ours) != 0 || len(sus) != 1 || sus[0].PID != pg {
		t.Errorf("a platform binary alone: ours %v, suspects %v; want it only as a suspect", ours, sus)
	}

	leader := exec.Command(exe, "-test-sleep")
	leader.Env = append(os.Environ(), marker)
	ownGroup(leader)
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = leader.Process.Kill(); _ = leader.Wait() })
	member := exec.Command("/bin/sleep", "300")
	member.Env = append(os.Environ(), marker)
	member.SysProcAttr = groupOf(leader.Process.Pid)
	if err := member.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = member.Process.Kill(); _ = member.Wait() })
	time.Sleep(200 * time.Millisecond)
	ours, _, err = scanProcs(runMark{Marker: marker})
	if err != nil {
		t.Fatal(err)
	}
	found := map[int]bool{}
	for _, p := range ours {
		found[p.PID] = true
	}
	if !found[leader.Process.Pid] || !found[member.Process.Pid] || found[pg] {
		t.Errorf("found %v; want the marked leader and its platform-binary member, not the lone platform binary", ours)
	}
}
