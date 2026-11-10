// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

// IC-W59 — 형식 1 영수증이 남은 상태 디렉토리를 이 판으로 연다. 영수증은 읽히고(복구 필요가 되지 않는다) 같은
// 확정을 다시 보내면 「이미 확정됨」이다. 그 영수증의 정책 지문은 「확인할 수 없음」이며, **지문이 없는 것을 규칙
// 0개의 지문으로 읽지 않는다.** 형식 2는 지문이 있을 때만 지문을 돌려준다. 이 판이 모르는 더 새 형식은 손상이다.
func TestReceiptFormatsAndPolicyDigest(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	if _, err := w.finalize(s, sessA, planV2, ledgerAdd); err != nil {
		t.Fatal(err)
	}
	s.Close()
	p := s.receiptPath(sessA)
	var raw map[string]any
	if err := json.Unmarshal([]byte(read(t, p)), &raw); err != nil {
		t.Fatal(err)
	}
	rewrite := func(format any, policy any) {
		t.Helper()
		raw["format"] = format
		if policy == nil {
			delete(raw, "scope_policy")
		} else {
			raw["scope_policy"] = policy
		}
		b, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const digest = "scope-policy-v1:4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"
	for _, c := range []struct {
		name     string
		format   any
		policy   any
		want     string
		wantOK   bool
		readable bool
	}{
		{"format 1 as written today", 1, nil, "", false, true},
		{"format 1 carrying a digest it cannot have", 1, digest, "", false, true},
		{"format 2 with a digest", 2, digest, digest, true, true},
		{"format 2 with the field missing", 2, nil, "", false, true},
		{"format 2 with an empty field", 2, "", "", false, true},
		{"a newer format", 3, digest, "", false, false},
	} {
		rewrite(c.format, c.policy)
		re := w.open(t, nil)
		rc, err := re.readReceipt(sessA)
		if !c.readable {
			if !errors.Is(err, ErrDamaged) {
				t.Errorf("%s: err = %v, want ErrDamaged", c.name, err)
			}
			re.Close()
			continue
		}
		if err != nil || rc == nil {
			t.Fatalf("%s: receipt not read: %v", c.name, err)
		}
		got, ok := rc.PolicyDigest()
		if got != c.want || ok != c.wantOK {
			t.Errorf("%s: PolicyDigest() = %q, %v; want %q, %v", c.name, got, ok, c.want, c.wantOK)
		}
		// 같은 확정을 다시 보내도 복구 필요가 아니다.
		if out, err := w.finalize(re, sessA, planV2, ledgerAdd); err != nil || out.Status != OutcomeAlready {
			t.Errorf("%s: finalize again = %+v, %v; want %s", c.name, out, err, OutcomeAlready)
		}
		re.Close()
	}
}

// IC-W58 — 정책 지문을 주면 영수증이 형식 2이고 그 지문을 적는다. 영수증은 출력과 같은 트랜잭션의 마지막 항목이다.
// **형식 2는 지문 없이 쓸 수 없다**: 지문의 모양이 아니면 ErrPolicyDigest이고 계획도 영수증도 쓰지 않는다.
// OutputReceipt는 계획 파일을 지금 내용 그대로 쓴 세션의 영수증만 돌려준다.
func TestFinalizeRecordsThePolicyDigest(t *testing.T) {
	const digest = "scope-policy-v1:4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"
	finalize := func(s *State, w world, id Identity, policy string) (Outcome, error) {
		payload := Sum([]byte(planV2 + ledgerAdd))
		return s.FinalizeChecked(id, []string{w.plan, w.session, w.ledger}, Check{}, payload, time.Second, func() (Parts, error) {
			return Parts{PayloadSHA256: payload, ScopePolicy: policy,
				Replace: []Change{{Path: w.plan, Content: []byte(planV2), Backup: true}, {Path: w.session, Content: []byte("finalized\n")}},
				Append:  []Addition{{Path: w.ledger, Data: []byte(ledgerAdd)}}}, nil
		})
	}

	w := newWorld(t)
	s := w.open(t, nil)
	snap := w.snapshot(t)
	for _, bad := range []string{"not-a-digest", "scope-policy-v1:abc", "scope-policy-v0:" + strings.Repeat("a", 64), digest + "\n"} {
		if _, err := finalize(s, w, sessA, bad); !errors.Is(err, ErrPolicyDigest) {
			t.Errorf("policy %q: err = %v, want ErrPolicyDigest", bad, err)
		}
	}
	for k, v := range w.snapshot(t) {
		if snap[k] != v {
			t.Errorf("%s changed although the digest was refused", k)
		}
	}
	if rc, _ := s.readReceipt(sessA); rc != nil {
		t.Fatal("a receipt was written for a refused digest")
	}

	out, err := finalize(s, w, sessA, digest)
	if err != nil || out.Status != OutcomeDone {
		t.Fatalf("finalize = %+v, %v", out, err)
	}
	rc, err := s.readReceipt(sessA)
	if err != nil || rc == nil {
		t.Fatal(err)
	}
	if got, ok := rc.PolicyDigest(); rc.Format != receiptFormatPolicy || !ok || got != digest {
		t.Errorf("receipt format %d digest %q %v; want format %d with %q", rc.Format, got, ok, receiptFormatPolicy, digest)
	}
	if rc.TxID == "" || len(rc.Outputs) != 2 {
		t.Errorf("receipt %+v is not the last item of the finalize transaction", rc)
	}

	if got, err := s.OutputReceipt(sessA, w.plan); err != nil || got.TxID != rc.TxID {
		t.Errorf("OutputReceipt for the finalized plan = %v, %v", got, err)
	}
	other := Identity{Kind: "review", Org: "local", SessionID: "never-finalized"}
	if _, err := s.OutputReceipt(other, w.plan); !errors.Is(err, ErrNoReceipt) {
		t.Errorf("another session: err = %v, want ErrNoReceipt", err)
	}
	if _, err := s.OutputReceipt(sessA, w.ledger); !errors.Is(err, ErrOutputMismatch) {
		t.Errorf("a file the session did not output as a replacement: err = %v, want ErrOutputMismatch", err)
	}
	if err := os.WriteFile(w.plan, []byte(planV2+" "), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OutputReceipt(sessA, w.plan); !errors.Is(err, ErrOutputMismatch) {
		t.Errorf("the plan changed after finalize: err = %v, want ErrOutputMismatch", err)
	}
	if _, err := s.OutputReceipt(Identity{Kind: "review", Org: "../x", SessionID: "s"}, w.plan); err == nil {
		t.Error("an unusable identity was looked up")
	}
}

// IC-W63: **상태 문서의 형식이 옛 판을 막는다.** 새 상태 디렉토리는 형식 2로 만든다. 옛 판(형식 1)의 상태 디렉토리는 그대로
// 열고, 형식 1 영수증만 쓰는 동안은 형식 1로 둔다. 정책 지문을 담은 확정(형식 2 영수증)을 하기 직전에 형식 2로 올리고 ID와
// 권한(0600)은 그대로다. 올리는 자리에서 멈추면 영수증도 출력도 쓰지 않는다. 형식 1 · 2가 아닌 상태 문서는 손상이다.
func TestStateFormatRaisedBeforePolicyReceipt(t *testing.T) {
	const digest = "scope-policy-v1:4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"
	readDoc := func(t *testing.T, dir string) stateDoc {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, stateFile))
		if err != nil {
			t.Fatal(err)
		}
		var d stateDoc
		if err := json.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	legacy := func(t *testing.T, w world) string {
		t.Helper()
		if err := os.Mkdir(w.stateDir, 0o700); err != nil {
			t.Fatal(err)
		}
		id := strings.Repeat("ab", 16)
		b, _ := json.Marshal(stateDoc{Format: stateFormatLegacy, StateID: id})
		if err := os.WriteFile(filepath.Join(w.stateDir, stateFile), append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		return id
	}
	policyFinalize := func(s *State, w world) (Outcome, error) {
		payload := Sum([]byte(planV2 + ledgerAdd))
		return s.FinalizeChecked(sessA, []string{w.plan, w.session, w.ledger}, Check{}, payload, time.Second, func() (Parts, error) {
			return Parts{PayloadSHA256: payload, ScopePolicy: digest,
				Replace: []Change{{Path: w.plan, Content: []byte(planV2), Backup: true}, {Path: w.session, Content: []byte("finalized\n")}},
				Append:  []Addition{{Path: w.ledger, Data: []byte(ledgerAdd)}}}, nil
		})
	}

	t.Run("a new state directory starts at format 2", func(t *testing.T) {
		w := newWorld(t)
		w.open(t, nil)
		if d := readDoc(t, w.stateDir); d.Format != stateFormat {
			t.Errorf("new state format = %d, want %d", d.Format, stateFormat)
		}
	})

	t.Run("a legacy state stays at format 1 while only format 1 receipts are written", func(t *testing.T) {
		w := newWorld(t)
		id := legacy(t, w)
		s := w.open(t, nil)
		if s.ID != id {
			t.Fatalf("legacy state id = %s, want %s", s.ID, id)
		}
		if _, err := w.finalize(s, sessA, planV2, ledgerAdd); err != nil {
			t.Fatal(err)
		}
		if d := readDoc(t, w.stateDir); d.Format != stateFormatLegacy || d.StateID != id {
			t.Errorf("after a format 1 receipt: state %+v, want format 1 with the same id", d)
		}
	})

	t.Run("raised just before the first format 2 receipt", func(t *testing.T) {
		w := newWorld(t)
		id := legacy(t, w)
		stop := errors.New("stop at the state format")
		s := w.open(t, func(point, path string) error {
			if point == FaultStateFormat {
				return stop
			}
			return nil
		})
		snap := w.snapshot(t)
		if _, err := policyFinalize(s, w); !errors.Is(err, stop) {
			t.Fatalf("finalize with a fault at the state format: err = %v, want the fault", err)
		}
		for k, v := range w.snapshot(t) {
			if snap[k] != v {
				t.Errorf("%s changed although the state format was not raised", k)
			}
		}
		if rc, _ := s.readReceipt(sessA); rc != nil {
			t.Fatal("a receipt was written although the state format was not raised")
		}
		if d := readDoc(t, w.stateDir); d.Format != stateFormatLegacy {
			t.Fatalf("state format after the fault = %d, want 1", d.Format)
		}
		s.Close()

		s = w.open(t, nil)
		if out, err := policyFinalize(s, w); err != nil || out.Status != OutcomeDone {
			t.Fatalf("finalize = %+v, %v", out, err)
		}
		if d := readDoc(t, w.stateDir); d.Format != stateFormat || d.StateID != id {
			t.Errorf("after a format 2 receipt: state %+v, want format %d with id %s", d, stateFormat, id)
		}
		if m := mode(t, filepath.Join(w.stateDir, stateFile)); m != filePerm {
			t.Errorf("state.json mode = %#o, want %#o", m, filePerm)
		}
		if rc, err := s.readReceipt(sessA); err != nil || rc == nil || rc.Format != receiptFormatPolicy {
			t.Errorf("receipt = %+v, %v; want format 2", rc, err)
		}
		s.Close()
		if again := open(t, w.stateDir); again.ID != id {
			t.Errorf("reopened id = %s, want %s", again.ID, id)
		}
		if got, err := ReadStateID(w.stateDir); err != nil || got != id {
			t.Errorf("ReadStateID = %q, %v", got, err)
		}
	})

	t.Run("an unknown state format is damage", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), DirName)
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(stateDoc{Format: stateFormat + 1, StateID: strings.Repeat("cd", 16)})
		if err := os.WriteFile(filepath.Join(dir, stateFile), b, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(dir, Options{Warn: quiet, UID: -1}); !errors.Is(err, ErrDamaged) {
			t.Errorf("Open of format %d: err = %v, want ErrDamaged", stateFormat+1, err)
		}
		if _, err := ReadStateID(dir); !errors.Is(err, ErrDamaged) {
			t.Errorf("ReadStateID of format %d: err = %v, want ErrDamaged", stateFormat+1, err)
		}
	})
}
