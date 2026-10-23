// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

var sessA = Identity{Kind: "review", Org: "local", SessionID: "6f1c2a7e-0d4b-4c55-9e3a-1b2c3d4e5f60"}

// finalize: 세션 하나의 확정. 계획과 세션을 바꾸고 원장에 판정을 더한다.
func (w world) finalize(s *State, id Identity, plan, judg string) (Outcome, error) {
	payload := Sum([]byte(plan + judg))
	return s.Finalize(id, payload,
		[]Change{{Path: w.plan, Content: []byte(plan), Backup: true}, {Path: w.session, Content: []byte("finalized " + id.SessionID + "\n")}},
		[]Addition{{Path: w.ledger, Data: []byte(judg)}}, time.Second)
}

// IC-W20: 확정은 영수증(0600)을 남긴다. 같은 내용으로 다시 확정하면 아무것도 다시 쓰지 않고 이전
// 결과를 돌려준다. 원장에 판정이 두 번 쌓이지 않는다. 서버를 다시 띄워도 같다.
func TestFinalizeIsIdempotent(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	out, err := w.finalize(s, sessA, planV2, ledgerAdd)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != OutcomeDone {
		t.Fatalf("first finalize = %s, want %s", out.Status, OutcomeDone)
	}
	p := s.receiptPath(sessA)
	if m := mode(t, p); m != 0o600 {
		t.Errorf("receipt mode = %#o, want 0600", m)
	}
	var rc Receipt
	if err := json.Unmarshal([]byte(read(t, p)), &rc); err != nil {
		t.Fatal(err)
	}
	if rc.Identity() != sessA || len(rc.Outputs) != 2 || len(rc.Ledger) != 1 ||
		rc.Ledger[0].Offset != int64(len(ledgerBase)) || rc.Ledger[0].Length != int64(len(ledgerAdd)) {
		t.Errorf("receipt = %+v", rc)
	}
	if rc.Outputs[0].Path != "plan.json" {
		t.Errorf("output path = %q, want it relative to the state directory's parent", rc.Outputs[0].Path)
	}
	snap := w.snapshot(t)

	again, err := w.finalize(s, sessA, planV2, ledgerAdd)
	if err != nil || again.Status != OutcomeAlready {
		t.Fatalf("second finalize = %+v, %v; want %s", again, err, OutcomeAlready)
	}
	s.Close()
	restarted := w.open(t, nil)
	third, err := w.finalize(restarted, sessA, planV2, ledgerAdd)
	if err != nil || third.Status != OutcomeAlready || third.Receipt.TxID != rc.TxID {
		t.Fatalf("after a restart = %+v, %v; want %s with the same receipt", third, err, OutcomeAlready)
	}
	for k, v := range w.snapshot(t) {
		if snap[k] != v {
			t.Errorf("%s changed on a repeated finalize: %q → %q", k, snap[k], v)
		}
	}
}

// IC-W21: 같은 세션을 다른 내용으로 다시 확정하면 거부하고 새 세션을 열라고 알린다. 아무것도 바뀌지 않는다.
func TestFinalizeDifferentPayloadRefused(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	if _, err := w.finalize(s, sessA, planV2, ledgerAdd); err != nil {
		t.Fatal(err)
	}
	snap := w.snapshot(t)
	_, err := w.finalize(s, sessA, "plan-v3\n", ledgerAdd)
	if !errors.Is(err, ErrSessionFinalized) || !strings.Contains(err.Error(), "new session") {
		t.Fatalf("err = %v, want ErrSessionFinalized asking for a new session", err)
	}
	for k, v := range w.snapshot(t) {
		if snap[k] != v {
			t.Errorf("%s changed: %q → %q", k, snap[k], v)
		}
	}
}

// IC-W22: 확정된 세션은 일반 저장도 막힌다. 다른 세션의 저장은 지나간다.
func TestSaveAfterFinalizeRefused(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	if err := s.Save(sessA, []Change{{Path: w.session, Content: []byte("draft\n")}}, time.Second); err != nil {
		t.Fatalf("save before finalize: %v", err)
	}
	if _, err := w.finalize(s, sessA, planV2, ledgerAdd); err != nil {
		t.Fatal(err)
	}
	before := read(t, w.session)
	err := s.Save(sessA, []Change{{Path: w.session, Content: []byte("late edit\n")}}, time.Second)
	if !errors.Is(err, ErrSessionFinalized) {
		t.Fatalf("save after finalize: err = %v, want ErrSessionFinalized", err)
	}
	if read(t, w.session) != before {
		t.Error("the finalized session was overwritten")
	}
	other := Identity{Kind: "review", Org: "local", SessionID: "other-session"}
	if err := s.Save(other, []Change{{Path: w.session, Content: []byte("new session\n")}}, time.Second); err != nil {
		t.Fatalf("save of another session: %v", err)
	}
}

// IC-W23: 다른 세션이 뒤에 같은 출력을 확정했으면, 앞 세션을 같은 내용으로 다시 확정할 때 「대체됨」으로
// 알리고 다시 쓰지 않는다. 오류가 아니다.
func TestFinalizeSuperseded(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	if _, err := w.finalize(s, sessA, planV2, ledgerAdd); err != nil {
		t.Fatal(err)
	}
	sessB := Identity{Kind: "review", Org: "local", SessionID: "session-b"}
	if _, err := w.finalize(s, sessB, "plan-b\n", "{\"b\":1}\n"); err != nil {
		t.Fatal(err)
	}
	out, err := w.finalize(s, sessA, planV2, ledgerAdd)
	if err != nil {
		t.Fatalf("err = %v, want a superseded outcome", err)
	}
	if out.Status != OutcomeSuperseded || out.By == nil || out.By.Identity() != sessB {
		t.Fatalf("outcome = %+v, want superseded by %s", out, sessB)
	}
	if read(t, w.plan) != "plan-b\n" {
		t.Error("the later session's plan was overwritten")
	}
}

// IC-W24: 확정한 출력이 밖에서 바뀌었거나(뒤 세션의 영수증 없음) 원장 구간이 영수증과 다르면 복구
// 필요다. 다시 쓰지 않는다.
func TestFinalizeDetectsTampering(t *testing.T) {
	cases := map[string]func(w world){
		"output edited by hand": func(w world) { os.WriteFile(w.plan, []byte("edited\n"), 0o600) },
		"ledger rewritten": func(w world) {
			os.WriteFile(w.ledger, []byte(ledgerBase+strings.ToUpper(ledgerAdd)), 0o600)
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			s := w.open(t, nil)
			if _, err := w.finalize(s, sessA, planV2, ledgerAdd); err != nil {
				t.Fatal(err)
			}
			tamper(w)
			snap := w.snapshot(t)
			_, err := w.finalize(s, sessA, planV2, ledgerAdd)
			if !errors.Is(err, ErrNeedsRecovery) {
				t.Fatalf("err = %v, want ErrNeedsRecovery", err)
			}
			for k, v := range w.snapshot(t) {
				if snap[k] != v {
					t.Errorf("%s changed: %q → %q", k, snap[k], v)
				}
			}
		})
	}
}

// IC-W25: 영수증을 쓰기 직전에 멈춘 확정은, 다시 열어 복구하면 영수증까지 끝난다. 그 뒤 같은 확정은
// 이전 결과다. 같은 세션을 동시에 두 번 확정하면 한쪽만 쓰고 다른 쪽은 이전 결과를 받는다.
func TestFinalizeAfterCrashAndRace(t *testing.T) {
	w := newWorld(t)
	stop := stopAt{point: FaultApply, path: "4"}
	s := w.open(t, stop.hook)
	if _, err := w.finalize(s, sessA, planV2, ledgerAdd); !errors.Is(err, errStop) {
		t.Fatalf("err = %v, want the injected stop", err)
	}
	s.Close()
	again := w.open(t, nil)
	if err := again.Recover(time.Second); err != nil {
		t.Fatal(err)
	}
	if rc, err := again.Finalized(sessA); err != nil || rc == nil {
		t.Fatalf("receipt after recovery: %v, %v", rc, err)
	}
	out, err := w.finalize(again, sessA, planV2, ledgerAdd)
	if err != nil || out.Status != OutcomeAlready {
		t.Fatalf("finalize after recovery = %+v, %v; want %s", out, err, OutcomeAlready)
	}
	if read(t, w.ledger) != ledgerBase+ledgerAdd {
		t.Errorf("ledger = %q; the entry must be there exactly once", read(t, w.ledger))
	}

	w2 := newWorld(t)
	s2 := w2.open(t, nil)
	var wg sync.WaitGroup
	results := make([]string, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := w2.finalize(s2, sessA, planV2, ledgerAdd)
			if err != nil {
				results[i] = err.Error()
				return
			}
			results[i] = out.Status
		}(i)
	}
	wg.Wait()
	got := results[0] + "+" + results[1]
	if got != OutcomeDone+"+"+OutcomeAlready && got != OutcomeAlready+"+"+OutcomeDone {
		t.Errorf("two finalizes at once = %s, want one done and one already", got)
	}
	if read(t, w2.ledger) != ledgerBase+ledgerAdd {
		t.Errorf("ledger = %q; the entry must be there exactly once", read(t, w2.ledger))
	}
}

// IC-W26: 종류 · 조직 · 세션 ID에 경로 구분자나 「..」가 있으면 받지 않는다. 영수증 경로가 상태
// 디렉토리 밖으로 나가지 않는다.
func TestIdentityIsChecked(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	for _, id := range []Identity{
		{Kind: "review", Org: "local", SessionID: "../../escape"},
		{Kind: "review", Org: "a/b", SessionID: "s"},
		{Kind: "..", Org: "local", SessionID: "s"},
		{Kind: "review", Org: "local", SessionID: ""},
		{Kind: "review", Org: "local", SessionID: ".hidden"},
	} {
		if _, err := w.finalize(s, id, planV2, ledgerAdd); err == nil || errors.Is(err, ErrNeedsRecovery) {
			t.Errorf("Finalize(%v): err = %v, want it refused", id, err)
		}
		if err := s.Save(id, []Change{{Path: w.session, Content: []byte("x")}}, time.Second); err == nil {
			t.Errorf("Save(%v) was accepted", id)
		}
	}
	same(t, w.snapshot(t), before)
}
