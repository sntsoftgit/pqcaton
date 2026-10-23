// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// 새 회차. 확정된 세션은 저장도 다시 확정도 막히므로, 바꾸려면 **새 ID의 세션**을 연다.
//
// 새 회차는 확정한 세션의 출력 하나(세션 파일)를 새 세션의 내용으로 바꾼다. 그러면 옛 영수증의 그 출력이
// 지금 파일과 달라지는데, 바꾼 것은 확정이 아니라서 「뒤 확정이 썼다(대체됨)」로도 잡히지 않는다. 그대로
// 두면 옛 세션의 확정 요청을 다시 보내거나 옛 세션을 판정할 때 「복구 필요」로 오판한다.
//
// 그래서 새 회차는 **회차 표지**를 남긴다. 「이 확정 세션의 이 출력들은 저 새 세션이 넘겨받았다」는
// 기록이고, 세션 파일의 교체와 **한 트랜잭션**으로 쓴다(둘 중 하나만 남지 않는다). 영수증 판정은 표지가
// 넘겨받은 출력이 바뀐 것을 대체됨으로 본다. 넘겨받지 않은 출력(계획 · 정책 · 원장)은 지금처럼 잰다.
// 확정된 세션 하나에서 새 회차는 한 번만 열린다(표지가 이미 있으면 화면이 오래된 것이다).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	roundDir    = "rounds"
	roundFormat = 1
)

// ErrNotFinalized: 확정하지 않은 세션에서 새 회차를 열려고 했다. 확정하지 않은 세션은 그대로 고친다.
var ErrNotFinalized = errors.New("this session is not finalized yet")

// Round: 회차 표지.
type Round struct {
	Format int    `json:"format"`
	Kind   string `json:"kind"`
	Org    string `json:"org"`
	// From: 넘겨준(확정된) 세션. To: 넘겨받은 새 세션.
	From   string    `json:"from_session_id"`
	To     string    `json:"to_session_id"`
	Paths  []string  `json:"paths"`
	Opened time.Time `json:"opened"`
}

func (s *State) roundPath(from Identity) string {
	return filepath.Join(s.Dir, roundDir, from.Kind, from.Org, from.SessionID+".json")
}

// readRound: 그 세션에서 연 회차의 표지. 없으면 nil.
func (s *State) readRound(from Identity) (*Round, error) {
	p := s.roundPath(from)
	if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	b, err := s.readPrivate(p)
	if err != nil {
		return nil, err
	}
	var r Round
	if err := json.Unmarshal(b, &r); err != nil || r.Format != roundFormat || r.Kind != from.Kind || r.Org != from.Org || r.From != from.SessionID {
		return nil, fmt.Errorf("%w: round marker %s; keep a copy and run pqcaton-ui -recover-status", ErrDamaged, p)
	}
	return &r, nil
}

// handedOver: 그 출력을 새 회차가 넘겨받았나.
func (s *State) handedOver(rc Receipt, path string) (bool, error) {
	r, err := s.readRound(rc.Identity())
	if err != nil || r == nil {
		return false, err
	}
	return slices.Contains(r.Paths, path), nil
}

// OpenRound: 확정된 세션(from)에서 새 회차(to)를 연다. build가 잠금 안에서 새 세션의 내용을 만들고, 그
// 파일들과 회차 표지를 한 트랜잭션으로 쓴다. 넘겨받는 출력은 build가 낸 파일 가운데 from의 영수증에 있는
// 것이다. from이 확정되지 않았으면 ErrNotFinalized, to가 이미 확정됐거나 from에서 이미 새 회차를 열었으면
// ErrStale(화면이 오래됐다), rev가 다르면 ErrStale이다.
func (s *State) OpenRound(from, to Identity, targets []string, chk Check, wait time.Duration, build func() ([]Change, error)) error {
	for _, id := range []Identity{from, to} {
		if err := id.check(); err != nil {
			return err
		}
	}
	if from.Kind != to.Kind || from.Org != to.Org || from.SessionID == to.SessionID {
		return fmt.Errorf("store: a new round must be the same kind and organization with a new session id (%s → %s)", from, to)
	}
	return s.withLocks(dirsOf(targets), wait, func() error {
		rc, err := s.readReceipt(from)
		if err != nil {
			return err
		}
		if rc == nil {
			return fmt.Errorf("%w (%s); change it and finalize it instead", ErrNotFinalized, from)
		}
		if next, err := s.readReceipt(to); err != nil || next != nil {
			if err != nil {
				return err
			}
			return fmt.Errorf("%w: the new session %s is already finalized", ErrStale, to)
		}
		if r, err := s.readRound(from); err != nil || r != nil {
			if err != nil {
				return err
			}
			return fmt.Errorf("%w: a new round was already opened from %s", ErrStale, from)
		}
		if err := chk.verify(); err != nil {
			return err
		}
		replace, err := build()
		if err != nil {
			return err
		}
		tx := Tx{Kind: to.Kind, Org: to.Org, SessionID: to.SessionID, Replace: replace}
		if err := within(tx, targets); err != nil {
			return err
		}
		out := map[string]bool{}
		for _, o := range rc.Outputs {
			out[o.Path] = true
		}
		mark := Round{Format: roundFormat, Kind: from.Kind, Org: from.Org, From: from.SessionID, To: to.SessionID,
			Opened: time.Now().UTC()}
		for _, c := range replace {
			abs, err := filepath.Abs(c.Path)
			if err != nil {
				return err
			}
			if rel := s.rel(abs); out[rel] {
				mark.Paths = append(mark.Paths, rel)
			}
		}
		b, err := json.MarshalIndent(mark, "", "  ")
		if err != nil {
			return err
		}
		p := s.roundPath(from)
		if err := mkdirPrivate(filepath.Dir(p)); err != nil {
			return err
		}
		tx.Create = []Change{{Path: p, Content: append(b, '\n')}}
		rec, err := s.plan(tx)
		if err != nil {
			return err
		}
		if err := chk.verify(); err != nil {
			return err
		}
		return s.run1(rec, tx)
	})
}
