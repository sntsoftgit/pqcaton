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

// ErrSameRuleset — 세션이 이미 지금 상류 규칙 판이다. 넘길 것이 없다.
var ErrSameRuleset = errors.New("the session is already under the current upstream ruleset")

// ErrOrgMismatch — 옛 세션 · 다시 세운 세션 · 요청한 조직이 같지 않다. 항목 ID가 같아도 다른 조직의 결론을 옮기지 않는다.
var ErrOrgMismatch = errors.New("the old session, the rebuilt session and the requested organization are not the same organization")

// Inputs — 회차가 읽는 입력. 판정 화면이 rev로 덮는 넷과 같다(세션 · 선언 · 결과 폴더 · 자산 스코프 정책). 빈 경로는 뺀다.
func Inputs(oldPath, declPath, resultsDir, scopeAssets string) []store.Input {
	in := []store.Input{{Name: "session", Path: oldPath}}
	if declPath != "" {
		in = append(in, store.Input{Name: "declaration", Path: declPath})
	}
	if resultsDir != "" {
		in = append(in, store.Input{Name: "results", Path: resultsDir, Dir: true})
	}
	if scopeAssets != "" {
		in = append(in, store.Input{Name: "scope-assets", Path: scopeAssets})
	}
	return in
}

// Snapshot — 입력의 지문을 지금 재어 Check로 돌려준다. 명령이 쓴다(화면은 폼에 실려 온 rev를 쓴다). 저장소가 다시 세우기
// 전과 쓰기 직전에 같은 입력을 다시 재므로, 그 사이에 하나라도 바뀌면 ErrStale이고 아무것도 쓰지 않는다.
func Snapshot(inputs []store.Input) (store.Check, error) {
	rev, err := store.Rev(inputs)
	if err != nil {
		return store.Check{}, err
	}
	return store.Check{Inputs: inputs, Rev: rev}, nil
}

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
		key = strings.TrimPrefix(internalID(oldPath, "", oldBytes), "unnamed-")[:12]
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
	// 옛 세션은 쓰지 않을 때도(다른 자리에 새 세션을 쓸 때) 잠그고 남은 준비 파일을 본다.
	t := []string{req.OldPath, req.OutPath, report}
	if same(req.OldPath, req.OutPath) {
		t = append(t[1:], archive)
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
	if err := sameOrg(old.Scope, req.Org, "", false); err != nil {
		return Result{}, err
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
	from := store.Identity{Kind: Kind, Org: req.Org, SessionID: internalID(req.OldPath, old.SessionID, raw)}
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
		if err := sameOrg(cur.Scope, req.Org, rebuilt.Scope, true); err != nil {
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

// sameOrg — 옛 세션의 스코프와 요청한 조직이 같은가(rebuiltToo면 다시 세운 세션의 스코프도).
func sameOrg(oldScope, org, rebuiltScope string, rebuiltToo bool) error {
	want := "org://" + org
	if oldScope != want || (rebuiltToo && rebuiltScope != want) {
		return fmt.Errorf("%w: old session %q, rebuilt %q, requested %q", ErrOrgMismatch, oldScope, rebuiltScope, want)
	}
	return nil
}

// internalID — 저장소가 그 세션을 부르는 이름. 세션 ID가 있으면 그것이고, 없으면(옛 빌드의 파일) **절대 경로와 옛 바이트의
// 해시**다. 하나의 이름(unnamed-session)을 함께 쓰면 한 상태 폴더의 이름 없는 세션 둘 가운데 첫 회차의 표지가 둘째를 막는다.
func internalID(path, id string, raw []byte) string {
	if id != "" {
		return id
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	h := sha256.Sum256(append([]byte(abs+"\x00"), raw...))
	return "unnamed-" + hex.EncodeToString(h[:8])
}

func same(a, b string) bool {
	x, err1 := filepath.Abs(a)
	y, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && x == y
}
