// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// Package store — 화면과 명령이 파일을 바꿀 때 지나가는 한 길이다.
//
// 바꾸는 쪽은 모두 **상태 디렉토리** 하나를 함께 쓴다. 거기에 트랜잭션 기록과 영수증이 있고,
// 실행 잠금(run.lock)이 있다. 바꾸는 대상 디렉토리에는 잠금 파일(.pqcaton.lock)과 준비 파일만 둔다.
//
// **동일성은 경로가 아니라 상태 ID로 본다.** 상태 디렉토리를 만들 때 무작위 ID를 만들어 두고, 대상
// 디렉토리의 잠금 파일에 같은 ID를 적는다. 경로를 권위값으로 삼으면 백업해 옮긴 데이터가 막힌다.
// 경로는 안내에만 쓴다.
//
// 상용 지원은 Linux 로컬 파일시스템뿐이다. macOS는 경고하고 돌고, NFS·CIFS는 시작 전에 거부하며,
// 그 밖의 운영체제에서는 바꾸는 일을 하지 않는다.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// DirName: 기본 상태 디렉토리 이름. 세션 파일이 있는 디렉토리 아래에 둔다.
const DirName = ".pqcaton"

const (
	stateFile   = "state.json"
	runLockFile = "run.lock"
	stateFormat = 1
	idBytes     = 16
)

// 상태 디렉토리와 그 안의 파일 권한. 영수증에 무엇을 언제 확정했는지가 담기므로 남이 읽지 못하게 한다.
const (
	dirPerm  fs.FileMode = 0o700
	filePerm fs.FileMode = 0o600
)

var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

var (
	// ErrBusy: 다른 작성자(서버나 명령)가 이 상태 디렉토리를 쥐고 있다.
	ErrBusy = errors.New("another pqcaton writer is using this state directory")
	// ErrStateMismatch: 대상 디렉토리가 다른 상태 디렉토리에 묶여 있다.
	ErrStateMismatch = errors.New("the directory belongs to a different pqcaton state directory")
	// ErrTargetBusy: 대상 디렉토리의 잠금을 기다리다 시간이 지났다.
	ErrTargetBusy = errors.New("another save is in progress")
	// ErrUnsafeFile: 링크·소유자·권한 때문에 쓰지 않는 파일이다.
	ErrUnsafeFile = errors.New("unsafe file")
	// ErrUnsupported: 이 플랫폼이나 파일시스템에서는 바꾸는 일을 하지 않는다.
	ErrUnsupported = errors.New("not supported here")
	// ErrDamaged: 상태 디렉토리의 파일(상태 문서 · 잠금 파일 · 기록 · 영수증)을 읽을 수 없다. 화면은 이 값으로
	// 종류를 가리고 문장을 해석하지 않는다.
	ErrDamaged = errors.New("damaged")
)

// State: 열어 둔 상태 디렉토리.
type State struct {
	// Dir: 상태 디렉토리의 절대 경로. 안내에만 쓴다.
	Dir string
	// ID: 상태 ID. 대상 디렉토리의 잠금 파일에 같은 값이 적힌다.
	ID string

	uid     int
	warn    func(string)
	faultFn func(point, path string) error
	run     *os.File

	// active: 이 프로세스에서 지금 진행 중인 트랜잭션. 복구가 남의 진행 중인 기록을 끝내지 않게 한다.
	mu     sync.Mutex
	active map[string]bool
}

// Options: Open의 선택 사항.
type Options struct {
	// Warn: 막지는 않지만 알려야 할 것(검증하지 않은 플랫폼, 남이 읽을 수 있는 파일)을 받는다.
	// 비우면 표준오류로 낸다.
	Warn func(string)
	// UID: 파일 소유자로 인정할 사용자. 테스트만 바꾼다. 0 이하면 현재 프로세스다(root로 돌면
	// 현재 프로세스가 곧 0이다).
	UID int
	// Fault: 실패 주입. 정해진 자리(Fault* 상수)에서 불리고, 오류를 돌려주면 거기서 멈춘다.
	// 테스트만 쓴다. 전역 변수가 아니라 객체에 두어 테스트끼리 섞이지 않는다.
	Fault func(point, path string) error
}

type stateDoc struct {
	Format  int    `json:"format"`
	StateID string `json:"state_id"`
}

// DefaultDir: 세션 파일 옆의 기본 상태 디렉토리.
func DefaultDir(sessionFile string) string {
	return filepath.Join(filepath.Dir(sessionFile), DirName)
}

// Open: 상태 디렉토리를 연다. 없으면 만든다. 실행 잠금은 잡지 않는다(AcquireRun).
//
// 막는 것: 이 플랫폼·파일시스템에서 바꾸는 일, 심볼릭 링크인 상태 디렉토리, 남의 소유, 0700보다
// 넓은 디렉토리, 0600보다 넓은 안의 파일. **고쳐 주지 않고 멈춘다.** 넓은 권한은 이미 누가 읽었을
// 수 있다는 뜻이라 조용히 좁히면 그 사실이 묻힌다.
func Open(dir string, opt Options) (*State, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	s := &State{Dir: abs, uid: opt.UID, warn: opt.Warn, faultFn: opt.Fault}
	if s.uid <= 0 {
		s.uid = os.Getuid()
	}
	if s.warn == nil {
		s.warn = func(m string) { fmt.Fprintln(os.Stderr, "warning:", m) }
	}
	if err := platformCheck(s.warn); err != nil {
		return nil, err
	}
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	if err := fsCheck(abs, s.warn); err != nil {
		return nil, err
	}
	if err := s.ensureID(); err != nil {
		return nil, err
	}
	if err := s.checkTree(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *State) ensureDir() error {
	fi, err := os.Lstat(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(s.Dir, dirPerm); err != nil {
			return fmt.Errorf("create state directory %s: %w", s.Dir, err)
		}
		// umask가 0700을 좁힐 수는 있어도 넓히지는 않는다. 그래도 적힌 대로 맞춘다.
		if err := os.Chmod(s.Dir, dirPerm); err != nil {
			return err
		}
		fi, err = os.Lstat(s.Dir)
	}
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("%w: the state directory %s is not a plain directory (a symbolic link or a file)", ErrUnsafeFile, s.Dir)
	}
	if err := s.checkOwner(s.Dir, fi); err != nil {
		return err
	}
	if fi.Mode().Perm()&^dirPerm != 0 {
		return fmt.Errorf("%w: the state directory %s is open to other users (%#o); run: chmod 700 %s",
			ErrUnsafeFile, s.Dir, fi.Mode().Perm(), s.Dir)
	}
	return nil
}

// ensureID: state.json을 읽는다. 없으면 만든다. **덮어쓰지 않는다.** 둘이 동시에 만들어도 먼저
// 자리를 잡은 쪽의 ID가 남고, 다른 쪽은 그것을 읽는다(링크는 자리가 차 있으면 실패한다).
func (s *State) ensureID() error {
	p := filepath.Join(s.Dir, stateFile)
	if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
		id, err := newID()
		if err != nil {
			return err
		}
		b, _ := json.Marshal(stateDoc{Format: stateFormat, StateID: id})
		if err := createExclusive(p, append(b, '\n')); err != nil && !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("create %s: %w", p, err)
		}
	}
	b, err := s.readPrivate(p)
	if err != nil {
		return err
	}
	var doc stateDoc
	if err := json.Unmarshal(b, &doc); err != nil || doc.Format != stateFormat || !idPattern.MatchString(doc.StateID) {
		return fmt.Errorf("%w: %s is damaged or from another version; restore it from a backup of this state directory", ErrDamaged, p)
	}
	s.ID = doc.StateID
	return nil
}

// checkTree: 상태 디렉토리 안의 모든 것이 링크가 아니고, 내 것이고, 남에게 닫혀 있는가.
func (s *State) checkTree() error {
	return filepath.WalkDir(s.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%w: %s is a symbolic link; the state directory holds only plain files", ErrUnsafeFile, p)
		case fi.IsDir():
			if err := s.checkOwner(p, fi); err != nil {
				return err
			}
			if fi.Mode().Perm()&^dirPerm != 0 {
				return fmt.Errorf("%w: %s is open to other users (%#o); run: chmod 700 %s", ErrUnsafeFile, p, fi.Mode().Perm(), p)
			}
		case !fi.Mode().IsRegular():
			return fmt.Errorf("%w: %s is not a plain file", ErrUnsafeFile, p)
		default:
			if err := s.checkPlain(p, fi); err != nil {
				return err
			}
			if fi.Mode().Perm()&^filePerm != 0 {
				return fmt.Errorf("%w: %s is open to other users (%#o); run: chmod 600 %s", ErrUnsafeFile, p, fi.Mode().Perm(), p)
			}
		}
		return nil
	})
}

// readPrivate: 상태 디렉토리 안의 파일을 읽는다. 링크·남의 소유·넓은 권한이면 읽지 않는다.
func (s *State) readPrivate(p string) ([]byte, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if err := s.checkPlain(p, fi); err != nil {
		return nil, err
	}
	if fi.Mode().Perm()&^filePerm != 0 {
		return nil, fmt.Errorf("%w: %s is open to other users (%#o); run: chmod 600 %s", ErrUnsafeFile, p, fi.Mode().Perm(), p)
	}
	return os.ReadFile(p)
}

// checkPlain: 보통 파일이고, 링크가 하나뿐이고, 내 것인가.
//
// 하드 링크가 둘 이상이면 다른 이름으로 같은 내용을 가리키는 자리가 있다. 그 자리를 거쳐 바뀌면
// 이쪽이 모르는 변경이 되고, rename으로 교체하면 다른 이름 쪽만 옛 내용으로 남는다.
func (s *State) checkPlain(p string, fi fs.FileInfo) error {
	if fi.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s is a symbolic link", ErrUnsafeFile, p)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a plain file", ErrUnsafeFile, p)
	}
	if n := linkCount(fi); n > 1 {
		return fmt.Errorf("%w: %s has %d hard links; keep a single copy and try again", ErrUnsafeFile, p, n)
	}
	return s.checkOwner(p, fi)
}

// checkOwner: 이 프로세스의 사용자가 소유한 것인가. rename으로 교체하면 소유자가 바뀌므로 남의
// 파일은 처음부터 쓰지 않는다.
func (s *State) checkOwner(p string, fi fs.FileInfo) error {
	if uid, ok := ownerOf(fi); ok && uid != s.uid {
		return fmt.Errorf("%w: %s is owned by another user (uid %d); run pqcaton as that user, or copy the file", ErrUnsafeFile, p, uid)
	}
	return nil
}

// createExclusive: 자리가 비어 있을 때만 만든다. 내용을 다 쓰고 fsync한 임시 파일을 링크로 붙이므로,
// 반쯤 쓴 파일이 그 이름으로 보이는 순간이 없다.
func createExclusive(p string, content []byte) error {
	dir := filepath.Dir(p)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(p)+".new-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(filePerm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(name, p); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func newID() (string, error) {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
