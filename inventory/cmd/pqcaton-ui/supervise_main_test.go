// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/sign"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// TestMain: 화면은 자기 바이너리를 감독 모드로 띄운다. 시험에서 그 바이너리는 이 시험 바이너리라, 감독 모드로 불리면
// 시험 대신 감독을 돈다.
func TestMain(m *testing.M) {
	// 가짜 ansible-playbook은 FAKE_* 변수로 움직인다. 제품의 허용 목록에는 없으므로 시험 바이너리에서만 더한다. 임시 키 자리는
	// 감독(다른 프로세스)까지 닿도록 환경 변수로 바꾼다.
	envKeepPrefixes = append(envKeepPrefixes, "FAKE_")
	if base := os.Getenv("PQCATON_TEST_KEY_BASE"); base != "" {
		runKeyBase = func() string { return base }
	}
	if len(os.Args) == 4 && os.Args[1] == superviseFlag {
		beforeSupervise()
		os.Exit(supervise(os.Args[2], os.Args[3]))
	}
	if len(os.Args) >= 5 && os.Args[1] == "-test-sign" {
		os.Exit(testSign(os.Args[2], os.Args[3], os.Args[4], os.Args[5:]))
	}
	if len(os.Args) == 2 && testModes[os.Args[1]] != nil {
		testModes[os.Args[1]]()
		os.Exit(0)
	}
	// 화면은 pqcota 이력 없이 시작하지 않는다. 시험은 빈 메모리 이력으로 띄운다. 이력이 필요한 시험은 서버에 직접 건다
	// (useUpstream), 시작 조건을 재는 시험은 이 둘을 바꾼다.
	os.Setenv(upstream.DSNEnv, "memory://tests")
	openUpstream = func(dsn, org string) (upstream.Store, string, error) {
		if dsn == "" {
			return nil, "", upstream.ErrNoDSN
		}
		return memStore{history.NewMemStore()}, org, nil
	}
	os.Exit(m.Run())
}

// beforeSupervise: 감독으로 다시 뜬 시험 바이너리가 감독을 돌기 전에 부른다. 플랫폼별 시험 파일이 채운다(IC-U205).
var beforeSupervise = func() {}

// testModes: 시험이 이 시험 바이너리를 다른 모양의 프로세스로 띄울 때 쓰는 모드(플랫폼별 시험 파일이 더 채운다).
// -test-sleep은 오래 도는 프로세스다. /bin/sleep과 달리 플랫폼 바이너리가 아니라 macOS에서도 환경(표지)이 읽힌다.
var testModes = map[string]func(){"-test-sleep": func() { time.Sleep(300 * time.Second) }}

// testSign: 가짜 ansible-playbook이 수집기 대신 서명된 결과를 내게 하는 도우미(`-test-sign <키 파일> <수집기 ID> <노드> [tamper]`).
// 키 파일은 실행의 임시 키 파일(개인 키 한 줄)이다. tamper면 서명한 뒤 봉투의 판을 바꾼다(변조).
func testSign(keyFile, collector, node string, rest []string) int {
	priv, err := os.ReadFile(keyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "test-sign: cannot read the key file")
		return 2
	}
	r := &discoveryv1.CollectionResult{Envelope: &commonv1.Envelope{CollectorId: collector, CollectorVersion: "v0.16.0", TargetNodeId: node}}
	if err := sign.SignAll(string(priv), []*discoveryv1.CollectionResult{r}); err != nil {
		fmt.Fprintln(os.Stderr, "test-sign: cannot sign")
		return 2
	}
	if len(rest) > 0 && rest[0] == "tamper" {
		r.Envelope.CollectorVersion = "v9.9.9"
	}
	b, _ := protojson.Marshal(r)
	fmt.Println(string(b))
	return 0
}
