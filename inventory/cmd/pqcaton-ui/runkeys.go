// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 03 관측의 임시 서명 키와 노드 점검, 결과 서명 검증.
//
// **키는 모든 노드의 점검이 통과한 뒤에만 만든다.** 감독이 키 없는 점검 플레이북(check.yml)으로 노드마다 `LC_ALL=C sudo -n -l`을
// 모으고 classifySudo로 판정한다. 하나라도 거절 · 확인 불가면 키를 만들지 않고 관측을 시작하지 않는다.
//
// **임시 키는 작업 폴더 밖에 둔다.** 실행 자리(observations/<id>/)는 작업 폴더 안이라 백업이나 실행 기록 수집에 섞일 수 있다.
// 자리는 XDG_RUNTIME_DIR(이 사용자만 들어가는 폴더일 때) 아니면 시스템 임시 폴더 아래 pqcaton-keys-<실행 ID>이고 0700이다.
// 키 파일은 처음부터 0600으로 만든다(O_EXCL). 자리를 실행 자리의 기록(run-keys.json, 경로만 담는다)에 먼저 확정해 쓴 뒤에 만든다.
// 그래서 비정상 종료 뒤에도 다음 시작이 그 기록으로 찾아 지운다.
//
// **키 값은 감독 · Ansible · ssh의 환경과 명령줄에 넣지 않는다.** 플레이북에는 run-keys.json의 경로만 추가 변수로 넘기고, 내장
// 조회 플러그인(ansible.builtin.file)이 읽어 모듈의 stdin으로 넘긴다.
//
// **지우지 못하면 무시하지 않는다.** 정상 종료 · 실패 · 취소 · 화면 없이 마무리 · 재시작 복구 어디서든 지우고, 지우지 못하면 실행
// 기록에 KeysLeft로 남겨 「회수 필요」로 보이고 새 관측을 막는다. 프로세스가 비정상으로 끝난 뒤 다음 시작까지는 파일이 남을 수 있다.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/sign"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

const (
	runKeysVars   = "run-keys.json"
	runKeyPrefix  = "pqcaton-keys-"
	runCheckDir   = "check"
	exitNotSigned = 6 // 감독: 노드 점검이나 키 준비가 막아 관측을 시작하지 않았다
)

// runKeysRecord: 실행 자리의 run-keys.json. 플레이북의 추가 변수이자 회수의 기록이다. 경로만 담는다.
type runKeysRecord struct {
	Dir  string            `json:"pqcaton_key_dir"`
	Keys map[string]string `json:"pqcaton_keys"`
}

// runKeyBase: 임시 키 폴더를 만들 자리. 시험이 바꾼다.
var runKeyBase = func() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() && ownedByMe(fi) && fi.Mode().Perm()&0o077 == 0 {
			return d
		}
	}
	return os.TempDir()
}

// makeRunKeys: 실행의 서명 키를 임시 폴더에 둔다(감독이 점검 통과 뒤 부른다). 원본 키 파일은 다시 점검한다. 돌려주는 까닭에는
// 키 값을 싣지 않는다.
func makeRunKeys(dir string, m runManifest) (string, error) {
	keyDir := filepath.Join(runKeyBase(), runKeyPrefix+m.ID)
	if m.KeyHome != "" && inside(m.KeyHome, keyDir) {
		return "", errors.New("the temporary key folder would be inside the workspace")
	}
	ids := make([]string, 0, len(m.CollectorKeys))
	for id := range m.CollectorKeys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	rec := runKeysRecord{Dir: keyDir, Keys: map[string]string{}}
	for _, id := range ids {
		rec.Keys[id] = filepath.Join(keyDir, id)
	}
	// 기록부터 확정한다. 폴더를 만든 뒤 죽어도 다음 시작이 이 기록으로 찾는다.
	if err := writeDurable(filepath.Join(dir, runKeysVars), rec); err != nil {
		return "", fmt.Errorf("the key record could not be written (%s)", writeFailure(err))
	}
	if err := os.Mkdir(keyDir, 0o700); err != nil {
		return "", fmt.Errorf("the temporary key folder could not be made (%s)", writeFailure(err))
	}
	if fi, err := os.Lstat(keyDir); err != nil || !fi.IsDir() || fi.Mode()&fs.ModeSymlink != 0 || !ownedByMe(fi) || fi.Mode().Perm() != 0o700 {
		return "", errors.New("the temporary key folder is not a folder only this user can open")
	}
	for _, id := range ids {
		src := m.CollectorKeys[id]
		k := readCollectorKey(m.KeyHome, filepath.Dir(src), id)
		if k.State != ui.KeyOK || k.Path != src {
			return "", fmt.Errorf("the signing key for %s cannot be used (%s)", id, k.State)
		}
		f, err := os.OpenFile(rec.Keys[id], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return "", fmt.Errorf("a temporary key file could not be made (%s)", writeFailure(err))
		}
		_, werr := f.WriteString(k.Private)
		serr := f.Sync()
		cerr := f.Close()
		if werr != nil || serr != nil || cerr != nil {
			return "", errors.New("a temporary key file could not be written")
		}
	}
	return keyDir, nil
}

// removeRunKeys: 실행의 임시 키를 지운다. 기록이 없으면 만든 적이 없다. 지우지 못하면 그 까닭(경로는 싣고 키는 싣지 않는다).
// 기록이 가리키는 폴더가 pqcaton이 만든 이름이 아니면 건드리지 않고 까닭을 돌려준다.
func removeRunKeys(dir string) string {
	p := filepath.Join(dir, runKeysVars)
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	var rec runKeysRecord
	if err != nil || json.Unmarshal(b, &rec) != nil || rec.Dir == "" {
		return "the record of the temporary signing keys (" + p + ") cannot be read; remove the folder it names yourself, then this record"
	}
	if !strings.HasPrefix(filepath.Base(rec.Dir), runKeyPrefix) || !filepath.IsAbs(rec.Dir) {
		return "the recorded key folder " + rec.Dir + " is not one pqcaton makes, so it was left as it is"
	}
	if err := removeAll(rec.Dir); err != nil {
		return "the temporary signing keys could not be removed from " + rec.Dir + " (" + writeFailure(err) + ")"
	}
	if _, err := os.Lstat(rec.Dir); !errors.Is(err, fs.ErrNotExist) {
		return "the temporary signing keys are still in " + rec.Dir
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "the record of the temporary signing keys could not be removed (" + writeFailure(err) + ")"
	}
	return ""
}

// removeAll: 임시 키 폴더를 지우는 함수. 시험이 실패를 넣는다.
var removeAll = os.RemoveAll

// judgeChecks: 점검 플레이북이 남긴 노드마다의 `sudo -n -l` 결과를 판정하고 줄마다 say로 알린다. 모두 통과면 true.
func judgeChecks(dir string, nodes []runSlot, say func(string, ...any)) bool {
	all := true
	for _, sl := range nodes {
		b, err := os.ReadFile(filepath.Join(dir, runCheckDir, sl.Alias+".txt"))
		if err != nil {
			all = false
			say("pqcaton: node %s: the sudo check gave no answer (the node may be unreachable), so no signing key was made", sl.Node)
			continue
		}
		uid, rest, _ := strings.Cut(string(b), "\n")
		if strings.TrimSpace(uid) == "uid=0" {
			// root로 접속한다: Ansible은 접속 계정과 권한 상승 대상이 같으면 권한 상승을 건너뛰므로(become_allow_same_user = False,
			// 이 실행의 설정이 고정한다) 키가 sudo를 거치지 않는다.
			say("pqcaton: node %s: passed: the node is reached as root, so the key does not go through sudo", sl.Node)
			continue
		}
		first, rest, _ := strings.Cut(rest, "\n")
		rc, err := strconv.Atoi(strings.TrimSpace(first))
		if err != nil {
			rc = -1
		}
		c := classifySudo(rc, rest)
		switch c.Verdict {
		case sudoPass:
			say("pqcaton: node %s: sudo check passed: %s", sl.Node, c.Why)
		case sudoRefuse:
			all = false
			say("pqcaton: node %s: refused: %s", sl.Node, c.Why)
		default:
			all = false
			say("pqcaton: node %s: cannot be confirmed: %s", sl.Node, c.Why)
		}
		for _, l := range c.Lines {
			say("pqcaton:     sudo policy: %s", l)
		}
		if c.Verdict != sudoPass {
			say("pqcaton:     check it on the node with: LC_ALL=C sudo -n -l   (as the account the node is reached with)")
		}
	}
	return all
}

// signatureProblem: 결과 하나의 서명 검증(상류 sign.VerifyBound, 적재 `pqcota-ingest`와 같은 판정). 통과면 빈 값과 맞은 키의 지문.
// 까닭은 넷으로 가른다. 「다른 ID의 키」는 다른 ID에 등록된 공개 키로 검증이 **성공**했을 때만이다. 변조와 등록되지 않은 키는 서명만으로
// 가를 수 없어 넷째 까닭이 두 가능성을 함께 말한다. 키 값은 싣지 않는다.
func signatureProblem(keys map[string][]string, r *discoveryv1.CollectionResult) (string, string) {
	id := r.GetEnvelope().GetCollectorId()
	err := sign.VerifyBound(keys, r)
	if err == nil {
		k, _ := sign.MatchingKey(keys[id], r)
		return "", sign.Fingerprint(k)
	}
	if errors.Is(err, sign.ErrNoSignature) {
		return "not signed", ""
	}
	if len(keys[id]) == 0 {
		return "no trusted public key is listed for " + id, ""
	}
	var others []string
	for other, ks := range keys {
		if other == id {
			continue
		}
		if _, ok := sign.MatchingKey(ks, r); ok {
			others = append(others, other)
		}
	}
	if len(others) > 0 {
		sort.Strings(others)
		return "signed with the key listed for " + strings.Join(others, ", ") + ", not " + id, ""
	}
	return "the signature does not match the keys listed for " + id + " (the result was changed after signing, or it was signed with a key that is not listed)", ""
}
