// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

import (
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// search — 회수가 본 검색 하나(언제, 몇 개를 찾았나).
type search struct {
	at    time.Time
	found int
}

// injectFind: 검색을 바꿔 끼우고 결과를 적는다. answer가 n번째 검색(1부터)의 결과를 정한다.
func injectFind(t *testing.T, answer func(n int, m runMark) ([]markedProc, error)) *[]search {
	t.Helper()
	oldSettle, oldFind := reapSettle, findMarked
	reapSettle = 50 * time.Millisecond
	t.Cleanup(func() { reapSettle, findMarked = oldSettle, oldFind })
	log := &[]search{}
	findMarked = func(m runMark) ([]markedProc, error) {
		ps, err := answer(len(*log)+1, m)
		*log = append(*log, search{time.Now(), len(ps)})
		return ps, err
	}
	return log
}

// endsWithTwoQuietSearches: 마지막 두 검색이 모두 비었고 그 사이가 reapSettle 이상인가.
func endsWithTwoQuietSearches(log []search) bool {
	n := len(log)
	return n >= 2 && log[n-1].found == 0 && log[n-2].found == 0 && log[n-1].at.Sub(log[n-2].at) >= reapSettle
}

// IC-U202 — **회수는 한 번 비었다고 끝내지 않는다.** 검색 결과를 넣어 차례를 정해 둔다. (1) 빈 결과 → 늦게 나타남 → 끝냄:
// 첫 검색에서만 표지가 있는 실제 자식을 숨겨도(exec 도중 환경이 빈 채로 읽힌 경우) 회수는 기다린 뒤 다시 찾아 그 자식을
// 끝내고, 간격을 둔 연속 두 번의 빈 검색으로 마친다. (2) 처음부터 아무것도 없으면 간격을 둔 빈 검색 두 번으로 마친다.
// (3) 끝낼 때마다 다시 나타나면 정해진 횟수 뒤 성공으로 치지 않고 돌려준다(부르는 쪽이 「회수 필요」로 적는다). 걸리는 시간도
// 유한하다. (4) 검색 오류는 「남은 것 없음」이 아니다.
func TestReapNeedsTwoQuietSearches(t *testing.T) {
	t.Run("empty, then late, then ended", func(t *testing.T) {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		marker := runMarker("20261112T000000-reapsettle00")
		cmd := exec.Command(exe, "-test-sleep")
		cmd.Env = append(os.Environ(), marker)
		ownGroup(cmd)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan struct{})
		go func() { _ = cmd.Wait(); close(exited) }()
		t.Cleanup(func() { kill(cmd); <-exited })
		log := injectFind(t, func(n int, m runMark) ([]markedProc, error) {
			if n == 1 {
				return nil, nil // exec 도중이라 표지가 안 보였다
			}
			return markedProcs(m)
		})
		stuck, err := reapRun(runMark{Marker: marker}, time.Second)
		if err != nil || len(stuck) != 0 {
			t.Fatalf("reap: stuck %v, err %v", stuck, err)
		}
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			t.Fatal("the child hidden from the first search is still running: one empty search ended the reap")
		}
		if (*log)[0].found != 0 || (*log)[1].found != 1 || !endsWithTwoQuietSearches(*log) {
			t.Errorf("searches %+v: want empty, then the late child, and two quiet searches at the end", *log)
		}
	})

	t.Run("nothing there", func(t *testing.T) {
		log := injectFind(t, func(int, runMark) ([]markedProc, error) { return nil, nil })
		if stuck, err := reapRun(runMark{Marker: "PQCATON_RUN=none"}, time.Second); err != nil || len(stuck) != 0 {
			t.Fatalf("stuck %v, err %v", stuck, err)
		}
		if len(*log) != 2 || !endsWithTwoQuietSearches(*log) {
			t.Errorf("searches %+v: want exactly two quiet searches with the wait between them", *log)
		}
	})

	t.Run("keeps appearing", func(t *testing.T) {
		ghost := []markedProc{{PID: 1 << 30, Start: "never"}}
		// 홀수 번째 검색(회수의 검색)에서 보이고, 짝수 번째(killMarked의 검색)에서는 사라진다: 끝낼 때마다 다시 나타나는 것.
		log := injectFind(t, func(n int, _ runMark) ([]markedProc, error) {
			if n%2 == 1 {
				return ghost, nil
			}
			return nil, nil
		})
		start := time.Now()
		stuck, err := reapRun(runMark{Marker: "PQCATON_RUN=ghost"}, 10*time.Millisecond)
		if err != nil || len(stuck) != 1 {
			t.Fatalf("stuck %v, err %v; want the process returned as not ended", stuck, err)
		}
		if len(*log) != 2*reapRounds+1 || time.Since(start) > 10*time.Second {
			t.Errorf("%d searches in %v: want %d, bounded", len(*log), time.Since(start), 2*reapRounds+1)
		}
	})

	t.Run("search error", func(t *testing.T) {
		boom := errors.New("injected: cannot list processes")
		injectFind(t, func(n int, _ runMark) ([]markedProc, error) {
			if n == 2 {
				return nil, boom
			}
			return nil, nil
		})
		if _, err := reapRun(runMark{Marker: "PQCATON_RUN=err"}, time.Second); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the search error", err)
		}
	})
}

// IC-U203 — **관측은 정리 · 회수 · 마지막 기록이 끝나야 끝난다. 그 전에는 새 관측을 받지 않고 시작 양식도 보이지 않는다.**
// 회수 단계를 붙잡아 「프로세스는 끝났지만 마무리가 아직」인 틈을 만든다. 그동안 화면은 「마무리 중」을 보이며 시작 양식을
// 내놓지 않고, 상태 조각은 계속 다시 받으며, 새 관측 요청은 409(run-busy)다. 회수를 풀면 기록이 최종 상태로 쓰인 뒤에야 시작
// 양식이 돌아오고 새 관측을 받는다. 순서: 프로세스 종료 → 회수 → 마지막 기록 → 실행 중 해제 → 시작 양식.
func TestNoNewObservationWhileFinishing(t *testing.T) {
	s := runReady(t, "fail")
	held, release := make(chan struct{}), make(chan struct{})
	old := reapMarked
	var once sync.Once
	// 회수는 관측 뒤와 노드 정리 뒤에 두 번 불린다. 첫 번째만 붙잡는다.
	reapMarked = func(rm runMark, grace time.Duration) ([]markedProc, error) {
		first := false
		once.Do(func() { first = true })
		if first {
			close(held)
			<-release
		}
		return old(rm, grace)
	}
	t.Cleanup(func() { reapMarked = old })
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("start = %d", w.Code)
	}
	select {
	case <-held:
	case <-time.After(15 * time.Second):
		t.Fatal("the run did not reach the reap")
	}
	m := readManifest(runDir(t, s), "")
	if m.State != ui.RunRunning {
		t.Fatalf("the record says %s before the reap finished; want running until the last write", m.State)
	}
	if st := s.run.status(); st == nil || st.State != ui.RunFinishing {
		t.Fatalf("status while finishing = %+v, want finishing", st)
	}
	page := get(t, s, ui.ScreenObserve+"?lang=en").Body.String()
	if strings.Contains(page, `action="`+ui.RunPath+`"`) {
		t.Error("the start form is offered while the run is finishing")
	}
	if !strings.Contains(page, "finishing") || !strings.Contains(page, `hx-trigger="every 2s"`) {
		t.Error("the screen does not say it is finishing, or stops refreshing")
	}
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedRunBusy) {
		t.Errorf("a new observation while finishing = %d, want 409 %s", w.Code, ui.RefusedRunBusy)
	}
	close(release)
	st := waitRun(t, s)
	if st.State != ui.RunFailed {
		t.Fatalf("final state %s, want failed", st.State)
	}
	if m := readManifest(runDir(t, s), ""); m.State != ui.RunFailed {
		t.Errorf("the record after finishing says %s", m.State)
	}
	if page := get(t, s, ui.ScreenObserve+"?lang=en").Body.String(); !strings.Contains(page, `action="`+ui.RunPath+`"`) {
		t.Error("the start form did not come back after the run finished")
	}
	reapMarked = old
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Errorf("a new observation after finishing = %d, want accepted", w.Code)
	}
	waitRun(t, s)
}

// IC-U204 — **마지막 기록을 쓰지 못하면 끝난 것으로 치지 않는다.** 마지막 기록 쓰기에 실패를 넣는다(오류 원문에 경로와 비밀값이
// 든다). 실행은 끝나(done이 닫힌다) 화면이 기다리다 멈추지는 않지만, 상태는 「회수 필요」이고 까닭은 안전한 분류(디스크 공간
// 없음)만 보이며 원문의 경로 · 비밀값은 화면 · 상태 · 로그 어디에도 없다. 시작 양식을 내놓지 않고 새 관측 요청은 409
// (run-not-ready)이며, 디스크의 기록은 「실행 중」으로 남는다. 다시 시작할 때의 복구가 기록을 쓰지 못하면 그것도 「회수 필요」로
// 막는다. 디스크가 쓸 수 있게 된 뒤 다시 시작하면 복구가 그 실행을 마무리하고, 시작 양식과 새 관측이 돌아온다.
func TestUnwrittenFinalRecordBlocksTheNextObservation(t *testing.T) {
	s := runReady(t, "fail")
	old := writeFinalManifest
	t.Cleanup(func() { writeFinalManifest = old })
	diskFull := func(string, runManifest) error {
		return &fs.PathError{Op: "open", Path: "/work/s3cret-pw/manifest.json.tmp", Err: syscall.ENOSPC}
	}
	writeFinalManifest = diskFull
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("start = %d", w.Code)
	}
	st := waitRun(t, s)
	if st.State != ui.RunStuck || !strings.Contains(st.StuckWhy, "could not be written (no space left on the disk)") {
		t.Fatalf("status after an unwritten final record = %s %q, want stuck with the safe reason", st.State, st.StuckWhy)
	}
	blocked := func(when string) {
		t.Helper()
		page := get(t, s, ui.ScreenObserve+"?lang=en").Body.String()
		for _, leak := range []string{"s3cret-pw", "/work/", "manifest.json.tmp"} {
			if strings.Contains(page, leak) || strings.Contains(s.run.status().Log, leak) || strings.Contains(s.run.status().StuckWhy, leak) {
				t.Errorf("%s: the raw error text %q reached the screen, status or log", when, leak)
			}
		}
		if strings.Contains(page, `action="`+ui.RunPath+`"`) {
			t.Errorf("%s: the start form is offered although a record was not written", when)
		}
		if !strings.Contains(page, "no space left on the disk") {
			t.Errorf("%s: the screen does not give the reason", when)
		}
		if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedRunNotReady) {
			t.Errorf("%s: a new observation = %d, want 409 %s", when, w.Code, ui.RefusedRunNotReady)
		}
		if err := s.startRun(defaultWindow); !errors.Is(err, errRunStuck) {
			t.Errorf("%s: startRun directly = %v, want errRunStuck", when, err)
		}
	}
	blocked("after the run")
	dir := runDir(t, s)
	if m := readManifest(dir, ""); m.State != ui.RunRunning {
		t.Errorf("the record on disk says %s; want it left as running for the restart to recover", m.State)
	}
	restart := func() {
		s.run.mu.Lock()
		s.run.job, s.run.unrecorded = nil, nil // 새 프로세스: 메모리의 상태는 없다
		s.run.mu.Unlock()
		s.recoverRuns()
	}

	// 다시 시작했는데 디스크가 아직 가득 차 있으면 복구도 기록을 쓰지 못한다.
	restart()
	if st := s.run.status(); st == nil || st.State != ui.RunStuck || !strings.Contains(st.StuckWhy, "recovered run record could not be written") {
		t.Fatalf("after a restart that cannot write = %+v", st)
	}
	blocked("after a restart that cannot write")
	if m := readManifest(dir, ""); m.State != ui.RunRunning {
		t.Errorf("a failed recovery changed the record on disk to %s", m.State)
	}

	// 디스크가 쓸 수 있게 된 뒤 다시 시작하면 복구가 마무리한다.
	writeFinalManifest = old
	restart()
	if m := readManifest(dir, ""); m.State == ui.RunRunning || m.State == ui.RunStuck {
		t.Fatalf("the restart did not finish the run: %s", m.State)
	}
	if page := get(t, s, ui.ScreenObserve+"?lang=en").Body.String(); !strings.Contains(page, `action="`+ui.RunPath+`"`) {
		t.Error("the start form did not come back after the recovery")
	}
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Errorf("a new observation after the recovery = %d, want accepted", w.Code)
	}
	waitRun(t, s)
}

// failSupervisorRecordEnv: 이 환경 변수가 있으면 감독으로 다시 뜬 시험 바이너리가 자기 기록 쓰기에 실패한다(TestMain).
const failSupervisorRecordEnv = "PQCATON_TEST_FAIL_SUPERVISOR_RECORD"

func init() {
	beforeSupervise = func() {
		if os.Getenv(failSupervisorRecordEnv) != "" {
			writeSupervisorRecord = func(string, supervisorRecord) error { return leakyWriteError("supervisor.json.tmp") }
		}
	}
}

// leakyWriteError: 원문에 경로와 가짜 비밀값이 든 쓰기 오류(디스크 공간 없음).
func leakyWriteError(name string) error {
	return &fs.PathError{Op: "open", Path: "/work/s3cret-pw/" + name, Err: syscall.ENOSPC}
}

// IC-U205 — **기록 쓰기 실패의 원문은 화면 · 상태 · 실행 로그 어디에도 나오지 않는다.** 마지막 기록(IC-U204) 밖의 세 기록
// 쓰기에 실패를 넣는다. 오류 원문에는 경로와 가짜 비밀값이 든다. 시작할 때의 기록, 결과를 바꾸기 직전의 기록, 감독이 자기를
// 적는 기록이다(감독은 따로 뜬 프로세스라 환경 변수로 넣는다). 셋 모두 분류(디스크 공간 없음)만 보이고 원문은 없다. 계약도
// 같다: 그 실행은 「실패」로 끝나고 마지막 기록은 쓰이며 지금 결과는 그대로다. 남은 것이 없으므로 다음 관측은 받는다.
func TestRecordWriteFailuresShowOnlyTheClass(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		inject     func(t *testing.T)
	}{
		{"start record", "the run record could not be written (no space left on the disk)", func(t *testing.T) {
			old := writeStartManifest
			writeStartManifest = func(string, runManifest) error { return leakyWriteError("manifest.json.tmp") }
			t.Cleanup(func() { writeStartManifest = old })
		}},
		{"record before the swap", "the results could not become current (no space left on the disk)", func(t *testing.T) {
			old := writeSwapManifest
			writeSwapManifest = func(string, runManifest) error { return leakyWriteError("manifest.json.tmp") }
			t.Cleanup(func() { writeSwapManifest = old })
		}},
		{"supervisor record", "the supervisor could not record itself (no space left on the disk)", func(t *testing.T) {
			t.Setenv(failSupervisorRecordEnv, "1")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := runReady(t, "ok")
			tc.inject(t)
			if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
				t.Fatalf("start = %d", w.Code)
			}
			st := waitRun(t, s)
			if st.State != ui.RunFailed {
				t.Fatalf("state %s, want failed", st.State)
			}
			dir := runDir(t, s)
			logFile, _ := os.ReadFile(filepath.Join(dir, runLog))
			page := get(t, s, ui.ScreenObserve+"?lang=en").Body.String()
			for _, leak := range []string{"s3cret-pw", "/work/", ".json.tmp"} {
				for where, text := range map[string]string{"screen": page, "status log": st.Log, "status reason": st.StuckWhy, "run log": string(logFile)} {
					if strings.Contains(text, leak) {
						t.Errorf("the raw error text %q reached the %s", leak, where)
					}
				}
			}
			if !strings.Contains(st.Log, tc.want) || !strings.Contains(string(logFile), tc.want) {
				t.Errorf("the status log or the run log does not carry the safe class %q", tc.want)
			}
			if m := readManifest(dir, ""); m.State != ui.RunFailed {
				t.Errorf("the record on disk says %s, want failed", m.State)
			}
			if !isFile(filepath.Join(s.results, oldResult)) {
				t.Error("the current results changed although the run failed")
			}
			// 남은 것이 없으므로 다음 관측은 받는다. 실패를 걷어 낸 뒤의 관측은 끝까지 간다.
			if !strings.Contains(page, `action="`+ui.RunPath+`"`) {
				t.Error("the start form is not offered after a failed run")
			}
			writeStartManifest, writeSwapManifest = writeManifest, writeManifest
			os.Unsetenv(failSupervisorRecordEnv)
			if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
				t.Fatalf("a new observation = %d, want accepted", w.Code)
			}
			if st := waitRun(t, s); st.State != ui.RunDone {
				t.Errorf("the next observation ended %s, want done", st.State)
			}
		})
	}
}
