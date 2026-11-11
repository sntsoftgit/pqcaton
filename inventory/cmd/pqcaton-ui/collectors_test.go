// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// 관측 실행 시험(observerun_test.go 의 runReady)에 기대므로 그것과 같이 유닉스에서만 돈다.

//go:build unix

package main

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/collectorver"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// 판정 자체는 collectorver 가 실제로 빌드한 바이너리로 잰다. 여기서는 판정 함수를 갈아 끼워, 상태마다 화면 · 시작 ·
// 표준 오류가 어떻게 움직이는지 잰다.

const pin = "v0.10.7"

// judged: 수집기마다 같은 판정을 돌려주는 판정 함수.
func judged(state collectorver.State, reason collectorver.Reason, declared string) func(string, string, string) collectorver.File {
	return func(_, rel, _ string) collectorver.File {
		f := collectorver.File{Path: rel, State: state, Reason: reason, Declared: declared}
		if declared != "" {
			f.Seen = []string{declared}
		}
		if state == collectorver.Verified {
			f.Revision = "aa5423e1e03aa80b6c20de2357a89c1e0ff09475"
		}
		return f
	}
}

func versionReady(t *testing.T, check func(string, string, string) collectorver.File, allow bool) (*server, *bytes.Buffer) {
	t.Helper()
	s := runReady(t, "ok")
	s.collectorPin, s.checkCollector, s.allowUnverified = pin, check, allow
	var stderr bytes.Buffer
	s.stderr = &stderr
	return s, &stderr
}

func startRefused(t *testing.T, s *server) {
	t.Helper()
	w := postForm(t, s, ui.RunPath, url.Values{})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedRunNotReady) {
		t.Fatalf("start = %d, want 409 %s", w.Code, ui.RefusedRunNotReady)
	}
	if s.run.status() != nil {
		t.Fatal("something ran")
	}
}

// IC-U165 — **모두 확인되면 통과하고, 03 화면이 파일마다 판과 출처를 보인다.** 확인됨은 의존에도 replace 가 없는
// 빌드뿐이다(상류가 그런 릴리스 빌드를 낼 때).
func TestCollectorVersionsVerified(t *testing.T) {
	s, _ := versionReady(t, judged(collectorver.Verified, collectorver.ReasonOK, pin), false)
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	for _, want := range []string{`data-check="collector-version"`, `data-collector-state="verified"`,
		"선언된 판: v0.10.7 · 실제 소스: 태그 커밋 aa5423e1e03a · 확인됨",
		`action="` + ui.RunPath + `"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the observe screen lacks %q", want)
		}
	}
}

// IC-U166 — **판이 맞지 않으면 시작하지 않고, -allow-unverified-collectors 로도 넘기지 못한다.** 옛 수집기가 적재를
// 통째로 막던 사고가 여기 걸린다. 수집기끼리 판이 섞인 것도 같다.
func TestCollectorVersionMismatchCannotBeAllowed(t *testing.T) {
	for name, check := range map[string]func(string, string, string) collectorver.File{
		"old version": judged(collectorver.Mismatch, collectorver.ReasonOldVersion, "v0.10.6"),
		"mixed": func(p, rel, w string) collectorver.File {
			v := pin
			if strings.HasSuffix(rel, "jvmscan") {
				v = "v0.10.6"
			}
			return collectorver.File{Path: rel, State: collectorver.Unverified, Reason: collectorver.ReasonNoPin, Declared: v, Seen: []string{v}}
		},
	} {
		s, stderr := versionReady(t, check, true)
		page := get(t, s, ui.ScreenObserve+"?lang=en").Body.String()
		if !strings.Contains(page, `data-check="collector-mismatch"`) || !strings.Contains(page, "cannot be used even with -allow-unverified-collectors") {
			t.Errorf("%s: the observe screen does not refuse the mismatch", name)
		}
		startRefused(t, s)
		if strings.Contains(stderr.String(), "allow-unverified") {
			t.Errorf("%s: a mismatch was logged as allowed: %s", name, stderr.String())
		}
	}
}

// IC-U167 — **판을 확인할 수 없으면 기본으로 시작하지 않는다.** 상류 데모처럼 replace 로 빌드한 수집기는 선언된 판이
// 맞아도 「실제 소스: 로컬 replace(devel) · 확인 불가」로, 태그 체크아웃이어도 의존이 replace 면 「수집기 소스 v0.10.7 확인 ·
// replace 의존 소스 확인 불가」로 보인다. -allow-unverified-collectors 로 시작한 화면만 허용하고, 그 사실을 화면과 표준
// 오류에 남긴다. 작업 폴더에는 남기지 않는다.
func TestUnverifiedCollectorsNeedTheFlag(t *testing.T) {
	for reason, line := range map[collectorver.Reason]string{
		collectorver.ReasonReplaced:     "선언된 판: v0.10.7 · 실제 소스: 로컬 replace(devel) · 확인 불가",
		collectorver.ReasonDepsReplaced: "수집기 소스 v0.10.7 확인 · replace 의존 소스 확인 불가",
	} {
		s, _ := versionReady(t, judged(collectorver.Unverified, reason, pin), false)
		page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
		for _, want := range []string{`data-check="collector-unverified"`, line, "-allow-unverified-collectors로 시작하십시오"} {
			if !strings.Contains(page, want) {
				t.Errorf("%s without the flag: the observe screen lacks %q", reason, want)
			}
		}
		startRefused(t, s)
	}
	check := judged(collectorver.Unverified, collectorver.ReasonReplaced, pin)

	s, stderr := versionReady(t, check, true)
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	if !strings.Contains(page, `data-check="collector-allowed"`) || !strings.Contains(page, "작업 폴더에 남지 않습니다") {
		t.Errorf("with the flag, the observe screen does not show the allowance:\n%s", page)
	}
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("start with the flag = %d\n%s", w.Code, w.Body.String())
	}
	waitRun(t, s)
	log := stderr.String()
	for _, want := range []string{"-allow-unverified-collectors", "linux-amd64/pqcota-jvmscan (local-replace)", "pinned " + pin} {
		if !strings.Contains(log, want) {
			t.Errorf("stderr lacks %q:\n%s", want, log)
		}
	}
	root := filepath.Dir(s.results)
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, _ := os.ReadFile(p); bytes.Contains(b, []byte("allow-unverified")) {
			t.Errorf("the allowance is kept in the workspace: %s", p)
		}
		return nil
	})
}

func writeJar(t *testing.T, path, manifest string) {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, _ := w.Create("META-INF/MANIFEST.MF")
	fw.Write([]byte(manifest))
	w.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// IC-U168 — **collector.jar 는 판이 없으면 경고만 하고, 판이 있는데 다르면 막는다.** 지금 상류의 JAR 은 판을 싣지 않아
// 경고로 보이고 시작은 된다. override 도 요구하지 않는다.
func TestCollectorJarVersion(t *testing.T) {
	s, _ := versionReady(t, judged(collectorver.Verified, collectorver.ReasonOK, pin), false)
	writeJar(t, filepath.Join(s.collectors, "collector.jar"), "Manifest-Version: 1.0\nAgent-Class: pqcota.jvm.IntrospectAgent\n")
	page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	if !strings.Contains(page, `data-check="jar-version"`) || !strings.Contains(page, "시작은 막지 않습니다") || !strings.Contains(page, `action="`+ui.RunPath+`"`) {
		t.Errorf("a jar without a version should warn and still allow the start:\n%s", page)
	}

	writeJar(t, filepath.Join(s.collectors, "collector.jar"), "Manifest-Version: 1.0\nImplementation-Version: 0.10.6\n")
	page = get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
	if !strings.Contains(page, `data-check="jar-mismatch"`) {
		t.Errorf("a jar of another version should be refused:\n%s", page)
	}
	startRefused(t, s)
}

// IC-U169 — **-allow-unverified-collectors 는 기본으로 꺼져 있고, 주면 서버에 그대로 닿는다.**
func TestAllowUnverifiedFlag(t *testing.T) {
	for _, c := range []struct {
		args []string
		want bool
	}{{nil, false}, {[]string{"-allow-unverified-collectors"}, true}} {
		cfg, err := parseStart(append([]string{"-workspace", t.TempDir()}, c.args...), &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.server().allowUnverified; got != c.want {
			t.Errorf("args %v: allowUnverified = %v, want %v", c.args, got, c.want)
		}
	}
}

// IC-U170 — **화면이 pqcota-discovery 를 링크하지 않아도 고정 판을 안다.** 빌드 정보에는 실제로 링크한 모듈만 실려서,
// discovery 만 찾던 때는 데모에서 모든 수집기가 「고정한 판을 알 수 없음」이 됐다. discovery 가 있으면 그 판, 없으면
// pqcota-common 의 판이다. 둘 다 없으면 빈 값이다.
func TestCollectorPinFromLinkedModules(t *testing.T) {
	common := &debug.Module{Path: "github.com/randyinthedev-hash/pqcota-common", Version: "v0.10.7"}
	other := &debug.Module{Path: "github.com/go-chi/chi/v5", Version: "v5.4.0"}
	for _, c := range []struct {
		deps []*debug.Module
		want string
	}{
		{[]*debug.Module{other, common}, "v0.10.7"},
		{[]*debug.Module{common, {Path: collectorver.DiscoveryModule, Version: "v0.10.8"}}, "v0.10.8"},
		{[]*debug.Module{other}, ""},
	} {
		if got := pinFrom(c.deps); got != c.want {
			t.Errorf("pinFrom(%v) = %q, want %q", c.deps, got, c.want)
		}
	}
}
