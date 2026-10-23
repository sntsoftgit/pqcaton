// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// CommandWait: 명령이 남은 기록을 복구할 때 대상 잠금을 기다리는 시간.
const CommandWait = 5 * time.Second

// ErrNeedOutput: 원장에 쓰는 확정을 표준출력으로 내려 했다.
var ErrNeedOutput = errors.New("-judgments needs -o <file>")

// OpenCommand: 파일을 바꾸는 명령이 일하기 전에 부른다. 상태 디렉토리를 열고 실행 잠금을 **기다리지
// 않고** 잡는다(서버가 쥐고 있으면 곧바로 ErrBusy). 그다음 쓸 파일의 디렉토리에서 기록 없는 준비
// 파일을 찾고, 남은 기록을 복구한다. 복구 필요가 남아 있으면 일하지 않는다.
//
// 돌려받은 State는 명령이 끝날 때 Close한다.
func OpenCommand(dir string, targets []string, opt Options) (*State, error) {
	st, err := Open(dir, opt)
	if err != nil {
		return nil, err
	}
	if err := st.AcquireRun(); err != nil {
		return nil, err
	}
	var dirs []string
	for _, t := range targets {
		if t != "" {
			dirs = append(dirs, filepath.Dir(t))
		}
	}
	if err := st.CheckOrphans(dirs); err != nil {
		st.Close()
		return nil, err
	}
	if err := st.Recover(CommandWait); err != nil {
		st.Close()
		return nil, err
	}
	return st, nil
}

// Describe: 확정 결과를 사람이 읽는 한 줄로(명령의 표준오류).
func (o Outcome) Describe() string {
	switch o.Status {
	case OutcomeAlready:
		return "already finalized with the same content; nothing was written again"
	case OutcomeSuperseded:
		by := "another session"
		if o.By != nil {
			by = fmt.Sprintf("session %s", o.By.SessionID)
		}
		return fmt.Sprintf("the outputs of this session now hold a later finalize by %s; the earlier result was not written again", by)
	}
	return "finalized"
}
