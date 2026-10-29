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
	// Stuck · StuckWhy: 끝내지 못한 표지 프로세스와 그 까닭(확인하지 못한 경우 PID가 비고 까닭만 있다).
	Stuck    []int  `json:"stuck,omitempty"`
	StuckWhy string `json:"stuck_why,omitempty"`
}

// runMarker: 실행 하나의 프로세스에 붙는 표지.
func runMarker(id string) string { return "PQCATON_RUN=" + id }

// markedProc: 표지를 가진 프로세스 하나. 찾을 때의 시작 시각을 함께 들고 있어, 신호를 보내기 전에 같은 프로세스인지
// 다시 본다(signalProc). 그 사이 끝나 PID가 다른 프로세스에 다시 쓰였으면 건드리지 않는다.
type markedProc struct {
	PID   int
	Start string
}

// runMark: 이 실행의 프로세스를 가리는 것. 표지(환경 변수 한 줄)와, 이 실행이 띄운 것으로 아는 프로세스 묶음(감독 ·
// 정리 명령의 묶음). 묶음은 환경을 읽을 수 없는 프로세스를 가릴 때만 쓴다(markedProcs).
type runMark struct {
	Marker string
	Groups []int
}

// procInfo: 검색 중의 프로세스 하나.
type procInfo struct {
	markedProc
	ppid, pgid         int
	marked, unreadable bool
}

// belonging: 표지가 있는 프로세스와, 환경을 읽을 수 없는데 부모나 프로세스 묶음이 이 실행의 것인 프로세스. 계보를 따라
// 끝까지 넓힌다(표지 프로세스의 자식의 자식까지).
func belonging(all []procInfo, groups []int) []markedProc {
	in, grp := map[int]bool{}, map[int]bool{}
	for _, g := range groups {
		if g > 0 {
			grp[g] = true
		}
	}
	for _, p := range all {
		if p.marked {
			in[p.PID], grp[p.pgid] = true, true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, p := range all {
			if p.unreadable && !in[p.PID] && (in[p.ppid] || grp[p.pgid]) {
				in[p.PID], changed = true, true
			}
		}
	}
	var out []markedProc
	for _, p := range all {
		if in[p.PID] {
			out = append(out, p.markedProc)
		}
	}
	return out
}

// findMarked: 이 실행의 프로세스를 찾는 함수. 시험이 검색 실패를 주입한다.
var findMarked = markedProcs

// killMarked: 표지를 가진 프로세스(자기 자신 빼고)를 끝낸다. 끝내라고 알리고, grace까지 기다리고, 남으면 강제로 끝내고,
// 다시 기다린다. 끝내 남은 PID를 돌려준다(비면 다 끝났다). **프로세스를 확인하지 못하면 오류다** — 「확인할 수 없음」은
// 「남은 것 없음」이 아니다. 부르는 쪽은 둘 다 「회수 필요」로 닫는다.
func killMarked(rm runMark, grace time.Duration) ([]int, error) {
	ps, err := findMarked(rm)
	if err != nil || len(ps) == 0 {
		return nil, err
	}
	for _, p := range ps {
		_ = signalProc(p, false)
	}
	if done, err := waitMarkedGone(rm, grace); err != nil || done {
		return nil, err
	}
	ps, err = findMarked(rm)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		_ = signalProc(p, true)
	}
	if done, err := waitMarkedGone(rm, 5*time.Second); err != nil || done {
		return nil, err
	}
	ps, err = findMarked(rm)
	if err != nil {
		return nil, err
	}
	var left []int
	for _, p := range ps {
		left = append(left, p.PID)
	}
	return left, nil
}

func waitMarkedGone(rm runMark, limit time.Duration) (bool, error) {
	deadline := time.Now().Add(limit)
	for {
		ps, err := findMarked(rm)
		if err != nil {
			return false, err
		}
		if len(ps) == 0 {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// reaped: killMarked의 결과를 「회수 필요」의 까닭으로. 비면 다 끝났다.
func reaped(stuck []int, err error) string {
	switch {
	case err != nil:
		return "could not inspect the processes: " + err.Error()
	case len(stuck) > 0:
		return fmt.Sprintf("processes %v did not end", stuck)
	}
	return ""
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
		finishOrphaned(dir, m)
		return 4
	default:
	}

	marker := runMarker(m.ID)
	mine := runMark{Marker: marker, Groups: []int{syscall.Getpgrp()}}
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
		_, _ = killMarked(mine, stopGrace)
		<-done
	case <-parentGone:
		orphaned = true
		_, _ = killMarked(mine, stopGrace)
		<-done
	}
	// Ansible이 끝나도 표지를 가진 자식(세션을 떼어 나간 것 포함)이 남을 수 있다. 다 끝낸다.
	stuck, err := killMarked(mine, stopGrace)
	if why := reaped(stuck, err); why != "" {
		_ = writeDurable(filepath.Join(dir, supervisorFile), supervisorRecord{PID: os.Getpid(), PGID: syscall.Getpgrp(), Start: start, Stuck: stuck, StuckWhy: why})
		say("pqcaton: %s", why)
		if orphaned {
			markStuck(dir, m, stuck, why)
			return 4
		}
	} else if orphaned {
		finishOrphaned(dir, m)
		return 4
	}
	code := cmd.ProcessState.ExitCode()
	if code < 0 {
		code = 125
	}
	return code
}

// finishOrphaned: 화면 없이 마무리한다. 노드를 정리하고(정리가 남긴 자식까지 회수한다), 인벤토리를 지우고, 받던 것을
// partial/로 옮기고, 「중단됨」으로 적는다. 정리가 남긴 프로세스를 끝내지 못하면 아무것도 지우거나 옮기지 않고 「회수
// 필요」로 적는다.
func finishOrphaned(dir string, m runManifest) {
	logf := appendRunLog(dir)
	logf("pqcaton: the screen went away; the supervisor stopped the observation and cleans up")
	cleaned, why := runCleanup(dir, m.ID, logf)
	if why != "" {
		markStuck(dir, m, nil, why)
		return
	}
	m.NodeCleanup = cleaned
	removeRunSecrets(dir)
	if isDir(filepath.Join(dir, "next")) {
		_ = os.Rename(filepath.Join(dir, "next"), filepath.Join(dir, "partial"))
	}
	m.State, m.Ended = ui.RunInterrupted, time.Now()
	_ = writeManifest(dir, m)
}

// markStuck: 「회수 필요」로 적는다. 인벤토리 · 받던 것 · 노드 정리는 그대로 둔다(남은 프로세스가 아직 쓸 수 있다).
func markStuck(dir string, m runManifest, stuck []int, why string) {
	m.State, m.Stuck, m.StuckWhy = ui.RunStuck, stuck, why
	_ = writeManifest(dir, m)
	appendRunLog(dir)("pqcaton: " + why + "; nothing was cleaned up. Stop them, then restart the screen")
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
