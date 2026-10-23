// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const txA = "0123456789abcdef0123456789abcdef"

// replace: Inspect → Prepare → Install. 처리기가 할 순서 그대로다.
func replace(t *testing.T, s *State, target string, content []byte) error {
	t.Helper()
	tg, err := s.Inspect(target)
	if err != nil {
		return err
	}
	prep := PreparedPath(tg.Path, txA)
	if err := s.Prepare(prep, content, tg.Mode); err != nil {
		return err
	}
	return s.Install(prep, tg.Path)
}

// IC-W9: 교체는 준비 파일을 거쳐 rename으로 끝난다. 있던 파일의 권한은 이어받고, 새 파일은 0600이며,
// 끝나면 준비 파일이 남지 않는다. 남이 읽을 수 있는 파일은 경고만 한다.
func TestReplacePreservesMode(t *testing.T) {
	dir := t.TempDir()
	var warned []string
	s, err := Open(filepath.Join(dir, DirName), Options{Warn: func(m string) { warned = append(warned, m) }, UID: -1})
	if err != nil {
		t.Fatal(err)
	}

	fresh := filepath.Join(dir, "plan.json")
	if err := replace(t, s, fresh, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if m := mode(t, fresh); m != 0o600 {
		t.Errorf("new file mode = %#o, want 0600", m)
	}

	shared := filepath.Join(dir, "session.json")
	if err := os.WriteFile(shared, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o640); err != nil {
		t.Fatal(err)
	}
	warned = nil
	if err := replace(t, s, shared, []byte("newer")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(shared); string(b) != "newer" {
		t.Errorf("content = %q, want newer", b)
	}
	if m := mode(t, shared); m != 0o640 {
		t.Errorf("mode = %#o, want the old 0640 kept", m)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "read by other users") {
		t.Errorf("warnings = %q, want one about readers", warned)
	}
	left, err := FindPrepared(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("prepared files left behind: %v", left)
	}
}

// IC-W10: 심볼릭 링크 · 하드 링크 둘 · 남의 소유 · group/other 쓰기 권한인 대상은 쓰지 않는다.
// 대상은 한 바이트도 바뀌지 않는다.
func TestInspectRefusesUnsafeTargets(t *testing.T) {
	cases := map[string]func(t *testing.T, dir, p string) Options{
		"symlink": func(t *testing.T, dir, p string) Options {
			real := filepath.Join(dir, "real.json")
			os.WriteFile(real, []byte("old"), 0o600)
			os.Remove(p)
			if err := os.Symlink(real, p); err != nil {
				t.Fatal(err)
			}
			return Options{UID: -1}
		},
		"hard link": func(t *testing.T, dir, p string) Options {
			if err := os.Link(p, filepath.Join(dir, "other-name.json")); err != nil {
				t.Fatal(err)
			}
			return Options{UID: -1}
		},
		"other owner": func(t *testing.T, dir, p string) Options {
			return Options{UID: os.Getuid() + 1}
		},
		"group writable": func(t *testing.T, dir, p string) Options {
			os.Chmod(p, 0o660)
			return Options{UID: -1}
		},
		"other writable": func(t *testing.T, dir, p string) Options {
			os.Chmod(p, 0o602)
			return Options{UID: -1}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "plan.json")
			if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			// 상태 디렉토리는 먼저 이 사용자로 연다. 소유자 검사는 대상에서 걸려야 한다.
			sd := filepath.Join(dir, DirName)
			if _, err := Open(sd, Options{Warn: quiet, UID: -1}); err != nil {
				t.Fatal(err)
			}
			opt := setup(t, dir, p)
			s := &State{Dir: sd, uid: opt.UID, warn: quiet}
			if s.uid < 0 {
				s.uid = os.Getuid()
			}
			err := replace(t, s, p, []byte("new"))
			if !errors.Is(err, ErrUnsafeFile) {
				t.Fatalf("err = %v, want ErrUnsafeFile", err)
			}
			if b, _ := os.ReadFile(p); string(b) != "old" {
				t.Errorf("target became %q", b)
			}
		})
	}
}

// IC-W11: 쓰다 멈추면(준비 파일 쓰기 · fsync · rename · 디렉토리 fsync 앞) 대상은 옛 내용이거나
// 새 내용이다. 반쯤 쓴 내용은 보이지 않는다. 멈춘 자리에 남은 준비 파일은 지우지 않고 남긴다.
func TestReplaceStopsCleanly(t *testing.T) {
	points := map[string]string{
		FaultPrepareWrite:  "old",
		FaultPrepareSync:   "old",
		FaultInstallRename: "old",
		FaultInstallSync:   "new",
	}
	stop := errors.New("stopped here")
	for point, want := range points {
		t.Run(point, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "plan.json")
			if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := Open(filepath.Join(dir, DirName), Options{Warn: quiet, UID: -1,
				Fault: func(at, _ string) error {
					if at == point {
						return stop
					}
					return nil
				}})
			if err != nil {
				t.Fatal(err)
			}
			if err := replace(t, s, p, []byte("new")); !errors.Is(err, stop) {
				t.Fatalf("err = %v, want the injected stop", err)
			}
			if b, _ := os.ReadFile(p); string(b) != want {
				t.Errorf("target = %q, want %q", b, want)
			}
			left, _ := FindPrepared(dir)
			wantLeft := point != FaultInstallSync
			if (len(left) == 1) != wantLeft {
				t.Errorf("prepared files = %v, want left behind: %v", left, wantLeft)
			}
			for _, id := range left {
				if id != txA {
					t.Errorf("prepared file carries txid %s, want %s", id, txA)
				}
			}
		})
	}
}

// IC-W12: 준비 파일 자리가 차 있으면 만들지 않는다. 그 자리의 심볼릭 링크도 따라가지 않는다.
func TestPrepareIsExclusive(t *testing.T) {
	dir := t.TempDir()
	s := open(t, filepath.Join(dir, DirName))
	target := filepath.Join(dir, "plan.json")
	prep := PreparedPath(target, txA)

	if err := os.WriteFile(prep, []byte("someone else's"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Prepare(prep, []byte("mine"), 0o600); err == nil {
		t.Fatal("Prepare wrote over an existing file")
	}
	if b, _ := os.ReadFile(prep); string(b) != "someone else's" {
		t.Errorf("the existing file became %q", b)
	}

	os.Remove(prep)
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, prep); err != nil {
		t.Fatal(err)
	}
	if err := s.Prepare(prep, []byte("mine"), 0o600); err == nil {
		t.Fatal("Prepare followed a symbolic link")
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Errorf("the link target became %q", b)
	}
}
