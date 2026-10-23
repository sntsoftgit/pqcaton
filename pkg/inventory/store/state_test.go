// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func quiet(string) {}

func open(t *testing.T, dir string) *State {
	t.Helper()
	s, err := Open(dir, Options{Warn: quiet, UID: -1})
	if err != nil {
		t.Fatalf("Open(%s): %v", dir, err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mode(t *testing.T, p string) os.FileMode {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// IC-W1: 처음 열면 상태 디렉토리(0700)와 상태 ID(0600)를 만들고, 다시 열면 같은 ID를 읽는다.
func TestOpenCreatesState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), DirName)
	s := open(t, dir)
	if !idPattern.MatchString(s.ID) {
		t.Fatalf("state id %q is not 32 hex digits", s.ID)
	}
	if m := mode(t, dir); m != 0o700 {
		t.Errorf("state directory mode = %#o, want 0700", m)
	}
	if m := mode(t, filepath.Join(dir, stateFile)); m != 0o600 {
		t.Errorf("state.json mode = %#o, want 0600", m)
	}
	again := open(t, dir)
	if again.ID != s.ID {
		t.Errorf("reopened id = %s, want %s", again.ID, s.ID)
	}
	other := open(t, filepath.Join(t.TempDir(), DirName))
	if other.ID == s.ID {
		t.Error("two state directories got the same id")
	}
}

// IC-W2: 상태 디렉토리가 0700보다, 안의 파일이 0600보다 넓으면 열지 않는다. 고쳐 주지도 않는다.
func TestOpenRefusesWidePermissions(t *testing.T) {
	cases := map[string]func(dir string) string{
		"directory": func(dir string) string { return dir },
		"state.json": func(dir string) string {
			return filepath.Join(dir, stateFile)
		},
		"another file": func(dir string) string {
			p := filepath.Join(dir, "receipt.json")
			if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
			return p
		},
	}
	for name, pick := range cases {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), DirName)
			open(t, dir)
			p := pick(dir)
			wide := os.FileMode(0o640)
			if p == dir {
				wide = 0o750
			}
			if err := os.Chmod(p, wide); err != nil {
				t.Fatal(err)
			}
			_, err := Open(dir, Options{Warn: quiet, UID: -1})
			if !errors.Is(err, ErrUnsafeFile) || !strings.Contains(err.Error(), "chmod") {
				t.Fatalf("Open with %s at %#o: err = %v, want ErrUnsafeFile with a chmod hint", name, wide, err)
			}
			if m := mode(t, p); m != wide {
				t.Errorf("the mode was changed to %#o; Open must not fix it quietly", m)
			}
		})
	}
}

// IC-W3: 심볼릭 링크인 상태 디렉토리, 하드 링크가 둘인 상태 파일, 상태 디렉토리 안의 링크를 거부한다.
func TestOpenRefusesLinks(t *testing.T) {
	t.Run("symlinked directory", func(t *testing.T) {
		root := t.TempDir()
		real := filepath.Join(root, "real")
		open(t, real)
		link := filepath.Join(root, DirName)
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(link, Options{Warn: quiet, UID: -1}); !errors.Is(err, ErrUnsafeFile) {
			t.Fatalf("err = %v, want ErrUnsafeFile", err)
		}
	})
	t.Run("hard-linked state.json", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, DirName)
		open(t, dir)
		if err := os.Link(filepath.Join(dir, stateFile), filepath.Join(root, "copy.json")); err != nil {
			t.Fatal(err)
		}
		_, err := Open(dir, Options{Warn: quiet, UID: -1})
		if !errors.Is(err, ErrUnsafeFile) || !strings.Contains(err.Error(), "hard links") {
			t.Fatalf("err = %v, want ErrUnsafeFile about hard links", err)
		}
	})
	t.Run("symlink inside", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), DirName)
		open(t, dir)
		if err := os.Symlink("/etc/hostname", filepath.Join(dir, "receipt.json")); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(dir, Options{Warn: quiet, UID: -1}); !errors.Is(err, ErrUnsafeFile) {
			t.Fatalf("err = %v, want ErrUnsafeFile", err)
		}
	})
}

// IC-W4: 남의 소유인 상태 디렉토리는 열지 않는다. rename으로 교체하면 소유자가 바뀌기 때문이다.
// 다른 사용자를 만들 수 없으므로 「이 프로세스의 사용자」를 바꿔 잰다.
func TestOpenRefusesOtherOwner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), DirName)
	open(t, dir)
	_, err := Open(dir, Options{Warn: quiet, UID: os.Getuid() + 1})
	if !errors.Is(err, ErrUnsafeFile) || !strings.Contains(err.Error(), "owned by another user") {
		t.Fatalf("err = %v, want ErrUnsafeFile about the owner", err)
	}
}

// IC-W5: 실행 잠금은 작성자 하나만 쥔다. 둘째는 **기다리지 않고** 거부되고, 첫째가 놓으면 잡힌다.
func TestRunLockIsExclusive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), DirName)
	server := open(t, dir)
	if err := server.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	cli := open(t, dir)
	start := time.Now()
	err := cli.AcquireRun()
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("second writer: err = %v, want ErrBusy", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("the second writer waited %v; it must be refused at once", d)
	}
	if m := mode(t, filepath.Join(dir, runLockFile)); m != 0o600 {
		t.Errorf("run.lock mode = %#o, want 0600", m)
	}
	server.Close()
	if err := cli.AcquireRun(); err != nil {
		t.Fatalf("after the first writer let go: %v", err)
	}
}

// IC-W6: 대상 잠금 파일에는 상태 ID가 적힌다. 다른 상태 디렉토리는 거부되고, 함께 옮긴 데이터는
// 그대로 열린다(경로가 아니라 ID로 본다).
func TestTargetLockBindsStateID(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	s := open(t, filepath.Join(data, DirName))
	if err := s.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	release, err := s.LockTargets([]string{data}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	release()
	lock := filepath.Join(data, TargetLockName)
	b, err := os.ReadFile(lock)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), s.ID) {
		t.Errorf("lock file %q does not name state %s", b, s.ID)
	}
	if m := mode(t, lock); m != 0o600 {
		t.Errorf("lock file mode = %#o, want 0600", m)
	}

	if err := os.Mkdir(filepath.Join(root, "elsewhere"), 0o700); err != nil {
		t.Fatal(err)
	}
	other := open(t, filepath.Join(root, "elsewhere", DirName))
	if err := other.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	if _, err := other.LockTargets([]string{data}, time.Second); !errors.Is(err, ErrStateMismatch) || !strings.Contains(err.Error(), "-state-dir") {
		t.Fatalf("another state directory: err = %v, want ErrStateMismatch with a -state-dir hint", err)
	}
	s.Close()

	moved := filepath.Join(root, "moved")
	if err := os.Rename(data, moved); err != nil {
		t.Fatal(err)
	}
	m := open(t, filepath.Join(moved, DirName))
	if m.ID != s.ID {
		t.Fatalf("moved state id = %s, want %s", m.ID, s.ID)
	}
	if err := m.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	release, err = m.LockTargets([]string{moved}, time.Second)
	if err != nil {
		t.Fatalf("moved data with its state directory: %v", err)
	}
	release()
}

// IC-W7: 대상 잠금은 정해진 시간까지 기다린다. 그 안에 풀리면 잡고, 못 잡으면 ErrTargetBusy로
// 끝나며 그동안 잡은 다른 잠금도 놓는다.
func TestTargetLockWaits(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, d := range []string{a, b} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	s := open(t, filepath.Join(root, DirName))
	if err := s.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LockTargets(nil, 0); err != nil {
		t.Fatal(err)
	}
	holdB, err := s.LockTargets([]string{b}, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = s.LockTargets([]string{a, b}, 150*time.Millisecond)
	if !errors.Is(err, ErrTargetBusy) {
		t.Fatalf("err = %v, want ErrTargetBusy", err)
	}
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Errorf("gave up after %v; it should wait the full time", d)
	}
	onlyA, err := s.LockTargets([]string{a}, 0)
	if err != nil {
		t.Fatalf("a should have been released after the failed attempt: %v", err)
	}
	onlyA()

	go func() {
		time.Sleep(100 * time.Millisecond)
		holdB()
	}()
	both, err := s.LockTargets([]string{b, a}, 2*time.Second)
	if err != nil {
		t.Fatalf("the lock was released within the wait: %v", err)
	}
	both()
}

// IC-W8: 네트워크 파일시스템은 이름표로 가려낸다. 로컬은 지나간다.
func TestRemoteFileSystems(t *testing.T) {
	linux := map[uint32]string{0x6969: "NFS", 0xFF534D42: "CIFS", 0xFE534D42: "SMB2", 0x517B: "SMB", 0xEF53: "", 0x58465342: "", 0x01021994: ""}
	for magic, want := range linux {
		if got := remoteLinuxFS(magic); got != want {
			t.Errorf("remoteLinuxFS(%#x) = %q, want %q", magic, got, want)
		}
	}
	darwin := map[string]string{"nfs": "NFS", "smbfs": "SMB", "afpfs": "AFP", "webdav": "WebDAV", "apfs": "", "hfs": ""}
	for name, want := range darwin {
		if got := remoteDarwinFS(name); got != want {
			t.Errorf("remoteDarwinFS(%q) = %q, want %q", name, got, want)
		}
	}
}
