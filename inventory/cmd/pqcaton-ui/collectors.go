// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 관측에 쓰는 수집기의 판(collectorver). 시작 전 점검(preflight)에 들어간다.
//
// **세 상태이고, 넘길 수 있는 것은 하나뿐이다.** 확인됨은 통과한다. 맞지 않음(고정 판이 아닌 정식 판이 보이거나 수집기끼리
// 판이 섞임)은 어떤 옵션으로도 넘기지 못한다. 옛 pqcota-jvmscan 이 적재 요청을 통째로 막던 사고가 여기 걸린다. 확인 불가
// (상류 데모 방식의 replace 빌드 · devel · dirty · 빌드 정보 없음)는 기본으로 막고, 시작 옵션
// -allow-unverified-collectors 로만 넘긴다. 넘겼다는 사실과 그 파일 목록은 화면과 표준 오류에만 남기고 작업 폴더에는
// 쓰지 않는다(운영 기록이 아니라 그 화면 한 번의 선택이다).
//
// collector.jar 는 상류가 판을 싣지 않아 지금은 판 차단 대상이 아니다. 판이 없으면 경고만 보이고, 판이 있는데 다르면 막는다.
// 설계: pqcaton-aai plans/2026-11-11-v0.27.0-collector-key-reclaim.md

import (
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/collectorver"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// collectorPin: 이 화면이 고정한 상류 수집기 판(go.mod 가 정한다). 시험 바이너리처럼 빌드 정보에 의존 모듈이 없으면 빈
// 값이고, 그러면 모든 수집기가 확인 불가다.
func collectorPin() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return pinFrom(bi.Deps)
}

// pinFrom: 링크한 pqcota-discovery 의 판, 없으면 pqcota-common 의 판.
//
// **화면은 pqcota-discovery 의 패키지를 링크하지 않는다.** 빌드 정보에는 실제로 링크한 모듈만 실리므로, discovery 만 찾으면
// go.mod 가 그 판을 요구해도 빈 값이 된다. 데모 종단 검증에서 모든 수집기가 「고정한 판을 알 수 없음」으로 나와 드러났다.
// 상류 다섯 리포는 같은 태그로 나오고, go.mod 의 상류 모듈 판이 모두 같다는 것은 판 시험(IC-U144)이 잰다. 그래서
// 언제나 링크하는 pqcota-common 의 판이 곧 수집기가 맞춰야 할 판이다.
func pinFrom(deps []*debug.Module) string {
	var common string
	for _, d := range deps {
		switch d.Path {
		case collectorver.DiscoveryModule:
			return d.Version
		case "github.com/randyinthedev-hash/pqcota-common":
			common = d.Version
		}
	}
	return common
}

// collectorBins: 아키텍처 폴더마다 실어 보내는 수집기. preflight 가 있는지 보는 것과 같은 목록이다.
var collectorBins = map[string][]string{
	"linux-amd64":   {"pqcota-nodescan", "pqcota-netcap", "pqcota-jvmscan"},
	"linux-arm64":   {"pqcota-nodescan", "pqcota-netcap", "pqcota-jvmscan"},
	"windows-amd64": {"pqcota-cngscan.exe", "pqcota-jvmscan.exe"},
}

// collectorFiles: dirs(아키텍처 폴더 이름)의 수집기 파일을 판정한다. 판정 함수는 시험이 바꾼다.
func (s *server) collectorFiles(dirs []string) []collectorver.File {
	check := s.checkCollector
	if check == nil {
		check = collectorver.CheckGo
	}
	var out []collectorver.File
	for _, d := range dirs {
		for _, n := range collectorBins[d] {
			rel := d + "/" + n
			out = append(out, check(filepath.Join(s.collectors, d, n), rel, s.collectorPin))
		}
	}
	return out
}

func toUI(fs []collectorver.File) []ui.CollectorFile {
	out := make([]ui.CollectorFile, 0, len(fs))
	for _, f := range fs {
		out = append(out, ui.CollectorFile{Path: f.Path, State: string(f.State), Reason: string(f.Reason),
			Declared: f.Declared, Revision: f.Revision, DepsReplaced: f.DepsReplaced})
	}
	return out
}

// versionChecks: 수집기 판 점검. 맞지 않음 → 막음(넘길 수 없음), 확인 불가 → 막음(옵션으로 허용하면 경고), 모두 확인됨 → 통과.
func (s *server) versionChecks(files []collectorver.File) []ui.RunCheck {
	if len(files) == 0 {
		return nil
	}
	pin := s.collectorPin
	if pin == "" {
		pin = "?"
	}
	var mismatch, unverified bool
	for _, f := range files {
		switch f.State {
		case collectorver.Mismatch:
			mismatch = true
		case collectorver.Unverified:
			unverified = true
		}
	}
	if mixed := collectorver.Mixed(files); mixed != nil {
		mismatch = true
	}
	rows := toUI(files)
	switch {
	case mismatch:
		return []ui.RunCheck{{Item: ui.RunCollectorMismatch, OK: false, Blocking: true, Detail: pin, Files: rows}}
	case unverified && s.allowUnverified:
		return []ui.RunCheck{{Item: ui.RunCollectorAllowed, OK: false, Blocking: false, Detail: pin, Files: rows}}
	case unverified:
		return []ui.RunCheck{{Item: ui.RunCollectorUnverified, OK: false, Blocking: true, Detail: pin, Files: rows}}
	}
	return []ui.RunCheck{{Item: ui.RunCollectorVersion, OK: true, Blocking: true, Detail: pin, Files: rows}}
}

// jarCheck: collector.jar 가 있을 때의 판 점검. 판 정보가 없으면 경고만, 다른 판이면 막는다. 확인되면 따로 적지 않는다.
func (s *server) jarCheck(path string) *ui.RunCheck {
	f := collectorver.CheckJar(path, "collector.jar", s.collectorPin)
	rows := toUI([]collectorver.File{f})
	switch {
	case f.State == collectorver.Mismatch:
		return &ui.RunCheck{Item: ui.RunJarMismatch, OK: false, Blocking: true, Detail: s.collectorPin, Files: rows}
	case f.State == collectorver.Unverified:
		return &ui.RunCheck{Item: ui.RunJarVersion, OK: false, Blocking: false, Detail: path, Files: rows}
	}
	return nil
}

// logAllowed: 확인 불가를 허용한 수집기 목록을 표준 오류에 적는다. 작업 폴더에는 쓰지 않는다.
func logAllowed(w io.Writer, when string, checks []ui.RunCheck) {
	if w == nil {
		return
	}
	for _, c := range checks {
		if c.Item != ui.RunCollectorAllowed {
			continue
		}
		var names []string
		for _, f := range c.Files {
			if f.State == string(collectorver.Unverified) {
				names = append(names, f.Path+" ("+f.Reason+")")
			}
		}
		fmt.Fprintf(w, "warning: %s: -allow-unverified-collectors lets these collectors run without a verified version (pinned %s): %s\n",
			when, c.Detail, strings.Join(names, ", "))
	}
}
