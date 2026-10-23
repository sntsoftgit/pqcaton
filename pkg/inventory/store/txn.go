// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// 트랜잭션. 파일 여럿(계획, 백업, 판정 원장, 영수증)을 한 번에 바꾼다.
//
// 순서가 요점이다.
//
//  1. PREPARING 기록을 쓴다. 앞으로 만들 준비 파일을 **모두 미리 열거한다.**
//  2. 준비 파일을 만든다. 바꿀 내용, 백업할 내용, 원장에 더할 바이트를 전부 여기서 쓰고 fsync한다.
//  3. 기록을 PREPARED로 바꾼다. **여기가 확정 시점이다.** 이 뒤로는 되돌리지 않고 끝까지 간다.
//  4. 항목을 차례로 적용한다. 항목은 서로 독립이고, 각자 적용 전과 후의 해시를 가진다.
//  5. 기록을 지운다.
//
// 기록이 준비 파일보다 먼저인 까닭: 기록 없는 준비 파일은 누가 만든 것인지 가릴 수 없다.
// 그래서 기록에 없는 준비 파일을 만나면 지우지 않고 멈춘다(CheckOrphans).
//
// 복구는 항목마다 지금 파일의 해시를 보고 판정한다. 적용 후 해시면 끝난 것이고, 적용 전 해시면
// 적용하고, 둘 다 아니면 **고치지 않고 멈춘다(복구 필요).** 남이 바꾼 파일 위에 덮어쓰면 그
// 변경이 사라진다.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	txnDir    = "txn"
	txnFormat = 1

	statePreparing = "PREPARING"
	statePrepared  = "PREPARED"
)

// 항목 종류.
const (
	ItemBackup  = "backup"
	ItemReplace = "replace"
	ItemAppend  = "append"
	ItemCreate  = "create"
)

// 트랜잭션의 실패 주입 자리.
const (
	FaultRecordPreparing = "tx.record.preparing"
	FaultPrepared        = "tx.prepare"
	FaultRecordPrepared  = "tx.record.prepared"
	FaultApply           = "tx.apply"
	FaultAppendHalf      = "tx.append.half"
	FaultDone            = "tx.done"
)

// ErrNeedsRecovery: 파일이 기록과 맞지 않아 멈췄다. 자동으로 고치지 않는다.
var ErrNeedsRecovery = errors.New("recovery needed")

// Change: 통째로 바꿀 파일. Backup이면 지금 내용을 <경로>.prev로 남긴다.
type Change struct {
	Path    string
	Content []byte
	Backup  bool
}

// Addition: 원장 끝에 더할 바이트.
type Addition struct {
	Path string
	Data []byte
}

// Tx: 한 번에 바꿀 것. Create는 없어야 하는 파일(영수증)이다.
type Tx struct {
	Kind, Org, SessionID string
	PayloadSHA256        string
	Replace              []Change
	Append               []Addition
	Create               []Change
}

// Item: 기록에 적히는 적용 항목. 경로는 상태 디렉토리의 부모 안이면 상대 경로다.
type Item struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Prepared string `json:"prepared"`
	// Before: 적용 전 해시. 빈 문자열은 「파일이 없다」이다.
	Before string `json:"before,omitempty"`
	// After: 적용 후 해시. 원장 추가는 더한 구간의 해시다.
	After    string `json:"after"`
	BaseSize int64  `json:"base_size,omitempty"`
	BaseHash string `json:"base_hash,omitempty"`
	Length   int64  `json:"length,omitempty"`
}

// Record: 트랜잭션 기록.
type Record struct {
	Format        int       `json:"format"`
	TxID          string    `json:"txid"`
	State         string    `json:"state"`
	Kind          string    `json:"kind"`
	Org           string    `json:"org"`
	SessionID     string    `json:"session_id"`
	PayloadSHA256 string    `json:"payload_sha256"`
	Created       time.Time `json:"created"`
	Prepared      []string  `json:"prepared"`
	Items         []Item    `json:"items"`
}

// Problem: 복구 필요인 항목 하나.
type Problem struct {
	TxID string `json:"txid"`
	Item Item   `json:"item"`
	// Current: 지금 파일의 해시(원장은 기준 길이 뒤 구간이 아니라 전체). 없으면 빈 문자열.
	Current string `json:"current"`
	Size    int64  `json:"size"`
	Why     string `json:"why"`
}

// RecoveryError: 복구 필요. 무엇이 어긋났는지 담는다.
type RecoveryError struct{ Problems []Problem }

func (e *RecoveryError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d file(s) do not match the transaction record", ErrNeedsRecovery, len(e.Problems))
	for _, p := range e.Problems {
		fmt.Fprintf(&b, "\n  %s (%s): %s", p.Item.Path, p.Item.Kind, p.Why)
	}
	b.WriteString("\n  Nothing was changed. Run pqcaton-ui -recover-status to see the details.")
	return b.String()
}

func (e *RecoveryError) Unwrap() error { return ErrNeedsRecovery }

// Do: 트랜잭션 하나를 끝까지 한다. 대상 디렉토리 잠금을 wait까지 기다려 잡고, 그 안에서 이 디렉토리에
// 걸린 남은 기록을 먼저 복구한 뒤 쓴다. 세션에 묶인 저장과 확정은 Save와 Finalize를 쓴다.
func (s *State) Do(tx Tx, wait time.Duration) error {
	return s.withLocks(tx.dirs(), wait, func() error {
		rec, err := s.plan(tx)
		if err != nil {
			return err
		}
		return s.run1(rec, tx)
	})
}

// Create: 없는 파일만 만든다(선언 · 계층 CSV처럼 화면이 처음 만드는 파일). 대상 디렉토리를 잠그고 남은 기록을
// 복구한 뒤 만든다. **하나라도 이미 있으면 ErrExists이고 아무것도 쓰지 않는다** — 덮어쓰지 않는다.
func (s *State) Create(files []Change, wait time.Duration) error {
	var paths []string
	for _, c := range files {
		paths = append(paths, c.Path)
	}
	return s.withLocks(dirsOf(paths), wait, func() error {
		rec, err := s.plan(Tx{Kind: "create", Create: files})
		if err != nil {
			return err
		}
		return s.run1(rec, Tx{Kind: "create", Create: files})
	})
}

func (tx Tx) dirs() []string {
	var out []string
	for _, c := range tx.Replace {
		out = append(out, filepath.Dir(c.Path))
	}
	for _, a := range tx.Append {
		out = append(out, filepath.Dir(a.Path))
	}
	return out
}

// plan: 대상을 살펴 기록을 만든다. 아직 아무것도 쓰지 않는다.
func (s *State) plan(tx Tx) (*Record, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	rec := &Record{Format: txnFormat, TxID: id, State: statePreparing, Kind: tx.Kind, Org: tx.Org,
		SessionID: tx.SessionID, PayloadSHA256: tx.PayloadSHA256, Created: time.Now().UTC()}
	add := func(it Item) {
		rec.Items = append(rec.Items, it)
		rec.Prepared = append(rec.Prepared, it.Prepared)
	}
	var replaces []Item
	for _, c := range tx.Replace {
		tg, err := s.Inspect(c.Path)
		if err != nil {
			return nil, err
		}
		if c.Backup && tg.Exists {
			bk, err := s.Inspect(tg.Path + ".prev")
			if err != nil {
				return nil, err
			}
			add(Item{Kind: ItemBackup, Path: s.rel(bk.Path), Prepared: s.rel(PreparedPath(bk.Path, id)),
				Before: bk.SHA256, After: tg.SHA256})
		}
		replaces = append(replaces, Item{Kind: ItemReplace, Path: s.rel(tg.Path),
			Prepared: s.rel(PreparedPath(tg.Path, id)), Before: tg.SHA256, After: Sum(c.Content)})
	}
	for _, it := range replaces {
		add(it)
	}
	for _, a := range tx.Append {
		tg, err := s.Inspect(a.Path)
		if err != nil {
			return nil, err
		}
		base := Sum(nil)
		if tg.Exists {
			base = tg.SHA256
		}
		add(Item{Kind: ItemAppend, Path: s.rel(tg.Path), Prepared: s.rel(PreparedPath(tg.Path, id)),
			BaseSize: tg.Size, BaseHash: base, After: Sum(a.Data), Length: int64(len(a.Data))})
	}
	for _, c := range tx.Create {
		abs, err := filepath.Abs(c.Path)
		if err != nil {
			return nil, err
		}
		if _, err := os.Lstat(abs); err == nil {
			return nil, fmt.Errorf("%w: %s", ErrExists, abs)
		}
		add(Item{Kind: ItemCreate, Path: s.rel(abs), Prepared: s.rel(PreparedPath(abs, id)), After: Sum(c.Content)})
	}
	return rec, nil
}

// run1: 기록 → 준비 → PREPARED → 적용 → 기록 삭제.
func (s *State) run1(rec *Record, tx Tx) error {
	s.setActive(rec.TxID, true)
	defer s.setActive(rec.TxID, false)
	if err := s.fault(FaultRecordPreparing, ""); err != nil {
		return err
	}
	if err := s.writeRecord(rec); err != nil {
		return err
	}
	if err := s.prepareAll(rec, tx); err != nil {
		// 아직 확정 전이다. 기록에 열거된 것만 지우고 기록을 지운다. 지우다 실패하면 기록이 남아
		// 다음 복구가 다시 지운다.
		if aerr := s.abort(rec); aerr != nil {
			return fmt.Errorf("%w (and cleaning up failed: %v)", err, aerr)
		}
		return err
	}
	if err := s.fault(FaultRecordPrepared, ""); err != nil {
		return err
	}
	rec.State = statePrepared
	if err := s.writeRecord(rec); err != nil {
		return err
	}
	return s.finish(rec)
}

// prepareAll: 기록에 적은 준비 파일을 만든다.
func (s *State) prepareAll(rec *Record, tx Tx) error {
	contents := map[string][]byte{}
	for _, c := range tx.Replace {
		abs, _ := filepath.Abs(c.Path)
		contents[s.rel(abs)] = c.Content
	}
	for _, c := range tx.Create {
		abs, _ := filepath.Abs(c.Path)
		contents[s.rel(abs)] = c.Content
	}
	adds := map[string][]byte{}
	for _, a := range tx.Append {
		abs, _ := filepath.Abs(a.Path)
		adds[s.rel(abs)] = a.Data
	}
	for _, it := range rec.Items {
		var body []byte
		mode := filePerm
		switch it.Kind {
		case ItemBackup:
			src := strings.TrimSuffix(s.abs(it.Path), ".prev")
			b, err := readExact(src, it.After)
			if err != nil {
				return err
			}
			body = b
		case ItemReplace:
			body = contents[it.Path]
			if fi, err := os.Lstat(s.abs(it.Path)); err == nil {
				mode = fi.Mode().Perm()
			}
		case ItemAppend:
			body = adds[it.Path]
		case ItemCreate:
			body = contents[it.Path]
		}
		if err := s.Prepare(s.abs(it.Prepared), body, mode); err != nil {
			return err
		}
	}
	return s.fault(FaultPrepared, "")
}

// abort: PREPARING 기록을 걷는다. **기록에 열거된 준비 파일만** 지운다.
func (s *State) abort(rec *Record) error {
	for _, p := range rec.Prepared {
		if err := os.Remove(s.abs(p)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return s.removeRecord(rec.TxID)
}

// finish: PREPARED 기록의 항목을 끝까지 적용하고 기록을 지운다.
//
// 어긋난 항목을 만나면 **거기서 멈춘다.** 뒤의 항목, 특히 마지막의 영수증은 쓰지 않는다. 영수증은
// 「끝났다」는 증거이므로 어긋난 채로 남기면 안 된다.
func (s *State) finish(rec *Record) error {
	for i, it := range rec.Items {
		if err := s.fault(FaultApply, fmt.Sprint(i)); err != nil {
			return err
		}
		p, err := s.apply(rec.TxID, it)
		if err != nil {
			return err
		}
		if p != nil {
			return &RecoveryError{Problems: []Problem{*p}}
		}
	}
	if err := s.fault(FaultDone, ""); err != nil {
		return err
	}
	return s.removeRecord(rec.TxID)
}

// apply: 항목 하나. 끝났으면 아무것도 하지 않고, 적용 전이면 적용하고, 그 밖이면 Problem.
func (s *State) apply(txid string, it Item) (*Problem, error) {
	path, prep := s.abs(it.Path), s.abs(it.Prepared)
	if it.Kind == ItemAppend {
		return s.applyAppend(txid, it)
	}
	cur, size, err := currentHash(path)
	if err != nil {
		return nil, err
	}
	switch {
	case cur == it.After:
		return nil, removeIfExists(prep)
	case cur == it.Before:
		ok, err := hasHash(prep, it.After)
		if err != nil {
			return nil, err
		}
		if !ok {
			return &Problem{TxID: txid, Item: it, Current: cur, Size: size,
				Why: "the prepared copy is missing or changed"}, nil
		}
		if fi, err := os.Lstat(path); err == nil {
			if err := s.checkPlain(path, fi); err != nil {
				return nil, err
			}
		}
		return nil, s.Install(prep, path)
	}
	return &Problem{TxID: txid, Item: it, Current: cur, Size: size,
		Why: "the file was changed outside pqcaton since the transaction began"}, nil
}

// applyAppend: 원장 추가. **자르지 않는다.** 기준 길이까지가 그대로이고, 넘친 바이트가 준비한 추가분의
// 정확한 앞부분일 때만 남은 바이트를 이어 쓴다.
func (s *State) applyAppend(txid string, it Item) (*Problem, error) {
	path, prep := s.abs(it.Path), s.abs(it.Prepared)
	prob := func(why string) (*Problem, error) {
		cur, size, err := currentHash(path)
		if err != nil {
			return nil, err
		}
		return &Problem{TxID: txid, Item: it, Current: cur, Size: size, Why: why}, nil
	}
	f, err := os.OpenFile(path, os.O_RDONLY|noFollow, 0)
	var size int64
	var prefix, tail []byte
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if it.BaseSize != 0 {
			return prob("the ledger is gone")
		}
	case err != nil:
		return nil, err
	default:
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		if err := s.checkPlain(path, fi); err != nil {
			f.Close()
			return nil, err
		}
		size = fi.Size()
		if size < it.BaseSize {
			f.Close()
			return prob("the ledger is shorter than when the transaction began")
		}
		prefix = make([]byte, it.BaseSize)
		_, err = io.ReadFull(f, prefix)
		if err == nil {
			tail, err = io.ReadAll(f)
		}
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	if Sum(prefix) != it.BaseHash {
		return prob("the ledger was changed before the point this transaction appends at")
	}
	written := size - it.BaseSize
	if written == it.Length && Sum(tail) == it.After {
		return nil, removeIfExists(prep)
	}
	if written >= it.Length {
		return prob("the ledger has bytes after this transaction's entry that it did not write")
	}
	chunk, err := readExact(prep, it.After)
	if err != nil {
		return prob("the prepared entry is missing or changed")
	}
	if !bytes.Equal(tail, chunk[:written]) {
		return prob("the ledger ends with bytes that are not this transaction's entry")
	}
	if err := s.appendBytes(path, chunk[written:]); err != nil {
		return nil, err
	}
	return nil, removeIfExists(prep)
}

// appendBytes: O_APPEND로 이어 쓰고 fsync한다. 원장이 없으면 0600으로 만든다.
func (s *State) appendBytes(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|noFollow, filePerm)
	if err != nil {
		return err
	}
	if err := s.fault(FaultAppendHalf, path); err != nil {
		f.Write(b[:len(b)/2])
		f.Sync()
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

// Recover: 남은 기록을 모두 정리한다. 시작할 때 실행 잠금을 잡은 뒤 부른다.
// PREPARING은 걷고, PREPARED는 끝까지 간다. 복구 필요가 하나라도 있으면 RecoveryError.
func (s *State) Recover(wait time.Duration) error {
	recs, err := s.Records()
	if err != nil {
		return err
	}
	var dirs []string
	for _, r := range recs {
		dirs = append(dirs, s.recordDirs(r)...)
	}
	release, err := s.LockTargets(dirs, wait)
	if err != nil {
		return err
	}
	defer release()
	return s.recoverIn(nil)
}

// recoverIn: 기록을 정리한다. only가 비어 있지 않으면 그 디렉토리 안에만 걸린 기록만 본다(나머지는
// 잠그지 않았으므로 건드리지 않는다).
func (s *State) recoverIn(only []string) error {
	recs, err := s.Records()
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, d := range only {
		abs, _ := filepath.Abs(d)
		allowed[abs] = true
	}
	var probs []Problem
	for _, r := range recs {
		if s.isActive(r.TxID) {
			continue
		}
		if only != nil && !subset(s.recordDirs(r), allowed) {
			continue
		}
		switch r.State {
		case statePreparing:
			if err := s.abort(r); err != nil {
				return err
			}
		case statePrepared:
			err := s.finish(r)
			var re *RecoveryError
			if errors.As(err, &re) {
				probs = append(probs, re.Problems...)
				continue
			}
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("transaction record %s has an unknown state %q", r.TxID, r.State)
		}
	}
	if len(probs) > 0 {
		return &RecoveryError{Problems: probs}
	}
	return nil
}

func (s *State) recordDirs(r *Record) []string {
	var out []string
	for _, it := range r.Items {
		d := filepath.Dir(s.abs(it.Path))
		if !strings.HasPrefix(d+string(filepath.Separator), s.Dir+string(filepath.Separator)) && d != s.Dir {
			out = append(out, d)
		}
	}
	return out
}

func (s *State) setActive(txid string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		s.active = map[string]bool{}
	}
	if on {
		s.active[txid] = true
	} else {
		delete(s.active, txid)
	}
}

func (s *State) isActive(txid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[txid]
}

func subset(dirs []string, allowed map[string]bool) bool {
	for _, d := range dirs {
		if !allowed[d] {
			return false
		}
	}
	return true
}

// Records: 남은 트랜잭션 기록. 만든 시각 순이다.
func (s *State) Records() ([]*Record, error) {
	dir := filepath.Join(s.Dir, txnDir)
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Record
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") {
			continue
		}
		b, err := s.readPrivate(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var r Record
		if err := json.Unmarshal(b, &r); err != nil || r.Format != txnFormat || !idPattern.MatchString(r.TxID) ||
			name != r.TxID+".json" {
			return nil, fmt.Errorf("%w: transaction record %s; keep a copy and run pqcaton-ui -recover-status", ErrDamaged, filepath.Join(dir, name))
		}
		out = append(out, &r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, nil
}

// CheckOrphans: 기록에 없는 준비 파일이 있으면 멈춘다. **지우지 않는다.** 누가 만든 것인지 가릴 수 없다.
func (s *State) CheckOrphans(dirs []string) error {
	recs, err := s.Records()
	if err != nil {
		return err
	}
	listed := map[string]bool{}
	for _, r := range recs {
		for _, p := range r.Prepared {
			listed[s.abs(p)] = true
		}
	}
	seen := map[string]bool{}
	var orphans []string
	for _, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			return err
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		found, err := FindPrepared(abs)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		for p := range found {
			if !listed[p] {
				orphans = append(orphans, p)
			}
		}
	}
	if len(orphans) == 0 {
		return nil
	}
	sort.Strings(orphans)
	return fmt.Errorf("%w: these files look like pqcaton's prepared files but no transaction record lists them, so they were left alone:\n  %s\n  Move them out of the way after checking them, then start again",
		ErrNeedsRecovery, strings.Join(orphans, "\n  "))
}

// writeRecord: 기록을 임시 파일 → fsync → rename → 디렉토리 fsync로 쓴다.
func (s *State) writeRecord(r *Record) error {
	dir := filepath.Join(s.Dir, txnDir)
	if err := os.Mkdir(dir, dirPerm); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return replacePrivate(filepath.Join(dir, r.TxID+".json"), append(b, '\n'))
}

func (s *State) removeRecord(txid string) error {
	dir := filepath.Join(s.Dir, txnDir)
	if err := os.Remove(filepath.Join(dir, txid+".json")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncDir(dir)
}

// replacePrivate: 상태 디렉토리 안의 파일을 원자적으로 바꾼다(0600).
func replacePrivate(p string, content []byte) error {
	dir := filepath.Dir(p)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(p)+".new-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(name)
		}
	}()
	if err := tmp.Chmod(filePerm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, p); err != nil {
		return err
	}
	ok = true
	return syncDir(dir)
}

// rel: 상태 디렉토리의 부모 안이면 상대 경로, 밖이면 절대 경로. 상태 디렉토리를 데이터와 함께 옮겨도
// 기록과 영수증이 그대로 선다. 밖의 파일은 따라오지 않는다.
func (s *State) rel(abs string) string {
	parent := filepath.Dir(s.Dir)
	r, err := filepath.Rel(parent, abs)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return abs
	}
	return filepath.ToSlash(r)
}

func (s *State) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(filepath.Dir(s.Dir), filepath.FromSlash(p))
}

// currentHash: 지금 내용의 해시와 크기. 없으면 빈 문자열.
func currentHash(p string) (string, int64, error) {
	sum, size, err := hashFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", 0, nil
	}
	return sum, size, err
}

func hasHash(p, want string) (bool, error) {
	got, _, err := currentHash(p)
	if err != nil {
		return false, err
	}
	return got != "" && got == want, nil
}

// readExact: 내용이 그 해시일 때만 돌려준다.
func readExact(p, want string) ([]byte, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != want {
		return nil, fmt.Errorf("%s changed while it was being saved", p)
	}
	return b, nil
}

func removeIfExists(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
