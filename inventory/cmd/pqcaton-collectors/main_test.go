// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 시험은 기본으로 linux-amd64 대상이다(arm64 는 IC-CF7).
const (
	archAsset  = "pqcota-linux-amd64.tar.gz"
	archPrefix = "pqcota-linux-amd64"
	outArch    = "linux-amd64"
	testTag    = "v9.9.9"
	testCommit = "c4d8f366fdeda2de7c6bab0b2822c91ede7d448d"
	testSum    = "h1:VdoBmrIMS9B0atRmzkqvSCOxgdtThVM1C2fpFyfu5b4="
)

// entry — 가짜 묶음의 항목 하나.
type entry struct {
	name string
	body string
	kind byte // tar.TypeReg 등(0이면 보통 파일)
	link string
}

// release — 가짜 릴리스 자산. 고치고 나서 build 로 묶는다.
type release struct {
	arch     string // 대상(linux-amd64 · linux-arm64)
	tag      string
	entries  []entry
	files    map[string]string // manifest 가 적는 묶음 안 파일 → 내용(해시를 만든다)
	jarMF    string
	jarEntry struct{ version, commit, dirty string }
	commit   string // manifest 의 pqcota-discovery 커밋
	sumsSkip string // SHA256SUMS 에서 뺄 자산
	modSum   string // manifest 가 적는 discovery 모듈 h1
}

func newRelease() *release { return newReleaseFor(outArch) }

func newReleaseFor(arch string) *release {
	r := &release{arch: arch, tag: testTag, files: map[string]string{}, commit: testCommit, modSum: testSum}
	prefix := "pqcota-" + arch
	r.entries = append(r.entries, entry{name: prefix + "/", kind: tar.TypeDir})
	for _, b := range wantBins {
		body := "binary " + arch + " " + b
		r.entries = append(r.entries, entry{name: prefix + "/" + b, body: body})
		r.files[prefix+"/"+b] = body
	}
	r.jarMF = "Manifest-Version: 1.0\nImplementation-Version: " + testTag + "\nX-Pqcota-Commit: " + testCommit + "\nX-Pqcota-Dirty: false\n"
	r.jarEntry.version, r.jarEntry.commit, r.jarEntry.dirty = testTag, testCommit, "false"
	return r
}

func tarGz(t *testing.T, es []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range es {
		h := &tar.Header{Name: e.name, Mode: 0o755, Typeflag: e.kind, Linkname: e.link}
		if e.kind == 0 {
			h.Typeflag = tar.TypeReg
		}
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func jarBytes(t *testing.T, mf string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("META-INF/MANIFEST.MF")
	f.Write([]byte(mf))
	w.Close()
	return buf.Bytes()
}

// build — 자산 넷을 만든다(이름 → 내용).
func (r *release) build(t *testing.T) map[string][]byte {
	t.Helper()
	arch := tarGz(t, r.entries)
	jar := jarBytes(t, r.jarMF)
	var files []map[string]any
	for p, body := range r.files {
		files = append(files, map[string]any{"path": p, "sha256": sha([]byte(body)), "kind": "go",
			"main":    "github.com/randyinthedev-hash/pqcota-discovery/cmd/" + filepath.Base(p),
			"modules": []map[string]string{{"path": "github.com/randyinthedev-hash/pqcota-discovery", "version": r.tag, "sum": r.modSum}}})
	}
	m := map[string]any{"format": "pqcota-release-manifest/1", "tag": r.tag, "go": "go1.26.11",
		"repos": []map[string]string{{"name": "pqcota-discovery", "commit": r.commit, "origin_ref": "refs/tags/" + r.tag}},
		"assets": []map[string]any{
			{"name": "pqcota-" + r.arch + ".tar.gz", "sha256": sha(arch), "files": files},
			{"name": jarAsset, "sha256": sha(jar), "jar": map[string]string{"version": r.jarEntry.version, "commit": r.jarEntry.commit, "dirty": r.jarEntry.dirty}},
		}}
	mb, _ := json.Marshal(m)
	var sums strings.Builder
	for name, b := range map[string][]byte{"pqcota-" + r.arch + ".tar.gz": arch, jarAsset: jar, manifestName: mb} {
		if name != r.sumsSkip {
			fmt.Fprintf(&sums, "%s  %s\n", sha(b), name)
		}
	}
	return map[string][]byte{"pqcota-" + r.arch + ".tar.gz": arch, jarAsset: jar, manifestName: mb, sumsName: []byte(sums.String())}
}

// fakeAtt — 증명 묶음이 파일의 다이제스트에 묶인 것을 흉내 낸다. deny 에 든 파일 이름은 확인에 실패한다.
type fakeAtt struct{ deny string }

func (fakeAtt) Download(file, bundle string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return os.WriteFile(bundle, []byte("attests "+sha(b)), 0o644)
}

func (a fakeAtt) Verify(file, bundle, tag string) error {
	b, _ := os.ReadFile(file)
	got, _ := os.ReadFile(bundle)
	if a.deny != "" && filepath.Base(file) == a.deny {
		return errors.New("attestation of " + a.deny + ": the signer does not match")
	}
	if string(got) != "attests "+sha(b) || tag != testTag {
		return errors.New("attestation of " + filepath.Base(file) + ": no matching attestation")
	}
	return nil
}

func testTool(t *testing.T, assets map[string][]byte) (*tool, *int, *[]string) {
	t.Helper()
	calls, logs := 0, []string{}
	tl := &tool{
		get: func(u string) ([]byte, error) {
			calls++
			if !strings.HasPrefix(u, "https://github.com/"+upstreamRepo+"/releases/download/"+testTag+"/") {
				return nil, fmt.Errorf("unexpected URL %s", u)
			}
			b, ok := assets[filepath.Base(u)]
			if !ok {
				return nil, errors.New("HTTP 404")
			}
			return b, nil
		},
		att: fakeAtt{}, pinned: testTag, sum: testSum,
		logf:  func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) },
		build: func(string, mfile, string, string) error { return nil }, arch: outArch,
	}
	return tl, &calls, &logs
}

// leftovers — parent 아래에 남은 임시 폴더.
func leftovers(t *testing.T, parent string) []string {
	t.Helper()
	ents, _ := os.ReadDir(parent)
	var out []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".collectors-fetch-") {
			out = append(out, e.Name())
		}
	}
	return out
}

// IC-CF1 — **공식 릴리스 자산만 정해진 순서로 받아 확인한 뒤 한 번에 옮긴다.** 받기 → 자산의 출처 증명 → SHA256SUMS → 안전한
// 풀기 → manifest 대조 → 꺼낸 실행 파일의 출처 증명 → 예상한 파일만 → 원자적 이동의 여덟 단계를 거치고, 대상에는 정해진
// 파일만 있다(수집기 셋 · collector.jar · manifest · SHA256SUMS · 태그 · 증명 묶음). 다시 확인(verify)해도 통과한다.
func TestFetchPreparesOnlyVerifiedAssets(t *testing.T) {
	tl, _, logs := testTool(t, newRelease().build(t))
	parent := t.TempDir()
	out := filepath.Join(parent, "collectors-"+testTag)
	if err := tl.fetch(out, testTag); err != nil {
		t.Fatal(err)
	}
	have, err := listFiles(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := sameSet(have, tl.expectedFiles()); err != nil {
		t.Fatal(err)
	}
	if len(*logs) != 8 || !strings.HasPrefix((*logs)[0], "1/8 ") || !strings.HasPrefix((*logs)[7], "8/8 ") {
		t.Errorf("want the eight steps in order, got %q", *logs)
	}
	if err := tl.verify(out, testTag); err != nil {
		t.Errorf("verify after fetch: %v", err)
	}
	if l := leftovers(t, parent); len(l) != 0 {
		t.Errorf("temporary folders left: %v", l)
	}
}

// IC-CF2 — **go.mod 가 고정한 판이 아닌 태그는 받기 전에 거절한다.** 아무것도 내려받지 않고 아무것도 만들지 않는다.
func TestFetchRefusesAnotherTag(t *testing.T) {
	tl, calls, _ := testTool(t, newRelease().build(t))
	out := filepath.Join(t.TempDir(), "c")
	if err := tl.fetch(out, "v0.13.0"); err == nil || !strings.Contains(err.Error(), "not the upstream version go.mod pins") {
		t.Fatalf("want a refusal before downloading, got %v", err)
	}
	if *calls != 0 {
		t.Errorf("%d downloads happened before the refusal", *calls)
	}
	if _, err := os.Lstat(out); err == nil {
		t.Error("the target was created")
	}
}

// IC-CF3 — **자산이 불완전하거나 바뀌었으면 막고, 대상 폴더와 부분 파일을 남기지 않는다.** 묶음 안 파일이 빠짐 · 실행 파일 한
// 바이트가 다름(묶음의 해시는 맞게 바꿔 manifest 의 파일 해시만 다르다) · 다른 태그의 자산 · 경로 탈출(`..`) · 절대 경로 ·
// 심볼릭 링크 · 하드 링크 · manifest 에 없는 파일 · 같은 경로 두 번 · 대소문자 충돌 · 예상 밖 폴더 · SHA256SUMS 에 자산이
// 빠짐 · 자산의 출처 증명 실패 · 꺼낸 실행 파일의 출처 증명 실패 · JAR 의 커밋이 manifest 와 다름 · 상류 h1 이 이 리포가
// 고정한 값과 다름.
func TestFetchRefusesBadAssets(t *testing.T) {
	for name, mutate := range map[string]func(r *release, tl *tool){
		"a file is missing": func(r *release, _ *tool) { r.entries = r.entries[:len(r.entries)-1] },
		"one byte changed": func(r *release, _ *tool) {
			r.entries[1].body = "binary linux-amd64 pqcota-jvmscaN"
		},
		"another tag": func(r *release, _ *tool) { r.tag = "v0.13.0" },
		"path escape": func(r *release, _ *tool) {
			r.entries = append(r.entries, entry{name: archPrefix + "/../evil", body: "x"})
		},
		"absolute path": func(r *release, _ *tool) { r.entries = append(r.entries, entry{name: "/etc/evil", body: "x"}) },
		"symbolic link": func(r *release, _ *tool) {
			r.entries = append(r.entries, entry{name: archPrefix + "/ln", kind: tar.TypeSymlink, link: "/etc/passwd"})
		},
		"hard link": func(r *release, _ *tool) {
			r.entries = append(r.entries, entry{name: archPrefix + "/hl", kind: tar.TypeLink, link: archPrefix + "/pqcota-netcap"})
		},
		"extra file": func(r *release, _ *tool) {
			r.entries = append(r.entries, entry{name: archPrefix + "/extra", body: "x"})
		},
		"duplicate path": func(r *release, _ *tool) { r.entries = append(r.entries, r.entries[1]) },
		"case collision": func(r *release, _ *tool) {
			r.entries = append(r.entries, entry{name: archPrefix + "/PQCOTA-NETCAP", body: "x"})
		},
		"extra folder": func(r *release, _ *tool) {
			r.entries = append(r.entries, entry{name: archPrefix + "/sub/", kind: tar.TypeDir})
		},
		"sums miss a jar": func(r *release, _ *tool) { r.sumsSkip = jarAsset },
		"asset attestation": func(_ *release, tl *tool) {
			tl.att = fakeAtt{deny: archAsset}
		},
		"collector attestation": func(_ *release, tl *tool) { tl.att = fakeAtt{deny: "pqcota-netcap"} },
		"jar commit":            func(r *release, _ *tool) { r.jarEntry.commit = strings.Repeat("a", 40) },
		"another h1":            func(r *release, _ *tool) { r.modSum = "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" },
	} {
		t.Run(name, func(t *testing.T) {
			r := newRelease()
			tl, _, _ := testTool(t, nil)
			mutate(r, tl)
			if name == "one byte changed" {
				// 묶음 · SHA256SUMS · 자산 해시는 바뀐 묶음에 맞고, manifest 의 파일 해시만 원래 것이다.
				r.files[archPrefix+"/"+wantBins[0]] = "binary linux-amd64 " + wantBins[0]
			}
			a := r.build(t)
			assets := a
			tl.get = func(u string) ([]byte, error) { return assets[filepath.Base(u)], nil }
			parent := t.TempDir()
			out := filepath.Join(parent, "c")
			if err := tl.fetch(out, testTag); err == nil {
				t.Fatal("a bad release was accepted")
			} else {
				t.Logf("refused: %v", err)
			}
			if _, err := os.Lstat(out); err == nil {
				t.Error("the target was created")
			}
			if l := leftovers(t, parent); len(l) != 0 {
				t.Errorf("temporary folders left: %v", l)
			}
		})
	}
}

// IC-CF4 — **대상이 이미 있으면 덮어쓰지 않는다.** 같은 내용으로 확인되면 그대로 쓰고, 바뀌었으면 실패하며 그 폴더를 고치지
// 않는다.
func TestFetchNeverOverwrites(t *testing.T) {
	tl, _, logs := testTool(t, newRelease().build(t))
	out := filepath.Join(t.TempDir(), "c")
	if err := tl.fetch(out, testTag); err != nil {
		t.Fatal(err)
	}
	*logs = nil
	if err := tl.fetch(out, testTag); err != nil || len(*logs) != 1 || !strings.Contains((*logs)[0], "reused") {
		t.Fatalf("a second fetch should reuse the verified folder: %v %q", err, *logs)
	}
	bin := filepath.Join(out, "collectors", outArch, wantBins[1])
	os.WriteFile(bin, []byte("tampered"), 0o755)
	if err := tl.fetch(out, testTag); err == nil || !strings.Contains(err.Error(), "left as it is") {
		t.Fatalf("a changed folder should be refused, got %v", err)
	}
	if b, _ := os.ReadFile(bin); string(b) != "tampered" {
		t.Error("the existing folder was changed")
	}
}

// IC-CF5 — **준비한 폴더를 쓰기 직전에 다시 확인한다.** 실행 파일 한 바이트가 바뀜 · 파일이 더해짐 · 파일이 빠짐 · 다른 태그로
// 준비됨 · manifest 가 SHA256SUMS 와 어긋남이면 막는다. 보관한 증명 묶음으로 출처 증명도 다시 확인한다.
func TestVerifyCatchesChangesAfterPreparing(t *testing.T) {
	for name, change := range map[string]func(dir string){
		"one byte changed": func(dir string) {
			p := filepath.Join(dir, "collectors", outArch, wantBins[2])
			b, _ := os.ReadFile(p)
			b[0] ^= 1
			os.WriteFile(p, b, 0o755)
		},
		"extra file":   func(dir string) { os.WriteFile(filepath.Join(dir, "collectors", "extra"), []byte("x"), 0o644) },
		"missing file": func(dir string) { os.Remove(filepath.Join(dir, "collectors", jarAsset)) },
		"another tag":  func(dir string) { os.WriteFile(filepath.Join(dir, "provenance", "TAG"), []byte("v0.13.0\n"), 0o644) },
		"manifest edited": func(dir string) {
			p := filepath.Join(dir, "provenance", manifestName)
			b, _ := os.ReadFile(p)
			os.WriteFile(p, append(b, ' '), 0o644)
		},
		"bundle swapped": func(dir string) {
			os.WriteFile(filepath.Join(dir, "provenance", "attestations", bundleName(jarAsset)), []byte("attests 00"), 0o644)
		},
	} {
		t.Run(name, func(t *testing.T) {
			tl, _, _ := testTool(t, newRelease().build(t))
			dir := filepath.Join(t.TempDir(), "c")
			if err := tl.fetch(dir, testTag); err != nil {
				t.Fatal(err)
			}
			change(dir)
			if err := tl.verify(dir, testTag); err == nil {
				t.Fatal("a changed folder passed verify")
			} else {
				t.Logf("refused: %v", err)
			}
		})
	}
}

// IC-CF6 — **HTTPS 와 정해진 호스트만 받는다.** http 주소, 다른 호스트, http 로 가는 리디렉션은 거절한다.
func TestOnlyHTTPS(t *testing.T) {
	if err := allowedURL("https", "github.com"); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"http://github.com/x", "https://example.com/x", "ftp://github.com/x"} {
		p, _ := url.Parse(u)
		if allowedURL(p.Scheme, p.Hostname()) == nil {
			t.Errorf("%s was allowed", u)
		}
	}
	redirect, _ := http.NewRequest(http.MethodGet, "http://objects.githubusercontent.com/x", nil)
	if checkRedirect(redirect, []*http.Request{{}}) == nil {
		t.Error("a redirect to http was followed")
	}
	if _, err := httpsGet("http://github.com/x"); err == nil {
		t.Error("httpsGet fetched an http URL")
	}
}

// IC-CF7 — **대상 OS · 아키텍처는 늘 명시하고 준비 폴더에 적는다.** linux-arm64 대상은 arm64 묶음을 받아 collectors/linux-arm64 에
// 두고 provenance/TARGET 에 linux/arm64 를 적는다. 그 폴더를 linux-amd64 로 다시 확인하면 파일 목록보다 먼저 대상이 달라 막는다.
// 지원하지 않는 대상(darwin-arm64)은 받기 전에 거절한다. 실행 파일의 Go 빌드 정보에 적힌 GOOS · GOARCH 가 대상과 다르면
// 「exec format error」가 나기 전에 막는다(실행하지 않고 읽는다).
func TestTargetArchitecture(t *testing.T) {
	tl, _, _ := testTool(t, newReleaseFor("linux-arm64").build(t))
	tl.arch = "linux-arm64"
	out := filepath.Join(t.TempDir(), "c")
	if err := tl.fetch(out, testTag); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "provenance", "TARGET")); string(b) != "linux/arm64\n" {
		t.Errorf("TARGET = %q, want linux/arm64", b)
	}
	if _, err := os.Stat(filepath.Join(out, "collectors", "linux-arm64", "pqcota-nodescan")); err != nil {
		t.Errorf("the arm64 collectors are not under collectors/linux-arm64: %v", err)
	}
	tl.arch = "linux-amd64"
	if err := tl.verify(out, testTag); err == nil || !strings.Contains(err.Error(), "prepared for linux/arm64, not linux/amd64") {
		t.Errorf("verifying an arm64 folder for amd64 = %v, want the target refused first", err)
	}
	other, calls, _ := testTool(t, newRelease().build(t))
	other.arch = "darwin-arm64"
	if err := other.fetch(filepath.Join(t.TempDir(), "c"), testTag); err == nil || !strings.Contains(err.Error(), "not supported") || *calls != 0 {
		t.Errorf("an unsupported target = %v after %d downloads, want a refusal before downloading", err, *calls)
	}

	// 실제 Go 빌드 정보: arm64 로 빌드한 프로그램은 linux/amd64 대상에서 막힌다.
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "go.mod"), []byte("module example.com/x\n\ngo 1.21\n"), 0o644)
	os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	bin := filepath.Join(t.TempDir(), "x")
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", bin, ".")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOFLAGS=-mod=mod", "GOWORK=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	if err := checkBuild(bin, mfile{Main: "example.com/x"}, testTag, "linux/amd64"); err == nil || !strings.Contains(err.Error(), "built for linux/arm64, not linux/amd64") {
		t.Errorf("checkBuild of an arm64 program for linux/amd64 = %v, want the architecture refused", err)
	}
	if err := checkBuild(bin, mfile{Main: "example.com/x"}, testTag, "linux/arm64"); err != nil {
		t.Errorf("checkBuild of an arm64 program for linux/arm64 = %v", err)
	}
}
