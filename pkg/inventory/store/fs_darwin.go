// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"fmt"
	"syscall"
)

// platformCheck: macOS는 돌지만 검증하지 않았다.
func platformCheck(warn func(string)) error {
	warn("macOS is not a verified platform for saving; Linux on a local file system is")
	return nil
}

// fsCheck: 네트워크 파일시스템이면 시작하지 않는다.
func fsCheck(dir string, _ func(string)) error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return err
	}
	b := make([]byte, 0, len(st.Fstypename))
	for _, c := range st.Fstypename {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	if name := remoteDarwinFS(string(b)); name != "" {
		return fmt.Errorf("%w: %s is on %s; keep the session and the state directory on a local file system", ErrUnsupported, dir, name)
	}
	return nil
}
