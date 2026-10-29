// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func init() {
	// 덤프를 막아 환경을 읽을 수 없는 같은 사용자의 프로세스(OpenSSH 클라이언트 · sshd 세션과 같은 모양).
	testModes["-test-nondumpable"] = func() {
		_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
		time.Sleep(300 * time.Second)
	}
}

// IC-U137 — **기록해 둔 프로세스 묶음 번호를 믿지 않는다.** 지난 감독이 끝난 뒤 그 묶음 번호가 다른 묶음에 다시 쓰였고,
// 그 묶음의 같은 사용자 프로세스가 환경을 읽을 수 없어도(덤프 금지), 회수는 그 프로세스를 끝내지 않는다. 묶음은 지금
// 표지 프로세스가 있을 때만 이 실행의 것이다. 같은 묶음에 표지 프로세스가 있으면(감독이 살아 있는 동안처럼) 그
// 덤프 금지 구성원은 이 실행의 것으로 끝낸다.
func TestRecordedGroupIsNotTrusted(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spawn := func(env ...string) *exec.Cmd {
		cmd := exec.Command(exe, "-test-nondumpable")
		cmd.Env = append(os.Environ(), env...)
		ownGroup(cmd)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { kill(cmd); _ = cmd.Wait() })
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.ReadFile("/proc/" + itoa(cmd.Process.Pid) + "/environ"); err != nil {
				break // 덤프 금지가 걸렸다
			}
			if time.Now().After(deadline) {
				t.Fatal("the helper did not become non-dumpable")
			}
			time.Sleep(10 * time.Millisecond)
		}
		return cmd
	}
	alive := func(cmd *exec.Cmd) bool { _, err := procStart(cmd.Process.Pid); return err == nil }

	s := runReady(t, "ok")
	old := supervisorWait
	supervisorWait = 200 * time.Millisecond
	t.Cleanup(func() { supervisorWait = old })

	// 다른 묶음(번호가 지난 감독의 것과 같다)의 덤프 금지 프로세스.
	unrelated := spawn()
	id := "20261029T000005-111111111111"
	dir := filepath.Join(s.runsDir(), id)
	if err := os.MkdirAll(filepath.Join(dir, "next"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(dir, runManifest{ID: id, State: ui.RunRunning, Started: time.Now()}); err != nil {
		t.Fatal(err)
	}
	pgid := unrelated.Process.Pid
	if err := writeDurable(filepath.Join(dir, supervisorFile), supervisorRecord{PID: pgid, PGID: pgid, Start: "the-old-supervisor"}); err != nil {
		t.Fatal(err)
	}
	s.recoverRuns()
	time.Sleep(300 * time.Millisecond)
	if !alive(unrelated) {
		t.Fatal("a process in a reused group was stopped")
	}
	if m := readManifest(dir, ""); m.State != ui.RunInterrupted {
		t.Errorf("state %s", m.State)
	}

	// 같은 묶음에 지금 표지 프로세스가 있으면 그 덤프 금지 구성원은 이 실행의 것이다.
	marker := runMarker("20261029T000006-222222222222")
	leader := exec.Command("sleep", "300")
	leader.Env = append(os.Environ(), marker)
	ownGroup(leader)
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { kill(leader); _ = leader.Wait() })
	member := exec.Command(exe, "-test-nondumpable")
	member.SysProcAttr = &unix.SysProcAttr{Setpgid: true, Pgid: leader.Process.Pid}
	if err := member.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = member.Process.Kill(); _ = member.Wait() })
	time.Sleep(200 * time.Millisecond)
	ps, err := markedProcs(runMark{Marker: marker})
	if err != nil {
		t.Fatal(err)
	}
	found := map[int]bool{}
	for _, p := range ps {
		found[p.PID] = true
	}
	if !found[leader.Process.Pid] || !found[member.Process.Pid] || found[unrelated.Process.Pid] {
		t.Errorf("found %v; want the marked leader and its non-dumpable member, not the unrelated process", ps)
	}
}

func itoa(n int) string { return string(appendInt(nil, n)) }

func appendInt(b []byte, n int) []byte {
	if n >= 10 {
		b = appendInt(b, n/10)
	}
	return append(b, byte('0'+n%10))
}
