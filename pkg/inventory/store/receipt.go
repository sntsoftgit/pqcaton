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
	Format        int    `json:"format"`
	Kind          string `json:"kind"`
	Org           string `json:"org"`
	SessionID     string `json:"session_id"`
	PayloadSHA256 string `json:"payload_sha256"`
	TxID          string `json:"txid"`
	// Seq: 확정 순번. 같은 출력을 쓴 확정끼리의 앞뒤는 이것으로 가린다. 벽시각은 되돌아가거나 같은
	// 값이 나올 수 있어 순서로 쓰지 않는다(Finished는 사람이 읽는 기록일 뿐이다).
	Seq      int64         `json:"seq"`
	Finished time.Time     `json:"finished"`
	Outputs  []ReceiptFile `json:"outputs"`
	Ledger   []LedgerRange `json:"ledger"`
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

// Check: 화면이 그려질 때 본 입력과 그때의 rev. 저장과 확정은 **잠금을 잡은 뒤** 이 rev를 다시
// 계산해 견준다. 처리기가 따로 견주고 저장을 부르면 그 사이에 다른 저장이 끼어든다.
//
// 비어 있으면(Inputs도 Rev도 없음) 견주지 않는다. 화면 없이 파일을 바로 읽어 쓰는 명령의 길이다.
type Check struct {
	Inputs []Input
	Rev    string
}

func (c Check) empty() bool { return len(c.Inputs) == 0 && c.Rev == "" }

// verify: 잠금 안에서 부른다.
func (c Check) verify() error {
	if c.empty() {
		return nil
	}
	if c.Rev == "" {
		return fmt.Errorf("%w: the request carries no rev", ErrStale)
	}
	now, err := Rev(c.Inputs)
	if err != nil {
		return err
	}
	if now != c.Rev {
		return fmt.Errorf("%w: the files changed after the screen was drawn", ErrStale)
	}
	return nil
}

// ErrStale: 화면이 본 것과 지금 파일이 다르다. 저장하지 않았다.
var ErrStale = errors.New("the screen is out of date")

// Parts: 확정이 쓸 것. build가 잠금 안에서 지금 파일을 읽어 만든다.
type Parts struct {
	Replace       []Change
	Append        []Addition
	PayloadSHA256 string
}

// FinalizeChecked: 세션을 확정한다. 순서는 잠금 → 남은 기록 복구 → 영수증 확인 → rev 확인 →
// build → payload 확인 → 쓰기이고, 모두 한 잠금 안에서 일어난다.
//
// targets는 쓸 파일 전부(출력과 원장)다. 잠글 디렉토리를 build 전에 알아야 한다. build가 targets 밖의
// 파일을 내면 쓰지 않는다. payloadSHA256은 확인 화면이 보여 준 내용의 해시다. 이미 영수증이 있으면
// rev는 보지 않는다. 확정이 세션 파일을 다시 쓰므로 rev가 달라져 있는 것이 정상이다.
func (s *State) FinalizeChecked(id Identity, targets []string, chk Check, payloadSHA256 string, wait time.Duration,
	build func() (Parts, error)) (Outcome, error) {
	if err := id.check(); err != nil {
		return Outcome{}, err
	}
	var out Outcome
	err := s.withLocks(dirsOf(targets), wait, func() error {
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
		if err := chk.verify(); err != nil {
			return err
		}
		parts, err := build()
		if err != nil {
			return err
		}
		if parts.PayloadSHA256 != payloadSHA256 {
			return fmt.Errorf("%w: the content to finalize is not what was confirmed", ErrStale)
		}
		tx := Tx{Kind: id.Kind, Org: id.Org, SessionID: id.SessionID, PayloadSHA256: payloadSHA256,
			Replace: parts.Replace, Append: parts.Append}
		if err := within(tx, targets); err != nil {
			return err
		}
		out, err = s.finalizeLocked(id, tx, chk)
		return err
	})
	return out, err
}

// Finalize: rev 없이 확정한다. 화면 없이 파일을 바로 읽어 쓰는 명령의 길이다.
func (s *State) Finalize(id Identity, payloadSHA256 string, replace []Change, add []Addition, wait time.Duration) (Outcome, error) {
	var targets []string
	for _, c := range replace {
		targets = append(targets, c.Path)
	}
	for _, a := range add {
		targets = append(targets, a.Path)
	}
	return s.FinalizeChecked(id, targets, Check{}, payloadSHA256, wait, func() (Parts, error) {
		return Parts{Replace: replace, Append: add, PayloadSHA256: payloadSHA256}, nil
	})
}

// finalizeLocked: 영수증을 마지막 항목으로 붙여 트랜잭션을 한다. 잠금 안에서 부른다.
func (s *State) finalizeLocked(id Identity, tx Tx, chk Check) (Outcome, error) {
	rc, err := s.newReceipt(tx)
	if err != nil {
		return Outcome{}, err
	}
	p := s.receiptPath(id)
	if err := mkdirPrivate(filepath.Dir(p)); err != nil {
		return Outcome{}, err
	}
	tx.Create = []Change{{Path: p}}
	rec, err := s.plan(tx)
	if err != nil {
		return Outcome{}, err
	}
	// 첫 기록을 쓰기 직전에 한 번 더 견준다(recheck).
	if err := chk.verify(); err != nil {
		return Outcome{}, err
	}
	// 영수증의 txid는 기록의 txid와 같아야 한다. plan이 ID를 만들므로 영수증은 그 뒤에 쓴다.
	rc.TxID = rec.TxID
	b, err := json.MarshalIndent(rc, "", "  ")
	if err != nil {
		return Outcome{}, err
	}
	tx.Create[0].Content = append(b, '\n')
	rec.Items[len(rec.Items)-1].After = Sum(tx.Create[0].Content)
	if err := s.run1(rec, tx); err != nil {
		return Outcome{}, err
	}
	return Outcome{Status: OutcomeDone, Receipt: rc}, nil
}

// SaveChecked: 확정 전 세션의 일반 저장. 잠금 → 남은 기록 복구 → 확정 여부 → rev 확인 → build →
// 쓰기가 한 잠금 안에서 일어난다. 확정된 세션이면 ErrSessionFinalized, 화면이 오래됐으면 ErrStale.
func (s *State) SaveChecked(id Identity, targets []string, chk Check, wait time.Duration, build func() ([]Change, error)) error {
	if err := id.check(); err != nil {
		return err
	}
	return s.withLocks(dirsOf(targets), wait, func() error {
		prev, err := s.readReceipt(id)
		if err != nil {
			return err
		}
		if prev != nil {
			return fmt.Errorf("%w (%s); open a new session to make changes", ErrSessionFinalized, id)
		}
		if err := chk.verify(); err != nil {
			return err
		}
		replace, err := build()
		if err != nil {
			return err
		}
		tx := Tx{Kind: id.Kind, Org: id.Org, SessionID: id.SessionID, Replace: replace}
		if err := within(tx, targets); err != nil {
			return err
		}
		rec, err := s.plan(tx)
		if err != nil {
			return err
		}
		// 첫 기록을 쓰기 직전에 한 번 더 견준다(recheck).
		if err := chk.verify(); err != nil {
			return err
		}
		return s.run1(rec, tx)
	})
}

// Move: 이름 바꾸기 하나. To는 없어야 한다.
type Move struct{ From, To string }

// MoveChecked: 여러 파일의 이름을 **한 트랜잭션으로** 바꾼다. 잠금 → 남은 기록 복구 → rev 확인 → 계획 → 쓰기가 한
// 잠금 안에서 일어난다. 새 이름마다 옛 내용을 만들고(Create: 이미 있으면 ErrExists이고 아무것도 하지 않는다), 다
// 만든 뒤에 옛 이름을 지운다(Remove). 도중에 끊기면 다음 복구가 끝까지 하거나(PREPARED) 걷어 낸다(PREPARING) —
// 일부만 옮겨진 채로 남지 않는다. From이 없는 Move는 건너뛴다. 옮길 것이 없으면 아무것도 하지 않는다.
func (s *State) MoveChecked(id Identity, moves []Move, chk Check, wait time.Duration) (int, error) {
	if err := id.check(); err != nil {
		return 0, err
	}
	var targets []string
	for _, m := range moves {
		targets = append(targets, m.From, m.To)
	}
	n := 0
	err := s.withLocks(dirsOf(targets), wait, func() error {
		if err := chk.verify(); err != nil {
			return err
		}
		tx := Tx{Kind: id.Kind, Org: id.Org, SessionID: id.SessionID}
		for _, m := range moves {
			fi, err := os.Lstat(m.From)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if err := s.checkPlain(m.From, fi); err != nil {
				return err
			}
			b, err := os.ReadFile(m.From)
			if err != nil {
				return err
			}
			tx.Create = append(tx.Create, Change{Path: m.To, Content: b})
			tx.Remove = append(tx.Remove, m.From)
		}
		if len(tx.Create) == 0 {
			return nil
		}
		rec, err := s.plan(tx)
		if err != nil {
			return err
		}
		if err := chk.verify(); err != nil {
			return err
		}
		n = len(tx.Create)
		return s.run1(rec, tx)
	})
	return n, err
}

// recheck: 처음 견준 뒤 build가 입력을 다시 읽는다. 잠금을 지키지 않는 작성자(손 편집)가 그 사이에
// 입력을 바꾸면, build는 바뀐 내용 위에 옛 화면의 폼 값을 얹고 plan은 바뀐 파일을 「적용 전」으로
// 적는다. 그래서 plan 뒤, 첫 기록을 쓰기 직전에 다시 견준다. 그 뒤에 바뀐 대상 파일은 plan이 적은
// 적용 전 해시가 막는다(복구 필요).

// Save: rev 없이 저장한다. 화면 없이 파일을 바로 읽어 쓰는 명령의 길이다.
func (s *State) Save(id Identity, replace []Change, wait time.Duration) error {
	var targets []string
	for _, c := range replace {
		targets = append(targets, c.Path)
	}
	return s.SaveChecked(id, targets, Check{}, wait, func() ([]Change, error) { return replace, nil })
}

// within: 트랜잭션이 잠근 대상 밖을 쓰지 않는가.
func within(tx Tx, targets []string) error {
	ok := map[string]bool{}
	for _, t := range targets {
		abs, err := filepath.Abs(t)
		if err != nil {
			return err
		}
		ok[abs] = true
	}
	var paths []string
	for _, c := range tx.Replace {
		paths = append(paths, c.Path)
	}
	for _, a := range tx.Append {
		paths = append(paths, a.Path)
	}
	if len(paths) == 0 {
		return errors.New("store: nothing to write")
	}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		if !ok[abs] {
			return fmt.Errorf("store: %s was not named as a target before locking", abs)
		}
	}
	return nil
}

func dirsOf(paths []string) []string {
	var out []string
	for _, p := range paths {
		out = append(out, filepath.Dir(p))
	}
	return out
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
	// 순번은 잠금 안에서 지금까지의 최댓값 + 1이다. 같은 출력을 쓰는 확정은 같은 디렉토리 잠금을
	// 지나므로 차례로 번호를 받는다. 출력이 겹치지 않는 확정끼리는 번호가 같을 수 있지만, 그때는
	// 앞뒤를 물을 일이 없다.
	all, err := s.receipts()
	if err != nil {
		return rc, err
	}
	for _, r := range all {
		if r.Seq >= rc.Seq {
			rc.Seq = r.Seq + 1
		}
	}
	if rc.Seq == 0 {
		rc.Seq = 1
	}
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
	superseded := false
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
		// 새 회차가 넘겨받은 출력이다(round.go). 그 뒤에 바뀐 것은 새 세션의 일이다.
		handed, err := s.handedOver(rc, o.Path)
		if err != nil {
			return Outcome{}, err
		}
		if handed {
			superseded = true
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
	if by != nil || superseded {
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
		if r.Identity() == rc.Identity() || r.Seq <= rc.Seq {
			continue
		}
		for _, o := range r.Outputs {
			if o.Path == path && o.SHA256 == cur && (found == nil || r.Seq > found.Seq) {
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
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
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
		return nil, fmt.Errorf("%w: receipt %s; keep a copy and run pqcaton-ui -recover-status", ErrDamaged, p)
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
