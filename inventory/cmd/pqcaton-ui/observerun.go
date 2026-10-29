// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 03 관측 화면의 관측 실행.
//
// **화면은 Ansible을 부를 뿐이다.** 노드에 붙고 수집기를 돌리는 일은 운영자가 설치한 `ansible-playbook`이
// 하고, 화면은 그 전에 빠진 것이 없는지 보고(preflight), 실행을 하나만 띄우고, 로그를 보이고, 끝나면 결과를
// 작업 폴더의 `results/`로 옮긴다.
//
// **비밀은 실행하는 동안에만 있다.** `targets.ini`(접근 비밀이 실린다)는 상태 폴더의 작업 자리에 소유자 전용으로
// 쓰고, 실행이 끝나면 작업 자리를 통째로 지운다. 상류도 이 파일을 「런타임 전용, 영속하지 않음」으로 둔다.
//
// **결과는 다 받은 뒤에 옮긴다.** Ansible이 결과를 쓰는 자리는 작업 자리 안이다. `results/`에 바로 쓰게 하면
// 실행 중에 화면을 다시 불러온 대조가 반쯤 쓴 파일을 읽는다.

import (
	"bufio"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

//go:embed observe.yml
var observePlaybook []byte

const (
	// maxLogLines: 화면에 남기는 로그 줄 수. 넘으면 앞에서 버린다.
	maxLogLines = 4000
	// stopGrace: 중지를 누른 뒤 강제로 끝내기까지 기다리는 시간.
	stopGrace = 10 * time.Second
	// 관측 구간(초)의 기본값과 범위.
	defaultWindow = 60
	minWindow     = 5
	maxWindow     = 3600
)

// ansibleCommand: 부를 명령. 시험이 가짜로 바꿔 낀다.
var ansibleCommand = "ansible-playbook"

// runner: 관측 실행 하나. 서버에 하나만 있다.
type runner struct {
	mu  sync.Mutex
	job *runJob
}

type runJob struct {
	id             string
	state          ui.RunState
	started, ended time.Time
	log            []string
	dropped        int
	received       int
	exitCode       int
	cmd            *exec.Cmd
	stopRequested  bool
	window         int
}

// status: 화면에 보일 것. 잠금 안에서 복사해 내보낸다.
func (r *runner) status() *ui.RunStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	j := r.job
	if j == nil {
		return nil
	}
	st := &ui.RunStatus{State: j.state, Started: j.started, Ended: j.ended, Received: j.received,
		Exit: j.exitCode, Window: j.window, Dropped: j.dropped}
	st.Log = strings.Join(j.log, "\n")
	return st
}

func (r *runner) running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.job != nil && r.job.state == ui.RunRunning
}

func (r *runner) logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.appendLocked(fmt.Sprintf(format, args...))
}

func (r *runner) appendLocked(line string) {
	j := r.job
	j.log = append(j.log, line)
	if len(j.log) > maxLogLines {
		j.dropped += len(j.log) - maxLogLines
		j.log = append([]string(nil), j.log[len(j.log)-maxLogLines:]...)
	}
}

// preflight: 관측을 시작하기 전에 볼 것. 막는 것(Blocking)이 하나라도 있으면 시작 단추가 없다.
func (s *server) preflight() []ui.RunCheck {
	var out []ui.RunCheck
	add := func(item ui.RunItem, ok bool, blocking bool, detail string) {
		out = append(out, ui.RunCheck{Item: item, OK: ok, Blocking: blocking, Detail: detail})
	}
	hosts, err := s.loadHosts()
	add(ui.RunHosts, err == nil && len(hosts) > 0, true, fmt.Sprint(len(hosts)))
	path, err := exec.LookPath(ansibleCommand)
	add(ui.RunAnsible, err == nil, true, path)
	add(ui.RunCollectors, s.collectors != "" && isDir(s.collectors), true, s.collectors)
	if s.collectors != "" && isDir(s.collectors) {
		var linux, windows bool
		for _, h := range hosts {
			if h.OS == inventory.OSWindows {
				windows = true
			} else {
				linux = true
			}
		}
		if linux {
			ok := false
			for _, arch := range []string{"linux-amd64", "linux-arm64"} {
				ok = ok || hasAll(filepath.Join(s.collectors, arch), "pqcota-nodescan", "pqcota-netcap", "pqcota-jvmscan")
			}
			add(ui.RunLinuxBins, ok, true, filepath.Join(s.collectors, "linux-amd64"))
		}
		if windows {
			add(ui.RunWindowsBins, hasAll(filepath.Join(s.collectors, "windows-amd64"), "pqcota-cngscan.exe", "pqcota-jvmscan.exe"),
				true, filepath.Join(s.collectors, "windows-amd64"))
		}
		add(ui.RunJar, isFile(filepath.Join(s.collectors, "collector.jar")), false, filepath.Join(s.collectors, "collector.jar"))
	}
	return out
}

func isDir(p string) bool  { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
func isFile(p string) bool { fi, err := os.Stat(p); return err == nil && fi.Mode().IsRegular() }

func hasAll(dir string, names ...string) bool {
	for _, n := range names {
		if !isFile(filepath.Join(dir, n)) {
			return false
		}
	}
	return true
}

// errRunBusy: 이미 관측이 돌고 있다.
var errRunBusy = errors.New("an observation is already running")

// startRun: 관측을 띄운다. 작업 자리를 만들고 인벤토리 · 플레이북 · 변수를 쓰고 `ansible-playbook`을 부른다.
// 끝나기를 기다리지 않는다. 띄우기 전에 실패하면 그 실행은 「실패」로 남고 까닭이 로그에 적힌다(화면이 그대로
// 보인다). 오류로 돌려주는 것은 이미 돌고 있을 때뿐이다.
func (s *server) startRun(window int) error {
	r := &s.run
	r.mu.Lock()
	if r.job != nil && r.job.state == ui.RunRunning {
		r.mu.Unlock()
		return errRunBusy
	}
	id, err := runID()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	r.job = &runJob{id: id, state: ui.RunRunning, started: time.Now(), window: window}
	r.mu.Unlock()

	fail := func(err error) error {
		r.mu.Lock()
		r.appendLocked("pqcaton: " + err.Error())
		r.job.state, r.job.ended = ui.RunFailed, time.Now()
		r.mu.Unlock()
		return nil
	}
	hosts, err := s.loadHosts()
	if err != nil {
		return fail(fmt.Errorf("cannot read %s: %w", wsHosts, err))
	}
	work := filepath.Join(s.st.Dir, "observe", id)
	if err := os.MkdirAll(filepath.Join(work, "results"), 0o700); err != nil {
		return fail(err)
	}
	vars, _ := json.Marshal(map[string]any{
		"results_dir": filepath.Join(work, "results"), "bin_root": s.collectors,
		"jar_present": isFile(filepath.Join(s.collectors, "collector.jar")), "netcap_window": window, "run_id": id,
	})
	for name, body := range map[string][]byte{
		"targets.ini": []byte(inventory.RenderAnsibleInventory(hosts)), "observe.yml": observePlaybook, "vars.json": vars,
	} {
		if err := os.WriteFile(filepath.Join(work, name), body, 0o600); err != nil {
			_ = os.RemoveAll(work)
			return fail(err)
		}
	}
	cmd := exec.Command(ansibleCommand, "-i", "targets.ini", "-e", "@vars.json", "observe.yml")
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "ANSIBLE_NOCOLOR=1", "ANSIBLE_RETRY_FILES_ENABLED=0")
	ownGroup(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	r.logf("pqcaton: %s -i targets.ini -e @vars.json observe.yml (%d nodes, network window %ds)", ansibleCommand, len(hosts), window)
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(work)
		return fail(err)
	}
	r.mu.Lock()
	r.job.cmd = cmd
	r.mu.Unlock()

	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			r.mu.Lock()
			r.appendLocked(sc.Text())
			r.mu.Unlock()
		}
	}()
	go func() {
		err := cmd.Wait()
		_ = pw.Close()
		received, moveErr := s.takeResults(filepath.Join(work, "results"))
		// 비밀이 실린 인벤토리와 함께 작업 자리를 지운다.
		rmErr := os.RemoveAll(work)
		r.mu.Lock()
		defer r.mu.Unlock()
		j := r.job
		j.received, j.ended = received, time.Now()
		j.exitCode = cmd.ProcessState.ExitCode()
		switch {
		case j.stopRequested:
			j.state = ui.RunStopped
		case err != nil:
			j.state = ui.RunFailed
		default:
			j.state = ui.RunDone
		}
		if moveErr != nil {
			r.appendLocked("pqcaton: could not move the results: " + moveErr.Error())
			j.state = ui.RunFailed
		}
		if rmErr != nil {
			r.appendLocked("pqcaton: could not remove the work directory " + work + ": " + rmErr.Error())
		}
		r.appendLocked(fmt.Sprintf("pqcaton: finished (exit %d), %d result files received", j.exitCode, received))
	}()
	return nil
}

// takeResults: 작업 자리에서 받은 결과를 작업 폴더의 results/로 옮긴다. 같은 이름은 새 관측으로 갈아 끼운다
// (rename은 같은 파일 시스템에서 한 번에 바뀐다). 빈 파일은 옮기지 않는다(그 노드에서 받은 것이 없다).
func (s *server) takeResults(from string) (int, error) {
	ents, err := os.ReadDir(from)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range ents {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		src := filepath.Join(from, e.Name())
		if fi, err := e.Info(); err != nil || fi.Size() == 0 {
			continue
		}
		if err := os.Rename(src, filepath.Join(s.results, e.Name())); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// stopRun: 돌고 있는 관측을 멈춘다. 프로세스 묶음에 끝내라고 알리고, stopGrace 안에 끝나지 않으면 강제로 끝낸다.
func (s *server) stopRun() bool {
	r := &s.run
	r.mu.Lock()
	j := r.job
	if j == nil || j.state != ui.RunRunning || j.cmd == nil || j.cmd.Process == nil {
		r.mu.Unlock()
		return false
	}
	j.stopRequested = true
	cmd := j.cmd
	r.appendLocked("pqcaton: stop requested")
	r.mu.Unlock()
	terminate(cmd)
	go func() {
		time.Sleep(stopGrace)
		if s.run.running() {
			kill(cmd)
		}
	}()
	return true
}

func runID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b), nil
}
