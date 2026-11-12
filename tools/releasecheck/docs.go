// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
)

// publicDocs — 설치 명령을 공개하는 문서. 사용자가 그대로 붙여 넣는 자리라, 여기 적힌 명령은 발행 전에 실제로 설치해 본다.
// **경로와 언어를 여기 고정한다**: 영어 기본 문서(`x.md`)와 한국어 짝(`x.ko.md`)을 모두 읽는다. 문서를 옮기면 이 목록과
// release-verify.sh의 목록을 함께 고친다(checkdocs가 짝을 재고, 두 판의 명령이 다르면 막는다).
var publicDocs = []string{"README.md", "README.ko.md", "docs/operator-guide.md", "docs/operator-guide.ko.md"}

var installLine = regexp.MustCompile("go install (github\\.com/[^\\s`@]+)@(v[^\\s`)]+)")

func readGoMod(repo string) (*modfile.File, error) {
	p := filepath.Join(repo, "go.mod")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return modfile.ParseLax(p, data, nil)
}

func modulePath(repo string) (string, error) {
	mf, err := readGoMod(repo)
	if err != nil {
		return "", err
	}
	if mf.Module == nil {
		return "", errors.New("go.mod has no module line")
	}
	return mf.Module.Mod.Path, nil
}

// command 는 문서에 공개한 설치 명령 하나다.
type command struct {
	Own     bool   // 이 모듈의 명령인가, 외부 상류 명령인가
	Pkg     string // 설치할 패키지 경로
	Version string // 문서에 적힌 판
	Want    string // 그래야 하는 판
	Where   string // 처음 나온 자리(file:line)
}

// findCommands 는 공개 문서의 `go install` 명령을 뽑고 판을 견준다. 이 모듈의 명령은 version 이어야 하고,
// 외부 명령은 go.mod 가 그 모듈에 고정한 판이어야 한다. 판이 틀린 줄은 모두 모아 한꺼번에 알린다.
func findCommands(repo, version string) ([]command, error) {
	mf, err := readGoMod(repo)
	if err != nil {
		return nil, err
	}
	own := mf.Module.Mod.Path
	seen := map[string]bool{}
	var out []command
	var problems []string
	for _, doc := range publicDocs {
		data, err := os.ReadFile(filepath.Join(repo, doc))
		if err != nil {
			return nil, err
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range installLine.FindAllStringSubmatch(line, -1) {
				c := command{Pkg: m[1], Version: m[2], Where: fmt.Sprintf("%s:%d", doc, i+1)}
				if c.Pkg == own || strings.HasPrefix(c.Pkg, own+"/") {
					c.Own, c.Want = true, version
				} else {
					c.Want = pinned(mf, c.Pkg)
					if c.Want == "" {
						problems = append(problems, fmt.Sprintf("%s: %s is not from this module nor from a module go.mod requires", c.Where, c.Pkg))
						continue
					}
				}
				if c.Version != c.Want {
					problems = append(problems, fmt.Sprintf("%s: %s@%s, want @%s", c.Where, c.Pkg, c.Version, c.Want))
					continue
				}
				if key := c.Pkg + "@" + c.Version; !seen[key] {
					seen[key] = true
					out = append(out, c)
				}
			}
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("published install commands do not match:\n  %s", strings.Join(problems, "\n  "))
	}
	if !hasOwn(out) {
		return nil, fmt.Errorf("found no install command for %s in %s", own, strings.Join(publicDocs, ", "))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Own && !out[j].Own })
	return out, nil
}

func hasOwn(cs []command) bool {
	for _, c := range cs {
		if c.Own {
			return true
		}
	}
	return false
}

// pinned 는 pkg 를 담은 모듈 가운데 가장 긴 경로의 require 판을 돌려준다. 없으면 "".
func pinned(mf *modfile.File, pkg string) string {
	best, ver := "", ""
	for _, r := range mf.Require {
		p := r.Mod.Path
		if (pkg == p || strings.HasPrefix(pkg, p+"/")) && len(p) > len(best) {
			best, ver = p, r.Mod.Version
		}
	}
	return ver
}

func cmdCommands(args []string) error {
	fs := flag.NewFlagSet("commands", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository root")
	version := fs.String("version", "", "version being released")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" {
		return errors.New("commands needs -version")
	}
	cs, err := findCommands(*repo, *version)
	if err != nil {
		return err
	}
	for _, c := range cs {
		kind := "external"
		if c.Own {
			kind = "own"
		}
		fmt.Printf("%s\t%s@%s\t%s\n", kind, c.Pkg, c.Version, c.Where)
	}
	return nil
}

var (
	versionHeading = regexp.MustCompile(`(?m)^## (v\d+\.\d+\.\d+): .*\((\d{4}-\d{2}-\d{2})\)\s*$`)
	anyHeading     = regexp.MustCompile(`(?m)^## .*$`)
	apacheLine     = regexp.MustCompile(`\*\*Apache-2\.0 [^:*]*: (\d{4}-\d{2}-\d{2})\*\*`)
)

// checkNotes 는 노트 맨 위 판 절이 version 이고 제목 날짜가 date, Apache-2.0 전환일이 정확히 4년 뒤인지 본다.
// draft 는 작성 중인 절의 제목 표지다. 그 표지를 단 제목이 맨 위 판 절보다 앞에 있으면 아직 발행할 때가 아니다.
func checkNotes(notes, version, date, draft string) error {
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return fmt.Errorf("release date %q is not YYYY-MM-DD", date)
	}
	loc := versionHeading.FindStringSubmatchIndex(notes)
	if loc == nil {
		return errors.New("release notes have no '## vX.Y.Z: … (YYYY-MM-DD)' heading")
	}
	if draft != "" {
		for _, h := range anyHeading.FindAllStringIndex(notes[:loc[0]], -1) {
			if strings.Contains(notes[h[0]:h[1]], draft) {
				return fmt.Errorf("a draft section comes before the top release: %q", notes[h[0]:h[1]])
			}
		}
	}
	gotVer, gotDate := notes[loc[2]:loc[3]], notes[loc[4]:loc[5]]
	if gotVer != version {
		return fmt.Errorf("the top release in the notes is %s, want %s", gotVer, version)
	}
	if gotDate != date {
		return fmt.Errorf("the %s heading is dated %s, want the release date %s", version, gotDate, date)
	}
	section := notes[loc[1]:]
	if next := anyHeading.FindStringIndex(section); next != nil {
		section = section[:next[0]]
	}
	m := apacheLine.FindStringSubmatch(section)
	if m == nil {
		return fmt.Errorf("the %s section has no Apache-2.0 change date line", version)
	}
	if want := day.AddDate(4, 0, 0).Format("2006-01-02"); m[1] != want {
		return fmt.Errorf("the Apache-2.0 change date is %s, want %s (four years after %s)", m[1], want, date)
	}
	return nil
}

// markers — 되풀이해 줄 수 있는 플래그. 노트마다 작성 중 표지가 말이 달라 둘을 받는다.
type markers []string

func (m *markers) String() string     { return strings.Join(*m, ",") }
func (m *markers) Set(v string) error { *m = append(*m, v); return nil }

// notes: 영어 노트와 한국어 노트를 모두 본다. 맨 위 판 · 날짜 · 전환일이 둘 다 맞아야 한다.
func cmdNotes(args []string) error {
	fs := flag.NewFlagSet("notes", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository root")
	version := fs.String("version", "", "version being released")
	date := fs.String("date", "", "release date YYYY-MM-DD")
	var drafts markers
	fs.Var(&drafts, "draft-marker", "heading marker of an unfinished section (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	en, ko, err := readNotes(*repo)
	if err != nil {
		return err
	}
	for _, n := range []struct{ name, text string }{{notesEN, en}, {notesKO, ko}} {
		if err := checkNotesAll(n.text, *version, *date, drafts); err != nil {
			return fmt.Errorf("%s: %w", n.name, err)
		}
	}
	fmt.Printf("✓ notes: %s dated %s in %s and %s, Apache-2.0 change date four years later\n", *version, *date, notesEN, notesKO)
	return nil
}

func checkNotesAll(text, version, date string, drafts []string) error {
	if len(drafts) == 0 {
		return checkNotes(text, version, date, "")
	}
	for _, d := range drafts {
		if err := checkNotes(text, version, date, d); err != nil {
			return err
		}
	}
	return nil
}
