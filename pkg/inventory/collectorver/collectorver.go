// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// Package collectorver — 관측에 쓰는 상류 수집기가 이 판이 고정한 상류 판으로 빌드됐는지 본다.
//
// 왜 있는가: v0.10.7 전의 `pqcota-jvmscan`은 JVM이 둘 이상인 노드에서 결과를 여러 줄로 써서, 그 노드가 든 적재 요청
// 전체가 거절됐다. 화면은 수집기 파일이 있는지만 봐서, 옛 수집기를 둔 채 관측해도 아무도 몰랐다.
//
// **수집기를 실행하지 않는다.** Go 바이너리는 빌드 정보(debug/buildinfo)만, JAR 은 manifest 와 pom.properties 만 읽는다.
// 받은 바이너리를 실행하는 것은 그 자체로 위험이고, 실행해 판을 묻는 명령도 상류에 없다(pqcota-keygen 처럼 -h 를 모르는
// 도구도 있다).
//
// 세 상태(실물로 확인한 빌드 방식별 결과는 pqcaton-aai plans/2026-11-11-v0.27.0-collector-key-reclaim.md 3절):
//
//   - Verified: 수집기 자신이 고정 판이고 빌드 정보의 주 모듈 내용 해시(h1)가 이 리포가 고정한 값(Pinned, go.sum 에서
//     생성)과 같으며, 상류 의존에도 replace 가 없다. 상류 v0.14.0 의 릴리스 자산이 이렇다.
//   - Mismatch: 보이는 판(수집기 자신 · require) 가운데 고정 판이 아닌 정식 판이 있거나, **판은 같지만 h1 이 다르다**
//     (ReasonOtherContent). 옛 수집기 · 같은 판 이름의 다른 내용이 여기 걸린다. 우회할 수 없다.
//   - Unverified: 판을 믿을 수 없다. 수집기 자신이 replace(상류 데모 방식) · (devel) · dirty · 의사 판이거나, 빌드 정보가
//     없거나, **수집기 자신은 맞아도 상류 의존이 replace 다**(ReasonDepsReplaced). 태그 체크아웃에서 replace 없이 빌드해도
//     빌드 정보에 주 모듈의 h1 이 없으면(VCS 빌드) 견줄 수 없어 여기다(ReasonNoSum).
//
// 의존 replace 를 확인됨으로 세지 않는 까닭: 옆 pqcota-common 을 v0.10.6 으로 바꿔 빌드해도 빌드 정보에는
// `v0.10.7 => ../pqcota-common (devel)` 만 남아 실제 소스 판이 드러나지 않았다(실물 확인). 상류는 지금 의존을 늘 옆 폴더
// replace 로 빌드하므로, 지금의 상류 빌드는 모두 확인 불가다. 그것이 바이너리로 알 수 있는 것의 한계다.
package collectorver

import (
	"archive/zip"
	"bufio"
	"debug/buildinfo"
	"io"
	"path"
	"regexp"
	"strings"
)

// DiscoveryModule — 수집기를 내는 상류 모듈.
const DiscoveryModule = "github.com/randyinthedev-hash/pqcota-discovery"

// upstreamPrefix — 판을 견줄 상류 모듈의 앞머리.
const upstreamPrefix = "github.com/randyinthedev-hash/pqcota"

// State — 파일 하나의 판정.
type State string

const (
	Verified   State = "verified"
	Mismatch   State = "mismatch"
	Unverified State = "unverified"
)

// Reason — 왜 그 판정인가. 화면이 말로 옮긴다.
type Reason string

const (
	ReasonOK           Reason = ""
	ReasonNoBuildInfo  Reason = "no-build-info"  // Go 빌드 정보가 없다(다른 종류의 파일이거나 지운 빌드)
	ReasonOtherModule  Reason = "other-module"   // 상류 수집기 모듈이 아닌 것으로 빌드됐다
	ReasonReplaced     Reason = "local-replace"  // 수집기 자신이 로컬 replace 로 빌드됐다(상류 데모 방식)
	ReasonDevel        Reason = "devel"          // 판이 (devel) 이다(-buildvcs=false 나 .git 없는 소스)
	ReasonDirty        Reason = "dirty"          // 고친 트리에서 빌드했다
	ReasonPseudo       Reason = "pseudo-version" // 태그가 아닌 커밋에서 빌드했다
	ReasonOldVersion   Reason = "old-version"    // 고정 판이 아닌 정식 판이다
	ReasonNoVersion    Reason = "no-version"     // JAR 에 판 정보가 없다
	ReasonUnreadable   Reason = "unreadable"     // 파일을 읽지 못했다
	ReasonNoPin        Reason = "no-pin"         // 이 화면이 고정한 상류 판을 알 수 없다
	ReasonDepsReplaced Reason = "deps-replaced"  // 수집기 자신은 고정 판이지만 상류 의존이 replace 라 그 소스를 확인할 수 없다
	ReasonOtherContent Reason = "other-content"  // 판은 고정 판인데 주 모듈의 h1 이 고정한 값과 다르다(같은 판 이름의 다른 내용)
	ReasonNoSum        Reason = "no-sum"         // 빌드 정보에 주 모듈의 h1 이 없다(태그 체크아웃에서 직접 빌드)
	ReasonNoCommit     Reason = "no-commit"      // JAR 에 커밋이 없거나 꼴이 틀렸거나, 수정 여부가 true · false 가 아니다
)

// Pin — 이 리포가 고정한 상류 모듈 하나: go.mod 의 판과 go.sum 의 내용 해시(h1). 값은 pin_gen.go(tools/genpin 이 생성)에 있다.
type Pin struct{ Version, Sum string }

// SumFor — 수집기 모듈(pqcota-discovery)의 판이 want 일 때 견줄 h1. 고정한 판과 다르면 빈 값이다(그러면 확인됨이 될 수 없다).
func SumFor(want string) string {
	if p, ok := Pinned[DiscoveryModule]; ok && p.Version == want {
		return p.Sum
	}
	return ""
}

// File — 수집기 파일 하나를 읽은 결과.
type File struct {
	Path     string // 수집기 폴더 기준 상대 경로(화면에 보인다)
	State    State
	Reason   Reason
	Declared string // 읽을 수 있었던 판(수집기 자신, 없으면 빈 값). replace 면 require 판이다
	Revision string // VCS 커밋, JAR 이면 X-Pqcota-Commit(있으면)
	Sum      string // 빌드 정보의 주 모듈 h1(있으면)
	// DepsReplaced — 상류 의존이 로컬 replace 라 그 실제 판을 확인하지 않았다.
	DepsReplaced bool
	// Seen — 판정에 쓴 정식 판들(수집기 자신과 상류 의존의 require). 수집기끼리 섞였는지 볼 때 쓴다.
	Seen []string
}

var (
	semver = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	pseudo = regexp.MustCompile(`\d{14}-[0-9a-f]{12}$`)
	// describe — `git describe --tags` 가 태그 뒤 커밋에서 내는 꼴(v0.14.0-3-gabc1234). 상류는 직접 빌드한 JAR 의 판을 이렇게 싣는다.
	describe = regexp.MustCompile(`^v\d+\.\d+\.\d+-\d+-g[0-9a-f]+(-dirty)?$`)
	// commitHex — 받아들이는 커밋 꼴: git 의 SHA-1(40자) · SHA-256(64자) 16진수.
	commitHex = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

// CheckGo — Go 수집기 바이너리를 읽는다. want 는 이 화면이 고정한 상류 판, wantSum 은 그 판의 주 모듈 h1(SumFor)이다.
func CheckGo(file, rel, want, wantSum string) File {
	f := File{Path: rel}
	if want == "" {
		f.State, f.Reason = Unverified, ReasonNoPin
		return f
	}
	bi, err := buildinfo.ReadFile(file)
	if err != nil {
		f.State, f.Reason = Unverified, ReasonNoBuildInfo
		return f
	}
	modified := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			f.Revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	main := bi.Main
	f.Sum = main.Sum
	f.Declared = strings.TrimSuffix(main.Version, "+dirty")
	if f.Declared == "(devel)" {
		f.Declared = ""
	}
	if semver.MatchString(f.Declared) {
		f.Seen = append(f.Seen, f.Declared)
	}
	for _, d := range bi.Deps {
		if !strings.HasPrefix(d.Path, upstreamPrefix) {
			continue
		}
		// 로컬 폴더 replace 는 판이 (devel) 로 남는다. 다른 모듈로 바꾼 replace 도 require 판과 내용이 다르므로 함께 센다.
		if d.Replace != nil {
			f.DepsReplaced = true
		}
		if semver.MatchString(d.Version) {
			f.Seen = append(f.Seen, d.Version)
		}
	}
	for _, v := range f.Seen {
		if v != want {
			f.State, f.Reason = Mismatch, ReasonOldVersion
			return f
		}
	}
	switch {
	case main.Path != DiscoveryModule:
		f.State, f.Reason = Unverified, ReasonOtherModule
	case main.Replace != nil:
		f.State, f.Reason = Unverified, ReasonReplaced
	case f.Declared == "":
		f.State, f.Reason = Unverified, ReasonDevel
	case f.Declared == want && main.Sum != "" && wantSum != "" && main.Sum != wantSum:
		f.State, f.Reason = Mismatch, ReasonOtherContent
	case strings.HasSuffix(main.Version, "+dirty") || modified:
		f.State, f.Reason = Unverified, ReasonDirty
	case pseudo.MatchString(f.Declared):
		f.State, f.Reason = Unverified, ReasonPseudo
	case f.Declared != want:
		f.State, f.Reason = Mismatch, ReasonOldVersion
	case f.DepsReplaced:
		f.State, f.Reason = Unverified, ReasonDepsReplaced
	case wantSum == "":
		f.State, f.Reason = Unverified, ReasonNoPin
	case main.Sum == "":
		f.State, f.Reason = Unverified, ReasonNoSum
	default:
		f.State = Verified
	}
	return f
}

// maxShown — 화면에 옮기는 판 문자열의 최대 길이. manifest 는 임의 문자열이라 길이를 자르고 제어 문자를 뺀다.
const maxShown = 64

// clip — manifest 에서 읽은 값을 화면에 옮길 수 있게 다듬는다. 제어 문자는 빼고, maxShown 자보다 길면 잘라 「…」를 붙인다.
func clip(v string) string {
	v = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, v)
	if r := []rune(v); len(r) > maxShown {
		return string(r[:maxShown]) + "…"
	}
	return v
}

// CheckJar — collector.jar 를 읽는다. manifest 의 Implementation-Version, 없으면 pom.properties 의 version 을 본다.
// 상류 v0.14.0 부터 manifest 에 판(Implementation-Version) · 커밋(X-Pqcota-Commit) · 수정 여부(X-Pqcota-Dirty)가 실린다.
// **확인됨은 셋이 모두 맞을 때뿐이다**: 정식 판이 고정 판이고, 커밋이 40 · 64자 16진수이며, 수정 여부가 false 다. 정식 판이
// 다르면 Mismatch 다(우회할 수 없다). 그 밖은 모두 Unverified 이고 부르는 쪽은 수집기의 확인 불가처럼 다룬다(override 필요):
// 판 정보 없음(ReasonNoVersion), devel · 태그 뒤 커밋(ReasonDevel · ReasonPseudo), dirty(ReasonDirty), 커밋 · 수정 여부가
// 없거나 꼴이 틀림(ReasonNoCommit). JAR 이 없으면 이 함수를 부르지 않는다(선택 항목이다).
//
// **Mismatch 는 정식 판(vX.Y.Z)이 다를 때만이다.** 상류가 git 없이 직접 빌드한 JAR 은 판이 `devel` 이고, git 에서 빌드하면
// `git describe` 꼴(v0.14.0-3-gabc · v0.14.0-dirty)이다. 이것은 다른 판이 아니라 판을 믿을 수 없는 빌드라 Unverified 다
// (devel · 태그 뒤 커밋 · dirty). 커밋은 40자 · 64자 16진수일 때만 받는다. 화면에 옮기는 판은 clip 으로 다듬는다.
func CheckJar(file, rel, want string) File {
	f := File{Path: rel}
	r, err := zip.OpenReader(file)
	if err != nil {
		f.State, f.Reason = Unverified, ReasonUnreadable
		return f
	}
	defer r.Close()
	var manifest, pom, dirty string
	for _, zf := range r.File {
		switch {
		case strings.EqualFold(zf.Name, "META-INF/MANIFEST.MF"):
			manifest = readProp(zf, "Implementation-Version", ':')
			if c := readProp(zf, "X-Pqcota-Commit", ':'); commitHex.MatchString(c) {
				f.Revision = c
			}
			dirty = readProp(zf, "X-Pqcota-Dirty", ':')
		case strings.HasPrefix(zf.Name, "META-INF/maven/") && path.Base(zf.Name) == "pom.properties" && pom == "":
			pom = readProp(zf, "version", '=')
		}
	}
	v := manifest
	if v == "" {
		v = pom
	}
	if v == "" {
		f.State, f.Reason = Unverified, ReasonNoVersion
		return f
	}
	if !strings.HasPrefix(v, "v") && v != "" && v[0] >= '0' && v[0] <= '9' {
		v = "v" + v
	}
	f.Declared = clip(v)
	if semver.MatchString(v) {
		f.Seen = []string{v}
	}
	switch {
	case want == "":
		f.State, f.Reason = Unverified, ReasonNoPin
	case semver.MatchString(v) && v != want:
		f.State, f.Reason = Mismatch, ReasonOldVersion
	case strings.HasSuffix(v, "-dirty") && !describe.MatchString(v):
		f.State, f.Reason = Unverified, ReasonDirty
	case describe.MatchString(v):
		f.State, f.Reason = Unverified, ReasonPseudo
	case !semver.MatchString(v):
		f.State, f.Reason = Unverified, ReasonDevel
	case strings.EqualFold(dirty, "true"):
		f.State, f.Reason = Unverified, ReasonDirty
	case f.Revision == "" || !strings.EqualFold(dirty, "false"):
		f.State, f.Reason = Unverified, ReasonNoCommit
	default:
		f.State = Verified
	}
	return f
}

// readProp 는 key<sep>value 꼴 줄에서 key 의 값을 읽는다. 커지지 않게 앞 64KiB 만 본다.
func readProp(zf *zip.File, key string, sep byte) string {
	rc, err := zf.Open()
	if err != nil {
		return ""
	}
	defer rc.Close()
	sc := bufio.NewScanner(io.LimitReader(rc, 64<<10))
	for sc.Scan() {
		line := sc.Text()
		i := strings.IndexByte(line, sep)
		if i > 0 && strings.TrimSpace(line[:i]) == key {
			return strings.TrimSpace(line[i+1:])
		}
	}
	return ""
}

// Mixed — 수집기들이 서로 다른 정식 판을 보이는가. 그렇다면 하나하나가 맞아 보여도 막는다.
func Mixed(files []File) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		for _, v := range f.Seen {
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}
