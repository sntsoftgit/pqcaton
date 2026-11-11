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
		"docs/operator-guide.md":    "```\ngo install github.com/o/app/cmd/ui@v1.2.3\n```\n",
		"docs/operator-guide.en.md": "```\ngo install github.com/u/common/cmd/keygen@v0.10.7\n```\nold: go install …/keygen@v0.10.4\n",
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
