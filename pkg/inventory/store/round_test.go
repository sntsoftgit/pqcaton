// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"errors"
	"os"
	"testing"
	"time"
)

// finalized: 세션 s-1을 확정한 세계(계획 · 세션 교체, 원장 추가).
func finalized(t *testing.T, fault func(point, path string) error) (world, *State, Identity) {
	t.Helper()
	w := newWorld(t)
	s := w.open(t, fault)
	id := Identity{Kind: "review", Org: "local", SessionID: "s-1"}
	if _, err := s.Finalize(id, "p1", []Change{{Path: w.plan, Content: []byte(planV2)}, {Path: w.session, Content: []byte(sessV2)}},
		[]Addition{{Path: w.ledger, Data: []byte(ledgerAdd)}}, time.Second); err != nil {
		t.Fatal(err)
	}
	return w, s, id
}

// IC-W46: 새 회차는 확정된 세션에서만, 한 번만 열린다. 새 세션 파일과 회차 표지가 함께 남고, 그 뒤 새 세션을
// 저장해 세션 파일이 또 바뀌어도 옛 세션은 「복구 필요」가 아니라 「대체됨」이다. 옛 세션의 같은 확정을 다시
// 보내도 아무것도 다시 쓰지 않고 대체됨을 돌려준다. 새 세션은 확정 전이라 「미확정」이다.
func TestNewRoundHandsOverTheSessionFile(t *testing.T) {
	w, s, from := finalized(t, nil)
	to := Identity{Kind: "review", Org: "local", SessionID: "s-2"}
	open := func(to Identity, content string) error {
		return s.OpenRound(from, to, []string{w.session}, Check{}, time.Second, func() ([]Change, error) {
			return []Change{{Path: w.session, Content: []byte(content)}}, nil
		})
	}

	draft := Identity{Kind: "review", Org: "local", SessionID: "draft"}
	if err := s.OpenRound(draft, to, []string{w.session}, Check{}, time.Second, func() ([]Change, error) {
		return []Change{{Path: w.session, Content: []byte("x\n")}}, nil
	}); !errors.Is(err, ErrNotFinalized) {
		t.Fatalf("a round from an unfinalized session: %v, want ErrNotFinalized", err)
	}
	if err := open(to, "session-r2\n"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, w.session); got != "session-r2\n" {
		t.Fatalf("session = %q", got)
	}
	if r, err := s.readRound(from); err != nil || r == nil || r.To != "s-2" || len(r.Paths) != 1 {
		t.Fatalf("round marker: %+v, %v", r, err)
	}
	if err := open(Identity{Kind: "review", Org: "local", SessionID: "s-3"}, "again\n"); !errors.Is(err, ErrStale) {
		t.Errorf("a second round from the same session: %v, want ErrStale", err)
	}

	// 새 세션을 저장해 세션 파일이 다시 바뀐다.
	if err := s.SaveChecked(to, []string{w.session}, Check{}, time.Second, func() ([]Change, error) {
		return []Change{{Path: w.session, Content: []byte("session-r2-edited\n")}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id   Identity
		want string
	}{{from, FinalSuperseded}, {to, FinalNone}} {
		if got, err := s.VerifyFinalized(c.id, []string{w.session, w.plan, w.ledger}, time.Second); err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.id, got, err, c.want)
		}
	}
	out, err := s.Finalize(from, "p1", []Change{{Path: w.plan, Content: []byte(planV2)}, {Path: w.session, Content: []byte(sessV2)}},
		[]Addition{{Path: w.ledger, Data: []byte(ledgerAdd)}}, time.Second)
	if err != nil || out.Status != OutcomeSuperseded {
		t.Errorf("finalizing the old session again: %+v, %v; want superseded", out.Status, err)
	}
	if got := read(t, w.session); got != "session-r2-edited\n" {
		t.Errorf("the old finalize wrote the session again: %q", got)
	}

	// 넘겨받지 않은 출력(계획)이 밖에서 바뀌면 여전히 복구 필요다.
	if err := os.WriteFile(w.plan, []byte("edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.VerifyFinalized(from, []string{w.session, w.plan, w.ledger}, time.Second); err != nil || got != FinalRecovery {
		t.Errorf("a plan changed by hand: %q, %v; want recovery", got, err)
	}
}

// IC-W47: 새 회차를 여는 도중 어디서 멈춰도, 다시 열어 복구하면 세션 파일과 회차 표지가 함께 있거나 함께 없다.
func TestNewRoundIsAtomic(t *testing.T) {
	for _, c := range []struct {
		stop      stopAt
		committed bool
	}{
		{stopAt{point: FaultRecordPreparing}, false},
		{stopAt{point: FaultPrepared}, false},
		{stopAt{point: FaultRecordPrepared}, false},
		{stopAt{point: FaultApply, path: "0"}, true},
		{stopAt{point: FaultApply, path: "1"}, true},
		{stopAt{point: FaultDone}, true},
	} {
		t.Run(c.stop.point+c.stop.path, func(t *testing.T) {
			var stop *stopAt
			w, s, from := finalized(t, func(point, path string) error {
				if stop == nil {
					return nil
				}
				return stop.hook(point, path)
			})
			stop = &c.stop
			to := Identity{Kind: "review", Org: "local", SessionID: "s-2"}
			err := s.OpenRound(from, to, []string{w.session}, Check{}, time.Second, func() ([]Change, error) {
				return []Change{{Path: w.session, Content: []byte("session-r2\n")}}, nil
			})
			if !errors.Is(err, errStop) {
				t.Fatalf("OpenRound: %v, want the injected stop", err)
			}
			s.Close()
			again := w.open(t, nil)
			if err := again.Recover(time.Second); err != nil {
				t.Fatal(err)
			}
			r, err := again.readRound(from)
			if err != nil {
				t.Fatal(err)
			}
			session := read(t, w.session)
			if c.committed != (r != nil) || c.committed != (session == "session-r2\n") {
				t.Errorf("marker %v, session %q; want both or neither (committed %v)", r != nil, session, c.committed)
			}
			if got, err := again.VerifyFinalized(from, []string{w.session, w.plan, w.ledger}, time.Second); err != nil || got == FinalRecovery {
				t.Errorf("the finalized session after a stopped round: %q, %v", got, err)
			}
		})
	}
}
