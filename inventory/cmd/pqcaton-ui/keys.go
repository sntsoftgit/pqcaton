// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 승인 키의 자리. **보안에 민감한 변경이라 규칙을 여기 한곳에 둔다.**
//
// 서명용 개인 키(approval.key)와 확인용 공개 키(approval.pub)는 각각 다음 순서로 정한다.
//
//  1. 명시한 경로(-approval-key · -approval-verify-key)가 있으면 그것. 잘못됐으면 화면을 시작하지 않는다(운영자가 그 파일을
//     쓰라고 했으므로 조용히 다른 모드로 열지 않는다).
//  2. 없고 -no-default-approval-key 면 찾지 않는다. 명시 경로와 함께 줘도 기본 검색만 끈다.
//  3. 둘 다 없으면 os.UserConfigDir()/pqcaton/ 의 그 이름. 없으면 정상 시작하고 새 서명만 하지 않는다. 있는데 잘못됐으면 정상
//     시작하되 06 과 표준 오류에 분명히 알리고 새 서명을 막는다.
//
// 기본 파일을 만들거나 권한을 고치지 않는다. 개인 키 내용은 화면에도 로그에도 남기지 않는다. 화면에는 기본 위치를 사용자 설정
// 폴더 기준의 상대 위치로만 보이고, 로그에는 해석한 전체 경로와 안전한 이유 한 줄만 남긴다.
//
// 확인용 공개 키를 따로 두는 까닭: 06 은 서명된 계획의 승인을 공개 키로 확인한다. 공개 키를 개인 키 파일에서만 얻으면 개인 키가
// 없을 때 이미 서명된 계획의 생성까지 막힌다. pqcota-keygen 의 PQCOTA_VERIFY_KEY 줄만 담은 파일이면 개인 키 없이도 확인한다.

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

const (
	defaultKeyName    = "approval.key"
	defaultVerifyName = "approval.pub"
)

// keyPlace: 키 하나의 자리. path 가 비면 찾지 않는다(자동 검색을 껐거나 사용자 설정 폴더를 알 수 없다).
type keyPlace struct {
	path   string
	source ui.KeySource
	where  string // 화면에 보일 자리
}

// resolveKeys: 개인 키와 확인용 공개 키의 자리를 함께 정한다. **우선순위는 명시 개인 키 → 명시 공개 키 → 기본 개인 키 → 기본
// 공개 키이고, 명시한 것이 하나라도 있으면 기본 위치는 둘 다 읽지 않는다.** 명시 키와 기본 키가 섞이면 어느 쪽이 서명하고 어느
// 쪽이 확인하는지 운영자가 알 수 없기 때문이다. 명시한 개인 키만 있으면 그 키에서 공개 키를 유도하고, 명시한 공개 키만 있으면
// 확인 · 생성만 한다. -no-default-approval-key 는 기본 위치 둘 다를 끈다.
func resolveKeys(key, verify string, noDefault bool, userConfigDir func() (string, error)) (keyPlace, keyPlace) {
	if key != "" || verify != "" {
		return explicitOrOther(key), explicitOrOther(verify)
	}
	return resolveKey("", noDefault, defaultKeyName, userConfigDir), resolveKey("", noDefault, defaultVerifyName, userConfigDir)
}

// explicitOrOther: 명시한 자리, 없으면 「다른 하나를 명시해서 찾지 않음」.
func explicitOrOther(p string) keyPlace {
	if p == "" {
		return keyPlace{source: ui.KeyOtherGiven}
	}
	p = absPath(p)
	return keyPlace{path: p, source: ui.KeyExplicit, where: p}
}

// resolveKey: 키 하나의 자리. 자동 검색 끔 · 기본 위치. userConfigDir 은 시험이 바꾼다.
func resolveKey(explicit string, noDefault bool, name string, userConfigDir func() (string, error)) keyPlace {
	if explicit != "" {
		return explicitOrOther(explicit)
	}
	if noDefault {
		return keyPlace{source: ui.KeySearchOff}
	}
	rel := ui.KeyDefaultFolder + "/" + name
	dir, err := userConfigDir()
	if err != nil || dir == "" {
		return keyPlace{source: ui.KeyDefault, where: rel}
	}
	return keyPlace{path: filepath.Join(dir, ui.KeyDefaultFolder, name), source: ui.KeyDefault, where: rel}
}

// fileProblem: 키 파일 자체의 점검(링크 · 종류 · 작업 폴더 밖 · 소유자 · 하드 링크). 없으면 KeyMissing 이다.
func (s *server) fileProblem(p string) (fs.FileInfo, ui.KeyState) {
	fi, err := os.Lstat(p)
	switch {
	case err != nil && os.IsNotExist(err):
		return nil, ui.KeyMissing
	case err != nil:
		return nil, ui.KeyUnreadable
	case fi.Mode()&fs.ModeSymlink != 0:
		return nil, ui.KeyLink
	case !fi.Mode().IsRegular():
		return nil, ui.KeyUnreadable
	}
	if home := s.keyHome(); home != "" && inside(home, p) {
		return nil, ui.KeyInside
	}
	if !ownedByMe(fi) {
		return nil, ui.KeyOwner
	}
	if links(fi) > 1 {
		return nil, ui.KeyLinks
	}
	return fi, ui.KeyOK
}

// readVerify: 확인용 공개 키를 점검한다. 다른 사용자가 고칠 수 있으면 거부하고(읽기는 허용), 개인 키 줄이 들어 있으면 받지 않는다.
// verifyChosen: 확인용 공개 키를 고른 상태인가(파일이 있거나 명시했다). 없음 · 끔 · 지정 안 함은 고르지 않은 것이다.
func verifyChosen(st ui.KeyState) bool {
	return st != ui.KeyMissing && st != ui.KeyDisabled && st != ui.KeyNotGiven
}

func (s *server) readVerify() ui.KeyCheck {
	k := ui.KeyCheck{Path: s.verifyKey, Source: s.verifySource, Where: s.verifyWhere}
	if s.verifyKey == "" {
		k.State = ui.KeyMissing
		if s.verifySource == ui.KeySearchOff {
			k.State = ui.KeyDisabled
		}
		return k
	}
	// 기본 위치에 없는 공개 키는 고르지 않은 것이다(KeyMissing). 명시한 파일이 없으면 읽을 수 없는 것이다.
	fi, st := s.fileProblem(s.verifyKey)
	if st != ui.KeyOK {
		k.State = st
		return k
	}
	if fi.Mode().Perm()&0o022 != 0 {
		k.State = ui.KeyUnwritable
		return k
	}
	b, err := os.ReadFile(s.verifyKey)
	if err != nil {
		k.State = ui.KeyUnreadable
		return k
	}
	if strings.Contains(string(b), "PQCOTA_SIGN_KEY=") {
		k.State = ui.KeyHasPrivate
		return k
	}
	pub := publicKeyIn(b)
	raw, err := base64.StdEncoding.DecodeString(pub)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		k.State = ui.KeyBad
		return k
	}
	k.State, k.Public, k.Fingerprint = ui.KeyOK, pub, keyFingerprint(pub)
	return k
}

// publicKeyIn: PQCOTA_VERIFY_KEY 줄이면 그 값, 아니면 주석과 빈 줄을 뺀 첫 줄.
func publicKeyIn(b []byte) string {
	var first string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "PQCOTA_VERIFY_KEY="); ok {
			return strings.TrimSpace(v)
		}
		if first == "" && line != "" && !strings.HasPrefix(line, "#") {
			first = line
		}
	}
	return first
}

// checkKeysAtStart: 명시한 키가 잘못됐으면 오류를 돌려준다(시작하지 않는다). 기본 위치의 키는 한 줄씩 알리기만 한다.
// 개인 키 내용은 적지 않는다. 경로와 상태 코드만 적는다.
func (s *server) checkKeysAtStart(stderr io.Writer) error {
	key, _ := s.readKey()
	verify := s.readVerify()
	for _, c := range []struct {
		what string
		k    ui.KeyCheck
	}{{"approval key", key}, {"approval verification key", verify}} {
		switch {
		case c.k.Source == ui.KeyExplicit && c.k.State != ui.KeyOK && c.k.State != ui.KeyMismatch:
			return fmt.Errorf("the %s %s cannot be used (%s); nothing was started", c.what, c.k.Path, c.k.State)
		case c.k.Source == ui.KeyDefault && c.k.State == ui.KeyOK:
			fmt.Fprintf(stderr, "%s: %s (default location)\n", c.what, c.k.Path)
		case c.k.Source == ui.KeyDefault && c.k.State == ui.KeyMissing && c.k.Path != "":
			fmt.Fprintf(stderr, "%s: none at %s (default location)\n", c.what, c.k.Path)
		case c.k.Source == ui.KeyDefault && c.k.State != ui.KeyMissing:
			fmt.Fprintf(stderr, "warning: %s at %s (default location) cannot be used (%s); new approval signatures are blocked\n",
				c.what, c.k.Path, c.k.State)
		}
	}
	switch {
	case key.State == ui.KeyOK && verify.State == ui.KeyOK && key.Public != verify.Public:
		// 둘 다 명시했으면 운영자가 고른 짝이 틀린 것이다. 시작하지 않는다.
		if key.Source == ui.KeyExplicit && verify.Source == ui.KeyExplicit {
			return fmt.Errorf("the approval key %s and the verification key %s are different keys; nothing was started", key.Path, verify.Path)
		}
		fmt.Fprintf(stderr, "warning: the approval key %s and the verification key %s are different keys; signing and checking are blocked\n",
			key.Path, verify.Path)
	case key.State == ui.KeyOK && verifyChosen(verify.State) && verify.State != ui.KeyOK:
		fmt.Fprintf(stderr, "warning: the verification key %s cannot be used (%s) next to a usable approval key; signing and generation are blocked\n",
			verify.Path, verify.State)
	}
	return nil
}
