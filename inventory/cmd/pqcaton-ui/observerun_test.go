// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// fakeAnsible: ansible-playbook 대신 부를 스크립트. 화면이 넘긴 인자 그대로 받아 vars.json에서 결과 자리를 읽고,
// 모드(FAKE_MODE)대로 결과를 쓰고 끝난다. 받은 인벤토리의 권한과 비밀이 실렸는지를 로그로 남긴다.
const fakeAnsible = `#!/bin/sh
[ "$1" = "-i" ] && [ "$2" = "targets.ini" ] && [ "$3" = "-e" ] && [ "$4" = "@vars.json" ] && [ "$5" = "observe.yml" ] || { echo "bad args: $*"; exit 9; }
rd=$(sed -n 's/.*"results_dir":"\([^"]*\)".*/\1/p' vars.json)
win=$(sed -n 's/.*"netcap_window":\([0-9]*\).*/\1/p' vars.json)
echo "PLAY [pqcaton observation] window=$win"
echo "INI_MODE=$(stat -c %a targets.ini 2>/dev/null || stat -f %Lp targets.ini)"
grep -q 'ansible_ssh_pass=s3cret-pw' targets.ini && echo "INI_HAS_SECRET=yes"
grep -q 'name: pqcaton observation' observe.yml && echo "PLAYBOOK=embedded"
printf '{"envelope":{"targetNodeId":"web"}}' > "$rd/web-openssl.json"
case "$FAKE_MODE" in
fail) echo "fatal: [db]: UNREACHABLE!"; exit 2 ;;
hang) sleep 300 ;;
esac
printf '{"envelope":{"targetNodeId":"web"}}' > "$rd/web-net.json"
: > "$rd/db-openssl.json"
echo "PLAY RECAP"
`

// runReady: 관측을 시작할 수 있게 갖춘 서버. 가짜 ansible-playbook과 Linux 수집기 폴더를 둔다.
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
	t.Cleanup(func() {
		s.stopRun()
		waitRun(t, s)
		ansibleCommand = old
	})
	return s
}

// waitRun: 실행이 끝나기를 기다린다(최대 15초).
func waitRun(t *testing.T, s *server) *ui.RunStatus {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for s.run.running() {
		if time.Now().After(deadline) {
			t.Fatal("the observation did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return s.run.status()
}

// IC-U116 — **관측을 화면에서 실행하고 결과를 넘겨받는다.** 「관측 시작」은 내장 플레이북과 소유자 전용 인벤토리
// (접근 비밀이 실린다)를 상태 폴더의 작업 자리에 쓰고 ansible-playbook을 부른다. 끝나면 받은 결과(빈 파일은 빼고)를
// 작업 폴더의 results로 옮기고 작업 자리를 통째로 지운다. 실행 상태 조각은 받은 수와 로그를 보이고, 대조로 가는
// 길을 낸다.
func TestObservationRunsAndHandsOverResults(t *testing.T) {
	s := runReady(t, "ok")
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	if !strings.Contains(page, `action="`+ui.RunPath+`"`) {
		t.Fatalf("the observe screen has no start form:\n%s", page)
	}
	w := postForm(t, s, ui.RunPath, url.Values{"window": {"7"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("start = %d\n%s", w.Code, w.Body.String())
	}
	st := waitRun(t, s)
	if st.State != ui.RunDone || st.Received != 2 {
		t.Fatalf("state %s, received %d\n%s", st.State, st.Received, st.Log)
	}
	for _, want := range []string{"window=7", "INI_MODE=600", "INI_HAS_SECRET=yes", "PLAYBOOK=embedded", "PLAY RECAP"} {
		if !strings.Contains(st.Log, want) {
			t.Errorf("the log lacks %q:\n%s", want, st.Log)
		}
	}
	for _, f := range []string{"web-openssl.json", "web-net.json"} {
		if _, err := os.Stat(filepath.Join(s.results, f)); err != nil {
			t.Errorf("%s was not handed over: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(s.results, "db-openssl.json")); err == nil {
		t.Error("an empty result was handed over")
	}
	if ents, _ := os.ReadDir(filepath.Join(s.st.Dir, "observe")); len(ents) != 0 {
		t.Errorf("the work directory (with the inventory's secrets) was left: %v", ents)
	}
	frag := get(t, s, ui.RunPath+"?lang=ko").Body.String()
	if !strings.Contains(frag, "결과 파일 2개를 받았습니다") || !strings.Contains(frag, ui.ScreenSurvey) || strings.Contains(frag, "every 2s") {
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

// IC-U118 — **관측은 한 번에 하나이고 멈출 수 있다.** 돌고 있는 동안 다시 시작하면 409(`run-busy`)다. 「중지」를
// 누르면 프로세스 묶음을 끝내고 그때까지 받은 결과만 옮기며, 상태는 「멈춤」이다. 관측 구간이 범위 밖이면 400이다.
func TestObservationRunsOnceAndStops(t *testing.T) {
	s := runReady(t, "hang")
	if w := postForm(t, s, ui.RunPath, url.Values{"window": {"4"}}); w.Code != http.StatusBadRequest {
		t.Errorf("window 4 = %d, want 400", w.Code)
	}
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("start = %d", w.Code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(s.run.status().Log, "INI_MODE") {
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
}

// IC-U119 — **실패한 관측도 받은 만큼은 넘기고 실패로 남는다.** ansible-playbook이 0이 아닌 코드로 끝나면 상태는
// 「실패」이고 종료 코드와 로그(어느 노드의 어느 단계인지)가 보인다. 그때까지 받은 결과는 옮기고 작업 자리는 지운다.
func TestFailedObservationKeepsWhatArrived(t *testing.T) {
	s := runReady(t, "fail")
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("start = %d", w.Code)
	}
	st := waitRun(t, s)
	if st.State != ui.RunFailed || st.Exit != 2 || st.Received != 1 || !strings.Contains(st.Log, "UNREACHABLE") {
		t.Errorf("state %s, exit %d, received %d\n%s", st.State, st.Exit, st.Received, st.Log)
	}
	if frag := get(t, s, ui.RunPath+"?lang=ko").Body.String(); !strings.Contains(frag, "종료 코드 2") {
		t.Errorf("the failed fragment does not show the exit code:\n%s", frag)
	}
	if ents, _ := os.ReadDir(filepath.Join(s.st.Dir, "observe")); len(ents) != 0 {
		t.Errorf("the work directory was left: %v", ents)
	}
}
