// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

// 06 실행의 감독 프로세스. 03의 감독(supervise.go)과 같은 틀이다: 화면이 자기 바이너리를 감독 모드로 띄우고, 감독이
// Ansible을 띄운다. 자기를 기록한 뒤에야 띄우고, 화면이 사라지면 끝내고, 표지로 프로세스를 찾아 끝낸다.
//
// 06의 감독이 더 하는 일은 셋이다.
//
//   - **기록이 준비된 뒤에만 원격 작업을 허용한다.** 감독이 이번 실행의 기록 파일을 만들고 첫 줄을 쓴다. 전용 callback은
//     그 파일에 덧붙이기만 하고(만들지 않는다) 「준비됨」을 쓴 뒤 감독의 허용 파일을 기다린다. 감독은 그 줄을 읽은 뒤에만
//     허용 파일을 둔다. 기록을 만들 수 없거나, callback을 불러오지 못하거나, 준비 신호가 오지 않으면 첫 작업 전에 끝난다.
//   - **결과를 기다리는 상한.** 어떤 노드가 작업을 시작한 뒤 상한 동안 결과가 없으면 실행을 끝낸다. 그 노드의 상태는
//     「결과를 알 수 없음」이다: 이미 노드에 건너간 요청은 컨트롤러가 거둘 수 없고, 뒤늦게 끝날 수 있다.
//   - **Ansible의 출력을 남기지 않는다.** 전용 callback은 화면에 아무것도 쓰지 않는다. 그 밖의 출력(경고 · 오류)에는
//     플레이북의 줄이 실릴 수 있어 버린다. 화면이 보이는 것은 기록에서 근거를 댈 수 있는 것뿐이다.

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/provrun"
)

const provSuperviseFlag = "-provision-supervise"

// provReadyWait: callback의 준비 신호를 기다리는 시간. 넘으면 원격 작업을 허용하지 않고 끝낸다.
var provReadyWait = 20 * time.Second

// provSupervise: 감독 모드. 종료 코드는 Ansible의 것이다(신호로 끝났으면 125). 기록을 준비하지 못하면 3이고 Ansible을
// 띄우지 않는다.
func provSupervise(dir, ansible string) int {
	signal.Ignore(syscall.SIGPIPE)
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM, syscall.SIGINT)

	var m provManifest
	if b, err := os.ReadFile(filepath.Join(dir, provManifestFile)); err != nil || json.Unmarshal(b, &m) != nil || m.ID == "" {
		return 3
	}
	end := func(exit int, reason string) int {
		_ = writeDurable(filepath.Join(dir, provResultFile), provrun.Result{Exit: exit, Reason: reason, Ended: time.Now()})
		removeProvSecrets(dir)
		return exit
	}
	start, err := procStart(os.Getpid())
	if err != nil || recordSupervisor(dir, supervisorRecord{PID: os.Getpid(), PGID: syscall.Getpgrp(), Start: start}) != "" {
		return end(3, provrun.EndNotReady)
	}
	parentGone := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, os.Stdin); close(parentGone) }()

	// 기록 파일은 감독이 만든다. 이미 있으면(다른 실행의 것이면) 쓰지 않는다.
	rec := filepath.Join(dir, provEventsFile)
	f, err := os.OpenFile(rec, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return end(3, provrun.EndNotReady)
	}
	_, werr := f.Write(provrun.Header(m.ID))
	if serr := f.Sync(); werr != nil || serr != nil || f.Close() != nil {
		return end(3, provrun.EndNotReady)
	}

	marker := runMarker(m.ID)
	mine := runMark{Marker: marker, Suspect: []int{syscall.Getpgrp()}}
	goFile := filepath.Join(dir, provGoFile)
	args := []string{"-i", provInventory, "-e", "@vars.json"}
	if m.Check {
		args = append(args, "--check")
	}
	cmd := exec.Command(ansible, append(args, provPlaybook)...)
	cmd.Dir = dir
	cmd.Env = append(ansibleEnv(dir, marker), "PQCATON_EVENTS="+rec, "PQCATON_RUN_ID="+m.ID, "PQCATON_GO="+goFile)
	if err := cmd.Start(); err != nil {
		return end(3, provrun.EndNotReady)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()

	reason, ready := "", false
	stop := func(why string) {
		if reason == "" {
			reason = why
			_, _ = killMarked(mine, stopGrace)
		}
	}
	stall := time.Duration(m.Stall) * time.Second
	began := time.Now()
	seen := map[string]time.Time{}
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
loop:
	for {
		select {
		case <-done:
			break loop
		case <-term:
			stop(provrun.EndStopped)
		case <-parentGone:
			parentGone = nil // 닫힌 채널을 다시 고르지 않는다
			stop(provrun.EndOrphaned)
		case now := <-tick.C:
			if reason != "" {
				continue
			}
			b, err := os.ReadFile(rec)
			if err != nil {
				stop(provrun.EndNotReady)
				continue
			}
			if !ready {
				switch {
				case provrun.Ready(b, m.ID):
					// 준비 신호를 읽었다. 이제 허용한다. 허용 파일을 두지 못하면 callback이 기다리다 스스로 끝난다.
					ready = writeDurable(goFile, map[string]string{"run": m.ID}) == nil
				case now.Sub(began) > provReadyWait:
					stop(provrun.EndNotReady)
				}
				continue
			}
			for _, k := range provrun.Pending(b) {
				if _, ok := seen[k]; !ok {
					seen[k] = now
				}
				if stall > 0 && now.Sub(seen[k]) > stall {
					stop(provrun.EndStall)
					break
				}
			}
		}
	}
	code := cmd.ProcessState.ExitCode()
	if code < 0 {
		code = 125
	}
	if reason == "" && !ready {
		reason = provrun.EndNotReady // Ansible이 준비 신호 전에 스스로 끝났다(callback을 불러오지 못했다)
	}
	// Ansible이 끝나도 표지를 가진 자식이 남을 수 있다. 다 끝낸 뒤에 결과를 적는다.
	_, _ = reapRun(mine, stopGrace)
	return end(code, reason)
}
