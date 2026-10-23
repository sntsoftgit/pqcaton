// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// 저수준 쓰기. 대상 파일은 **제자리에서 고쳐 쓰지 않는다.** 같은 디렉토리에 준비 파일을 다 쓰고
// fsync한 뒤 rename으로 바꿔 끼운다. 그러면 어느 순간에 멈춰도 대상은 옛 내용이거나 새 내용이고,
// 반쯤 쓴 내용이 그 이름으로 보이는 일이 없다.
//
// 준비 파일 이름에는 트랜잭션 ID가 들어간다(.<이름>.pqcaton-<txid>.tmp). 복구는 기록에 열거된
// 이름만 지운다. 이름 모양만 보고 한꺼번에 지우면 남이 만든 파일을 지울 수 있다.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// 실패를 주입하는 자리. 테스트가 Options.Fault로 이 자리에서 오류를 내게 해 멈춘 상태를 만든다.
const (
	FaultPrepareWrite  = "prepare.write"
	FaultPrepareSync   = "prepare.sync"
	FaultInstallRename = "install.rename"
	FaultInstallSync   = "install.syncdir"
)

var preparedPattern = regexp.MustCompile(`^\..+\.pqcaton-([0-9a-f]{32})\.tmp$`)

// Target: 쓰기 전에 본 대상 파일.
type Target struct {
	Path   string
	Exists bool
	// Mode: 교체한 파일이 가질 권한. 있던 파일이면 그 권한, 새 파일이면 0600.
	Mode fs.FileMode
	// SHA256: 지금 내용의 해시. 없으면 빈 문자열.
	SHA256 string
	Size   int64
}

// Inspect: 대상 파일을 쓸 수 있는지 보고 지금 내용의 해시를 잰다.
//
// 거부하는 것: 심볼릭 링크, 하드 링크 둘 이상, 남의 소유, group·other 쓰기 권한. 쓰기 권한이
// 열린 파일은 남이 이미 바꿨을 수 있어 이쪽이 그 위에 확정을 쌓으면 안 된다. 읽기 권한만 열려
// 있으면 경고하고 지나간다.
func (s *State) Inspect(p string) (Target, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return Target{}, err
	}
	t := Target{Path: abs, Mode: filePerm}
	fi, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return Target{}, err
	}
	if err := s.checkPlain(abs, fi); err != nil {
		return Target{}, err
	}
	perm := fi.Mode().Perm()
	if perm&0o022 != 0 {
		return Target{}, fmt.Errorf("%w: %s can be written by other users (%#o); run: chmod go-w %s", ErrUnsafeFile, abs, perm, abs)
	}
	if perm&0o044 != 0 {
		s.warn(fmt.Sprintf("%s can be read by other users (%#o); consider: chmod go-r %s", abs, perm, abs))
	}
	sum, size, err := hashFile(abs)
	if err != nil {
		return Target{}, err
	}
	t.Exists, t.Mode, t.SHA256, t.Size = true, perm, sum, size
	return t, nil
}

// PreparedPath: 대상 옆에 둘 준비 파일의 경로.
func PreparedPath(target, txid string) string {
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".pqcaton-"+txid+".tmp")
}

// Prepare: 준비 파일을 만들고 내용을 다 써서 fsync한다. **자리가 차 있으면 만들지 않는다.** 링크를
// 따라가지도 않는다. 권한은 mode로 맞춘다(있던 파일의 권한을 이어받게).
func (s *State) Prepare(path string, content []byte, mode fs.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|noFollow, filePerm)
	if err != nil {
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := s.fault(FaultPrepareWrite, path); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	if err := s.fault(FaultPrepareSync, path); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Install: 준비 파일을 대상 자리로 옮기고 디렉토리를 fsync한다. 그 뒤에야 바뀐 이름이 디스크에 남는다.
func (s *State) Install(prepared, target string) error {
	if err := s.fault(FaultInstallRename, target); err != nil {
		return err
	}
	if err := os.Rename(prepared, target); err != nil {
		return err
	}
	if err := s.fault(FaultInstallSync, target); err != nil {
		return err
	}
	return syncDir(filepath.Dir(target))
}

// FindPrepared: 디렉토리에 남은 준비 파일과 그 트랜잭션 ID. 지우지 않는다. 무엇을 할지는 부르는
// 쪽이 기록과 맞춰 본 뒤 정한다.
func FindPrepared(dir string) (map[string]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range ents {
		if m := preparedPattern.FindStringSubmatch(e.Name()); m != nil {
			out[filepath.Join(dir, e.Name())] = m[1]
		}
	}
	return out, nil
}

func (s *State) fault(point, path string) error {
	if s.faultFn == nil {
		return nil
	}
	return s.faultFn(point, path)
}

// hashFile: 내용의 sha256과 크기.
func hashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// Sum: 바이트의 sha256.
func Sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
