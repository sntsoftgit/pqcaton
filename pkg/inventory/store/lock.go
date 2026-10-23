// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// 잠금은 두 겹이다.
//
//   - 실행 잠금(run.lock): 상태 디렉토리 하나에 작성자는 하나다. 서버는 수명 내내 쥐고, 명령은 일하는
//     동안 쥔다. **기다리지 않는다.** 서버가 쥐고 있는데 명령이 기다리면 서버가 내려갈 때까지 멈춰
//     있게 되고, 그 사이 화면에서 본 것과 다른 결과가 나간다.
//   - 대상 잠금(.pqcaton.lock): 서버 안에서 요청 하나가 쓰는 동안 같은 디렉토리를 쓰는 다른 요청을
//     세운다. flock은 열린 파일마다 걸리므로 한 프로세스의 두 요청끼리도 서로 막는다. 잠그는 순서를
//     경로 순으로 고정해 서로 기다리다 멈추는 일이 없게 한다.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TargetLockName: 대상 디렉토리의 잠금 파일. 내용은 그 디렉토리를 쓰는 상태 디렉토리의 ID다.
const TargetLockName = ".pqcaton.lock"

const lockPrefix = "pqcaton-state "

// lockPoll: 대상 잠금을 기다리는 동안 다시 시도하는 간격.
const lockPoll = 20 * time.Millisecond

// AcquireRun: 실행 잠금을 잡는다. 다른 작성자가 쥐고 있으면 기다리지 않고 ErrBusy.
func (s *State) AcquireRun() error {
	if s.run != nil {
		return nil
	}
	p := filepath.Join(s.Dir, runLockFile)
	f, err := s.openLock(p)
	if err != nil {
		return err
	}
	if err := tryLock(f); err != nil {
		f.Close()
		if errors.Is(err, errWouldBlock) {
			return fmt.Errorf("%w (%s); stop the running pqcaton-ui first, or save from the screen", ErrBusy, s.Dir)
		}
		return err
	}
	s.run = f
	return nil
}

// Close: 실행 잠금을 놓는다.
func (s *State) Close() error {
	if s.run == nil {
		return nil
	}
	err := s.run.Close()
	s.run = nil
	return err
}

// openLock: 상태 디렉토리 안의 잠금 파일을 연다. 없으면 0600으로 만든다.
func (s *State) openLock(p string) (*os.File, error) {
	if fi, err := os.Lstat(p); err == nil {
		if err := s.checkPlain(p, fi); err != nil {
			return nil, err
		}
		if fi.Mode().Perm()&^filePerm != 0 {
			return nil, fmt.Errorf("%w: %s is open to other users (%#o); run: chmod 600 %s", ErrUnsafeFile, p, fi.Mode().Perm(), p)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return os.OpenFile(p, os.O_RDWR|os.O_CREATE|noFollow, filePerm)
}

// LockTargets: 대상 디렉토리들의 잠금을 경로 순서로 잡는다. 하나라도 wait 안에 못 잡으면 잡은 것을
// 모두 놓고 ErrTargetBusy. 돌려받은 함수로 놓는다.
//
// 잠금 파일이 없으면 이 상태 ID를 적어 만든다. 있는데 ID가 다르면 ErrStateMismatch. **고쳐 쓰지
// 않는다.** 다른 상태 디렉토리가 이 디렉토리를 쓰고 있다는 뜻이고, 둘이 같은 원장에 쓰면 영수증이
// 서로를 모른다.
func (s *State) LockTargets(dirs []string, wait time.Duration) (func(), error) {
	if s.run == nil {
		return nil, errors.New("store: LockTargets before AcquireRun")
	}
	return s.lockInOrder(dirs, wait, s.openTargetLock)
}

// lockInOrder: 디렉토리들을 경로 순서로 잠근다. open이 nil 파일을 돌려주면 그 디렉토리는 건너뛴다.
func (s *State) lockInOrder(dirs []string, wait time.Duration, open func(dir string) (*os.File, error)) (func(), error) {
	uniq := map[string]bool{}
	for _, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			return nil, err
		}
		uniq[abs] = true
	}
	order := make([]string, 0, len(uniq))
	for d := range uniq {
		order = append(order, d)
	}
	sort.Strings(order)

	var held []*os.File
	release := func() {
		for i := len(held) - 1; i >= 0; i-- {
			held[i].Close()
		}
	}
	deadline := time.Now().Add(wait)
	for _, d := range order {
		f, err := open(d)
		if err != nil {
			release()
			return nil, err
		}
		if f == nil {
			continue
		}
		for {
			err = tryLock(f)
			if err == nil || !errors.Is(err, errWouldBlock) || !time.Now().Before(deadline) {
				break
			}
			time.Sleep(lockPoll)
		}
		if err != nil {
			f.Close()
			release()
			if errors.Is(err, errWouldBlock) {
				return nil, fmt.Errorf("%w in %s; wait for it to finish and try again", ErrTargetBusy, d)
			}
			return nil, err
		}
		held = append(held, f)
	}
	return release, nil
}

// openTargetLock: 대상 디렉토리의 잠금 파일을 열고 상태 ID를 맞춘다.
//
// 만들기와 검사는 프로세스 안에서 한 번에 하나씩 한다. createExclusive는 임시 파일을 링크로 붙인 뒤
// 임시 이름을 지우므로, 그 사이에 다른 요청이 검사하면 링크가 둘로 보인다. 다른 프로세스는 실행
// 잠금이 막으므로 여기서 겹치지 않는다.
func (s *State) openTargetLock(dir string) (*os.File, error) {
	p := filepath.Join(dir, TargetLockName)
	s.mu.Lock()
	fi, err := s.ensureTargetLock(p)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if fi.Mode().Perm()&^filePerm != 0 {
		return nil, fmt.Errorf("%w: %s is open to other users (%#o); run: chmod 600 %s", ErrUnsafeFile, p, fi.Mode().Perm(), p)
	}
	f, err := os.OpenFile(p, os.O_RDWR|noFollow, 0)
	if err != nil {
		return nil, err
	}
	if same, err := sameFile(f, fi); err != nil || !same {
		f.Close()
		if err == nil {
			err = fmt.Errorf("%s was replaced while it was being opened", p)
		}
		return nil, err
	}
	if err := s.checkLockOwner(f, p, dir); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// checkLockOwner: 잠금 파일에 적힌 상태 ID가 이 상태 디렉토리인가.
func (s *State) checkLockOwner(f *os.File, p, dir string) error {
	b := make([]byte, 128)
	n, _ := f.ReadAt(b, 0)
	got := strings.TrimSpace(string(b[:n]))
	id := strings.TrimPrefix(got, lockPrefix)
	if got == id || !idPattern.MatchString(id) {
		return fmt.Errorf("%s is damaged; it should name a pqcaton state directory", p)
	}
	if id != s.ID {
		return fmt.Errorf("%w: %s is used with state %s, this command uses state %s (%s); run it with the -state-dir the screen uses",
			ErrStateMismatch, dir, id, s.ID, s.Dir)
	}
	return nil
}

// openExistingLock: **읽기용.** 대상 디렉토리에 잠금 파일이 있을 때만 연다. 없으면 nil이다 — 그
// 디렉토리를 이 도구가 쓴 적이 없다는 뜻이라 견줄 확정도 없다. 만들지 않는다(읽기가 남의 디렉토리에
// 파일을 남기면 안 된다). 링크 · 소유자 · 권한 · 상태 ID는 쓰기와 같이 본다.
func (s *State) openExistingLock(dir string) (*os.File, error) {
	p := filepath.Join(dir, TargetLockName)
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.checkPlain(p, fi); err != nil {
		return nil, err
	}
	if fi.Mode().Perm()&^filePerm != 0 {
		return nil, fmt.Errorf("%w: %s is open to other users (%#o); run: chmod 600 %s", ErrUnsafeFile, p, fi.Mode().Perm(), p)
	}
	f, err := os.OpenFile(p, os.O_RDONLY|noFollow, 0)
	if err != nil {
		return nil, err
	}
	if same, err := sameFile(f, fi); err != nil || !same {
		f.Close()
		if err == nil {
			err = fmt.Errorf("%s was replaced while it was being opened", p)
		}
		return nil, err
	}
	if err := s.checkLockOwner(f, p, dir); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// ensureTargetLock: 잠금 파일이 없으면 만들고, 링크·소유자·권한을 본다.
func (s *State) ensureTargetLock(p string) (fs.FileInfo, error) {
	if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
		if err := createExclusive(p, []byte(lockPrefix+s.ID+"\n")); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create %s: %w", p, err)
		}
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if err := s.checkPlain(p, fi); err != nil {
		return nil, err
	}
	return fi, nil
}

// sameFile: 연 파일이 검사한 그 파일인가. 검사와 열기 사이에 바뀌어 끼워졌으면 아니다.
func sameFile(f *os.File, checked fs.FileInfo) (bool, error) {
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	return os.SameFile(fi, checked), nil
}
