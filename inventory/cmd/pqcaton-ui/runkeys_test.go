// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// privateKeys: 시험 서버의 수집기 서명 키(개인 키 값). 이 값이 환경 · 명령줄 · 로그에 없어야 한다.
func privateKeys(t *testing.T, s *server) []string {
	t.Helper()
	var out []string
	for _, id := range neededCollectorIDs(false) {
		k := readCollectorKey(s.keyHome(), s.collectorKeys, id)
		if k.State != ui.KeyOK {
			t.Fatalf("%s: %s", id, k.State)
		}
		out = append(out, k.Private)
	}
	return out
}

func fakeOut(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(os.Getenv("FAKE_OUT"), name))
	if err != nil {
		return ""
	}
	return string(b)
}

func startAndWait(t *testing.T, s *server) *ui.RunStatus {
	t.Helper()
	if err := s.startRun(5); err != nil {
		t.Fatal(err)
	}
	return waitRun(t, s)
}

// IC-U212 — **모든 노드의 점검이 통과해야 키를 만든다.** 감독은 키 없는 점검 플레이북으로 노드마다 `sudo -n -l`을 모아 판정한다.
// 한 노드라도 입출력 기록을 켰거나(거절) 답이 없으면(확인 불가) 임시 키 폴더를 만들지 않고 관측 플레이북을 부르지 않는다. 실행은
// 「실패」이고 지금 결과는 그대로다. 로그는 노드 · 까닭 · 그 정책 줄 · 노드에서 확인할 명령을 보인다.
func TestNoKeyUnlessEveryNodePassesTheCheck(t *testing.T) {
	for _, c := range []struct{ mode, want string }{
		{"sudolog", "node db: refused: sudo records the input and output"},
		{"noanswer", "node db: the sudo check gave no answer"},
	} {
		s := runReady(t, c.mode)
		st := startAndWait(t, s)
		if st.State != ui.RunFailed {
			t.Errorf("%s: state %s, want failed\n%s", c.mode, st.State, st.Log)
		}
		for _, want := range []string{"CHECK ran CONFIG=ansible.cfg", "node web: sudo check passed", c.want, "no signing key was made and the observation did not start"} {
			if !strings.Contains(st.Log, want) {
				t.Errorf("%s: the log lacks %q\n%s", c.mode, want, st.Log)
			}
		}
		if c.mode == "sudolog" && (!strings.Contains(st.Log, "sudo policy: log_input") || !strings.Contains(st.Log, "LC_ALL=C sudo -n -l")) {
			t.Errorf("%s: the policy line or the command to check it is not shown\n%s", c.mode, st.Log)
		}
		if strings.Contains(fakeOut(t, "env.txt"), "BOOK=observe.yml") || fakeOut(t, "keys.txt") != "" {
			t.Errorf("%s: the observation ran or keys were made after a failed check", c.mode)
		}
		if ents, _ := os.ReadDir(runKeyBase()); len(ents) != 0 {
			t.Errorf("%s: something was made in the key folder: %v", c.mode, ents)
		}
		if got := names(t, s.results); !contains(got, oldResult) {
			t.Errorf("%s: the current results changed: %v", c.mode, got)
		}
		// 관측이 노드에서 시작되지 않았으므로 노드 정리는 「불필요」다(실패로 보이지 않는다). 정리 플레이북을 부르지 않는다.
		if st.NodeCleanup != cleanupNotNeeded || strings.Contains(st.Log, "CLEANUP ran") || isFile(filepath.Join(runDir(t, s), runObserveStarted)) {
			t.Errorf("%s: node cleanup %q (want %q, and no cleanup playbook)\n%s", c.mode, st.NodeCleanup, cleanupNotNeeded, st.Log)
		}
		if frag := get(t, s, ui.RunPath+"?lang=ko").Body.String(); !strings.Contains(frag, "노드에서 정리할 것이 없습니다") || strings.Contains(frag, "msg bad") {
			t.Errorf("%s: the fragment does not say there was nothing to clean up:\n%s", c.mode, frag)
		}
	}
	// 이번 판이 아닌 기록(수집기 키 칸이 없다)은 표지가 없어도 시작 여부를 알 수 없으므로 시작한 것으로 본다.
	dir := t.TempDir()
	if !remoteStarted(dir, runManifest{ID: "old"}) || remoteStarted(dir, runManifest{ID: "new", CollectorKeys: map[string]string{idOpenSSL: "/k"}}) {
		t.Error("remoteStarted: an old record must count as started, a new one without the mark as not started")
	}
}

// IC-U213 — **서명 키는 작업 폴더 밖의 임시 파일에서 노드 표준 입력으로만 간다.** 점검이 통과하면 감독이 실행의 임시 키 폴더를
// 작업 폴더 밖에 0700으로, 키 파일을 0600으로 만들고, 관측 플레이북에는 그 경로를 담은 run-keys.json만 넘긴다. Ansible이 받은 환경과
// 명령줄에는 개인 키 값도 PQCOTA_SIGN_KEY도 없고, 환경은 허용 목록(ANSIBLE_* · PQCOTA_* 없이)에 이 실행의 설정(ANSIBLE_CONFIG)뿐이다.
// 서명된 결과는 신뢰할 공개 키로 검증되어(키 지문이 로그에) 지금 결과가 된다. 끝나면 임시 키 폴더와 그 기록이 없다. 로그에도 키가 없다.
func TestSigningKeysTravelOnlyAsTemporaryFiles(t *testing.T) {
	t.Setenv("ANSIBLE_CALLBACKS_ENABLED", "operator.callback")
	t.Setenv("ANSIBLE_CONFIG", "/operator/ansible.cfg")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	t.Setenv("FAKE_ROOT", "n1") // web은 root로 접속한다(sudo를 거치지 않는다)
	s := runReady(t, "ok")
	privs := privateKeys(t, s)
	st := startAndWait(t, s)
	if st.State != ui.RunDone || st.Received != 4 {
		t.Fatalf("state %s, received %d\n%s", st.State, st.Received, st.Log)
	}
	for _, want := range []string{"signature checked (key ", "temporary signing keys for this run are in",
		"node web: passed: the node is reached as root, so the key does not go through sudo", "node db: sudo check passed", "SSH_ARGS=-C -o ControlMaster=no -o ControlPersist=no MARKED=yes"} {
		if !strings.Contains(st.Log, want) {
			t.Errorf("the log lacks %q\n%s", want, st.Log)
		}
	}
	keys := fakeOut(t, "keys.txt")
	for _, want := range []string{"DIRMODE=700", "OPENSSL_MODE=600", "NETWORK_MODE=600"} {
		if !strings.Contains(keys, want) {
			t.Errorf("keys.txt lacks %q:\n%s", want, keys)
		}
	}
	kdir := regexp.MustCompile(`KEYDIR=(\S+)`).FindStringSubmatch(keys)
	if kdir == nil || inside(s.keyHome(), kdir[1]) || !strings.HasPrefix(kdir[1], runKeyBase()) {
		t.Errorf("the key folder %v is inside the workspace or not under the key base", kdir)
	} else if _, err := os.Lstat(kdir[1]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the key folder %s is still there", kdir[1])
	}
	if isFile(filepath.Join(runDir(t, s), runKeysVars)) {
		t.Error("run-keys.json is still in the run directory")
	}
	env := fakeOut(t, "env.txt")
	for _, p := range privs {
		if strings.Contains(env, p) || strings.Contains(st.Log, p) || strings.Contains(readFile(t, filepath.Join(runDir(t, s), runLog)), p) {
			t.Error("a private key value reached the environment, the command line or the log")
		}
	}
	for _, line := range strings.Split(env, "\n") {
		k, _, _ := strings.Cut(line, "=")
		switch {
		case k == "PQCOTA_SIGN_KEY", strings.HasPrefix(k, "PQCOTA_"):
			t.Errorf("the environment carries %s", k)
		case strings.HasPrefix(k, "ANSIBLE_") && k != "ANSIBLE_CONFIG":
			t.Errorf("the environment carries %s", k)
		case k == "ANSIBLE_CONFIG" && !strings.HasSuffix(line, string(filepath.Separator)+runConfigName):
			t.Errorf("ANSIBLE_CONFIG is not this run's configuration: %s", line)
		}
	}
	if !strings.Contains(env, "SSH_AUTH_SOCK=/tmp/agent.sock") {
		t.Error("the ssh agent socket was not passed on")
	}
	if !strings.Contains(env, "BOOK=observe.yml ARGS=-i targets.yml -e @vars.json -e @"+runKeysVars+" observe.yml") {
		t.Errorf("the observation was not given only the key record:\n%s", env)
	}
}

// IC-U214 — **서명 검증을 지나지 못한 결과가 하나라도 있으면 지금 결과를 바꾸지 않는다.** 서명 없음 · 다른 ID의 키(그 키로 검증이
// 성공할 때만 그렇게 말한다) · 서명 뒤 변조(또는 등록되지 않은 키)를 까닭으로 가르고, 받은 세대는 partial/로 가고 이전 결과는 그대로다.
func TestUnverifiedResultsDoNotBecomeCurrent(t *testing.T) {
	for _, c := range []struct{ mode, want string }{
		{"unsigned", "db (openssl): not signed"},
		{"wrongkey", "db (openssl): signed with the key listed for network-collector, not openssl-collector"},
		{"tamper", "db (openssl): the signature does not match the keys listed for openssl-collector (the result was changed after signing, or it was signed with a key that is not listed)"},
	} {
		s := runReady(t, c.mode)
		st := startAndWait(t, s)
		if st.State != ui.RunIncomplete || !contains(st.Missing, c.want) {
			t.Errorf("%s: state %s, missing %q\n%s", c.mode, st.State, st.Missing, st.Log)
		}
		if got := names(t, s.results); strings.Join(got, ",") != oldResult {
			t.Errorf("%s: results/ = %v, want only the earlier result", c.mode, got)
		}
		if !isDir(filepath.Join(runDir(t, s), "partial")) {
			t.Errorf("%s: what arrived was not kept in partial/", c.mode)
		}
	}
}

// IC-U215 — **임시 키를 지우지 못하면 「회수 필요」로 남고 새 관측을 막는다. 재시작 복구가 다시 지운다.** 키 폴더를 지울 수 없게 해 두면
// 감독도 화면도 지우지 못하고, 실행 상태(끝남)와 따로 KeysLeft가 기록된다. 01은 「회수 필요」와 그 자리를 보이고 시작을 받지 않는다.
// 지울 수 있게 되면 다시 시작한 화면의 복구가 지우고 KeysLeft를 비우며, 실행 상태는 끝남 그대로다.
func TestKeysLeftBehindBlockUntilRecovered(t *testing.T) {
	s := runReady(t, "ok")
	base := runKeyBase()
	t.Cleanup(func() { os.Chmod(base, 0o700) })
	// 관측 플레이북이 끝나자마자 키 폴더의 부모를 쓰기 금지로 바꾼다. 감독(다른 프로세스)도 화면도 키 폴더를 지우지 못한다.
	wrap := filepath.Join(t.TempDir(), "ansible-playbook")
	os.WriteFile(wrap, []byte("#!/bin/sh\n\""+ansibleCommand+"\" \"$@\"; rc=$?\ncase \"$*\" in *observe.yml*) chmod 500 \""+base+"\";; esac\nexit $rc\n"), 0o755)
	ansibleCommand = wrap
	st := startAndWait(t, s)
	if st.State != ui.RunStuck || !strings.Contains(st.StuckWhy, "could not be removed") {
		t.Fatalf("state %s, why %q\n%s", st.State, st.StuckWhy, st.Log)
	}
	if c := checkOf(s.preflight(), ui.RunStuckItem); c == nil || c.OK {
		t.Errorf("01 does not show the run that needs reclaiming: %+v", c)
	}
	if err := s.startRun(5); !errors.Is(err, errRunStuck) {
		t.Errorf("a new observation = %v, want errRunStuck", err)
	}
	m := readManifest(runDir(t, s), "")
	if m.State != ui.RunDone || m.KeysLeft == "" {
		t.Errorf("the record = %s / %q, want done with keys left", m.State, m.KeysLeft)
	}
	// 지울 수 있게 되고 화면이 다시 시작한다.
	os.Chmod(base, 0o700)
	s.run.mu.Lock()
	s.run.job = nil
	s.run.mu.Unlock()
	s.recoverRuns()
	m = readManifest(runDir(t, s), "")
	if m.State != ui.RunDone || m.KeysLeft != "" {
		t.Errorf("after recovery the record = %s / %q, want done and no keys left", m.State, m.KeysLeft)
	}
	if ents, _ := os.ReadDir(base); len(ents) != 0 {
		t.Errorf("the key folder is still there after recovery: %v", ents)
	}
	if c := checkOf(s.preflight(), ui.RunStuckItem); c != nil {
		t.Errorf("01 still blocks after recovery: %+v", c)
	}
}

// IC-U216 — **멈춘 관측과 재시작 복구도 임시 키를 지운다.** 관측 플레이북이 도는 중에 멈추면 감독이 키를 지운다. 비정상 종료로 기록과
// 키가 남은 자리는 다음 시작의 복구가 지운다(실행 상태는 건드리지 않는다).
func TestStoppedAndRecoveredRunsRemoveTheKeys(t *testing.T) {
	s := runReady(t, "hang")
	if err := s.startRun(5); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(s.run.status().Log, "HANGING") {
		if time.Now().After(deadline) {
			t.Fatalf("the observation did not reach the playbook\n%s", s.run.status().Log)
		}
		time.Sleep(50 * time.Millisecond)
	}
	<-s.stopRun()
	st := s.run.status()
	if st.State != ui.RunStopped {
		t.Errorf("state %s\n%s", st.State, st.Log)
	}
	if ents, _ := os.ReadDir(runKeyBase()); len(ents) != 0 {
		t.Errorf("keys are left after a stop: %v", ents)
	}
	// 비정상 종료 흉내: 기록과 키 폴더를 남긴다.
	dir := runDir(t, s)
	kd := filepath.Join(runKeyBase(), runKeyPrefix+st.ID)
	os.Mkdir(kd, 0o700)
	os.WriteFile(filepath.Join(kd, idOpenSSL), []byte("secret"), 0o600)
	writeDurable(filepath.Join(dir, runKeysVars), runKeysRecord{Dir: kd, Keys: map[string]string{idOpenSSL: filepath.Join(kd, idOpenSSL)}})
	s.run.mu.Lock()
	s.run.job = nil
	s.run.mu.Unlock()
	s.recoverRuns()
	if _, err := os.Lstat(kd); !errors.Is(err, os.ErrNotExist) || isFile(filepath.Join(dir, runKeysVars)) {
		t.Error("the recovery did not remove the keys and their record")
	}
	if m := readManifest(dir, ""); m.State != ui.RunStopped || m.KeysLeft != "" {
		t.Errorf("the record after recovery = %s / %q", m.State, m.KeysLeft)
	}
	// 기록이 pqcaton이 만들지 않은 폴더를 가리키면 건드리지 않는다.
	other := t.TempDir()
	writeDurable(filepath.Join(dir, runKeysVars), runKeysRecord{Dir: other})
	if why := removeRunKeys(dir); why == "" || !isDir(other) {
		t.Errorf("a record naming %s = %q (the folder must be left and reported)", other, why)
	}
}

// IC-U217 — **01의 플레이북과 설정은 검증한 실행 조건만 쓴다.** 플레이북은 내장 모듈(ansible.builtin.*)만 쓰고, 플레이마다 연결은
// ansible.builtin.ssh, 권한 상승은 ansible.builtin.sudo다. 서명 키는 `stdin`에 내장 file 조회로만 들어가고 그 작업은 no_log이며,
// environment:로도 env 조회로도 넘기지 않는다. 서명 작업의 결과는 stdout · stderr · rc(와 skipped 여부)로만 쓰고, 결과 전체를
// 출력 · 저장하지 않는다(debug · to_json 없음). Windows 작업은 없다. 설정은 모든 플러그인 · 컬렉션 경로를 빈 폴더로 두고, pipelining을
// 켜고, 원격 파일 보존 · 출력 수준 · 인자 표시를 고정하며, 빈 callbacks_enabled를 적지 않는다(2.21이 예외로 멈춘다). 환경은
// ANSIBLE_* · PQCOTA_*를 물려주지 않고 ssh-agent와 언어 설정은 물려준다.
func TestPlaybooksAndConfigUseOnlyVerifiedConditions(t *testing.T) {
	books := map[string]string{"observe.yml": string(observePlaybook), "observe-cleanup.yml": string(cleanupPlaybook), "observe-check.yml": string(checkPlaybook)}
	module := regexp.MustCompile(`(?m)^\s+-?\s*([a-z_]+\.[a-z_]+\.[a-z_]+|[a-z_]+):\s`)
	known := map[string]bool{"name": true, "block": true, "always": true, "register": true, "when": true, "loop": true, "delegate_to": true,
		"become": true, "become_method": true, "become_user": true, "changed_when": true, "failed_when": true, "no_log": true, "environment": true,
		"vars": true, "tasks": true, "hosts": true, "gather_facts": true, "connection": true, "argv": true, "stdin": true, "src": true, "dest": true,
		"mode": true, "stage": true, "arch": true, "iface": true, "sign_wrapper": true, "content": true, "stop_script": true}
	for name, b := range books {
		for _, m := range module.FindAllStringSubmatch(b, -1) {
			if !known[m[1]] && !strings.HasPrefix(m[1], "ansible.builtin.") {
				t.Errorf("%s: %q is not a built-in module or a known keyword", name, m[1])
			}
		}
		if !strings.Contains(b, "connection: ansible.builtin.ssh") {
			t.Errorf("%s: the play does not fix the connection", name)
		}
		for _, bad := range []string{"ansible.windows", "win_", "debug", "to_json", "to_nice_json", "lookup('ansible.builtin.env'", "lookup('env'", "PQCOTA_SIGN_KEY:"} {
			if strings.Contains(b, bad) {
				t.Errorf("%s: carries %q", name, bad)
			}
		}
	}
	obs := string(observePlaybook)
	if !strings.Contains(obs, "become_method: ansible.builtin.sudo") {
		t.Error("observe.yml does not fix become to sudo")
	}
	// 서명 작업: stdin이 있는 작업마다 no_log와 file 조회. 그 register 이름은 .stdout · .stderr · .rc · skipped로만 쓰인다.
	tasks := strings.Split(obs, "\n        - name: ")
	for _, task := range tasks {
		if !strings.Contains(task, "stdin:") {
			continue
		}
		if !strings.Contains(task, "no_log: true") || !strings.Contains(task, "stdin: \"{{ lookup('ansible.builtin.file', pqcaton_keys['") {
			t.Errorf("a signing task without no_log or the file lookup:\n%s", task)
		}
		reg := regexp.MustCompile(`register: (\w+)`).FindStringSubmatch(task)
		if reg == nil {
			t.Errorf("a signing task without register:\n%s", task)
			continue
		}
		uses := regexp.MustCompile(`(?:^|[^-\w])`+reg[1]+`\b(\.\w+| is \w+( \w+)?)?`).FindAllStringSubmatch(obs, -1)
		for _, u := range uses {
			switch u[1] {
			case "", ".stdout", ".stderr", ".rc", " is not skipped", " is skipped":
			default:
				t.Errorf("the result of %s is used as %q", reg[1], u[0])
			}
		}
	}
	cfg := string(ansibleConfig("/run"))
	for _, want := range []string{"callback_plugins = /run/" + runPluginsNone, "connection_plugins = /run/" + runPluginsNone,
		"become_plugins = /run/" + runPluginsNone, "vars_plugins = /run/" + runPluginsNone, "collections_path = /run/" + runPluginsNone,
		"collections_scan_sys_path = False", "stdout_callback = ansible.builtin.default", "display_args_to_stdout = False",
		"keep_remote_files = False", "verbosity = 0", "pipelining = True", "transport = ansible.builtin.ssh", "become_allow_same_user = False"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("the configuration lacks %q", want)
		}
	}
	if strings.Contains(cfg, "callbacks_enabled") {
		t.Error("the configuration names callbacks_enabled")
	}
	t.Setenv("ANSIBLE_STDOUT_CALLBACK", "operator")
	t.Setenv("PQCOTA_DSN", "postgres://secret")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent")
	t.Setenv("LC_ALL", "C")
	env := strings.Join(ansibleEnv("/run", "PQCATON_RUN=x"), "\n")
	for _, bad := range []string{"ANSIBLE_STDOUT_CALLBACK", "PQCOTA_DSN"} {
		if strings.Contains(env, bad) {
			t.Errorf("the environment carries %s", bad)
		}
	}
	for _, want := range []string{"SSH_AUTH_SOCK=/tmp/agent", "LC_ALL=C", "PQCATON_RUN=x", "ANSIBLE_CONFIG=/run/" + runConfigName} {
		if !strings.Contains(env, want) {
			t.Errorf("the environment lacks %s", want)
		}
	}
	v := runVars()
	if v["ansible_become_method"] != "ansible.builtin.sudo" || v["ansible_pipelining"] != true || v["ansible_connection"] != nil {
		t.Errorf("run vars = %v (become fixed, pipelining on, no connection in extra vars)", v)
	}
}

// IC-U218 — **멈추거나 실패한 관측의 정리는 노드에 남은 이 실행의 수집기를 끝낸다.** 컨트롤러가 Ansible과 ssh를 끝내도 pipelining과
// sudo로 띄운 원격 수집기는 제 관측 구간까지 돈다(실험실에서 멈춘 뒤 pqcota-netcap이 root로 남았다). 정리 플레이북은 스테이징 자리를
// 지우기 전에, 명령줄이 이 실행의 스테이징 자리로 시작하는 프로세스를 끝낸다. 그 스크립트를 그대로 돌려 잰다: 이 실행의 자리에서 뜬
// 프로세스는 끝나고, 다른 자리(다른 실행)의 프로세스는 남는다.
func TestCleanupStopsTheCollectorsOfTheRun(t *testing.T) {
	book := string(cleanupPlaybook)
	if strings.Index(book, "Stop the collectors of this run") < 0 || strings.Index(book, "Stop the collectors of this run") > strings.Index(book, "Remove the staging directory") {
		t.Fatal("the cleanup does not stop the collectors before removing the staging directory")
	}
	a := strings.Index(book, "stop_script: |\n")
	b := strings.Index(book, "\n  tasks:")
	var script []string
	for _, l := range strings.Split(book[a+len("stop_script: |\n"):b], "\n") {
		script = append(script, strings.TrimPrefix(l, "      "))
	}
	sleepBin, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep binary")
	}
	start := func(stage string) *exec.Cmd {
		t.Helper()
		if err := os.MkdirAll(stage, 0o700); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(sleepBin)
		bin := filepath.Join(stage, "pqcota-netcap")
		os.WriteFile(bin, b, 0o755)
		c := exec.Command(bin, "60")
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Process.Kill(); _, _ = c.Process.Wait() })
		return c
	}
	root := t.TempDir()
	ours := start(filepath.Join(root, "pqcaton-observe-run1"))
	other := start(filepath.Join(root, "pqcaton-observe-run2"))
	out, err := exec.Command("/bin/sh", "-c", strings.Join(script, "\n"), "pqcaton-cleanup", filepath.Join(root, "pqcaton-observe-run1")).CombinedOutput()
	if err != nil {
		t.Fatalf("the stop script failed: %v\n%s", err, out)
	}
	done := make(chan error, 1)
	go func() { done <- ours.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("the collector of this run is still running")
	}
	if err := other.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("a process of another run was stopped: %v", err)
	}
}
