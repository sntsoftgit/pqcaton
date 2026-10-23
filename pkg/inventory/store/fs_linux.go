// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"fmt"
	"syscall"
)

// platformCheck: Linux가 상용 지원 플랫폼이다.
func platformCheck(func(string)) error { return nil }

// fsCheck: 네트워크 파일시스템이면 시작하지 않는다. 잠금과 rename의 원자성을 믿을 수 없다.
func fsCheck(dir string, _ func(string)) error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return err
	}
	if name := remoteLinuxFS(uint32(st.Type)); name != "" {
		return fmt.Errorf("%w: %s is on %s; keep the session and the state directory on a local file system", ErrUnsupported, dir, name)
	}
	return nil
}
