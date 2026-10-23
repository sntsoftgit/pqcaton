// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func (w world) check(t *testing.T) Check {
	t.Helper()
	in := []Input{{Name: "session", Path: w.session}, {Name: "plan", Path: w.plan}}
	r, err := Rev(in)
	if err != nil {
		t.Fatal(err)
	}
	return Check{Inputs: in, Rev: r}
}

// IC-W30: rev는 저장 잠금 안에서 견준다. 같은 오래된 화면에서 동시에 저장하면 하나만 쓰이고 다른
// 하나는 「화면이 오래됐다」로 거부된다. rev가 없거나 다르면 쓰지 않는다. 확정은 확인한 payload와
// 지금 만든 payload가 다르면 쓰지 않는다. 잠근 대상 밖은 쓰지 않는다.
func TestCheckedWritesCompareRevUnderTheLock(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	chk := w.check(t)

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = s.SaveChecked(sessA, []string{w.session}, chk, 5*time.Second, func() ([]Change, error) {
				return []Change{{Path: w.session, Content: []byte{byte('a' + i), '\n'}}}, nil
			})
		}(i)
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, ErrStale):
			t.Errorf("err = %v, want nil or ErrStale", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d saves from the same screen went through, want exactly 1", won)
	}

	before := read(t, w.session)
	for name, c := range map[string]Check{
		"stale rev": chk,
		"no rev":    {Inputs: chk.Inputs},
	} {
		err := s.SaveChecked(sessA, []string{w.session}, c, time.Second, func() ([]Change, error) {
			return []Change{{Path: w.session, Content: []byte("late\n")}}, nil
		})
		if !errors.Is(err, ErrStale) {
			t.Errorf("%s: err = %v, want ErrStale", name, err)
		}
	}
	if read(t, w.session) != before {
		t.Error("a refused save changed the session")
	}

	err := s.SaveChecked(sessA, []string{w.session}, w.check(t), time.Second, func() ([]Change, error) {
		return []Change{{Path: w.plan, Content: []byte("sneaky\n")}}, nil
	})
	if err == nil || read(t, w.plan) != planV1 {
		t.Errorf("a write outside the locked targets: err = %v, plan = %q", err, read(t, w.plan))
	}

	targets := []string{w.plan, w.session, w.ledger}
	confirmed := Sum([]byte("what the confirm page showed"))
	_, err = s.FinalizeChecked(sessA, targets, w.check(t), confirmed, time.Second, func() (Parts, error) {
		return Parts{Replace: []Change{{Path: w.plan, Content: []byte(planV2)}}, PayloadSHA256: Sum([]byte("something else"))}, nil
	})
	if !errors.Is(err, ErrStale) || read(t, w.plan) != planV1 {
		t.Fatalf("finalize with a different payload: err = %v, plan = %q", err, read(t, w.plan))
	}
	build := func() (Parts, error) {
		return Parts{Replace: []Change{{Path: w.plan, Content: []byte(planV2)}},
			Append: []Addition{{Path: w.ledger, Data: []byte(ledgerAdd)}}, PayloadSHA256: confirmed}, nil
	}
	old := w.check(t)
	if out, err := s.FinalizeChecked(sessA, targets, old, confirmed, time.Second, build); err != nil || out.Status != OutcomeDone {
		t.Fatalf("finalize = %+v, %v", out, err)
	}
	// 확정이 파일을 바꿨으므로 같은 확인 화면의 rev는 이제 낡았다. 그래도 같은 내용이면 이전 결과다.
	if out, err := s.FinalizeChecked(sessA, targets, old, confirmed, time.Second, build); err != nil || out.Status != OutcomeAlready {
		t.Fatalf("the same confirmed finalize again = %+v, %v; want %s", out, err, OutcomeAlready)
	}
}

// IC-W31: rev는 링크가 가리키는 파일의 내용까지 본다. 링크는 그대로인 채 대상만 바뀌어도, 링크가
// 끊겨도 rev가 바뀐다. 입력 디렉토리 자체가 링크면 가리키는 디렉토리를 훑는다. 안쪽의 디렉토리
// 링크는 따라가지 않고 거부한다.
func TestRevFollowsLinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	results := filepath.Join(dir, "results")
	for _, d := range []string{real, results} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(real, "node-a.json")
	if err := os.WriteFile(target, []byte("{\"a\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(results, "node-a.json")); err != nil {
		t.Fatal(err)
	}
	in := []Input{{Name: "results", Path: results, Dir: true}}
	first := mustRev(t, in)
	if err := os.WriteFile(target, []byte("{\"a\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := mustRev(t, in)
	if second == first {
		t.Error("the link's target changed but rev did not")
	}
	os.Remove(target)
	if mustRev(t, in) == second {
		t.Error("the link broke but rev did not change")
	}

	linked := filepath.Join(dir, "linked-results")
	if err := os.Symlink(real, linked); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("{\"a\":3}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	viaLink := []Input{{Name: "results", Path: linked, Dir: true}}
	a := mustRev(t, viaLink)
	if err := os.WriteFile(target, []byte("{\"a\":4}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if mustRev(t, viaLink) == a {
		t.Error("a file inside a linked input directory changed but rev did not")
	}

	if err := os.Symlink(real, filepath.Join(results, "nested")); err != nil {
		t.Fatal(err)
	}
	if _, err := Rev(in); !errors.Is(err, ErrUnsupported) {
		t.Errorf("a link to a directory inside an input: err = %v, want ErrUnsupported", err)
	}
}

// IC-W32: 확정의 앞뒤는 영수증 순번으로 가린다. 벽시각이 거꾸로 적혀 있어도 뒤에 확정한 세션이
// 「대체한 쪽」이다.
func TestSupersededUsesSequence(t *testing.T) {
	w := newWorld(t)
	s := w.open(t, nil)
	a, err := w.finalize(s, sessA, planV2, ledgerAdd)
	if err != nil {
		t.Fatal(err)
	}
	sessB := Identity{Kind: "review", Org: "local", SessionID: "session-b"}
	b, err := w.finalize(s, sessB, "plan-b\n", "{\"b\":1}\n")
	if err != nil {
		t.Fatal(err)
	}
	if a.Receipt.Seq < 1 || b.Receipt.Seq <= a.Receipt.Seq {
		t.Fatalf("sequence numbers a=%d b=%d, want b after a", a.Receipt.Seq, b.Receipt.Seq)
	}
	// 시계가 뒤로 갔던 것처럼 적는다: 앞 세션은 미래, 뒤 세션은 과거.
	skew := func(id Identity, at time.Time) {
		p := s.receiptPath(id)
		var r Receipt
		if err := json.Unmarshal([]byte(read(t, p)), &r); err != nil {
			t.Fatal(err)
		}
		r.Finished = at
		out, _ := json.Marshal(r)
		if err := os.WriteFile(p, out, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	skew(sessA, time.Now().Add(24*time.Hour))
	skew(sessB, time.Now().Add(-24*time.Hour))
	out, err := w.finalize(s, sessA, planV2, ledgerAdd)
	if err != nil || out.Status != OutcomeSuperseded || out.By == nil || out.By.Identity() != sessB {
		t.Fatalf("outcome = %+v, %v; want superseded by %s", out, err, sessB)
	}
}
