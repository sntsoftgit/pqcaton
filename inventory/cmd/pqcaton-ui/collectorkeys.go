// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 03 관측의 수집기 서명 키(pqcota v0.15.0의 수집기 ID 결합).
//
// **키는 수집기 ID마다 하나다.** 결과에 적히는 ID는 명령마다 정해져 있다(pqcota-nodescan → openssl-collector,
// pqcota-netcap → network-collector, pqcota-jvmscan → jvm-collector). 같은 ID의 수집기를 여러 노드에서 돌리면 모든 노드가 그 키를
// 함께 쓴다. 상류는 아직 키를 노드에 묶지 않으므로, 그 키를 가진 쪽은 그 ID로 어느 노드의 결과든 서명할 수 있다.
//
// **승인 키와 자리 · 용도를 나눈다.** 서명 키는 사용자 설정 폴더의 pqcaton/collector-keys/<ID>.key(`pqcota-keygen -collector <ID>`의
// 출력)이고, 신뢰할 공개 키는 같은 폴더의 verify.pub(적재에 주는 것과 같은 `PQCOTA_VERIFY_KEY=<ID>:<키>,…` 한 줄)다. 둘 다 작업
// 폴더 밖이다. 검증할 공개 키는 운영자가 둔 verify.pub에서만 가져오고, 개인 키에서 뽑아 쓰지 않는다.
// 설계: pqcaton-aai plans/2026-11-18-v0.31.0-pqcota-v0150-v0160-uptake.md 1 · 2절.

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/sign"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

const (
	collectorKeyFolder  = "collector-keys" // 사용자 설정 폴더의 pqcaton/ 안
	collectorVerifyName = "verify.pub"
	collectorKeyExt     = ".key"
)

// 03이 돌리는 수집기의 ID. 결과 파일의 종류(collectorOf)와 같은 이름이다.
const (
	idOpenSSL = "openssl-collector"
	idNetwork = "network-collector"
	idJVM     = "jvm-collector"
)

// neededCollectorIDs: 이번 관측이 서명 키를 넘길 수집기. Linux 노드는 OpenSSL과 네트워크, collector.jar가 있으면 JVM도.
func neededCollectorIDs(jar bool) []string {
	ids := []string{idOpenSSL, idNetwork}
	if jar {
		ids = append(ids, idJVM)
	}
	return ids
}

// collectorKeyPlaces: -collector-keys · -collector-verify-keys가 비면 사용자 설정 폴더의 기본 자리다.
func collectorKeyPlaces(dir, verify string, userConfigDir func() (string, error)) (string, string) {
	if dir == "" {
		if base, err := userConfigDir(); err == nil && base != "" {
			dir = filepath.Join(base, ui.KeyDefaultFolder, collectorKeyFolder)
		}
	}
	if verify == "" && dir != "" {
		verify = filepath.Join(dir, collectorVerifyName)
	}
	return absPath(dir), absPath(verify)
}

// collectorKey: 서명 키 하나의 점검 결과. State가 KeyOK가 아니면 Private · Public은 비어 있다.
type collectorKey struct {
	ID      string
	Path    string
	State   ui.KeyState
	Private string
	Public  string
}

// dirProblem: 서명 키 폴더의 점검. 링크가 아니고, 이 사용자의 것이고, 소유자만 들어가며, 작업 폴더 밖이어야 한다.
func dirProblem(home, dir string) ui.KeyState {
	if dir == "" {
		return ui.KeyNotGiven
	}
	fi, err := os.Lstat(dir)
	switch {
	case err != nil && os.IsNotExist(err):
		return ui.KeyMissing
	case err != nil:
		return ui.KeyUnreadable
	case fi.Mode()&os.ModeSymlink != 0:
		return ui.KeyLink
	case !fi.IsDir():
		return ui.KeyUnreadable
	case home != "" && inside(home, dir):
		return ui.KeyInside
	case !ownedByMe(fi):
		return ui.KeyOwner
	case fi.Mode().Perm()&0o077 != 0:
		return ui.KeyOpen
	}
	return ui.KeyOK
}

// readCollectorKey: <dir>/<id>.key를 읽어 점검한다. 파일 규칙은 승인 키와 같다(링크 · 작업 폴더 안 · 소유자 · 하드 링크 · 0600).
// 내용은 `pqcota-keygen -collector <id>`의 출력이어야 한다: 개인 키 줄과, 같은 ID에 묶인 검증 줄이 있고 둘이 같은 키쌍이다.
// 플래그 없이 만든 키(승인 키 꼴)는 받지 않는다.
func readCollectorKey(home, dir, id string) collectorKey {
	k := collectorKey{ID: id, Path: filepath.Join(dir, id+collectorKeyExt)}
	if st := dirProblem(home, dir); st != ui.KeyOK {
		k.State = st
		return k
	}
	fi, st := fileProblemIn(home, k.Path)
	if st != ui.KeyOK {
		k.State = st
		return k
	}
	if fi.Mode().Perm()&0o077 != 0 {
		k.State = ui.KeyOpen
		return k
	}
	b, err := os.ReadFile(k.Path)
	if err != nil {
		k.State = ui.KeyUnreadable
		return k
	}
	line := publicKeyIn(b)
	gotID, pub, bound := strings.Cut(line, ":")
	switch {
	case !bound:
		k.State = ui.KeyNotCollector
		return k
	case strings.TrimSpace(gotID) != id:
		k.State = ui.KeyOtherCollector
		return k
	}
	raw, err := base64.StdEncoding.DecodeString(privateKeyIn(b))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		k.State = ui.KeyBad
		return k
	}
	derived := base64.StdEncoding.EncodeToString(ed25519.PrivateKey(raw).Public().(ed25519.PublicKey))
	if strings.TrimSpace(pub) != derived {
		k.State = ui.KeyMismatch
		return k
	}
	k.State, k.Private, k.Public = ui.KeyOK, base64.StdEncoding.EncodeToString(raw), derived
	return k
}

// collectorVerify: 신뢰할 공개 키(verify.pub)의 점검 결과.
type collectorVerify struct {
	Path  string
	State ui.KeyState
	Keys  map[string][]string // 수집기 ID → 허용하는 공개 키들(상류 sign.ParseCollectorKeys)
}

// readCollectorVerify: verify.pub을 읽는다. 다른 사용자가 고칠 수 있으면 거부하고(읽기는 허용), 개인 키가 들어 있으면 받지 않는다.
// 값은 상류 적재(`pqcota-ingest`)와 같은 파서로 읽고 **결합 형식만** 받는다. 옛 키 목록 꼴은 어느 키가 어느 수집기의 것인지 말하지 않는다.
func readCollectorVerify(home, p string) collectorVerify {
	v := collectorVerify{Path: p}
	if p == "" {
		v.State = ui.KeyNotGiven
		return v
	}
	fi, st := fileProblemIn(home, p)
	if st != ui.KeyOK {
		v.State = st
		return v
	}
	if fi.Mode().Perm()&0o022 != 0 {
		v.State = ui.KeyUnwritable
		return v
	}
	b, err := os.ReadFile(p)
	if err != nil {
		v.State = ui.KeyUnreadable
		return v
	}
	if strings.Contains(string(b), "PQCOTA_SIGN_KEY=") {
		v.State = ui.KeyHasPrivate
		return v
	}
	bound, legacy, err := sign.ParseCollectorKeys(publicKeyIn(b))
	switch {
	case err != nil || (len(bound) == 0 && len(legacy) == 0):
		v.State = ui.KeyBad
	case len(legacy) > 0:
		v.State = ui.KeyLegacyList
	default:
		v.State, v.Keys = ui.KeyOK, bound
	}
	return v
}

// collectorKeyChecks: 03 시작 전 점검의 서명 키 두 줄. 서명 키(ID마다)와 신뢰할 공개 키가 맞물리는지 본다.
//
//   - 서명 키: 필요한 ID마다 키가 있고 쓸 수 있어야 한다. 빠진 ID와 까닭을 적는다.
//   - 신뢰할 공개 키: 결합 형식이고, 서명 키마다 그 공개 키가 **자기 ID 아래** 있어야 한다. 한 공개 키가 두 ID에 있으면 상류가
//     그 ID들을 구별하지 못하므로 막는다. 승인 공개 키와 같은 키를 서명 키로 쓰는 것도 막는다(용도를 나눈다).
func (s *server) collectorKeyChecks(ids []string) []ui.RunCheck {
	home := s.keyHome()
	var keys []collectorKey
	var bad []string
	for _, id := range ids {
		k := readCollectorKey(home, s.collectorKeys, id)
		keys = append(keys, k)
		if k.State != ui.KeyOK {
			bad = append(bad, id+": "+string(k.State))
		}
	}
	keyDetail := strings.Join(ids, " · ") + " (" + s.collectorKeys + ")"
	if len(bad) > 0 {
		keyDetail = strings.Join(bad, " · ") + " (" + s.collectorKeys + ")"
	}
	out := []ui.RunCheck{{Item: ui.RunCollectorKeys, OK: len(bad) == 0, Blocking: true, Detail: keyDetail}}

	v := readCollectorVerify(home, s.collectorVerify)
	var problems []string
	if v.State != ui.KeyOK {
		problems = append(problems, string(v.State))
	} else {
		owners := map[string][]string{} // 공개 키 → 그 키를 둔 ID들
		for id, ks := range v.Keys {
			for _, k := range ks {
				owners[k] = append(owners[k], id)
			}
		}
		var shared []string
		for _, ids := range owners {
			if len(ids) > 1 {
				sort.Strings(ids)
				shared = append(shared, strings.Join(ids, "+"))
			}
		}
		sort.Strings(shared)
		for _, sh := range shared {
			problems = append(problems, "one key under "+sh)
		}
		for _, k := range keys {
			if k.State != ui.KeyOK {
				continue
			}
			if !contains(v.Keys[k.ID], k.Public) {
				problems = append(problems, k.ID+": not listed")
			}
		}
		if ak, _ := s.readKey(); ak.State == ui.KeyOK {
			for _, k := range keys {
				if k.State == ui.KeyOK && k.Public == ak.Public {
					problems = append(problems, k.ID+": the approval key")
				}
			}
		}
	}
	detail := s.collectorVerify
	if len(problems) > 0 {
		detail = strings.Join(problems, " · ") + " (" + s.collectorVerify + ")"
	}
	return append(out, ui.RunCheck{Item: ui.RunCollectorVerify, OK: len(problems) == 0, Blocking: true, Detail: detail})
}

// runKeySources: 관측을 띄울 때 쓸 서명 키 파일(ID → 경로)과 그 개인 키(로그 가림용). 점검을 다시 하고, 하나라도 쓸 수 없으면 오류다.
func (s *server) runKeySources(ids []string) (map[string]string, []string, error) {
	paths := map[string]string{}
	var privs []string
	for _, id := range ids {
		k := readCollectorKey(s.keyHome(), s.collectorKeys, id)
		if k.State != ui.KeyOK {
			return nil, nil, fmt.Errorf("the signing key for %s cannot be used (%s)", id, k.State)
		}
		paths[id], privs = k.Path, append(privs, k.Private)
	}
	return paths, privs, nil
}
