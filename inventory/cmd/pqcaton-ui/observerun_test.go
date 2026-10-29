// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// fakeAnsible: ansible-playbook 대신 부를 스크립트. 화면이 넘긴 인자 그대로 받아 vars.json에서 결과 자리를 읽고,
// 인벤토리의 별칭(n1, n2…)마다 모드(FAKE_MODE)대로 결과를 쓰고 끝난다. 받은 인벤토리의 권한을 로그로 남기고,
// 비밀번호가 든 줄을 **일부러 그대로 찍는다**(화면이 가리는지 본다).
const fakeAnsible = `#!/bin/sh
# orphan: 세션을 떼어 나간(가능하면) 자식을 남긴다. 시험 바이너리를 쓴다 — /bin/sleep은 macOS에서 환경이 안 보이는
# 플랫폼 바이너리라 표지로 증명되지 않는다(그 경우는 IC-U138이 따로 잰다).
orphan() {
  if command -v setsid >/dev/null; then setsid "$FAKE_HELPER" -test-sleep >/dev/null 2>&1 &
  else "$FAKE_HELPER" -test-sleep >/dev/null 2>&1 & fi
}
[ "$1" = "-i" ] && [ "$2" = "targets.yml" ] && [ "$3" = "-e" ] && [ "$4" = "@vars.json" ] || { echo "bad args: $*"; exit 9; }
if [ "$5" = "cleanup.yml" ]; then
  grep -q 'state: absent' cleanup.yml && echo "CLEANUP ran"
  if [ "$FAKE_CLEANUP" = "orphan" ]; then orphan; fi
  exit 0
fi
[ "$5" = "observe.yml" ] || { echo "bad playbook: $5"; exit 9; }
rd=$(sed -n 's/.*"results_dir":"\([^"]*\)".*/\1/p' vars.json)
win=$(sed -n 's/.*"netcap_window":\([0-9]*\).*/\1/p' vars.json)
echo "PLAY [pqcaton observation] window=$win"
echo "INI_MODE=$(stat -c %a targets.yml 2>/dev/null || stat -f %Lp targets.yml)"
grep 'ansible_ssh_pass' targets.yml
grep -q 'name: pqcaton observation' observe.yml && echo "PLAYBOOK=embedded"
echo "SSH_ARGS=$ANSIBLE_SSH_ARGS MARKED=${PQCATON_RUN:+yes}"
if [ "$FAKE_MODE" = "escape" ]; then orphan; fi
res() { printf '{"envelope":{"collectorId":"%s","targetNodeId":"%s"}}' "$1" "$2"; }
for a in $(sed -n 's/^    \(n[0-9]*\):$/\1/p' targets.yml); do
  node=$(sed -n "/^    $a:\$/{n;s/.*pqcaton_node: !unsafe '\(.*\)'\$/\1/p;}" targets.yml)
  res openssl-collector "$node" > "$rd/$a-openssl.json"
  case "$FAKE_MODE" in
  fail) echo "fatal: [$a]: UNREACHABLE!"; exit 2 ;;
  hang) echo "HANGING"; sleep 300 & wait ;;
  badjson) echo 'ERROR: collector crashed' > "$rd/$a-openssl.json" ;;
  wrongnode) res openssl-collector other-node > "$rd/$a-openssl.json" ;;
  wrongcollector) res network-collector "$node" > "$rd/$a-openssl.json" ;;
  esac
  [ "$FAKE_MODE" = "incomplete" ] && [ "$a" = "n2" ] && continue
  res network-collector "$node" > "$rd/$a-net.json"
done
: > "$rd/n1-jca.jsonl"
echo "PLAY RECAP"
`

// oldResult: 화면이 관측을 돌리기 전부터 results/에 있던 결과(직접 넣은 것).
const oldResult = "old-openssl.json"

// runReady: 관측을 시작할 수 있게 갖춘 서버. 가짜 ansible-playbook과 Linux 수집기 폴더, 이전 결과 하나를 둔다.
func runReady(t *testing.T, mode string) *server {
	t.Helper()
	s := everyScreen(t)
	bin := t.TempDir()
	fake := filepath.Join(bin, "ansible-playbook")
	if err := os.WriteFile(fake, []byte(fakeAnsible), 0o755); err != nil {
		t.Fatal(err)
	}
	old := ansibleCommand
	ansibleCommand = fake
	t.Setenv("FAKE_MODE", mode)
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_HELPER", helper)
	s.collectors = t.TempDir()
	linux := filepath.Join(s.collectors, "linux-amd64")
	if err := os.MkdirAll(linux, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"pqcota-nodescan", "pqcota-netcap", "pqcota-jvmscan"} {
		if err := os.WriteFile(filepath.Join(linux, n), []byte("bin"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.results, oldResult), []byte(`{"envelope":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.shutdownRun()
		ansibleCommand = old
	})
	return s
}

// waitRun: 실행이 끝나고 정리까지 마치기를 기다린다(최대 15초).
func waitRun(t *testing.T, s *server) *ui.RunStatus {
	t.Helper()
	s.run.mu.Lock()
	j := s.run.job
	s.run.mu.Unlock()
	if j == nil {
		t.Fatal("nothing ran")
	}
	select {
	case <-j.done:
	case <-time.After(15 * time.Second):
		t.Fatal("the observation did not finish")
	}
	return s.run.status()
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// runDir: 마지막 실행의 작업 자리.
func runDir(t *testing.T, s *server) string {
	t.Helper()
	s.run.mu.Lock()
	defer s.run.mu.Unlock()
	return s.run.job.dir
}

// IC-U116 — **완전히 성공한 관측만 지금 결과가 되고, 한 번에 바뀐다.** 「관측 시작」은 소유자 전용 인벤토리(접근
// 비밀이 실린다)와 내장 플레이북을 실행 자리에 쓰고 정해진 인자로 부른다. 모든 노드의 결과가 오고 0으로 끝나면 받은
// 세대(빈 파일은 빼고)와 results/를 맞바꾼다. 이전 결과는 섞이지 않고 실행 자리의 previous/로 간다. 인벤토리는
// 지우고, 기록과 로그는 남는다. 로그에 비밀번호가 찍혀도 화면과 로그 파일에는 가려져 있다.
func TestObservationRunsAndHandsOverResults(t *testing.T) {
	s := runReady(t, "ok")
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	if !strings.Contains(page, `action="`+ui.RunPath+`"`) {
		t.Fatalf("the observe screen has no start form:\n%s", page)
	}
	if err := os.Chmod(s.results, 0o755); err != nil {
		t.Fatal(err)
	}
	w := postForm(t, s, ui.RunPath, url.Values{"window": {"7"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("start = %d\n%s", w.Code, w.Body.String())
	}
	st := waitRun(t, s)
	if st.State != ui.RunDone || st.Received != 4 {
		t.Fatalf("state %s, received %d\n%s", st.State, st.Received, st.Log)
	}
	for _, want := range []string{"window=7", "INI_MODE=600", "PLAYBOOK=embedded", "PLAY RECAP", "ansible_ssh_pass: !unsafe '********'",
		"SSH_ARGS=-C -o ControlMaster=no -o ControlPersist=no MARKED=yes"} {
		if !strings.Contains(st.Log, want) {
			t.Errorf("the log lacks %q:\n%s", want, st.Log)
		}
	}
	dir := runDir(t, s)
	// 이전 결과가 넓은 권한이어도 상태 폴더의 검사에 걸리지 않게, 실행 자리는 상태 폴더 밖 results/ 옆이다.
	if strings.HasPrefix(dir, s.st.Dir) || filepath.Dir(filepath.Dir(dir)) != filepath.Dir(s.results) {
		t.Errorf("the run directory %s is not beside results/ or sits in the state directory", dir)
	}
	logFile := readFile(t, filepath.Join(dir, runLog))
	if strings.Contains(st.Log, "s3cret-pw") || strings.Contains(logFile, "s3cret-pw") {
		t.Error("the password shows in the log")
	}
	want := []string{currentMark, "n1-net.json", "n1-openssl.json", "n2-net.json", "n2-openssl.json"}
	if got := names(t, s.results); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("results/ = %v, want %v", got, want)
	}
	if got := names(t, filepath.Join(dir, "previous")); strings.Join(got, ",") != oldResult {
		t.Errorf("previous/ = %v, want the earlier result", got)
	}
	if got := names(t, dir); strings.Join(got, ",") != strings.Join([]string{runLog, runManifestFile, "previous", supervisorFile}, ",") {
		t.Errorf("the run directory keeps %v (the inventory must be gone)", got)
	}
	if cur := s.currentRun(); cur == nil || cur.ID != st.ID || len(cur.Nodes) != 2 || cur.Nodes[1].Node != "db" {
		t.Errorf("the current mark = %+v", cur)
	}
	frag := get(t, s, ui.RunPath+"?lang=ko").Body.String()
	if !strings.Contains(frag, "파일 4개가 지금 결과가 됐습니다") || !strings.Contains(frag, ui.ScreenSurvey) || strings.Contains(frag, "every 2s") {
		t.Errorf("the finished fragment:\n%s", frag)
	}
}

// IC-U117 — **갖출 것이 빠졌으면 시작하지 않는다.** ansible-playbook을 찾지 못하거나 수집기 폴더가 없으면 03 화면의
// 점검 목록이 무엇이 빠졌고 무엇을 하면 되는지 두 말로 적고 시작 양식을 내지 않는다. 주소로 직접 보내도 409
// (`run-not-ready`)이고 아무것도 실행하지 않는다.
func TestObservationNeedsItsTools(t *testing.T) {
	for _, c := range []struct{ lang, ansible, folder string }{
		{"ko", "이 기계에 Ansible을 설치하십시오", "작업 폴더에 collectors 폴더를 두거나"},
		{"en", "Install Ansible on this machine", "Put a collectors folder in the workspace"},
	} {
		s := runReady(t, "ok")
		ansibleCommand = "pqcaton-no-such-ansible"
		s.collectors = filepath.Join(t.TempDir(), "missing")
		page := get(t, s, ui.ScreenObserve+"?lang="+c.lang).Body.String()
		for _, want := range []string{c.ansible, c.folder} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: the checklist does not say %q", c.lang, want)
			}
		}
		if strings.Contains(page, `action="`+ui.RunPath+`"`) {
			t.Errorf("%s: a start form is shown although tools are missing", c.lang)
		}
		w := postFormLang(t, s, ui.RunPath, c.lang, url.Values{})
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedRunNotReady) {
			t.Errorf("%s: start = %d, want 409 %s", c.lang, w.Code, ui.RefusedRunNotReady)
		}
		if s.run.status() != nil {
			t.Errorf("%s: something ran", c.lang)
		}
	}
}

// IC-U118 — **관측은 한 번에 하나이고, 멈춘 관측은 지금 결과를 건드리지 않는다.** 돌고 있는 동안 다시 시작하면
// 409(`run-busy`)다. 「중지」는 프로세스 묶음을 끝내고, 그때까지 받은 것은 실행 자리의 partial/로 격리한다.
// results/는 그대로다. 노드에 남았을 스테이징 폴더는 정리 플레이북으로 한 번 더 지운다. 관측 구간이 범위 밖이면 400이다.
func TestObservationRunsOnceAndStops(t *testing.T) {
	s := runReady(t, "hang")
	if w := postForm(t, s, ui.RunPath, url.Values{"window": {"4"}}); w.Code != http.StatusBadRequest {
		t.Errorf("window 4 = %d, want 400", w.Code)
	}
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("start = %d", w.Code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(s.run.status().Log, "HANGING") {
		if time.Now().After(deadline) {
			t.Fatalf("the fake never started:\n%s", s.run.status().Log)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if frag := get(t, s, ui.RunPath).Body.String(); !strings.Contains(frag, "every 2s") || !strings.Contains(frag, ui.RunStopPath) {
		t.Errorf("the running fragment does not poll or offer a stop:\n%s", frag)
	}
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedRunBusy) {
		t.Errorf("second start = %d, want 409 %s", w.Code, ui.RefusedRunBusy)
	}
	if w := postForm(t, s, ui.RunStopPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("stop = %d", w.Code)
	}
	st := waitRun(t, s)
	if st.State != ui.RunStopped || st.Received != 1 {
		t.Errorf("after stop: state %s, received %d\n%s", st.State, st.Received, st.Log)
	}
	if got := names(t, s.results); strings.Join(got, ",") != oldResult {
		t.Errorf("a stopped run changed results/: %v", got)
	}
	if got := names(t, filepath.Join(runDir(t, s), "partial")); strings.Join(got, ",") != "n1-openssl.json" {
		t.Errorf("partial/ = %v", got)
	}
	if !strings.Contains(st.Log, "CLEANUP ran") {
		t.Errorf("a stopped run did not clean the nodes:\n%s", st.Log)
	}
}

// IC-U119 — **실패하거나 빠진 노드가 있는 관측은 지금 결과가 되지 않는다.** ansible-playbook이 0이 아닌 코드로
// 끝나면 「실패」이고 종료 코드와 로그가 보인다. 0으로 끝났어도 어느 노드의 결과가 빠졌으면 「불완전」이고 빠진 노드와
// 무엇이 빠졌는지가 보인다. 두 경우 모두 results/는 그대로이고 받은 것은 partial/로 격리하며 인벤토리는 지운다.
func TestFailedObservationLeavesResultsAlone(t *testing.T) {
	for _, c := range []struct {
		mode  string
		state ui.RunState
		frag  string
	}{
		{"fail", ui.RunFailed, "종료 코드 2"},
		{"incomplete", ui.RunIncomplete, "db (net)"},
	} {
		s := runReady(t, c.mode)
		if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
			t.Fatalf("%s: start = %d", c.mode, w.Code)
		}
		st := waitRun(t, s)
		if st.State != c.state {
			t.Errorf("%s: state %s\n%s", c.mode, st.State, st.Log)
		}
		if frag := get(t, s, ui.RunPath+"?lang=ko").Body.String(); !strings.Contains(frag, c.frag) || strings.Contains(frag, ui.ScreenSurvey) {
			t.Errorf("%s: the fragment does not show %q or still points at reconciliation:\n%s", c.mode, c.frag, frag)
		}
		if got := names(t, s.results); strings.Join(got, ",") != oldResult {
			t.Errorf("%s: results/ changed: %v", c.mode, got)
		}
		dir := runDir(t, s)
		if _, err := os.Stat(filepath.Join(dir, runInventory)); err == nil {
			t.Errorf("%s: the inventory was left", c.mode)
		}
		if !isDir(filepath.Join(dir, "partial")) {
			t.Errorf("%s: nothing was kept for diagnosis", c.mode)
		}
		if !strings.Contains(st.Log, "CLEANUP ran") {
			t.Errorf("%s: the nodes were not cleaned", c.mode)
		}
	}
}

// IC-U123 — **접근 정보의 값은 인벤토리의 문법이 되지 않는다.** 노드 ID가 `cmdb://web-1`처럼 기호를 담고, 이름 ·
// 계정 · 키 경로에 공백 · 작은따옴표 · `{{ }}`가 있어도 인벤토리의 호스트 이름은 별칭(n1)이고, 값은 모두 !unsafe
// 문자열(작은따옴표는 겹쳐 적는다)이라 한 줄을 벗어나거나 템플릿으로 풀리지 않는다. 줄바꿈 · 탭 · YAML이 줄바꿈으로
// 읽는 U+0085 · U+2028 · U+2029는 노드 ID · 계정 · 키 경로 · 비밀번호 어디에 있어도 인벤토리가 되지 않고, 화면이 저장하지
// 않으며(422), 손으로 넣은 파일이면 실행 전 점검이 막는다.
func TestHostValuesStayValues(t *testing.T) {
	hosts := []inventory.Host{{NodeID: "cmdb://web-1", Name: "x ansible_become_pass=y", IP: "10.0.0.2", Port: 22,
		SSHUser: "o'neil", SSHKey: "/k/{{ lookup('pipe','id') }}", SSHPass: "p w", OS: inventory.OSLinux, Conn: inventory.ConnSSH}}
	slots, b, err := renderTargets(hosts)
	if err != nil {
		t.Fatal(err)
	}
	inv := string(b)
	for _, want := range []string{
		"\n    n1:\n",
		"      pqcaton_node: !unsafe 'cmdb://web-1'\n",
		"      ansible_user: !unsafe 'o''neil'\n",
		"      ansible_ssh_private_key_file: !unsafe '/k/{{ lookup(''pipe'',''id'') }}'\n",
		"      ansible_ssh_pass: !unsafe 'p w'\n",
	} {
		if !strings.Contains(inv, want) {
			t.Errorf("the inventory lacks %q:\n%s", want, inv)
		}
	}
	if strings.Contains(inv, "ansible_become_pass") || len(slots) != 1 || slots[0].Alias != "n1" || slots[0].Node != "cmdb://web-1" {
		t.Errorf("slots %+v, inventory:\n%s", slots, inv)
	}
	for _, h := range []inventory.Host{
		{NodeID: "web\nevil ansible_host=1.2.3.4", IP: "10.0.0.2", Port: 22},
		{NodeID: "web\u2028evil: x", IP: "10.0.0.2", Port: 22},
		{NodeID: "web", IP: "10.0.0.2", Port: 22, SSHUser: "a\u2029b"},
		{NodeID: "web", IP: "10.0.0.2", Port: 22, SSHKey: "/k\u0085x"},
		{NodeID: "web", IP: "10.0.0.2", Port: 22, SSHPass: "p\u2028w"},
	} {
		if _, _, err := renderTargets([]inventory.Host{h}); err == nil {
			t.Errorf("a line separator in %+v became an inventory", h)
		}
	}

	s := runReady(t, "ok")
	before := readFile(t, s.hosts)
	for _, id := range []string{"web\tx", "web\u2028x", "web\u2029x"} {
		w := postForm(t, s, ui.HostsSavePath, withRev(url.Values{"host.id.0": {id}, "host.ip.0": {"10.0.0.2"}}, s.revFor(ui.ScreenObserve)))
		if w.Code != http.StatusUnprocessableEntity || readFile(t, s.hosts) != before {
			t.Errorf("%q in a node ID: %d, file changed %v", id, w.Code, readFile(t, s.hosts) != before)
		}
	}
	if err := os.WriteFile(s.hosts, []byte("node_id,ip\n\"we\nb\",10.0.0.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusConflict {
		t.Errorf("a hand-edited line break: start = %d, want 409", w.Code)
	}
}

// IC-U124 — **화면이 끝나면 관측도 끝나고, 비밀이 남지 않는다.** 서버가 끝날 때 돌던 관측을 멈추고 정리가 끝나기를
// 기다린다(인벤토리가 지워진 뒤에 돌아온다). 서버가 비정상으로 끝나 실행 자리가 남았으면, 다음 시작이 인벤토리를
// 지우고 받던 것을 partial/로 옮기고 「중단됨」으로 적으며, 03 화면이 그것을 보인다.
func TestShutdownAndRestartCleanUp(t *testing.T) {
	s := runReady(t, "hang")
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("start = %d", w.Code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(s.run.status().Log, "HANGING") {
		if time.Now().After(deadline) {
			t.Fatal("the fake never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	dir := runDir(t, s)
	s.shutdownRun()
	if _, err := os.Stat(filepath.Join(dir, runInventory)); err == nil {
		t.Error("shutdown returned before the inventory was removed")
	}
	if st := s.run.status(); st.State != ui.RunStopped {
		t.Errorf("after shutdown: %s", st.State)
	}

	// 비정상 종료가 남긴 자리.
	crashed := filepath.Join(s.runsDir(), "20261029T000000-000000000000")
	if err := os.MkdirAll(filepath.Join(crashed, "next"), 0o700); err != nil {
		t.Fatal(err)
	}
	m := runManifest{ID: "20261029T000000-000000000000", State: ui.RunRunning, Started: time.Now().Add(time.Hour)}
	if err := writeManifest(crashed, m); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{runInventory: "ansible_ssh_pass: !unsafe 's3cret-pw'\n", "next/n1-openssl.json": "{}"} {
		if err := os.WriteFile(filepath.Join(crashed, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s.recoverRuns()
	if _, err := os.Stat(filepath.Join(crashed, runInventory)); err == nil {
		t.Error("the leftover inventory was not removed")
	}
	if !isFile(filepath.Join(crashed, "partial", "n1-openssl.json")) {
		t.Error("what arrived before the crash was not kept in partial/")
	}
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	if !strings.Contains(page, "지난 관측은 화면이 도중에 끝나 중단됐습니다") {
		t.Errorf("the observe screen does not say the last run was interrupted")
	}
	if got := names(t, s.results); strings.Join(got, ",") != oldResult {
		t.Errorf("recovery changed results/: %v", got)
	}
}

// dirswap가 이 플랫폼에서 실제로 맞바꾸는지(Linux renameat2 · macOS renamex_np).
func TestDirSwapExchanges(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "a"), filepath.Join(d, "b")
	for p, body := range map[string]string{a: "A", b: "B"} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "f"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := dirswap(a, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(a, "f")); !bytes.Equal(got, []byte("B")) {
		t.Errorf("a/f = %q after the swap", got)
	}
}

// IC-U127 — **결과는 내용까지 맞아야 지금 결과가 된다.** 받은 파일을 모두 상류 디코더로 읽어, 깨진 JSON(수집기 오류
// 문구) · 다른 노드의 결과 · 파일 종류와 다른 수집기의 결과가 하나라도 있으면 「불완전」이고, 어느 노드의 어느 결과가
// 왜 맞지 않는지가 보이며, results/는 그대로이고 받은 것은 partial/로 간다.
func TestResultsMustCheckOut(t *testing.T) {
	for _, c := range []struct{ mode, why string }{
		{"badjson", "web (openssl): not a collection result"},
		{"wrongnode", `web (openssl): for node "other-node", not this node`},
		{"wrongcollector", `web (openssl): from "network-collector", not openssl-collector`},
	} {
		s := runReady(t, c.mode)
		if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
			t.Fatalf("%s: start = %d", c.mode, w.Code)
		}
		st := waitRun(t, s)
		if st.State != ui.RunIncomplete || !strings.Contains(strings.Join(st.Missing, "\n"), c.why) {
			t.Errorf("%s: state %s, missing %v", c.mode, st.State, st.Missing)
		}
		if got := names(t, s.results); strings.Join(got, ",") != oldResult {
			t.Errorf("%s: results/ changed: %v", c.mode, got)
		}
		if !isDir(filepath.Join(runDir(t, s), "partial")) {
			t.Errorf("%s: nothing kept for diagnosis", c.mode)
		}
	}
}

// IC-U128 — **결과를 맞바꾼 순간이 커밋이다.** 맞바꾸기가 실패하면 지금 결과는 그대로이고 받은 것은 partial/로 간다.
// 맞바꾼 뒤 이전 결과를 previous/로 옮기지 못해도 실행은 성공이고 새 결과가 지금 결과이며, 이전 결과의 보관만 「정리
// 필요」로 남아 다음 시작이 마친다. 맞바꾼 직후 화면이 끊긴 자리도 다음 시작이 results/의 기록으로 알아보고 마친다.
func TestSwapIsTheCommitPoint(t *testing.T) {
	t.Run("swap fails", func(t *testing.T) {
		s := runReady(t, "ok")
		old := swapDirs
		swapDirs = func(string, string) error { return errors.New("injected") }
		t.Cleanup(func() { swapDirs = old })
		postForm(t, s, ui.RunPath, url.Values{})
		st := waitRun(t, s)
		if st.State != ui.RunFailed || strings.Join(names(t, s.results), ",") != oldResult {
			t.Errorf("state %s, results %v", st.State, names(t, s.results))
		}
		if !isFile(filepath.Join(runDir(t, s), "partial", "n1-openssl.json")) {
			t.Error("the new results were not kept in partial/")
		}
	})
	t.Run("keeping the earlier results fails", func(t *testing.T) {
		s := runReady(t, "ok")
		old := keepPrevious
		keepPrevious = func(string) error { return errors.New("injected") }
		postForm(t, s, ui.RunPath, url.Values{})
		st := waitRun(t, s)
		keepPrevious = old
		if st.State != ui.RunDone || s.currentRun() == nil || s.currentRun().ID != st.ID {
			t.Fatalf("state %s, current %+v", st.State, s.currentRun())
		}
		dir := runDir(t, s)
		if !isFile(filepath.Join(dir, "next", oldResult)) || isDir(filepath.Join(dir, "partial")) {
			t.Error("the earlier results were misfiled")
		}
		s.recoverRuns()
		if !isFile(filepath.Join(dir, "previous", oldResult)) {
			t.Error("the next start did not finish keeping the earlier results")
		}
	})
	t.Run("stopped right after the swap", func(t *testing.T) {
		s := runReady(t, "ok")
		id := "20261029T000000-aaaaaaaaaaaa"
		dir := filepath.Join(s.runsDir(), id)
		next := filepath.Join(dir, "next")
		if err := os.MkdirAll(next, 0o700); err != nil {
			t.Fatal(err)
		}
		m := runManifest{ID: id, State: ui.RunRunning, Started: time.Now(), Swapping: true,
			Nodes: []runSlot{{Alias: "n1", Node: "web"}}}
		mark := m
		mark.State = ui.RunDone
		if err := writeManifestAs(filepath.Join(next, currentMark), mark); err != nil {
			t.Fatal(err)
		}
		if err := writeManifest(dir, m); err != nil {
			t.Fatal(err)
		}
		if err := dirswap(next, s.results); err != nil {
			t.Fatal(err)
		}
		s.recoverRuns()
		if st := s.run.status(); st.State != ui.RunDone {
			t.Errorf("state after recovery = %s", st.State)
		}
		if !isFile(filepath.Join(dir, "previous", oldResult)) || s.currentRun().ID != id {
			t.Error("the swap was not finished as committed")
		}
	})
}

// IC-U129 — **화면이 비정상으로 끝나도 다음 시작이 남은 실행을 끝낸다.** 감독이 살아 있으면(기록의 PID와 시작 시각이
// 같으면) 마무리하기를 기다리고, 너무 오래 걸리면 끝낸다. 시작 시각이 다르면(PID가 다른 프로세스에 다시 쓰였으면)
// 건드리지 않는다. 그 뒤 이 실행의 표지를 가진 프로세스(감독이 죽어 남은 Ansible과 그 자식)를 모두 끝내고, 남은
// 인벤토리로 노드 정리를 돌리고, 인벤토리를 지우고, partial/로 옮기고, 「중단됨」으로 적는다. 노드에 닿지 못했으면 03
// 화면이 남았을 스테이징 폴더를 실행 ID와 함께 적는다.
func TestRestartReapsAnOrphanedRun(t *testing.T) {
	s := runReady(t, "ok")
	old := supervisorWait
	supervisorWait = 300 * time.Millisecond
	t.Cleanup(func() { supervisorWait = old })
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spawn := func(env ...string) *exec.Cmd {
		cmd := exec.Command(exe, "-test-sleep")
		cmd.Env = append(os.Environ(), env...)
		ownGroup(cmd)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { kill(cmd); _ = cmd.Wait() })
		return cmd
	}
	orphan := func(id string, sup *exec.Cmd, start string, inventory bool) string {
		dir := filepath.Join(s.runsDir(), id)
		if err := os.MkdirAll(filepath.Join(dir, "next"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := writeManifest(dir, runManifest{ID: id, State: ui.RunRunning, Started: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if sup != nil {
			if err := writeDurable(filepath.Join(dir, supervisorFile), supervisorRecord{PID: sup.Process.Pid, PGID: sup.Process.Pid, Start: start}); err != nil {
				t.Fatal(err)
			}
		}
		if inventory {
			for name, body := range map[string]string{runInventory: "targets:\n", "vars.json": `{"run_id":"` + id + `"}`} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
		return dir
	}
	gone := func(cmd *exec.Cmd) bool {
		_, err := procStart(cmd.Process.Pid)
		return err != nil
	}

	id := "20261029T000001-bbbbbbbbbbbb"
	sup := spawn()
	supStart, err := procStart(sup.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	ansible := spawn(runMarker(id))    // 감독이 남긴 Ansible
	stray := spawn(runMarker("other")) // 다른 실행의 프로세스
	dir := orphan(id, sup, supStart, true)
	reused := spawn()
	orphan("20261029T000000-cccccccccccc", reused, "not-its-start", false)

	s.recoverRuns()
	if !gone(sup) || !gone(ansible) {
		t.Errorf("after recovery: supervisor gone %v, marked process gone %v", gone(sup), gone(ansible))
	}
	if gone(reused) || gone(stray) {
		t.Error("a process that was not this run's was signalled")
	}
	m := readManifest(dir, "")
	if m.State != ui.RunInterrupted || m.NodeCleanup != "done" || isFile(filepath.Join(dir, runInventory)) || !isDir(filepath.Join(dir, "partial")) {
		t.Errorf("after recovery: %+v, inventory left %v", m, isFile(filepath.Join(dir, runInventory)))
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, runLog)), "CLEANUP ran") {
		t.Error("the nodes were not cleaned with the leftover inventory")
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, runLog)), fmt.Sprintf("stopping processes [%d]", ansible.Process.Pid)) {
		t.Error("the log does not name the processes recovery stopped")
	}

	orphan("20261029T000002-dddddddddddd", nil, "", false)
	s.recoverRuns()
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	if !strings.Contains(page, "/tmp/pqcaton-observe-20261029T000002-dddddddddddd") {
		t.Error("the screen does not name the staging folder that may be left")
	}
}

// IC-U131 — **화면이 사라지면 감독이 마무리한다.** 감독은 자기를 기록한 뒤에야 Ansible을 띄우고, 화면과 이은 표준 입력이
// 닫히면(화면이 어떻게 끝나든) Ansible과 이 실행의 표지를 가진 프로세스를 모두 끝내고, 노드를 정리하고, 인벤토리를
// 지우고, 받던 것을 partial/로 옮기고, 「중단됨」으로 적은 뒤 끝난다. 다음 시작을 기다리지 않는다.
func TestSupervisorFinishesWhenTheScreenGoes(t *testing.T) {
	s := runReady(t, "hang")
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("start = %d", w.Code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(s.run.status().Log, "HANGING") {
		if time.Now().After(deadline) {
			t.Fatalf("the fake never started:\n%s", s.run.status().Log)
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.run.mu.Lock()
	j := s.run.job
	j.detached = true
	rec := readSupervisor(j.dir)
	s.run.mu.Unlock()
	if rec == nil || rec.PID != j.cmd.Process.Pid || rec.Start == "" {
		t.Fatalf("the supervisor did not record itself before starting Ansible: %+v", rec)
	}
	_ = j.stdin.Close() // 화면이 사라졌다
	select {
	case <-j.done:
	case <-time.After(40 * time.Second):
		t.Fatal("the supervisor did not finish")
	}
	m := readManifest(j.dir, "")
	if m.State != ui.RunInterrupted || m.NodeCleanup != "done" || isFile(filepath.Join(j.dir, runInventory)) || !isDir(filepath.Join(j.dir, "partial")) {
		t.Errorf("after the screen went: %+v, inventory left %v", m, isFile(filepath.Join(j.dir, runInventory)))
	}
	if left, _ := markedProcs(runMark{Marker: runMarker(m.ID)}); len(left) > 0 {
		t.Errorf("processes of the run are left: %v", left)
	}
	if got := names(t, s.results); strings.Join(got, ",") != oldResult {
		t.Errorf("results/ changed: %v", got)
	}
}

// IC-U132 — **끝내지 못한 프로세스가 있으면 아무것도 지우거나 옮기지 않는다.** 회수가 이 실행의 프로세스를 끝내지 못하면
// 노드 정리를 겹쳐 돌리지 않고, 인벤토리를 지우지 않고, 받던 것을 옮기지 않고, 「회수 필요」로 적는다. 03 화면은 그 PID와
// 스테이징 폴더를 보이고, 새 관측은 409(`run-not-ready`)로 시작하지 않는다. 프로세스가 사라진 뒤 다시 시작하면 남은 것을 회수한다.
func TestStuckRunIsLeftAlone(t *testing.T) {
	s := runReady(t, "ok")
	old := reapMarked
	reapMarked = func(runMark, time.Duration) ([]int, error) { return []int{4242}, nil }
	id := "20261029T000003-eeeeeeeeeeee"
	dir := filepath.Join(s.runsDir(), id)
	if err := os.MkdirAll(filepath.Join(dir, "next"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(dir, runManifest{ID: id, State: ui.RunRunning, Started: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, runInventory), []byte("targets:\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.recoverRuns()
	m := readManifest(dir, "")
	if m.State != ui.RunStuck || !isFile(filepath.Join(dir, runInventory)) || !isDir(filepath.Join(dir, "next")) ||
		strings.Contains(readFile(t, filepath.Join(dir, runLog)), "CLEANUP ran") {
		t.Errorf("a stuck run was touched: %+v", m)
	}
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	for _, want := range []string{"4242", "/tmp/pqcaton-observe-" + id, "끝내지 못한 지난 관측", "끝내지 못했거나 확인하지 못했습니다"} {
		if !strings.Contains(page, want) {
			t.Errorf("the observe screen does not show %q", want)
		}
	}
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusConflict {
		t.Errorf("start with a stuck run = %d, want 409", w.Code)
	}
	reapMarked = old
	s.recoverRuns()
	if m := readManifest(dir, ""); m.State != ui.RunInterrupted || isFile(filepath.Join(dir, runInventory)) {
		t.Errorf("after the processes went: %+v", m)
	}
}

// IC-U133 — **관측이 끝나면 그 실행의 프로세스가 하나도 남지 않는다.** 화면이 띄운 관측은 SSH 연결 공유를 쓰지 않고
// (`ControlMaster=no`), Ansible과 그 자식은 실행 표지를 물려받는다. 세션을 떼어 나간 자식이 있어도 감독이 표지로 찾아
// 끝내므로, 정상 종료와 중지 뒤에 표지를 가진 프로세스가 없다.
func TestNothingOfTheRunIsLeft(t *testing.T) {
	for _, stop := range []bool{false, true} {
		s := runReady(t, "escape")
		if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
			t.Fatalf("start = %d", w.Code)
		}
		if stop {
			s.stopRun()
		}
		st := waitRun(t, s)
		if left, _ := markedProcs(runMark{Marker: runMarker(st.ID)}); len(left) > 0 {
			t.Errorf("stop=%v: processes of the run are left: %v", stop, left)
		}
	}
}

// IC-U134 — **프로세스를 확인하지 못한 것은 「남은 것 없음」이 아니다.** 프로세스 목록이나 이 사용자 프로세스의 환경을
// 읽지 못하면 회수는 실패다. 관측을 마칠 때든 다시 시작할 때든 노드 정리 · 인벤토리 삭제 · 결과 이동을 하지 않고 「회수
// 필요」로 닫으며, 화면이 그 까닭을 보이고 새 관측을 시작하지 않는다.
func TestUnknownProcessesAreNotNone(t *testing.T) {
	failing := func(runMark) ([]markedProc, error) { return nil, errors.New("injected: cannot list processes") }
	t.Run("finishing a run", func(t *testing.T) {
		s := runReady(t, "fail")
		old := findMarked
		findMarked = failing
		t.Cleanup(func() { findMarked = old })
		postForm(t, s, ui.RunPath, url.Values{})
		st := waitRun(t, s)
		dir := runDir(t, s)
		if st.State != ui.RunStuck || !strings.Contains(st.StuckWhy, "could not inspect") || !isFile(filepath.Join(dir, runInventory)) ||
			strings.Contains(st.Log, "CLEANUP ran") {
			t.Errorf("state %s, why %q, inventory kept %v", st.State, st.StuckWhy, isFile(filepath.Join(dir, runInventory)))
		}
		if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusConflict {
			t.Errorf("start after an unknown state = %d, want 409", w.Code)
		}
	})
	t.Run("restarting", func(t *testing.T) {
		s := runReady(t, "ok")
		old := findMarked
		findMarked = failing
		t.Cleanup(func() { findMarked = old })
		id := "20261029T000004-ffffffffffff"
		dir := filepath.Join(s.runsDir(), id)
		if err := os.MkdirAll(filepath.Join(dir, "next"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := writeManifest(dir, runManifest{ID: id, State: ui.RunRunning, Started: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, runInventory), []byte("targets:\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		s.recoverRuns()
		if m := readManifest(dir, ""); m.State != ui.RunStuck || !isFile(filepath.Join(dir, runInventory)) || !isDir(filepath.Join(dir, "next")) {
			t.Errorf("an unknown state was treated as none: %+v", m)
		}
		if page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String(); !strings.Contains(page, "확인하지 못했습니다") {
			t.Error("the screen does not say the processes could not be inspected")
		}
	})
}

// IC-U135 — **노드 정리가 남긴 자식도 회수한다.** 정리용 Ansible도 실행 표지를 물려받는다. 정리 플레이북이 세션을 떼어 나간
// 자식을 남기고 0으로 끝나도, 끝난 뒤 그 표지로 찾아 끝낸 다음에야 인벤토리를 지운다. 끝내지 못하면 인벤토리를 지우지
// 않고 「회수 필요」로 닫는다.
func TestCleanupOrphansAreReaped(t *testing.T) {
	t.Setenv("FAKE_CLEANUP", "orphan")
	s := runReady(t, "fail")
	postForm(t, s, ui.RunPath, url.Values{})
	st := waitRun(t, s)
	if st.State != ui.RunFailed || st.NodeCleanup != "done" || isFile(filepath.Join(runDir(t, s), runInventory)) {
		t.Errorf("state %s, cleanup %s, inventory left %v", st.State, st.NodeCleanup, isFile(filepath.Join(runDir(t, s), runInventory)))
	}
	if left, err := markedProcs(runMark{Marker: runMarker(st.ID)}); err != nil || len(left) > 0 {
		t.Errorf("the cleanup left %v (%v)", left, err)
	}

	s = runReady(t, "fail")
	old := reapMarked
	calls := 0
	reapMarked = func(m runMark, d time.Duration) ([]int, error) {
		if calls++; calls >= 2 { // 관측 회수는 지나고, 정리 뒤의 회수가 실패한다
			return []int{4343}, nil
		}
		return old(m, d)
	}
	t.Cleanup(func() { reapMarked = old })
	postForm(t, s, ui.RunPath, url.Values{})
	st = waitRun(t, s)
	// 이 부분은 회수가 실패한 것으로 꾸며 정리의 고아를 일부러 남긴다. 시험이 끝나면 진짜로 거둔다.
	t.Cleanup(func() { _, _ = killMarked(runMark{Marker: runMarker(st.ID)}, time.Second) })
	if st.State != ui.RunStuck || !isFile(filepath.Join(runDir(t, s), runInventory)) {
		t.Errorf("after an unreaped cleanup: state %s, inventory kept %v", st.State, isFile(filepath.Join(runDir(t, s), runInventory)))
	}
}

// IC-U136 — **찾은 뒤 PID가 다른 프로세스에 다시 쓰였으면 신호를 보내지 않는다.** 표지 프로세스는 찾을 때의 시작 시각과
// 함께 잡아 두고, 신호를 보내기 직전에 같은 프로세스인지 다시 본다(Linux는 pidfd를 연 뒤에 본다). 시작 시각이 다른 살아
// 있는 프로세스는 끝내지 않는다.
func TestSignalsGoOnlyToTheProcessFound(t *testing.T) {
	cmd := exec.Command("sleep", "300")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	other := markedProc{PID: cmd.Process.Pid, Start: "not-its-start"}
	if err := signalProc(other, true); err != nil {
		t.Fatal(err)
	}
	old := findMarked
	n := 0
	findMarked = func(runMark) ([]markedProc, error) {
		if n++; n == 1 {
			return []markedProc{other}, nil
		}
		return nil, nil
	}
	t.Cleanup(func() { findMarked = old })
	if _, err := killMarked(runMark{Marker: "PQCATON_RUN=x"}, time.Second); err != nil {
		t.Fatal(err)
	}
	// 신호는 비동기로 닿는다. 보냈다면 죽을 만큼 기다린 뒤에 본다.
	time.Sleep(300 * time.Millisecond)
	if _, err := procStart(cmd.Process.Pid); err != nil {
		t.Error("a process whose PID was reused was signalled")
	}
	start, _ := procStart(cmd.Process.Pid)
	if err := signalProc(markedProc{PID: cmd.Process.Pid, Start: start}, true); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if cmd.ProcessState == nil || cmd.ProcessState.Success() {
		t.Error("the process that was found was not signalled")
	}
}
