// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// verdict: 오류 없이 판정되어야 하는 자리.
func verdict(t *testing.T, s *State, id Identity) string {
	t.Helper()
	v, err := s.VerifyFinalized(id, time.Second)
	if err != nil {
		t.Fatalf("VerifyFinalized(%s): %v", id, err)
	}
	return v
}

// IC-W39: 확정 상태는 영수증이 있는지가 아니라 출력 해시와 원장 구간으로 판정한다. 확정 전은 미확정,
// 확정 뒤는 확정됨이다. 계획을 손으로 고치거나 원장의 그 구간을 잘라 내면 복구 필요다. 판정은 아무
// 파일도 바꾸지 않는다.
func TestVerifyFinalizedReadsOutputs(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	if v := verdict(t, s, sessA); v != FinalNone {
		t.Fatalf("before finalizing: %s, want %s", v, FinalNone)
	}
	if _, err := w.finalize(s, sessA, planV2, ledgerAdd); err != nil {
		t.Fatal(err)
	}
	snap := w.snapshotAll(t)
	if v := verdict(t, s, sessA); v != FinalDone {
		t.Fatalf("after finalizing: %s, want %s", v, FinalDone)
	}
	if got := w.snapshotAll(t); !equalMaps(got, snap) {
		t.Error("checking the finalize state changed a file")
	}

	if err := os.WriteFile(w.plan, []byte("edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if v := verdict(t, s, sessA); v != FinalRecovery {
		t.Errorf("plan edited by hand: %s, want %s", v, FinalRecovery)
	}
	if err := os.WriteFile(w.plan, []byte(planV2), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.ledger, []byte(ledgerBase), 0o600); err != nil {
		t.Fatal(err)
	}
	if v := verdict(t, s, sessA); v != FinalRecovery {
		t.Errorf("ledger cut back: %s, want %s", v, FinalRecovery)
	}
	if recs, _ := s.Records(); len(recs) != 0 {
		t.Error("checking wrote a transaction record")
	}
}

// IC-W40: 대상 디렉토리에 잠금 파일이 없으면 만들지 않고 잠그지 않은 채 판정한다. 읽기가 남의
// 디렉토리에 파일을 남기면 안 된다.
func TestVerifyFinalizedCreatesNoLock(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	if _, err := w.finalize(s, sessA, planV2, ledgerAdd); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(w.data, TargetLockName)
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if v := verdict(t, s, sessA); v != FinalDone {
		t.Errorf("without a lock file: %s, want %s", v, FinalDone)
	}
	if _, err := os.Lstat(lock); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("checking created %s", TargetLockName)
	}
}

// IC-W41: 확정과 겹치지 않는다. 다른 요청이 대상 잠금을 쥐고 있으면 기다리다 ErrTargetBusy이고,
// 끝나지 않은 기록이 그 디렉토리를 건드리면 ErrInProgress다. 어느 쪽도 「복구 필요」로 단정하지
// 않는다 — 부르는 쪽은 「확인할 수 없음」으로 보인다.
func TestVerifyFinalizedDoesNotGuessMidway(t *testing.T) {
	w := newWorld(t)
	var fault func(point, path string) error
	s := w.open(t, func(point, path string) error {
		if fault != nil {
			return fault(point, path)
		}
		return nil
	})
	if _, err := w.finalize(s, sessA, planV2, ledgerAdd); err != nil {
		t.Fatal(err)
	}
	release, err := s.LockTargets([]string{w.data}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := s.VerifyFinalized(sessA, 50*time.Millisecond); !errors.Is(err, ErrTargetBusy) {
		t.Errorf("while another request holds the lock: %q, %v; want ErrTargetBusy", v, err)
	}
	release()

	// 두 번째 확정이 적용 도중에 멈춘다. 기록이 남아 있는 동안은 판정하지 않는다.
	stop := &stopAt{point: FaultApply, path: "0"}
	fault = stop.hook
	sessB := Identity{Kind: "review", Org: "local", SessionID: "session-b"}
	if _, err := w.finalize(s, sessB, "plan-b\n", "{\"b\":1}\n"); !errors.Is(err, errStop) {
		t.Fatalf("the stopped finalize: %v", err)
	}
	if v, err := s.VerifyFinalized(sessA, time.Second); !errors.Is(err, ErrInProgress) {
		t.Errorf("with an unfinished record: %q, %v; want ErrInProgress", v, err)
	}
}

// IC-W42: 대체됨은 뒤 영수증이 그 출력을 지금 내용으로 썼고 순번이 더 클 때만이다. 뒤 세션이 같은
// 계획을 다시 확정하면 앞 세션은 대체됨, 뒤 세션은 확정됨이다. 뒤 영수증의 순번이 앞보다 작게
// 적혀 있으면 그 영수증은 설명이 되지 않으므로 앞 세션은 복구 필요다.
func TestVerifyFinalizedSupersededNeedsALaterReceipt(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	if _, err := w.finalize(s, sessA, planV2, ledgerAdd); err != nil {
		t.Fatal(err)
	}
	sessB := Identity{Kind: "review", Org: "local", SessionID: "session-b"}
	if _, err := w.finalize(s, sessB, "plan-b\n", "{\"b\":1}\n"); err != nil {
		t.Fatal(err)
	}
	if v := verdict(t, s, sessA); v != FinalSuperseded {
		t.Errorf("a: %s, want %s", v, FinalSuperseded)
	}
	if v := verdict(t, s, sessB); v != FinalDone {
		t.Errorf("b: %s, want %s", v, FinalDone)
	}

	p := s.receiptPath(sessB)
	var r Receipt
	if err := json.Unmarshal([]byte(read(t, p)), &r); err != nil {
		t.Fatal(err)
	}
	r.Seq = 0
	b, _ := json.Marshal(r)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if v := verdict(t, s, sessA); v != FinalRecovery {
		t.Errorf("a with b's sequence before it: %s, want %s", v, FinalRecovery)
	}
}

// snapshotAll: 데이터 디렉토리와 상태 디렉토리의 파일 전부(잠금 파일 포함).
func (w world) snapshotAll(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range []string{w.data, w.stateDir} {
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				b, _ := os.ReadFile(p)
				out[p] = string(b)
			}
			return nil
		})
	}
	return out
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}
