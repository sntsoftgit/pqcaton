// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/ingest"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/normalize"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/resultio"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/rulesetround"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// memStore — 메모리 이력을 upstream.Store로.
type memStore struct{ upstream.Lookup }

func (memStore) Close() {}

// ingestedUnder — 서버의 결과 폴더를 정책 파일 policyPath로 적재한 메모리 pqcota 이력. 그 정책 파일을 지금 읽는다.
func ingestedUnder(t *testing.T, s *server, policyPath string, into *history.MemStore) *history.MemStore {
	t.Helper()
	policy, err := review.LoadAssetPolicy(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, flaws := resultio.LoadDir(s.results)
	if len(flaws) != 0 {
		t.Fatal(flaws)
	}
	if into == nil {
		into = history.NewMemStore()
	}
	if _, err := ingest.IngestWith(loaded, ingest.IngestOptions{SnapshotPrefix: "t" + time.Now().Format("150405.000"),
		RulesetVersion: normalize.RulesetVersion, Store: into, AssetPolicy: policy}); err != nil {
		t.Fatal(err)
	}
	return into
}

// useUpstream — 서버의 pqcota 이력을 l로 건다(시작할 때 연 것과 같은 자리).
func useUpstream(s *server, l upstream.Lookup) {
	s.upstream, s.upstreamOrgName = memStore{l}, s.orgName()
}

// downLookup — 조회가 실패하는 pqcota 이력.
type downLookup struct{}

func (downLookup) ByContentHashV1(string, string, string) (*history.Snapshot, error) {
	return nil, errors.New("dial tcp: connection refused")
}
func (downLookup) Latest(string) (*history.Snapshot, error) {
	return nil, errors.New("dial tcp: connection refused")
}

// changingLookup — 대조하는 동안 결과 폴더에 관측을 하나 더한다(대조와 잠금 사이에 입력이 바뀌는 경우).
type changingLookup struct {
	upstream.Lookup
	change func()
	once   sync.Once
}

func (c *changingLookup) Latest(node string) (*history.Snapshot, error) {
	c.once.Do(c.change)
	return c.Lookup.Latest(node)
}

// activationReady — 정책 A로 판정한 세션 · 02에서 확정한 정책 B가 있는 서버. pqcota 이력은 아직 비어 있다.
func activationReady(t *testing.T) (*server, review.Session) {
	t.Helper()
	s, _ := keyReady(t)
	observed(t, s.results, "web", "libssl.so.3", "/usr/sbin/nginx")
	observed(t, s.results, "web", "libcrypto.so.3", "/usr/bin/python3")
	setPolicy(t, s, policyA)
	old := savedSession(t, s)
	finalizeCandidate(t, s, policyB)
	useUpstream(s, history.NewMemStore())
	return s, old
}

// IC-U196 — **화면은 pqcota 이력 없이 시작하지 않는다.** PQCOTA_DSN이 없거나, 연결이 실패하거나, 필수 조회(스키마 · 읽기
// 권한)가 실패하면 종료 코드 1로 끝나고 포트를 열지 않으며 주소 파일을 쓰지 않는다. 표준오류에는 DSN의 사용자 · 비밀번호 ·
// 호스트가 없다. 빈 메모리 이력이면 뜬다.
func TestScreenDoesNotStartWithoutPqcota(t *testing.T) {
	oldServe, oldOpen, oldUp, oldStop := serveHTTP, openBrowser, openUpstream, stopSignal
	t.Cleanup(func() { serveHTTP, openBrowser, openUpstream, stopSignal = oldServe, oldOpen, oldUp, oldStop })
	openBrowser = func(string, io.Writer) {}
	stopSignal = func() <-chan os.Signal { c := make(chan os.Signal); close(c); return c }
	served := false
	serveHTTP = func(net.Listener, http.Handler) error { served = true; return nil }
	const dsn = "postgres://secretuser:secretpw@127.0.0.1:1/pqcota?connect_timeout=2"
	for name, c := range map[string]struct {
		dsn  string
		open func(string, string) (upstream.Store, string, error)
	}{
		"no PQCOTA_DSN": {"", oldUp},
		"unreachable":   {dsn, upstream.OpenPg},
		"probe fails": {dsn, func(_, org string) (upstream.Store, string, error) {
			return memStore{downLookup{}}, org, nil
		}},
	} {
		t.Run(name, func(t *testing.T) {
			configDir(t) // 사용자 설정 폴더에 pqcota.env가 없다
			t.Setenv(upstream.DSNEnv, c.dsn)
			openUpstream = c.open
			served = false
			ws := t.TempDir()
			var stderr bytes.Buffer
			if code := run([]string{"-workspace", ws, "-no-browser", "-addr", "127.0.0.1:0"}, io.Discard, &stderr); code != 1 || served {
				t.Fatalf("exit %d, served %v\n%s", code, served, stderr.String())
			}
			if _, err := os.Stat(filepath.Join(ws, store.DirName, addrFile)); err == nil {
				t.Error("an address file was written")
			}
			for _, secret := range []string{"secretuser", "secretpw", "127.0.0.1:1"} {
				if strings.Contains(stderr.String(), secret) {
					t.Errorf("stderr carries %q: %s", secret, stderr.String())
				}
			}
			if !strings.Contains(stderr.String(), "pqcota history") {
				t.Errorf("stderr does not name the pqcota history: %s", stderr.String())
			}
		})
	}
	t.Run("empty history", func(t *testing.T) {
		openUpstream = oldUp
		served = false
		if code := run([]string{"-workspace", t.TempDir(), "-no-browser", "-addr", "127.0.0.1:0"}, io.Discard, io.Discard); code != 0 || !served {
			t.Errorf("exit %d, served %v", code, served)
		}
	})
}

// IC-U197 — **05의 활성화는 pqcota 이력 대조를 거쳐야 한다.** 활성화 상자에 운영자 진술 칸이 없고 대조 단계를 적는다. pqcota가
// 옛 정책으로만 적재돼 있으면 확인 요청이 409 `upstream-missing`이고, 닿지 못하면 503 `upstream-unreachable`이다. 새 정책으로
// 적재돼 있으면 확인 페이지가 「확인 시점에 pqcota 이력과 일치했습니다」와 노드 수를 적는다. 확인 페이지 뒤에 pqcota가 다른
// 정책으로 다시 적재되면 실행이 409 `upstream-missing`이다(실행은 처음부터 다시 대조한다). 거절은 모두 작업 폴더를 그대로 둔다.
// 운영자 진술 칸을 보내도 대조를 건너뛰지 않는다.
func TestActivationReconcilesPqcota(t *testing.T) {
	s, _ := activationReady(t)
	dir := filepath.Dir(s.path)
	page := get(t, s, ui.ScreenReview+"?lang=ko").Body.String()
	if strings.Contains(page, `type="checkbox" name="reingested"`) || !strings.Contains(page, "pqcota 재적재 결과 대조") ||
		!strings.Contains(page, "하나라도 없거나 다르거나 조회할 수 없으면 활성화하지 않습니다") {
		t.Fatal("the activation box still takes a statement or does not describe the reconciliation")
	}
	before := snapshotDir(t, dir)
	refused := func(what string, w interface {
		Result() *http.Response
	}, code int, kind string) {
		t.Helper()
		res := w.Result()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != code || !strings.Contains(string(b), kind) {
			t.Errorf("%s = %d, want %d %s", what, res.StatusCode, code, kind)
		}
	}
	useUpstream(s, ingestedUnder(t, s, s.scopeAssets, nil))
	refused("confirm, pqcota under the old policy", postForm(t, s, ui.ReviewScopeActivateConfirmPath, url.Values{"reingested": {"on"}}),
		http.StatusConflict, ui.RefusedUpstreamMissing)
	useUpstream(s, downLookup{})
	refused("confirm, pqcota down", postForm(t, s, ui.ReviewScopeActivateConfirmPath, url.Values{}), http.StatusServiceUnavailable, ui.RefusedUpstreamUnreachable)
	sameDir(t, before, snapshotDir(t, dir))

	useUpstream(s, ingestedUnder(t, s, s.scopeOut, nil))
	w := postFormLang(t, s, ui.ReviewScopeActivateConfirmPath, "ko", url.Values{})
	if w.Code != http.StatusOK {
		t.Fatalf("confirm = %d\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "확인 시점에 pqcota 이력과 일치했습니다(노드 1개") || strings.Contains(body, "증명") || strings.Contains(body, "보장") {
		t.Error("the confirmation page does not say the history matched at the time of checking, or claims more")
	}
	useUpstream(s, ingestedUnder(t, s, s.scopeAssets, nil))
	refused("activate, pqcota re-ingested under the old policy", postForm(t, s, ui.ReviewScopeActivatePath, hiddenFields(body)),
		http.StatusConflict, ui.RefusedUpstreamMissing)
	sameDir(t, before, snapshotDir(t, dir))
}

// IC-U198 — **대조가 통과하면 활성화 트랜잭션과 upstream-verified 처리 기록이 함께 쓰인다.** 새 세션의 근거와 처리 기록의
// basis가 upstream-verified이고, 전환 보고서에 upstream_check(확인 시각 · 조직 · 노드별 지문)가 실린다. 05는 「확인 시점에
// pqcota 이력과 일치했습니다」를 보인다. 대조 도중에 결과 폴더가 바뀌면 잠금 안에서 409 `stale`이고 아무것도 쓰지 않는다.
func TestActivationWritesTheReconciliation(t *testing.T) {
	t.Run("inputs changed during the reconciliation", func(t *testing.T) {
		s, old := activationReady(t)
		dir := filepath.Dir(s.path)
		mem := ingestedUnder(t, s, s.scopeOut, nil)
		useUpstream(s, mem)
		w := postForm(t, s, ui.ReviewScopeActivateConfirmPath, url.Values{})
		if w.Code != http.StatusOK {
			t.Fatalf("confirm = %d", w.Code)
		}
		before := snapshotDir(t, dir)
		useUpstream(s, &changingLookup{Lookup: mem, change: func() {
			observed(t, s.results, "db", "libssl.so.3", "/usr/sbin/nginx")
		}})
		if w := postForm(t, s, ui.ReviewScopeActivatePath, hiddenFields(w.Body.String())); w.Code != http.StatusConflict ||
			!strings.Contains(w.Body.String(), ui.RefusedStale) {
			t.Fatalf("activate = %d, want 409 %s", w.Code, ui.RefusedStale)
		}
		after := snapshotDir(t, dir)
		for p := range after {
			if _, ok := before[p]; !ok && !strings.Contains(p, "results") {
				t.Errorf("a file was written: %s", p)
			}
		}
		if rc, _ := s.st.RoundReceiptOf(rulesetround.ScopeActivationReceipt, store.Identity{Kind: kindReview, Org: s.orgName(), SessionID: old.SessionID}); rc != nil {
			t.Error("a record was written for inputs that were not reconciled")
		}
	})
	t.Run("success", func(t *testing.T) {
		s, old := activationReady(t)
		useUpstream(s, ingestedUnder(t, s, s.scopeOut, nil))
		w := postForm(t, s, ui.ReviewScopeActivateConfirmPath, url.Values{})
		if w.Code != http.StatusOK {
			t.Fatalf("confirm = %d", w.Code)
		}
		if w := postForm(t, s, ui.ReviewScopeActivatePath, hiddenFields(w.Body.String())); w.Code != http.StatusSeeOther {
			t.Fatalf("activate = %d\n%s", w.Code, w.Body.String())
		}
		next, err := review.Load(s.path)
		if err != nil || next.ScopeChange == nil || next.ScopeChange.Basis != rulesetround.BasisUpstream {
			t.Fatalf("new session basis: %+v %v", next.ScopeChange, err)
		}
		rc, err := s.st.RoundReceiptOf(rulesetround.ScopeActivationReceipt, store.Identity{Kind: kindReview, Org: s.orgName(), SessionID: old.SessionID})
		if err != nil || rc == nil || rc.Round == nil || rc.Round.Basis != rulesetround.BasisUpstream {
			t.Fatalf("record basis: %+v %v", rc, err)
		}
		b, err := os.ReadFile(filepath.Join(filepath.Dir(s.path), next.ScopeChange.Report))
		if err != nil {
			t.Fatal(err)
		}
		rep, err := review.DecodeReport(b)
		if err != nil || rep.UpstreamCheck == nil || rep.UpstreamCheck.Org != s.orgName() || rep.UpstreamCheck.CheckedAt == "" ||
			len(rep.UpstreamCheck.Snapshots) == 0 {
			t.Fatalf("report upstream check: %+v %v", rep.UpstreamCheck, err)
		}
		if page := get(t, s, ui.ScreenReview+"?lang=ko").Body.String(); !strings.Contains(page, "확인 시점에 pqcota 이력과 일치했습니다") {
			t.Error("05 does not show the reconciliation as the basis")
		}
	})
}

// IC-U199 — **이전 정책 복귀는 그 정책으로 다시 적재하기 전에는 막는다.** A로 적재한 뒤 B로 다시 적재한 pqcota 이력에서 A를
// 활성화하려 하면, A의 결과가 이력에 있어도 노드의 최신이 아니므로 409 `upstream-revert`이고 아무것도 쓰지 않는다. 화면은
// 다시 적재한 뒤 활성화하라고 안내한다. 운영자 진술로 풀 길은 없다. A로 다시 적재한 뒤의 통과는 pkg/inventory/upstream의
// Postgres 대조 시험이 잰다.
func TestRevertingToAnEarlierPolicyIsBlocked(t *testing.T) {
	s, _ := keyReady(t)
	observed(t, s.results, "web", "libssl.so.3", "/usr/sbin/nginx")
	observed(t, s.results, "web", "libcrypto.so.3", "/usr/bin/python3")
	setPolicy(t, s, policyB)
	savedSession(t, s)
	finalizeCandidate(t, s, policyA)
	dir := filepath.Dir(s.path)
	mem := ingestedUnder(t, s, s.scopeOut, nil) // A
	time.Sleep(1100 * time.Millisecond)
	ingestedUnder(t, s, s.scopeAssets, mem) // 그 뒤 B
	useUpstream(s, mem)
	before := snapshotDir(t, dir)
	w := postFormLang(t, s, ui.ReviewScopeActivateConfirmPath, "ko", url.Values{"reingested": {"on"}})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedUpstreamRevert) ||
		!strings.Contains(w.Body.String(), "이 정책으로 결과를 다시 적재한 뒤 활성화하십시오") {
		t.Fatalf("reverting = %d, want 409 %s", w.Code, ui.RefusedUpstreamRevert)
	}
	sameDir(t, before, snapshotDir(t, dir))
}

// countingOpen — openUpstream 자리. 몇 번 불렸는지와 받은 DSN · 조직을 남긴다.
type countingOpen struct {
	calls    int
	dsn, org string
	lookup   upstream.Lookup
}

func (c *countingOpen) open(dsn, org string) (upstream.Store, string, error) {
	c.calls++
	c.dsn, c.org = dsn, org
	l := c.lookup
	if l == nil {
		l = history.NewMemStore()
	}
	return memStore{l}, org, nil
}

// IC-U200 — **pqcota.env는 데이터로만, 안전한 파일에서만 읽는다.** PQCOTA_DSN이 환경에 있으면 그것이 먼저다. 비었으면 사용자
// 설정 폴더의 pqcaton/pqcota.env를 읽는다. 링크 · 보통 파일이 아님 · 그룹이나 다른 사용자에게 권한이 있음(0640 · 0604 ·
// 0620) · 하드 링크가 둘 · 다른 변수 · 따옴표 · 두 번 적기 · 빈 값 · PQCOTA_DSN 줄 없음은 모두 거절하고 화면이 시작하지
// 않는다. 셸 문법(`$(…)` · 백틱)은 평가하지 않고 값 그대로이며, 값 안의 `=`는 첫 `=`에서만 나눠 그대로 남는다. 사용자 설정
// 폴더가 링크로 작업 폴더 안을 가리키면 실제 경로로 보아 거절한다. 어느 오류에도 값이 없다. 화면이 띄우는 실제 자식
// 프로세스(감독 · Ansible의 관측과 정리)의 환경에는 PQCOTA_DSN이 없고, 화면 표준오류 · 관측 로그에도 값이 없다(관측 프로세스는
// 유닉스에서만 띄우므로 그 부분은 upstream_unix_test.go에 있다).
func TestPqcotaEnvFileIsReadSafely(t *testing.T) {
	const secret = "postgres://ro:s3cretpw@db.internal/pqcota"
	s, _ := newServer(t)
	t.Run("environment first", func(t *testing.T) {
		dir := configDir(t)
		if err := os.WriteFile(filepath.Join(dir, "pqcaton", dsnFile), []byte("PQCOTA_DSN=postgres://file\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(upstream.DSNEnv, secret)
		if v, _, err := s.loadDSN(); err != nil || v != secret {
			t.Errorf("got %q, %v; want the environment", v, err)
		}
	})
	t.Setenv(upstream.DSNEnv, "")
	write := func(t *testing.T, body string, mode os.FileMode) string {
		t.Helper()
		dir := configDir(t)
		p := filepath.Join(dir, "pqcaton", dsnFile)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	t.Run("good file", func(t *testing.T) {
		write(t, "# pqcota\n\nPQCOTA_DSN="+secret+"\n", 0o600)
		if v, where, err := s.loadDSN(); err != nil || v != secret || strings.Contains(where, secret) {
			t.Errorf("got %q from %q, %v", v, where, err)
		}
	})
	t.Run("shell syntax is data", func(t *testing.T) {
		write(t, "PQCOTA_DSN=postgres://u:$(touch /tmp/pqcaton-pwned)`id`@h/db\n", 0o600)
		if v, _, err := s.loadDSN(); err != nil || v != "postgres://u:$(touch /tmp/pqcaton-pwned)`id`@h/db" {
			t.Errorf("got %q, %v", v, err)
		}
	})
	t.Run("missing", func(t *testing.T) {
		configDir(t)
		if _, _, err := s.loadDSN(); !errors.Is(err, upstream.ErrNoDSN) {
			t.Errorf("err = %v, want ErrNoDSN", err)
		}
	})
	refused := func(t *testing.T, what string) {
		t.Helper()
		_, _, err := s.loadDSN()
		if !errors.Is(err, errDSNFile) {
			t.Fatalf("%s: err = %v, want errDSNFile", what, err)
		}
		if strings.Contains(err.Error(), "s3cretpw") || strings.Contains(err.Error(), "db.internal") {
			t.Errorf("%s: the error carries the value: %v", what, err)
		}
	}
	for _, mode := range []os.FileMode{0o640, 0o604, 0o620, 0o660} {
		t.Run(fmt.Sprintf("mode %04o", mode), func(t *testing.T) {
			write(t, "PQCOTA_DSN="+secret+"\n", mode)
			refused(t, fmt.Sprintf("mode %04o", mode))
		})
	}
	for name, body := range map[string]string{
		"another variable": "PQCOTA_DSN=" + secret + "\nPATH=/tmp\n",
		"export":           "export PQCOTA_DSN=" + secret + "\n",
		"quoted":           "PQCOTA_DSN=\"" + secret + "\"\n",
		"twice":            "PQCOTA_DSN=" + secret + "\nPQCOTA_DSN=" + secret + "\n",
		"empty value":      "PQCOTA_DSN=\n",
		"no line":          "# nothing here\n",
		"spaces":           "PQCOTA_DSN= " + secret + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			write(t, body, 0o600)
			refused(t, name)
		})
	}
	t.Run("symbolic link", func(t *testing.T) {
		dir := configDir(t)
		real := filepath.Join(t.TempDir(), "real.env")
		if err := os.WriteFile(real, []byte("PQCOTA_DSN="+secret+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, filepath.Join(dir, "pqcaton", dsnFile)); err != nil {
			t.Fatal(err)
		}
		refused(t, "symbolic link")
	})
	t.Run("an = inside the value is kept", func(t *testing.T) {
		const v = "postgres://ro:pw=x@h/db?sslmode=disable&application_name=pqcaton=ui"
		write(t, "PQCOTA_DSN="+v+"\n", 0o600)
		if got, _, err := s.loadDSN(); err != nil || got != v {
			t.Errorf("got %q, %v; want the value split only at the first =", got, err)
		}
	})
	t.Run("inside the workspace through a linked parent", func(t *testing.T) {
		// 사용자 설정 폴더가 작업 폴더 안을 가리키는 링크여도 실제 경로로 보아 거절한다.
		ws := t.TempDir()
		real := filepath.Join(ws, "conf")
		if err := os.MkdirAll(filepath.Join(real, "pqcaton"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(real, "pqcaton", dsnFile), []byte("PQCOTA_DSN="+secret+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(t.TempDir(), "config")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		old := userConfigDir
		userConfigDir = func() (string, error) { return link, nil }
		t.Cleanup(func() { userConfigDir = old })
		prevWS := s.workspace
		s.workspace = ws
		t.Cleanup(func() { s.workspace = prevWS })
		refused(t, "inside the workspace")
	})
	t.Run("hard link", func(t *testing.T) {
		p := write(t, "PQCOTA_DSN="+secret+"\n", 0o600)
		if err := os.Link(p, p+".copy"); err != nil {
			t.Fatal(err)
		}
		refused(t, "hard link")
	})
	t.Run("directory", func(t *testing.T) {
		dir := configDir(t)
		if err := os.Mkdir(filepath.Join(dir, "pqcaton", dsnFile), 0o700); err != nil {
			t.Fatal(err)
		}
		refused(t, "directory")
	})
	t.Run("the screen starts from the file and its children do not get the value", func(t *testing.T) {
		oldServe, oldOpen, oldUp, oldStop := serveHTTP, openBrowser, openUpstream, stopSignal
		t.Cleanup(func() { serveHTTP, openBrowser, openUpstream, stopSignal = oldServe, oldOpen, oldUp, oldStop })
		openBrowser = func(string, io.Writer) {}
		stopSignal = func() <-chan os.Signal { c := make(chan os.Signal); close(c); return c }
		serveHTTP = func(net.Listener, http.Handler) error { return nil }
		co := &countingOpen{}
		openUpstream = co.open
		write(t, "PQCOTA_DSN="+secret+"\n", 0o600)
		var stderr bytes.Buffer
		if code := run([]string{"-workspace", t.TempDir(), "-no-browser", "-addr", "127.0.0.1:0"}, io.Discard, &stderr); code != 0 || co.dsn != secret {
			t.Fatalf("exit %d, dsn given %v\n%s", code, co.dsn == secret, stderr.String())
		}
		if strings.Contains(stderr.String(), "s3cretpw") {
			t.Error("stderr carries the value")
		}
		bad := write(t, "PQCOTA_DSN="+secret+"\n", 0o644)
		co.calls = 0
		stderr.Reset()
		if code := run([]string{"-workspace", t.TempDir(), "-no-browser", "-addr", "127.0.0.1:0"}, io.Discard, &stderr); code != 1 || co.calls != 0 ||
			!strings.Contains(stderr.String(), "chmod 600") || strings.Contains(stderr.String(), "s3cretpw") {
			t.Errorf("an open file (%s): exit %d, opened %d times\n%s", bad, code, co.calls, stderr.String())
		}
		t.Setenv(upstream.DSNEnv, secret)
		for _, e := range childEnv() {
			if strings.HasPrefix(e, upstream.DSNEnv+"=") {
				t.Error("a child process would get PQCOTA_DSN")
			}
		}
	})
}

// IC-U201 — **pqcota 이력의 조직은 판정 조직이다.** -upstream-org에 다른 조직을 주면 화면이 시작하지 않고 pqcota 이력을 열지도
// 않는다. 같은 값은 받는다. 판정 세션의 조직이 판정 조직과 다르면(다른 조직에서 옮겨 온 세션) 05의 활성화가 pqcota 이력을
// 조회하지 않고 409 `upstream-org`이며 아무것도 쓰지 않는다. 연 이력의 조직이 다를 때도 같다.
func TestPqcotaOrganizationIsTheJudgmentsOrganization(t *testing.T) {
	oldServe, oldOpen, oldUp, oldStop := serveHTTP, openBrowser, openUpstream, stopSignal
	t.Cleanup(func() { serveHTTP, openBrowser, openUpstream, stopSignal = oldServe, oldOpen, oldUp, oldStop })
	openBrowser = func(string, io.Writer) {}
	stopSignal = func() <-chan os.Signal { c := make(chan os.Signal); close(c); return c }
	serveHTTP = func(net.Listener, http.Handler) error { return nil }
	co := &countingOpen{}
	openUpstream = co.open
	var stderr bytes.Buffer
	if code := run([]string{"-workspace", t.TempDir(), "-no-browser", "-addr", "127.0.0.1:0", "-org", "acme", "-upstream-org", "other-corp"},
		io.Discard, &stderr); code != 1 || co.calls != 0 {
		t.Fatalf("another organization: exit %d, opened %d times\n%s", code, co.calls, stderr.String())
	}
	if code := run([]string{"-workspace", t.TempDir(), "-no-browser", "-addr", "127.0.0.1:0", "-org", "acme", "-upstream-org", "acme"},
		io.Discard, io.Discard); code != 0 || co.calls != 1 || co.org != "acme" {
		t.Fatalf("the same organization: exit %d, opened %d times for %q", code, co.calls, co.org)
	}

	for name, tamper := range map[string]func(s *server){
		"session of another organization": func(s *server) {
			sf, err := review.Load(s.path)
			if err != nil {
				t.Fatal(err)
			}
			sf.Scope = "org://other-corp"
			if err := review.Save(s.path, sf); err != nil {
				t.Fatal(err)
			}
		},
		"history of another organization": func(s *server) { s.upstreamOrgName = "other-corp" },
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := activationReady(t)
			counted := &slowCount{Lookup: ingestedUnder(t, s, s.scopeOut, nil)}
			useUpstream(s, counted)
			tamper(s)
			dir := filepath.Dir(s.path)
			before := snapshotDir(t, dir)
			w := postForm(t, s, ui.ReviewScopeActivateConfirmPath, url.Values{})
			if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedUpstreamOrg) || counted.calls != 0 {
				t.Errorf("confirm = %d, looked up %d times; want 409 %s without a lookup", w.Code, counted.calls, ui.RefusedUpstreamOrg)
			}
			sameDir(t, before, snapshotDir(t, dir))
		})
	}
}

// slowCount — 조회 횟수만 세는 이력.
type slowCount struct {
	upstream.Lookup
	calls int
}

func (c *slowCount) ByContentHashV1(node, ruleset, digest string) (*history.Snapshot, error) {
	c.calls++
	return c.Lookup.ByContentHashV1(node, ruleset, digest)
}

func (c *slowCount) Latest(node string) (*history.Snapshot, error) {
	c.calls++
	return c.Lookup.Latest(node)
}
