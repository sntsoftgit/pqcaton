// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package scope_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	kscope "github.com/randyinthedev-hash/pqcota-common/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

func policyFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "policy.csv")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func fileDigest(t *testing.T, body string) string {
	t.Helper()
	d, err := scope.PolicyFileDigest(policyFile(t, body))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// IC-S16 — note · 주석 줄 · 빈 줄 · 머리글 · 칸 앞 공백만 다른 두 정책은 지문이 같다. 판정에 쓰이지 않는 것이
// 지문을 바꾸면 설명을 다듬을 때마다 「정책이 바뀌었다」는 알림이 뜬다.
func TestPolicyDigestIgnoresWhatDoesNotDecide(t *testing.T) {
	a := fileDigest(t, "action,runtime,lib,app_key,note\n"+
		"exclude,openssl,libcrypto.so.*,/usr/bin/python*,python runtime\n"+
		"include,openssl,libssl.so.3,*,payment gateway\n")
	b := fileDigest(t, "# reviewed 2026-11-06\n\n"+
		"exclude, openssl, libcrypto.so.*, /usr/bin/python*, a runtime pulled in by a package\n"+
		"\n"+
		"include,openssl,libssl.so.3,*\n")
	if a != b {
		t.Errorf("only notes, comments, blank lines and the header differ, but the digests differ:\n %s\n %s", a, b)
	}
	if !strings.HasPrefix(a, scope.PolicyDigestPrefix) {
		t.Errorf("digest %q does not carry its version %q", a, scope.PolicyDigestPrefix)
	}
}

// IC-S17 — 같은 규칙을 순서만 바꾼 두 정책은 지문이 다르다. 상류가 고르는 자산도 실제로 다르다(뒤 규칙이 이긴다).
func TestPolicyDigestKeepsRuleOrder(t *testing.T) {
	excludeThenInclude := &kscope.AssetPolicy{Rules: []kscope.AssetRule{ex("openssl", "*", "/usr/bin/pay*", ""), inc("openssl", "libssl.so.3", "*", "")}}
	includeThenExclude := &kscope.AssetPolicy{Rules: []kscope.AssetRule{inc("openssl", "libssl.so.3", "*", ""), ex("openssl", "*", "/usr/bin/pay*", "")}}
	if scope.PolicyDigest(excludeThenInclude) == scope.PolicyDigest(includeThenExclude) {
		t.Fatal("the rule order changed and the digest did not")
	}
	f := finding("libssl.so.3", "/usr/bin/payd")
	if excludeThenInclude.Managed(f) == includeThenExclude.Managed(f) {
		t.Fatal("precondition: upstream should decide differently when the order flips")
	}
}

// IC-S18 — 빈 칸과 `*`는 지문이 같다. 패턴의 대소문자는 가린다(상류 glob이 가린다).
func TestPolicyDigestTreatsEmptyAsAny(t *testing.T) {
	empty := scope.PolicyDigest(&kscope.AssetPolicy{Rules: []kscope.AssetRule{ex("", "", "/usr/sbin/sshd*", "")}})
	star := scope.PolicyDigest(&kscope.AssetPolicy{Rules: []kscope.AssetRule{ex("*", "*", "/usr/sbin/sshd*", "")}})
	if empty != star {
		t.Errorf("an empty field and * mean the same, but the digests differ")
	}
	upper := scope.PolicyDigest(&kscope.AssetPolicy{Rules: []kscope.AssetRule{ex("OpenSSL", "*", "/usr/sbin/sshd*", "")}})
	lower := scope.PolicyDigest(&kscope.AssetPolicy{Rules: []kscope.AssetRule{ex("openssl", "*", "/usr/sbin/sshd*", "")}})
	if upper == lower {
		t.Errorf("upstream matches patterns case-sensitively, so OpenSSL and openssl must not share a digest")
	}
}

// IC-S19 — 정책 파일이 없는 것 · 빈 경로 · 규칙 0개 파일은 지문이 같다. 읽을 수 없는 파일과 형식이 틀린 파일은
// **빈 정책이 되지 않고 오류다.** 빈 정책으로 읽으면 깨진 정책이 「제한 없음」으로 비교를 통과한다.
func TestPolicyDigestOfMissingAndBrokenFiles(t *testing.T) {
	none := scope.PolicyDigest(nil)
	missing, err := scope.PolicyFileDigest(filepath.Join(t.TempDir(), "absent.csv"))
	if err != nil {
		t.Fatal(err)
	}
	noPath, err := scope.PolicyFileDigest("")
	if err != nil {
		t.Fatal(err)
	}
	for name, d := range map[string]string{
		"missing file":     missing,
		"empty path":       noPath,
		"header only":      fileDigest(t, "action,runtime,lib,app_key,note\n"),
		"empty file":       fileDigest(t, ""),
		"no rules at all":  scope.PolicyDigest(&kscope.AssetPolicy{}),
		"comment and gaps": fileDigest(t, "# nothing excluded yet\n\n"),
	} {
		if d != none {
			t.Errorf("%s: digest %s, want the no-rule digest %s", name, d, none)
		}
	}
	for name, path := range map[string]string{
		"a directory":    t.TempDir(),
		"a bad action":   policyFile(t, "action,runtime,lib,app_key,note\ndrop,openssl,libssl.so.3,*,\n"),
		"a broken quote": policyFile(t, "action,runtime,lib,app_key,note\nexclude,\"openssl,libssl.so.3,*,\n"),
	} {
		if d, err := scope.PolicyFileDigest(path); err == nil {
			t.Errorf("%s: read as digest %s, want an error (never the empty policy)", name, d)
		}
	}
}
