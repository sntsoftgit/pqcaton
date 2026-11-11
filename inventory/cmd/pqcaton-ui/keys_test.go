// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/sign"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// 승인 키의 자리(keys.go). 시험은 사용자 설정 폴더를 임시 폴더로 바꾼다.

func configDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = old })
	if err := os.MkdirAll(filepath.Join(dir, "pqcaton"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// placeKeys: s 의 키 자리를 규칙(resolveKey)대로 다시 정한다.
func placeKeys(s *server, explicitKey, explicitVerify string, noDefault bool) {
	k, v := resolveKeys(explicitKey, explicitVerify, noDefault, userConfigDir)
	s.approvalKey, s.keySource, s.keyWhere = k.path, k.source, k.where
	s.verifyKey, s.verifySource, s.verifyWhere = v.path, v.source, v.where
}

func writePub(t *testing.T, p, pub string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(p, []byte("# verify\nPQCOTA_VERIFY_KEY="+pub+"\n"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
}

func privateOf(t *testing.T, p string) string {
	t.Helper()
	return privateKeyIn([]byte(readFile(t, p)))
}

func approvePage(t *testing.T, s *server, lang string) string {
	t.Helper()
	return get(t, s, ui.ScreenApprove+"?lang="+lang).Body.String()
}

// IC-U171 — **명시한 -approval-key 가 언제나 우선한다.** 기본 위치에 다른 키가 있어도, -no-default-approval-key 를 함께 줘도
// 명시한 키로 서명한다.
func TestExplicitKeyWins(t *testing.T) {
	dir := configDir(t)
	writeKey(t, filepath.Join(dir, "pqcaton", "approval.key"))
	for _, noDefault := range []bool{false, true} {
		s, _ := keyReady(t)
		explicit := s.approvalKey
		placeKeys(s, explicit, "", noDefault)
		if s.approvalKey != explicit || s.keySource != ui.KeyExplicit {
			t.Fatalf("noDefault=%v: key %s (%s), want the explicit %s", noDefault, s.approvalKey, s.keySource, explicit)
		}
		k, _ := s.readKey()
		if page := approvePage(t, s, "ko"); !strings.Contains(page, "지정한 승인 키를 사용합니다") || !strings.Contains(page, k.Fingerprint) {
			t.Errorf("noDefault=%v: the screen does not use the explicit key", noDefault)
		}
		if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
			t.Errorf("noDefault=%v: sign with the explicit key = %d", noDefault, w.Code)
		}
	}
}

// IC-U172 — **-no-default-approval-key 는 기본 검색만 끈다.** 기본 위치에 키가 있어도 쓰지 않고, 06 은 「자동 검색을
// 껐습니다」를 보이며 서명 양식을 내지 않는다.
func TestNoDefaultKeyTurnsTheSearchOff(t *testing.T) {
	dir := configDir(t)
	pub := writeKey(t, filepath.Join(dir, "pqcaton", "approval.key"))
	writePub(t, filepath.Join(dir, "pqcaton", "approval.pub"), pub, 0o644)
	s, _ := keyReady(t)
	placeKeys(s, "", "", true)
	if s.approvalKey != "" || s.verifyKey != "" {
		t.Fatalf("the default keys are still used: %q %q", s.approvalKey, s.verifyKey)
	}
	page := approvePage(t, s, "ko")
	if !strings.Contains(page, "기본 승인 키 자동 검색을 껐습니다") || strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) {
		t.Errorf("the screen does not say the search is off, or offers signing:\n%s", page)
	}
}

// IC-U173 — **기본 위치에 키가 없으면 정상 시작하고 새 서명만 하지 않는다.**
func TestDefaultKeyMissing(t *testing.T) {
	configDir(t)
	s, _ := keyReady(t)
	placeKeys(s, "", "", false)
	var stderr bytes.Buffer
	if err := s.checkKeysAtStart(&stderr); err != nil {
		t.Fatalf("a missing default key must not stop the start: %v", err)
	}
	if !strings.Contains(stderr.String(), "approval key: none at ") {
		t.Errorf("stderr does not say where it looked:\n%s", stderr.String())
	}
	page := approvePage(t, s, "ko")
	if !strings.Contains(page, "승인 키가 없습니다") || strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) {
		t.Errorf("the screen does not say there is no key, or offers signing")
	}
}

// IC-U174 — **기본 위치의 정상 키를 쓴다. 화면에는 사용자 설정 폴더 기준의 위치와 지문만 보이고, 개인 키는 어디에도 없다.**
func TestDefaultKeyIsUsed(t *testing.T) {
	dir := configDir(t)
	keyFile := filepath.Join(dir, "pqcaton", "approval.key")
	writeKey(t, keyFile)
	s, _ := keyReady(t)
	placeKeys(s, "", "", false)
	var stderr bytes.Buffer
	if err := s.checkKeysAtStart(&stderr); err != nil {
		t.Fatal(err)
	}
	k, _ := s.readKey()
	priv := privateOf(t, keyFile)
	for lang, want := range map[string][]string{
		"ko": {"기본 승인 키를 사용합니다.", "위치: 사용자 설정 폴더의 pqcaton/approval.key", "키 지문: " + k.Fingerprint},
		"en": {"The default approval key is used.", "Location: pqcaton/approval.key in the user configuration folder"},
	} {
		page := approvePage(t, s, lang)
		for _, w := range want {
			if !strings.Contains(page, w) {
				t.Errorf("%s: the screen lacks %q", lang, w)
			}
		}
		if strings.Contains(page, priv) || strings.Contains(page, dir) {
			t.Errorf("%s: the screen shows the private key or the full configuration path", lang)
		}
	}
	if strings.Contains(stderr.String(), priv) || !strings.Contains(stderr.String(), keyFile) {
		t.Errorf("stderr must name the full path and never the private key:\n%s", stderr.String())
	}
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign with the default key = %d", w.Code)
	}
	if b := readFile(t, s.signedPath()); strings.Contains(b, priv) {
		t.Error("the signed plan holds the private key")
	}
}

// IC-U175 — **기본 위치의 키가 잘못됐으면(권한 · 심볼릭 링크) 정상 시작하되, 06 과 표준 오류에 분명히 알리고 서명을 막는다.**
func TestDefaultKeyUnusable(t *testing.T) {
	for name, setup := range map[string]func(t *testing.T, keyFile string){
		"open": func(t *testing.T, keyFile string) {
			writeKey(t, keyFile)
			os.Chmod(keyFile, 0o644)
		},
		"symlink": func(t *testing.T, keyFile string) {
			real := filepath.Join(t.TempDir(), "real.key")
			writeKey(t, real)
			if err := os.Symlink(real, keyFile); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := configDir(t)
			setup(t, filepath.Join(dir, "pqcaton", "approval.key"))
			s, _ := keyReady(t)
			placeKeys(s, "", "", false)
			var stderr bytes.Buffer
			if err := s.checkKeysAtStart(&stderr); err != nil {
				t.Fatalf("an unusable default key must not stop the start: %v", err)
			}
			if !strings.Contains(stderr.String(), "warning: approval key at ") || !strings.Contains(stderr.String(), "new approval signatures are blocked") {
				t.Errorf("stderr does not warn:\n%s", stderr.String())
			}
			page := approvePage(t, s, "ko")
			if !strings.Contains(page, "기본 승인 키를 사용할 수 없습니다") || strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) {
				t.Errorf("the screen does not refuse the default key, or offers signing")
			}
			w := postForm(t, s, ui.SignConfirmPath, withRev(signForm("보안팀장"), s.revFor(ui.ScreenApprove)))
			if w.Code != http.StatusConflict {
				t.Errorf("sign = %d, want 409", w.Code)
			}
		})
	}
}

// IC-U176 — **명시한 키가 잘못됐으면 화면을 시작하지 않는다.** 서명 키도 확인용 공개 키도 같다. 조용히 다른 모드로 열지 않는다.
func TestExplicitUnusableKeyStopsTheStart(t *testing.T) {
	configDir(t)
	bad := filepath.Join(t.TempDir(), "approval.key")
	writeKey(t, bad)
	os.Chmod(bad, 0o644)
	for _, args := range [][]string{{"-approval-key", bad}, {"-approval-verify-key", filepath.Join(t.TempDir(), "missing.pub")}} {
		var stderr bytes.Buffer
		ws := t.TempDir()
		code := run(append([]string{"-workspace", ws, "-no-browser", "-addr", "127.0.0.1:0"}, args...), &bytes.Buffer{}, &stderr)
		if code != 1 || !strings.Contains(stderr.String(), "cannot be used") || !strings.Contains(stderr.String(), "nothing was started") {
			t.Errorf("%v: exit %d, want 1 with a reason:\n%s", args, code, stderr.String())
		}
		if _, err := os.Stat(filepath.Join(ws, ".pqcaton", "screen.addr")); err == nil {
			t.Errorf("%v: an address was recorded although the screen did not start", args)
		}
	}
}

// IC-U177 — **개인 키가 없어도 이미 서명된 계획은 확인용 공개 키로 확인하고 생성한다.** 확인용 공개 키도 없으면 확인하지
// 못한 것으로 두고 그 파일을 두라고 안내한다. 새 서명은 어느 경우에도 하지 않는다.
func TestSignedPlansGenerateWithoutThePrivateKey(t *testing.T) {
	dir := configDir(t)
	s, pub := keyReady(t)
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d", w.Code)
	}
	placeKeys(s, "", "", false) // 기본 위치에는 아직 아무것도 없다: 개인 키도 확인용 공개 키도 없다
	page := approvePage(t, s, "ko")
	if !strings.Contains(page, "확인용 공개 키(pqcaton/approval.pub)를 두십시오") || strings.Contains(page, `action="`+ui.GeneratePath+`"`) {
		t.Errorf("without any key the screen should ask for the verification key and not generate")
	}

	writePub(t, filepath.Join(dir, "pqcaton", "approval.pub"), pub, 0o644)
	placeKeys(s, "", "", false)
	page = approvePage(t, s, "ko")
	for _, want := range []string{"승인 키가 없습니다", "확인용 공개 키로 계속 확인하고 생성할 수 있습니다", "확인용 공개 키: pqcaton/approval.pub", `action="` + ui.GeneratePath + `"`} {
		if !strings.Contains(page, want) {
			t.Errorf("with the verification key the screen lacks %q", want)
		}
	}
	if strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) {
		t.Error("signing is offered without the private key")
	}
	w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("generate with the verification key = %d\n%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(s.provisionPath()); err != nil {
		t.Error("no playbook was generated")
	}
}

// IC-U178 — **기본 위치의 개인 키와 확인용 공개 키가 서로 다른 키면 서명도 확인도 하지 않는다.** 둘 다 명시했는데 다르면
// 화면을 시작하지 않는다(운영자가 고른 짝이 틀렸다).
func TestKeyAndVerifyKeyMustMatch(t *testing.T) {
	dir := configDir(t)
	s, _ := keyReady(t)
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d", w.Code)
	}
	other, _, err := sign.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(s.approvalKey, filepath.Join(dir, "pqcaton", "approval.key")); err != nil {
		t.Fatal(err)
	}
	writePub(t, filepath.Join(dir, "pqcaton", "approval.pub"), other, 0o644)
	placeKeys(s, "", "", false)
	page := approvePage(t, s, "en")
	if !strings.Contains(page, "are different keys") || strings.Contains(page, `action="`+ui.GeneratePath+`"`) {
		t.Errorf("a mismatched default pair should block checking and generation")
	}
	var stderr bytes.Buffer
	if err := s.checkKeysAtStart(&stderr); err != nil || !strings.Contains(stderr.String(), "are different keys") {
		t.Errorf("stderr does not warn about the mismatch (%v):\n%s", err, stderr.String())
	}

	pubFile := filepath.Join(t.TempDir(), "other.pub")
	writePub(t, pubFile, other, 0o644)
	placeKeys(s, filepath.Join(dir, "pqcaton", "approval.key"), pubFile, false)
	if err := s.checkKeysAtStart(&bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "nothing was started") {
		t.Errorf("an explicit mismatched pair must stop the start, got %v", err)
	}
}

// IC-U179 — **확인용 공개 키 파일은 개인 키 줄을 담지 않고, 다른 사용자가 고칠 수 없어야 한다.** 읽기는 넓게 열어도 된다.
func TestVerifyKeyFileRules(t *testing.T) {
	dir := configDir(t)
	s, pub := keyReady(t)
	keyText := readFile(t, s.approvalKey) // 아래에서 placeKeys 가 개인 키 자리를 비우므로 먼저 읽어 둔다
	pubFile := filepath.Join(dir, "pqcaton", "approval.pub")
	for name, c := range map[string]struct {
		write func()
		want  ui.KeyState
	}{
		"readable by others": {func() { writePub(t, pubFile, pub, 0o644) }, ui.KeyOK},
		"writable by others": {func() { writePub(t, pubFile, pub, 0o666) }, ui.KeyUnwritable},
		"holds a private key": {func() {
			os.Remove(pubFile)
			os.WriteFile(pubFile, []byte(keyText), 0o600)
		}, ui.KeyHasPrivate},
	} {
		c.write()
		placeKeys(s, "", pubFile, false)
		if got := s.readVerify().State; got != c.want {
			t.Errorf("%s: verification key state %s, want %s", name, got, c.want)
		}
	}
}

// IC-U180 — **명시한 개인 키가 있으면 기본 공개 키는 읽지 않는다.** 기본 위치에 다른 키의 approval.pub 이 있어도 명시한 키에서
// 공개 키를 유도해 서명 · 확인 · 생성한다.
func TestExplicitKeyIgnoresTheDefaultVerifyKey(t *testing.T) {
	dir := configDir(t)
	other, _, _ := sign.Generate()
	writePub(t, filepath.Join(dir, "pqcaton", "approval.pub"), other, 0o644)
	s, _ := keyReady(t)
	placeKeys(s, s.approvalKey, "", false)
	if s.verifyKey != "" || s.verifySource != ui.KeyOtherGiven {
		t.Fatalf("the default verification key is read next to an explicit key: %q (%s)", s.verifyKey, s.verifySource)
	}
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d", w.Code)
	}
	if w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("generate = %d", w.Code)
	}
}

// IC-U181 — **명시한 공개 키만 있으면 확인 · 생성만 한다.** 기본 위치의 개인 키는 읽지 않는다(새 서명을 하지 않는다).
func TestExplicitVerifyKeyOnly(t *testing.T) {
	dir := configDir(t)
	s, pub := keyReady(t)
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d", w.Code)
	}
	if err := os.Rename(s.approvalKey, filepath.Join(dir, "pqcaton", "approval.key")); err != nil {
		t.Fatal(err)
	}
	pubFile := filepath.Join(t.TempDir(), "approval.pub")
	writePub(t, pubFile, pub, 0o644)
	placeKeys(s, "", pubFile, false)
	if s.approvalKey != "" {
		t.Fatalf("the default private key is read next to an explicit verification key: %s", s.approvalKey)
	}
	page := approvePage(t, s, "ko")
	if !strings.Contains(page, "확인용 공개 키만 지정했습니다") || strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) ||
		!strings.Contains(page, `action="`+ui.GeneratePath+`"`) {
		t.Errorf("with only a verification key, signing must be off and generation on")
	}
	if w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("generate = %d", w.Code)
	}
}

// IC-U182 — **잘못된 기본 개인 키와 정상 기본 공개 키가 함께 있으면 서명은 막고 확인 · 생성은 한다.**
func TestBrokenDefaultKeyWithGoodVerifyKey(t *testing.T) {
	dir := configDir(t)
	s, pub := keyReady(t)
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d", w.Code)
	}
	keyFile := filepath.Join(dir, "pqcaton", "approval.key")
	if err := os.Rename(s.approvalKey, keyFile); err != nil {
		t.Fatal(err)
	}
	os.Chmod(keyFile, 0o644)
	writePub(t, filepath.Join(dir, "pqcaton", "approval.pub"), pub, 0o644)
	placeKeys(s, "", "", false)
	page := approvePage(t, s, "ko")
	if !strings.Contains(page, "기본 승인 키를 사용할 수 없습니다") || strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) {
		t.Errorf("a broken default key must block signing")
	}
	if !strings.Contains(page, "확인용 공개 키로 계속 확인하고 생성할 수 있습니다") || !strings.Contains(page, `action="`+ui.GeneratePath+`"`) {
		t.Errorf("a good default verification key must still allow generation")
	}
	if w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("generate = %d", w.Code)
	}
}

// IC-U183 — **정상 개인 키와 쓸 수 없는 공개 키를 함께 고르면 구성 오류로 서명도 생성도 막는다.**
func TestGoodKeyWithBrokenVerifyKeyIsAConfigurationError(t *testing.T) {
	dir := configDir(t)
	s, pub := keyReady(t)
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d", w.Code)
	}
	if err := os.Rename(s.approvalKey, filepath.Join(dir, "pqcaton", "approval.key")); err != nil {
		t.Fatal(err)
	}
	writePub(t, filepath.Join(dir, "pqcaton", "approval.pub"), pub, 0o666)
	placeKeys(s, "", "", false)
	page := approvePage(t, s, "ko")
	if !strings.Contains(page, "구성이 맞지 않습니다") || strings.Contains(page, `action="`+ui.SignConfirmPath+`"`) ||
		strings.Contains(page, `action="`+ui.GeneratePath+`"`) {
		t.Errorf("a usable key with a broken verification key must block signing and generation")
	}
	var stderr bytes.Buffer
	if err := s.checkKeysAtStart(&stderr); err != nil || !strings.Contains(stderr.String(), "signing and generation are blocked") {
		t.Errorf("stderr does not report the configuration error (%v):\n%s", err, stderr.String())
	}
	if w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove))); w.Code == http.StatusSeeOther {
		t.Error("generation went through a configuration error")
	}
}

// IC-U192 — **실제 os.UserConfigDir() 의 기본 위치에서, 공백이 든 경로여도 개인 키와 공개 키를 읽는다.** macOS 는
// ~/Library/Application Support/pqcaton/, Linux 는 $XDG_CONFIG_HOME/pqcaton/ 이다. 사용자 설정 폴더를 바꿔치기하지 않고 진짜
// 함수로 잰다(HOME · XDG_CONFIG_HOME 만 공백이 든 임시 폴더로 둔다). 명시 키 우선과 -no-default-approval-key 도 같은 자리에서 본다.
func TestRealUserConfigDirWithSpaces(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("the screen saves only on Linux and macOS")
	}
	home := filepath.Join(t.TempDir(), "home with space")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config dir"))
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "config dir")
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, "Library", "Application Support")
	}
	if dir != want {
		t.Fatalf("os.UserConfigDir() = %q, want %q", dir, want)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pqcaton"), 0o700); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(dir, "pqcaton", "approval.key")
	pub := writeKey(t, keyFile)
	writePub(t, filepath.Join(dir, "pqcaton", "approval.pub"), pub, 0o644)

	s, _ := keyReady(t)
	k, v := resolveKeys("", "", false, os.UserConfigDir)
	s.approvalKey, s.keySource, s.keyWhere = k.path, k.source, k.where
	s.verifyKey, s.verifySource, s.verifyWhere = v.path, v.source, v.where
	if s.approvalKey != keyFile || !strings.Contains(s.approvalKey, " ") {
		t.Fatalf("the default key path is %q, want %q", s.approvalKey, keyFile)
	}
	key, priv := s.readKey()
	if key.State != ui.KeyOK || priv == "" || s.readVerify().State != ui.KeyOK {
		t.Fatalf("keys at a path with spaces: key %s, verify %s", key.State, s.readVerify().State)
	}
	page := approvePage(t, s, "ko")
	if !strings.Contains(page, "기본 승인 키를 사용합니다.") || !strings.Contains(page, "사용자 설정 폴더의 pqcaton/approval.key") ||
		strings.Contains(page, priv) || strings.Contains(page, home) {
		t.Errorf("the screen does not use the default key at the real location, or shows too much")
	}
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign with the key at a path with spaces = %d", w.Code)
	}

	other := filepath.Join(t.TempDir(), "explicit key.key")
	writeKey(t, other)
	k, v = resolveKeys(other, "", false, os.UserConfigDir)
	if k.path != other || k.source != ui.KeyExplicit || v.path != "" {
		t.Errorf("an explicit key did not win over the real default location: %+v %+v", k, v)
	}
	k, v = resolveKeys("", "", true, os.UserConfigDir)
	if k.path != "" || v.path != "" || k.source != ui.KeySearchOff {
		t.Errorf("-no-default-approval-key still reads the real default location: %+v %+v", k, v)
	}
}
