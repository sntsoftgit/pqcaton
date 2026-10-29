// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

// 관측 감독 프로세스.
//
// 화면은 ansible-playbook을 직접 띄우지 않는다. 자기 바이너리를 감독 모드(`-observe-supervise <실행 자리> <ansible>`)로
// 띄우고, 감독이 Ansible을 띄운다. 까닭은 셋이다.
//
//   - **시작과 기록 사이에 틈이 없다.** 감독은 자기 PID와 시작 시각을 실행 자리에 확정해 쓴(fsync) 뒤에야 Ansible을
//     띄운다. 쓰지 못하면 Ansible을 띄우지 않고 끝난다. 그래서 디스크에 기록이 없는 관측은 원격에 닿은 적이 없다.
//   - **화면이 죽어도 감독이 마무리한다.** 화면과는 표준 입력 파이프로 이어져 있다. 화면이 어떻게 끝나든(강제 종료 포함)
//     파이프가 닫히고, 감독은 Ansible을 끝내고, 노드의 스테이징 폴더를 지우고, 인벤토리를 지우고, 받던 것을 partial/로
//     옮기고, 「중단됨」으로 적은 뒤 끝난다.
//   - **표지로 찾는다.** Ansible과 그 자식은 환경 변수 `PQCATON_RUN=<실행 ID>`를 물려받는다. 끝낼 때는 PID 하나가 아니라
//     이 표지를 가진 이 사용자의 프로세스를 모두 찾아 끝낸다. Ansible이 먼저 끝나 자식만 남았거나 자식이 세션을 떼어
//     나갔어도 찾는다. 감독 자신도 강제로 죽었으면, 다음 시작이 같은 표지로 찾는다(recoverRuns).

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

const (
	superviseFlag  = "-observe-supervise"
	supervisorFile = "supervisor.json"
	// sshArgs: 화면이 띄운 관측은 SSH 연결 공유를 쓰지 않는다. 공유 연결(mux)은 세션을 떼어 나가 기본 60초를 더 산다 —
	// 관측을 멈춘 뒤에 프로세스가 남는다.
	sshArgs = "-C -o ControlMaster=no -o ControlPersist=no"
)

// supervisorRecord: 감독의 식별 정보. 감독이 Ansible을 띄우기 전에 쓴다.
type supervisorRecord struct {
	PID   int    `json:"pid"`
	PGID  int    `json:"pgid"`
	Start string `json:"start"`
	// Stuck: 끝내지 못한 표지 프로세스. 비면 다 끝냈다.
	Stuck []int `json:"stuck,omitempty"`
}

// runMarker: 실행 하나의 프로세스에 붙는 표지.
func runMarker(id string) string { return "PQCATON_RUN=" + id }

// killMarked: 표지를 가진 프로세스(자기 자신 빼고)를 끝낸다. 끝내라고 알리고, grace까지 기다리고, 남으면 강제로
// 끝내고, 다시 기다린다. 끝내 남은 PID를 돌려준다(비면 다 끝났다).
func killMarked(marker string, grace time.Duration) []int {
	pids := markedProcs(marker)
	if len(pids) == 0 {
		return nil
	}
	for _, p := range pids {
		_ = syscallKill(p, false)
	}
	if waitMarkedGone(marker, grace) {
		return nil
	}
	for _, p := range markedProcs(marker) {
		_ = syscallKill(p, true)
	}
	if waitMarkedGone(marker, 5*time.Second) {
		return nil
	}
	return markedProcs(marker)
}

func waitMarkedGone(marker string, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for {
		if len(markedProcs(marker)) == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// writeDurable: 임시 파일 → fsync → rename → 폴더 fsync. 감독의 기록이 디스크에 닿은 뒤에야 Ansible을 띄운다.
func writeDurable(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(p))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// supervise: 감독 모드. 종료 코드는 Ansible의 것이다(신호로 끝났으면 125). 기록을 쓰지 못하면 3이고 Ansible을 띄우지
// 않는다. 화면이 사라졌으면 4다(마무리는 감독이 이미 했다).
func supervise(dir, ansible string) int {
	signal.Ignore(syscall.SIGPIPE) // 화면이 사라진 뒤 출력이 감독을 죽이지 않게 한다
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM, syscall.SIGINT)
	say := func(format string, args ...any) { fmt.Fprintf(os.Stdout, format+"\n", args...) }

	ansibleCommand = ansible // 화면이 정한 명령. 화면 없이 마무리할 때 정리 플레이북도 이것으로 부른다
	var m runManifest
	if b, err := os.ReadFile(filepath.Join(dir, runManifestFile)); err != nil || json.Unmarshal(b, &m) != nil || m.ID == "" {
		say("pqcaton: the supervisor cannot read the run record, so nothing was started")
		return 3
	}
	start, err := procStart(os.Getpid())
	if err == nil {
		err = writeDurable(filepath.Join(dir, supervisorFile), supervisorRecord{PID: os.Getpid(), PGID: syscall.Getpgrp(), Start: start})
	}
	if err != nil {
		say("pqcaton: the supervisor could not record itself, so nothing was started: %v", err)
		return 3
	}

	parentGone := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, os.Stdin); close(parentGone) }()
	select {
	case <-parentGone:
		finishOrphaned(dir, m, nil)
		return 4
	default:
	}

	marker := runMarker(m.ID)
	cmd := exec.Command(ansible, "-i", runInventory, "-e", "@vars.json", "observe.yml")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), marker, "ANSIBLE_NOCOLOR=1", "ANSIBLE_RETRY_FILES_ENABLED=0", "ANSIBLE_SSH_ARGS="+sshArgs)
	out, err := cmd.StdoutPipe()
	if err != nil {
		say("pqcaton: %v", err)
		return 3
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		say("pqcaton: %v", err)
		return 3
	}
	copied := make(chan struct{})
	go func() {
		defer close(copied)
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			fmt.Fprintln(os.Stdout, sc.Text()) // 화면이 사라졌으면 쓰지 못한다. 그래도 계속 읽어 Ansible이 막히지 않게 한다
		}
	}()
	done := make(chan error, 1)
	go func() { <-copied; done <- cmd.Wait() }()

	orphaned := false
	select {
	case <-done:
	case <-term:
		killMarked(marker, stopGrace)
		<-done
	case <-parentGone:
		orphaned = true
		killMarked(marker, stopGrace)
		<-done
	}
	// Ansible이 끝나도 표지를 가진 자식(세션을 떼어 나간 것 포함)이 남을 수 있다. 다 끝낸다.
	stuck := killMarked(marker, stopGrace)
	if len(stuck) > 0 {
		_ = writeDurable(filepath.Join(dir, supervisorFile), supervisorRecord{PID: os.Getpid(), PGID: syscall.Getpgrp(), Start: start, Stuck: stuck})
		say("pqcaton: these processes of the run did not end: %v", stuck)
	}
	if orphaned {
		finishOrphaned(dir, m, stuck)
		return 4
	}
	code := cmd.ProcessState.ExitCode()
	if code < 0 {
		code = 125
	}
	return code
}

// finishOrphaned: 화면 없이 마무리한다. 끝내지 못한 프로세스가 있으면 아무것도 지우거나 옮기지 않고 「회수 필요」로
// 적는다(그 프로세스가 아직 실행 자리를 쓸 수 있다). 다 끝냈으면 노드를 정리하고, 인벤토리를 지우고, 받던 것을
// partial/로 옮기고, 「중단됨」으로 적는다.
func finishOrphaned(dir string, m runManifest, stuck []int) {
	logf := appendRunLog(dir)
	if len(stuck) > 0 {
		m.State, m.Stuck = ui.RunStuck, stuck
		_ = writeManifest(dir, m)
		logf(fmt.Sprintf("pqcaton: the screen went away and processes %v of this run did not end; nothing was cleaned up", stuck))
		return
	}
	logf("pqcaton: the screen went away; the supervisor stopped the observation and cleans up")
	m.NodeCleanup = runCleanup(dir, m.ID, logf)
	removeRunSecrets(dir)
	if isDir(filepath.Join(dir, "next")) {
		_ = os.Rename(filepath.Join(dir, "next"), filepath.Join(dir, "partial"))
	}
	m.State, m.Ended = ui.RunInterrupted, time.Now()
	_ = writeManifest(dir, m)
}

// appendRunLog: 실행 로그 파일 끝에 덧붙이는 함수. 화면 없이(감독 · 다음 시작) 남길 때 쓴다.
func appendRunLog(dir string) func(string) {
	return func(line string) {
		if f, err := os.OpenFile(filepath.Join(dir, runLog), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600); err == nil {
			_, _ = f.WriteString(strings.TrimRight(line, "\n") + "\n")
			_ = f.Close()
		}
	}
}

// removeRunSecrets: 실행 자리에서 비밀이 실린 인벤토리와 실행용 파일을 지운다.
func removeRunSecrets(dir string) {
	for _, name := range []string{runInventory, "vars.json", "observe.yml", "cleanup.yml"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// readSupervisor: 감독의 기록. 없으면 nil이다(감독이 기록하기 전이면 Ansible도 뜬 적이 없다).
func readSupervisor(dir string) *supervisorRecord {
	b, err := os.ReadFile(filepath.Join(dir, supervisorFile))
	if err != nil {
		return nil
	}
	var r supervisorRecord
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}
