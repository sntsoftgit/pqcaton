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

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

// IC-W37 — **-recover-status는 고치지 않고 보여 준다.** 남은 기록마다 파일의 전체 경로 · 기대 해시 ·
// 지금 해시 · 준비 파일의 상태와 판정(mismatch 같은)을 보이고, 진단 묶음을 0600으로 새로 낸다. 남은
// 기록이 없으면 그렇다고 말한다. 파일은 한 바이트도 바뀌지 않는다.
func TestRecoverStatus(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, store.DirName)
	var b strings.Builder
	if err := printRecoverStatus(&b, stateDir, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "no unfinished transactions") {
		t.Errorf("a clean state directory: %s", b.String())
	}

	target := filepath.Join(dir, "session.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("stopped")
	st, err := store.Open(stateDir, store.Options{Fault: func(point, path string) error {
		if point == store.FaultApply && path == "0" {
			return stop
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AcquireRun(); err != nil {
		t.Fatal(err)
	}
	id := store.Identity{Kind: "review", Org: "local", SessionID: "s-1"}
	if err := st.Save(id, []store.Change{{Path: target, Content: []byte("{\"new\":1}\n")}}, time.Second); !errors.Is(err, stop) {
		t.Fatalf("err = %v, want the injected stop", err)
	}
	st.Close()
	if err := os.WriteFile(target, []byte("edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	b.Reset()
	diag := filepath.Join(dir, "diag.json")
	if err := printRecoverStatus(&b, stateDir, diag); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"mismatch", target, "review/local/s-1", store.Sum([]byte("{\"new\":1}\n"))[:12],
		store.Sum([]byte("edited by hand\n"))[:12], "present, matches", "keep a copy"} {
		if !strings.Contains(out, want) {
			t.Errorf("the status does not show %q:\n%s", want, out)
		}
	}
	fi, err := os.Stat(diag)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("diagnostics: %v %v", err, fi)
	}
	if b, _ := os.ReadFile(target); string(b) != "edited by hand\n" {
		t.Error("-recover-status changed the file")
	}
}
