// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// dirswap: 두 디렉터리를 한 번에 맞바꾼다(renameat2 RENAME_EXCHANGE). 중간 상태가 없다.
func dirswap(a, b string) error {
	return unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE)
}

// procStat: /proc/<pid>/stat에서 상태 · 부모 · 프로세스 묶음 · 시작 시각(부팅 뒤 클록 틱, 22번째 칸).
func procStat(pid int) (state string, ppid, pgid int, start string, err error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", 0, 0, "", err
	}
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return "", 0, 0, "", errors.New("unexpected /proc stat")
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 20 {
		return "", 0, 0, "", errors.New("unexpected /proc stat")
	}
	ppid, _ = strconv.Atoi(f[1])
	pgid, _ = strconv.Atoi(f[2])
	return f[0], ppid, pgid, f[19], nil
}

// procStart: 프로세스의 시작 시각. 같은 PID가 다른 프로세스에 다시 쓰였는지 가린다. 좀비는 끝난 것으로 본다.
func procStart(pid int) (string, error) {
	state, _, _, start, err := procStat(pid)
	if err != nil {
		return "", err
	}
	if state == "Z" || state == "X" {
		return "", errors.New("the process has ended")
	}
	return start, nil
}

// markedProcs: 이 실행의 프로세스와 그 시작 시각. 자기 자신과 좀비는 뺀다.
//
//   - 환경을 읽을 수 있으면 표지로 가린다.
//   - 같은 사용자인데 덤프를 막아(OpenSSH 클라이언트가 그렇다) 환경을 읽을 수 없으면, 부모가 이 실행의 프로세스이거나
//     프로세스 묶음이 이 실행의 묶음(표지 프로세스의 묶음과 rm.Groups)일 때 이 실행의 것으로 본다. 계보를 따라 넓힌다.
//
// **확인하지 못한 것을 「없음」으로 치지 않는다.** /proc을 읽지 못하거나 예상하지 못한 이유로 읽기에 실패하면 오류다
// (부르는 쪽이 「회수 필요」로 닫는다). 읽는 사이에 끝나 사라진 프로세스만 건너뛴다. 남의 프로세스는 건너뛴다.
func markedProcs(rm runMark) ([]markedProc, error) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("cannot list processes: %w", err)
	}
	want := []byte("\x00" + rm.Marker + "\x00")
	self, uid := os.Getpid(), os.Getuid()
	var all []procInfo
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		var st unix.Stat_t
		if err := unix.Stat("/proc/"+e.Name(), &st); err != nil {
			if gone(err) {
				continue
			}
			return nil, fmt.Errorf("cannot inspect process %d: %w", pid, err)
		}
		if int(st.Uid) != uid {
			continue
		}
		state, ppid, pgid, start, err := procStat(pid)
		if err != nil || state == "Z" || state == "X" {
			continue // 그 사이 끝났거나 좀비다
		}
		p := procInfo{markedProc: markedProc{PID: pid, Start: start}, ppid: ppid, pgid: pgid}
		env, err := os.ReadFile("/proc/" + e.Name() + "/environ")
		switch {
		case err == nil:
			p.marked = bytes.Contains(append([]byte{0}, env...), want)
		case errors.Is(err, fs.ErrPermission):
			p.unreadable = true
		case gone(err):
			continue
		default:
			return nil, fmt.Errorf("cannot read the environment of process %d: %w", pid, err)
		}
		all = append(all, p)
	}
	return belonging(all, rm.Groups), nil
}

// gone: 읽는 사이에 프로세스가 끝나 사라졌다.
func gone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, unix.ESRCH)
}

// signalProc: 찾을 때의 그 프로세스에만 신호를 보낸다. pidfd를 먼저 열고(이후 그 PID가 다른 프로세스에 다시 쓰여도 fd는
// 원래 것을 가리킨다), 그 뒤 시작 시각이 찾을 때와 같은지 본다. 다르면 원래 것은 이미 끝났으므로 아무것도 하지 않는다.
// pidfd가 없는 커널에서는 시작 시각을 견준 뒤 PID로 보낸다.
func signalProc(p markedProc, force bool) error {
	sig := unix.SIGTERM
	if force {
		sig = unix.SIGKILL
	}
	fd, err := unix.PidfdOpen(p.PID, 0)
	if err != nil {
		if errors.Is(err, unix.ESRCH) {
			return nil
		}
		if cur, err := procStart(p.PID); err != nil || cur != p.Start {
			return nil
		}
		return unix.Kill(p.PID, sig)
	}
	defer unix.Close(fd)
	if cur, err := procStart(p.PID); err != nil || cur != p.Start {
		return nil
	}
	err = unix.PidfdSendSignal(fd, sig, nil, 0)
	if errors.Is(err, unix.ESRCH) {
		return nil
	}
	return err
}
