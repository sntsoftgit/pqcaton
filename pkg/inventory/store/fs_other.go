// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build !linux && !darwin

package store

import "fmt"

// platformCheck: Linux와 macOS 밖에서는 바꾸는 일을 하지 않는다. 화면은 읽기만 한다.
func platformCheck(func(string)) error {
	return fmt.Errorf("%w: saving runs on Linux and Intel macOS (verified) and Apple Silicon macOS (not verified); this platform can only view", ErrUnsupported)
}

func fsCheck(string, func(string)) error { return nil }
