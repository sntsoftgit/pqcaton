// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// guidetool: 운영자 안내서의 명령을 돌리는 시험(guide_run_test.go)이 쓰는 도구. 제품 바이너리에 넣지 않는다.
//
//	guidetool seed  <작업 폴더>   선언(acme, 노드 web)과 관측 결과 하나를 둔다
//	guidetool stuck <작업 폴더>   세션 저장을 도중에 멈추고 세션 파일을 밖에서 고친다(복구 필요 상태)
//
// stuck은 셸이 읽을 수 있게 TARGET=… · PREPARED=… 두 줄을 찍는다. 안내서의 복구 절이 그 두 경로를 쓴다.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: guidetool seed|stuck <workspace>")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "seed":
		err = seed(os.Args[2])
	case "stuck":
		err = stuck(os.Args[2])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "guidetool:", err)
		os.Exit(1)
	}
}

func seed(ws string) error {
	if err := os.MkdirAll(filepath.Join(ws, "results"), 0o700); err != nil {
		return err
	}
	decl := `{"org":"acme","scope":["web"],"nodes":[{"name":"web","ips":["10.0.0.1"]}],"assets":[],"edges":[]}` + "\n"
	if err := os.WriteFile(filepath.Join(ws, "declaration.json"), []byte(decl), 0o600); err != nil {
		return err
	}
	cbom := `{"bomFormat":"CycloneDX","specVersion":"1.6","components":[{"type":"cryptographic-asset","name":"libcrypto",
"properties":[{"name":"pqcota:crypto_runtime","value":"openssl"},{"name":"pqcota:openssl.version","value":"3.0.2"}]}]}`
	b, err := protojson.Marshal(&discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{TargetNodeId: "web", CollectorId: "openssl-collector", CollectedAt: timestamppb.Now(),
			DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION},
		CbomCyclonedx: []byte(cbom), CyclonedxSpecVersion: "1.6",
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ws, "results", "web.json"), b, 0o600)
}

func stuck(ws string) error {
	target := filepath.Join(ws, "session.json")
	sf, err := review.Load(target)
	if err != nil {
		return err
	}
	sf.Reviewer = "guide"
	next, err := review.Encode(sf)
	if err != nil {
		return err
	}
	stop := errors.New("stopped on purpose")
	st, err := store.Open(filepath.Join(ws, store.DirName), store.Options{Fault: func(point, path string) error {
		if point == store.FaultApply && path == "0" {
			return stop
		}
		return nil
	}})
	if err != nil {
		return err
	}
	if err := st.AcquireRun(); err != nil {
		return err
	}
	id := store.Identity{Kind: "review", Org: "acme", SessionID: sf.SessionID}
	if err := st.Save(id, []store.Change{{Path: target, Content: next}}, time.Second); !errors.Is(err, stop) {
		return fmt.Errorf("the save did not stop where it should: %v", err)
	}
	rep, err := st.Status()
	if err != nil {
		return err
	}
	st.Close()
	if len(rep.Records) != 1 || len(rep.Records[0].Items) != 1 {
		return fmt.Errorf("unexpected records: %+v", rep.Records)
	}
	if err := os.WriteFile(target, []byte("edited by hand\n"), 0o600); err != nil {
		return err
	}
	it := rep.Records[0].Items[0]
	fmt.Printf("TARGET=%s\nPREPARED=%s\n", it.AbsPath, it.AbsPrepared)
	return nil
}
