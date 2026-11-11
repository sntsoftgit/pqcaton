// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build !linux && !darwin

package main

import "errors"

// 저장 · 관측을 지원하지 않는 플랫폼에서는 「회수 필요」가 생기지 않는다. 읽으려 하면 조회 실패로 보인다.
func readProc(int) (procFacts, error) {
	return procFacts{}, errors.New("not supported on this platform")
}

func procGone(error) bool { return false }

func procDenied(error) bool { return false }
