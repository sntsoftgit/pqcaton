// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// Package rulesetround는 규칙 전환 회차를 연다. 화면(pqcaton-ui)과 명령(pqcaton-decide new-round)이 이 함수 하나를
// 쓴다. 판정 쪽 변환(review.RulesetRound)과 저장소의 한 트랜잭션(store.OpenRulesetRound)을 잇는다.
//
// 둘이 다른 것은 「결과에서 지금 규칙으로 세션을 다시 세우는 방법」뿐이라 그것만 부르는 쪽이 넘긴다. 같은 입력이면
// 화면이든 명령이든 같은 세션 · 같은 보고서 바이트가 나온다.
package rulesetround

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

// Kind — 판정 세션의 영수증 종류. 화면과 명령이 같은 값을 쓴다.
const Kind = "review"

// unnamed — 세션 ID가 없는 옛 세션의 저장 이름. 화면의 것과 같다.
const unnamed = "unnamed-session"

// ErrSameRuleset — 세션이 이미 지금 상류 규칙 판이다. 넘길 것이 없다.
var ErrSameRuleset = errors.New("the session is already under the current upstream ruleset")

// Request — 회차 하나.
type Request struct {
	// OldPath: 옛 세션 파일. OutPath: 새 세션을 쓸 자리. 둘이 같으면 옛 파일을 보관본으로 옮긴다(화면이 그렇다).
	// 다르면 OutPath는 없던 파일이어야 하고 옛 파일은 제자리에 남는다.
	OldPath, OutPath string
	// ReportPath: 보고서 자리. 비우면 새 세션 옆에 정해진 이름([Names])이다. 없던 파일이어야 한다.
	ReportPath string
	Org        string
	NewID      string
	// Rebuild: 지금 규칙으로 결과에서 세운 세션. 저장소의 잠금 안에서 부른다.
	Rebuild func() (review.Session, error)
}

// Result — 연 회차.
type Result struct {
	NewSessionID    string
	Archive, Report string // 쓴 자리(보관본은 OutPath가 OldPath와 다르면 빈다)
}

var safeKey = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// Names — 보관본과 보고서의 자리. **파일 이름에 규칙 판 문자열을 넣지 않는다**(`pqcota-enrich/v2+…`의 `/`가 경로를
// 가른다). 규칙 판은 해시 앞 12자의 별칭으로, 세션은 그 ID(쓸 수 없는 글자가 있거나 없으면 내용 해시)로 가른다.
// 정확한 규칙 판은 보고서와 회차 표지에 적힌다.
func Names(oldPath, outPath string, old review.Session, oldBytes []byte) (archive, report string) {
	key := old.SessionID
	if !safeKey.MatchString(key) {
		h := sha256.Sum256(oldBytes)
		key = hex.EncodeToString(h[:6])
	}
	rh := sha256.Sum256([]byte(old.RulesetVersion))
	alias := hex.EncodeToString(rh[:6])
	base := strings.TrimSuffix(filepath.Base(oldPath), filepath.Ext(oldPath))
	archive = filepath.Join(filepath.Dir(oldPath), base+".ruleset-"+alias+"-"+key+".json")
	outBase := strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath))
	report = filepath.Join(filepath.Dir(outPath), outBase+".ruleset-change-"+key+".json")
	return archive, report
}

// Targets — 잠글 파일들(새 세션 · 보고서 · 보관본). 옛 세션을 읽어 이름을 정한다.
func Targets(req Request) ([]string, error) {
	old, raw, err := load(req.OldPath)
	if err != nil {
		return nil, err
	}
	archive, report := req.names(old, raw)
	t := []string{req.OutPath, report}
	if same(req.OldPath, req.OutPath) {
		t = append(t, archive)
	}
	return t, nil
}

// Open — 규칙 전환 회차를 연다. 옛 세션이 이미 지금 상류 규칙 판이면 ErrSameRuleset이다. 저장소의 오류(ErrStale:
// 이미 연 회차, ErrExists: 보관 자리가 있음 등)는 그대로 돌려준다.
func Open(st *store.State, req Request, chk store.Check, wait time.Duration) (Result, error) {
	old, raw, err := load(req.OldPath)
	if err != nil {
		return Result{}, err
	}
	if !changed(old) {
		return Result{}, ErrSameRuleset
	}
	archive, report := req.names(old, raw)
	targets, err := Targets(req)
	if err != nil {
		return Result{}, err
	}
	res := Result{NewSessionID: req.NewID, Report: report}
	archived := same(req.OldPath, req.OutPath)
	if archived {
		res.Archive = archive
	}
	from := store.Identity{Kind: Kind, Org: req.Org, SessionID: name(old.SessionID)}
	to := store.Identity{Kind: Kind, Org: req.Org, SessionID: req.NewID}
	err = st.OpenRulesetRound(from, to, targets, chk, wait, func() (store.RulesetRound, error) {
		// 잠금 안에서 다시 읽는다. 그 사이에 바뀌었으면 처음 정한 이름과 어긋나므로 멈춘다.
		cur, curRaw, err := load(req.OldPath)
		if err != nil {
			return store.RulesetRound{}, err
		}
		if a, r := req.names(cur, curRaw); a != archive || r != report || !changed(cur) {
			return store.RulesetRound{}, fmt.Errorf("%w: %s changed while the round was being opened", store.ErrStale, req.OldPath)
		}
		rebuilt, err := req.Rebuild()
		if err != nil {
			return store.RulesetRound{}, err
		}
		archiveName := ""
		if archived {
			archiveName = filepath.Base(archive)
		}
		next, rep := review.RulesetRound(cur, rebuilt, req.NewID, filepath.Base(report), archiveName)
		sb, err := review.Encode(next)
		if err != nil {
			return store.RulesetRound{}, err
		}
		rb, err := review.EncodeReport(rep)
		if err != nil {
			return store.RulesetRound{}, err
		}
		out := store.RulesetRound{FromPath: req.OldPath, Session: store.Change{Path: req.OutPath, Content: sb},
			Report: store.Change{Path: report, Content: rb}, FromRuleset: cur.RulesetVersion, ToRuleset: next.RulesetVersion}
		if archived {
			out.ArchivePath = archive
		}
		return out, nil
	})
	return res, err
}

func (req Request) names(old review.Session, raw []byte) (archive, report string) {
	archive, report = Names(req.OldPath, req.OutPath, old, raw)
	if req.ReportPath != "" {
		report = req.ReportPath
	}
	return archive, report
}

func load(p string) (review.Session, []byte, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return review.Session{}, nil, err
	}
	sf, err := review.Load(p)
	return sf, raw, err
}

// changed — 세션의 상류 규칙 판이 지금 도구와 다른가(빈 규칙 판도 다른 것이다).
func changed(sf review.Session) bool {
	return review.UpstreamRuleset(sf.RulesetVersion) != review.UpstreamRuleset(review.RulesetVersion)
}

func name(id string) string {
	if id == "" {
		return unnamed
	}
	return id
}

func same(a, b string) bool {
	x, err1 := filepath.Abs(a)
	y, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && x == y
}
