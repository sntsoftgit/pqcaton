// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// install · recover — 확인한 수집기를 화면 작업 폴더의 collectors/ 로 들인다. 화면이 도는 기계(데모에서는 pqcota-ctl
// 컨테이너) 안에서 돈다. 어디서 끊겨도 collectors/ 가 반쯤 바뀐 채로 남지 않는다.
//
// 작업 폴더 W 의 자리(모두 같은 파일시스템):
//
//	W/collectors                        지금 쓰는 수집기          W/.collectors.sha256           그 해시 목록
//	W/.collectors.incoming-<트랜잭션>    들일 복사본(ui.sh 가 둠)  W/.collectors.incoming-<…>.sha256 확인한 준비 폴더의 해시 목록
//	W/.collectors.old                   보관한 이전 수집기        W/.collectors.old.sha256       그 해시 목록
//	W/.collectors.rejected-<시각>        검증에 실패해 격리한 것(지우지 않는다)
//
// install 의 순서. 3이 확정 시점이다:
//
//  1. incoming 이 해시 목록과 같은지(파일 목록 · 해시) 본다. 다르면 incoming 을 버리고 멈춘다(collectors 는 그대로).
//  2. 지금 collectors(와 그 해시 목록)를 old 로 옮긴다.
//  3. incoming 해시 목록을 제자리로, incoming 을 rename 으로 collectors 에 들인다. 실패하면 old 를 되돌린다.
//  4. old 를 지운다.
//
// **잠금.** install 과 recover 는 화면과 같은 작업 폴더 실행 잠금(W/.pqcaton/run.lock, store.AcquireRun)을 잡고서만 한다.
// 기다리지 않는다: 화면이 떠 있거나 다른 install · recover 가 돌고 있으면 아무것도 바꾸지 않고 실패한다.
//
// **작업 폴더 안의 복사도 잠금 아래에서 한다.** ui.sh 는 확인한 수집기를 작업 폴더 **밖**의 임시 자리(source, 예:
// /tmp/pqcaton-collectors-*/collectors 와 그 옆의 .sha256)에 두고 install -from <source> 를 한 번 부른다. install 은
// source 를 1차로 검사하며 **내용 스냅샷**(파일 목록 · 종류 · 모드 · 크기 · SHA-256, 해시 목록 파일의 바이트와 SHA-256)을
// 메모리에 담고(잘못된 것은 작업 폴더에 흔적 없이 거절), 잠금을 잡은 뒤 source 를 다시 찍어 스냅샷과 같은지 보고, 옛 중단
// 상태를 정리하고, 작업 폴더 안에 고유 복사본(.collectors.incoming-*)을 만들어 복사한 뒤 그 복사본을 **처음 스냅샷과** 견주고
// 같을 때만 들인다. 복사본의 해시 목록도 스냅샷의 바이트로 쓴다. 그래서 source 의 파일과 해시 목록을 함께 제자리에서 고쳐도,
// 복사 도중에 고쳐도 통과하지 않는다(최종으로 믿는 것은 잠금 전에 읽은 내용이다). 그래서 작업 폴더 안에 복사본이 있는 동안에는
// 언제나 같은 실행이 잠금을 쥔다. 잠금을 얻은 recover 가 보는 복사본은 모두 실제 중단 흔적이라 버린다. 옛 중단 상태의 정리:
//
//   - incoming 만 있음: 불완전한 복사본이라 버린다. collectors 는 그대로.
//   - old 가 있고 collectors 가 없음: old 가 그 해시 목록과 같으면 되돌린다.
//   - old 와 collectors 가 모두 있음: collectors 가 그 해시 목록과 같으면 그대로 두고 old 를 지운다(3 뒤 4 전에 끊긴 경우).
//     같지 않으면 collectors 를 격리하고 old 를 되돌린다(old 가 그 해시 목록과 같을 때).
//   - 어느 쪽도 정상이라고 확인할 수 없으면 아무것도 지우거나 덮어쓰지 않고 실패한다.

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

// fsOps — install 이 쓰는 파일 연산. 시험이 실패 · 중단을 넣는다.
type fsOps interface {
	Rename(from, to string) error
	RemoveAll(path string) error
}

type osOps struct{}

func (osOps) Rename(from, to string) error { return os.Rename(from, to) }
func (osOps) RemoveAll(path string) error  { return os.RemoveAll(path) }

type place struct {
	ws, cur, curSum, old, oldSum string
}

// incomingPrefix — 복사본 자리의 앞머리. 뒤에 트랜잭션 ID 가 붙는다.
const incomingPrefix = ".collectors.incoming"

func places(ws string) place {
	return place{ws: ws, cur: filepath.Join(ws, "collectors"), curSum: filepath.Join(ws, ".collectors.sha256"),
		old: filepath.Join(ws, ".collectors.old"), oldSum: filepath.Join(ws, ".collectors.old.sha256")}
}

// lockWS — 작업 폴더 실행 잠금을 잡는다(화면과 같은 store.AcquireRun, 기다리지 않는다). 시험이 바꾼다.
var lockWS = func(ws string) (func(), error) {
	st, err := store.Open(filepath.Join(ws, store.DirName), store.Options{})
	if err != nil {
		return nil, err
	}
	if err := st.AcquireRun(); err != nil {
		st.Close()
		return nil, fmt.Errorf("the workspace is in use (the screen is running, or another install is): stop the screen first; nothing was changed (%v)", err)
	}
	return func() { st.Close() }, nil
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// readSums — `sha256sum` 꼴(`<64자>  ./<경로>`)의 해시 목록.
func readSums(p string) (map[string]string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		h, name, ok := strings.Cut(sc.Text(), "  ")
		name = strings.TrimPrefix(name, "./")
		if !ok || !hex64.MatchString(h) || name == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
			return nil, fmt.Errorf("%s has a malformed line", filepath.Base(p))
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("%s names %s twice", filepath.Base(p), name)
		}
		out[name] = h
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s is empty", filepath.Base(p))
	}
	return out, nil
}

// matches — dir 의 파일이 해시 목록과 정확히 같은가(더한 것 · 빠진 것 · 다른 해시가 없다).
func matches(dir, sumsFile string) error {
	want, err := readSums(sumsFile)
	if err != nil {
		return err
	}
	have, err := listFiles(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	if err := sameSet(have, names); err != nil {
		return err
	}
	for _, n := range names {
		h, err := shaFile(filepath.Join(dir, filepath.FromSlash(n)))
		if err != nil {
			return err
		}
		if h != want[n] {
			return fmt.Errorf("%s does not match its hash list", n)
		}
	}
	return nil
}

// recoverWS — 지난 install 이 끊긴 자리를 정리한다(잠금 안에서 부른다). 작업 폴더 안의 복사본은 잠금을 쥔 실행만 만들므로,
// 잠금을 얻었을 때 남은 복사본(.collectors.incoming*)은 모두 중단 흔적이라 버린다. 무엇을 했는지 한 줄씩 돌려준다.
func recoverWS(ws string, ops fsOps) ([]string, error) {
	p := places(ws)
	var did []string
	ents, err := os.ReadDir(ws)
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		path := filepath.Join(ws, e.Name())
		if !strings.HasPrefix(e.Name(), incomingPrefix) {
			continue
		}
		if err := ops.RemoveAll(path); err != nil {
			return did, err
		}
		did = append(did, "discarded a copy an earlier run left ("+e.Name()+")")
	}
	if !exists(p.old) {
		// 보관본은 없고 그 해시 목록만 남았다: 지금 것의 해시 목록이 없으면 그것이 지금 것의 목록이므로 되돌리고, 있으면 지운다.
		if exists(p.oldSum) {
			var err error
			if exists(p.cur) && !exists(p.curSum) {
				err = ops.Rename(p.oldSum, p.curSum)
			} else {
				err = ops.RemoveAll(p.oldSum)
			}
			if err != nil {
				return did, err
			}
		}
		return did, nil
	}
	// 보관본의 해시 목록. 지금 것을 옮긴 직후 목록을 옮기기 전에 끊겼으면 아직 원래 이름(지금 것의 자리)에 있다.
	oldSum := p.oldSum
	if !exists(oldSum) && !exists(p.cur) && exists(p.curSum) {
		oldSum = p.curSum
	}
	oldOK := matches(p.old, oldSum) == nil
	if exists(p.cur) {
		if err := matches(p.cur, p.curSum); err == nil {
			if err := ops.RemoveAll(p.old); err != nil {
				return did, err
			}
			if err := ops.RemoveAll(p.oldSum); err != nil {
				return did, err
			}
			return append(did, "kept the current collectors (they match their hash list) and removed the earlier copy"), nil
		}
		if !oldOK {
			return did, errors.New("neither the current collectors nor the earlier copy match their hash lists; nothing was removed or replaced")
		}
		rejected := filepath.Join(ws, ".collectors.rejected-"+strconv.FormatInt(time.Now().UnixNano(), 10))
		if err := ops.Rename(p.cur, rejected); err != nil {
			return did, err
		}
		if exists(p.curSum) {
			if err := ops.Rename(p.curSum, rejected+".sha256"); err != nil {
				return did, err
			}
		}
		did = append(did, "set aside the current collectors that do not match their hash list as "+filepath.Base(rejected))
	} else if !oldOK {
		return did, errors.New("the earlier copy does not match its hash list and there are no current collectors; nothing was removed or replaced")
	}
	if err := ops.Rename(p.old, p.cur); err != nil {
		return did, err
	}
	if oldSum == p.oldSum {
		if err := ops.Rename(p.oldSum, p.curSum); err != nil {
			return did, err
		}
	}
	return append(did, "adopted the earlier collectors again (restored)"), nil
}

// installWS — 복사본 inc 를 collectors 로 들인다(맨 위의 순서, 잠금 안에서 부른다).
func installWS(ws, inc string, ops fsOps) ([]string, error) {
	p := places(ws)
	incSum := inc + ".sha256"
	if filepath.Dir(filepath.Clean(inc)) != filepath.Clean(ws) || !incomingName.MatchString(filepath.Base(inc)) {
		return nil, fmt.Errorf("the copy must be %s/%s-<id>, not %s", ws, incomingPrefix, inc)
	}
	if !exists(inc) || !exists(incSum) {
		return nil, errors.New("there is no copy to put in (" + filepath.Base(inc) + " and its hash list)")
	}
	if err := matches(inc, incSum); err != nil {
		ops.RemoveAll(inc)
		ops.RemoveAll(incSum)
		return nil, fmt.Errorf("the copy does not match the verified collectors (%v); it was discarded and the collectors are unchanged", err)
	}
	if exists(p.old) || exists(p.oldSum) {
		return nil, errors.New("an earlier copy (.collectors.old) is still there; run recover first")
	}
	moved := false
	if exists(p.cur) {
		if err := ops.Rename(p.cur, p.old); err != nil {
			return nil, err
		}
		moved = true
		if exists(p.curSum) {
			if err := ops.Rename(p.curSum, p.oldSum); err != nil {
				ops.Rename(p.old, p.cur)
				return nil, err
			}
		}
	}
	undo := func(cause error) ([]string, error) {
		if moved {
			if exists(p.cur) {
				return nil, fmt.Errorf("%v; the collectors could not be restored automatically, run recover", cause)
			}
			ops.Rename(p.old, p.cur)
			if exists(p.oldSum) {
				ops.Rename(p.oldSum, p.curSum)
			}
		} else {
			ops.RemoveAll(p.curSum)
		}
		ops.RemoveAll(inc)
		ops.RemoveAll(incSum)
		return nil, fmt.Errorf("could not put the collectors in place (%v); the earlier collectors were restored", cause)
	}
	if err := ops.Rename(incSum, p.curSum); err != nil {
		return undo(err)
	}
	if err := ops.Rename(inc, p.cur); err != nil {
		return undo(err)
	}
	adopted := "adopted the new verified collectors (there were none before)"
	if moved {
		adopted = "adopted the new verified collectors in place of the earlier ones"
	}
	if err := ops.RemoveAll(p.old); err != nil {
		return []string{adopted}, fmt.Errorf("the collectors are in place, but the earlier copy could not be removed (%v); run recover", err)
	}
	if err := ops.RemoveAll(p.oldSum); err != nil {
		return []string{adopted}, fmt.Errorf("the collectors are in place, but the earlier hash list could not be removed (%v); run recover", err)
	}
	return []string{adopted}, nil
}

// incomingName — 복사본 자리의 이름(os.MkdirTemp 가 붙이는 글자들).
var incomingName = regexp.MustCompile(`^\.collectors\.incoming-[A-Za-z0-9._-]+$`)

// sourceInfo — 1차 검사에서 본 source 의 실제 경로 · 파일(device/inode)과 내용 스냅샷. 잠금 안에서 그대로인지 다시 견준다.
type sourceInfo struct {
	real      string
	dir, sums os.FileInfo
	snap      *snapshot
}

// snapshot — source 의 내용: 파일마다 종류(일반 파일만) · 모드 · 크기 · SHA-256, 그리고 해시 목록 파일의 바이트와 SHA-256.
type snapshot struct {
	sumsBytes []byte
	sumsHash  string
	files     map[string]fileEntry
}

type fileEntry struct {
	mode os.FileMode
	size int64
	sha  string
}

// takeSnapshot — dir 의 내용을 찍는다. 일반 파일과 폴더만 받고, 파일 목록과 해시는 sumsBytes(해시 목록)와 정확히 같아야 한다.
func takeSnapshot(dir string, sumsBytes []byte) (*snapshot, error) {
	tmp, err := os.CreateTemp("", "pqcaton-sums-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	tmp.Write(sumsBytes)
	tmp.Close()
	want, err := readSums(tmp.Name())
	if err != nil {
		return nil, err
	}
	snap := &snapshot{sumsBytes: sumsBytes, sumsHash: sha(sumsBytes), files: map[string]fileEntry{}}
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." || d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := shaFile(p)
		if err != nil {
			return err
		}
		snap.files[filepath.ToSlash(rel)] = fileEntry{mode: info.Mode(), size: info.Size(), sha: h}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(snap.files) != len(want) {
		return nil, fmt.Errorf("the files do not match the hash list (%d files, %d listed)", len(snap.files), len(want))
	}
	for n, h := range want {
		if e, ok := snap.files[n]; !ok || e.sha != h {
			return nil, fmt.Errorf("%s does not match the hash list", n)
		}
	}
	return snap, nil
}

// equal — 두 스냅샷의 내용이 같은가(해시 목록 바이트, 파일마다 모드 · 크기 · SHA-256). checkMode 가 거짓이면 모드는 보지 않는다.
func (a *snapshot) equal(b *snapshot, checkMode bool) error {
	if a.sumsHash != b.sumsHash {
		return errors.New("the hash list changed")
	}
	if len(a.files) != len(b.files) {
		return errors.New("the set of files changed")
	}
	for n, e := range a.files {
		f, ok := b.files[n]
		switch {
		case !ok:
			return fmt.Errorf("%s is gone", n)
		case e.sha != f.sha || e.size != f.size:
			return fmt.Errorf("%s changed", n)
		case checkMode && e.mode.Perm() != f.mode.Perm():
			return fmt.Errorf("the mode of %s changed", n)
		}
	}
	return nil
}

// checkSource — source 가 작업 폴더 밖의 진짜 폴더이고 옆에 해시 목록(<source>.sha256, 일반 파일)이 있는가. 문자열이 아니라
// 실제 경로로 따진다: 심볼릭 링크가 아닌 디렉터리, 실제 경로가 작업 폴더의 실제 경로 안이 아님, 작업 폴더도 심볼릭 링크 아님.
func checkSource(ws, src string) (*sourceInfo, error) {
	refuse := func(why string) (*sourceInfo, error) {
		return nil, fmt.Errorf("the collectors to put in must be a real folder outside the workspace with a hash list next to it (%s); nothing was changed", why)
	}
	wi, err := os.Lstat(ws)
	if err != nil || !wi.IsDir() || wi.Mode()&os.ModeSymlink != 0 {
		return refuse("the workspace is not a real folder")
	}
	realWS, err := filepath.EvalSymlinks(ws)
	if err != nil {
		return refuse("the workspace cannot be resolved")
	}
	if absWS, err := filepath.Abs(ws); err != nil || realWS != filepath.Clean(absWS) {
		return refuse("the path to the workspace goes through a symbolic link")
	}
	di, err := os.Lstat(src)
	if err != nil {
		return refuse("it does not exist")
	}
	if di.Mode()&os.ModeSymlink != 0 || !di.IsDir() {
		return refuse("it is a symbolic link or not a folder")
	}
	real, err := filepath.EvalSymlinks(src)
	if err != nil {
		return refuse("it cannot be resolved")
	}
	if real == realWS || strings.HasPrefix(real, realWS+string(filepath.Separator)) {
		return refuse("it is inside the workspace")
	}
	si, err := os.Lstat(src + ".sha256")
	if err != nil || !si.Mode().IsRegular() {
		return refuse("its hash list is missing, a symbolic link or not a regular file")
	}
	sumsBytes, err := os.ReadFile(src + ".sha256")
	if err != nil {
		return refuse("its hash list cannot be read")
	}
	snap, err := takeSnapshot(src, sumsBytes)
	if err != nil {
		return refuse(err.Error())
	}
	return &sourceInfo{real: real, dir: di, sums: si, snap: snap}, nil
}

// same — 잠금 안에서 본 source 가 1차 검사 때와 같은 실제 경로 · 파일(device/inode)이고 **내용도 같은가**(스냅샷).
func (a *sourceInfo) same(b *sourceInfo) error {
	if a.real != b.real || !os.SameFile(a.dir, b.dir) || !os.SameFile(a.sums, b.sums) {
		return errors.New("it is another folder or hash list now")
	}
	return a.snap.equal(b.snap, true)
}

// stageCopy — source 를 작업 폴더 안의 새 고유 복사본으로 복사한다(잠금 안에서 부른다). 일반 파일과 폴더만 옮기고 그 밖의 것
// (링크 · 장치 파일)이 있으면 멈춘다. 옆의 해시 목록은 source 의 파일이 아니라 스냅샷의 바이트(sums)로 쓴다. 시험이 바꾼다(복사
// 도중에 다른 실행을 끼우거나 source 를 고친다).
var stageCopy = func(ws, src string, sums []byte) (string, error) {
	inc, err := os.MkdirTemp(ws, incomingPrefix+"-")
	if err != nil {
		return "", err
	}
	err = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		dst := filepath.Join(inc, rel)
		switch {
		case rel == ".":
			return nil
		case d.IsDir():
			return os.Mkdir(dst, 0o755)
		case !d.Type().IsRegular():
			return fmt.Errorf("%s in the collectors to put in is not a regular file", rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(p, dst, info.Mode().Perm())
	})
	if err == nil {
		err = os.WriteFile(inc+".sha256", sums, 0o600)
	}
	if err != nil {
		os.RemoveAll(inc)
		os.RemoveAll(inc + ".sha256")
		return "", err
	}
	return inc, nil
}

func copyFile(from, to string, mode os.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Chmod(mode); err != nil { // umask 와 상관없이 source 의 모드 그대로(스냅샷과 견준다)
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// runWS — recover · install 명령. 둘 다 작업 폴더 실행 잠금을 잡고서만 한다. install 은 잠금 안에서 source 를 다시 확인하고,
// 복구한 뒤, 작업 폴더 안의 고유 복사본으로 복사해 다시 검증하고 들인다.
func runWS(cmd, ws, from string, ops fsOps) ([]string, error) {
	if cmd != "recover" && cmd != "install" {
		return nil, fmt.Errorf("unknown command %q", cmd)
	}
	var before *sourceInfo
	if cmd == "install" {
		if from == "" {
			return nil, errors.New("install needs -from <the collectors to put in>")
		}
		// 잘못된 source 는 잠금(상태 디렉터리를 만들 수 있다)보다 먼저 거절해, 작업 폴더에 아무 흔적도 남기지 않는다.
		var err error
		if before, err = checkSource(ws, from); err != nil {
			return nil, err
		}
	}
	release, err := lockWS(ws)
	if err != nil {
		return nil, err
	}
	defer release()
	if cmd == "install" {
		now, err := checkSource(ws, from)
		if err != nil {
			return nil, err
		}
		if err := before.same(now); err != nil {
			return nil, fmt.Errorf("the collectors to put in changed between the check and the lock (%v); nothing was changed", err)
		}
	}
	did, err := recoverWS(ws, ops)
	if err != nil || cmd == "recover" {
		return did, err
	}
	inc, err := stageCopy(ws, from, before.snap.sumsBytes)
	if err != nil {
		return did, fmt.Errorf("could not copy the collectors into the workspace (%v); the collectors are unchanged", err)
	}
	// 최종 기준은 잠금 전에 읽은 내용이다: 복사본이 처음 스냅샷과 같아야 들인다(복사 도중 source 가 바뀌면 여기서 막힌다).
	copied, err := takeSnapshot(inc, before.snap.sumsBytes)
	if err == nil {
		err = before.snap.equal(copied, true)
	}
	if err != nil {
		ops.RemoveAll(inc)
		ops.RemoveAll(inc + ".sha256")
		return did, fmt.Errorf("the copy in the workspace is not what was checked before the lock (%v); it was discarded and the collectors are unchanged", err)
	}
	more, err := installWS(ws, inc, ops)
	return append(did, more...), err
}
