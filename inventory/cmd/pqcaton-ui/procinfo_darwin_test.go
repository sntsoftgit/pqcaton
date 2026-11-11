// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"encoding/binary"
	"strings"
	"testing"
)

// IC-U190 — **macOS 의 kern.procargs2 응답에서 실행 파일과 인자 argc 개만 읽고, 뒤에 붙은 환경은 돌려주지 않는다.** 실제
// 프로세스 조회는 v0.27.0 최종 검증(Intel Mac)에서 본다.
func TestProcArgsStopsBeforeTheEnvironment(t *testing.T) {
	var buf []byte
	buf = binary.LittleEndian.AppendUint32(buf, 3)
	buf = append(buf, "/bin/sh\x00\x00\x00\x00"...)
	buf = append(buf, "sh\x00-c\x00sleep 30\x00"...)
	buf = append(buf, "PQCATON_TEST_SECRET=envsecretvalue\x00HOME=/Users/x\x00"...)
	exe, args, ok := procArgs(buf)
	if !ok || exe != "/bin/sh" || strings.Join(args, "|") != "sh|-c|sleep 30" {
		t.Fatalf("procArgs = %q %q %v", exe, args, ok)
	}
	if strings.Contains(exe+strings.Join(args, ""), "envsecretvalue") {
		t.Fatal("the environment leaked into the arguments")
	}
	if _, _, ok := procArgs(buf[:10]); ok {
		t.Fatal("a cut-off response must not be read as arguments")
	}
}
