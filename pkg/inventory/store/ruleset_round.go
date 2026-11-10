// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// 규칙 전환 회차. 상류 규칙 판이 바뀌어 옛 세션을 지금 규칙으로 다시 판정할 때 연다.
//
// 같은 규칙 판의 새 회차(OpenRound)와 다른 점은 셋이다. 확정하지 않은 세션에서도 연다. 옛 세션 파일을 덮지 않고
// **바이트 그대로 옮겨 보관한다.** 변환 보고서를 함께 쓴다. 넷(보관본 · 새 세션 · 보고서 · 회차 표지)이 한
// 트랜잭션이라 어느 자리에서 멈춰도 다시 열어 복구하면 넷이 다 있거나 넷 다 없다. 한 세션에서 회차는 한 번만
// 열린다: 표지가 이미 있으면 ErrStale이고 아무것도 만들지 않는다(같은 요청을 두 번 보내도 회차는 하나다).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// RoundReasonRuleset: 회차 표지의 Reason. 규칙 전환 회차다.
const RoundReasonRuleset = "ruleset-change"

// RoundReceipt — 회차와 한 트랜잭션으로 쓸 형식 2 영수증. 동일성은 (Kind, 조직, 옛 세션 ID)라 같은 옛 세션에서 같은 종류의
// 회차는 한 번만 열린다. 영수증은 조직 · 옛 세션 파일의 해시 · 정책 지문 · 새 세션 ID를 잇는다. 출력은 적지 않는다(판정
// 세션의 확정 영수증과 「대체됨」 판단이 섞이지 않게 한다).
type RoundReceipt struct {
	Kind        string
	ScopePolicy string
	Basis       string
}

// RulesetRound: 규칙 전환 회차에 쓸 것. build가 잠금 안에서 만든다.
type RulesetRound struct {
	// FromPath: 옛 세션 파일. 저장소가 잠금 안에서 이 바이트를 읽어 해시를 표지에 적고, ArchivePath에 옮긴다.
	FromPath string
	// ArchivePath: 옛 세션의 바이트를 옮겨 둘 자리. 새 세션을 FromPath에 쓰면 반드시 있어야 한다(덮지 않는다).
	// 새 세션을 다른 자리에 쓰면 비워도 된다: 옛 파일이 제자리에 남는다.
	ArchivePath string
	// Session: 새 세션 파일. Report: 변환 보고서. 보고서는 없던 파일이어야 한다.
	Session, Report Change
	FromRuleset     string
	ToRuleset       string
	// Reason: 회차 표지의 사유. 비면 RoundReasonRuleset이다. FromPolicy · ToPolicy: 정책 회차의 두 지문.
	Reason               string
	FromPolicy, ToPolicy string
	// Receipt: 있으면 회차와 한 트랜잭션으로 형식 2 영수증을 쓴다. 이미 있으면 ErrStale이고 아무것도 쓰지 않는다.
	Receipt *RoundReceipt
	// PolicyReplace · PolicyArchive: 활성화 회차가 함께 쓰는 것. 활성 정책 파일을 새 정책으로 바꾸고(Replace), 옛 정책의
	// 바이트를 보관본으로 만든다(Create, 없던 자리여야 한다). 둘 다 잠금 전에 대상으로 밝혀야 한다.
	PolicyReplace, PolicyArchive *Change
}

// OpenRulesetRound: 세션 from에서 규칙 전환 회차 to를 연다. targets는 쓸 파일들(보관본 · 새 세션 · 보고서)이다.
// from이 확정돼 있어도 되고 아니어도 된다. to가 이미 확정됐거나 from에서 이미 회차를 열었으면 ErrStale, rev가
// 다르면 ErrStale, 보관 자리나 보고서 자리에 파일이 이미 있으면 ErrExists이고 아무것도 쓰지 않는다.
func (s *State) OpenRulesetRound(from, to Identity, targets []string, chk Check, wait time.Duration, build func() (RulesetRound, error)) error {
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
		r, err := build()
		if err != nil {
			return err
		}
		fromAbs, err := filepath.Abs(r.FromPath)
		if err != nil {
			return err
		}
		sessAbs, err := filepath.Abs(r.Session.Path)
		if err != nil {
			return err
		}
		if r.ArchivePath == "" && fromAbs == sessAbs {
			return fmt.Errorf("store: the new session goes where the old one is, so the old one needs an archive path")
		}
		fi, err := os.Lstat(fromAbs)
		if err != nil {
			return err
		}
		if err := s.checkPlain(fromAbs, fi); err != nil {
			return err
		}
		old, err := os.ReadFile(fromAbs)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(old)
		reason := r.Reason
		if reason == "" {
			reason = RoundReasonRuleset
		}
		mark := Round{Format: roundFormat, Kind: from.Kind, Org: from.Org, From: from.SessionID, To: to.SessionID,
			Opened: time.Now().UTC(), Reason: reason, FromRuleset: r.FromRuleset, ToRuleset: r.ToRuleset,
			FromSHA256: hex.EncodeToString(sum[:]), FromPolicy: r.FromPolicy, ToPolicy: r.ToPolicy}
		tx := Tx{Kind: to.Kind, Org: to.Org, SessionID: to.SessionID, Replace: []Change{r.Session}}
		if fromAbs != sessAbs {
			// 새 세션을 다른 자리에 쓴다. 그 자리는 없던 파일이어야 한다(남의 파일을 덮지 않는다).
			if _, err := os.Lstat(sessAbs); err == nil {
				return fmt.Errorf("%w: %s", ErrExists, r.Session.Path)
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			tx.Replace = nil
			tx.Create = append(tx.Create, r.Session)
		}
		if r.ArchivePath != "" {
			tx.Create = append(tx.Create, Change{Path: r.ArchivePath, Content: old})
			if abs, err := filepath.Abs(r.ArchivePath); err == nil {
				mark.Archive = s.rel(abs)
			}
		}
		tx.Create = append(tx.Create, r.Report)
		if abs, err := filepath.Abs(r.Report.Path); err == nil {
			mark.Report = s.rel(abs)
		}
		if r.PolicyReplace != nil {
			tx.Replace = append(tx.Replace, *r.PolicyReplace)
		}
		if r.PolicyArchive != nil {
			tx.Create = append(tx.Create, *r.PolicyArchive)
		}
		if rc != nil {
			// 확정된 세션이었다. 영수증은 고치지 않고, 그 해시를 표지에 적어 옛 영수증 · 옛 세션 · 새 세션을 잇는다.
			rb, err := s.readPrivate(s.receiptPath(from))
			if err != nil {
				return err
			}
			rs := sha256.Sum256(rb)
			mark.ReceiptSHA256 = hex.EncodeToString(rs[:])
			out := map[string]bool{}
			for _, o := range rc.Outputs {
				out[o.Path] = true
			}
			if rel := s.rel(fromAbs); out[rel] && fromAbs == sessAbs {
				mark.Paths = append(mark.Paths, rel)
			}
		}
		// 쓰는 파일(보관본 · 새 세션 · 보고서)은 모두 잠금 전에 대상으로 밝힌 것이어야 한다. 표지는 상태 폴더 안이다.
		named := map[string]bool{}
		for _, t := range targets {
			if abs, err := filepath.Abs(t); err == nil {
				named[abs] = true
			}
		}
		for _, c := range append(append([]Change(nil), tx.Replace...), tx.Create...) {
			abs, err := filepath.Abs(c.Path)
			if err != nil {
				return err
			}
			if !named[abs] {
				return fmt.Errorf("store: %s was not named as a target before locking", abs)
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
		tx.Create = append(tx.Create, Change{Path: p, Content: append(b, '\n')})
		var rc2 *Receipt
		if r.Receipt != nil {
			// 회차의 형식 2 영수증. 형식 2를 쓰면 이 형식을 모르는 옛 판이 이 상태 폴더를 열지 못한다(의도다).
			if !policyDigestShape.MatchString(r.Receipt.ScopePolicy) {
				return fmt.Errorf("%w: %q", ErrPolicyDigest, r.Receipt.ScopePolicy)
			}
			rid := Identity{Kind: r.Receipt.Kind, Org: from.Org, SessionID: from.SessionID}
			if err := rid.check(); err != nil {
				return err
			}
			if prev, err := s.readReceipt(rid); err != nil || prev != nil {
				if err != nil {
					return err
				}
				return fmt.Errorf("%w: a %s round was already opened from %s", ErrStale, r.Receipt.Kind, from)
			}
			nr, err := s.newReceipt(Tx{Kind: rid.Kind, Org: rid.Org, SessionID: rid.SessionID, PayloadSHA256: hex.EncodeToString(sum[:])}, r.Receipt.ScopePolicy)
			if err != nil {
				return err
			}
			nr.Round = &ReceiptRound{FromSessionSHA256: hex.EncodeToString(sum[:]), ToSessionID: to.SessionID, FromPolicy: r.FromPolicy,
				Basis: r.Receipt.Basis}
			rc2 = &nr
			rp := s.receiptPath(rid)
			if err := mkdirPrivate(filepath.Dir(rp)); err != nil {
				return err
			}
			tx.Create = append(tx.Create, Change{Path: rp})
		}
		rec, err := s.plan(tx)
		if err != nil {
			return err
		}
		if rc2 != nil {
			// 영수증의 txid는 기록의 txid와 같아야 하므로 plan 뒤에 내용을 채운다(finalizeLocked와 같다).
			rc2.TxID = rec.TxID
			b, err := json.MarshalIndent(rc2, "", "  ")
			if err != nil {
				return err
			}
			last := len(tx.Create) - 1
			tx.Create[last].Content = append(b, '\n')
			rec.Items[len(rec.Items)-1].After = Sum(tx.Create[last].Content)
		}
		if err := chk.verify(); err != nil {
			return err
		}
		return s.run1(rec, tx)
	})
}

// RoundReceiptOf — 옛 세션 from에서 연 kind 종류 회차의 영수증. 없으면 nil이다.
func (s *State) RoundReceiptOf(kind string, from Identity) (*Receipt, error) {
	id := Identity{Kind: kind, Org: from.Org, SessionID: from.SessionID}
	if err := id.check(); err != nil {
		return nil, err
	}
	return s.readReceipt(id)
}

// RoundFrom: 세션 from에서 연 회차의 표지. 없으면 nil.
func (s *State) RoundFrom(from Identity) (*Round, error) {
	if err := from.check(); err != nil {
		return nil, err
	}
	return s.readRound(from)
}
