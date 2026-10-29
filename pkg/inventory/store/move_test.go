// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// moveWorld: 옮길 파일 셋(서명한 계획 · 적용 · 되돌리기 플레이북과 같은 모양)과 그 이동.
func moveWorld(t *testing.T) (world, []Move, map[string]string) {
	t.Helper()
	w := newWorld(t)
	files := map[string]string{"plan.signed.json": "signed\n", "plan.provision.yml": "apply\n", "plan.rollback.yml": "undo\n"}
	var moves []Move
	for name, body := range files {
		p := filepath.Join(w.data, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		ext := filepath.Ext(name)
		moves = append(moves, Move{From: p, To: filepath.Join(w.data, name[:len(name)-len(ext)]+".T1"+ext)})
	}
	return w, moves, files
}

// moved: 세 파일이 모두 옮겨졌으면 true, 모두 그대로면 false. 섞여 있으면 실패다.
func moved(t *testing.T, moves []Move, files map[string]string) bool {
	t.Helper()
	var at, from int
	for _, m := range moves {
		body := files[filepath.Base(m.From)]
		_, errFrom := os.Stat(m.From)
		_, errTo := os.Stat(m.To)
		switch {
		case errFrom != nil && errTo == nil && read(t, m.To) == body:
			at++
		case errFrom == nil && errTo != nil && read(t, m.From) == body:
			from++
		default:
			t.Errorf("%s is half moved (from %v, to %v)", filepath.Base(m.From), errFrom, errTo)
		}
	}
	if at != 0 && from != 0 {
		t.Errorf("mixed: %d moved, %d left", at, from)
	}
	return at == len(moves)
}

// IC-W49: 여러 파일의 이름 바꾸기는 한 트랜잭션이다. 어느 자리에서 멈춰도 다시 열어 복구하면 셋 다 옮겨졌거나 셋 다
// 그대로다(새 이름 셋을 다 만든 뒤 옛 이름을 지운다). 새 이름 하나라도 이미 있으면 ErrExists이고 아무것도 옮기지 않는다.
func TestMoveIsOneTransaction(t *testing.T) {
	cases := []struct {
		stop  stopAt
		moved bool
	}{
		{stopAt{point: FaultRecordPreparing}, false},
		{stopAt{point: FaultPrepareWrite}, false},
		{stopAt{point: FaultPrepared}, false},
		{stopAt{point: FaultRecordPrepared}, false},
		{stopAt{point: FaultApply, path: "1"}, true},
		{stopAt{point: FaultApply, path: "3"}, true},
		{stopAt{point: FaultApply, path: "4"}, true},
		{stopAt{point: FaultDone}, true},
	}
	for _, c := range cases {
		t.Run(c.stop.point+c.stop.path, func(t *testing.T) {
			w, moves, files := moveWorld(t)
			stop := c.stop
			s := w.open(t, stop.hook)
			if _, err := s.MoveChecked(Identity{Kind: "archive", Org: "local", SessionID: "a"}, moves, Check{}, time.Second); !errors.Is(err, errStop) {
				t.Fatalf("MoveChecked: %v, want the injected stop", err)
			}
			s.Close()
			again := w.open(t, nil)
			if err := again.Recover(time.Second); err != nil {
				t.Fatalf("Recover: %v", err)
			}
			if got := moved(t, moves, files); got != c.moved {
				t.Errorf("moved = %v, want %v", got, c.moved)
			}
		})
	}

	w, moves, files := moveWorld(t)
	if err := os.WriteFile(moves[1].To, []byte("taken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := w.open(t, nil)
	if _, err := s.MoveChecked(Identity{Kind: "archive", Org: "local", SessionID: "a"}, moves, Check{}, time.Second); !errors.Is(err, ErrExists) {
		t.Fatalf("a taken destination: %v, want ErrExists", err)
	}
	if read(t, moves[1].To) != "taken\n" {
		t.Error("the taken destination was overwritten")
	}
	_ = os.Remove(moves[1].To)
	if moved(t, moves, files) {
		t.Error("files moved although a destination was taken")
	}
}

// IC-W50: 일반 트랜잭션(Do)도 만들기 · 지우기 대상의 폴더를 잠근다. 다른 요청이 그 폴더를 쥐고 있으면 기다리다
// ErrTargetBusy이고 아무것도 지우지 않는다.
func TestDoLocksCreateAndRemoveTargets(t *testing.T) {
	w, moves, _ := moveWorld(t)
	s := w.open(t, nil)
	release, err := s.LockTargets([]string{w.data}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	err = s.Do(Tx{Kind: "archive", Org: "local", SessionID: "a", Remove: []string{moves[0].From}}, 50*time.Millisecond)
	if !errors.Is(err, ErrTargetBusy) {
		t.Fatalf("Do(Remove) while the folder is held: %v, want ErrTargetBusy", err)
	}
	if _, err := os.Stat(moves[0].From); err != nil {
		t.Error("the file was removed while the folder was held")
	}
}
