// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// rev: 화면이 그려질 때 본 입력 전부의 지문. 저장 요청은 이 값을 들고 오고, 서버는 잠금 안에서 다시
// 계산해 견준다. 다르면 화면이 오래된 것이므로 저장하지 않는다(409).
//
// **캐시를 쓰지 않는다.** 크기와 수정 시각이 같아도 내용이 다를 수 있다. 매번 내용을 읽어 해시한다.
// 읽는 동안 파일이나 목록이 바뀌면 그 지문은 어느 순간의 것도 아니므로, 읽기 전후의 (장치, inode,
// 크기, 수정 시각)과 목록을 견주어 다르면 다시 읽는다. 세 번 해도 다르면 ErrChanged.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// revFormat: 직렬화 판. 바꾸면 이전 화면의 rev는 모두 맞지 않게 된다(그래야 한다).
const revFormat = "rev/1"

const revAttempts = 3

// ErrChanged: 읽는 동안 입력이 계속 바뀌었다.
var ErrChanged = errors.New("the files changed while they were being read")

// Input: 지문에 들어가는 입력 하나. Name은 입력의 자리(「session」 「layer:0」 같은)다. 같은 파일이라도
// 자리가 다르면 다른 지문이 된다.
type Input struct {
	Name string
	Path string
	// Dir: 디렉토리면 아래의 보통 파일을 모두(정렬해서) 넣는다.
	Dir bool
}

// Rev: 입력 전부의 지문.
func Rev(inputs []Input) (string, error) {
	return rev(inputs, nil)
}

// rev: between은 첫 관측과 둘째 관측 사이에 불린다. 테스트가 그 틈에 파일을 바꾼다.
func rev(inputs []Input, between func()) (string, error) {
	for i := 0; i < revAttempts; i++ {
		before, err := observe(inputs)
		if err != nil {
			return "", err
		}
		sum, err := digest(inputs)
		if err != nil {
			return "", err
		}
		if between != nil {
			between()
		}
		after, err := observe(inputs)
		if err != nil {
			return "", err
		}
		if before.equal(after) {
			return sum, nil
		}
	}
	return "", fmt.Errorf("%w; wait a moment and reload the screen", ErrChanged)
}

// digest: 입력을 정해진 순서로 이어 해시한다.
func digest(inputs []Input) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n", revFormat)
	for _, in := range inputs {
		fmt.Fprintf(h, "input %q\n", in.Name)
		if !in.Dir {
			if err := digestFile(h, in.Path, ""); err != nil {
				return "", err
			}
			continue
		}
		files, err := listFiles(in.Path)
		if err != nil {
			return "", err
		}
		root := resolveDir(in.Path)
		fmt.Fprintf(h, "dir %d\n", len(files))
		for _, rel := range files {
			if err := digestFile(h, filepath.Join(root, rel), rel); err != nil {
				return "", err
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func digestFile(h io.Writer, p, rel string) error {
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(h, "absent %q\n", rel)
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		// 링크는 가리키는 곳과 그 내용을 함께 넣는다. 가리키는 곳만 넣으면 대상 파일의 내용이 바뀌어도
		// rev가 그대로라 오래된 화면의 저장이 지나간다.
		target, err := os.Readlink(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "link %q %q\n", rel, target)
		st, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(h, "dangling %q\n", rel)
			return nil
		}
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("%w: %s links to something that is not a plain file; inputs follow links to files only", ErrUnsupported, p)
		}
	} else if !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a plain file", ErrUnsupported, p)
	}
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	fh := sha256.New()
	n, err := io.Copy(fh, f)
	if err != nil {
		return err
	}
	fmt.Fprintf(h, "file %q %d %x\n", rel, n, fh.Sum(nil))
	return nil
}

// listFiles: 디렉토리 아래의 보통 파일과 링크를 상대 경로로 정렬해 낸다. 디렉토리가 없으면 빈 목록이다.
// 디렉토리 자체가 링크면 가리키는 디렉토리를 훑는다. 그 아래의 디렉토리 링크는 따라가지 않고
// digestFile이 거부한다(돌고 도는 링크를 피한다).
func listFiles(root string) ([]string, error) {
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && p == root {
			return filepath.SkipDir
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}

// sig: 파일 하나의 관측. 내용을 다시 읽지 않고 바뀌었는지만 본다.
type sig struct {
	fi  fs.FileInfo
	ok  bool
	key string
}

type observation []sig

// observe: 입력마다 (장치, inode, 크기, 수정 시각)과 디렉토리 목록.
func observe(inputs []Input) (observation, error) {
	var o observation
	add := func(p, key string) error {
		fi, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			o = append(o, sig{key: key})
			return nil
		}
		if err != nil {
			return err
		}
		o = append(o, sig{fi: fi, ok: true, key: key})
		if fi.Mode()&fs.ModeSymlink != 0 {
			// 링크가 가리키는 파일도 지켜본다. 링크 자체는 그대로인 채 대상만 바뀔 수 있다.
			st, err := os.Stat(p)
			if errors.Is(err, fs.ErrNotExist) {
				o = append(o, sig{key: key + " ->"})
				return nil
			}
			if err != nil {
				return err
			}
			o = append(o, sig{fi: st, ok: true, key: key + " ->"})
		}
		return nil
	}
	for _, in := range inputs {
		if !in.Dir {
			if err := add(in.Path, in.Name); err != nil {
				return nil, err
			}
			continue
		}
		files, err := listFiles(in.Path)
		if err != nil {
			return nil, err
		}
		o = append(o, sig{key: in.Name + "\x00" + strings.Join(files, "\x00")})
		root := resolveDir(in.Path)
		for _, rel := range files {
			if err := add(filepath.Join(root, rel), in.Name+"/"+rel); err != nil {
				return nil, err
			}
		}
	}
	return o, nil
}

func (a observation) equal(b observation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.key != y.key || x.ok != y.ok {
			return false
		}
		if !x.ok || x.fi == nil {
			continue
		}
		if !os.SameFile(x.fi, y.fi) || x.fi.Size() != y.fi.Size() || !x.fi.ModTime().Equal(y.fi.ModTime()) ||
			x.fi.Mode() != y.fi.Mode() {
			return false
		}
	}
	return true
}

// resolveDir: 디렉토리 입력이 링크면 가리키는 곳. 없거나 풀 수 없으면 그대로다.
func resolveDir(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}
