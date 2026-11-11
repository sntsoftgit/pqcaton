// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// Command releasecheck — 발행 관문(release-check.sh · release-verify.sh)이 부르는 도구.
//
// 순서와 기록은 스크립트가 맡고, 여기는 셸로 하면 틀리기 쉬운 일만 한다.
//
//   - zip: 커밋을 go 명령의 규칙 그대로 모듈 zip 으로 묶고 h1 해시를 찍는다(zip.CreateFromVCS).
//     프록시가 태그에서 zip 을 만드는 방식과 같아, 작업 트리에만 있는 파일은 들어가지 않는다.
//     Windows 예약 이름 · 대소문자 충돌 · 크기 한도는 여기서 걸린다. v0.26.2가 이것으로 막혔다.
//   - hashzip · hashmod: 받은 zip 과 go.mod 의 h1 해시. 발행 뒤 프록시 · 체크섬 DB 와 견줄 때 쓴다.
//   - commands: 문서에 공개한 `go install` 명령을 뽑고, 판이 맞는지 본다.
//   - notes: 릴리스 노트 맨 위 절의 판 · 날짜 · Apache-2.0 전환일을 본다.
//   - info · escape: 프록시 .info 확인과 모듈 경로 이스케이프.
//
// 도구는 별도 모듈이다. x/mod 와 govulncheck 를 위해 본 모듈의 go.mod 에 의존성을 늘리지 않는다.
// govulncheck 판은 이 모듈의 go.mod(tool 지시어)가 고정한다.
//
// 출력은 영어다(코드와 그 출력은 영어). 노트의 한국어 표지는 스크립트가 인자로 넘긴다.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "zip":
		err = cmdZip(os.Args[2:])
	case "hashzip":
		err = cmdHashZip(os.Args[2:])
	case "hashmod":
		err = cmdHashMod(os.Args[2:])
	case "commands":
		err = cmdCommands(os.Args[2:])
	case "notes":
		err = cmdNotes(os.Args[2:])
	case "info":
		err = cmdInfo(os.Args[2:])
	case "escape":
		err = cmdEscape(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: releasecheck zip|hashzip|hashmod|commands|notes|info|escape [flags]")
	os.Exit(2)
}
