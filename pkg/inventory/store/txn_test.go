// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// world: 확정 한 번이 건드리는 파일들. 계획(백업 있음), 세션, 판정 원장, 영수증.
type world struct {
	data, stateDir                    string
	plan, session, ledger, receiptDir string
}

const (
	planV1, planV2 = "plan-v1\n", "plan-v2\n"
	sessV1, sessV2 = "session-v1\n", "session-v2\n"
	ledgerBase     = "{\"n\":1}\n"
	ledgerAdd      = "{\"n\":2}\n{\"n\":3}\n"
	receiptBody    = "{\"receipt\":true}\n"
)

func newWorld(t *testing.T) world {
	t.Helper()
	data := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	w := world{data: data, stateDir: filepath.Join(data, DirName),
		plan: filepath.Join(data, "plan.json"), session: filepath.Join(data, "session.json"),
		ledger: filepath.Join(data, "judgments.jsonl")}
	w.receiptDir = filepath.Join(w.stateDir, "receipts")
	for p, c := range map[string]string{w.plan: planV1, w.session: sessV1, w.ledger: ledgerBase} {
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func (w world) open(t *testing.T, fault func(point, path string) error) *State {
	t.Helper()
	s, err := Open(w.stateDir, Options{Warn: quiet, UID: -1, Fault: fault})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(w.receiptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func (w world) tx() Tx {
	return Tx{Kind: "review", Org: "local", SessionID: "s-1", PayloadSHA256: Sum([]byte("payload")),
		Replace: []Change{{Path: w.plan, Content: []byte(planV2), Backup: true}, {Path: w.session, Content: []byte(sessV2)}},
		Append:  []Addition{{Path: w.ledger, Data: []byte(ledgerAdd)}},
		Create:  []Change{{Path: filepath.Join(w.receiptDir, "s-1.json"), Content: []byte(receiptBody)}},
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return "<absent>"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// snapshot: 확정이 건드리는 파일 다섯의 지금 내용.
func (w world) snapshot(t *testing.T) map[string]string {
	return map[string]string{
		"plan": read(t, w.plan), "plan.prev": read(t, w.plan+".prev"), "session": read(t, w.session),
		"ledger": read(t, w.ledger), "receipt": read(t, filepath.Join(w.receiptDir, "s-1.json")),
	}
}

var (
	before = map[string]string{"plan": planV1, "plan.prev": "<absent>", "session": sessV1, "ledger": ledgerBase, "receipt": "<absent>"}
	after  = map[string]string{"plan": planV2, "plan.prev": planV1, "session": sessV2, "ledger": ledgerBase + ledgerAdd, "receipt": receiptBody}
)

func same(t *testing.T, got, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

// clean: 준비 파일도 기록도 남지 않았다.
func (w world) clean(t *testing.T, s *State) {
	t.Helper()
	for _, d := range []string{w.data, w.receiptDir} {
		left, err := FindPrepared(d)
		if err != nil {
			t.Fatal(err)
		}
		if len(left) != 0 {
			t.Errorf("prepared files left in %s: %v", d, left)
		}
	}
	recs, err := s.Records()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Errorf("%d transaction records left", len(recs))
	}
}

// IC-W13: 확정 한 번이 계획 교체(백업 포함) · 세션 교체 · 원장 추가 · 영수증 생성을 함께 끝낸다.
// 끝나면 준비 파일도 기록도 남지 않고, 기록과 준비 파일은 0600이다.
func TestDoCompletes(t *testing.T) {
	w := newWorld(t)
	var modes []os.FileMode
	s := w.open(t, func(point, path string) error {
		if point == FaultRecordPrepared {
			for _, d := range []string{filepath.Join(w.stateDir, txnDir), w.data} {
				ents, _ := os.ReadDir(d)
				for _, e := range ents {
					if strings.HasSuffix(e.Name(), ".json") && d != w.data || preparedPattern.MatchString(e.Name()) {
						fi, _ := e.Info()
						modes = append(modes, fi.Mode().Perm())
					}
				}
			}
		}
		return nil
	})
	if err := s.Do(w.tx(), time.Second); err != nil {
		t.Fatal(err)
	}
	same(t, w.snapshot(t), after)
	w.clean(t, s)
	if len(modes) == 0 {
		t.Fatal("saw no record or prepared file at the commit point")
	}
	for _, m := range modes {
		if m != 0o600 {
			t.Errorf("a record or prepared file had mode %#o, want 0600", m)
		}
	}
}

type stopAt struct {
	point, path string
	fired       bool
}

var errStop = errors.New("the process stopped here")

func (f *stopAt) hook(point, path string) error {
	if f.fired || point != f.point || (f.path != "" && path != f.path) {
		return nil
	}
	f.fired = true
	return errStop
}

// IC-W14: 어느 자리에서 멈춰도 다시 열어 복구하면 확정 전이거나 확정 후다. 그 중간은 없다.
// PREPARED 전에 멈추면 기록에 적힌 준비 파일만 걷어 확정 전으로, 그 뒤면 끝까지 가서 확정 후로 간다.
func TestRecoverFromEveryStop(t *testing.T) {
	cases := []struct {
		stop      stopAt
		committed bool
	}{
		{stopAt{point: FaultRecordPreparing}, false},
		{stopAt{point: FaultPrepareWrite}, false},
		{stopAt{point: FaultPrepareSync}, false},
		{stopAt{point: FaultPrepared}, false},
		{stopAt{point: FaultRecordPrepared}, false},
		{stopAt{point: FaultApply, path: "0"}, true},
		{stopAt{point: FaultApply, path: "1"}, true},
		{stopAt{point: FaultApply, path: "2"}, true},
		{stopAt{point: FaultApply, path: "3"}, true},
		{stopAt{point: FaultApply, path: "4"}, true},
		{stopAt{point: FaultInstallRename}, true},
		{stopAt{point: FaultInstallSync}, true},
		{stopAt{point: FaultAppendHalf}, true},
		{stopAt{point: FaultDone}, true},
	}
	for _, c := range cases {
		t.Run(c.stop.point+c.stop.path, func(t *testing.T) {
			w := newWorld(t)
			stop := c.stop
			s := w.open(t, stop.hook)
			if err := s.Do(w.tx(), time.Second); !errors.Is(err, errStop) {
				t.Fatalf("Do: err = %v, want the injected stop", err)
			}
			s.Close()

			again := w.open(t, nil)
			if err := again.CheckOrphans([]string{w.data, w.receiptDir}); err != nil {
				t.Fatalf("CheckOrphans after a stop: %v", err)
			}
			if err := again.Recover(time.Second); err != nil {
				t.Fatalf("Recover: %v", err)
			}
			want := before
			if c.committed {
				want = after
			}
			same(t, w.snapshot(t), want)
			w.clean(t, again)

			if !c.committed {
				if err := again.Do(w.tx(), time.Second); err != nil {
					t.Fatalf("Do after the rollback: %v", err)
				}
				same(t, w.snapshot(t), after)
			}
		})
	}
}

// IC-W15: 기록에 없는 준비 파일은 지우지 않고 멈춘다. PREPARING 복구는 기록에 열거된 것만 걷고,
// 이름 모양이 같은 남의 파일은 그대로 둔다.
func TestOrphansAreNotDeleted(t *testing.T) {
	w := newWorld(t)
	stop := stopAt{point: FaultPrepared}
	s := w.open(t, stop.hook)
	if err := s.Do(w.tx(), time.Second); !errors.Is(err, errStop) {
		t.Fatalf("err = %v, want the injected stop", err)
	}
	s.Close()

	stranger := PreparedPath(w.plan, "ffffffffffffffffffffffffffffffff")
	if err := os.WriteFile(stranger, []byte("not ours"), 0o600); err != nil {
		t.Fatal(err)
	}
	again := w.open(t, nil)
	err := again.CheckOrphans([]string{w.data})
	if !errors.Is(err, ErrNeedsRecovery) || !strings.Contains(err.Error(), filepath.Base(stranger)) {
		t.Fatalf("CheckOrphans: err = %v, want it to name %s", err, stranger)
	}
	if err := again.Recover(time.Second); err != nil {
		t.Fatal(err)
	}
	if read(t, stranger) != "not ours" {
		t.Error("recovery removed a prepared-looking file that no record listed")
	}
	left, _ := FindPrepared(w.data)
	if len(left) != 1 {
		t.Errorf("prepared files after recovery = %v, want only the stranger", left)
	}
	same(t, w.snapshot(t), before)
}

// IC-W16: 원장에 추가분이 반만 쓰이고 멈추면, 복구는 **자르지 않고** 남은 바이트를 이어 쓴다.
// 원장은 한 번도 짧아지지 않고, 추가분은 정확히 한 번 들어간다.
func TestLedgerResumesWithoutTruncating(t *testing.T) {
	w := newWorld(t)
	stop := stopAt{point: FaultAppendHalf}
	s := w.open(t, stop.hook)
	if err := s.Do(w.tx(), time.Second); !errors.Is(err, errStop) {
		t.Fatalf("err = %v, want the injected stop", err)
	}
	s.Close()
	half := read(t, w.ledger)
	if half != ledgerBase+ledgerAdd[:len(ledgerAdd)/2] {
		t.Fatalf("ledger after the stop = %q, want the base and half the entry", half)
	}

	var sizes []int64
	again := w.open(t, nil)
	again.faultFn = func(point, path string) error {
		if fi, err := os.Stat(w.ledger); err == nil {
			sizes = append(sizes, fi.Size())
		}
		return nil
	}
	rep, err := again.Status()
	if err != nil {
		t.Fatal(err)
	}
	if v := verdictOf(rep, ItemAppend); v != VerdictPartial {
		t.Errorf("status of the half-written entry = %q, want %q", v, VerdictPartial)
	}
	if err := again.Recover(time.Second); err != nil {
		t.Fatal(err)
	}
	same(t, w.snapshot(t), after)
	for i := 1; i < len(sizes); i++ {
		if sizes[i] < sizes[i-1] {
			t.Fatalf("the ledger shrank during recovery: %v", sizes)
		}
	}
}

func verdictOf(rep Report, kind string) string {
	for _, r := range rep.Records {
		for _, it := range r.Items {
			if it.Kind == kind {
				return it.Verdict
			}
		}
	}
	return ""
}

// IC-W17: 반만 쓰인 원장의 넘친 바이트가 이 추가분과 다르거나, 기준 길이 앞부분이 바뀌었으면
// 복구 필요로 멈춘다. 원장은 한 바이트도 바뀌지 않는다.
func TestLedgerMismatchStops(t *testing.T) {
	cases := map[string]func(b []byte) []byte{
		"different overflow": func(b []byte) []byte {
			b[len(b)-1] ^= 0xff
			return b
		},
		"changed prefix": func(b []byte) []byte {
			b[2] ^= 0xff
			return b
		},
		"bytes after the entry": func(b []byte) []byte {
			return append(b, []byte(ledgerAdd+"{\"n\":9}\n")...)
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			stop := stopAt{point: FaultAppendHalf}
			s := w.open(t, stop.hook)
			if err := s.Do(w.tx(), time.Second); !errors.Is(err, errStop) {
				t.Fatalf("err = %v, want the injected stop", err)
			}
			s.Close()
			b := []byte(read(t, w.ledger))
			tampered := string(tamper(b))
			if err := os.WriteFile(w.ledger, []byte(tampered), 0o600); err != nil {
				t.Fatal(err)
			}

			again := w.open(t, nil)
			err := again.Recover(time.Second)
			var re *RecoveryError
			if !errors.As(err, &re) || !errors.Is(err, ErrNeedsRecovery) {
				t.Fatalf("Recover: err = %v, want a RecoveryError", err)
			}
			if len(re.Problems) != 1 || re.Problems[0].Item.Kind != ItemAppend {
				t.Errorf("problems = %+v, want the ledger only", re.Problems)
			}
			if got := read(t, w.ledger); got != tampered {
				t.Errorf("the ledger was changed to %q", got)
			}
		})
	}
}

// IC-W18: 확정 도중 대상이 밖에서 바뀌면 덮어쓰지 않고 복구 필요로 멈춘다. 뒤의 항목(원장, 영수증)도
// 적용하지 않는다. 상태 보기는 파일 · 기대
// 해시 · 지금 해시 · 준비 파일을 보이고, 진단 묶음을 0600으로 새로 내보낸다. 그동안 그 디렉토리의
// 다른 저장은 막힌다.
func TestExternalChangeNeedsRecovery(t *testing.T) {
	w := newWorld(t)
	stop := stopAt{point: FaultApply, path: "2"}
	s := w.open(t, stop.hook)
	if err := s.Do(w.tx(), time.Second); !errors.Is(err, errStop) {
		t.Fatalf("err = %v, want the injected stop", err)
	}
	s.Close()
	const theirs = "edited by hand\n"
	if err := os.WriteFile(w.session, []byte(theirs), 0o600); err != nil {
		t.Fatal(err)
	}

	again := w.open(t, nil)
	err := again.Recover(time.Second)
	var re *RecoveryError
	if !errors.As(err, &re) || len(re.Problems) != 1 || re.Problems[0].Item.Path != again.rel(w.session) {
		t.Fatalf("Recover: err = %v, want one problem on the session", err)
	}
	if !strings.Contains(err.Error(), "-recover-status") {
		t.Errorf("the error does not point at -recover-status: %v", err)
	}
	if read(t, w.session) != theirs {
		t.Error("recovery overwrote the change made outside pqcaton")
	}
	if read(t, w.plan) != planV2 {
		t.Error("the plan, replaced before the stop, should stay replaced")
	}
	if read(t, w.ledger) != ledgerBase || read(t, filepath.Join(w.receiptDir, "s-1.json")) != "<absent>" {
		t.Error("items after the mismatch were applied; recovery must stop at the first one")
	}

	rep, err := again.Status()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, it := range rep.Records[0].Items {
		if it.AbsPath == w.session {
			found = true
			if it.Verdict != VerdictMismatch || it.Current != Sum([]byte(theirs)) || it.After != Sum([]byte(sessV2)) || !it.PreparedMatches {
				t.Errorf("session status = %+v", it)
			}
		}
	}
	if !found {
		t.Fatal("the status does not list the session")
	}

	out := filepath.Join(t.TempDir(), "diag.json")
	if err := again.ExportDiagnostics(out); err != nil {
		t.Fatal(err)
	}
	if m := mode(t, out); m != 0o600 {
		t.Errorf("diagnostics mode = %#o, want 0600", m)
	}
	var back Report
	if err := json.Unmarshal([]byte(read(t, out)), &back); err != nil || len(back.Records) != 1 {
		t.Errorf("diagnostics do not read back: %v", err)
	}
	if strings.Contains(read(t, out), strings.TrimSpace(theirs)) {
		t.Error("diagnostics carry file contents; they should carry paths and hashes only")
	}
	if err := again.ExportDiagnostics(out); err == nil {
		t.Error("ExportDiagnostics overwrote an existing file")
	}

	other := Tx{Kind: "decl", Org: "local", SessionID: "d-1",
		Replace: []Change{{Path: filepath.Join(w.data, "declaration.json"), Content: []byte("{}")}}}
	if err := again.Do(other, time.Second); !errors.Is(err, ErrNeedsRecovery) {
		t.Errorf("another save in the same directory: err = %v, want ErrNeedsRecovery", err)
	}
}

// IC-W19: 같은 원장에 동시에 쓰는 요청 여럿은 차례로 들어간다. 줄이 섞이거나 빠지거나 두 번 들어가지 않는다.
func TestConcurrentAppends(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- s.Do(Tx{Kind: "review", Org: "local", SessionID: fmt.Sprint("s-", i),
				Replace: []Change{{Path: w.session, Content: []byte(fmt.Sprintf("session-%d\n", i))}},
				Append:  []Addition{{Path: w.ledger, Data: []byte(fmt.Sprintf("{\"g\":%d}\n", i))}},
			}, 10*time.Second)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSuffix(read(t, w.ledger), "\n"), "\n")
	if lines[0] != strings.TrimSpace(ledgerBase) {
		t.Errorf("the first line changed: %q", lines[0])
	}
	got := lines[1:]
	sort.Strings(got)
	var want []string
	for i := 0; i < n; i++ {
		want = append(want, fmt.Sprintf("{\"g\":%d}", i))
	}
	sort.Strings(want)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("ledger lines = %v, want each of %v once", got, want)
	}
	w.clean(t, s)
}

// IC-W45: Create는 없는 파일만 만든다. 하나라도 이미 있으면 ErrExists이고 아무것도 쓰지 않는다(있는 파일은
// 그대로, 없던 파일도 생기지 않는다). 끝나면 기록도 준비 파일도 남지 않는다.
func TestCreateNeverOverwrites(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	decl := filepath.Join(w.data, "declaration.json")
	layer := filepath.Join(w.data, "layer.csv")
	if err := s.Create([]Change{{Path: decl, Content: []byte("decl-v1\n")}}, time.Second); err != nil {
		t.Fatal(err)
	}
	if got := read(t, decl); got != "decl-v1\n" {
		t.Fatalf("created %q", got)
	}
	err := s.Create([]Change{{Path: layer, Content: []byte("layer\n")}, {Path: decl, Content: []byte("decl-v2\n")}}, time.Second)
	if !errors.Is(err, ErrExists) {
		t.Fatalf("err %v, want ErrExists", err)
	}
	if got := read(t, decl); got != "decl-v1\n" {
		t.Errorf("the existing file became %q", got)
	}
	if _, err := os.Stat(layer); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused create still wrote the other file: %v", err)
	}
	w.clean(t, s)
}
