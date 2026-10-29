// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"os"
	"testing"
)

// TestMain: 화면은 자기 바이너리를 감독 모드로 띄운다. 시험에서 그 바이너리는 이 시험 바이너리라, 감독 모드로 불리면
// 시험 대신 감독을 돈다.
func TestMain(m *testing.M) {
	if len(os.Args) == 4 && os.Args[1] == superviseFlag {
		os.Exit(supervise(os.Args[2], os.Args[3]))
	}
	os.Exit(m.Run())
}
