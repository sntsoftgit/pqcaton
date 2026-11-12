// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
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
