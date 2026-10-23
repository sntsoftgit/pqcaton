// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// 복구 상태 보기. **읽기만 한다.** 복구 필요는 자동으로 고치지 않으므로, 운영자가 무엇이 어긋났는지
// 보고 판단할 재료를 낸다(-recover-status). 일반 --force는 두지 않는다.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"time"
)

// 항목 판정.
const (
	VerdictDone     = "done"
	VerdictPending  = "pending"
	VerdictPartial  = "partial"
	VerdictMismatch = "mismatch"
)

// ItemStatus: 항목 하나의 지금 상태.
type ItemStatus struct {
	Item
	AbsPath         string `json:"abs_path"`
	Current         string `json:"current"`
	Size            int64  `json:"size"`
	PreparedPresent bool   `json:"prepared_present"`
	PreparedMatches bool   `json:"prepared_matches"`
	Verdict         string `json:"verdict"`
}

// RecordStatus: 남은 기록 하나.
type RecordStatus struct {
	Record *Record      `json:"record"`
	Items  []ItemStatus `json:"items"`
}

// Report: 상태 디렉토리의 복구 상태.
type Report struct {
	StateDir string         `json:"state_dir"`
	StateID  string         `json:"state_id"`
	Taken    time.Time      `json:"taken"`
	Records  []RecordStatus `json:"records"`
}

// Status: 남은 기록마다 파일의 지금 해시와 기대 해시, 준비 파일이 있는지를 모은다.
func (s *State) Status() (Report, error) {
	rep := Report{StateDir: s.Dir, StateID: s.ID, Taken: time.Now().UTC()}
	recs, err := s.Records()
	if err != nil {
		return rep, err
	}
	for _, r := range recs {
		rs := RecordStatus{Record: r}
		for _, it := range r.Items {
			st, err := s.itemStatus(it)
			if err != nil {
				return rep, err
			}
			rs.Items = append(rs.Items, st)
		}
		rep.Records = append(rep.Records, rs)
	}
	return rep, nil
}

func (s *State) itemStatus(it Item) (ItemStatus, error) {
	st := ItemStatus{Item: it, AbsPath: s.abs(it.Path)}
	cur, size, err := currentHash(st.AbsPath)
	if err != nil {
		return st, err
	}
	st.Current, st.Size = cur, size
	prep, _, err := currentHash(s.abs(it.Prepared))
	if err != nil {
		return st, err
	}
	st.PreparedPresent = prep != ""
	st.PreparedMatches = prep == it.After
	if it.Kind != ItemAppend {
		switch cur {
		case it.After:
			st.Verdict = VerdictDone
		case it.Before:
			st.Verdict = VerdictPending
		default:
			st.Verdict = VerdictMismatch
		}
		return st, nil
	}
	st.Verdict, err = s.appendVerdict(it, st.AbsPath)
	return st, err
}

func (s *State) appendVerdict(it Item, path string) (string, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		if it.BaseSize == 0 {
			return VerdictPending, nil
		}
		return VerdictMismatch, nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	prefix := make([]byte, it.BaseSize)
	if _, err := io.ReadFull(f, prefix); err != nil {
		return VerdictMismatch, nil
	}
	if Sum(prefix) != it.BaseHash {
		return VerdictMismatch, nil
	}
	tail, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	switch n := int64(len(tail)); {
	case n == 0:
		return VerdictPending, nil
	case n == it.Length && Sum(tail) == it.After:
		return VerdictDone, nil
	case n < it.Length:
		chunk, err := os.ReadFile(s.abs(it.Prepared))
		if err == nil && Sum(chunk) == it.After && bytes.Equal(tail, chunk[:n]) {
			return VerdictPartial, nil
		}
	}
	return VerdictMismatch, nil
}

// ExportDiagnostics: 복구 상태를 JSON으로 내보낸다. 0600으로 새로 만들고, 있는 파일은 덮지 않는다.
// 내용에는 경로와 해시만 있고 파일 내용은 없다.
func (s *State) ExportDiagnostics(path string) error {
	rep, err := s.Status()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|noFollow, filePerm)
	if err != nil {
		return err
	}
	if err := f.Chmod(filePerm); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
