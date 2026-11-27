// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// collectorKeyDir: 수집기 서명 키 폴더(0700, 작업 폴더 밖)에 ID마다 pqcota-keygen -collector의 출력을 두고, 그 공개 키들을
// 결합 형식으로 적은 신뢰할 공개 키 파일을 둔다. 폴더와 그 파일의 경로를 돌려준다.
func collectorKeyDir(t *testing.T, ids ...string) (string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "collector-keys")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var line []string
	for _, id := range ids {
		text, pub := keygenV016(t, id)
		if err := os.WriteFile(filepath.Join(dir, id+collectorKeyExt), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		line = append(line, id+":"+pub)
	}
	verify := filepath.Join(dir, collectorVerifyName)
	if err := os.WriteFile(verify, []byte("PQCOTA_VERIFY_KEY="+strings.Join(line, ",")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, verify
}

func checkOf(cs []ui.RunCheck, item ui.RunItem) *ui.RunCheck {
	for i := range cs {
		if cs[i].Item == item {
			return &cs[i]
		}
	}
	return nil
}

// IC-U210 — **02는 수집기 ID별 서명 키와 신뢰할 공개 키가 맞물려야 시작한다.** 필요한 ID(OpenSSL · 네트워크, collector.jar가 있으면
// JVM)마다 `pqcota-keygen -collector <ID>`의 출력이 소유자 전용 파일로 작업 폴더 밖 0700 폴더에 있어야 한다. 빠진 ID, 다른 ID의 키,
// 수집기 ID가 없는 키(승인 키 꼴), 다른 사용자가 읽을 수 있는 키, 넓게 열린 폴더는 「서명 키」에서 막고 ID와 까닭을 적는다. 신뢰할
// 공개 키는 결합 형식이어야 하고(옛 목록 꼴 · 개인 키가 든 파일은 막는다), 서명 키마다 그 공개 키가 자기 ID 아래 있어야 하며, 한
// 공개 키를 두 ID에 두거나 승인 키를 서명 키로 쓰면 막는다. 다 맞으면 두 줄 모두 통과다.
func TestCollectorKeysMustMatchTheTrustedKeys(t *testing.T) {
	s := everyScreen(t)
	ids := neededCollectorIDs(false)
	fresh := func() {
		s.collectorKeys, s.collectorVerify = collectorKeyDir(t, ids...)
	}
	keys := func() (*ui.RunCheck, *ui.RunCheck) {
		cs := s.collectorKeyChecks(ids)
		return checkOf(cs, ui.RunCollectorKeys), checkOf(cs, ui.RunCollectorVerify)
	}
	fresh()
	if k, v := keys(); !k.OK || !v.OK {
		t.Fatalf("matching keys: keys %+v, verify %+v", k, v)
	}
	for _, c := range []struct {
		name   string
		change func()
		keyBad string // 서명 키 줄의 Detail에 나와야 하는 것(비면 통과해야 한다)
		verBad string
	}{
		{"a missing ID", func() { os.Remove(filepath.Join(s.collectorKeys, idNetwork+collectorKeyExt)) }, idNetwork + ": missing", ""},
		{"a key of another ID", func() {
			text, _ := keygenV016(t, idOpenSSL)
			os.WriteFile(filepath.Join(s.collectorKeys, idNetwork+collectorKeyExt), []byte(text), 0o600)
		}, idNetwork + ": " + string(ui.KeyOtherCollector), ""},
		{"an approval-style key", func() {
			text, _ := keygenV016(t, "")
			os.WriteFile(filepath.Join(s.collectorKeys, idOpenSSL+collectorKeyExt), []byte(text), 0o600)
		}, idOpenSSL + ": " + string(ui.KeyNotCollector), ""},
		{"a key others can read", func() { os.Chmod(filepath.Join(s.collectorKeys, idOpenSSL+collectorKeyExt), 0o644) }, idOpenSSL + ": " + string(ui.KeyOpen), ""},
		{"an open folder", func() { os.Chmod(s.collectorKeys, 0o755) }, idOpenSSL + ": " + string(ui.KeyOpen), ""},
		{"the legacy list", func() {
			b, _ := os.ReadFile(s.collectorVerify)
			line := strings.TrimPrefix(strings.TrimSpace(string(b)), "PQCOTA_VERIFY_KEY=")
			var bare []string
			for _, e := range strings.Split(line, ",") {
				_, k, _ := strings.Cut(e, ":")
				bare = append(bare, k)
			}
			os.WriteFile(s.collectorVerify, []byte("PQCOTA_VERIFY_KEY="+strings.Join(bare, ",")+"\n"), 0o644)
		}, "", string(ui.KeyLegacyList)},
		{"a private key in the trusted file", func() {
			b, _ := os.ReadFile(filepath.Join(s.collectorKeys, idOpenSSL+collectorKeyExt))
			os.WriteFile(s.collectorVerify, b, 0o600)
		}, "", string(ui.KeyHasPrivate)},
		{"a signing key not listed", func() {
			_, other := keygenV016(t, idOpenSSL)
			b, _ := os.ReadFile(s.collectorVerify)
			kept := []string{}
			for _, e := range strings.Split(strings.TrimPrefix(strings.TrimSpace(string(b)), "PQCOTA_VERIFY_KEY="), ",") {
				if !strings.HasPrefix(e, idOpenSSL+":") {
					kept = append(kept, e)
				}
			}
			kept = append(kept, idOpenSSL+":"+other)
			os.WriteFile(s.collectorVerify, []byte("PQCOTA_VERIFY_KEY="+strings.Join(kept, ",")+"\n"), 0o644)
		}, "", idOpenSSL + ": not listed"},
		{"one key under two IDs", func() {
			b, _ := os.ReadFile(s.collectorVerify)
			line := strings.TrimSpace(string(b))
			_, first, _ := strings.Cut(strings.Split(strings.TrimPrefix(line, "PQCOTA_VERIFY_KEY="), ",")[0], ":")
			os.WriteFile(s.collectorVerify, []byte(line+",jvm-collector:"+first+"\n"), 0o644)
		}, "", "one key under"},
		{"the approval key as a signing key", func() {
			b, _ := os.ReadFile(filepath.Join(s.collectorKeys, idOpenSSL+collectorKeyExt))
			ak := filepath.Join(t.TempDir(), "approval.key")
			// 같은 키쌍을 승인 키 꼴(수집기 ID 없는 검증 줄)로 둔다.
			text := strings.Replace(string(b), "PQCOTA_VERIFY_KEY="+idOpenSSL+":", "PQCOTA_VERIFY_KEY=", 1)
			os.WriteFile(ak, []byte(text), 0o600)
			s.approvalKey = ak
		}, "", idOpenSSL + ": the approval key"},
	} {
		fresh()
		s.approvalKey = ""
		c.change()
		k, v := keys()
		if c.keyBad != "" && (k.OK || !strings.Contains(k.Detail, c.keyBad)) {
			t.Errorf("%s: signing keys %+v, want blocked with %q", c.name, k, c.keyBad)
		}
		if c.verBad != "" && (v.OK || !strings.Contains(v.Detail, c.verBad)) {
			t.Errorf("%s: trusted keys %+v, want blocked with %q", c.name, v, c.verBad)
		}
		if !k.Blocking || !v.Blocking {
			t.Errorf("%s: the key checks must block", c.name)
		}
	}
	if got := neededCollectorIDs(true); strings.Join(got, ",") != "openssl-collector,network-collector,jvm-collector" {
		t.Errorf("with collector.jar the IDs are %v", got)
	}
}

// IC-U211 — **관측할 노드에 Windows가 있으면 실행 전에 막는다.** 이번 판의 02는 Windows 노드에 서명 키를 넘기지 않는다. 02 점검에
// 「Windows 노드」 줄이 그 노드 ID와 함께 막는 항목으로 나오고, 주소로 직접 시작해도 아무것도 실행하지 않은 「실패」다(점검 플레이북도
// 돌지 않는다).
func TestWindowsNodesAreRefusedBeforeTheRun(t *testing.T) {
	s := runReady(t, "ok")
	b, err := os.ReadFile(s.hosts)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.hosts, append(b, []byte("win-1,win,10.0.0.9,22,admin,,,"+inventory.OSWindows+",ssh\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	c := checkOf(s.preflight(), ui.RunWindowsUnsupported)
	if c == nil || c.OK || !c.Blocking || !strings.Contains(c.Detail, "win-1") {
		t.Fatalf("the Windows check = %+v", c)
	}
	if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusConflict {
		t.Errorf("start from the screen = %d, want 409", w.Code)
	}
	if err := s.startRun(5); err != nil {
		t.Fatal(err)
	}
	st := waitRun(t, s)
	if st.State != ui.RunFailed || !strings.Contains(st.Log, "Windows nodes are not observed") || strings.Contains(st.Log, "CHECK ran") {
		t.Errorf("state %s\n%s", st.State, st.Log)
	}
}
