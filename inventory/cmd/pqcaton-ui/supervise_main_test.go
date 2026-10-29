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
	if len(os.Args) == 2 && testModes[os.Args[1]] != nil {
		testModes[os.Args[1]]()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// testModes: 시험이 이 시험 바이너리를 다른 모양의 프로세스로 띄울 때 쓰는 모드(플랫폼별 시험 파일이 채운다).
var testModes = map[string]func(){}
