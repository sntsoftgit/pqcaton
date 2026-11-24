// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// fakeDot — `dot`이라는 이름의 스크립트를 PATH 맨 앞에 둔다.
func fakeDot(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake dot is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dot"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func forgetPicture() {
	dotCache.Lock()
	dotCache.filled = false
	dotCache.Unlock()
}

// IC-U229 — **그림이 오래 걸리거나 상한을 넘어도 표에서 노드를 고르고 DOT을 받을 수 있다.**
//
// 이력 화면은 `dot`을 돌리지 않고 뜬다. 그림은 따로 받는 조각이고, 그리는 데 시간과 크기 상한이 있다. 상한을 넘거나 검사에서
// 걸러지면 까닭을 적고, 표 · 노드 고르기 · DOT 내려받기와 04의 DOT 원문은 그대로 된다. 같은 그림은 다시 그리지 않는다.
func TestPictureNeverHoldsTheTable(t *testing.T) {
	s, _ := activationReady(t)
	useEndpoints(t, inventory.NewMemMetaStore(), nil)
	s.upstream, s.upstreamOrgName = wholeMem{ingestedUnder(t, s, s.scopeAssets, nil)}, s.orgName()

	oldRun, oldTimeout, oldMax := runDot, dotTimeout, dotMaxSVG
	t.Cleanup(func() { runDot, dotTimeout, dotMaxSVG = oldRun, oldTimeout, oldMax; forgetPicture() })
	calls := 0
	answer := func(ctx context.Context, _ string) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() } // 끝나지 않는 dot
	runDot = func(ctx context.Context, dot string) ([]byte, error) { calls++; return answer(ctx, dot) }
	dotTimeout = 50 * time.Millisecond
	forgetPicture()

	node := ""
	works := func(what string) {
		t.Helper()
		page := get(t, s, ui.ScreenHistTopo+"?lang=ko&node="+node)
		if body := page.Body.String(); page.Code != http.StatusOK || !strings.Contains(body, "data-hist-counts") ||
			strings.Count(body, "data-hist-summary=") != 1 || strings.Contains(body, "<svg") {
			t.Errorf("%s: the table with the node %q chosen: %d", what, node, page.Code)
		}
		if d := get(t, s, ui.HistTopoDotPath); d.Code != http.StatusOK || !strings.HasPrefix(d.Body.String(), "digraph pqcota_topology {") {
			t.Errorf("%s: the DOT download: %d", what, d.Code)
		}
	}
	picture := func() string { return get(t, s, ui.HistTopoPicturePath+"?lang=ko").Body.String() }

	// 고를 노드 하나를 이력에서 읽는다.
	top, err := s.readHistTopology()
	if err != nil || len(top.Nodes) == 0 {
		t.Fatalf("the history has %d nodes: %v", len(top.Nodes), err)
	}
	node = top.Nodes[0].ID

	// 화면과 내려받기는 dot을 부르지 않는다.
	works("before any picture")
	if calls != 0 {
		t.Fatalf("the screen or the download ran dot %d times", calls)
	}
	// 끝나지 않는 dot: 상한에서 끊고 까닭을 적는다. 같은 그림을 다시 청해도 다시 돌리지 않는다.
	if p := picture(); !strings.Contains(p, `data-picture="timeout"`) {
		t.Errorf("a dot that never ends: %s", p)
	}
	picture()
	if calls != 1 {
		t.Errorf("the same picture was drawn %d times, want once", calls)
	}
	works("after a timeout")

	// 상한보다 큰 그림.
	forgetPicture()
	answer = func(context.Context, string) ([]byte, error) { return nil, errDotTooLarge }
	if p := picture(); !strings.Contains(p, `data-picture="too-large"`) {
		t.Errorf("a drawing over the size limit: %s", p)
	}
	works("after a drawing over the limit")

	// 검사에서 걸러진 그림: 이력 화면은 까닭을 적고, 04는 까닭과 DOT 원문을 보인다.
	forgetPicture()
	answer = func(context.Context, string) ([]byte, error) {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path fill="url(https://example.invalid/#a)" d="M0,0"/></svg>`), nil
	}
	if p := picture(); !strings.Contains(p, `data-picture="refused"`) || strings.Contains(p, "<svg") {
		t.Errorf("a refused drawing: %s", p)
	}
	works("after a refused drawing")
	if body := get(t, s, ui.ScreenSurvey+"?lang=ko").Body.String(); !strings.Contains(body, `data-picture="refused"`) ||
		!strings.Contains(body, "digraph crypto_topology") || strings.Contains(body, "example.invalid") {
		t.Error("04 with a refused drawing does not show the reason and the DOT source")
	}

	// 그려진 그림은 조각으로 온다. 관측에서 온 글자는 이스케이프된 채다.
	forgetPicture()
	answer = func(context.Context, string) ([]byte, error) { return []byte(graphvizSVG), nil }
	if p := picture(); !strings.Contains(p, "data-hist-picture") || !strings.Contains(p, "&lt;script&gt;alert(1)&lt;/script&gt;") || strings.Contains(p, "<script") {
		t.Errorf("a drawn picture: %s", p)
	}

	// 이력을 읽지 못하면 조각은 그 사실을 적는다(끼워 넣는 쪽이 오류 응답을 싣지 않으므로 200이다).
	useEndpoints(t, nil, errors.New("pqcota history: unreachable: timed out"))
	if w := get(t, s, ui.HistTopoPicturePath+"?lang=ko"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `data-picture="unreadable"`) {
		t.Errorf("the picture of an unreadable history: %d %s", w.Code, w.Body.String())
	}
}

// IC-U229 — 실제 실행에서의 상한. 끝없이 쓰는 `dot`은 크기 상한에서, 멈춰 있는 `dot`은 시간 상한에서 끝낸다.
func TestDotRunIsBounded(t *testing.T) {
	oldTimeout, oldMax := dotTimeout, dotMaxSVG
	t.Cleanup(func() { dotTimeout, dotMaxSVG = oldTimeout, oldMax; forgetPicture() })

	fakeDot(t, "cat >/dev/null; while :; do echo 0123456789012345678901234567890123456789; done")
	dotMaxSVG, dotTimeout = 64<<10, 20*time.Second
	forgetPicture()
	start := time.Now()
	if svg, state := renderDOT("digraph a {}"); state != svgTooLarge || svg != "" {
		t.Errorf("an endless dot: state %q", state)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("stopping an endless dot took %s", time.Since(start))
	}

	fakeDot(t, "exec sleep 30")
	dotMaxSVG, dotTimeout = oldMax, 200*time.Millisecond
	forgetPicture()
	start = time.Now()
	if svg, state := renderDOT("digraph b {}"); state != svgTimeout || svg != "" {
		t.Errorf("a dot that hangs: state %q", state)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("stopping a hanging dot took %s", time.Since(start))
	}

	// dot이 없으면 그 사실만 알리고 담아 두지 않는다.
	t.Setenv("PATH", t.TempDir())
	forgetPicture()
	if _, state := renderDOT("digraph c {}"); state != svgNoDot {
		t.Errorf("without dot: state %q", state)
	}
	dotCache.Lock()
	filled := dotCache.filled
	dotCache.Unlock()
	if filled {
		t.Error("a missing dot was remembered: installing it would not show until the DOT changes")
	}
}
