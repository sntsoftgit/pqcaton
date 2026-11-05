// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// rulesetRound: 옛 세션 자리에 새 세션을 쓰고 옛 바이트를 archive에 옮기는 전환.
func rulesetRound(w world) (RulesetRound, []string) {
	archive := filepath.Join(w.data, "session.ruleset-abc-s-1.json")
	report := filepath.Join(w.data, "ruleset-change-s-1.json")
	return RulesetRound{FromPath: w.session, ArchivePath: archive,
		Session: Change{Path: w.session, Content: []byte("session-v3\n")}, Report: Change{Path: report, Content: []byte("report\n")},
		FromRuleset: "enrich/v2+plan/v3", ToRuleset: "enrich/v3+plan/v3"}, []string{w.session, archive, report}
}

// IC-W51: 규칙 전환 회차는 한 트랜잭션이다. 어느 자리에서 멈춰도 다시 열어 복구하면 보관본 · 새 세션 · 보고서 · 회차
// 표지가 넷 다 있거나 넷 다 없다. 넷 다 없으면 옛 세션 파일은 바이트 그대로다.
func TestRulesetRoundIsOneTransaction(t *testing.T) {
	for _, c := range []struct {
		stop      stopAt
		committed bool
	}{
		{stopAt{point: FaultRecordPreparing}, false},
		{stopAt{point: FaultPrepared}, false},
		{stopAt{point: FaultRecordPrepared}, false},
		{stopAt{point: FaultApply, path: "0"}, true},
		{stopAt{point: FaultApply, path: "2"}, true},
		{stopAt{point: FaultApply, path: "3"}, true},
		{stopAt{point: FaultDone}, true},
	} {
		t.Run(c.stop.point+c.stop.path, func(t *testing.T) {
			var stop *stopAt
			w, s, from := finalized(t, func(point, path string) error {
				if stop == nil {
					return nil
				}
				return stop.hook(point, path)
			})
			before := read(t, w.session)
			stop = &c.stop
			r, targets := rulesetRound(w)
			to := Identity{Kind: "review", Org: "local", SessionID: "s-2"}
			if err := s.OpenRulesetRound(from, to, targets, Check{}, time.Second, func() (RulesetRound, error) { return r, nil }); !errors.Is(err, errStop) {
				t.Fatalf("OpenRulesetRound: %v, want the injected stop", err)
			}
			s.Close()
			again := w.open(t, nil)
			if err := again.Recover(time.Second); err != nil {
				t.Fatal(err)
			}
			mark, err := again.readRound(from)
			if err != nil {
				t.Fatal(err)
			}
			got := []bool{mark != nil, read(t, w.session) == "session-v3\n", read(t, r.ArchivePath) == before, read(t, r.Report.Path) == "report\n"}
			for i, g := range got {
				if g != c.committed {
					t.Errorf("part %d present = %v, want %v (marker, session, archive, report)", i, g, c.committed)
				}
			}
			if !c.committed && read(t, w.session) != before {
				t.Error("the old session file changed although nothing was committed")
			}
		})
	}
}

// IC-W52: 규칙 전환 회차의 나머지 불변식. 확정된 세션이면 영수증은 그대로이고 회차 표지가 옛 영수증 해시 · 옛 세션
// 해시 · 새 세션 ID를 잇는다. 옛 세션은 「복구 필요」가 아니라 「대체됨」이다. 보관본은 옛 바이트 그대로다. 같은 전환을
// 다시 보내면 ErrStale이고 아무것도 더 만들지 않는다. 보관 자리가 이미 있으면 ErrExists이고 아무것도 쓰지 않는다.
// 확정하지 않은 세션에서도 열리고, 새 세션을 다른 자리에 쓰면 옛 파일은 제자리에 남는다.
func TestRulesetRoundLinksTheOldReceiptAndHappensOnce(t *testing.T) {
	w, s, from := finalized(t, nil)
	oldBytes := read(t, w.session)
	receipt, err := os.ReadFile(s.receiptPath(from))
	if err != nil {
		t.Fatal(err)
	}
	r, targets := rulesetRound(w)
	to := Identity{Kind: "review", Org: "local", SessionID: "s-2"}
	open := func(to Identity) error {
		return s.OpenRulesetRound(from, to, targets, Check{}, time.Second, func() (RulesetRound, error) { return r, nil })
	}
	if err := open(to); err != nil {
		t.Fatal(err)
	}
	mark, err := s.RoundFrom(from)
	if err != nil || mark == nil {
		t.Fatalf("no round marker: %v", err)
	}
	hash := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	if mark.Reason != RoundReasonRuleset || mark.To != "s-2" || mark.FromSHA256 != hash([]byte(oldBytes)) || mark.ReceiptSHA256 != hash(receipt) ||
		mark.FromRuleset != r.FromRuleset || mark.ToRuleset != r.ToRuleset || mark.Archive == "" || mark.Report == "" || len(mark.Paths) != 1 {
		t.Errorf("the marker does not link the old receipt, the old session and the new one: %+v", mark)
	}
	if now, _ := os.ReadFile(s.receiptPath(from)); string(now) != string(receipt) {
		t.Error("the old receipt was rewritten")
	}
	if read(t, r.ArchivePath) != oldBytes {
		t.Error("the archive is not the old bytes")
	}
	if got, err := s.VerifyFinalized(from, []string{w.session, w.plan, w.ledger}, time.Second); err != nil || got != FinalSuperseded {
		t.Errorf("the old finalized session: %q, %v; want superseded", got, err)
	}

	entries := func() int { e, _ := os.ReadDir(w.data); return len(e) }
	n := entries()
	if err := open(Identity{Kind: "review", Org: "local", SessionID: "s-3"}); !errors.Is(err, ErrStale) {
		t.Errorf("the same change again: %v, want ErrStale", err)
	}
	if entries() != n || read(t, w.session) != "session-v3\n" {
		t.Error("a second request wrote something")
	}

	// 확정하지 않은 세션. 보관 자리에 이미 파일이 있으면 아무것도 쓰지 않는다.
	w2 := newWorld(t)
	s2 := w2.open(t, nil)
	draft := Identity{Kind: "review", Org: "local", SessionID: "d-1"}
	r2, t2 := rulesetRound(w2)
	if err := os.WriteFile(r2.ArchivePath, []byte("taken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = s2.OpenRulesetRound(draft, Identity{Kind: "review", Org: "local", SessionID: "d-2"}, t2, Check{}, time.Second,
		func() (RulesetRound, error) { return r2, nil })
	if !errors.Is(err, ErrExists) || read(t, w2.session) != sessV1 || read(t, r2.Report.Path) != "<absent>" || read(t, r2.ArchivePath) != "taken\n" {
		t.Fatalf("a taken archive path: %v; session %q", err, read(t, w2.session))
	}
	_ = os.Remove(r2.ArchivePath)
	if mark, _ := s2.RoundFrom(draft); mark != nil {
		t.Fatal("a refused round left a marker")
	}

	// 새 세션을 다른 자리에 쓴다(명령의 -o). 옛 파일은 제자리에 남고 보관본이 없어도 된다.
	newPath := filepath.Join(w2.data, "session.v3.json")
	r2.ArchivePath, r2.Session.Path = "", newPath
	t2 = []string{newPath, r2.Report.Path}
	if err := s2.OpenRulesetRound(draft, Identity{Kind: "review", Org: "local", SessionID: "d-2"}, t2, Check{}, time.Second,
		func() (RulesetRound, error) { return r2, nil }); err != nil {
		t.Fatal(err)
	}
	if read(t, w2.session) != sessV1 || read(t, newPath) != "session-v3\n" {
		t.Errorf("old %q, new %q", read(t, w2.session), read(t, newPath))
	}
	if mark, _ := s2.RoundFrom(draft); mark == nil || mark.ReceiptSHA256 != "" || mark.Archive != "" {
		t.Errorf("an unfinalized round's marker: %+v", mark)
	}
}
