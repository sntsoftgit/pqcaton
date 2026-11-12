// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// 릴리스 본문은 영어 노트의 그 판 절이다. 맨 위에 한국어 노트의 같은 판으로 가는 링크를 둔다.
// 링크는 모두 태그에 고정한다. GitHub 도 본문의 상대 링크를 태그 기준으로 풀지만, 그 규칙에 기대지 않고
// 본문만 떼어 읽어도(메일 · 피드) 같은 곳으로 가게 한다.
const (
	notesEN = "RELEASE_NOTES.md"
	notesKO = "RELEASE_NOTES.ko.md"
)

var (
	bodyLink   = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)((?:\s+"[^"]*")?)\)`)
	bodyInline = regexp.MustCompile("`[^`]*`")
	bodyFence  = regexp.MustCompile("^\\s*```")
	bodyScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	bodyMdLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// section 은 notes 에서 version 절의 제목 줄과 본문(다음 ## 앞까지)을 돌려준다.
func section(notes, version string) (heading, body string, err error) {
	for _, loc := range versionHeading.FindAllStringSubmatchIndex(notes, -1) {
		if notes[loc[2]:loc[3]] != version {
			continue
		}
		heading = strings.TrimSpace(notes[loc[0]:loc[1]])
		rest := notes[loc[1]:]
		if next := anyHeading.FindStringIndex(rest); next != nil {
			rest = rest[:next[0]]
		}
		return heading, strings.TrimSpace(rest), nil
	}
	return "", "", fmt.Errorf("no '## %s: … (YYYY-MM-DD)' section", version)
}

// slug — GitHub 의 제목 앵커(checkdocs 와 같은 규칙).
func slug(text string) string {
	t := bodyMdLink.ReplaceAllString(text, "$1")
	t = strings.NewReplacer("`", "", "**", "", "__", "").Replace(t)
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(t)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// fixLink 는 노트 안의 링크 대상 하나를 태그 고정 주소로 바꾼다. 바깥 주소 · 메일은 그대로 둔다.
// 노트는 리포 맨 위에 있으므로 상대 경로는 리포 맨 위 기준이다.
func fixLink(target, blob string) (string, error) {
	if bodyScheme.MatchString(target) {
		return target, nil
	}
	p, frag, _ := strings.Cut(target, "#")
	if p == "" {
		p = notesEN
	}
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("link %q is absolute within the site; write it relative to the repository root", target)
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("link %q leaves the repository", target)
	}
	out := blob + "/" + clean
	if frag != "" {
		out += "#" + frag
	}
	return out, nil
}

// rewriteLinks 는 코드 블록 · 인라인 코드 밖의 링크만 바꾼다(코드 안의 대괄호는 링크가 아니다).
func rewriteLinks(text, blob string) (string, error) {
	var out []string
	fenced := false
	var firstErr error
	fix := func(seg string) string {
		return bodyLink.ReplaceAllStringFunc(seg, func(m string) string {
			sub := bodyLink.FindStringSubmatch(m)
			t, err := fixLink(sub[2], blob)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return m
			}
			return "[" + sub[1] + "](" + t + sub[3] + ")"
		})
	}
	for _, line := range strings.Split(text, "\n") {
		if bodyFence.MatchString(line) {
			fenced = !fenced
			out = append(out, line)
			continue
		}
		if fenced {
			out = append(out, line)
			continue
		}
		var b strings.Builder
		last := 0
		for _, loc := range bodyInline.FindAllStringIndex(line, -1) {
			b.WriteString(fix(line[last:loc[0]]))
			b.WriteString(line[loc[0]:loc[1]])
			last = loc[1]
		}
		b.WriteString(fix(line[last:]))
		out = append(out, b.String())
	}
	return strings.Join(out, "\n"), firstErr
}

// releaseBody 는 영어 노트의 version 절로 릴리스 본문을 만든다. 한국어 노트에 같은 판 · 같은 날짜의 절이 없으면 거절한다.
// koLabel 은 맨 위 링크의 글자다. Go 문자열에 한국어를 두지 않으려고 부르는 쪽이 넘긴다.
func releaseBody(en, ko, module, version, koLabel string) (string, error) {
	if koLabel == "" {
		return "", errors.New("the Korean link label is empty")
	}
	if !strings.HasPrefix(module, "github.com/") {
		return "", fmt.Errorf("module %s is not on github.com", module)
	}
	hEN, body, err := section(en, version)
	if err != nil {
		return "", fmt.Errorf("%s: %w", notesEN, err)
	}
	hKO, _, err := section(ko, version)
	if err != nil {
		return "", fmt.Errorf("%s: %w", notesKO, err)
	}
	if de, dk := versionHeading.FindStringSubmatch(hEN)[2], versionHeading.FindStringSubmatch(hKO)[2]; de != dk {
		return "", fmt.Errorf("%s is dated %s in %s but %s in %s", version, de, notesEN, dk, notesKO)
	}
	blob := "https://" + module + "/blob/" + version
	fixed, err := rewriteLinks(body, blob)
	if err != nil {
		return "", err
	}
	top := fmt.Sprintf("[%s](%s/%s#%s)", koLabel, blob, notesKO, slug(strings.TrimPrefix(hKO, "## ")))
	return top + "\n\n" + fixed + "\n", nil
}

func readNotes(repo string) (en, ko string, err error) {
	b, err := os.ReadFile(filepath.Join(repo, notesEN))
	if err != nil {
		return "", "", err
	}
	k, err := os.ReadFile(filepath.Join(repo, notesKO))
	if err != nil {
		return "", "", err
	}
	return string(b), string(k), nil
}

// body: 릴리스 본문을 표준 출력으로 낸다. 발행은 이 출력을 그대로 본문으로 쓴다.
func cmdBody(args []string) error {
	fs := flag.NewFlagSet("body", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository root (notes and go.mod)")
	version := fs.String("version", "", "version being released")
	koLabel := fs.String("ko-label", "", "text of the link to the Korean notes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	en, ko, err := readNotes(*repo)
	if err != nil {
		return err
	}
	mod, err := modulePath(*repo)
	if err != nil {
		return err
	}
	b, err := releaseBody(en, ko, mod, *version, *koLabel)
	if err != nil {
		return err
	}
	fmt.Print(b)
	return nil
}

// normBody — 웹에서 고치면 줄 끝이 CRLF 가 되고 끝 공백이 달라진다. 그 차이는 내용 차이로 보지 않는다.
func normBody(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

var koTop = regexp.MustCompile(`^\[[^\]]+\]\((https://[^)#\s]+)#([^)\s]+)\)$`)

// checkBody 는 실제 릴리스 본문(got)이 만든 본문(want)과 같은지, 맨 위 한국어 링크의 앵커가 그 태그의
// 한국어 노트(koNotes)에 있는지 본다. 열어 볼 한국어 노트 주소(앵커 없이)를 돌려준다.
func checkBody(want, got, koNotes string) (string, error) {
	w, g := normBody(want), normBody(got)
	if w != g {
		wl, gl := strings.Split(w, "\n"), strings.Split(g, "\n")
		for i := 0; i < len(wl) || i < len(gl); i++ {
			var a, b string
			if i < len(wl) {
				a = wl[i]
			}
			if i < len(gl) {
				b = gl[i]
			}
			if a != b {
				return "", fmt.Errorf("the release body differs from the English notes at line %d:\n  want %q\n  got  %q", i+1, a, b)
			}
		}
	}
	first, _, _ := strings.Cut(g, "\n")
	m := koTop.FindStringSubmatch(first)
	if m == nil {
		return "", fmt.Errorf("the first line of the release body is not a link to the Korean notes: %q", first)
	}
	frag, err := url.PathUnescape(m[2])
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(koNotes, "\n") {
		if strings.HasPrefix(line, "## ") && slug(strings.TrimPrefix(line, "## ")) == frag {
			return m[1], nil
		}
	}
	return "", fmt.Errorf("the Korean notes at that tag have no heading for #%s", frag)
}

// bodycheck: release-verify 가 부른다. -release 는 GitHub API 의 릴리스 JSON 이다.
func cmdBodyCheck(args []string) error {
	fs := flag.NewFlagSet("bodycheck", flag.ContinueOnError)
	want := fs.String("want", "", "body built by 'releasecheck body' from the release commit")
	rel := fs.String("release", "", "GitHub API release JSON")
	ko := fs.String("ko", "", "Korean notes at the release tag")
	if err := fs.Parse(args); err != nil {
		return err
	}
	w, err := os.ReadFile(*want)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*rel)
	if err != nil {
		return err
	}
	var r struct {
		Body       string `json:"body"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("release JSON: %w", err)
	}
	if r.Draft {
		return errors.New("the release is still a draft")
	}
	k, err := os.ReadFile(*ko)
	if err != nil {
		return err
	}
	u, err := checkBody(string(w), r.Body, string(k))
	if err != nil {
		return err
	}
	fmt.Println(u)
	return nil
}
