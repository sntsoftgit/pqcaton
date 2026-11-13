// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package collectorver

import (
	"archive/zip"
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

const commonModule = "github.com/randyinthedev-hash/pqcota-common"

// moduleProxy 는 mods(모듈@판 → 파일 이름 → 내용)를 파일 프록시로 만든다(망 없이).
func moduleProxy(t *testing.T, dir string, mods map[string]map[string]string) string {
	t.Helper()
	for mv, files := range mods {
		mod, ver, _ := strings.Cut(mv, "@")
		at := filepath.Join(dir, filepath.FromSlash(mod), "@v")
		if err := os.MkdirAll(at, 0o755); err != nil {
			t.Fatal(err)
		}
		zf, err := os.Create(filepath.Join(at, ver+".zip"))
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(zf)
		for name, body := range files {
			w, _ := zw.Create(mv + "/" + name)
			w.Write([]byte(body))
		}
		zw.Close()
		zf.Close()
		for name, body := range map[string]string{"list": ver + "\n", ver + ".mod": files["go.mod"],
			ver + ".info": `{"Version":"` + ver + `","Time":"2026-11-12T00:00:00Z"}`} {
			os.WriteFile(filepath.Join(at, name), []byte(body), 0o644)
		}
	}
	return dir
}

// releaseBuild 는 상류 v0.14.0 릴리스처럼 수집기 모듈을 **의존 모듈로 받아** 빌드한다(replace 없음, VCS 없음). 빌드 정보의
// 주 모듈이 pqcota-discovery 이고 판과 h1 이 실린다. message 를 바꾸면 같은 판 이름의 다른 내용이 된다. 돌려주는 값은
// 바이너리와, 빌드한 모듈의 go.sum 에 적힌 pqcota-discovery 의 h1 이다.
func releaseBuild(t *testing.T, message string) (bin, sum string) {
	t.Helper()
	root := t.TempDir()
	proxy := moduleProxy(t, filepath.Join(root, "proxy"), map[string]map[string]string{
		commonModule + "@" + want: {"go.mod": "module " + commonModule + "\n\ngo 1.21\n",
			"c/c.go": "package c\n\nfunc Name() string { return \"common\" }\n"},
		DiscoveryModule + "@" + want: {"go.mod": "module " + DiscoveryModule + "\n\ngo 1.21\n\nrequire " + commonModule + " " + want + "\n",
			"cmd/pqcota-jvmscan/main.go": "package main\n\nimport \"" + commonModule + "/c\"\n\nfunc main() { println(c.Name(), \"" + message + "\") }\n"},
	})
	wrap := filepath.Join(root, "wrap")
	if err := os.MkdirAll(wrap, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wrap, "go.mod"), []byte("module example.com/release\n\ngo 1.21\n\nrequire "+DiscoveryModule+" "+want+"\n"), 0o644)
	os.WriteFile(filepath.Join(wrap, "tools.go"), []byte("//go:build tools\n\npackage release\n\nimport _ \""+DiscoveryModule+"/cmd/pqcota-jvmscan\"\n"), 0o644)
	env := append(os.Environ(), "GOTOOLCHAIN=local", "GOWORK=off", "CGO_ENABLED=0", "GOPROXY=file://"+filepath.ToSlash(proxy),
		"GONOSUMDB=github.com/randyinthedev-hash", "GOSUMDB=off", "GOMODCACHE="+filepath.Join(root, "gmc"), "GOFLAGS=-mod=mod -modcacherw")
	goCmd := filepath.Join(runtime.GOROOT(), "bin", "go")
	tidy := exec.Command(goCmd, "mod", "tidy")
	tidy.Dir, tidy.Env = wrap, env
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
	bin = filepath.Join(root, "pqcota-jvmscan")
	build := exec.Command(goCmd, "build", "-trimpath", "-o", bin, DiscoveryModule+"/cmd/pqcota-jvmscan")
	build.Dir, build.Env = wrap, env
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	gs, err := os.ReadFile(filepath.Join(wrap, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(gs), "\n") {
		if f := strings.Fields(l); len(f) == 3 && f[0] == DiscoveryModule && f[1] == want {
			sum = f[2]
		}
	}
	if sum == "" {
		t.Fatalf("no h1 for %s in go.sum:\n%s", DiscoveryModule, gs)
	}
	return bin, sum
}

// IC-CV11 — 상류 v0.14.0 릴리스 모양의 빌드(수집기 모듈을 의존 모듈로 받아 빌드: 주 모듈에 판과 h1, replace · VCS 없음)는
// h1 이 고정한 값과 같을 때만 확인됨이다. 같은 빌드도 고정한 h1 이 다르면 맞지 않음(판은 같지만 내용 해시가 다름)이고,
// 고정한 h1 을 모르면 확인 불가(고정 판 모름)다. **같은 판 이름에 내용만 바꾼 모듈**로 빌드한 수집기는 h1 이 달라 맞지 않음이다.
func TestReleaseShapedBuildNeedsThePinnedSum(t *testing.T) {
	bin, sum := releaseBuild(t, "release")
	got := CheckGo(bin, "linux-amd64/pqcota-jvmscan", want, sum)
	expect(t, got, Verified, ReasonOK)
	if got.Sum != sum || got.Revision != "" || got.DepsReplaced {
		t.Fatalf("want the module h1 %s, no VCS revision and no replaced dependency, got %+v", sum, got)
	}
	expect(t, CheckGo(bin, "x", want, sumPinned), Mismatch, ReasonOtherContent)
	expect(t, CheckGo(bin, "x", want, ""), Unverified, ReasonNoPin)

	tampered, otherSum := releaseBuild(t, "tampered")
	if otherSum == sum {
		t.Fatalf("changing the module content should change its h1 (%s)", sum)
	}
	expect(t, CheckGo(tampered, "x", want, sum), Mismatch, ReasonOtherContent)
}

// IC-CV13 — 생성 코드(pin_gen.go)의 고정 값은 리포의 go.mod · go.sum 과 같다. go.mod 를 올리고 `go run ./tools/genpin` 을
// 돌리지 않으면 여기서 막힌다(`make verify-generated` 도 같은 것을 잰다). 수집기 모듈(pqcota-discovery)의 고정 값이 반드시 있다.
func TestPinnedMatchesGoSum(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	require := regexp.MustCompile(`(?m)^\s*(github\.com/randyinthedev-hash/pqcota[a-z-]*)\s+(v\S+)`)
	versions := map[string]string{}
	for _, m := range require.FindAllStringSubmatch(string(mod), -1) {
		versions[m[1]] = m[2]
	}
	f, err := os.Open(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sums := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fl := strings.Fields(sc.Text()); len(fl) == 3 {
			sums[fl[0]+"@"+fl[1]] = fl[2]
		}
	}
	if _, ok := Pinned[DiscoveryModule]; !ok {
		t.Fatalf("pin_gen.go has no pin for %s", DiscoveryModule)
	}
	if len(Pinned) != len(versions) {
		t.Errorf("pin_gen.go pins %d modules, go.mod requires %d", len(Pinned), len(versions))
	}
	for m, p := range Pinned {
		if versions[m] != p.Version {
			t.Errorf("%s: pin_gen.go says %s, go.mod requires %s (run go run ./tools/genpin)", m, p.Version, versions[m])
		}
		if s := sums[m+"@"+p.Version]; s != p.Sum {
			t.Errorf("%s %s: pin_gen.go says %s, go.sum says %s (run go run ./tools/genpin)", m, p.Version, p.Sum, s)
		}
	}
	if SumFor(Pinned[DiscoveryModule].Version) != Pinned[DiscoveryModule].Sum || SumFor("v0.0.0") != "" {
		t.Error("SumFor does not follow the pin")
	}
}
