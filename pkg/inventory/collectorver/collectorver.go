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
//   - Verified: 수집기 자신이 고정 판(태그 · 커밋 · 수정 없음)이고, 상류 의존에도 replace 가 없다.
//   - Mismatch: 보이는 판(수집기 자신 · require) 가운데 고정 판이 아닌 정식 판이 있다. 옛 수집기가 여기 걸린다. 우회할 수 없다.
//   - Unverified: 판을 믿을 수 없다. 수집기 자신이 replace(상류 데모 방식) · (devel) · dirty · 의사 판이거나, 빌드 정보가
//     없거나, **수집기 자신은 맞아도 상류 의존이 replace 다**(ReasonDepsReplaced).
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
)

// File — 수집기 파일 하나를 읽은 결과.
type File struct {
	Path     string // 수집기 폴더 기준 상대 경로(화면에 보인다)
	State    State
	Reason   Reason
	Declared string // 읽을 수 있었던 판(수집기 자신, 없으면 빈 값). replace 면 require 판이다
	Revision string // VCS 커밋(있으면)
	// DepsReplaced — 상류 의존이 로컬 replace 라 그 실제 판을 확인하지 않았다.
	DepsReplaced bool
	// Seen — 판정에 쓴 정식 판들(수집기 자신과 상류 의존의 require). 수집기끼리 섞였는지 볼 때 쓴다.
	Seen []string
}

var (
	semver = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	pseudo = regexp.MustCompile(`\d{14}-[0-9a-f]{12}$`)
)

// CheckGo — Go 수집기 바이너리를 읽는다. want 는 이 화면이 고정한 상류 판이다.
func CheckGo(file, rel, want string) File {
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
	case strings.HasSuffix(main.Version, "+dirty") || modified:
		f.State, f.Reason = Unverified, ReasonDirty
	case pseudo.MatchString(f.Declared):
		f.State, f.Reason = Unverified, ReasonPseudo
	case f.Declared != want:
		f.State, f.Reason = Mismatch, ReasonOldVersion
	case f.DepsReplaced:
		f.State, f.Reason = Unverified, ReasonDepsReplaced
	default:
		f.State = Verified
	}
	return f
}

// CheckJar — collector.jar 를 읽는다. manifest 의 Implementation-Version, 없으면 pom.properties 의 version 을 본다.
// 판 정보가 없으면 Unverified(ReasonNoVersion)이고, 부르는 쪽은 그것을 경고로만 다룬다(JAR 은 선택 항목이다).
func CheckJar(file, rel, want string) File {
	f := File{Path: rel}
	r, err := zip.OpenReader(file)
	if err != nil {
		f.State, f.Reason = Unverified, ReasonUnreadable
		return f
	}
	defer r.Close()
	var manifest, pom string
	for _, zf := range r.File {
		switch {
		case strings.EqualFold(zf.Name, "META-INF/MANIFEST.MF"):
			manifest = readProp(zf, "Implementation-Version", ':')
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
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	f.Declared = v
	f.Seen = []string{v}
	switch {
	case want == "":
		f.State, f.Reason = Unverified, ReasonNoPin
	case v != want:
		f.State, f.Reason = Mismatch, ReasonOldVersion
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
