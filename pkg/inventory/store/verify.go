// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// 확정 상태의 읽기 전용 확인. 화면의 단계 카드가 쓴다.
//
// **영수증이 있다는 것만으로 「확정됨」이라 하지 않는다.** 영수증의 출력 해시와 원장 구간을 지금
// 파일과 견준다. 확정과 겹치지 않게 대상 잠금 아래에서 보고, 잠근 뒤 영수증을 다시 읽는다. 아무것도
// 쓰지 않는다: 복구하지 않고, 잠금 파일을 만들지 않고, 영수증을 고치지 않는다.

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// 확정 상태.
const (
	// FinalNone: 영수증이 없다(미확정).
	FinalNone = "none"
	// FinalDone: 영수증의 출력과 원장 구간이 지금 파일과 맞는다.
	FinalDone = "finalized"
	// FinalSuperseded: 출력이 바뀌었는데, 순번이 더 큰 다른 세션의 영수증이 그 경로를 지금 내용으로 썼다.
	FinalSuperseded = "superseded"
	// FinalRecovery: 영수증과 지금 파일이 어긋나고, 그것을 설명하는 뒤 확정이 없다.
	FinalRecovery = "recovery"
)

// ErrInProgress: 그 출력 디렉토리에 끝나지 않은 트랜잭션 기록이 있다. 지금은 판정할 수 없다.
var ErrInProgress = errors.New("a save to these files has not finished")

// VerifyFinalized: 이 세션의 확정 상태를 읽기만 해서 판정한다. targets는 그 세션의 확정이 쓰는 파일들
// (확정을 부를 때 주는 것과 같은 목록)이다. **영수증을 처음 읽기 전에** 그 디렉토리들을 잠근다. 그래야
// 영수증을 읽고 잠그는 사이에 확정이 끼어들어 판정의 근거가 달라지는 일이 없다. 영수증이 그 밖의
// 디렉토리를 가리키면 그것까지 합쳐 다시 잠그고 처음부터 다시 본다.
//
// 일시적으로 판정할 수 없으면(잠금 대기 초과, 끝나지 않은 기록, 읽기 실패) 오류를 돌려준다. 부르는 쪽은
// 그것을 「복구 필요」로 단정하지 말고 「확인할 수 없음」으로 보여야 한다.
func (s *State) VerifyFinalized(id Identity, targets []string, wait time.Duration) (string, error) {
	if err := id.check(); err != nil {
		return "", err
	}
	locked := map[string]bool{}
	for _, d := range dirsOf(targets) {
		abs, err := filepath.Abs(d)
		if err != nil {
			return "", err
		}
		locked[abs] = true
	}
	for attempt := 0; attempt < 2; attempt++ {
		dirs := make([]string, 0, len(locked))
		for d := range locked {
			dirs = append(dirs, d)
		}
		release, err := s.lockInOrder(dirs, wait, s.openExistingLock)
		if err != nil {
			return "", err
		}
		v, extra, err := s.verifyLocked(id, locked)
		release()
		if err != nil || len(extra) == 0 {
			return v, err
		}
		for _, d := range extra {
			locked[d] = true
		}
	}
	return "", fmt.Errorf("the receipt for %s keeps naming directories outside the ones it was checked with", id)
}

// verifyLocked: 잠금 아래에서 판정한다. 영수증이 잠그지 않은 디렉토리를 가리키면 판정하지 않고 그
// 디렉토리들을 돌려준다.
func (s *State) verifyLocked(id Identity, locked map[string]bool) (string, []string, error) {
	rc, err := s.readReceipt(id)
	if err != nil || rc == nil {
		return FinalNone, nil, err
	}
	var dirs, extra []string
	for _, o := range rc.Outputs {
		dirs = append(dirs, filepath.Dir(s.abs(o.Path)))
	}
	for _, l := range rc.Ledger {
		dirs = append(dirs, filepath.Dir(s.abs(l.Path)))
	}
	for _, d := range dirs {
		if !locked[d] {
			extra = append(extra, d)
		}
	}
	if len(extra) > 0 {
		return "", extra, nil
	}
	// 끝나지 않은 기록이 이 디렉토리를 건드리면 출력이 반쯤 바뀐 상태일 수 있다. 어긋남으로 읽지 않는다.
	recs, err := s.Records()
	if err != nil {
		return "", nil, err
	}
	for _, r := range recs {
		for _, d := range s.recordDirs(r) {
			if locked[d] {
				return "", nil, fmt.Errorf("%w (%s)", ErrInProgress, r.TxID)
			}
		}
	}
	out, err := s.verifyReceipt(*rc)
	var rerr *RecoveryError
	switch {
	case errors.As(err, &rerr):
		return FinalRecovery, nil, nil
	case err != nil:
		return "", nil, err
	case out.Status == OutcomeSuperseded:
		return FinalSuperseded, nil, nil
	}
	return FinalDone, nil, nil
}

// StateInputs: 확정 상태를 정하는 상태 디렉토리 안의 파일 — 이 세션의 영수증과 트랜잭션 기록
// 디렉토리. 화면이 카드의 지문에 넣는다. 그 사이에 확정이 영수증을 쓰거나 기록을 남기면 지문이 바뀐다.
func (s *State) StateInputs(ids ...Identity) []Input {
	in := []Input{{Name: "state:txn", Path: filepath.Join(s.Dir, txnDir), Dir: true}}
	for _, id := range ids {
		in = append(in, Input{Name: "state:receipt:" + id.String(), Path: s.receiptPath(id)})
	}
	return in
}
