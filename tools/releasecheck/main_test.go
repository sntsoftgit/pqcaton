// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// gitRepo 는 files 를 담은 임시 git 리포를 만들고 커밋 하나를 남긴다.
func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git is required for these tests")
	}
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "x"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

const testMod = "module example.com/m\n\ngo 1.21\n"

// v0.26.2를 막은 경로: 커밋에 Windows 예약 이름이 있으면 zip 을 만들지 않고 그 경로를 댄다.
func TestZipRejectsWindowsReservedName(t *testing.T) {
	repo := gitRepo(t, map[string]string{"go.mod": testMod, "testdata/nul/a.json": "{}"})
	out := filepath.Join(t.TempDir(), "m.zip")
	_, err := makeZip(repo, "HEAD", "example.com/m", "v1.0.0", out)
	if err == nil || !strings.Contains(err.Error(), "testdata/nul/a.json") {
		t.Fatalf("want an error naming testdata/nul/a.json, got %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatal("a failed zip must not leave a file behind")
	}
}

// 대소문자만 다른 두 경로는 대소문자를 가리지 않는 파일 시스템에서 겹친다.
func TestZipRejectsCaseCollision(t *testing.T) {
	repo := gitRepo(t, map[string]string{"go.mod": testMod, "docs/A.md": "a", "docs/a.md": "b"})
	if _, err := makeZip(repo, "HEAD", "example.com/m", "v1.0.0", filepath.Join(t.TempDir(), "m.zip")); err == nil {
		t.Fatal("want a case-collision error, got none")
	}
}

// 뜬 h1 이 go 명령이 그 zip 을 파일 프록시에서 받아 계산한 값과 같다. 망 없이 돈다.
func TestZipHashMatchesGoCommand(t *testing.T) {
	repo := gitRepo(t, map[string]string{"go.mod": testMod, "p/p.go": "package p\n"})
	proxy := t.TempDir()
	at := filepath.Join(proxy, "example.com", "m", "@v")
	if err := os.MkdirAll(at, 0o755); err != nil {
		t.Fatal(err)
	}
	h1, err := makeZip(repo, "HEAD", "example.com/m", "v1.0.0", filepath.Join(at, "v1.0.0.zip"))
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"list": "v1.0.0\n", "v1.0.0.mod": testMod, "v1.0.0.info": `{"Version":"v1.0.0","Time":"2026-01-01T00:00:00Z"}`,
	} {
		if err := os.WriteFile(filepath.Join(at, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "mod", "download", "-json", "example.com/m@v1.0.0")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "GOPROXY=file://"+filepath.ToSlash(proxy), "GONOSUMDB=example.com", "GOFLAGS=-modcacherw",
		"GOMODCACHE="+t.TempDir(), "GOTOOLCHAIN=local")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go mod download: %v\n%s", err, out)
	}
	var got struct{ Sum, GoModSum string }
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Sum != h1 {
		t.Fatalf("go command computed %s, releasecheck computed %s", got.Sum, h1)
	}
	if mh, _ := modHash([]byte(testMod)); got.GoModSum != mh {
		t.Fatalf("go.mod hash: go command %s, releasecheck %s", got.GoModSum, mh)
	}
}

func docsRepo(t *testing.T, readme string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":                    "module github.com/o/app\n\ngo 1.21\n\nrequire github.com/u/common v0.10.7\n",
		"README.md":                 readme,
		"README.ko.md":              readme,
		"docs/operator-guide.md":    "```\ngo install github.com/o/app/cmd/ui@v1.2.3\n```\n",
		"docs/operator-guide.ko.md": "```\ngo install github.com/u/common/cmd/keygen@v0.10.7\n```\nold: go install …/keygen@v0.10.4\n",
	}
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCommandsSplitOwnAndExternal(t *testing.T) {
	cs, err := findCommands(docsRepo(t, "go install github.com/o/app/cmd/ui@v1.2.3\n"), "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || !cs[0].Own || cs[0].Pkg != "github.com/o/app/cmd/ui" || cs[1].Own || cs[1].Pkg != "github.com/u/common/cmd/keygen" {
		t.Fatalf("want own ui then external keygen once each, got %+v", cs)
	}
}

func TestCommandsRejectWrongVersions(t *testing.T) {
	for name, tc := range map[string]struct{ readme, version string }{
		"own stale":         {"go install github.com/o/app/cmd/ui@v1.2.2\n", "v1.2.3"},
		"external mismatch": {"go install github.com/u/common/cmd/keygen@v0.10.6\n", "v1.2.3"},
		"unknown module":    {"go install github.com/x/y/cmd/z@v1.0.0\n", "v1.2.3"},
	} {
		if _, err := findCommands(docsRepo(t, tc.readme), tc.version); err == nil {
			t.Errorf("%s: want an error, got none", name)
		}
	}
}

const notesFixture = `# notes

## Roadmap (plan)

## v1.2.3: something (2026-11-11)

**Apache-2.0 change: 2030-11-11** (four years)

## v1.2.2: older (2026-11-10)

**Apache-2.0 change: 2030-11-10**
`

func TestNotes(t *testing.T) {
	if err := checkNotes(notesFixture, "v1.2.3", "2026-11-11", "DRAFT"); err != nil {
		t.Fatalf("want ok, got %v", err)
	}
	bad := map[string]func() error{
		"wrong version": func() error { return checkNotes(notesFixture, "v1.2.4", "2026-11-11", "") },
		"wrong date":    func() error { return checkNotes(notesFixture, "v1.2.3", "2026-11-12", "") },
		"apache date": func() error {
			return checkNotes(strings.Replace(notesFixture, "2030-11-11", "2030-11-10", 1), "v1.2.3", "2026-11-11", "")
		},
		"draft first": func() error {
			return checkNotes(strings.Replace(notesFixture, "## v1.2.3", "## next patch (DRAFT)\n\n## v1.2.3", 1), "v1.2.3", "2026-11-11", "DRAFT")
		},
		"bad date": func() error { return checkNotes(notesFixture, "v1.2.3", "2026/11/11", "") },
	}
	for name, f := range bad {
		if f() == nil {
			t.Errorf("%s: want an error, got none", name)
		}
	}
}

// 윤일에 내면 4년 뒤도 윤일이다(2100년 같은 예외는 이 리포가 만날 일이 없다).
func TestNotesLeapDay(t *testing.T) {
	n := "## v1.0.0: x (2028-02-29)\n\n**Apache-2.0 change: 2032-02-29**\n"
	if err := checkNotes(n, "v1.0.0", "2028-02-29", ""); err != nil {
		t.Fatal(err)
	}
}

func TestInfo(t *testing.T) {
	ok := `{"Version":"v1.2.3","Origin":{"VCS":"git","Hash":"abc"}}`
	if err := checkInfo([]byte(ok), "v1.2.3", "abc"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"Version":"v1.2.2","Origin":{"Hash":"abc"}}`, `{"Version":"v1.2.3","Origin":{"Hash":"def"}}`, `{"Version":"v1.2.3"}`} {
		if checkInfo([]byte(bad), "v1.2.3", "abc") == nil {
			t.Errorf("want an error for %s", bad)
		}
	}
}

// 로그에 쓰기 전에 거치는 가림(redact.sh)을 고정 픽스처로 잰다. *_KEY= · *_TOKEN= · *_PASSWORD= · DSN 의 사용자 정보 ·
// PEM 개인 키 블록. 첫 시험에서 pqcota-keygen 이 찍은 PQCOTA_SIGN_KEY= 가 그대로 로그에 남았던 꼴도 들어 있다.
func TestRedactFixtures(t *testing.T) {
	in, err := os.Open(filepath.Join("testdata", "redact-in.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	want, err := os.ReadFile(filepath.Join("testdata", "redact-want.txt"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", "source ./redact.sh && redact")
	cmd.Stdin = in
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("redact: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("redact output differs\n--- got\n%s--- want\n%s", got, want)
	}
}

const fixtureEN = `English · [Korean](RELEASE_NOTES.ko.md)

# Release notes

## v1.2.3: Scope rounds go through the history (2026-11-11)

**Apache-2.0 conversion: 2030-11-11**

- See [the table](docs/compatibility.md), [notes](#v122-older-2026-11-10), [up](./README.md#how-it-fits-together) and [pqcota](https://github.com/u/common/issues/1).
- In code, ` + "`[x](docs/a.md)`" + ` stays.

` + "```" + `
[y](docs/b.md)
` + "```" + `

## v1.2.2: older (2026-11-10)

**Apache-2.0 conversion: 2030-11-10**
`

const fixtureKO = `[English](RELEASE_NOTES.md) · Korean

# notes

## v1.2.3: Korean title, with ` + "`code`" + ` (2026-11-11)

**Apache-2.0 change: 2030-11-11**

- [table](docs/compatibility.ko.md)

## v1.2.2: older (2026-11-10)

**Apache-2.0 change: 2030-11-10**
`

func TestBodyRewritesLinksToTheTag(t *testing.T) {
	b, err := releaseBody(fixtureEN, fixtureKO, "github.com/o/app", "v1.2.3", "KO")
	if err != nil {
		t.Fatal(err)
	}
	blob := "https://github.com/o/app/blob/v1.2.3/"
	for _, want := range []string{
		"[KO](" + blob + "RELEASE_NOTES.ko.md#v123-korean-title-with-code-2026-11-11)\n\n**Apache-2.0 conversion: 2030-11-11**",
		"[the table](" + blob + "docs/compatibility.md)",
		"[notes](" + blob + "RELEASE_NOTES.md#v122-older-2026-11-10)",
		"[up](" + blob + "README.md#how-it-fits-together)",
		"[pqcota](https://github.com/u/common/issues/1)",
		"`[x](docs/a.md)`",
		"\n[y](docs/b.md)\n",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("body lacks %q:\n%s", want, b)
		}
	}
	if strings.Contains(b, "## v1.2.2") || strings.Contains(b, "## v1.2.3") {
		t.Errorf("body must hold only the section text:\n%s", b)
	}
	// 한국어 절의 상대 링크도 같은 규칙으로 바뀐다(한국어 노트만 본문으로 쓰는 날을 위해).
	ko, err := rewriteLinks("[t](docs/compatibility.ko.md) [h](#v123-title)", "https://github.com/o/app/blob/v1.2.3")
	if err != nil || ko != "[t]("+blob+"docs/compatibility.ko.md) [h]("+blob+"RELEASE_NOTES.md#v123-title)" {
		t.Errorf("Korean links: %q %v", ko, err)
	}
}

func TestBodyRefuses(t *testing.T) {
	for name, tc := range map[string]struct{ en, ko, version, label string }{
		"no Korean section": {fixtureEN, strings.Replace(fixtureKO, "## v1.2.3", "## v1.2.0", 1), "v1.2.3", "KO"},
		"other date":        {fixtureEN, strings.Replace(fixtureKO, "(2026-11-11)", "(2026-11-12)", 1), "v1.2.3", "KO"},
		"no English":        {fixtureEN, fixtureKO, "v1.2.4", "KO"},
		"empty label":       {fixtureEN, fixtureKO, "v1.2.3", ""},
		"escaping link":     {strings.Replace(fixtureEN, "docs/compatibility.md", "../x.md", 1), fixtureKO, "v1.2.3", "KO"},
	} {
		if _, err := releaseBody(tc.en, tc.ko, "github.com/o/app", tc.version, tc.label); err == nil {
			t.Errorf("%s: want an error, got none", name)
		}
	}
}

func TestBodyCheck(t *testing.T) {
	want, err := releaseBody(fixtureEN, fixtureKO, "github.com/o/app", "v1.2.3", "KO")
	if err != nil {
		t.Fatal(err)
	}
	// 웹에서 고친 본문은 CRLF 가 된다. 그것은 같은 본문이다.
	u, err := checkBody(want, strings.ReplaceAll(want, "\n", "\r\n")+"  \r\n", fixtureKO)
	if err != nil || u != "https://github.com/o/app/blob/v1.2.3/RELEASE_NOTES.ko.md" {
		t.Fatalf("want ok, got %q %v", u, err)
	}
	if _, err := checkBody(want, strings.Replace(want, "older", "newer", 1)+"\nextra", fixtureKO); err == nil {
		t.Error("a different body: want an error")
	}
	if _, err := checkBody(want, want, strings.Replace(fixtureKO, "Korean title", "Other title", 1)); err == nil {
		t.Error("missing Korean anchor: want an error")
	}
	if _, err := checkBody("x", "x", fixtureKO); err == nil {
		t.Error("no Korean link on top: want an error")
	}
}

func TestNotesBothFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("RELEASE_NOTES.md", fixtureEN)
	write("RELEASE_NOTES.ko.md", fixtureKO)
	args := []string{"-repo", dir, "-version", "v1.2.3", "-date", "2026-11-11", "-draft-marker", "DRAFT", "-draft-marker", "in progress"}
	if err := cmdNotes(args); err != nil {
		t.Fatalf("want ok, got %v", err)
	}
	write("RELEASE_NOTES.ko.md", strings.Replace(fixtureKO, "2030-11-11", "2030-11-12", 1))
	if err := cmdNotes(args); err == nil || !strings.Contains(err.Error(), "RELEASE_NOTES.ko.md") {
		t.Fatalf("a wrong Korean date: want an error naming the Korean notes, got %v", err)
	}
	write("RELEASE_NOTES.ko.md", fixtureKO)
	write("RELEASE_NOTES.md", strings.Replace(fixtureEN, "# Release notes", "# Release notes\n\n## next (in progress)", 1))
	if err := cmdNotes(args); err == nil || !strings.Contains(err.Error(), "RELEASE_NOTES.md") {
		t.Fatalf("an English draft first: want an error, got %v", err)
	}
}
