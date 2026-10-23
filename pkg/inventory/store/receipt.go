// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// 영수증. 확정이 **한 번만** 일어나게 한다. 재시작해도 남는다.
//
// 동일성은 종류 + 조직 + 세션 ID뿐이다. payload는 동일성에 넣지 않고 영수증에 해시로 적는다.
// 같은 세션을 다시 확정하면
//
//   - payload가 같으면 출력과 원장 구간을 확인하고 이전 결과를 돌려준다(다시 쓰지 않는다).
//   - 출력이 다른 세션의 후속 확정으로 바뀌었으면 「대체됨」으로 알린다. 오류가 아니다.
//   - 그 밖에 어긋나면 복구 필요다.
//   - payload가 다르면 거부한다. 새 세션을 열어야 한다.
//
// 확정된 세션은 일반 저장도 막는다. 확정한 뒤에 세션이 바뀌면 영수증이 가리키는 것과 화면이 다르다.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

const (
	receiptDir    = "receipts"
	receiptFormat = 1
)

// 확정 결과.
const (
	OutcomeDone       = "done"
	OutcomeAlready    = "already"
	OutcomeSuperseded = "superseded"
)

// ErrSessionFinalized: 이미 확정된 세션이다. 다른 내용으로 다시 확정하거나 저장할 수 없다.
var ErrSessionFinalized = errors.New("this session is already finalized")

var namePart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Identity: 확정의 동일성.
type Identity struct {
	Kind, Org, SessionID string
}

func (id Identity) String() string { return id.Kind + "/" + id.Org + "/" + id.SessionID }

func (id Identity) check() error {
	for _, p := range []string{id.Kind, id.Org, id.SessionID} {
		if !namePart.MatchString(p) {
			return fmt.Errorf("store: %q is not a usable kind, organization or session id", p)
		}
	}
	return nil
}

// ReceiptFile: 확정이 쓴 출력 하나.
type ReceiptFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// LedgerRange: 확정이 원장에 더한 구간.
type LedgerRange struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
	SHA256 string `json:"sha256"`
}

// Receipt: 확정 영수증.
type Receipt struct {
	Format        int           `json:"format"`
	Kind          string        `json:"kind"`
	Org           string        `json:"org"`
	SessionID     string        `json:"session_id"`
	PayloadSHA256 string        `json:"payload_sha256"`
	TxID          string        `json:"txid"`
	Finished      time.Time     `json:"finished"`
	Outputs       []ReceiptFile `json:"outputs"`
	Ledger        []LedgerRange `json:"ledger"`
}

// Identity: 영수증의 동일성.
func (r Receipt) Identity() Identity { return Identity{r.Kind, r.Org, r.SessionID} }

// Outcome: 확정의 결과.
type Outcome struct {
	Status  string
	Receipt Receipt
	// By: 대체됨이면 출력을 바꾼 뒤 세션의 영수증.
	By *Receipt
}

// Finalize: 세션을 확정한다. Replace는 출력(계획·정책·세션), Append는 원장에 더할 판정이다.
func (s *State) Finalize(id Identity, payloadSHA256 string, replace []Change, add []Addition, wait time.Duration) (Outcome, error) {
	if err := id.check(); err != nil {
		return Outcome{}, err
	}
	tx := Tx{Kind: id.Kind, Org: id.Org, SessionID: id.SessionID, PayloadSHA256: payloadSHA256, Replace: replace, Append: add}
	var out Outcome
	err := s.withLocks(tx.dirs(), wait, func() error {
		prev, err := s.readReceipt(id)
		if err != nil {
			return err
		}
		if prev != nil {
			if prev.PayloadSHA256 != payloadSHA256 {
				return fmt.Errorf("%w (%s) with different content; open a new session to finalize again", ErrSessionFinalized, id)
			}
			out, err = s.verifyReceipt(*prev)
			return err
		}
		rc, err := s.newReceipt(tx)
		if err != nil {
			return err
		}
		b, err := json.MarshalIndent(rc, "", "  ")
		if err != nil {
			return err
		}
		p := s.receiptPath(id)
		if err := mkdirPrivate(filepath.Dir(p)); err != nil {
			return err
		}
		tx.Create = []Change{{Path: p, Content: append(b, '\n')}}
		rec, err := s.plan(tx)
		if err != nil {
			return err
		}
		// 영수증의 txid는 기록의 txid와 같아야 한다. plan이 ID를 만들므로 영수증을 다시 쓴다.
		rc.TxID = rec.TxID
		if b, err = json.MarshalIndent(rc, "", "  "); err != nil {
			return err
		}
		tx.Create[0].Content = append(b, '\n')
		last := len(rec.Items) - 1
		rec.Items[last].After = Sum(tx.Create[0].Content)
		if err := s.run1(rec, tx); err != nil {
			return err
		}
		out = Outcome{Status: OutcomeDone, Receipt: rc}
		return nil
	})
	return out, err
}

// Save: 확정 전 세션의 일반 저장. 확정된 세션이면 ErrSessionFinalized.
func (s *State) Save(id Identity, replace []Change, wait time.Duration) error {
	if err := id.check(); err != nil {
		return err
	}
	tx := Tx{Kind: id.Kind, Org: id.Org, SessionID: id.SessionID, Replace: replace}
	return s.withLocks(tx.dirs(), wait, func() error {
		prev, err := s.readReceipt(id)
		if err != nil {
			return err
		}
		if prev != nil {
			return fmt.Errorf("%w (%s); open a new session to make changes", ErrSessionFinalized, id)
		}
		rec, err := s.plan(tx)
		if err != nil {
			return err
		}
		return s.run1(rec, tx)
	})
}

// Finalized: 이 세션의 영수증. 확정 전이면 nil.
func (s *State) Finalized(id Identity) (*Receipt, error) {
	if err := id.check(); err != nil {
		return nil, err
	}
	return s.readReceipt(id)
}

// newReceipt: 쓰기 전 지금 상태로 영수증을 만든다. 잠금 안에서 부른다.
func (s *State) newReceipt(tx Tx) (Receipt, error) {
	rc := Receipt{Format: receiptFormat, Kind: tx.Kind, Org: tx.Org, SessionID: tx.SessionID,
		PayloadSHA256: tx.PayloadSHA256, Finished: time.Now().UTC()}
	for _, c := range tx.Replace {
		abs, err := filepath.Abs(c.Path)
		if err != nil {
			return rc, err
		}
		rc.Outputs = append(rc.Outputs, ReceiptFile{Path: s.rel(abs), SHA256: Sum(c.Content)})
	}
	for _, a := range tx.Append {
		tg, err := s.Inspect(a.Path)
		if err != nil {
			return rc, err
		}
		rc.Ledger = append(rc.Ledger, LedgerRange{Path: s.rel(tg.Path), Offset: tg.Size,
			Length: int64(len(a.Data)), SHA256: Sum(a.Data)})
	}
	return rc, nil
}

// verifyReceipt: 같은 payload로 다시 확정했을 때. 출력과 원장 구간이 영수증과 맞으면 이전 결과다.
func (s *State) verifyReceipt(rc Receipt) (Outcome, error) {
	var probs []Problem
	var by *Receipt
	for _, o := range rc.Outputs {
		cur, size, err := currentHash(s.abs(o.Path))
		if err != nil {
			return Outcome{}, err
		}
		if cur == o.SHA256 {
			continue
		}
		later, err := s.laterWriter(rc, o.Path, cur)
		if err != nil {
			return Outcome{}, err
		}
		if later != nil {
			by = later
			continue
		}
		probs = append(probs, Problem{TxID: rc.TxID, Item: Item{Kind: ItemReplace, Path: o.Path, After: o.SHA256},
			Current: cur, Size: size, Why: "the output differs from what this session finalized, and no later session wrote it"})
	}
	for _, l := range rc.Ledger {
		ok, err := rangeHas(s.abs(l.Path), l.Offset, l.Length, l.SHA256)
		if err != nil {
			return Outcome{}, err
		}
		if !ok {
			probs = append(probs, Problem{TxID: rc.TxID, Item: Item{Kind: ItemAppend, Path: l.Path, BaseSize: l.Offset,
				Length: l.Length, After: l.SHA256}, Why: "the ledger no longer holds this session's entries where the receipt says"})
		}
	}
	if len(probs) > 0 {
		return Outcome{}, &RecoveryError{Problems: probs}
	}
	if by != nil {
		return Outcome{Status: OutcomeSuperseded, Receipt: rc, By: by}, nil
	}
	return Outcome{Status: OutcomeAlready, Receipt: rc}, nil
}

// laterWriter: 그 출력을 지금 내용으로 쓴 뒤 세션의 영수증.
func (s *State) laterWriter(rc Receipt, path, cur string) (*Receipt, error) {
	if cur == "" {
		return nil, nil
	}
	all, err := s.receipts()
	if err != nil {
		return nil, err
	}
	var found *Receipt
	for i := range all {
		r := all[i]
		if r.Identity() == rc.Identity() || r.Finished.Before(rc.Finished) {
			continue
		}
		for _, o := range r.Outputs {
			if o.Path == path && o.SHA256 == cur && (found == nil || r.Finished.After(found.Finished)) {
				found = &all[i]
			}
		}
	}
	return found, nil
}

// receipts: 영수증 전부.
func (s *State) receipts() ([]Receipt, error) {
	root := filepath.Join(s.Dir, receiptDir)
	var out []Receipt
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && p == root {
			return filepath.SkipDir
		}
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(p) != ".json" || d.Name()[0] == '.' {
			return nil
		}
		r, err := s.parseReceipt(p)
		if err != nil {
			return err
		}
		out = append(out, *r)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Finished.Before(out[j].Finished) })
	return out, err
}

func (s *State) receiptPath(id Identity) string {
	return filepath.Join(s.Dir, receiptDir, id.Kind, id.Org, id.SessionID+".json")
}

func (s *State) readReceipt(id Identity) (*Receipt, error) {
	p := s.receiptPath(id)
	if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	r, err := s.parseReceipt(p)
	if err != nil {
		return nil, err
	}
	if r.Identity() != id {
		return nil, fmt.Errorf("receipt %s names %s; keep a copy and run pqcaton-ui -recover-status", p, r.Identity())
	}
	return r, nil
}

func (s *State) parseReceipt(p string) (*Receipt, error) {
	b, err := s.readPrivate(p)
	if err != nil {
		return nil, err
	}
	var r Receipt
	if err := json.Unmarshal(b, &r); err != nil || r.Format != receiptFormat {
		return nil, fmt.Errorf("receipt %s is damaged; keep a copy and run pqcaton-ui -recover-status", p)
	}
	return &r, nil
}

// withLocks: 대상 디렉토리 잠금 → 그 디렉토리의 남은 기록 복구 → fn.
func (s *State) withLocks(dirs []string, wait time.Duration, fn func() error) error {
	if s.run == nil {
		return errors.New("store: writing before AcquireRun")
	}
	if len(dirs) == 0 {
		return errors.New("store: a transaction must change at least one file outside the state directory")
	}
	release, err := s.LockTargets(dirs, wait)
	if err != nil {
		return err
	}
	defer release()
	if err := s.recoverIn(dirs); err != nil {
		return err
	}
	return fn()
}

// mkdirPrivate: 상태 디렉토리 안에 0700 디렉토리를 만든다(사이 단계까지).
func mkdirPrivate(dir string) error {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	return os.Chmod(dir, dirPerm)
}

// rangeHas: 파일의 [off, off+n) 구간이 그 해시인가.
func rangeHas(p string, off, n int64, want string) (bool, error) {
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	b := make([]byte, n)
	if _, err := f.ReadAt(b, off); err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}
	return Sum(b) == want, nil
}
