// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// collectors — 상류 pqcota 의 공식 릴리스 수집기를 받아 확인하고(fetch), 받아 둔 것을 쓰기 직전에 다시 확인한다(verify).
//
// 데모(ui.sh)와 종단 검증이 쓰는 **유일한 수집기 경로**다. 받은 파일은 실행하지 않는다(Go 빌드 정보와 JAR manifest 만 읽는다).
//
// fetch 의 순서(하나라도 실패하면 대상 폴더를 만들거나 바꾸지 않고, 임시 폴더를 지운다):
//
//  1. 모든 자산을 대상 옆의 새 임시 폴더에 받는다(HTTPS 만, 리디렉션도 HTTPS 와 정해진 호스트만).
//  2. 자산의 출처 증명(attestation)을 저장소 · 서명 워크플로 · 태그 ref 에 고정해 확인한다. 증명 묶음(bundle)은 보관한다.
//  3. SHA256SUMS 가 받은 자산을 정확히 담고 해시가 같은지 본다.
//  4. 압축을 안전하게 푼다(절대 경로 · `..` · 링크 · 장치 파일 · manifest 에 없는 파일 · 중복 · 대소문자 충돌 · 정해진 폴더 밖을 거절).
//  5. release manifest 의 파일마다 해시 · 판 · 커밋과 견준다(이 리포가 고정한 h1 과도 견준다).
//  6. 압축에서 꺼낸 실행 파일의 출처 증명을 확인한다.
//  7. 예상한 파일만 있는지 본다.
//  8. 완성한 폴더를 대상 자리로 원자적으로 옮긴다(rename).
//
// 대상이 이미 있으면 덮어쓰지 않는다. verify 로 같은 내용인지 확인되면 그대로 쓰고, 아니면 실패한다.
//
// usage:
//
//	go run ./tools/collectors fetch  -arch linux-amd64|linux-arm64 -out <폴더> [-tag vX.Y.Z]
//	go run ./tools/collectors verify -arch linux-amd64|linux-arm64 -dir <폴더> [-tag vX.Y.Z]
//	collectors recover -ws <작업 폴더>   /  collectors install -ws <작업 폴더> -from <작업 폴더 밖 source>   (화면이 도는 기계 안에서, install.go)
//
// 대상(-arch)은 따로 고르지 않고 늘 명시한다. 준비 폴더의 provenance/TARGET 에 적히고, verify 와 ui.sh 가 다시 견준다.
package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/collectorver"
)

const (
	// upstreamRepo — 자산을 받는 상류 저장소. 출처 증명도 이 저장소 · 이 워크플로 · 태그 ref 로 고정한다.
	upstreamRepo = "randyinthedev-hash/pqcota"
	workflow     = upstreamRepo + "/.github/workflows/release.yml"
	jarAsset     = "collector.jar"
	manifestName = "pqcota-release-manifest.json"
	sumsName     = "SHA256SUMS"
	maxAsset     = 64 << 20 // 자산 하나의 최대 크기
	maxFile      = 64 << 20 // 묶음 안 파일 하나의 최대 크기
	minGh        = "2.49.0" // gh attestation 이 들어온 판
)

// wantBins — 노드용 묶음에 있어야 하는 수집기. 이것 말고는 받지 않는다.
var wantBins = []string{"pqcota-jvmscan", "pqcota-netcap", "pqcota-nodescan"}

// targets — 지원하는 대상(준비 폴더의 아키텍처 이름 → OS/아키텍처). 노드와 화면을 돌리는 컨테이너가 이 가운데 하나다.
var targets = map[string]struct{ goos, goarch string }{"linux-amd64": {"linux", "amd64"}, "linux-arm64": {"linux", "arm64"}}

// archAsset · archPrefix — 대상의 노드용 묶음과 그 안의 폴더. 작업 폴더의 collectors/ 아래 이름은 t.arch 그대로다.
func (t *tool) archAsset() string  { return "pqcota-" + t.arch + ".tar.gz" }
func (t *tool) archPrefix() string { return "pqcota-" + t.arch }

// target — 준비 폴더의 provenance/TARGET 에 적는 값(예: linux/arm64).
func (t *tool) target() string { x := targets[t.arch]; return x.goos + "/" + x.goarch }

// assets — 받는 자산. 모두 출처 증명을 확인한다.
func (t *tool) assets() []string { return []string{manifestName, sumsName, t.archAsset(), jarAsset} }

// allowedHosts — 자산을 받을 때 허용하는 호스트(리디렉션 포함).
var allowedHosts = map[string]bool{"github.com": true, "objects.githubusercontent.com": true, "release-assets.githubusercontent.com": true}

// manifest — 상류 pqcota-release-manifest/1.
type manifest struct {
	Format string `json:"format"`
	Tag    string `json:"tag"`
	Repos  []struct {
		Name      string `json:"name"`
		Commit    string `json:"commit"`
		OriginRef string `json:"origin_ref"`
	} `json:"repos"`
	Assets []struct {
		Name   string  `json:"name"`
		SHA256 string  `json:"sha256"`
		Files  []mfile `json:"files"`
		Jar    *struct {
			Version string `json:"version"`
			Commit  string `json:"commit"`
			Dirty   string `json:"dirty"`
		} `json:"jar"`
	} `json:"assets"`
}

type mfile struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Kind    string `json:"kind"`
	Main    string `json:"main"`
	Modules []struct {
		Path    string `json:"path"`
		Version string `json:"version"`
		Sum     string `json:"sum"`
	} `json:"modules"`
}

// attester — 출처 증명을 다루는 쪽. 시험이 바꾼다.
type attester interface {
	// Download 는 file 의 증명 묶음을 bundle 에 저장한다. Verify 는 그 묶음으로 file 을 확인한다(저장소 · 워크플로 · 태그 고정).
	Download(file, bundle string) error
	Verify(file, bundle, tag string) error
}

// tool — fetch · verify 가 쓰는 의존. 시험이 바꾼다.
type tool struct {
	get    func(url string) ([]byte, error) // 자산 하나를 받는다
	att    attester
	logf   func(format string, args ...any)
	pinned string // 이 리포가 고정한 상류 판(collectorver.Pinned)
	sum    string // 고정한 pqcota-discovery 의 h1
	build  func(file string, f mfile, want, target string) error
	arch   string // 대상(targets 의 키). fetch · verify 가 -arch 로 받는다
	// screenRule — 화면과 같은 판정(collectorver.CheckGo)으로도 「확인됨」인지 본다. 가짜 수집기를 쓰는 시험만 끈다.
	screenRule bool
}

func newTool() *tool {
	p := collectorver.Pinned[collectorver.DiscoveryModule]
	return &tool{get: httpsGet, att: &ghAttester{}, logf: func(f string, a ...any) { fmt.Printf(f+"\n", a...) },
		pinned: p.Version, sum: p.Sum, build: checkBuild, screenRule: true}
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func shaFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checkRedirect — 리디렉션도 HTTPS 와 정해진 호스트만 따른다.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 5 {
		return errors.New("too many redirects")
	}
	return allowedURL(req.URL.Scheme, req.URL.Hostname())
}

func allowedURL(scheme, host string) error {
	if scheme != "https" {
		return fmt.Errorf("refusing a %s URL: only https is allowed", scheme)
	}
	if !allowedHosts[host] {
		return fmt.Errorf("refusing the host %s", host)
	}
	return nil
}

// httpsGet — 자산 하나를 받는다. 인증 정보를 보내지 않고(공개 자산), 응답 헤더를 기록하지 않는다.
func httpsGet(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if err := allowedURL(req.URL.Scheme, req.URL.Hostname()); err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "pqcaton-collectors")
	c := &http.Client{Timeout: 5 * time.Minute, CheckRedirect: checkRedirect}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxAsset+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxAsset {
		return nil, fmt.Errorf("the asset is larger than %d bytes", maxAsset)
	}
	return b, nil
}

func assetURL(tag, name string) string {
	return "https://github.com/" + upstreamRepo + "/releases/download/" + tag + "/" + name
}

// ghAttester — gh attestation 으로 확인한다. gh 의 출력은 실패할 때 첫 줄만 보인다(토큰 · 헤더를 싣지 않는다).
//
// root 는 이번 실행이 시작할 때 `gh attestation trusted-root` 로 새로 받은 신뢰 루트다. 확인마다 신뢰 루트를 다시 받으면
// 한 번에 몇 초씩 걸려, 한 실행 안에서는 한 번 받은 것을 쓴다. **준비 폴더에는 보관하지 않는다**(묶음과 함께 바꿔치기할 수 있다).
type ghAttester struct{ root string }

// fetchRoot — 신뢰 루트를 dir 아래 파일로 받는다.
func (g *ghAttester) fetchRoot(dir string) error {
	out, err := exec.Command("gh", "attestation", "trusted-root").Output()
	if err != nil || len(out) == 0 {
		return errors.New("gh attestation trusted-root failed: the trust root for the attestations could not be fetched")
	}
	g.root = filepath.Join(dir, "trusted-root.jsonl")
	return os.WriteFile(g.root, out, 0o600)
}

func (g *ghAttester) Download(file, bundle string) error {
	dir, err := os.MkdirTemp(filepath.Dir(bundle), ".bundle-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	abs, _ := filepath.Abs(file)
	cmd := exec.Command("gh", "attestation", "download", abs, "--repo", upstreamRepo)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh attestation download %s: %s", filepath.Base(file), firstLine(out))
	}
	ents, err := os.ReadDir(dir)
	if err != nil || len(ents) != 1 || !strings.HasSuffix(ents[0].Name(), ".jsonl") {
		return fmt.Errorf("gh attestation download %s: want one bundle file", filepath.Base(file))
	}
	return os.Rename(filepath.Join(dir, ents[0].Name()), bundle)
}

func (g *ghAttester) Verify(file, bundle, tag string) error {
	args := []string{"attestation", "verify", file, "--bundle", bundle, "--repo", upstreamRepo,
		"--signer-workflow", workflow, "--source-ref", "refs/tags/" + tag, "--deny-self-hosted-runners"}
	if g.root != "" {
		args = append(args, "--custom-trusted-root", g.root)
	}
	cmd := exec.Command("gh", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("attestation of %s: %s", filepath.Base(file), firstLine(out))
	}
	return nil
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// checkGh — gh 가 있고 판이 minGh 이상이며 로그인되어 있는가. 로그인 정보는 출력하지 않는다.
func checkGh() error {
	out, err := exec.Command("gh", "--version").Output()
	if err != nil {
		return errors.New("gh is required (GitHub CLI " + minGh + " or later) to verify the release attestations")
	}
	m := regexp.MustCompile(`gh version (\d+)\.(\d+)\.(\d+)`).FindSubmatch(out)
	if m == nil || !atLeast(string(m[1]), string(m[2]), string(m[3]), minGh) {
		return fmt.Errorf("gh %s or later is required for gh attestation", minGh)
	}
	if err := exec.Command("gh", "auth", "status").Run(); err != nil {
		return errors.New("gh is not logged in: run gh auth login (gh attestation needs it)")
	}
	return nil
}

func atLeast(a, b, c, min string) bool {
	p := strings.Split(min, ".")
	got := []string{a, b, c}
	for i := range 3 {
		x, _ := strconv.Atoi(got[i])
		y, _ := strconv.Atoi(p[i])
		if x != y {
			return x > y
		}
	}
	return true
}

// expectedFiles — 준비한 폴더에 있어야 하는 파일 전부(이것 말고는 없어야 한다).
func (t *tool) expectedFiles() []string {
	outArch, archAsset := t.arch, t.archAsset()
	out := []string{"collectors/" + jarAsset, "provenance/" + manifestName, "provenance/" + sumsName, "provenance/TAG", "provenance/TARGET",
		"provenance/attestations/" + jarAsset + ".jsonl", "provenance/attestations/" + manifestName + ".jsonl",
		"provenance/attestations/" + sumsName + ".jsonl", "provenance/attestations/" + archAsset + ".jsonl"}
	for _, b := range wantBins {
		out = append(out, "collectors/"+outArch+"/"+b, "provenance/attestations/"+outArch+"-"+b+".jsonl")
	}
	sort.Strings(out)
	return out
}

// listFiles — dir 아래의 파일(상대 경로). 보통 파일과 폴더가 아닌 것(링크 등)이 있으면 실패한다.
func listFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", rel)
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}

func sameSet(got, want []string) error {
	g, w := map[string]bool{}, map[string]bool{}
	for _, x := range got {
		g[x] = true
	}
	for _, x := range want {
		w[x] = true
	}
	var extra, missing []string
	for x := range g {
		if !w[x] {
			extra = append(extra, x)
		}
	}
	for x := range w {
		if !g[x] {
			missing = append(missing, x)
		}
	}
	sort.Strings(extra)
	sort.Strings(missing)
	if len(extra)+len(missing) > 0 {
		return fmt.Errorf("unexpected files %v, missing files %v", extra, missing)
	}
	return nil
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// parseSums — SHA256SUMS 를 엄격하게 읽는다(`<64자 해시>  <이름>`, 이름 중복 없음).
func parseSums(b []byte) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		h, name, ok := strings.Cut(line, "  ")
		if !ok || !hex64.MatchString(h) || name == "" || strings.ContainsAny(name, "/\\ ") {
			return nil, fmt.Errorf("SHA256SUMS has a malformed line")
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("SHA256SUMS names %s twice", name)
		}
		out[name] = h
	}
	return out, nil
}

// readManifest — manifest 를 읽고 태그 · 형식 · 저장소 ref 를 확인한다.
func readManifest(b []byte, tag string) (*manifest, error) {
	var m manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("release manifest: %w", err)
	}
	if m.Format != "pqcota-release-manifest/1" {
		return nil, fmt.Errorf("release manifest format %q is not pqcota-release-manifest/1", m.Format)
	}
	if m.Tag != tag {
		return nil, fmt.Errorf("the release manifest is for %s, not %s", m.Tag, tag)
	}
	for _, r := range m.Repos {
		if r.OriginRef != "refs/tags/"+tag {
			return nil, fmt.Errorf("the release manifest says %s was built from %s, not refs/tags/%s", r.Name, r.OriginRef, tag)
		}
	}
	return &m, nil
}

func (m *manifest) asset(name string) (sha256 string, files []mfile, jar *struct{ Version, Commit, Dirty string }, ok bool) {
	for _, a := range m.Assets {
		if a.Name == name {
			if a.Jar != nil {
				jar = &struct{ Version, Commit, Dirty string }{a.Jar.Version, a.Jar.Commit, a.Jar.Dirty}
			}
			return a.SHA256, a.Files, jar, true
		}
	}
	return "", nil, nil, false
}

func (m *manifest) commit(repo string) string {
	for _, r := range m.Repos {
		if r.Name == repo {
			return r.Commit
		}
	}
	return ""
}

// extract — 노드용 묶음을 dst 아래에 안전하게 푼다. 허용하는 것은 archPrefix 폴더 하나와, manifest 가 적은 수집기 파일뿐이다.
func extract(archive []byte, dst, archPrefix string, files []mfile) error {
	allowed := map[string]bool{}
	for _, f := range files {
		allowed[f.Path] = true
	}
	for _, b := range wantBins {
		if !allowed[archPrefix+"/"+b] {
			return fmt.Errorf("the release manifest does not list %s/%s", archPrefix, b)
		}
	}
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	seen, fold := map[string]bool{}, map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(h.Name, "/")
		clean := path.Clean(name)
		switch {
		case name == "" || strings.HasPrefix(h.Name, "/") || strings.Contains(h.Name, "\\") || filepath.IsAbs(h.Name):
			return fmt.Errorf("the archive holds an absolute or malformed path %q", h.Name)
		case clean != name || clean == ".." || strings.HasPrefix(clean, "../"):
			return fmt.Errorf("the archive holds a path that is not plain (%q)", h.Name)
		case clean != archPrefix && !strings.HasPrefix(clean, archPrefix+"/"):
			return fmt.Errorf("the archive holds %q outside %s/", h.Name, archPrefix)
		}
		low := strings.ToLower(clean)
		if seen[clean] {
			return fmt.Errorf("the archive holds %q twice", clean)
		}
		if other, ok := fold[low]; ok {
			return fmt.Errorf("the archive holds %q and %q, which collide without case", other, clean)
		}
		seen[clean], fold[low] = true, clean
		switch h.Typeflag {
		case tar.TypeDir:
			if clean != archPrefix {
				return fmt.Errorf("the archive holds an unexpected folder %q", clean)
			}
		case tar.TypeReg:
			if !allowed[clean] {
				return fmt.Errorf("the archive holds %q, which the release manifest does not list", clean)
			}
			if h.Size > maxFile {
				return fmt.Errorf("%s is larger than %d bytes", clean, maxFile)
			}
			p := filepath.Join(dst, filepath.FromSlash(clean))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, io.LimitReader(tr, maxFile))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("the archive holds %q, which is not a regular file (links and devices are refused)", clean)
		}
	}
	for p := range allowed {
		if !seen[p] {
			return fmt.Errorf("the archive does not hold %s, which the release manifest lists", p)
		}
	}
	return nil
}

// checkBuild — 수집기의 Go 빌드 정보(실행하지 않는다)가 manifest 의 항목과 같은가: 주 패키지, 상류 모듈마다 판과 h1.
func checkBuild(file string, f mfile, want, target string) error {
	bi, err := buildinfo.ReadFile(file)
	if err != nil {
		return fmt.Errorf("%s has no Go build information", filepath.Base(file))
	}
	// 대상 OS · 아키텍처(빌드 정보의 GOOS · GOARCH). 다르면 그 기계에서 「exec format error」가 날 파일이다.
	var goos, goarch string
	for _, st := range bi.Settings {
		switch st.Key {
		case "GOOS":
			goos = st.Value
		case "GOARCH":
			goarch = st.Value
		}
	}
	if goos+"/"+goarch != target {
		return fmt.Errorf("%s is built for %s/%s, not %s", filepath.Base(file), goos, goarch, target)
	}
	if bi.Path != f.Main {
		return fmt.Errorf("%s is built from %s, the release manifest says %s", filepath.Base(file), bi.Path, f.Main)
	}
	got := map[string]string{bi.Main.Path: bi.Main.Version + " " + bi.Main.Sum}
	for _, d := range bi.Deps {
		if d.Replace != nil {
			return fmt.Errorf("%s replaces %s", filepath.Base(file), d.Path)
		}
		got[d.Path] = d.Version + " " + d.Sum
	}
	for _, m := range f.Modules {
		if got[m.Path] != m.Version+" "+m.Sum {
			return fmt.Errorf("%s: %s is %q in its build information, the release manifest says %s %s",
				filepath.Base(file), m.Path, got[m.Path], m.Version, m.Sum)
		}
		if m.Version != want {
			return fmt.Errorf("%s: %s is %s, not %s", filepath.Base(file), m.Path, m.Version, want)
		}
	}
	return nil
}

// checkCollectors — collectors/ 아래 파일마다 manifest 의 해시 · 판 · 커밋과 같은가(fetch 의 5와 verify 가 함께 쓴다).
func (t *tool) checkCollectors(dir string, m *manifest, tag string) error {
	archAsset, archPrefix, outArch := t.archAsset(), t.archPrefix(), t.arch
	_, files, _, ok := m.asset(archAsset)
	if !ok {
		return fmt.Errorf("the release manifest does not list %s", archAsset)
	}
	byPath := map[string]mfile{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	for _, b := range wantBins {
		f, ok := byPath[archPrefix+"/"+b]
		if !ok {
			return fmt.Errorf("the release manifest does not list %s/%s", archPrefix, b)
		}
		p := filepath.Join(dir, outArch, b)
		h, err := shaFile(p)
		if err != nil {
			return err
		}
		if h != f.SHA256 {
			return fmt.Errorf("%s/%s does not match the release manifest (SHA-256 %s, want %s)", outArch, b, h, f.SHA256)
		}
		if f.Kind != "go" {
			return fmt.Errorf("the release manifest says %s is %q, not a Go program", b, f.Kind)
		}
		if err := t.build(p, f, tag, t.target()); err != nil {
			return err
		}
		// 이 리포가 고정한 h1(go.sum)과도 같아야 한다. 화면도 같은 값으로 판정한다.
		for _, mod := range f.Modules {
			if mod.Path == collectorver.DiscoveryModule && mod.Sum != t.sum {
				return fmt.Errorf("%s: the release h1 of %s is %s, this repository pins %s", b, mod.Path, mod.Sum, t.sum)
			}
		}
		if t.screenRule {
			if got := collectorver.CheckGo(p, outArch+"/"+b, tag, t.sum); got.State != collectorver.Verified {
				return fmt.Errorf("%s/%s is not verified by the screen's rule (%s %s)", outArch, b, got.State, got.Reason)
			}
		}
	}
	jarSum, _, jar, ok := m.asset(jarAsset)
	if !ok || jar == nil {
		return fmt.Errorf("the release manifest does not describe %s", jarAsset)
	}
	jp := filepath.Join(dir, jarAsset)
	h, err := shaFile(jp)
	if err != nil {
		return err
	}
	if h != jarSum {
		return fmt.Errorf("%s does not match the release manifest (SHA-256 %s, want %s)", jarAsset, h, jarSum)
	}
	got := collectorver.CheckJar(jp, jarAsset, tag)
	if jar.Version != tag || jar.Dirty != "false" || jar.Commit != m.commit("pqcota-discovery") {
		return fmt.Errorf("the release manifest describes %s as %s %s dirty=%s, want %s, the pqcota-discovery commit and dirty=false",
			jarAsset, jar.Version, jar.Commit, jar.Dirty, tag)
	}
	if got.State != collectorver.Verified || got.Declared != jar.Version || got.Revision != jar.Commit {
		return fmt.Errorf("%s is %s (%s) with version %q and commit %q; the release manifest says %s %s",
			jarAsset, got.State, got.Reason, got.Declared, got.Revision, jar.Version, jar.Commit)
	}
	return nil
}

func bundleName(rel string) string { return strings.ReplaceAll(rel, "/", "-") + ".jsonl" }

// fetch — 위 순서대로 받아 out 에 둔다.
func (t *tool) fetch(out, tag string) error {
	if _, ok := targets[t.arch]; !ok {
		return fmt.Errorf("the target %q is not supported (linux-amd64 or linux-arm64); nothing was downloaded", t.arch)
	}
	archAsset, archPrefix, outArch := t.archAsset(), t.archPrefix(), t.arch
	if tag != t.pinned {
		return fmt.Errorf("the tag %s is not the upstream version go.mod pins (%s); nothing was downloaded", tag, t.pinned)
	}
	if _, err := os.Lstat(out); err == nil {
		if err := t.verify(out, tag); err != nil {
			return fmt.Errorf("%s already exists and is not the verified %s collectors (%v); it was left as it is", out, tag, err)
		}
		t.logf("✓ %s already holds the verified %s collectors; reused", out, tag)
		return nil
	}
	parent := filepath.Dir(out)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".collectors-fetch-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	dl := filepath.Join(tmp, "download")
	stage := filepath.Join(tmp, "out")
	prov := filepath.Join(stage, "provenance")
	bundles := filepath.Join(prov, "attestations")
	for _, d := range []string{dl, filepath.Join(stage, "collectors", outArch), bundles} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	// 1. 받는다.
	got := map[string][]byte{}
	for _, a := range t.assets() {
		b, err := t.get(assetURL(tag, a))
		if err != nil {
			return fmt.Errorf("%s: %w", a, err)
		}
		if err := os.WriteFile(filepath.Join(dl, a), b, 0o644); err != nil {
			return err
		}
		got[a] = b
	}
	t.logf("1/8 downloaded %d assets of %s for %s", len(t.assets()), tag, t.target())

	// 2. 자산의 출처 증명.
	for _, a := range t.assets() {
		b := filepath.Join(bundles, bundleName(a))
		if err := t.att.Download(filepath.Join(dl, a), b); err != nil {
			return err
		}
		if err := t.att.Verify(filepath.Join(dl, a), b, tag); err != nil {
			return err
		}
	}
	t.logf("2/8 attestations of the assets verified (%s, release.yml, refs/tags/%s)", upstreamRepo, tag)

	// 3. SHA256SUMS.
	sums, err := parseSums(got[sumsName])
	if err != nil {
		return err
	}
	for _, a := range []string{manifestName, archAsset, jarAsset} {
		if sums[a] == "" {
			return fmt.Errorf("SHA256SUMS does not list %s", a)
		}
		if sums[a] != sha(got[a]) {
			return fmt.Errorf("%s does not match SHA256SUMS", a)
		}
	}
	t.logf("3/8 SHA256SUMS lists the assets and they match")

	// 4. 안전하게 푼다.
	m, err := readManifest(got[manifestName], tag)
	if err != nil {
		return err
	}
	archSum, files, _, ok := m.asset(archAsset)
	if !ok || archSum != sha(got[archAsset]) {
		return fmt.Errorf("%s does not match the release manifest", archAsset)
	}
	x := filepath.Join(tmp, "extract")
	if err := extract(got[archAsset], x, archPrefix, files); err != nil {
		return err
	}
	for _, b := range wantBins {
		if err := os.Rename(filepath.Join(x, archPrefix, b), filepath.Join(stage, "collectors", outArch, b)); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(stage, "collectors", jarAsset), got[jarAsset], 0o644); err != nil {
		return err
	}
	t.logf("4/8 unpacked %s safely", archAsset)

	// 5. manifest 와 견준다.
	if err := t.checkCollectors(filepath.Join(stage, "collectors"), m, tag); err != nil {
		return err
	}
	t.logf("5/8 every file matches the release manifest (SHA-256, versions, h1, commit)")

	// 6. 꺼낸 실행 파일의 출처 증명.
	for _, b := range wantBins {
		f := filepath.Join(stage, "collectors", outArch, b)
		bn := filepath.Join(bundles, bundleName(outArch+"/"+b))
		if err := t.att.Download(f, bn); err != nil {
			return err
		}
		if err := t.att.Verify(f, bn, tag); err != nil {
			return err
		}
	}
	t.logf("6/8 attestations of the unpacked collectors verified")

	// 7. 예상한 파일만.
	for _, a := range []string{manifestName, sumsName} {
		if err := os.WriteFile(filepath.Join(prov, a), got[a], 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(prov, "TAG"), []byte(tag+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(prov, "TARGET"), []byte(t.target()+"\n"), 0o644); err != nil {
		return err
	}
	have, err := listFiles(stage)
	if err != nil {
		return err
	}
	if err := sameSet(have, t.expectedFiles()); err != nil {
		return err
	}
	t.logf("7/8 only the expected files are there")

	// 8. 원자적으로 옮긴다.
	if err := os.Rename(stage, out); err != nil {
		return fmt.Errorf("could not move the prepared collectors into place: %w", err)
	}
	t.logf("8/8 %s holds the verified %s collectors for %s", out, tag, t.target())
	return nil
}

// verify — 준비한 폴더를 쓰기 직전에 다시 확인한다: 예상한 파일만 있고, 파일마다 manifest 의 해시 · 판 · 커밋과 같으며,
// 보관한 증명 묶음으로 출처 증명이 다시 확인되는가. "준비 완료" 표시만 믿지 않는다.
func (t *tool) verify(dir, tag string) error {
	if _, ok := targets[t.arch]; !ok {
		return fmt.Errorf("the target %q is not supported (linux-amd64 or linux-arm64)", t.arch)
	}
	outArch := t.arch
	if tag != t.pinned {
		return fmt.Errorf("the tag %s is not the upstream version go.mod pins (%s)", tag, t.pinned)
	}
	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))) }
	// 태그와 대상 OS · 아키텍처를 파일 목록보다 먼저 견준다(다른 대상의 폴더면 파일 이름부터 다르다).
	tb, err := read("provenance/TAG")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(tb)) != tag {
		return fmt.Errorf("the folder was prepared for %s, not %s", strings.TrimSpace(string(tb)), tag)
	}
	if tg, err := read("provenance/TARGET"); err != nil || strings.TrimSpace(string(tg)) != t.target() {
		return fmt.Errorf("the folder was prepared for %s, not %s", strings.TrimSpace(string(tg)), t.target())
	}
	have, err := listFiles(dir)
	if err != nil {
		return err
	}
	if err := sameSet(have, t.expectedFiles()); err != nil {
		return err
	}
	mb, err := read("provenance/" + manifestName)
	if err != nil {
		return err
	}
	m, err := readManifest(mb, tag)
	if err != nil {
		return err
	}
	sb, err := read("provenance/" + sumsName)
	if err != nil {
		return err
	}
	sums, err := parseSums(sb)
	if err != nil {
		return err
	}
	if sums[manifestName] != sha(mb) {
		return fmt.Errorf("the kept release manifest does not match the kept SHA256SUMS")
	}
	if err := t.checkCollectors(filepath.Join(dir, "collectors"), m, tag); err != nil {
		return err
	}
	jarSum, _, _, _ := m.asset(jarAsset)
	if sums[jarAsset] != jarSum {
		return fmt.Errorf("the kept SHA256SUMS and release manifest disagree on %s", jarAsset)
	}
	subjects := map[string]string{"provenance/" + manifestName: bundleName(manifestName), "provenance/" + sumsName: bundleName(sumsName),
		"collectors/" + jarAsset: bundleName(jarAsset)}
	for _, b := range wantBins {
		subjects["collectors/"+outArch+"/"+b] = bundleName(outArch + "/" + b)
	}
	keys := make([]string, 0, len(subjects))
	for k := range subjects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, rel := range keys {
		if err := t.att.Verify(filepath.Join(dir, filepath.FromSlash(rel)), filepath.Join(dir, "provenance", "attestations", subjects[rel]), tag); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: collectors fetch -arch <linux-amd64|linux-arm64> -out <dir> [-tag vX.Y.Z] | verify -arch <...> -dir <dir> [-tag vX.Y.Z]")
		os.Exit(2)
	}
	t := newTool()
	rootDir, err := os.MkdirTemp("", "pqcaton-trust-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ collectors:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(rootDir)
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	tag := fs.String("tag", t.pinned, "upstream release tag (must be the version go.mod pins)")
	out := fs.String("out", "", "folder to create with the verified collectors (fetch)")
	dir := fs.String("dir", "", "prepared folder to check again (verify)")
	arch := fs.String("arch", "", "target of the collectors: linux-amd64 or linux-arm64 (the machine that runs the screen and its nodes)")
	ws := fs.String("ws", "", "screen workspace whose collectors/ to recover or put in place (recover, install)")
	from := fs.String("from", "", "the collectors to put in: a real folder outside the workspace with <folder>.sha256 next to it (install)")
	fs.Parse(os.Args[2:])
	t.arch = *arch
	if os.Args[1] == "recover" || os.Args[1] == "install" {
		os.RemoveAll(rootDir) // 출처 증명을 쓰지 않는다
		if *ws == "" {
			fmt.Fprintln(os.Stderr, "✗ collectors: "+os.Args[1]+" needs -ws")
			os.Exit(1)
		}
		did, err := runWS(os.Args[1], *ws, *from, osOps{})
		for _, d := range did {
			t.logf("   %s", d)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "✗ collectors:", err)
			os.Exit(1)
		}
		return
	}
	if os.Args[1] == "fetch" || os.Args[1] == "verify" {
		if err = checkGh(); err == nil {
			err = t.att.(*ghAttester).fetchRoot(rootDir)
		}
	}
	switch {
	case err != nil:
	case os.Args[1] == "fetch" && *out == "":
		err = errors.New("fetch needs -out")
	case os.Args[1] == "fetch":
		err = t.fetch(*out, *tag)
	case os.Args[1] == "verify" && *dir == "":
		err = errors.New("verify needs -dir")
	case os.Args[1] == "verify":
		if err = t.verify(*dir, *tag); err == nil {
			t.logf("✓ %s holds the verified %s collectors", *dir, *tag)
		}
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		os.RemoveAll(rootDir)
		fmt.Fprintln(os.Stderr, "✗ collectors:", err)
		os.Exit(1)
	}
}
