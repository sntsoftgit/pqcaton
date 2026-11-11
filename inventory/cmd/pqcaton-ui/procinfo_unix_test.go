// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// 실제 프로세스를 띄워 플랫폼의 조회기(Linux 는 /proc, macOS 는 sysctl)로 읽는다. 두 플랫폼에서 같은 시험이 돈다.

//go:build linux || darwin

package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// 「회수 필요」 프로세스 정보(procinfo).

// 인자와 환경에 넣는 비밀. 화면과 정보 어디에도 나오면 안 된다.
var procSecrets = []string{"hunter2", "s3cret", "abc123token", "xyzsignkey", "envsecretvalue"}

// liveProc: 비밀 인자와 비밀 환경 변수를 가진 살아 있는 프로세스. sh 가 sleep 을 기다리는 동안 살아 있다(exec 하지 않게 ; true).
func liveProc(t *testing.T) (*exec.Cmd, ui.ProcRecord) {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "sleep 30; true", "pqcaton-proc-test",
		"--password", "hunter2", "postgres://dbuser:s3cret@db/x", "API_TOKEN=abc123token", "SIGN_KEY=xyzsignkey")
	cmd.Env = append(os.Environ(), "PQCATON_TEST_SECRET=envsecretvalue")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for i := 0; i < 50; i++ { // exec 이 끝나 인자가 바뀔 때까지 잠깐 기다린다
		if f, err := readProc(cmd.Process.Pid); err == nil && strings.Contains(strings.Join(f.Args, " "), "hunter2") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	start, err := procStart(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	return cmd, ui.ProcRecord{PID: cmd.Process.Pid, Start: start}
}

func noSecrets(t *testing.T, what, s string) {
	t.Helper()
	for _, sec := range procSecrets {
		if strings.Contains(s, sec) {
			t.Errorf("%s shows the secret %q", what, sec)
		}
	}
}

// IC-U184 — **남긴 그 프로세스가 살아 있으면 PID · PPID · PGID · 사용자 · 실행 파일 · 시작 시각을 보이고, 인자는 가린 값만,
// 환경 변수는 아예 싣지 않는다.**
func TestStuckProcessAlive(t *testing.T) {
	cmd, rec := liveProc(t)
	got := inspect(rec)
	if got.State != ui.ProcSame || got.PPID != os.Getpid() || got.PGID == 0 || got.User == "" || got.Exe == "" || got.Started == "" {
		t.Fatalf("a live process: %+v", got)
	}
	if !strings.Contains(got.Args, "--password ***") || !strings.Contains(got.Args, "postgres://***@db/x") ||
		!strings.Contains(got.Args, "API_TOKEN=***") || !strings.Contains(got.Args, "SIGN_KEY=***") {
		t.Errorf("the arguments are not masked as expected: %q", got.Args)
	}
	noSecrets(t, "the process details", fmt.Sprintf("%+v", got))
	_ = cmd
}

// IC-U185 — **이미 종료된 프로세스는 「이미 종료됨」이다.** 오류 페이지가 아니다.
func TestStuckProcessGone(t *testing.T) {
	cmd, rec := liveProc(t)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if got := inspect(rec); got.State != ui.ProcGone || got.Args != "" {
		t.Fatalf("an ended process: %+v", got)
	}
}

// IC-U186 — **PID 가 다른 프로세스에 다시 쓰였으면(시작 시각이 다르면) 「다른 프로세스로 바뀜」이고 세부를 싣지 않는다.**
// 회수 대상으로 안내하지 않는다.
func TestStuckProcessReused(t *testing.T) {
	_, rec := liveProc(t)
	rec.Start = "1"
	if got := inspect(rec); got.State != ui.ProcReused || got.Args != "" || got.Exe != "" || got.PPID != 0 {
		t.Fatalf("a reused PID: %+v", got)
	}
}

// IC-U187 — **조회 권한이 없거나 이 사용자의 프로세스가 아니면 「조회 권한 없음」이고, 실행 파일과 인자를 모델에 싣지 않는다.**
// 조회기를 주입해 확정적으로 잰다(조회기가 실행 파일과 인자를 읽었어도 싣지 않는다). 시작 시각을 남기지 않은 옛 기록은 「같은
// 프로세스인지 확인할 수 없음」, 조회가 실패하면 「조회 실패」다.
func TestStuckProcessDeniedUnconfirmedFailed(t *testing.T) {
	old := readProcFn
	t.Cleanup(func() { readProcFn = old })
	rec := ui.ProcRecord{PID: 4242, Start: "777"}
	cases := map[string]struct {
		facts procFacts
		err   error
		want  ui.ProcState
	}{
		"permission denied": {err: fs.ErrPermission, want: ui.ProcDenied},
		"another user's process": {facts: procFacts{Start: "777", UID: os.Getuid() + 1, PPID: 1, Exe: "/usr/bin/secret-exe",
			Args: []string{"tool", "--password", "hunter2"}}, want: ui.ProcDenied},
		"lookup failed": {err: errors.New("i/o error"), want: ui.ProcFailed},
	}
	for name, c := range cases {
		readProcFn = func(int) (procFacts, error) { return c.facts, c.err }
		got := inspect(rec)
		if got.State != c.want || got.Exe != "" || got.Args != "" || got.PPID != 0 || got.User != "" {
			t.Errorf("%s: %+v, want %s with no details", name, got, c.want)
		}
		noSecrets(t, name, fmt.Sprintf("%+v", got))
	}
	readProcFn = old
	if got := inspect(ui.ProcRecord{PID: os.Getpid()}); got.State != ui.ProcUnconfirmed || got.Args != "" {
		t.Fatalf("a record without a start time: %+v", got)
	}
}

// IC-U187 보조 — 이 기계의 PID 1 이 다른 사용자의 것이면 실물로도 「조회 권한 없음」이다. 조건이 맞지 않는 환경에서는 잴 것이
// 없다(핵심 판정은 위의 주입 시험이 확정적으로 잰다).
func TestStuckProcessPID1WhenOthers(t *testing.T) {
	f, err := readProc(1)
	if err != nil {
		t.Skip("PID 1 cannot be read here: " + err.Error())
	}
	if f.UID == os.Getuid() {
		t.Skip("PID 1 belongs to this user here, so it is not another user's process")
	}
	start, err := procStart(1)
	if err != nil {
		t.Skip("PID 1 cannot be read here: " + err.Error())
	}
	if got := inspect(ui.ProcRecord{PID: 1, Start: start}); got.State != ui.ProcDenied || got.Exe != "" || got.Args != "" {
		t.Fatalf("another user's PID 1: %+v", got)
	}
}

// IC-U188 — **인자 가림.** DSN 의 사용자 정보, password= · token= · secret= 꼴, …_KEY= 꼴, 값을 받는 비밀 플래그의 다음 인자,
// PEM 개인 키를 가리고 나머지는 그대로 둔다.
func TestRedactArgs(t *testing.T) {
	got := redactArgs([]string{"ansible-playbook", "-i", "inv.ini", "-e", "ansible_ssh_pass=hunter2", "--vault-password-file", "/k",
		"--token", "abc123token", "mysql://root:s3cret@h/db", "DB_PASSWORD=s3cret", "SIGN_KEY=xyzsignkey", "key=value",
		"-----BEGIN PRIVATE KEY-----", "observe.yml"})
	want := "ansible-playbook -i inv.ini -e ansible_ssh_pass=*** --vault-password-file *** --token *** mysql://***@h/db " +
		"DB_PASSWORD=*** SIGN_KEY=*** key=value [private key redacted] observe.yml"
	if got != want {
		t.Fatalf("redactArgs:\n got  %s\n want %s", got, want)
	}
}

func treeSum(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		fi, _ := d.Info()
		line := p + " " + fi.Mode().String()
		if d.Type().IsRegular() {
			b, _ := os.ReadFile(p)
			line += fmt.Sprintf(" %x", sha256.Sum256(b))
		}
		lines = append(lines, line)
		return nil
	})
	sort.Strings(lines)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(lines, "\n"))))
}

// IC-U189 — **03 화면은 남긴 프로세스의 지금 상태를 보이되, 가린 인자만 HTML 에 싣고 끝내는 단추는 두지 않는다.** 「다시
// 확인」은 다시 읽기만 해서, 두 번 불러도 작업 폴더(실행 기록 · 영수증 · 상태 폴더)가 한 바이트도 바뀌지 않는다.
func TestStuckProcessesOnTheObserveScreen(t *testing.T) {
	s := runReady(t, "ok")
	_, rec := liveProc(t)
	id := "20261111T000001-ffffffffffff"
	dir := filepath.Join(s.runsDir(), id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	m := runManifest{ID: id, State: ui.RunStuck, Started: time.Now(), Stuck: []int{rec.PID, 999999},
		StuckProcs: []stuckProc{{PID: rec.PID, Start: rec.Start}, {PID: 999999, Start: "5"}}, StuckWhy: "processes did not end"}
	if err := writeManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(s.results)
	before := treeSum(t, root)
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	for _, want := range []string{`data-proc-state="same"`, "이 관측이 남긴 프로세스입니다", fmt.Sprintf("PID %d", rec.PID),
		`data-proc-state="gone"`, "인자 보기(비밀값은 가림)", "--password ***", "다시 확인"} {
		if !strings.Contains(page, want) {
			t.Errorf("the observe screen lacks %q", want)
		}
	}
	noSecrets(t, "the observe screen", page)
	section := page[strings.Index(page, `id="run-procs"`):]
	if end := strings.Index(section, "</ul>"); end > 0 && strings.Contains(section[:end], "<form") {
		t.Error("the process list carries a form (a stop button)")
	}
	get(t, s, ui.ScreenObserve+"?lang=en")
	if after := treeSum(t, root); after != before {
		t.Error("checking again changed the workspace")
	}
}

// IC-U191 — **시작 시각을 남기지 않은 이전 판의 기록(PID만 있는 stuck)도 사라지지 않고 「같은 프로세스인지 확인할 수 없음」으로
// 보인다.** 살아 있는 프로세스의 PID여도 세부와 인자를 싣지 않는다.
func TestOldStuckRecordIsUnconfirmed(t *testing.T) {
	s := runReady(t, "ok")
	_, rec := liveProc(t)
	id := "20261111T000002-aaaaaaaaaaaa"
	dir := filepath.Join(s.runsDir(), id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(dir, runManifest{ID: id, State: ui.RunStuck, Started: time.Now(), Stuck: []int{rec.PID}, StuckWhy: "old"}); err != nil {
		t.Fatal(err)
	}
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	if !strings.Contains(page, `data-proc-state="unconfirmed"`) || !strings.Contains(page, fmt.Sprintf("PID %d", rec.PID)) ||
		!strings.Contains(page, "같은 프로세스인지 확인할 수 없음") {
		t.Errorf("an old record is not shown as unconfirmed")
	}
	if strings.Contains(page, "--password") {
		t.Error("an unconfirmed process shows its arguments")
	}
	noSecrets(t, "the observe screen", page)
}
