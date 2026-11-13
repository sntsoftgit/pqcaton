// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package collectorver

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const want = "v0.10.7"

// sumPinned — 시험의 고정 h1. 옆 폴더 · VCS 빌드는 빌드 정보에 주 모듈 h1 이 없어 이 값과 견줄 일이 없다.
const sumPinned = "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

// 픽스처는 리포에 바이너리를 넣지 않고 시험 중에 만든다. 상류와 같은 모양으로 둔다: 수집기 모듈이 의존 모듈을 옆 폴더
// replace 로 가리키고, 태그를 단 git 체크아웃에서 빌드한다. 실물로 확인한 빌드 방식마다 바이너리 하나씩이다.

type fixture struct {
	t    *testing.T
	root string
}

func newFixture(t *testing.T, discoveryVersion, commonVersion string) *fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git is required to build the collector fixtures")
	}
	f := &fixture{t: t, root: t.TempDir()}
	f.write("pqcota-common/go.mod", "module github.com/randyinthedev-hash/pqcota-common\n\ngo 1.21\n")
	f.write("pqcota-common/c/c.go", "package c\n\nfunc Name() string { return \"common\" }\n")
	f.write("pqcota-discovery/go.mod", "module "+DiscoveryModule+"\n\ngo 1.21\n\nrequire github.com/randyinthedev-hash/pqcota-common "+
		commonVersion+"\n\nreplace github.com/randyinthedev-hash/pqcota-common => ../pqcota-common\n")
	f.write("pqcota-discovery/cmd/pqcota-jvmscan/main.go",
		"package main\n\nimport \"github.com/randyinthedev-hash/pqcota-common/c\"\n\nfunc main() { println(c.Name()) }\n")
	f.git("pqcota-discovery", "init", "-q")
	f.git("pqcota-discovery", "add", "-A")
	f.git("pqcota-discovery", "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "x")
	f.git("pqcota-discovery", "tag", discoveryVersion)
	return f
}

func (f *fixture) write(rel, body string) {
	f.t.Helper()
	p := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) git(dir string, args ...string) {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = filepath.Join(f.root, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// build 는 dir 에서 pkg 를 빌드한다. env 는 덧붙일 환경 변수다.
func (f *fixture) build(dir, pkg string, env []string, flags ...string) string {
	f.t.Helper()
	out := filepath.Join(f.t.TempDir(), "pqcota-jvmscan")
	args := append([]string{"build", "-o", out}, flags...)
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), append(args, pkg)...)
	cmd.Dir = filepath.Join(f.root, dir)
	cmd.Env = append(os.Environ(), append([]string{"GOTOOLCHAIN=local", "GOPROXY=off", "GOWORK=off", "GOFLAGS=-mod=mod", "CGO_ENABLED=0"}, env...)...)
	if b, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("go build: %v\n%s", err, b)
	}
	return out
}

func expect(t *testing.T, got File, state State, reason Reason) {
	t.Helper()
	if got.State != state || got.Reason != reason {
		t.Fatalf("got %s/%q (declared %q, seen %v), want %s/%q", got.State, got.Reason, got.Declared, got.Seen, state, reason)
	}
}

// IC-CV1 — 깨끗한 태그 체크아웃이어도 의존이 옆 폴더 replace 면 확인됨이 아니다. 수집기 자신의 판 · 커밋은 읽히지만 replace 한
// 의존의 실제 소스는 알 수 없어 확인 불가(replace 의존)다. 지금의 상류 빌드가 모두 이렇다.
func TestTagCheckoutWithReplacedDepsIsUnverified(t *testing.T) {
	f := newFixture(t, want, want)
	got := CheckGo(f.build("pqcota-discovery", "./cmd/pqcota-jvmscan", nil), "linux-amd64/pqcota-jvmscan", want, sumPinned)
	expect(t, got, Unverified, ReasonDepsReplaced)
	if got.Declared != want || got.Revision == "" || !got.DepsReplaced {
		t.Fatalf("want declared %s, a revision and DepsReplaced, got %+v", want, got)
	}
}

// IC-CV9 — 옆 pqcota-common 을 v0.10.6 으로 바꿔 빌드해도 빌드 정보에는 드러나지 않는다. 그래서 그런 빌드는 결코 확인됨이
// 되지 않아야 한다(실물에서 확인한 한계를 그대로 시험으로 고정한다).
func TestSwappedSiblingIsNeverVerified(t *testing.T) {
	f := newFixture(t, want, want)
	f.write("pqcota-common/c/c.go", "package c\n\n// the v0.10.6 source\nfunc Name() string { return \"common v0.10.6\" }\n")
	f.git("pqcota-common", "init", "-q")
	f.git("pqcota-common", "add", "-A")
	f.git("pqcota-common", "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "x")
	f.git("pqcota-common", "tag", "v0.10.6")
	got := CheckGo(f.build("pqcota-discovery", "./cmd/pqcota-jvmscan", nil), "x", want, sumPinned)
	if got.State == Verified {
		t.Fatalf("a build against a swapped sibling must not be verified: %+v", got)
	}
	expect(t, got, Unverified, ReasonDepsReplaced)
}

// IC-CV10 — replace 없이 의존을 그 판으로 받아 태그 체크아웃에서 빌드해도, 빌드 정보에 주 모듈의 h1 이 없으므로(VCS 빌드)
// 고정한 h1 과 견줄 수 없어 확인 불가(h1 없음)다. 의존에 replace 는 없고 커밋은 읽힌다. 의존은 시험 안의 파일 프록시가 준다(망 없이).
func TestReleaseBuildWithoutReplaceIsVerified(t *testing.T) {
	root := t.TempDir()
	proxy := filepath.Join(root, "proxy")
	commonModule := "github.com/randyinthedev-hash/pqcota-common"
	at := filepath.Join(proxy, filepath.FromSlash(commonModule), "@v")
	if err := os.MkdirAll(at, 0o755); err != nil {
		t.Fatal(err)
	}
	gomod := "module " + commonModule + "\n\ngo 1.21\n"
	zf, err := os.Create(filepath.Join(at, want+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	for name, body := range map[string]string{"go.mod": gomod, "c/c.go": "package c\n\nfunc Name() string { return \"common\" }\n"} {
		w, _ := zw.Create(commonModule + "@" + want + "/" + name)
		w.Write([]byte(body))
	}
	zw.Close()
	zf.Close()
	for name, body := range map[string]string{"list": want + "\n", want + ".mod": gomod, want + ".info": `{"Version":"` + want + `","Time":"2026-11-10T00:00:00Z"}`} {
		os.WriteFile(filepath.Join(at, name), []byte(body), 0o644)
	}
	f := &fixture{t: t, root: root}
	f.write("pqcota-discovery/go.mod", "module "+DiscoveryModule+"\n\ngo 1.21\n\nrequire "+commonModule+" "+want+"\n")
	f.write("pqcota-discovery/cmd/pqcota-jvmscan/main.go",
		"package main\n\nimport \""+commonModule+"/c\"\n\nfunc main() { println(c.Name()) }\n")
	env := []string{"GOPROXY=file://" + filepath.ToSlash(proxy), "GONOSUMDB=github.com/randyinthedev-hash", "GOSUMDB=off",
		"GOMODCACHE=" + filepath.Join(root, "gmc"), "GOFLAGS=-mod=mod -modcacherw"}
	// go.sum 을 먼저 만들어 커밋해야 빌드한 트리가 고친 트리(dirty)가 되지 않는다.
	tidy := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "mod", "tidy")
	tidy.Dir = filepath.Join(root, "pqcota-discovery")
	tidy.Env = append(os.Environ(), append([]string{"GOTOOLCHAIN=local", "GOWORK=off"}, env...)...)
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
	f.git("pqcota-discovery", "init", "-q")
	f.git("pqcota-discovery", "add", "-A")
	f.git("pqcota-discovery", "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "x")
	f.git("pqcota-discovery", "tag", want)
	got := CheckGo(f.build("pqcota-discovery", "./cmd/pqcota-jvmscan", env), "x", want, sumPinned)
	expect(t, got, Unverified, ReasonNoSum)
	if got.DepsReplaced || got.Revision == "" || got.Sum != "" {
		t.Fatalf("want no replaced dependency, a revision and no module h1, got %+v", got)
	}
}

// IC-CV2 — 옛 판으로 빌드한 수집기는 맞지 않음이다. v0.26.2 때 옛 pqcota-jvmscan 이 적재를 통째로 막았다.
func TestOldVersionIsMismatch(t *testing.T) {
	f := newFixture(t, "v0.10.6", "v0.10.6")
	expect(t, CheckGo(f.build("pqcota-discovery", "./cmd/pqcota-jvmscan", nil), "x", want, sumPinned), Mismatch, ReasonOldVersion)
}

// IC-CV3 — 판을 믿을 수 없는 빌드는 확인 불가다: -buildvcs=false(devel), 고친 트리(dirty), 태그 뒤 커밋(의사 판).
func TestUntrustedBuildsAreUnverified(t *testing.T) {
	f := newFixture(t, want, want)
	expect(t, CheckGo(f.build("pqcota-discovery", "./cmd/pqcota-jvmscan", nil, "-buildvcs=false"), "x", want, sumPinned), Unverified, ReasonDevel)

	f.write("pqcota-discovery/README.md", "changed\n")
	expect(t, CheckGo(f.build("pqcota-discovery", "./cmd/pqcota-jvmscan", nil), "x", want, sumPinned), Unverified, ReasonDirty)

	f.git("pqcota-discovery", "add", "-A")
	f.git("pqcota-discovery", "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "after the tag")
	expect(t, CheckGo(f.build("pqcota-discovery", "./cmd/pqcota-jvmscan", nil), "x", want, sumPinned), Unverified, ReasonPseudo)
}

// IC-CV4 — 상류 데모처럼 다른 모듈에서 수집기를 replace 로 끌어와 빌드하면, 선언된 판이 맞아도 확인 불가다.
func TestDemoStyleReplaceIsUnverified(t *testing.T) {
	f := newFixture(t, want, want)
	f.write("pqcota/go.mod", "module github.com/randyinthedev-hash/pqcota\n\ngo 1.21\n\nrequire (\n\t"+DiscoveryModule+" "+want+
		"\n\tgithub.com/randyinthedev-hash/pqcota-common "+want+"\n)\n\nreplace (\n\t"+DiscoveryModule+
		" => ../pqcota-discovery\n\tgithub.com/randyinthedev-hash/pqcota-common => ../pqcota-common\n)\n")
	f.write("pqcota/tools.go", "//go:build tools\n\npackage pqcota\n\nimport _ \""+DiscoveryModule+"/cmd/pqcota-jvmscan\"\n")
	got := CheckGo(f.build("pqcota", DiscoveryModule+"/cmd/pqcota-jvmscan", nil), "x", want, sumPinned)
	expect(t, got, Unverified, ReasonReplaced)
	if got.Declared != want {
		t.Fatalf("want the declared version %s shown, got %q", want, got.Declared)
	}
}

// IC-CV5 — Windows 수집기(PE)도 같은 빌드 정보로 읽는다(의존이 replace 라 리눅스와 같이 확인 불가다).
func TestWindowsBinaryIsRead(t *testing.T) {
	f := newFixture(t, want, want)
	bin := f.build("pqcota-discovery", "./cmd/pqcota-jvmscan", []string{"GOOS=windows", "GOARCH=amd64"})
	expect(t, CheckGo(bin, "windows-amd64/pqcota-jvmscan.exe", want, sumPinned), Unverified, ReasonDepsReplaced)
}

// IC-CV6 — Go 빌드 정보가 없는 파일, 다른 모듈로 빌드한 프로그램, 고정 판을 모르는 화면은 확인 불가다.
func TestNoBuildInfoOtherModuleNoPin(t *testing.T) {
	junk := filepath.Join(t.TempDir(), "pqcota-netcap")
	if err := os.WriteFile(junk, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	expect(t, CheckGo(junk, "x", want, sumPinned), Unverified, ReasonNoBuildInfo)

	f := newFixture(t, want, want)
	f.write("other/go.mod", "module example.com/other\n\ngo 1.21\n")
	f.write("other/main.go", "package main\n\nfunc main() {}\n")
	expect(t, CheckGo(f.build("other", ".", nil, "-buildvcs=false"), "x", want, sumPinned), Unverified, ReasonOtherModule)

	expect(t, CheckGo(junk, "x", "", ""), Unverified, ReasonNoPin)
}

func jar(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "collector.jar")
	out, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(out)
	for name, body := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out.Close()
	return p
}

// IC-CV7 — collector.jar 는 실행하지 않고 manifest 나 pom.properties 로 판을 읽는다. v0.14.0 전 상류의 JAR 처럼 판이 없으면
// 확인 불가(판 없음)이고, 판이 있는데 다르면 맞지 않음이다. 판만 맞고 커밋 · 수정 여부가 없으면 확인됨이 아니라 확인 불가(커밋
// 정보 없음)다.
func TestJar(t *testing.T) {
	upstreamToday := "Manifest-Version: 1.0\nAgent-Class: pqcota.jvm.IntrospectAgent\nCan-Redefine-Classes: false\n"
	expect(t, CheckJar(jar(t, map[string]string{"META-INF/MANIFEST.MF": upstreamToday}), "collector.jar", want), Unverified, ReasonNoVersion)
	expect(t, CheckJar(jar(t, map[string]string{"META-INF/MANIFEST.MF": upstreamToday + "Implementation-Version: 0.10.7\n"}), "x", want), Unverified, ReasonNoCommit)
	expect(t, CheckJar(jar(t, map[string]string{"META-INF/MANIFEST.MF": "Implementation-Version: v0.10.6\n"}), "x", want), Mismatch, ReasonOldVersion)
	expect(t, CheckJar(jar(t, map[string]string{"META-INF/maven/pqcota/collector/pom.properties": "groupId=pqcota\nversion=0.10.7\n"}), "x", want), Unverified, ReasonNoCommit)
	junk := filepath.Join(t.TempDir(), "collector.jar")
	os.WriteFile(junk, []byte("not a zip"), 0o644)
	expect(t, CheckJar(junk, "x", want), Unverified, ReasonUnreadable)
}

// IC-CV12 — 상류 v0.14.0 의 collector.jar 는 manifest 에 커밋과 수정 여부를 싣는다. 커밋은 Revision 으로 읽히고, 고친
// 트리에서 만든 것(X-Pqcota-Dirty: true)은 Go 의 dirty 빌드처럼 확인 불가(dirty)다. 판이 다르면 dirty 보다 먼저 맞지 않음이다.
func TestJarCommitAndDirty(t *testing.T) {
	const commit = "c4d8f366fdeda2de7c6bab0b2822c91ede7d448d"
	release := "Manifest-Version: 1.0\nImplementation-Version: v0.10.7\nX-Pqcota-Commit: " + commit + "\nX-Pqcota-Dirty: false\n"
	got := CheckJar(jar(t, map[string]string{"META-INF/MANIFEST.MF": release}), "collector.jar", want)
	expect(t, got, Verified, ReasonOK)
	if got.Revision != commit {
		t.Fatalf("want the commit %s read, got %q", commit, got.Revision)
	}
	unknown := strings.Replace(release, "X-Pqcota-Dirty: false", "X-Pqcota-Dirty: unknown", 1)
	expect(t, CheckJar(jar(t, map[string]string{"META-INF/MANIFEST.MF": unknown}), "x", want), Unverified, ReasonNoCommit)
	dirty := strings.Replace(release, "X-Pqcota-Dirty: false", "X-Pqcota-Dirty: true", 1)
	expect(t, CheckJar(jar(t, map[string]string{"META-INF/MANIFEST.MF": dirty}), "x", want), Unverified, ReasonDirty)
	old := strings.Replace(dirty, "v0.10.7", "v0.10.6", 1)
	expect(t, CheckJar(jar(t, map[string]string{"META-INF/MANIFEST.MF": old}), "x", want), Mismatch, ReasonOldVersion)

	// 상류가 직접 빌드한 JAR 의 판은 정식 판이 아니다: git 이 없으면 devel, git 에서는 describe 꼴. 다른 판이 아니라 확인 불가다.
	mf := func(v string) string {
		return "Manifest-Version: 1.0\nImplementation-Version: " + v + "\nX-Pqcota-Commit: unknown\nX-Pqcota-Dirty: unknown\n"
	}
	for v, reason := range map[string]Reason{"devel": ReasonDevel, "v0.10.7-dirty": ReasonDirty, "v0.10.7-3-gabc1234": ReasonPseudo,
		"v0.10.7-3-gabc1234-dirty": ReasonPseudo, "<script>alert(1)</script>": ReasonDevel} {
		got := CheckJar(jar(t, map[string]string{"META-INF/MANIFEST.MF": mf(v)}), "x", want)
		expect(t, got, Unverified, reason)
		if got.Revision != "" || len(got.Seen) != 0 {
			t.Errorf("%s: a non-commit (unknown) must not be read as a commit, and a non-release version must not count as seen: %+v", v, got)
		}
	}
	// manifest 의 값은 임의 문자열이다. 화면에 옮기는 판은 제어 문자를 빼고 64자에서 자른다. 커밋은 40 · 64자 16진수만 받는다.
	long := CheckJar(jar(t, map[string]string{"META-INF/MANIFEST.MF": mf("v" + strings.Repeat("9", 500))}), "x", want)
	if r := []rune(long.Declared); len(r) != maxShown+1 || !strings.HasSuffix(long.Declared, "…") {
		t.Errorf("a 500-character version should be cut to %d characters and an ellipsis, got %d", maxShown, len(r))
	}
	for _, c := range []string{"C4D8F366FDEDA2DE7C6BAB0B2822C91EDE7D448D", commit[:39], commit + "0", "c4d8f366fded zzz"} {
		got := CheckJar(jar(t, map[string]string{"META-INF/MANIFEST.MF": strings.Replace(release, commit, c, 1)}), "x", want)
		if got.Revision != "" || got.State != Unverified || got.Reason != ReasonNoCommit {
			t.Errorf("%q is not a commit: want it unread and the jar cannot be verified (no commit), got %+v", c, got)
		}
	}
}

// IC-CV8 — 수집기끼리 서로 다른 정식 판을 보이면 알아챈다.
func TestMixed(t *testing.T) {
	a := File{Seen: []string{"v0.10.7"}}
	b := File{Seen: []string{"v0.10.6", "v0.10.7"}}
	if got := Mixed([]File{a, b}); strings.Join(got, ",") != "v0.10.7,v0.10.6" {
		t.Fatalf("want both versions, got %v", got)
	}
	if Mixed([]File{a, a}) != nil {
		t.Fatal("one version is not mixed")
	}
}
