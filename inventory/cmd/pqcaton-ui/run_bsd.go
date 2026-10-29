// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix && !linux && !darwin

package main

import "errors"

// 저장을 지원하는 플랫폼은 Linux와 macOS뿐이다. 그 밖의 유닉스는 컴파일만 되게 둔다.
func dirswap(string, string) error {
	return errors.New("swapping directories is not supported on this platform")
}

func procStart(int) (string, error) { return "", errors.New("not supported on this platform") }

func markedProcs(string) []int { return nil }
