// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	invscope "github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

const (
	scopeA = "action,runtime,lib,app_key,note\nexclude,*,*,/usr/sbin/sshd*,managed by OS patching\n"
	scopeB = "action,runtime,lib,app_key,note\nexclude,*,*,/usr/bin/python*,package runtime\n"
)

func writePolicy(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// sessionUnder: ready()의 세션을 지문 digest로 연 것처럼 고친다. 빈 값이면 지문 칸이 없는 옛 파일이다.
func sessionUnder(t *testing.T, dir, digest string) string {
	t.Helper()
	sess := ready(t, dir)
	sf, err := review.Load(sess)
	if err != nil {
		t.Fatal(err)
	}
	sf.ScopePolicyDigestV1 = digest
	if err := review.Save(sess, sf); err != nil {
		t.Fatal(err)
	}
	return sess
}

func receiptOf(t *testing.T, sess string) *store.Receipt {
	t.Helper()
	st, err := store.Open(store.DefaultDir(sess), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rc, err := st.Finalized(store.Identity{Kind: "review", Org: "local", SessionID: "01234567-89ab-4cde-8f01-23456789abcd"})
	if err != nil {
		t.Fatal(err)
	}
	return rc
}

// IC-D29 — **정책 지문이 없는 옛 세션 파일은 close가 거절한다.** 빈 정책으로 읽지 않는다: 그렇게 읽으면 정책 파일로
// 세운 옛 세션을 정책 없이 닫아도 통과한다. -o로도 표준출력으로도 거절하고, 새 세션을 열라고 안내하며 아무것도 쓰지
// 않는다.
func TestCloseRefusesASessionWithoutPolicy(t *testing.T) {
	dir := t.TempDir()
	sess := sessionUnder(t, dir, "")
	before := read(t, sess)
	plan := filepath.Join(dir, "plan.json")
	for name, out := range map[string]string{"with -o": plan, "to stdout": ""} {
		got, err := stdout(t, func() error { return closeSession(sess, "", "local", out, "", "") })
		if !errors.Is(err, review.ErrScopePolicyMissing) || !strings.Contains(err.Error(), "open a new session") {
			t.Errorf("%s: err = %v, want ErrScopePolicyMissing with the way out", name, err)
		}
		if got != "" {
			t.Errorf("%s: a plan went to stdout", name)
		}
	}
	if read(t, plan) != "<absent>" || read(t, sess) != before {
		t.Error("a refused close wrote the plan or rewrote the session")
	}
}

// IC-D30 — **정책 파일로 연 세션은 같은 정책으로만 닫힌다.** 다른 정책 파일이나 -scope-assets 생략(「정책 없음」)은
// 거절하고 아무것도 쓰지 않는다. 같은 파일이면 확정하고, 영수증이 형식 2이며 그 정책의 지문을 적는다.
func TestCloseRefusesAChangedPolicy(t *testing.T) {
	dir := t.TempDir()
	a := writePolicy(t, dir, "scope-a.csv", scopeA)
	b := writePolicy(t, dir, "scope-b.csv", scopeB)
	digestA, err := invscope.PolicyFileDigest(a)
	if err != nil {
		t.Fatal(err)
	}
	sess := sessionUnder(t, dir, digestA)
	before := read(t, sess)
	plan := filepath.Join(dir, "plan.json")
	for name, given := range map[string]string{"another policy": b, "no -scope-assets": ""} {
		if _, err := stdout(t, func() error { return closeSession(sess, "", "local", plan, "", given) }); !errors.Is(err, review.ErrScopePolicy) {
			t.Errorf("%s: err = %v, want ErrScopePolicy", name, err)
		}
	}
	if read(t, plan) != "<absent>" || read(t, sess) != before || receiptOf(t, sess) != nil {
		t.Fatal("a refused close wrote something")
	}
	if _, err := stdout(t, func() error { return closeSession(sess, "", "local", plan, "", a) }); err != nil {
		t.Fatalf("close with the same policy: %v", err)
	}
	rc := receiptOf(t, sess)
	if got, ok := rc.PolicyDigest(); rc.Format != 2 || !ok || got != digestA {
		t.Errorf("receipt format %d digest %q %v; want format 2 with %s", rc.Format, got, ok, digestA)
	}
}

// IC-D31 — **정책 없이 연 세션은 -scope-assets 없이 닫힌다.** 「정책 없음」도 지문이 있고, 영수증은 형식 2로 그
// 지문(규칙 0개)을 적는다.
func TestCloseWithoutPolicyRecordsTheEmptyPolicy(t *testing.T) {
	dir := t.TempDir()
	sess := sessionUnder(t, dir, invscope.PolicyDigest(nil))
	plan := filepath.Join(dir, "plan.json")
	if _, err := stdout(t, func() error { return closeSession(sess, "", "local", plan, "", "") }); err != nil {
		t.Fatalf("close: %v", err)
	}
	rc := receiptOf(t, sess)
	if got, ok := rc.PolicyDigest(); rc.Format != 2 || !ok || got != invscope.PolicyDigest(nil) {
		t.Errorf("receipt format %d digest %q %v; want format 2 with the no-rule digest", rc.Format, got, ok)
	}
}

// IC-D32 — **open은 실제로 읽은 정책의 지문을 세션에 적는다.** -scope-assets를 주면 그 파일의 지문이고, 주지 않으면
// 규칙 0개의 지문이다. 지문 칸이 비어 있는 세션은 나오지 않는다.
func TestOpenRecordsThePolicyItRead(t *testing.T) {
	_, _, declPath, results := oldRound(t)
	a := writePolicy(t, t.TempDir(), "scope-a.csv", scopeA)
	want, err := invscope.PolicyFileDigest(a)
	if err != nil {
		t.Fatal(err)
	}
	for given, digest := range map[string]string{a: want, "": invscope.PolicyDigest(nil)} {
		sf, _, err := sessionFromResults(declPath, "local", results, given)
		if err != nil {
			t.Fatal(err)
		}
		if sf.ScopePolicyDigestV1 != digest {
			t.Errorf("open with -scope-assets %q recorded %q, want %q", given, sf.ScopePolicyDigestV1, digest)
		}
	}
}
