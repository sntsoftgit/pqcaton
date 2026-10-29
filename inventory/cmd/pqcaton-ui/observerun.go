// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 03 관측 화면의 관측 실행.
//
// **화면은 Ansible을 부를 뿐이다.** 노드에 붙고 수집기를 돌리는 일은 운영자가 설치한 `ansible-playbook`이
// 하고, 화면은 그 전에 빠진 것이 없는지 보고(preflight), 실행을 하나만 띄우고, 로그를 보이고, 끝나면 결과를
// 세대로 다룬다.
//
// **접근 정보는 인벤토리의 문법이 되지 않는다.** hosts.csv의 값은 사람이 적은 것이라 공백 · 따옴표 ·
// `{{ }}`가 들어올 수 있다. 상류 RenderAnsibleInventory는 값을 INI 한 줄에 그대로 이어 붙여, 그런 값이 다른
// 변수를 끼워 넣는다. 그래서 인벤토리를 여기서 YAML로 쓰고 **모든 값을 `!unsafe` 문자열로** 싣는다(템플릿으로
// 풀리지 않는다). 호스트 이름은 우리가 정한 별칭(n1, n2…)이고, 원래 노드 ID는 변수(pqcaton_node)로 넘긴다.
// 플레이북은 명령을 argv로 부르고, 결과 파일 이름도 별칭으로 짓는다.
//
// **결과는 세대로 바꾼다.** 실행마다 results/ 옆에 자리(`observations/<id>/`)를 만들고, Ansible은 그 안의 `next/`에
// 결과를 쓴다. 끝나고 **모든 노드의 결과가 다 왔고 ansible-playbook이 0으로 끝났을 때만** `next/`와
// 작업 폴더의 `results/`를 한 번에 맞바꾼다(dirswap). 맞바꾼 뒤 `next/`에 든 것은 이전 결과라 `previous/`로
// 이름을 바꿔 남긴다. 실패 · 중지 · 빠진 노드가 있는 실행은 `partial/`로 격리하고 `results/`는 건드리지
// 않는다. 이전 결과와 새 부분 결과가 한 번의 관측으로 섞이지 않는다.
//
// **비밀은 실행하는 동안에만 있다.** 인벤토리(`targets.yml`, 접근 비밀이 실린다)는 실행이 끝나면 지운다.
// 서버가 끝날 때도 실행을 끝내고 기다린 뒤 지우고, 서버가 비정상으로 끝났으면 다음 시작에서 남은 것을 지운다
// (recoverRuns). 로그는 비밀번호를 가린 뒤에 화면과 파일에 남긴다.

import (
	"bufio"
	"bytes"
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/resultio"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

//go:embed observe.yml
var observePlaybook []byte

// cleanupPlaybook: 멈추거나 실패한 실행 뒤 노드의 스테이징 폴더를 지운다(finishRun).
//
//go:embed observe-cleanup.yml
var cleanupPlaybook []byte

const (
	// maxLogLines: 화면에 남기는 로그 줄 수. 넘으면 앞에서 버린다(파일에는 다 남는다).
	maxLogLines = 4000
	// stopGrace: 끝내라고 알린 뒤 강제로 끝내기까지 기다리는 시간.
	stopGrace = 10 * time.Second
	// cleanupLimit: 노드 정리 플레이북에 주는 시간. 넘으면 끝내고 남은 노드를 로그에 남긴다.
	cleanupLimit = 30 * time.Second
	// 관측 구간(초)의 기본값과 범위.
	defaultWindow = 60
	minWindow     = 5
	maxWindow     = 3600
	// 작업 자리 안의 이름.
	runInventory    = "targets.yml"
	runManifestFile = "manifest.json"
	runLog          = "log.txt"
	// currentMark: results/ 안에서 지금 세대를 밝히는 기록. 확장자가 없어야 결과 로더(*.json · *.jsonl)가 읽지 않는다.
	currentMark = ".pqcaton-run"
)

// ansibleCommand: 부를 명령. 시험이 가짜로 바꿔 낀다.
var ansibleCommand = "ansible-playbook"

// runner: 관측 실행 하나. 서버에 하나만 있다.
type runner struct {
	mu  sync.Mutex
	job *runJob
}

type runJob struct {
	m             runManifest
	dir           string
	log           []string
	dropped       int
	secrets       []string
	logFile       *os.File
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stopRequested bool
	// detached: 시험이 화면이 사라진 것을 흉내 낸다. 감독이 끝나도 화면 쪽 마무리를 하지 않는다.
	detached bool
	// done: 프로세스가 끝나고 결과 처리와 정리까지 마치면 닫힌다.
	done chan struct{}
}

// runManifest: 실행 하나의 기록. 작업 자리에 남고, 지금 세대면 results/에도 같은 것이 있다.
type runManifest struct {
	ID       string      `json:"id"`
	State    ui.RunState `json:"state"`
	Started  time.Time   `json:"started"`
	Ended    time.Time   `json:"ended,omitzero"`
	Exit     int         `json:"exit"`
	Window   int         `json:"window"`
	Nodes    []runSlot   `json:"nodes"`
	Files    []string    `json:"files,omitempty"`
	Missing  []string    `json:"missing,omitempty"`
	Received int         `json:"received"`
	// Stuck · StuckWhy: 끝내지 못했거나 확인하지 못한 이 실행의 프로세스와 그 까닭. 있으면 「회수 필요」이고, 아무것도
	// 지우거나 옮기지 않았다.
	Stuck    []int  `json:"stuck,omitempty"`
	StuckWhy string `json:"stuck_why,omitempty"`
	// NodeCleanup: 끝까지 돌지 못한 실행 뒤 노드 정리의 결과(done · failed). 비면 정리할 일이 없었다.
	NodeCleanup string `json:"node_cleanup,omitempty"`
	// Swapping: results/와 맞바꾸기 직전에 적는다. 이 표시로 끊겼으면 다음 시작이 results/의 기록으로 교환 여부를 가린다.
	Swapping bool `json:"swapping,omitempty"`
	// PreviousPending: 교환은 됐지만 이전 결과를 previous/로 옮기지 못했다. 다음 시작이 마친다.
	PreviousPending bool `json:"previous_pending,omitempty"`
}

// runSlot: 인벤토리의 별칭과 그것이 가리키는 노드.
type runSlot struct {
	Alias string `json:"alias"`
	Node  string `json:"node"`
	OS    string `json:"os"`
}

// status: 화면에 보일 것. 잠금 안에서 복사해 내보낸다.
func (r *runner) status() *ui.RunStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	j := r.job
	if j == nil {
		return nil
	}
	m := j.m
	return &ui.RunStatus{ID: m.ID, State: m.State, Started: m.Started, Ended: m.Ended, Received: m.Received,
		Exit: m.Exit, Window: m.Window, Dropped: j.dropped, Missing: m.Missing, NodeCleanup: m.NodeCleanup, Stuck: m.Stuck,
		StuckWhy: m.StuckWhy,
		Log:      strings.Join(j.log, "\n")}
}

func (r *runner) running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.job != nil && r.job.m.State == ui.RunRunning
}

// appendLocked: 로그 한 줄. 비밀번호를 가리고 화면과 파일에 함께 남긴다.
func (r *runner) appendLocked(line string) {
	j := r.job
	line = redact(line, j.secrets)
	if j.logFile != nil {
		_, _ = j.logFile.WriteString(line + "\n")
	}
	j.log = append(j.log, line)
	if len(j.log) > maxLogLines {
		j.dropped += len(j.log) - maxLogLines
		j.log = append([]string(nil), j.log[len(j.log)-maxLogLines:]...)
	}
}

func (r *runner) logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.appendLocked(fmt.Sprintf(format, args...))
}

// redact: 비밀 문자열을 가린다. 긴 것부터 바꿔 한 비밀이 다른 비밀의 일부일 때 남지 않게 한다.
func redact(line string, secrets []string) string {
	for _, s := range secrets {
		line = strings.ReplaceAll(line, s, "********")
	}
	return line
}

// ── 접근 정보 → 인벤토리 ────────────────────────────────────────────────────

// errHostValue: 접근 정보의 값에 인쇄할 수 없는 문자가 있다. 줄바꿈(\n · \r · U+0085) · 줄 · 문단 구분자
// (U+2028 · U+2029)는 YAML이 줄바꿈으로 읽어 인벤토리의 구조를 벗어나고, 그 밖의 제어 · 서식 문자도 노드 ID나 경로에
// 정당한 쓰임이 없다.
var errHostValue = errors.New("a character that cannot be printed in the access file")

// printable: 인쇄할 수 있는 문자와 공백뿐인가. 저장(ui.ApplyHosts)과 실행이 같은 규칙을 쓴다.
func printable(v string) bool {
	return strings.IndexFunc(v, func(r rune) bool { return r != ' ' && !unicode.IsPrint(r) }) < 0
}

// hostValuesOK: 모든 값이 printable인가.
func hostValuesOK(hosts []inventory.Host) error {
	for _, h := range hosts {
		for _, v := range []string{h.NodeID, h.Name, h.IP, h.SSHUser, h.SSHKey, h.SSHPass, h.OS, h.Conn} {
			if !printable(v) {
				return fmt.Errorf("%w (node %q)", errHostValue, strings.Map(func(r rune) rune {
					if r != ' ' && !unicode.IsPrint(r) {
						return '?'
					}
					return r
				}, h.NodeID))
			}
		}
	}
	return nil
}

// yamlUnsafe: Ansible이 템플릿으로 풀지 않는 YAML 문자열. 작은따옴표 안에서는 작은따옴표 두 개가 하나다.
// 제어 문자는 hostValuesOK가 먼저 막는다.
func yamlUnsafe(s string) string { return "!unsafe '" + strings.ReplaceAll(s, "'", "''") + "'" }

// renderTargets: 실행용 인벤토리(YAML). 호스트 이름은 별칭이고 값은 모두 !unsafe다. 상류 RenderAnsibleInventory와
// 같은 연결 규칙(WinRM은 비밀번호, SSH로 붙는 Windows는 PowerShell)을 따른다.
func renderTargets(hosts []inventory.Host) ([]runSlot, []byte, error) {
	if err := hostValuesOK(hosts); err != nil {
		return nil, nil, err
	}
	var b bytes.Buffer
	b.WriteString("# pqcaton: runtime-only inventory for one observation. Removed when the run ends.\n")
	b.WriteString("targets:\n  hosts:\n")
	var slots []runSlot
	for i, h := range hosts {
		alias := "n" + strconv.Itoa(i+1)
		slots = append(slots, runSlot{Alias: alias, Node: h.NodeID, OS: h.OS})
		fmt.Fprintf(&b, "    %s:\n", alias)
		put := func(k, v string) {
			if v != "" {
				fmt.Fprintf(&b, "      %s: %s\n", k, yamlUnsafe(v))
			}
		}
		put("pqcaton_node", h.NodeID)
		put("ansible_host", h.IP)
		fmt.Fprintf(&b, "      ansible_port: %d\n", h.Port)
		put("ansible_user", h.SSHUser)
		if h.Conn == inventory.ConnWinRM {
			b.WriteString("      ansible_connection: winrm\n")
			put("ansible_password", h.SSHPass)
		} else {
			put("ansible_ssh_private_key_file", h.SSHKey)
			put("ansible_ssh_pass", h.SSHPass)
			if h.OS == inventory.OSWindows {
				b.WriteString("      ansible_shell_type: powershell\n")
			}
		}
	}
	return slots, b.Bytes(), nil
}

// ── 시작 전 점검 ────────────────────────────────────────────────────────────

// preflight: 관측을 시작하기 전에 볼 것. 막는 것(Blocking)이 하나라도 있으면 시작 단추가 없다.
//
// Linux 수집기는 아키텍처마다 폴더가 따로다. 노드의 아키텍처는 접속해야 알 수 있어 여기서는 **있는 것을
// 적고**, 없는 아키텍처의 노드는 플레이북이 첫 단계에서 그 까닭으로 멈춘다(복사하다 실패하지 않는다).
func (s *server) preflight() []ui.RunCheck {
	var out []ui.RunCheck
	add := func(item ui.RunItem, ok bool, blocking bool, detail string) {
		out = append(out, ui.RunCheck{Item: item, OK: ok, Blocking: blocking, Detail: detail})
	}
	hosts, err := s.loadHosts()
	if err == nil {
		err = hostValuesOK(hosts)
	}
	add(ui.RunHosts, err == nil && len(hosts) > 0, true, fmt.Sprint(len(hosts)))
	path, err := exec.LookPath(ansibleCommand)
	add(ui.RunAnsible, err == nil, true, path)
	if stuck := s.stuckRuns(); len(stuck) > 0 {
		add(ui.RunStuckItem, false, true, strings.Join(stuck, " · "))
	}
	add(ui.RunCollectors, s.collectors != "" && isDir(s.collectors), true, s.collectors)
	if s.collectors == "" || !isDir(s.collectors) {
		return out
	}
	var linux, windows bool
	for _, h := range hosts {
		if h.OS == inventory.OSWindows {
			windows = true
		} else {
			linux = true
		}
	}
	if linux {
		var arches []string
		for _, arch := range []string{"linux-amd64", "linux-arm64"} {
			if hasAll(filepath.Join(s.collectors, arch), "pqcota-nodescan", "pqcota-netcap", "pqcota-jvmscan") {
				arches = append(arches, arch)
			}
		}
		add(ui.RunLinuxBins, len(arches) > 0, true, strings.Join(arches, " · "))
	}
	if windows {
		add(ui.RunWindowsBins, hasAll(filepath.Join(s.collectors, "windows-amd64"), "pqcota-cngscan.exe", "pqcota-jvmscan.exe"),
			true, filepath.Join(s.collectors, "windows-amd64"))
	}
	add(ui.RunJar, isFile(filepath.Join(s.collectors, "collector.jar")), false, filepath.Join(s.collectors, "collector.jar"))
	return out
}

// stuckRuns: 끝내지 못한 프로세스가 남은 실행(「실행 ID: PID…」). 있으면 새 관측을 시작하지 않는다.
func (s *server) stuckRuns() []string {
	if s.results == "" {
		return nil
	}
	ents, err := os.ReadDir(s.runsDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if m := readManifest(filepath.Join(s.runsDir(), e.Name()), e.Name()); e.IsDir() && m.State == ui.RunStuck {
			out = append(out, fmt.Sprintf("%s: %v %s", m.ID, m.Stuck, m.StuckWhy))
		}
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

// ── 실행 ────────────────────────────────────────────────────────────────────

// errRunBusy: 이미 관측이 돌고 있다.
var errRunBusy = errors.New("an observation is already running")

// runsDir: 실행 자리들. results/ 옆(작업 폴더 모드면 작업 폴더의 observations/)이다.
//
// **상태 폴더 안에 두지 않는다.** 결과를 맞바꾸면 이전 results/가 실행 자리로 오는데, 거기에는 운영자가 넣은 것이
// 무엇이든 있을 수 있다(넓은 권한 · 링크). 상태 폴더는 그런 것을 「안전하지 않은 파일」로 보고 화면을 띄우지 않는다.
// results/와 같은 폴더에 있어야 맞바꿀 수 있기도 하다(같은 파일 시스템).
func (s *server) runsDir() string { return filepath.Join(filepath.Dir(s.results), wsObservations) }

// startRun: 관측을 띄운다. 작업 자리를 만들고 인벤토리 · 플레이북 · 변수를 쓰고 `ansible-playbook`을 부른다.
// 끝나기를 기다리지 않는다. 띄우기 전에 실패하면 그 실행은 「실패」로 남고 까닭이 로그에 적힌다(화면이 그대로
// 보인다). 오류로 돌려주는 것은 이미 돌고 있을 때뿐이다.
func (s *server) startRun(window int) error {
	r := &s.run
	r.mu.Lock()
	if r.job != nil && r.job.m.State == ui.RunRunning {
		r.mu.Unlock()
		return errRunBusy
	}
	id, err := runID()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	j := &runJob{m: runManifest{ID: id, State: ui.RunRunning, Started: time.Now(), Window: window},
		dir: filepath.Join(s.runsDir(), id), done: make(chan struct{})}
	r.job = j
	r.mu.Unlock()

	fail := func(err error) error {
		r.mu.Lock()
		r.appendLocked("pqcaton: " + err.Error())
		j.m.State, j.m.Ended = ui.RunFailed, time.Now()
		r.mu.Unlock()
		s.closeRun(j)
		return nil
	}
	hosts, err := s.loadHosts()
	if err != nil {
		return fail(fmt.Errorf("cannot read %s: %w", wsHosts, err))
	}
	for _, h := range hosts {
		if h.SSHPass != "" {
			j.secrets = append(j.secrets, h.SSHPass)
		}
	}
	sort.Slice(j.secrets, func(a, b int) bool { return len(j.secrets[a]) > len(j.secrets[b]) })
	slots, targets, err := renderTargets(hosts)
	if err != nil {
		return fail(err)
	}
	j.m.Nodes = slots
	if err := os.MkdirAll(filepath.Join(j.dir, "next"), 0o700); err != nil {
		return fail(err)
	}
	if j.logFile, err = os.OpenFile(filepath.Join(j.dir, runLog), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err != nil {
		return fail(err)
	}
	vars, _ := json.Marshal(map[string]any{
		"results_dir": filepath.Join(j.dir, "next"), "bin_root": s.collectors,
		"jar_present": isFile(filepath.Join(s.collectors, "collector.jar")), "netcap_window": window, "run_id": id,
	})
	// 기록을 먼저 쓴다. 인벤토리를 쓴 뒤 서버가 죽어도 다음 시작이 이 자리를 「돌던 것」으로 알아본다.
	if err := writeManifest(j.dir, j.m); err != nil {
		return fail(err)
	}
	for _, f := range []struct {
		name string
		body []byte
	}{{runInventory, targets}, {"observe.yml", observePlaybook}, {"cleanup.yml", cleanupPlaybook}, {"vars.json", vars}} {
		if err := os.WriteFile(filepath.Join(j.dir, f.name), f.body, 0o600); err != nil {
			return fail(err)
		}
	}
	ansible, err := exec.LookPath(ansibleCommand)
	if err != nil {
		return fail(err)
	}
	exe, err := supervisorExe()
	if err != nil {
		return fail(err)
	}
	cmd := exec.Command(exe, superviseFlag, j.dir, ansible)
	cmd.Dir = j.dir
	ownGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fail(err)
	}
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	r.logf("pqcaton: %s -i %s -e @vars.json observe.yml (%d nodes, network window %ds)", ansible, runInventory, len(hosts), window)
	// 감독이 자기를 기록한 뒤에야 Ansible을 띄운다. 화면이 어떻게 끝나든 표준 입력이 닫혀 감독이 마무리한다.
	if err := cmd.Start(); err != nil {
		return fail(err)
	}
	r.mu.Lock()
	j.cmd, j.stdin = cmd, stdin
	r.mu.Unlock()

	read := make(chan struct{})
	go func() {
		defer close(read)
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
		<-read
		r.mu.Lock()
		if j.detached {
			r.mu.Unlock()
			close(j.done)
			return
		}
		_ = j.stdin.Close()
		j.m.Exit = cmd.ProcessState.ExitCode()
		switch {
		case j.stopRequested:
			j.m.State = ui.RunStopped
		case err != nil:
			j.m.State = ui.RunFailed
		default:
			j.m.State = ui.RunDone
		}
		r.mu.Unlock()
		s.finishRun(j)
	}()
	return nil
}

// finishRun: 끝난 실행의 결과를 세대로 다루고 비밀을 지운다. 끝나면 done을 닫는다.
func (s *server) finishRun(j *runJob) {
	r := &s.run
	// 감독이 강제로 끝났으면(중지가 늦어 KILL) 표지를 가진 프로세스가 남았을 수 있다. 다 끝내기 전에는 아무것도 하지 않는다.
	stuck, err := reapMarked(runMark{Marker: runMarker(j.m.ID), Groups: []int{j.cmd.Process.Pid}}, stopGrace)
	if why := reaped(stuck, err); why != "" {
		r.mu.Lock()
		j.m.State, j.m.Stuck, j.m.StuckWhy, j.m.Ended = ui.RunStuck, stuck, why, time.Now()
		r.appendLocked("pqcaton: " + why + "; nothing was cleaned up. Stop them, then restart the screen")
		m := j.m
		if j.logFile != nil {
			_ = j.logFile.Close()
			j.logFile = nil
		}
		r.mu.Unlock()
		_ = writeManifest(j.dir, m)
		close(j.done)
		return
	}
	next := filepath.Join(j.dir, "next")
	files, missing := expectedResults(next, j.m.Nodes)
	r.mu.Lock()
	j.m.Files, j.m.Missing = files, missing
	if j.m.State == ui.RunDone && len(missing) > 0 {
		j.m.State = ui.RunIncomplete
	}
	state := j.m.State
	r.mu.Unlock()

	// 끝까지 돌지 못한 실행은 노드에 스테이징 폴더가 남았을 수 있다. 인벤토리가 아직 있을 때 한 번 지운다.
	if state != ui.RunDone {
		cleaned, why := runCleanup(j.dir, j.m.ID, func(line string) { r.logf("%s", line) })
		if why != "" {
			r.mu.Lock()
			j.m.State, j.m.StuckWhy, j.m.Ended = ui.RunStuck, why, time.Now()
			r.appendLocked("pqcaton: " + why + "; nothing was cleaned up. Stop them, then restart the screen")
			m := j.m
			if j.logFile != nil {
				_ = j.logFile.Close()
				j.logFile = nil
			}
			r.mu.Unlock()
			_ = writeManifest(j.dir, m)
			close(j.done)
			return
		}
		r.mu.Lock()
		j.m.NodeCleanup = cleaned
		r.mu.Unlock()
	}
	var notes []string
	if state == ui.RunDone {
		committed, note := s.promote(j, files)
		notes = append(notes, note...)
		if !committed {
			state = ui.RunFailed
		}
	} else {
		_ = os.Rename(next, filepath.Join(j.dir, "partial"))
		note := fmt.Sprintf("pqcaton: not used as the current results (%s); what arrived is kept for diagnosis in %s",
			state, filepath.Join(j.dir, "partial"))
		if len(missing) > 0 {
			note += "; missing or not valid: " + strings.Join(missing, ", ")
		}
		notes = append(notes, note)
	}
	r.mu.Lock()
	j.m.State, j.m.Ended, j.m.Received = state, time.Now(), len(files)
	for _, n := range notes {
		r.appendLocked(n)
	}
	r.appendLocked(fmt.Sprintf("pqcaton: finished (%s, exit %d)", state, j.m.Exit))
	r.mu.Unlock()
	s.closeRun(j)
}

// promote: 받은 세대를 지금 결과로 올린다. **dirswap이 성공한 순간이 커밋이다.**
//
//   - 교환 전에 실패하면 지금 결과는 그대로이고 받은 것은 partial/로 간다(committed=false).
//   - 교환한 뒤의 일(next/에 든 이전 결과를 previous/로 옮기기)이 실패해도 새 결과는 이미 지금 결과다. 실행은 성공으로
//     두고, 이전 결과의 보관만 「정리 필요」로 적는다. 다음 시작이 results/의 기록과 실행 기록을 견주어 마친다.
//
// 교환하기 전에 실행 기록을 「교환 중」으로 남긴다. 그 사이에 끊기면 다음 시작이 results/의 기록으로 교환이 일어났는지 안다.
func (s *server) promote(j *runJob, files []string) (bool, []string) {
	r := &s.run
	next := filepath.Join(j.dir, "next")
	r.mu.Lock()
	m := j.m
	r.mu.Unlock()
	m.State, m.Received, m.Ended = ui.RunDone, len(files), time.Now()
	err := writeManifestAs(filepath.Join(next, currentMark), m)
	if err == nil {
		r.mu.Lock()
		j.m.Swapping = true
		pre := j.m
		r.mu.Unlock()
		err = writeManifest(j.dir, pre)
	}
	if err == nil {
		err = swapDirs(next, s.results)
	}
	if err != nil {
		_ = os.Rename(next, filepath.Join(j.dir, "partial"))
		return false, []string{"pqcaton: the results could not become current, so the earlier results stay: " + err.Error()}
	}
	notes := []string{fmt.Sprintf("pqcaton: %d result files are now the current results", len(files))}
	r.mu.Lock()
	j.m.Swapping = false
	r.mu.Unlock()
	if err := keepPrevious(j.dir); err != nil {
		r.mu.Lock()
		j.m.PreviousPending = true
		r.mu.Unlock()
		notes = append(notes, "pqcaton: the earlier results are still in "+next+" and could not be kept as previous/ ("+
			err.Error()+"); the next start finishes this")
	} else {
		notes = append(notes, "pqcaton: the earlier results are kept in "+filepath.Join(j.dir, "previous"))
	}
	return true, notes
}

// swapDirs · keepPrevious: 교환과 그 뒤의 일. 시험이 실패를 주입한다.
var (
	swapDirs     = dirswap
	keepPrevious = finishSwap
)

// finishSwap: 교환 뒤 next/에 남은 이전 결과를 previous/로 옮긴다. 이미 옮겼으면 아무것도 하지 않는다.
func finishSwap(dir string) error {
	next := filepath.Join(dir, "next")
	if !isDir(next) {
		return nil
	}
	return os.Rename(next, filepath.Join(dir, "previous"))
}

// runCleanup: 정리 플레이북을 cleanupLimit 안에서 돌린다. 출력은 logf로 남기고, 다 지웠으면 "done", 아니면 "failed"다.
// **정리가 남긴 프로세스도 회수한다** — 정리용 Ansible도 같은 실행 표지를 물려받으므로, 끝난 뒤 그 표지로 남은 자식을
// 찾아 끝낸다. 끝내지 못하거나 확인하지 못하면 둘째 값에 그 까닭을 돌려준다(부르는 쪽은 인벤토리를 지우지 않는다).
// 실행 중인 관측의 끝(finishRun)과, 비정상 종료 뒤의 다음 시작(recoverRuns)이 함께 쓴다.
func runCleanup(dir, id string, logf func(string)) (string, string) {
	if !isFile(filepath.Join(dir, runInventory)) {
		logf("pqcaton: no inventory left to reach the nodes, so their staging folders were not removed")
		return "failed", ""
	}
	if !isFile(filepath.Join(dir, "cleanup.yml")) {
		if err := os.WriteFile(filepath.Join(dir, "cleanup.yml"), cleanupPlaybook, 0o600); err != nil {
			logf("pqcaton: node cleanup did not start: " + err.Error())
			return "failed", ""
		}
	}
	logf("pqcaton: removing the staging folders from the nodes")
	cmd := exec.Command(ansibleCommand, "-i", runInventory, "-e", "@vars.json", "cleanup.yml")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), runMarker(id), "ANSIBLE_NOCOLOR=1", "ANSIBLE_RETRY_FILES_ENABLED=0", "ANSIBLE_SSH_ARGS="+sshArgs)
	ownGroup(cmd)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		logf("pqcaton: node cleanup did not start: " + err.Error())
		return "failed", ""
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(cleanupLimit):
		kill(cmd)
		err = <-done
		logf(fmt.Sprintf("pqcaton: node cleanup took longer than %s and was stopped", cleanupLimit))
	}
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		logf(line)
	}
	if why := reaped(reapMarked(runMark{Marker: runMarker(id), Groups: []int{cmd.Process.Pid}}, stopGrace)); why != "" {
		return "", "after the node cleanup, " + why
	}
	if err != nil {
		logf(fmt.Sprintf("pqcaton: node cleanup did not finish everywhere (%v); a node may keep %s", err, stagingPaths(id)))
		return "failed", ""
	}
	return "done", ""
}

// stagingPaths: 노드에 남을 수 있는 스테이징 폴더. 화면과 안내서가 같은 말을 쓴다.
func stagingPaths(id string) string {
	return "/tmp/pqcaton-observe-" + id + " (Windows: C:\\Windows\\Temp\\pqcaton-observe-" + id + ")"
}

// closeRun: 비밀이 실린 파일을 지우고 기록을 남긴 뒤 done을 닫는다. 실패해도 done은 닫는다.
func (s *server) closeRun(j *runJob) {
	r := &s.run
	for _, name := range []string{runInventory, "vars.json", "observe.yml", "cleanup.yml"} {
		if err := os.Remove(filepath.Join(j.dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			r.logf("pqcaton: could not remove %s: %v", name, err)
		}
	}
	r.mu.Lock()
	m := j.m
	if j.logFile != nil {
		_ = j.logFile.Close()
		j.logFile = nil
	}
	r.mu.Unlock()
	if isDir(j.dir) {
		_ = writeManifest(j.dir, m)
	}
	close(j.done)
}

// collectorOf: 결과 파일의 종류(이름 끝)와 그 파일을 낸 수집기.
var collectorOf = map[string]string{
	"-openssl.json": "openssl-collector", "-net.json": "network-collector",
	"-jca.jsonl": "jvm-collector", "-cng.json": "cng-collector",
}

// expectedResults: 받은 세대가 지금 결과가 될 만한가. 노드마다 있어야 하는 결과(Linux는 OpenSSL과 네트워크, Windows는
// CNG. JCA는 JVM이 있는 노드에서만 나온다)가 다 왔는지 보고, **받은 파일을 모두 상류 디코더로 읽어** 깨진 것이 없는지,
// 봉투의 노드가 그 별칭의 노드인지, 수집기가 파일 종류와 맞는지 본다. 하나라도 어긋나면 그 까닭을 missing에 적는다
// (세대는 partial로 간다). 빈 파일은 지우고 받은 것으로 치지 않는다.
func expectedResults(dir string, slots []runSlot) (files, missing []string) {
	byAlias := map[string]runSlot{}
	for _, sl := range slots {
		byAlias[sl.Alias] = sl
	}
	ents, _ := os.ReadDir(dir)
	have := map[string]bool{}
	for _, e := range ents {
		name := e.Name()
		fi, err := e.Info()
		if err != nil || strings.HasPrefix(name, ".") {
			continue
		}
		if !fi.Mode().IsRegular() {
			missing = append(missing, name+": not a plain file")
			continue
		}
		if fi.Size() == 0 {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
		files = append(files, name)
		var alias, kind string
		for suf := range collectorOf {
			if strings.HasSuffix(name, suf) {
				alias, kind = strings.TrimSuffix(name, suf), suf
			}
		}
		sl, ok := byAlias[alias]
		if !ok {
			missing = append(missing, name+": not a result this run asked for")
			continue
		}
		label := sl.Node + " (" + strings.TrimPrefix(strings.TrimSuffix(strings.TrimSuffix(kind, ".json"), ".jsonl"), "-") + ")"
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			missing = append(missing, label+": "+err.Error())
			continue
		}
		res, flaws := resultio.Decode(b)
		switch {
		case len(flaws) > 0:
			missing = append(missing, label+": not a collection result ("+flaws[0].Error()+")")
			continue
		case len(res) == 0:
			missing = append(missing, label+": no collection result in it")
			continue
		}
		bad := ""
		for _, r := range res {
			env := r.GetEnvelope()
			if env.GetTargetNodeId() != sl.Node {
				bad = fmt.Sprintf("for node %q, not this node", env.GetTargetNodeId())
				break
			}
			if env.GetCollectorId() != collectorOf[kind] {
				bad = fmt.Sprintf("from %q, not %s", env.GetCollectorId(), collectorOf[kind])
				break
			}
		}
		if bad != "" {
			missing = append(missing, label+": "+bad)
			continue
		}
		have[name] = true
	}
	sort.Strings(files)
	for _, sl := range slots {
		need := []string{"-openssl.json", "-net.json"}
		if sl.OS == inventory.OSWindows {
			need = []string{"-cng.json"}
		}
		for _, suf := range need {
			if !have[sl.Alias+suf] && !isFile(filepath.Join(dir, sl.Alias+suf)) {
				missing = append(missing, sl.Node+" ("+strings.TrimPrefix(strings.TrimSuffix(suf, ".json"), "-")+")")
			}
		}
	}
	sort.Strings(missing)
	return files, missing
}

func writeManifest(dir string, m runManifest) error {
	return writeManifestAs(filepath.Join(dir, runManifestFile), m)
}

func writeManifestAs(p string, m runManifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ── 멈추기 · 끝내기 · 다시 시작 ────────────────────────────────────────────

// stopRun: 돌고 있는 관측을 멈춘다. 프로세스 묶음에 끝내라고 알리고, stopGrace 안에 끝나지 않으면 강제로
// 끝낸다. 정리가 끝났음을 알리는 채널을 돌려준다(돌고 있지 않으면 nil).
func (s *server) stopRun() <-chan struct{} {
	r := &s.run
	r.mu.Lock()
	j := r.job
	if j == nil || j.m.State != ui.RunRunning || j.cmd == nil || j.cmd.Process == nil {
		r.mu.Unlock()
		return nil
	}
	j.stopRequested = true
	cmd, done := j.cmd, j.done
	r.appendLocked("pqcaton: stop requested")
	r.mu.Unlock()
	terminate(cmd)
	go func() {
		select {
		case <-done:
		case <-time.After(stopGrace):
			kill(cmd)
		}
	}()
	return done
}

// shutdownRun: 서버가 끝날 때 부른다. 실행을 멈추고 **정리까지 끝나기를 기다린다** — 먼저 끝나면 Ansible 자식과
// 비밀이 실린 인벤토리가 남는다. 강제로 끝낸 뒤에도 마냥 기다리지는 않는다(그때 남은 것은 다음 시작이 지운다).
func (s *server) shutdownRun() {
	done := s.stopRun()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(stopGrace + cleanupLimit + 5*time.Second):
		fmt.Fprintln(os.Stderr, "observation: did not finish cleaning up; the next start removes what is left")
	}
}

// recoverRuns: 시작할 때 부른다. 화면이 실행 도중에 비정상으로 끝나 남은 자리를 마친다.
//
//  1. 감독이 아직 살아 있으면(기록의 PID와 시작 시각이 같으면) 마무리를 하고 있는 것이다. 끝나기를 기다리고, 너무
//     오래 걸리면 끝내라고 알리고, 그래도 남으면 강제로 끝낸다. 감독의 기록이 없으면 Ansible도 뜬 적이 없다.
//  2. 이 실행의 표지를 가진 프로세스를 모두 찾아 끝낸다(감독이 강제로 죽어 Ansible이나 그 자식이 남은 경우).
//     **끝내지 못하면 아무것도 지우거나 옮기지 않는다** — 그 프로세스가 아직 실행 자리를 쓸 수 있다. 「회수 필요」로
//     적고 새 관측 시작을 막으며, 화면이 PID와 스테이징 폴더를 보인다.
//  3. 감독이 이미 마무리했으면 그대로 둔다. 아니면 results/와 맞바꾸던 중이었는지 가려 마치거나, 남은 인벤토리로
//     노드 정리를 시도하고, 인벤토리를 지우고, 받던 것을 partial/로 격리하고, 「중단됨」으로 적는다.
//
// 가장 최근 실행을 화면에 보일 상태로 둔다.
func (s *server) recoverRuns() {
	ents, err := os.ReadDir(s.runsDir())
	if err != nil {
		return
	}
	var last *runJob
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(s.runsDir(), e.Name())
		m := readManifest(dir, e.Name())
		logf := func(line string) {
			fmt.Fprintf(os.Stderr, "observation %s: %s\n", m.ID, line)
			appendRunLog(dir)(line)
		}
		_, invErr := os.Stat(filepath.Join(dir, runInventory))
		switch {
		case m.State == ui.RunDone && m.PreviousPending:
			if err := finishSwap(dir); err == nil {
				m.PreviousPending = false
				_ = writeManifest(dir, m)
				logf("pqcaton: the earlier results are now kept in previous/")
			}
		case m.State == ui.RunRunning || m.State == ui.RunStuck || invErr == nil:
			waitSupervisor(dir, logf)
			mark := runMark{Marker: runMarker(m.ID)}
			if rec := readSupervisor(dir); rec != nil {
				mark.Groups = []int{rec.PGID}
			}
			if left, err := findMarked(mark); err == nil && len(left) > 0 {
				var pids []int
				for _, p := range left {
					pids = append(pids, p.PID)
				}
				logf(fmt.Sprintf("pqcaton: stopping processes %v left by the interrupted run", pids))
			}
			stuck, err := reapMarked(mark, stopGrace)
			if why := reaped(stuck, err); why != "" {
				m.State, m.Stuck, m.StuckWhy = ui.RunStuck, stuck, why
				_ = writeManifest(dir, m)
				logf("pqcaton: " + why + "; nothing was cleaned up. Stop them, then restart the screen")
				break
			}
			m = readManifest(dir, e.Name()) // 감독이 마무리했으면 그 기록을 따른다
			_, invErr = os.Stat(filepath.Join(dir, runInventory))
			if m.State != ui.RunRunning && m.State != ui.RunStuck && invErr != nil {
				break
			}
			m.Stuck, m.StuckWhy = nil, ""
			if cur := s.currentRun(); m.Swapping && cur != nil && cur.ID == m.ID {
				// 교환은 끝났다. 새 결과가 지금 결과다.
				m.State, m.Swapping = ui.RunDone, false
				if err := finishSwap(dir); err != nil {
					m.PreviousPending = true
				}
				logf("pqcaton: the screen stopped right after the results became current; finished keeping the earlier results")
			} else {
				m.Swapping = false
				if invErr == nil {
					cleaned, why := runCleanup(dir, m.ID, logf)
					if why != "" {
						m.State, m.StuckWhy = ui.RunStuck, why
						_ = writeManifest(dir, m)
						logf("pqcaton: " + why + "; nothing was cleaned up. Stop them, then restart the screen")
						break
					}
					m.NodeCleanup = cleaned
				} else if m.NodeCleanup == "" {
					m.NodeCleanup = "failed"
				}
				if isDir(filepath.Join(dir, "next")) {
					_ = os.Rename(filepath.Join(dir, "next"), filepath.Join(dir, "partial"))
				}
				m.State = ui.RunInterrupted
				logf("pqcaton: interrupted when the screen stopped; the inventory was removed")
			}
			removeRunSecrets(dir)
			_ = writeManifest(dir, m)
		}
		if last == nil || m.Started.After(last.m.Started) {
			j := &runJob{m: m, dir: dir, done: make(chan struct{})}
			close(j.done)
			if b, err := os.ReadFile(filepath.Join(dir, runLog)); err == nil {
				lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
				if len(lines) > maxLogLines {
					j.dropped, lines = len(lines)-maxLogLines, lines[len(lines)-maxLogLines:]
				}
				j.log = lines
			}
			last = j
		}
	}
	s.run.mu.Lock()
	s.run.job = last
	s.run.mu.Unlock()
}

// readManifest: 실행 기록. 읽지 못하면 이름만 채운 빈 기록이다.
func readManifest(dir, name string) runManifest {
	var m runManifest
	if b, err := os.ReadFile(filepath.Join(dir, runManifestFile)); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	if m.ID == "" {
		m.ID = name
	}
	return m
}

// supervisorWait: 살아 있는 감독이 스스로 마무리하기를 기다리는 시간(노드 정리 시간을 넉넉히 준다). 시험이 줄인다.
var supervisorWait = stopGrace + cleanupLimit + 10*time.Second

// waitSupervisor: 기록의 감독이 살아 있으면(PID와 시작 시각이 같으면) 끝나기를 기다리고, 그래도 남으면 끝내라고 알리고,
// 마지막에는 강제로 끝낸다. 같은 PID가 다른 프로세스에 다시 쓰였으면 건드리지 않는다.
func waitSupervisor(dir string, logf func(string)) {
	rec := readSupervisor(dir)
	if rec == nil || rec.PID <= 0 || rec.Start == "" {
		return
	}
	alive := func() bool { cur, err := procStart(rec.PID); return err == nil && cur == rec.Start }
	if !alive() {
		return
	}
	logf(fmt.Sprintf("pqcaton: the supervisor (pid %d) of the interrupted run is still finishing; waiting for it", rec.PID))
	for _, step := range []struct {
		signal bool
		force  bool
		limit  time.Duration
	}{{false, false, supervisorWait}, {true, false, stopGrace}, {true, true, 5 * time.Second}} {
		if step.signal {
			_ = signalProc(markedProc{PID: rec.PID, Start: rec.Start}, step.force)
		}
		deadline := time.Now().Add(step.limit)
		for alive() && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
		}
		if !alive() {
			return
		}
	}
}

// reapMarked: 화면 쪽에서 표지 프로세스를 끝내는 함수. 시험이 끝내지 못하는 경우를 주입한다.
var reapMarked = killMarked

// supervisorExe: 감독으로 띄울 바이너리(지금 도는 이것). 시험이 바꾼다.
var supervisorExe = os.Executable

// currentRun: results/가 지금 어느 실행의 결과인가. 화면에서 관측을 돌린 적이 없으면 nil이다.
func (s *server) currentRun() *runManifest {
	b, err := os.ReadFile(filepath.Join(s.results, currentMark))
	if err != nil {
		return nil
	}
	var m runManifest
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return &m
}

func runID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b), nil
}
