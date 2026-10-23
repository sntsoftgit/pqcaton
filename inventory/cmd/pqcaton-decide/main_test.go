// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

// ready: 확정을 통과하는 세션 파일. 화면이 쓰는 것과 같은 형식이다.
func ready(t *testing.T, dir string) string {
	t.Helper()
	sf := review.Session{
		Note: review.Note, Scope: "host://local", RulesetVersion: review.RulesetVersion,
		SessionID: "01234567-89ab-4cde-8f01-23456789abcd",
		Reviewer:  "보안팀", Signature: "sig",
		PolicyDecisions: map[string]string{"openssl/libssl": "PQC 라이브러리로 교체한다"},
		Items: []review.Item{{ID: "host://local/openssl/libssl", Policy: "openssl/libssl",
			Node: "host://local", Runtime: "openssl", State: "UNDECLARED", Conf: 0.6, Mandatory: true,
			Plan: true, Level: "L2", Kind: "REMEDIATION_KIND_CONFIG_ONLY", TargetAlgorithm: "ML-KEM (FIPS 203)"}},
	}
	p := filepath.Join(dir, "session.json")
	if err := review.Save(p, sf); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return "<absent>"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// stdout: 함수가 표준출력으로 낸 것.
func stdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = old
	b := make([]byte, 1<<20)
	n, _ := r.Read(b)
	return string(b[:n]), runErr
}

// IC-W33 — **원장에 쓰는 확정은 -o가 있어야 한다.** 원장과 표준출력은 한 트랜잭션으로 묶이지 않는다.
// -o 없이 -judgments를 주면 거부하고 아무것도 쓰지 않는다. -o도 -judgments도 없으면 예전처럼 표준출력으로만
// 계획을 내고 원장 · 영수증은 쓰지 않는다.
func TestCloseNeedsOutputForLedger(t *testing.T) {
	dir := t.TempDir()
	sess := ready(t, dir)
	ledger := filepath.Join(dir, "judgments.jsonl")
	out, err := stdout(t, func() error { return closeSession(sess, ledger, "local", "", "") })
	if !errors.Is(err, store.ErrNeedOutput) || !strings.Contains(err.Error(), "-o") {
		t.Fatalf("err = %v, want ErrNeedOutput naming -o", err)
	}
	if out != "" || read(t, ledger) != "<absent>" {
		t.Errorf("a refused close wrote something: stdout %q, ledger %q", out, read(t, ledger))
	}

	out, err = stdout(t, func() error { return closeSession(sess, "", "local", "", "") })
	if err != nil || !strings.Contains(out, "IN_REVIEW") {
		t.Fatalf("stdout close: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, store.DirName)); !os.IsNotExist(err) {
		t.Error("the stdout-only close made a state directory (receipt)")
	}
}

// IC-W34 — **close -o는 화면과 같은 트랜잭션이다.** 세션 · 계획 · 판정 원장 · 영수증을 함께 쓰고,
// 같은 세션을 다시 확정하면 아무것도 다시 쓰지 않는다(원장에 두 번 쌓이지 않는다). 표준출력은 비어 있다.
func TestCloseToFileIsOnce(t *testing.T) {
	dir := t.TempDir()
	sess := ready(t, dir)
	plan := filepath.Join(dir, "plan.json")
	ledger := filepath.Join(dir, "judgments.jsonl")
	out, err := stdout(t, func() error { return closeSession(sess, ledger, "local", plan, "") })
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("close -o wrote to stdout: %q", out)
	}
	if !strings.Contains(read(t, plan), "IN_REVIEW") || !strings.Contains(read(t, ledger), `"org":"local"`) {
		t.Fatalf("plan or ledger missing:\n%s\n%s", read(t, plan), read(t, ledger))
	}
	snap := []string{read(t, sess), read(t, plan), read(t, ledger)}

	if _, err := stdout(t, func() error { return closeSession(sess, ledger, "local", plan, "") }); err != nil {
		t.Fatal(err)
	}
	if again := []string{read(t, sess), read(t, plan), read(t, ledger)}; strings.Join(again, "|") != strings.Join(snap, "|") {
		t.Error("closing the same session again changed the files")
	}
	st, err := store.Open(store.DefaultDir(sess), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rc, err := st.Finalized(store.Identity{Kind: "review", Org: "local", SessionID: "01234567-89ab-4cde-8f01-23456789abcd"})
	if err != nil || rc == nil {
		t.Fatalf("no receipt: %v", err)
	}
}

// IC-W35 — **화면이 떠 있으면 명령은 기다리지 않고 거부된다.** 서버가 실행 잠금을 쥔 동안 close -o는
// 곧바로 거부되고 아무것도 쓰지 않는다.
func TestCloseRefusedWhileServerRuns(t *testing.T) {
	dir := t.TempDir()
	sess := ready(t, dir)
	server, err := store.Open(store.DefaultDir(sess), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	plan := filepath.Join(dir, "plan.json")
	ledger := filepath.Join(dir, "judgments.jsonl")
	before := read(t, sess)
	start := time.Now()
	_, err = stdout(t, func() error { return closeSession(sess, ledger, "local", plan, "") })
	if !errors.Is(err, store.ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("waited %v; it must be refused at once", d)
	}
	if read(t, plan) != "<absent>" || read(t, ledger) != "<absent>" || read(t, sess) != before {
		t.Error("a refused close wrote something")
	}
}
