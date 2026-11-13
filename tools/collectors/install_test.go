// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

// putDir — dir 에 files 를 쓰고, sumsFile 이 비지 않으면 그 해시 목록을 쓴다.
func putDir(t *testing.T, dir string, files map[string]string, sumsFile string) {
	t.Helper()
	var lines []string
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, sha([]byte(body))+"  ./"+name)
	}
	sort.Strings(lines)
	if sumsFile != "" {
		if err := os.WriteFile(sumsFile, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// snapshotWS — 작업 폴더 전체의 경로 → 내용.
func snapshotWS(t *testing.T, ws string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(ws, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(ws, p)
			b, _ := os.ReadFile(p)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return out
}

func collectorsOf(t *testing.T, ws string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for k, v := range snapshotWS(t, ws) {
		if strings.HasPrefix(k, "collectors/") {
			out[strings.TrimPrefix(k, "collectors/")] = v
		}
	}
	return out
}

func same(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

var (
	oldSet = map[string]string{"linux-amd64/pqcota-nodescan": "old nodescan", "collector.jar": "old jar"}
	newSet = map[string]string{"linux-amd64/pqcota-nodescan": "new nodescan", "linux-amd64/pqcota-netcap": "new netcap", "collector.jar": "new jar"}
)

// incOf — 시험이 쓰는 복사본 자리(트랜잭션 t1).
func incOf(ws string) string { return filepath.Join(ws, incomingPrefix+"-t1") }

// installed — 이미 oldSet 이 들어간 작업 폴더와, 들일 newSet 복사본(incOf).
func installed(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	p := places(ws)
	putDir(t, p.cur, oldSet, p.curSum)
	putDir(t, incOf(ws), newSet, incOf(ws)+".sha256")
	return ws
}

// crashOps — k 번째 파일 연산에서 프로세스가 죽은 것처럼 멈춘다(panic). 그 앞의 연산은 실제로 한다.
type crashOps struct{ n, k int }

type crashed struct{}

func (c *crashOps) step() {
	c.n++
	if c.n == c.k {
		panic(crashed{})
	}
}
func (c *crashOps) Rename(a, b string) error { c.step(); return os.Rename(a, b) }
func (c *crashOps) RemoveAll(p string) error { c.step(); return os.RemoveAll(p) }

// failOps — from(또는 지울 자리)이 match 를 담은 연산 하나만 실패한다.
type failOps struct{ match string }

func (f failOps) Rename(a, b string) error {
	if strings.HasSuffix(a, f.match) {
		return errors.New("injected rename failure")
	}
	return os.Rename(a, b)
}
func (f failOps) RemoveAll(p string) error { return os.RemoveAll(p) }

// IC-CF8 — **작업 폴더에 들이기는 반쯤 바뀌지 않는다.** 정상이면 새 수집기가 들어가고 보관본 · 복사본이 남지 않는다.
// 복사본이 해시 목록과 다르면 버리고 기존 것은 바이트까지 그대로다. 들이는 rename 이 실패하면 기존 것을 되돌린다.
// install 의 파일 연산 하나하나에서 끊긴 뒤(k 번째 연산에서 멈춤) recover 하면, collectors 는 언제나 기존 것이나 새 것
// 가운데 **하나와 정확히 같고**, 확정 시점(incoming → collectors rename) 뒤에 끊겼으면 새 것이다. 특히 새 rename 이
// 성공한 뒤 보관본을 지우기 전에 끊긴 경우는 새 것을 그대로 두고 보관본만 지운다.
func TestInstallIsAtomic(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		ws := installed(t)
		if _, err := installWS(ws, incOf(ws), osOps{}); err != nil {
			t.Fatal(err)
		}
		if !same(collectorsOf(t, ws), newSet) {
			t.Fatalf("collectors = %v", collectorsOf(t, ws))
		}
		p := places(ws)
		for _, x := range []string{incOf(ws), incOf(ws) + ".sha256", p.old, p.oldSum} {
			if exists(x) {
				t.Errorf("%s is left", filepath.Base(x))
			}
		}
		if err := matches(p.cur, p.curSum); err != nil {
			t.Errorf("the installed collectors do not match their hash list: %v", err)
		}
	})
	t.Run("the copy differs", func(t *testing.T) {
		ws := installed(t)
		os.WriteFile(filepath.Join(incOf(ws), "collector.jar"), []byte("new jaR"), 0o644)
		before := collectorsOf(t, ws)
		if _, err := installWS(ws, incOf(ws), osOps{}); err == nil || !strings.Contains(err.Error(), "collectors are unchanged") {
			t.Fatalf("a changed copy was put in: %v", err)
		}
		if !same(collectorsOf(t, ws), before) || exists(incOf(ws)) {
			t.Error("the collectors changed, or the copy was left")
		}
	})
	t.Run("the rename fails", func(t *testing.T) {
		ws := installed(t)
		before := snapshotWS(t, ws)
		if _, err := installWS(ws, incOf(ws), failOps{match: incomingPrefix + "-t1"}); err == nil || !strings.Contains(err.Error(), "were restored") {
			t.Fatalf("want the earlier collectors restored, got %v", err)
		}
		after := snapshotWS(t, ws)
		for k, v := range before {
			if strings.HasPrefix(k, "collectors/") || k == ".collectors.sha256" {
				if after[k] != v {
					t.Errorf("%s changed", k)
				}
			}
		}
		if exists(places(ws).old) || exists(incOf(ws)) {
			t.Error("a temporary folder is left")
		}
	})
	t.Run("interrupted at every step", func(t *testing.T) {
		// 정상 install 의 파일 연산 수를 센다.
		count := &crashOps{k: -1}
		if _, err := func() (any, error) { w := installed(t); return installWS(w, incOf(w), count) }(); err != nil {
			t.Fatal(err)
		}
		commit := 0 // incoming → collectors rename 의 차례
		for k := 1; k <= count.n; k++ {
			ws := installed(t)
			p := places(ws)
			ops := &crashOps{k: k}
			func() {
				defer func() {
					if r := recover(); r != nil {
						if _, ok := r.(crashed); !ok {
							panic(r)
						}
					}
				}()
				installWS(ws, incOf(ws), ops)
			}()
			afterCrash := exists(p.cur) && same(collectorsOf(t, ws), newSet)
			if afterCrash && commit == 0 {
				commit = k
			}
			did, err := recoverWS(ws, osOps{})
			if err != nil {
				t.Fatalf("interrupted at operation %d: recover failed: %v", k, err)
			}
			got := collectorsOf(t, ws)
			switch {
			case same(got, newSet):
			case same(got, oldSet):
				if commit != 0 && k >= commit {
					t.Errorf("interrupted at operation %d (after the commit at %d): recover went back to the earlier collectors (%q)", k, commit, did)
				}
			default:
				t.Errorf("interrupted at operation %d: the collectors are mixed: %v", k, got)
			}
			for _, x := range []string{incOf(ws), incOf(ws) + ".sha256", p.old, p.oldSum} {
				if exists(x) {
					t.Errorf("interrupted at operation %d: %s is left after recover", k, filepath.Base(x))
				}
			}
			if err := matches(p.cur, p.curSum); err != nil {
				t.Errorf("interrupted at operation %d: the collectors do not match their hash list after recover: %v", k, err)
			}
			t.Logf("interrupted at operation %d of %d: %q", k, count.n, did)
		}
		if commit == 0 {
			t.Fatal("no interruption point after the commit was seen")
		}
	})
	t.Run("renamed, then interrupted before removing the earlier copy", func(t *testing.T) {
		ws := installed(t)
		p := places(ws)
		// 새 rename 이 성공한 뒤 보관본을 지우는 연산에서 멈춘다.
		if _, err := installWS(ws, incOf(ws), failOnRemove{target: p.old}); err == nil {
			t.Fatal("want the removal of the earlier copy to fail")
		}
		if !same(collectorsOf(t, ws), newSet) || !exists(p.old) {
			t.Fatalf("want the new collectors in place and the earlier copy still there")
		}
		did, err := recoverWS(ws, osOps{})
		if err != nil || !same(collectorsOf(t, ws), newSet) || exists(p.old) || exists(p.oldSum) ||
			!strings.Contains(strings.Join(did, " "), "kept the current collectors") {
			t.Fatalf("recover should keep the new collectors and remove the earlier copy: %v %q", err, did)
		}
	})
}

// failOnRemove — target 을 지우는 연산만 실패한다.
type failOnRemove struct{ target string }

func (f failOnRemove) Rename(a, b string) error { return os.Rename(a, b) }
func (f failOnRemove) RemoveAll(p string) error {
	if p == f.target {
		return fmt.Errorf("interrupted")
	}
	return os.RemoveAll(p)
}

// IC-CF9 — **recover 는 끊긴 자리를 상태에 따라 가른다.** 복사본만 있으면 버린다(기존 것은 그대로). 보관본만 있고 지금 것이
// 없으면 보관본을 되돌린다. 둘 다 있고 지금 것이 해시 목록과 다르면 지금 것을 격리하고(지우지 않는다) 보관본을 되돌린다.
// 어느 쪽도 해시 목록과 같지 않으면 아무것도 지우거나 덮어쓰지 않고 실패한다.
func TestRecoverDecidesByState(t *testing.T) {
	t.Run("only a copy", func(t *testing.T) {
		ws := installed(t)
		if _, err := recoverWS(ws, osOps{}); err != nil {
			t.Fatal(err)
		}
		if !same(collectorsOf(t, ws), oldSet) || exists(incOf(ws)) {
			t.Error("the copy should be discarded and the collectors kept")
		}
	})
	t.Run("an earlier copy and no collectors", func(t *testing.T) {
		ws := t.TempDir()
		p := places(ws)
		putDir(t, p.old, oldSet, p.oldSum)
		if _, err := recoverWS(ws, osOps{}); err != nil || !same(collectorsOf(t, ws), oldSet) {
			t.Fatalf("the earlier copy should be restored: %v", err)
		}
	})
	t.Run("current does not match", func(t *testing.T) {
		ws := t.TempDir()
		p := places(ws)
		putDir(t, p.old, oldSet, p.oldSum)
		putDir(t, p.cur, newSet, p.curSum)
		os.WriteFile(filepath.Join(p.cur, "collector.jar"), []byte("broken"), 0o644)
		did, err := recoverWS(ws, osOps{})
		if err != nil || !same(collectorsOf(t, ws), oldSet) {
			t.Fatalf("the earlier copy should be restored: %v %q", err, did)
		}
		var rejected []string
		for k := range snapshotWS(t, ws) {
			if strings.HasPrefix(k, ".collectors.rejected-") {
				rejected = append(rejected, k)
			}
		}
		if len(rejected) == 0 {
			t.Error("the current collectors were not set aside")
		}
	})
	t.Run("neither can be confirmed", func(t *testing.T) {
		ws := t.TempDir()
		p := places(ws)
		putDir(t, p.old, oldSet, p.oldSum)
		putDir(t, p.cur, newSet, p.curSum)
		os.WriteFile(filepath.Join(p.cur, "collector.jar"), []byte("broken"), 0o644)
		os.WriteFile(filepath.Join(p.old, "collector.jar"), []byte("broken too"), 0o644)
		before := snapshotWS(t, ws)
		if _, err := recoverWS(ws, osOps{}); err == nil || !strings.Contains(err.Error(), "nothing was removed or replaced") {
			t.Fatalf("want a clear failure, got %v", err)
		}
		if !same(snapshotWS(t, ws), before) {
			t.Error("something was removed or replaced")
		}
	})
}

// source — 작업 폴더 밖의 들일 수집기(폴더와 옆의 해시 목록).
func source(t *testing.T, files map[string]string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "collectors")
	putDir(t, src, files, src+".sha256")
	return src
}

// IC-CF10 — **install -from <source> 는 한 번에 끝난다.** 잠금 안에서 옛 중단 상태(지난 실행이 남긴 복사본 · 새 rename 뒤
// 보관본을 지우기 전에 끊긴 자리)를 정리한 뒤, source 를 작업 폴더 안의 고유 복사본으로 복사해 다시 검증하고 들인다. 작업
// 폴더 안의 복사본은 잠금을 쥔 실행만 만들므로, 잠금을 얻었을 때 남은 것은 중단 흔적이라 버린다. source 는 그대로 두고, 무엇을
// 버리고 무엇을 채택했는지 로그에 남긴다. 복사본이 해시 목록과 다르면(source 의 파일이 목록과 다름) 아무것도 바꾸지 않는다.
func TestInstallCopiesUnderTheLock(t *testing.T) {
	ws := installed(t)
	p := places(ws)
	putDir(t, p.old, oldSet, p.oldSum) // 지난 install 이 새 rename 뒤에 끊겼다(지금 것 = oldSet 이 해시 목록과 같다)
	src := source(t, newSet)
	did, err := runWS("install", ws, src, osOps{})
	if err != nil || !same(collectorsOf(t, ws), newSet) {
		t.Fatalf("install should tidy and then put the source in: %v %q", err, did)
	}
	for k := range snapshotWS(t, ws) {
		if strings.HasPrefix(k, ".collectors.incoming") || strings.HasPrefix(k, ".collectors.old") {
			t.Errorf("%s is left", k)
		}
	}
	joined := strings.Join(did, " | ")
	if !strings.Contains(joined, "discarded a copy an earlier run left") || !strings.Contains(joined, "kept the current collectors") ||
		!strings.Contains(joined, "adopted the new verified collectors in place of the earlier ones") {
		t.Errorf("the log should say what was discarded, kept and adopted: %q", did)
	}
	if !exists(src) || !exists(src+".sha256") {
		t.Error("install removed the source")
	}
	bad := source(t, newSet)
	os.WriteFile(filepath.Join(bad, "collector.jar"), []byte("changed"), 0o644)
	before := collectorsOf(t, ws)
	// 잠금 전의 검사(내용 스냅샷)에서 먼저 막혀, 작업 폴더에 아무 흔적도 남기지 않는다.
	if _, err := runWS("install", ws, bad, osOps{}); err == nil || !strings.Contains(err.Error(), "does not match the hash list") ||
		!strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("a source that does not match its hash list should be refused before the lock, got %v", err)
	}
	if !same(collectorsOf(t, ws), before) {
		t.Error("the collectors changed")
	}
	for k := range snapshotWS(t, ws) {
		if strings.HasPrefix(k, ".collectors.incoming") {
			t.Errorf("%s is left after a refused install", k)
		}
	}
}

// IC-CF11 — **두 install 이 함께 시작해도 하나만 반영된다.** 작업 폴더 실행 잠금은 기다리지 않으므로, 잠금을 얻은 쪽만 들이고
// 다른 쪽은 아무것도 바꾸지 않고 실패한다. 끝난 collectors 는 이긴 쪽의 source 와 정확히 같다(섞이지 않는다).
func TestConcurrentInstallsApplyOnlyOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the workspace run lock is not supported on windows")
	}
	ws := t.TempDir()
	p := places(ws)
	putDir(t, p.cur, oldSet, p.curSum)
	a := source(t, newSet)
	b := source(t, map[string]string{"linux-amd64/pqcota-nodescan": "b nodescan", "collector.jar": "b jar"})
	held := make(chan struct{})
	slow := slowOps{held: held, gate: make(chan struct{})}
	type res struct {
		did []string
		err error
	}
	ra, rb := make(chan res, 1), make(chan res, 1)
	go func() { d, e := runWS("install", ws, a, slow); ra <- res{d, e} }()
	<-held
	go func() { d, e := runWS("install", ws, b, osOps{}); rb <- res{d, e} }()
	second := <-rb
	close(slow.release())
	first := <-ra
	if first.err != nil {
		t.Fatalf("the install that holds the lock failed: %v", first.err)
	}
	if second.err == nil || !strings.Contains(second.err.Error(), "the workspace is in use") {
		t.Fatalf("the second install should fail safely while the first holds the lock, got %v %q", second.err, second.did)
	}
	if !same(collectorsOf(t, ws), newSet) {
		t.Errorf("collectors = %v, want exactly the winning source", collectorsOf(t, ws))
	}
	if err := matches(p.cur, p.curSum); err != nil {
		t.Error(err)
	}
}

// slowOps — 첫 rename 앞에서 한 번 멈춰(held 를 닫고 gate 가 열리기를 기다린다) 잠금을 쥔 채 머문다. 시험마다 새로 만든다.
type slowOps struct{ held, gate chan struct{} }

func (s slowOps) release() chan struct{} { return s.gate }
func (s slowOps) Rename(a, b string) error {
	select {
	case <-s.held:
	default:
		close(s.held)
		<-s.gate
	}
	return os.Rename(a, b)
}
func (s slowOps) RemoveAll(p string) error { return os.RemoveAll(p) }

// IC-CF12 — **화면이 떠 있는 동안 collectors 를 바꾸지 않는다.** 화면이 작업 폴더 실행 잠금을 쥐고 있으면(store.AcquireRun)
// install 도 recover 도 「화면을 먼저 멈추라」고 거절하고, 작업 폴더는 바이트까지 그대로다. 화면이 내려간 뒤에는 들인다.
func TestNoInstallWhileTheScreenRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the workspace run lock is not supported on windows")
	}
	ws := installed(t)
	src := source(t, newSet)
	screen, err := store.Open(filepath.Join(ws, store.DirName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := screen.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	before := snapshotWS(t, ws)
	for _, cmd := range []string{"install", "recover"} {
		if _, err := runWS(cmd, ws, src, osOps{}); err == nil || !strings.Contains(err.Error(), "stop the screen first") {
			t.Errorf("%s while the screen runs = %v, want a refusal", cmd, err)
		}
	}
	if !same(snapshotWS(t, ws), before) {
		t.Error("the workspace changed while the screen was running")
	}
	screen.Close()
	if _, err := runWS("install", ws, src, osOps{}); err != nil || !same(collectorsOf(t, ws), newSet) {
		t.Fatalf("after the screen stopped the install should go through: %v", err)
	}
}

// IC-CF13 — **source 는 실제 경로로 따진다(잠금 전).** 작업 폴더 안(복사본 자리 · 보관본 · collectors 자신 포함) · 심볼릭
// 링크인 source · 폴더가 아닌 source · 해시 목록이 없거나 심볼릭 링크 · 작업 폴더로 가는 경로가 심볼릭 링크인 경우를 모두
// 잠금을 잡기 전에 막고, 작업 폴더에 아무 흔적도 남기지 않는다.
func TestInstallChecksTheRealPathOfTheSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links and the run lock are not exercised on windows")
	}
	ws := installed(t)
	p := places(ws)
	putDir(t, p.old, oldSet, p.oldSum)
	good := source(t, newSet)
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(good, link)
	os.Symlink(good+".sha256", link+".sha256")
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, []byte("x"), 0o644)
	os.WriteFile(file+".sha256", []byte("x"), 0o644)
	noSums := filepath.Join(t.TempDir(), "nosums")
	putDir(t, noSums, newSet, "")
	sumLink := filepath.Join(t.TempDir(), "sumlink")
	putDir(t, sumLink, newSet, "")
	os.Symlink(good+".sha256", sumLink+".sha256")
	inside := filepath.Join(ws, "staging")
	putDir(t, inside, newSet, inside+".sha256")
	wsLink := filepath.Join(t.TempDir(), "ws-link")
	os.Symlink(ws, wsLink)
	before := snapshotWS(t, ws)
	for name, c := range map[string]struct{ ws, from string }{
		"inside the workspace":       {ws, inside},
		"the copy place":             {ws, incOf(ws)},
		"the earlier copy":           {ws, p.old},
		"the collectors themselves":  {ws, p.cur},
		"a symbolic link":            {ws, link},
		"not a folder":               {ws, file},
		"no hash list":               {ws, noSums},
		"a symbolic link hash list":  {ws, sumLink},
		"a workspace through a link": {wsLink, good},
	} {
		if _, err := runWS("install", c.ws, c.from, osOps{}); err == nil || !strings.Contains(err.Error(), "nothing was changed") {
			t.Errorf("%s: install = %v, want a refusal", name, err)
		}
	}
	if !same(snapshotWS(t, ws), before) {
		t.Error("a refused install changed the workspace")
	}
}

// IC-CF14 — **잠금 안에서는 같은 실행만 작업 폴더의 복사본을 다룬다.** install 이 source 를 작업 폴더 안으로 복사하는 도중에
// recover 를 부르면 잠금을 얻지 못해 거절되고, 복사 중인 복사본도 source 도 건드리지 않는다. 그 install 은 끝까지 들인다.
// 1차 검사와 잠금 사이에 source 가 다른 폴더로 바뀌거나(device/inode), 같은 inode 의 파일과 해시 목록이 함께 제자리에서 고쳐지면
// 잠금 안의 재검사(내용 스냅샷)가 막는다. 복사 도중에 source 파일이 바뀌면 복사본이 처음 스냅샷과 달라 막는다. 어느 경우든
// 작업 폴더의 collectors 는 그대로이고 복사본은 남지 않는다.
func TestNothingSlipsBetweenTheCheckAndTheLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the workspace run lock is not supported on windows")
	}
	t.Run("recover during the copy", func(t *testing.T) {
		ws := installed(t)
		src := source(t, newSet)
		orig := stageCopy
		copying, resume := make(chan string), make(chan struct{})
		stageCopy = func(w, s string, sums []byte) (string, error) {
			inc, err := orig(w, s, sums)
			copying <- inc
			<-resume
			return inc, err
		}
		t.Cleanup(func() { stageCopy = orig })
		done := make(chan error, 1)
		go func() { _, err := runWS("install", ws, src, osOps{}); done <- err }()
		inc := <-copying
		snapIn, srcBefore := snapshotWS(t, ws), snapshotWS(t, filepath.Dir(src))
		if _, err := runWS("recover", ws, "", osOps{}); err == nil || !strings.Contains(err.Error(), "the workspace is in use") {
			t.Errorf("recover during the copy = %v, want it refused by the lock", err)
		}
		if !same(snapshotWS(t, ws), snapIn) || !exists(inc) || !same(snapshotWS(t, filepath.Dir(src)), srcBefore) {
			t.Error("recover touched the copy in progress or the source")
		}
		close(resume)
		if err := <-done; err != nil || !same(collectorsOf(t, ws), newSet) {
			t.Fatalf("the install should finish: %v", err)
		}
	})
	t.Run("the same files rewritten in place before the lock", func(t *testing.T) {
		ws := t.TempDir()
		putDir(t, places(ws).cur, oldSet, places(ws).curSum)
		src := source(t, newSet)
		orig := lockWS
		lockWS = func(w string) (func(), error) {
			// 1차 검사 뒤, 잠금 전: 같은 inode 의 파일과 해시 목록을 함께 고친다(서로 맞게).
			jar := filepath.Join(src, "collector.jar")
			f, _ := os.OpenFile(jar, os.O_WRONLY|os.O_TRUNC, 0)
			f.Write([]byte("rewritten jar"))
			f.Close()
			body, _ := os.ReadFile(src + ".sha256")
			nb := strings.Replace(string(body), sha([]byte("new jar")), sha([]byte("rewritten jar")), 1)
			g, _ := os.OpenFile(src+".sha256", os.O_WRONLY|os.O_TRUNC, 0)
			g.Write([]byte(nb))
			g.Close()
			return orig(w)
		}
		t.Cleanup(func() { lockWS = orig })
		before := snapshotWS(t, ws)
		if _, err := runWS("install", ws, src, osOps{}); err == nil || !strings.Contains(err.Error(), "changed between the check and the lock") {
			t.Fatalf("files and hash list rewritten in place should be refused, got %v", err)
		}
		after := snapshotWS(t, ws)
		for k, v := range before {
			if !strings.HasPrefix(k, ".pqcaton/") && after[k] != v {
				t.Errorf("%s changed", k)
			}
		}
		for k := range after {
			if strings.HasPrefix(k, ".collectors.incoming") {
				t.Errorf("%s is left", k)
			}
		}
	})
	t.Run("a source file changes during the copy", func(t *testing.T) {
		ws := t.TempDir()
		putDir(t, places(ws).cur, oldSet, places(ws).curSum)
		src := source(t, newSet)
		orig := stageCopy
		stageCopy = func(w, s string, sums []byte) (string, error) {
			// 잠금 안의 재검사 뒤, 복사하는 동안 source 파일이 바뀐다.
			os.WriteFile(filepath.Join(s, "linux-amd64", "pqcota-netcap"), []byte("changed during the copy"), 0o755)
			return orig(w, s, sums)
		}
		t.Cleanup(func() { stageCopy = orig })
		before := collectorsOf(t, ws)
		if _, err := runWS("install", ws, src, osOps{}); err == nil || !strings.Contains(err.Error(), "not what was checked before the lock") {
			t.Fatalf("a source changed during the copy should be refused, got %v", err)
		}
		if !same(collectorsOf(t, ws), before) {
			t.Error("the collectors changed")
		}
		for k := range snapshotWS(t, ws) {
			if strings.HasPrefix(k, ".collectors.incoming") {
				t.Errorf("%s is left", k)
			}
		}
	})
	t.Run("the source changes before the lock", func(t *testing.T) {
		ws := t.TempDir() // 복사본 자리 없이(그래야 이 실행이 남긴 것만 센다)
		putDir(t, places(ws).cur, oldSet, places(ws).curSum)
		src := source(t, newSet)
		orig := lockWS
		lockWS = func(w string) (func(), error) {
			// 1차 검사 뒤, 잠금 전: 같은 경로에 다른 폴더와 그에 맞는 해시 목록을 둔다(내용만 보면 그 자체로는 온전하다).
			os.Rename(src, src+".gone")
			os.Rename(src+".sha256", src+".gone.sha256")
			putDir(t, src, map[string]string{"collector.jar": "other"}, src+".sha256")
			return orig(w)
		}
		t.Cleanup(func() { lockWS = orig })
		before := collectorsOf(t, ws)
		if _, err := runWS("install", ws, src, osOps{}); err == nil || !strings.Contains(err.Error(), "changed between the check and the lock") {
			t.Fatalf("a source swapped before the lock should be refused, got %v", err)
		}
		if !same(collectorsOf(t, ws), before) {
			t.Error("the collectors changed")
		}
		for k := range snapshotWS(t, ws) {
			if strings.HasPrefix(k, ".collectors.incoming") {
				t.Errorf("%s is left", k)
			}
		}
	})
}
